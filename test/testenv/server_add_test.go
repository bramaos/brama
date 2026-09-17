//go:build testenv

package testenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestServerAdd registers both Servers with the real command, built the way a user
// builds it, reaching them through nothing but the ssh config.
//
// This is the criterion the fakes cannot reach: not "the command calls the installer"
// — internal/cli already proves that — but that a brama a developer compiled, handed
// only a host alias, ends up with a running Shim on a Server it had never seen.
func TestServerAdd(t *testing.T) {
	for _, alias := range []string{production, staging} {
		t.Run(alias, func(t *testing.T) {
			server := dial(t, alias)
			clearShimHome(t, server)

			project := projectDir(t)
			out := bramaJSON(t, project, "server", "add", "prod", "--host", alias)

			if got := out["status"]; got != "success" {
				t.Fatalf("status = %v, want success\n%v", got, out)
			}
			if got := out["platform"]; got != "linux/amd64" {
				t.Errorf("platform = %v, want linux/amd64", got)
			}
			if got := out["shim_change"]; got != "install" {
				t.Errorf("shim_change = %v, want install", got)
			}
			if got := out["shim_uploaded"]; got != true {
				t.Errorf("shim_uploaded = %v, want true", got)
			}

			// `--version` on the installed binary must match the calling brama. Both
			// ends of that are fetched independently: what `brama version` says about
			// itself, and what the binary on the Server prints when executed. Taking
			// the expected value from the shim_version field brama filled in about
			// the install it had just done would compare brama against itself.
			calling, _ := bramaJSON(t, project, "version")["version"].(string)
			if calling == "" {
				t.Fatal("brama version reported nothing")
			}
			assertRunning(t, server, calling)

			if got := out["shim_version"]; got != calling {
				t.Errorf("shim_version = %v, want %q", got, calling)
			}

			// The Shim lives under deploy's home, not root's: the login user is
			// unprivileged, and an install that needed sudo would have gone
			// unnoticed against a fake.
			if home := run(t, server, "ls -d ~/.brama"); home != "/home/deploy/.brama" {
				t.Errorf("shim home is %q, want /home/deploy/.brama", home)
			}

			// The file is written only once the Server has answered, so a servers:
			// entry always means "reachable, and the shim runs here".
			config, err := os.ReadFile(filepath.Join(project, "brama.yaml"))
			if err != nil {
				t.Fatalf("reading the written config: %v", err)
			}
			if !strings.Contains(string(config), "host: "+alias) {
				t.Errorf("brama.yaml does not record host %q:\n%s", alias, config)
			}
		})
	}
}

// TestServerAddDryRun connects, reports, and changes nothing — on both sides.
func TestServerAddDryRun(t *testing.T) {
	server := dial(t, production)
	clearShimHome(t, server)

	project := projectDir(t)
	before, err := os.ReadFile(filepath.Join(project, "brama.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	out := bramaJSON(t, project, "server", "add", "prod", "--host", production, "--dry-run")

	if got := out["shim_change"]; got != "install" {
		t.Errorf("shim_change = %v, want install — the plan still reports what it would do", got)
	}
	if got := out["shim_uploaded"]; got != false {
		t.Errorf("shim_uploaded = %v, want false", got)
	}

	if _, err := server.Run(t.Context(), "test -e ~/.brama/shim"); err == nil {
		t.Error("a dry run put a shim on the server")
	}

	after, err := os.ReadFile(filepath.Join(project, "brama.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("a dry run rewrote brama.yaml:\n%s", after)
	}
}

// projectDir is a throwaway project for the command to find and write back.
func projectDir(t *testing.T) string {
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
`
	if err := os.WriteFile(filepath.Join(root, "brama.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestMain owns the one binary every test in this file shares, so that building it
// once does not mean leaking its directory. A t.TempDir cannot be used: it belongs to
// whichever test happened to build first, and would be removed while later tests were
// still running it.
func TestMain(m *testing.M) {
	code := m.Run()
	if bramaDir != "" {
		_ = os.RemoveAll(bramaDir)
	}
	os.Exit(code)
}

var (
	bramaOnce sync.Once
	bramaDir  string
	bramaPath string
)

// bramaBinary compiles cmd/brama once per run, exactly as `make binary` does — shims
// embedded, version stamped to match them.
//
// A plain `go build` leaves main.version at its default, so the brama it produces
// reports `dev` while the builds `make shim` left in internal/shim/bin report the git
// version. brama installs a Shim and then asks it what it is, so that mismatch is a
// hard error — which is why `make binary` exists, and why this does what it does.
func bramaBinary(t *testing.T) string {
	t.Helper()

	bramaOnce.Do(func() {
		dir, err := os.MkdirTemp("", "brama-testenv-")
		if err != nil {
			t.Fatalf("creating a build directory: %v", err)
		}
		bramaDir = dir
		bramaPath = filepath.Join(dir, "brama")
		goBuild(t, "./cmd/brama", bramaPath, makeVersion(t))
	})
	return bramaPath
}

// bramaJSON runs the command in a project directory and decodes --json.
func bramaJSON(t *testing.T, project string, args ...string) map[string]any {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bramaBinary(t), append(args, "--json")...)
	cmd.Dir = project
	var stderr strings.Builder
	cmd.Stderr = &stderr

	// Both streams in the failure message: brama renders a failed result to stdout
	// like any other, so an error reported with stderr alone says only "exit 1".
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("brama %s: %v\n%s%s", strings.Join(args, " "), err, out, stderr.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decoding the result: %v\n%s", err, out)
	}
	return result
}
