package shim_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/shim"
)

// server stands in for a Server. It is a stand-in for an external system — a remote
// machine over a network — not for a collaborator inside brama.
type server struct {
	platform  string // what `uname -sm` answers
	installed string // version the Shim there reports; empty means no Shim
	reports   string // version a freshly installed Shim will report
	runErr    error

	log     []string
	uploads int
}

func newServer(installed string) *server {
	return &server{platform: "Linux x86_64", installed: installed, reports: "0.2.0"}
}

func (s *server) Run(_ context.Context, command string) (string, error) {
	s.log = append(s.log, command)
	if s.runErr != nil {
		return "", s.runErr
	}
	switch {
	case strings.Contains(command, "uname -sm"):
		return s.platform, nil
	case strings.HasSuffix(command, "--version"):
		if s.installed == "" {
			return "", errors.New("no such file or directory")
		}
		return s.installed + "\n", nil
	case strings.Contains(command, "chmod"):
		s.installed = s.reports
		return "", nil
	}
	return "", nil
}

func (s *server) Send(_ context.Context, command string, data []byte) error {
	s.log = append(s.log, command)
	if s.runErr != nil {
		return s.runErr
	}
	if len(data) == 0 {
		return errors.New("nothing sent")
	}
	s.uploads++
	return nil
}

