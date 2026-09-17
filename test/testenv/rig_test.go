//go:build testenv

package testenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/shim"
	"github.com/bramaos/brama/internal/ssh"
)

// The Servers, by the aliases ssh-setup.sh writes. Verbose on purpose: a developer's
// ~/.ssh/config holds live production hosts, and a half-guessed name must not be able
// to land on one.
const (
	production = "brama-testenv-production"
	staging    = "brama-testenv-staging"
)

// repoRoot is the checkout this file was compiled from. Derived from the compiler's
// own record of the path rather than from the working directory, so a helper still
// finds the Makefile's tree when `go test` is run from anywhere.
var repoRoot = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testenv: cannot locate this source file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}()

// requireReachable fails the run, with the command that fixes it, when the alias
// these tests reach through is not in the developer's ssh config.
//
// `ssh -G` resolves the config without opening a connection, and an unknown alias
// resolves to itself on port 22 — so the identity file the rig generated is the
// thing to look for. This is a hard failure, not a skip: nothing compiled this
// package without someone asking for the `testenv` tag.
func requireReachable(t *testing.T, alias string) {
	t.Helper()

	key := filepath.Join(repoRoot, "test", "testenv", "ssh", "id_ed25519")

	out, err := exec.CommandContext(t.Context(), "ssh", "-G", alias).Output()
	if err == nil && strings.Contains(string(out), key) {
		return
	}

	t.Fatalf(`%s does not resolve to the rig.

The Servers are reached through your ssh config, exactly as a rented one is.
Nothing writes to ~/.ssh/config for you — that file grants production access.

    make testenv-up

then add the Include line it prints to the TOP of ~/.ssh/config.`, alias)
}

// dial opens a real multiplexed ssh connection to a Server, closed when the test
// ends. This is internal/ssh unchanged: the point of the rig is that nothing about
// brama is different because it is being tested.
func dial(t *testing.T, alias string) *ssh.Session {
	t.Helper()
	requireReachable(t, alias)

	session, err := ssh.Open(t.Context(), ssh.Target{Host: alias})
	if err != nil {
		t.Fatalf("reaching %s: %v", alias, err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("closing the session to %s: %v", alias, err)
		}
	})
	return session
}

// run executes a command on the Server and returns its trimmed output, failing the
// test if it does not succeed.
func run(t *testing.T, s *ssh.Session, command string) string {
	t.Helper()
	out, err := s.Run(t.Context(), command)
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return strings.TrimSpace(out)
}

// platformOf asks the Server what it is, through the same parser brama uses.
func platformOf(t *testing.T, s *ssh.Session) shim.Platform {
	t.Helper()
	platform, err := shim.ParsePlatform(run(t, s, "uname -sm"))
	if err != nil {
		t.Fatalf("reading the server's platform: %v", err)
	}
	return platform
}

// goBuild compiles a package from this checkout, stamping main.version the way the
// Makefile does. The version has to be stamped rather than left at its default,
// because brama installs a Shim and then asks it what it is: two binaries from one
// build must agree, and that agreement is the linker flag.
func goBuild(t *testing.T, pkg, out, version string, env ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "build",
		"-trimpath",
		"-ldflags", "-s -w -X main.version="+version,
		"-o", out, pkg)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), env...)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s stamped %s: %v\n%s", pkg, version, err, output)
	}
}

// buildShim compiles cmd/brama-shim stamped with an arbitrary version.
//
// The embedded builds all carry this brama's version, so upgrade and replace — which
// are defined by two versions differing — cannot be reached with them. Compiling is
// how the test gets a second one, and it produces the same binary `make shim` does,
// with the same linker flags, rather than a stand-in that only claims to be one.
func buildShim(t *testing.T, p shim.Platform, version string) []byte {
	t.Helper()

	out := filepath.Join(t.TempDir(), "shim")
	goBuild(t, "./cmd/brama-shim", out, version,
		"CGO_ENABLED=0", "GOOS="+p.OS, "GOARCH="+p.Arch)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the built shim: %v", err)
	}
	return data
}

// makeVersion is the version the Makefile stamps, read back from the Makefile rather
// than worked out a second time here. `make shim` has already used it on the builds
// now embedded in internal/shim/bin, and a brama that disagrees with them cannot
// install them.
func makeVersion(t *testing.T) string {
	t.Helper()

	// --no-print-directory, or `make -C` prefixes its "Entering directory" notice to
	// the value and the linker is handed a flag it cannot parse.
	out, err := exec.CommandContext(t.Context(),
		"make", "--no-print-directory", "-C", repoRoot, "print-VERSION").Output()
	if err != nil {
		t.Fatalf("reading VERSION from the Makefile: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// sourceOf adapts a fixed build to shim.Source. The Platform argument is ignored:
// the caller built for the Server's own, and a mismatch would be a bug in the test
// rather than something the test should tolerate.
func sourceOf(data []byte) shim.Source {
	return func(shim.Platform) ([]byte, error) { return data, nil }
}

// clearShimHome removes the Shim from a Server, so a test can start from the state a
// freshly rented Server is in. Registered as cleanup too: these tests share two
// long-lived Servers, and one that leaves a Shim behind changes what the next run
// starts from.
func clearShimHome(t *testing.T, s *ssh.Session) {
	t.Helper()
	remove := func() {
		if _, err := s.Run(context.WithoutCancel(t.Context()), "rm -rf ~/.brama"); err != nil {
			t.Errorf("clearing ~/.brama: %v", err)
		}
	}
	remove()
	t.Cleanup(remove)
}
