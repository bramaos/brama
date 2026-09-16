package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/shim"
	"github.com/bramaos/brama/internal/ssh"
)

const testVersion = "0.1.0"

// projectWithConfig writes a valid brama.yaml carrying the commented servers
// skeleton, which is the state `brama init` leaves behind.
func projectWithConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	body := `version: 1

app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads

environments:
  local:
    url: https://acme.local.test

# servers:
#   prod:
#     host: prod.example.com
`
	if err := os.WriteFile(filepath.Join(root, config.Filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakeServer stands in for a Server. It is a stand-in for an external system — a
// remote machine over a network — not for a collaborator inside brama, which is what
// makes substituting it fair game.
type fakeServer struct {
	platform      string // what `uname -sm` answers
	installed     string // version the Shim on the box reports; empty means none
	uploadReports string // version a freshly installed Shim will report
	runErr        error

	log     []string
	uploads int
}

func (f *fakeServer) Run(_ context.Context, command string) (string, error) {
	f.log = append(f.log, command)
	if f.runErr != nil {
		return "", f.runErr
	}
	switch {
	case strings.Contains(command, "uname -sm"):
		return f.platform, nil
	case strings.HasSuffix(command, "--version"):
		if f.installed == "" {
			return "", errors.New("no such file or directory")
		}
		return f.installed + "\n", nil
	case strings.Contains(command, "chmod"):
		f.installed = f.uploadReports
		return "", nil
	}
	return "", nil
}

func (f *fakeServer) Send(_ context.Context, command string, data []byte) error {
	f.log = append(f.log, command)
	if f.runErr != nil {
		return f.runErr
	}
	if len(data) == 0 {
		return errors.New("nothing sent")
	}
	f.uploads++
	return nil
}

func (f *fakeServer) Close() error { return nil }

func (f *fakeServer) ran(substr string) bool {
	for _, c := range f.log {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// installerFor wires the command to a stand-in Server, and reports whether it was
// ever dialled — some outcomes must be reached without touching the network at all.
func installerFor(f *fakeServer) (installer, *bool) {
	dialled := false
	inst := installer{
		version: testVersion,
		dial: func(context.Context, ssh.Target) (session, error) {
			dialled = true
			return f, nil
		},
		source: fakeShimSource,
	}
	return inst, &dialled
}

// fakeShimSource stands in for the embedded binaries, which a plain checkout does
// not have. It keeps the real matrix, so an unsupported platform still fails the way
// it would in production.
func fakeShimSource(p shim.Platform) ([]byte, error) {
	for _, supported := range shim.SupportedPlatforms() {
		if supported == p {
			return []byte("fake-shim-binary"), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", shim.ErrUnsupportedPlatform, p)
}

func linuxServer() *fakeServer {
	return &fakeServer{platform: "Linux x86_64", uploadReports: testVersion}
}

func TestServerAddRegistersTheServerAndInstallsTheShim(t *testing.T) {
	root := projectWithConfig(t)
	remote := linuxServer()
	inst, _ := installerFor(remote)
	env, _, _ := testEnv()

	err := runServerAdd(env, root, "prod", config.Server{Host: "hetzner-prod"}, inst)
	if err != nil {
		t.Fatalf("runServerAdd() = %v, want success", err)
	}

	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatalf("config does not load after the write: %v", err)
	}
	if got := cfg.Servers["prod"].Host; got != "hetzner-prod" {
		t.Errorf("servers.prod.host = %q, want hetzner-prod", got)
	}
	if remote.uploads != 1 {
		t.Errorf("uploads = %d, want the shim sent once", remote.uploads)
	}
	if !remote.ran(shim.Path + " --version") {
		t.Error("the installed shim was never run, so the install was never proved")
	}
}

// Two projects commonly share one Server. The second registration should not re-send
// a binary that is already there — but it must still prove the one that is there runs.
func TestServerAddSkipsTheUploadWhenTheShimIsCurrent(t *testing.T) {
	root := projectWithConfig(t)
	remote := linuxServer()
	remote.installed = testVersion
	inst, _ := installerFor(remote)
	env, _, _ := testEnv()

	if err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst); err != nil {
		t.Fatalf("runServerAdd() = %v", err)
	}

	if remote.uploads != 0 {
		t.Errorf("uploads = %d, want none for a shim already at this version", remote.uploads)
	}
	if !remote.ran(shim.Path + " --version") {
		t.Error("the existing shim was not verified")
	}
}

// A servers: entry claims the Server is reachable and runs a Shim. If the probe
// cannot establish that, nothing may be recorded claiming it.
func TestServerAddWritesNothingWhenTheProbeFails(t *testing.T) {
	root := projectWithConfig(t)
	before := readConfig(t, root)

	remote := linuxServer()
	remote.runErr = errors.New("connection refused")
	inst, _ := installerFor(remote)
	env, _, _ := testEnv()

	err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst)
	if err == nil {
		t.Fatal("runServerAdd() = nil, want the failure to propagate")
	}
	if got := readConfig(t, root); !bytes.Equal(before, got) {
		t.Errorf("brama.yaml was modified despite a failed probe:\n%s", got)
	}
}

func TestServerAddWithoutAConfigPointsAtInit(t *testing.T) {
	root := t.TempDir()
	remote := linuxServer()
	inst, dialled := installerFor(remote)
	env, _, _ := testEnv()

	err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst)
	if err == nil {
		t.Fatal("runServerAdd() = nil, want an error when there is no brama.yaml")
	}
	if !strings.Contains(err.Error(), "brama init") {
		t.Errorf("error = %q, want it to name `brama init`", err)
	}
	if *dialled {
		t.Error("a connection was opened before the config was found, want none")
	}
}

// Refusing a duplicate is a decision about the file, so it is reached without
// touching the Server.
func TestServerAddRefusesADuplicateWithoutConnecting(t *testing.T) {
	root := projectWithConfig(t)
	remote := linuxServer()
	inst, _ := installerFor(remote)
	env, _, _ := testEnv()

	if err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst); err != nil {
		t.Fatalf("first runServerAdd() = %v", err)
	}

	second := linuxServer()
	inst2, dialled := installerFor(second)
	err := runServerAdd(env, root, "prod", config.Server{Host: "other"}, inst2)
	if err == nil {
		t.Fatal("second runServerAdd() = nil, want it to refuse a name already registered")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error = %q, want it to say the name is already registered", err)
	}
	if *dialled {
		t.Error("the duplicate check opened a connection, want it decided from the file alone")
	}
}

// A brama built with plain `go build` embeds no shims. That is this binary's
// problem, not the Server's, so it is settled before a connection is opened rather
// than after a handshake and a `uname`.
func TestServerAddFailsBeforeConnectingWhenNoShimIsEmbedded(t *testing.T) {
	root := projectWithConfig(t)
	inst, dialled := installerFor(linuxServer())
	inst.embedded = func() bool { return false }
	env, _, _ := testEnv()

	err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst)
	if !errors.Is(err, shim.ErrNotBuilt) {
		t.Fatalf("err = %v, want ErrNotBuilt", err)
	}
	if *dialled {
		t.Error("a connection was opened by a build with no shim to install")
	}
}