func (s *server) ran(substr string) bool {
	for _, c := range s.log {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// source stands in for the embedded binaries, which a plain checkout does not have.
// It keeps the real matrix, so an unsupported platform still fails as it would.
func source(p shim.Platform) ([]byte, error) {
	for _, supported := range shim.SupportedPlatforms() {
		if supported == p {
			return []byte("fake-shim-binary"), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", shim.ErrUnsupportedPlatform, p)
}

func ensure(t *testing.T, s *server, version string) shim.Step {
	t.Helper()
	step, err := shim.Ensure(context.Background(), s, version, source)
	if err != nil {
		t.Fatalf("Ensure() = %v, want success", err)
	}
	return step
}

func plan(t *testing.T, s *server, version string) shim.Step {
	t.Helper()
	step, err := shim.Plan(context.Background(), s, version, source)
	if err != nil {
		t.Fatalf("Plan() = %v, want success", err)
	}
	return step
}

func TestEnsureInstallsWhenNoShimIsThere(t *testing.T) {
	s := newServer("")
	step := ensure(t, s, "0.2.0")

	if step.Change != shim.ChangeInstall {
		t.Errorf("Change = %q, want install", step.Change)
	}
	if step.From != "" {
		t.Errorf("From = %q, want empty — there was nothing there", step.From)
	}
	if step.To != "0.2.0" {
		t.Errorf("To = %q, want 0.2.0", step.To)
	}
	if !step.Uploaded() {
		t.Error("Uploaded() = false, want the binary sent")
	}
	if s.uploads != 1 {
		t.Errorf("uploads = %d, want 1", s.uploads)
	}
}

// The whole of issue #4: an older Shim is brought up to the CLI's build, and the step
// says what changed rather than just that something did.
func TestEnsureUpgradesAnOlderShim(t *testing.T) {
	s := newServer("0.1.0")
	step := ensure(t, s, "0.2.0")

	if step.Change != shim.ChangeUpgrade {
		t.Errorf("Change = %q, want upgrade", step.Change)
	}
	if step.From != "0.1.0" || step.To != "0.2.0" {
		t.Errorf("Step = %s → %s, want 0.1.0 → 0.2.0", step.From, step.To)
	}
	if s.uploads != 1 {
		t.Errorf("uploads = %d, want the newer build sent", s.uploads)
	}
}

// A matching version is left alone — but it is still run, because executing it is
// what proves the install rather than what the filesystem claims about it.
// See docs/adr/0006-server-global-shim-home.md.
func TestEnsureLeavesACurrentShimAloneButStillRunsIt(t *testing.T) {
	s := newServer("0.2.0")
	step := ensure(t, s, "0.2.0")

	if step.Change != shim.ChangeNone {
		t.Errorf("Change = %q, want none", step.Change)
	}
	if step.Changed() {
		t.Error("Changed() = true, want false")
	}
	if s.uploads != 0 {
		t.Errorf("uploads = %d, want none for a shim already at this version", s.uploads)
	}
	if !s.ran(shim.Path + " --version") {
		t.Error("the existing shim was not run, so the install was never proved")
	}
}

// The Shim and the brama that drives it must come from one build, so a Shim that is
// not this brama's is replaced whichever way it differs. Only the direction brama can
// name is called an upgrade.
func TestEnsureReplacesAShimItCannotCallOlder(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		cli       string
	}{
		{"newer on the server", "0.3.0", "0.2.0"},
		{"neither can be ordered", "dev", "0.2.0"},
		{"the cli cannot be ordered", "0.1.0", "dev"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newServer(c.installed)
			s.reports = c.cli
			step := ensure(t, s, c.cli)

			if step.Change != shim.ChangeReplace {
				t.Errorf("Change = %q, want replace", step.Change)
			}
			if step.From != c.installed || step.To != c.cli {
				t.Errorf("Step = %s → %s, want %s → %s", step.From, step.To, c.installed, c.cli)
			}
			if s.uploads != 1 {
				t.Errorf("uploads = %d, want this brama's build sent", s.uploads)
			}
		})
	}
}

// Two builds stamped "dev" are as alike as anything can tell. Treating them as a
// difference would reinstall the Shim on every run of a development build.
func TestEnsureTreatsIdenticalUnorderableVersionsAsCurrent(t *testing.T) {
	s := newServer("dev")
	step := ensure(t, s, "dev")

	if step.Change != shim.ChangeNone {
		t.Errorf("Change = %q, want none", step.Change)
	}
	if s.uploads != 0 {
		t.Errorf("uploads = %d, want none", s.uploads)
	}
}

func TestEnsureFailsWhenTheInstalledShimReportsSomethingElse(t *testing.T) {
	s := newServer("0.1.0")
	s.reports = "0.1.0" // the swap silently did not take

	_, err := shim.Ensure(context.Background(), s, "0.2.0", source)
	if err == nil {
		t.Fatal("Ensure() = nil, want the mismatch reported")
	}
	if !strings.Contains(err.Error(), "0.2.0") {
		t.Errorf("error = %q, want it to name the version that was sent", err)
	}
}

func TestEnsureNamesAPlatformItHasNoBuildFor(t *testing.T) {
	s := newServer("")
	s.platform = "FreeBSD amd64"

	step, err := shim.Ensure(context.Background(), s, "0.2.0", source)
	if !errors.Is(err, shim.ErrUnsupportedPlatform) {
		t.Fatalf("err = %v, want ErrUnsupportedPlatform", err)
	}
	if !strings.Contains(err.Error(), "freebsd/amd64") {
		t.Errorf("error = %q, want it to name the platform it found", err)
	}
	if step.Platform.String() != "freebsd/amd64" {
		t.Errorf("Platform = %s, want the detected platform carried on the step", step.Platform)
	}
}

// --dry-run reports the upgrade instead of performing it. Nothing crosses the
// connection, and the Server is left running what it was running.
func TestPlanReportsAPendingUpgradeWithoutPerformingIt(t *testing.T) {
	s := newServer("0.1.0")
	step := plan(t, s, "0.2.0")

	if step.Change != shim.ChangeUpgrade {
		t.Errorf("Change = %q, want upgrade", step.Change)
	}
	if !step.Pending {
		t.Error("Pending = false, want the change reported as not yet made")
	}
	if step.Uploaded() {
		t.Error("Uploaded() = true, want nothing sent for a plan")
	}
	if s.uploads != 0 {
		t.Errorf("uploads = %d, want none", s.uploads)
	}
	if s.installed != "0.1.0" {
		t.Errorf("the server now runs %q, want it left at 0.1.0", s.installed)
	}
	if got := step.Running(); got != "0.1.0" {
		t.Errorf("Running() = %q, want the version still on the server", got)
	}
}

// A plan that says "will upgrade" and is followed by a run that cannot is worse than
// no plan. The build is looked up before the change is promised, not after.
func TestPlanFailsWhenThereIsNoBuildForTheServer(t *testing.T) {
	s := newServer("0.1.0")
	s.platform = "FreeBSD amd64"

	if _, err := shim.Plan(context.Background(), s, "0.2.0", source); !errors.Is(err, shim.ErrUnsupportedPlatform) {
		t.Fatalf("err = %v, want ErrUnsupportedPlatform", err)
	}
}

// Nothing to do is not pending, even under --dry-run: there is no change to report.
func TestPlanOfACurrentShimIsNotPending(t *testing.T) {
	s := newServer("0.2.0")
	step := plan(t, s, "0.2.0")

	if step.Change != shim.ChangeNone {
		t.Errorf("Change = %q, want none", step.Change)
	}
	if step.Pending {
		t.Error("Pending = true, want false when there is nothing to do")
	}
	if got := step.Running(); got != "0.2.0" {
		t.Errorf("Running() = %q, want 0.2.0", got)
	}
}

func TestRunningIsTheVersionTheServerEndsUpWith(t *testing.T) {
	s := newServer("0.1.0")
	step := ensure(t, s, "0.2.0")

	if got := step.Running(); got != "0.2.0" {
		t.Errorf("Running() = %q, want 0.2.0", got)
	}
}

func TestEnsureReportsAnUnreachableServer(t *testing.T) {
	s := newServer("0.1.0")
	s.runErr = errors.New("connection refused")

	if _, err := shim.Ensure(context.Background(), s, "0.2.0", source); err == nil {
		t.Fatal("Ensure() = nil, want the connection failure to propagate")
	}
}
