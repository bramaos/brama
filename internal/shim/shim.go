// Package shim carries the Shim binaries brama installs on a Server.
//
// The builds are embedded rather than downloaded. A Server's outbound network is the
// last thing brama has any business widening, and an embedded binary cannot
// disagree with the CLI that sent it — there is no version to negotiate, no release
// to fetch, and no second channel into production to trust.
//
// See docs/adr/0006-server-global-shim-home.md.
package shim

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// binaries holds the cross-built Shims. A checkout contains only PLACEHOLDER, so
// that `go build ./...` and `go test ./...` work for a contributor who never touches
// SSH; `make binary` fills the directory in. A build without them still compiles and
// fails at the point of use, which is the only place the difference matters.
//
//go:embed bin
var binaries embed.FS

// Errors a caller distinguishes. Unsupported is permanent — brama has no build for
// that Server. NotBuilt is this binary's problem, and `make binary` fixes it.
var (
	ErrUnsupportedPlatform = errors.New("no shim build for this platform")
	ErrNotBuilt            = errors.New("this brama was built without shim binaries")
)

// Platform is a Server's operating system and architecture, in Go's vocabulary.
type Platform struct {
	OS   string
	Arch string
}

func (p Platform) String() string { return p.OS + "/" + p.Arch }

// filename is the embedded name for a Platform's build.
func (p Platform) filename() string { return "bin/shim-" + p.OS + "-" + p.Arch }

// The Go architecture names in the build matrix. Named because they appear both here
// and as the targets of the uname translation below, and the two must not drift.
const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
)

// SupportedPlatforms is the matrix brama builds for.
//
// Linux only, and both architectures: arm64 is ordinary now at Hetzner, Graviton and
// Ampere, so shipping amd64 alone would strand real users. macOS is absent because a
// Mac serving production is rare enough not to be worth the weight in every binary.
func SupportedPlatforms() []Platform {
	return []Platform{
		{OS: "linux", Arch: archAMD64},
		{OS: "linux", Arch: archARM64},
	}
}

func supported(p Platform) bool {
	for _, candidate := range SupportedPlatforms() {
		if candidate == p {
			return true
		}
	}
	return false
}

// Available reports whether this build embedded any Shim at all.
//
// It exists so a brama built with plain `go build` can say so before it opens a
// connection. Which Platform is needed is not knowable until the Server answers, but
// "this binary has nothing to install" is knowable immediately, and making the user
// wait through a handshake to hear it would be gratuitous.
func Available() bool {
	for _, p := range SupportedPlatforms() {
		if _, err := fs.Stat(binaries, p.filename()); err == nil {
			return true
		}
	}
	return false
}

// Binary returns the Shim build for a Platform.
func Binary(p Platform) ([]byte, error) {
	if !supported(p) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, p)
	}
	data, err := binaries.ReadFile(p.filename())
	if err != nil {
		return nil, fmt.Errorf("%w: %s is in the matrix but was not embedded", ErrNotBuilt, p)
	}
	return data, nil
}

// unameArch maps what `uname -m` reports to what Go calls the same architecture.
// Anything absent is passed through: an unknown value that happens to match a Go
// name still resolves, and one that does not is reported as unsupported by name,
// which is more useful than "unrecognised".
var unameArch = map[string]string{
	"x86_64":  archAMD64,
	archAMD64: archAMD64,
	"aarch64": archARM64,
	archARM64: archARM64,
	"i686":    "386",
	"i386":    "386",
	"x86":     "386",
}

// ParsePlatform reads the output of `uname -sm`.
//
// It does not judge whether brama supports the result — Binary does that. Keeping
// the two apart is what lets an unsupported Server be reported as the platform it
// actually is, rather than as unreadable output.
func ParsePlatform(uname string) (Platform, error) {
	fields := strings.Fields(uname)
	if len(fields) != 2 {
		return Platform{}, fmt.Errorf("cannot read `uname -sm` output %q, want two fields like \"Linux x86_64\"", strings.TrimSpace(uname))
	}

	arch := fields[1]
	if mapped, ok := unameArch[arch]; ok {
		arch = mapped
	}
	return Platform{OS: strings.ToLower(fields[0]), Arch: arch}, nil
}