// A Server brama has no build for is a limitation, not a guardrail: it fails, and it
// names what it found so the user can say what they need.
func TestServerAddReportsAPlatformWithNoBuild(t *testing.T) {
	root := projectWithConfig(t)
	remote := &fakeServer{platform: "FreeBSD amd64", uploadReports: testVersion}
	inst, _ := installerFor(remote)
	env, _, _ := testEnv()

	err := runServerAdd(env, root, "prod", config.Server{Host: "h"}, inst)
	if err == nil {
		t.Fatal("runServerAdd() = nil, want an error for an unsupported platform")
	}
	if !strings.Contains(err.Error(), "freebsd/amd64") {
		t.Errorf("error = %q, want it to name the detected platform", err)
	}
}

// --json is a documented contract. Pinning the keys makes a rename a deliberate act.
func TestServerAddResultJSONContract(t *testing.T) {
	var out, errOut bytes.Buffer
	env := &console{Out: &out, Err: &errOut, JSON: true, Renderer: renderer.NewJSON(&out)}

	result := &ServerAddResult{
		Name:     "prod",
		Host:     "hetzner-prod",
		Platform: "linux/amd64",
		Shim:     shim.Report{Version: testVersion, Uploaded: false},
	}
	if err := env.Renderer.Result(result); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}

	for key, want := range map[string]any{
		"action":        "server_add",
		"status":        "success",
		"server":        "prod",
		"host":          "hetzner-prod",
		"user":          nil,
		"platform":      "linux/amd64",
		"shim_version":  testVersion,
		"shim_uploaded": false,
	} {
		value, ok := got[key]
		if !ok {
			t.Errorf("missing contract key %q in %v", key, got)
			continue
		}
		if value != want {
			t.Errorf("%s = %v, want %v", key, value, want)
		}
	}
}

func readConfig(t *testing.T, root string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}
	return body
}
