package shim_test

import (
	"errors"
	"testing"

	"github.com/bramaos/brama/internal/shim"
)

// `uname -sm` is what a Server answers with, and it does not speak Go's vocabulary:
// x86_64 is amd64, aarch64 is arm64. Getting this wrong ships a binary that cannot
// execute, so the mapping is pinned rather than inferred.
func TestParsePlatform(t *testing.T) {
	cases := []struct {
		uname string
		want  string
	}{
		{"Linux x86_64", "linux/amd64"},
		{"Linux aarch64", "linux/arm64"},
		{"Linux arm64", "linux/arm64"},
		{"Linux amd64", "linux/amd64"},
		{"  Linux   x86_64  \n", "linux/amd64"},
		{"Darwin arm64", "darwin/arm64"},
		{"FreeBSD amd64", "freebsd/amd64"},
		{"Linux i686", "linux/386"},
	}

	for _, c := range cases {
		t.Run(c.uname, func(t *testing.T) {
			got, err := shim.ParsePlatform(c.uname)
			if err != nil {
				t.Fatalf("ParsePlatform(%q): %v", c.uname, err)
			}
			if got.String() != c.want {
				t.Errorf("ParsePlatform(%q) = %s, want %s", c.uname, got, c.want)
			}
		})
	}
}

func TestParsePlatformRejectsUnreadableOutput(t *testing.T) {
	for _, uname := range []string{"", "Linux", "   "} {
		if got, err := shim.ParsePlatform(uname); err == nil {
			t.Errorf("ParsePlatform(%q) = %s, want an error", uname, got)
		}
	}
}

// A Server brama has no build for is a limitation, not a guardrail: it exits 1, and
// the message has to name what was detected so the user can say what they need.
func TestBinaryRejectsAPlatformOffTheMatrix(t *testing.T) {
	for _, uname := range []string{"FreeBSD amd64", "Darwin arm64", "Linux i686"} {
		p, err := shim.ParsePlatform(uname)
		if err != nil {
			t.Fatalf("ParsePlatform(%q): %v", uname, err)
		}
		if _, err := shim.Binary(p); !errors.Is(err, shim.ErrUnsupportedPlatform) {
			t.Errorf("Binary(%s) err = %v, want ErrUnsupportedPlatform", p, err)
		}
	}
}

// Supported platforms are in the matrix whether or not this build embedded them, so
// "we don't build for that" and "this binary was built without Shims" stay distinct
// errors — the first is permanent, the second is fixed by `make binary`.
func TestBinaryOnSupportedPlatformIsNeverUnsupported(t *testing.T) {
	for _, p := range shim.SupportedPlatforms() {
		_, err := shim.Binary(p)
		if errors.Is(err, shim.ErrUnsupportedPlatform) {
			t.Errorf("Binary(%s) reported unsupported, but it is in the matrix", p)
		}
		if err != nil && !errors.Is(err, shim.ErrNotBuilt) {
			t.Errorf("Binary(%s) err = %v, want nil or ErrNotBuilt", p, err)
		}
	}
}
