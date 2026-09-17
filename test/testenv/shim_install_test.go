//go:build testenv

package testenv

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/shim"
	"github.com/bramaos/brama/internal/ssh"
)

// TestShimInstall walks a real Server through every Change the version check can
// name, in the order a Server actually meets them.
//
// One test with ordered subtests rather than four independent ones: each Change is
// defined by what the Server was running beforehand, so the state left by the
// previous step *is* the fixture for the next. Splitting them would mean four Servers
// to set up, or four setups that re-create by hand what the step before produced.
func TestShimInstall(t *testing.T) {
	server := dial(t, production)
	clearShimHome(t, server)

	platform := platformOf(t, server)
	if platform != (shim.Platform{OS: "linux", Arch: "amd64"}) {
		t.Fatalf("the rig is linux/amd64 only, got %s", platform)
	}

	const (
		first = "v0.2.0"
		newer = "v0.3.0"
		older = "v0.1.0"
	)

	t.Run("install", func(t *testing.T) {
		step := ensure(t, server, platform, first)

		if step.Change != shim.ChangeInstall {
			t.Errorf("Change = %q, want %q", step.Change, shim.ChangeInstall)
		}
		if step.From != "" {
			t.Errorf("From = %q, want empty — nothing was installed", step.From)
		}
		if !step.Uploaded() {
			t.Error("Uploaded() = false, want true — a binary had to cross")
		}
		assertRunning(t, server, first)
	})

	t.Run("none", func(t *testing.T) {
		step := ensure(t, server, platform, first)

		if step.Change != shim.ChangeNone {
			t.Errorf("Change = %q, want %q", step.Change, shim.ChangeNone)
		}
		if step.Uploaded() {
			t.Error("Uploaded() = true, want false — the version already matched")
		}
		assertRunning(t, server, first)
	})

	t.Run("upgrade", func(t *testing.T) {
		step := ensure(t, server, platform, newer)

		if step.Change != shim.ChangeUpgrade {
			t.Errorf("Change = %q, want %q", step.Change, shim.ChangeUpgrade)
		}
		if step.From != first {
			t.Errorf("From = %q, want %q", step.From, first)
		}
		assertRunning(t, server, newer)
	})

	// A brama older than the Shim it finds installs its own build anyway: a Shim it
	// cannot drive protects nothing.
	// See docs/adr/0007-replace-a-shim-brama-cannot-call-older.md.
	t.Run("replace", func(t *testing.T) {
		step := ensure(t, server, platform, older)

		if step.Change != shim.ChangeReplace {
			t.Errorf("Change = %q, want %q", step.Change, shim.ChangeReplace)
		}
		if step.From != newer {
			t.Errorf("From = %q, want %q", step.From, newer)
		}
		assertRunning(t, server, older)
	})

	// Three installs have now happened on one Server. Two builds survive; the first is
	// pruned. Asserted here rather than inside `replace` because it is a property of
	// the sequence, not of any one step in it.
	t.Run("keeps two builds", func(t *testing.T) {
		names := strings.Fields(run(t, server, "ls -1 ~/.brama/shim-*"))
		if len(names) != 2 {
			t.Errorf("~/.brama holds %d builds, want 2: %v", len(names), names)
		}
	})
}

// TestShimInstallLeavesNoPartialFile is the atomicity claim, checked on a real
// filesystem.
//
// The install stages the upload under a `.part` name, chmods it, renames it to its
// versioned name, and renames a new symlink over the old one. None of those
// intermediate names may survive a successful install: a leftover `.part` means the
// staged copy was never moved, and a leftover `.shim.new` means the symlink was
// swapped by unlinking rather than by renaming — which is the window where every
// operation on the Server would find nothing at all at ~/.brama/shim.
func TestShimInstallLeavesNoPartialFile(t *testing.T) {
	server := dial(t, staging)
	clearShimHome(t, server)

	platform := platformOf(t, server)
	const version = "v0.4.0"
	ensure(t, server, platform, version)

	// `ls -a` rather than a glob: the staged name begins with a dot, and the shell
	// would hand the unmatched pattern back as a literal.
	entries := strings.Fields(run(t, server, "ls -a ~/.brama"))
	for _, name := range entries {
		if strings.HasSuffix(name, ".part") {
			t.Errorf("~/.brama still holds the staged upload %q", name)
		}
		if name == ".shim.new" {
			t.Error("~/.brama still holds the temporary symlink .shim.new")
		}
	}

	// The symlink must point at the versioned file by name, not be a copy of it:
	// that indirection is what makes the swap a rename.
	target := run(t, server, "readlink ~/.brama/shim")
	if want := "shim-" + version; target != want {
		t.Errorf("~/.brama/shim -> %q, want %q", target, want)
	}
	assertRunning(t, server, version)
}

// ensure runs the real install against the Server with a Shim stamped at version.
func ensure(t *testing.T, s *ssh.Session, p shim.Platform, version string) shim.Step {
	t.Helper()

	step, err := shim.Ensure(t.Context(), s, version, sourceOf(buildShim(t, p, version)))
	if err != nil {
		t.Fatalf("installing %s: %v", version, err)
	}
	return step
}

// assertRunning asks the installed binary what it is, over SSH, by executing it.
//
// Deliberately not a check of the symlink's name or the file's mtime: what the
// acceptance criterion asks is that `--version` on the installed binary matches the
// calling brama, and only running it can answer that. It is also the one assertion
// here that a fake could never have made — the fake returns whatever version the
// test author typed.
func assertRunning(t *testing.T, s *ssh.Session, want string) {
	t.Helper()
	if got := run(t, s, "~/.brama/shim --version"); got != want {
		t.Errorf("the installed shim reports %q, want %q", got, want)
	}
}
