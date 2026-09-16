// Package version orders the version strings brama is built from.
//
// It is not a general-purpose SemVer package, and using one would get this wrong.
// brama's version is whatever `git describe --tags --always --dirty` produced at link
// time, which is a SemVer tag with two possible suffixes bolted on: the distance from
// that tag, and whether the tree was dirty. SemVer reads `v0.1.0-3-gabc1234` as a
// prerelease of v0.1.0 and therefore ranks it *below* the tag, when it is three
// commits above it.
//
// The one caller is the Shim's version check: brama compares what it shipped against
// what answers on a Server, and upgrades when its own build is newer.
package version

import (
	"strconv"
	"strings"
)

// Ordering is how one version stands to another.
type Ordering int

const (
	// Unknown means at least one of the two is not a version this package can rank —
	// a plain `go build` stamps "dev", and a tree with no tags stamps a bare commit.
	// It is a distinct answer rather than an arbitrary one, because the caller's move
	// differs: there is no "newer" to act on, only a difference to report.
	Unknown Ordering = iota
	// Older means the first version precedes the second.
	Older
	// Same means the two name the same build.
	Same
	// Newer means the first version follows the second.
	Newer
)

func (o Ordering) String() string {
	switch o {
	case Older:
		return "older"
	case Same:
		return "same"
	case Newer:
		return "newer"
	default:
		return "unknown"
	}
}

// Compare reports how a stands to b.
//
// Identical strings are Same whether or not they can be parsed: two binaries stamped
// "dev" are as alike as anything can tell, and reporting a difference there would mean
// reinstalling a Shim on every run.
//
// A dirty build is the exception. `-dirty` is positive evidence of changes the version
// string does not name, so two of them are never provably the same build even when the
// strings match — and the person that costs an extra upload is the one changing the
// Shim, who is exactly the person a silent "already current" would mislead.
func Compare(a, b string) Ordering {
	a, b = normalize(a), normalize(b)

	unnamedChanges := isDirty(a) || isDirty(b)
	if a == b && !unnamedChanges {
		return Same
	}

	left, ok := parse(a)
	if !ok {
		return Unknown
	}
	right, ok := parse(b)
	if !ok {
		return Unknown
	}

	sign := left.compare(right)
	if sign == 0 && unnamedChanges {
		return Unknown
	}
	return ordering(sign)
}

func isDirty(v string) bool { return strings.HasSuffix(v, "-dirty") }

func ordering(sign int) Ordering {
	switch {
	case sign < 0:
		return Older
	case sign > 0:
		return Newer
	default:
		return Same
	}
}

// build is one parsed version, in the order its parts are compared.
type build struct {
	major, minor, patch int
	// pre is the SemVer prerelease, empty when the version is a release. A release
	// outranks every prerelease of the same numbers.
	pre string
	// ahead is how many commits `git describe` counted past the tag.
	ahead int
	// dirty is whether the tree had uncommitted changes.
	dirty bool
}

func (b build) compare(other build) int {
	for _, pair := range [][2]int{
		{b.major, other.major},
		{b.minor, other.minor},
		{b.patch, other.patch},
	} {
		if sign := cmp(pair[0], pair[1]); sign != 0 {
			return sign
		}
	}
	if sign := comparePrerelease(b.pre, other.pre); sign != 0 {
		return sign
	}
	if sign := cmp(b.ahead, other.ahead); sign != 0 {
		return sign
	}
	return cmp(boolRank(b.dirty), boolRank(other.dirty))
}

func normalize(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// parse reads a normalized version. It reports failure rather than guessing: a
// version this package cannot take apart is one it has no business ranking.
func parse(v string) (build, bool) {
	// Build metadata is ignored, per SemVer 2.0.0 §10.
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}

	var out build
	if isDirty(v) {
		out.dirty = true
		v = strings.TrimSuffix(v, "-dirty")
	}

	v, out.ahead = cutDescribeSuffix(v)

	numbers, pre, _ := strings.Cut(v, "-")
	out.pre = pre

	fields := strings.Split(numbers, ".")
	if len(fields) != 3 {
		return build{}, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return build{}, false
		}
		switch i {
		case 0:
			out.major = n
		case 1:
			out.minor = n
		case 2:
			out.patch = n
		}
	}
	return out, true
}

// cutDescribeSuffix strips the `-<distance>-g<commit>` that `git describe` appends
// when the build is not exactly on a tag, returning the distance it named.
//
// It is recognised by shape rather than by position, because a SemVer prerelease sits
// between the tag and the suffix: `0.2.0-rc.1-3-gabc1234` must keep `rc.1`.
func cutDescribeSuffix(v string) (string, int) {
	parts := strings.Split(v, "-")
	if len(parts) < 3 {
		return v, 0
	}

	commit, distance := parts[len(parts)-1], parts[len(parts)-2]
	if !isCommit(commit) {
		return v, 0
	}
	ahead, err := strconv.Atoi(distance)
	if err != nil || ahead < 0 {
		return v, 0
	}
	return strings.Join(parts[:len(parts)-2], "-"), ahead
}

// isCommit reports whether s is the abbreviated hash `git describe` writes: a `g`
// followed by hex digits.
func isCommit(s string) bool {
	if len(s) < 2 || s[0] != 'g' {
		return false
	}
	for _, r := range s[1:] {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// comparePrerelease implements SemVer 2.0.0 §11.4: an absent prerelease outranks a
// present one, numeric identifiers rank below alphanumeric ones, and a longer run of
// otherwise-equal identifiers outranks a shorter one.
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}

	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if sign := compareIdentifier(left[i], right[i]); sign != 0 {
			return sign
		}
	}
	return cmp(len(left), len(right))
}

func compareIdentifier(a, b string) int {
	left, leftNumeric := strconv.Atoi(a)
	right, rightNumeric := strconv.Atoi(b)

	switch {
	case leftNumeric == nil && rightNumeric == nil:
		return cmp(left, right)
	case leftNumeric == nil:
		return -1
	case rightNumeric == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}
