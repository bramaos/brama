package shim_test

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bramaos/brama/internal/shim"
)

// elfMachine is the machine a build for each supported Platform must report.
//
// A table rather than a lookup so that adding a Platform to the matrix without
// deciding what its binaries should look like fails here, loudly, instead of
// shipping unverified.
var elfMachine = map[shim.Platform]elf.Machine{
	{OS: "linux", Arch: "amd64"}: elf.EM_X86_64,
	{OS: "linux", Arch: "arm64"}: elf.EM_AARCH64,
}

// embeddedBuild returns the build for a Platform, or stops the test.
//
// A checkout has no builds in it, and a contributor who never touches SSH should not
// have to cross-build to run the suite — so locally, nothing embedded is a skip. In
// CI it is a failure: these tests are the only thing between a broken cross-build and
// a Server that cannot run what it was sent, and they only run at all because `make
// shim` precedes them. If that step is dropped or reordered, this says so rather than
// reporting green for a matrix nobody built. GitHub Actions always sets CI.
func embeddedBuild(t *testing.T, p shim.Platform) []byte {
	t.Helper()

	data, err := shim.Binary(p)
	if errors.Is(err, shim.ErrNotBuilt) {
		if os.Getenv("CI") != "" {
			t.Fatalf("no %s build embedded — `make shim` has to run before the tests", p)
		}
		t.Skipf("no %s build embedded — run `make shim`", p)
	}
	if err != nil {
		t.Fatalf("Binary(%s): %v", p, err)
	}
	return data
}

func embeddedELF(t *testing.T, p shim.Platform) *elf.File {
	t.Helper()

	// Parsing as ELF is also what rules out an OS mismatch: a darwin or windows build
	// is not an ELF file at all, so it never gets past here.
	f, err := elf.NewFile(bytes.NewReader(embeddedBuild(t, p)))
	if err != nil {
		t.Fatalf("the embedded %s build is not an ELF executable: %v", p, err)
	}
	return f
}

// The name of an embedded build is the only thing that decides which Server gets it,
// and nothing about the bytes has to agree with it. A cross-build that lost its
// GOARCH — a dropped environment variable, a copy into the wrong filename — embeds a
// plausible binary that only fails once it is on the far end, where the error is
// "exec format error" from a machine the user cannot easily inspect. So the header is
// checked here, on this side of the wire.
func TestEmbeddedBuildsAreForThePlatformTheyAreNamedFor(t *testing.T) {
	for _, p := range shim.SupportedPlatforms() {
		t.Run(p.String(), func(t *testing.T) {
			want, known := elfMachine[p]
			if !known {
				t.Fatalf("%s is in the matrix but this test does not know what its binary should be — add it to elfMachine", p)
			}

			f := embeddedELF(t, p)
			if f.Machine != want {
				t.Errorf("the embedded %s build is for %s, want %s", p, f.Machine, want)
			}
			if f.Class != elf.ELFCLASS64 {
				t.Errorf("the embedded %s build is %s, want %s", p, f.Class, elf.ELFCLASS64)
			}
		})
	}
}

// Guarantee 7 says the Shim adds no runtime dependency on the Server. A build that
// wants an interpreter has one: it needs a loader and a libc of the right vintage,
// and the Server that fails is the old one, in production, at the moment of the
// operation. CGO_ENABLED=0 is what keeps that from happening, and this is what
// notices when it stops being passed.
func TestEmbeddedBuildsAreStaticallyLinked(t *testing.T) {
	for _, p := range shim.SupportedPlatforms() {
		t.Run(p.String(), func(t *testing.T) {
			f := embeddedELF(t, p)

			for _, prog := range f.Progs {
				if prog.Type == elf.PT_INTERP {
					t.Errorf("the embedded %s build asks for a dynamic loader — it was built without CGO_ENABLED=0", p)
				}
			}

			// Reported, never swallowed: a guard that passes because it failed to
			// look is worse than no guard.
			libs, err := f.ImportedLibraries()
			if err != nil {
				t.Fatalf("reading the libraries the embedded %s build needs: %v", p, err)
			}
			if len(libs) > 0 {
				t.Errorf("the embedded %s build needs %v on the server", p, libs)
			}
		})
	}
}

// Header checks prove the bytes describe the right machine; only running one proves
// they are a program. A truncated embed, a build that lost its version stamp, or a
// `--version` contract that quietly changed all produce a file with a perfectly good
// ELF header, and every one of them first fails on the Server — where Install reads
// it as "the shim was installed but does not run".
//
// Only this machine's build can be executed here, which is the one architecture a
// cross-build is least likely to get wrong. It is still worth it: the failures above
// are not architecture-specific, and CI running amd64 catches all of them for both.
func TestEmbeddedBuildForThisMachineRuns(t *testing.T) {
	here := shim.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if _, err := shim.Binary(here); errors.Is(err, shim.ErrUnsupportedPlatform) {
		t.Skipf("this machine is %s, which brama ships no shim for", here)
	}

	path := filepath.Join(t.TempDir(), "shim")
	if err := os.WriteFile(path, embeddedBuild(t, here), 0o755); err != nil {
		t.Fatalf("writing the shim out: %v", err)
	}

	// The same bound the operation has: a Shim that hangs on --version is as broken
	// as one that will not start, and a test that hangs with it says less.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		t.Fatalf("the embedded %s build does not run: %v", here, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Errorf("the embedded %s build answered --version with nothing", here)
	}
}

var shimPlatforms = regexp.MustCompile(`(?m)^SHIM_PLATFORMS\s*:?=\s*(.*)$`)

// The matrix is stated twice: SupportedPlatforms is what brama promises a user, and
// the Makefile is what actually gets built. They drift silently and in the worse
// direction — a Platform added to the promise but not to the build is a Server that
// passes the "is this supported?" check and then fails with ErrNotBuilt, which reads
// like brama's own build is broken rather than like a missing line in a Makefile.
//
// Compared as sets: the order SHIM_PLATFORMS happens to be written in is nobody's
// business but the Makefile's.
func TestMakefileBuildsEverySupportedPlatform(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}

	match := shimPlatforms.FindSubmatch(body)
	if match == nil {
		t.Fatal("no SHIM_PLATFORMS in the Makefile — the shim build matrix has moved, and this test has not")
	}

	built := strings.Fields(string(match[1]))
	promised := make([]string, 0, len(shim.SupportedPlatforms()))
	for _, p := range shim.SupportedPlatforms() {
		promised = append(promised, p.String())
	}
	sort.Strings(built)
	sort.Strings(promised)

	if strings.Join(built, " ") != strings.Join(promised, " ") {
		t.Errorf("the Makefile builds %v, but SupportedPlatforms promises %v", built, promised)
	}
}
