package version

import "testing"

func TestCompareOrdersReleases(t *testing.T) {
	cases := []struct {
		a, b string
		want Ordering
	}{
		{"v0.1.0", "v0.1.0", Same},
		{"0.1.0", "v0.1.0", Same},
		{"v0.1.1", "v0.1.0", Newer},
		{"v0.1.0", "v0.1.1", Older},
		{"v0.2.0", "v0.1.9", Newer},
		{"v1.0.0", "v0.9.9", Newer},
		{"v0.9.9", "v1.0.0", Older},
		{"v0.1.10", "v0.1.9", Newer},
	}

	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			if got := Compare(c.a, c.b); got != c.want {
				t.Errorf("Compare(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// Build metadata is not part of the ordering, per SemVer 2.0.0.
func TestCompareIgnoresBuildMetadata(t *testing.T) {
	if got := Compare("v0.1.0+deadbeef", "v0.1.0"); got != Same {
		t.Errorf("Compare() = %v, want Same — build metadata does not order", got)
	}
}

// SemVer: a release outranks any prerelease of the same numbers.
func TestComparePrereleases(t *testing.T) {
	cases := []struct {
		a, b string
		want Ordering
	}{
		{"v0.2.0", "v0.2.0-rc.1", Newer},
		{"v0.2.0-rc.1", "v0.2.0", Older},
		{"v0.2.0-rc.2", "v0.2.0-rc.1", Newer},
		{"v0.2.0-rc.10", "v0.2.0-rc.2", Newer},
		{"v0.2.0-alpha", "v0.2.0-beta", Older},
		{"v0.2.0-rc.1.1", "v0.2.0-rc.1", Newer},
		{"v0.2.0-alpha.beta", "v0.2.0-alpha.1", Newer},
	}

	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			if got := Compare(c.a, c.b); got != c.want {
				t.Errorf("Compare(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// `git describe` appends the distance from the tag and the commit it found. A build
// three commits past v0.1.0 is newer than v0.1.0 — which is the opposite of what
// reading `-3-gabc1234` as a SemVer prerelease would say, and the reason this package
// exists rather than a general-purpose SemVer one.
func TestCompareReadsGitDescribeDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want Ordering
	}{
		{"v0.1.0-3-gabc1234", "v0.1.0", Newer},
		{"v0.1.0", "v0.1.0-3-gabc1234", Older},
		{"v0.1.0-7-gabc1234", "v0.1.0-3-gdef5678", Newer},
		{"v0.1.1", "v0.1.0-99-gabc1234", Newer},
		{"v0.2.0-rc.1-3-gabc1234", "v0.2.0-rc.1", Newer},
	}

	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			if got := Compare(c.a, c.b); got != c.want {
				t.Errorf("Compare(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// A dirty tree is ahead of the commit it was built from: it carries changes that
// commit does not. Ordering it below would mean a developer's own build never
// replaced the one it just superseded on their server.
func TestCompareRanksADirtyTreeAhead(t *testing.T) {
	cases := []struct {
		a, b string
		want Ordering
	}{
		{"v0.1.0-dirty", "v0.1.0", Newer},
		{"v0.1.0", "v0.1.0-dirty", Older},
		{"v0.1.0-3-gabc1234-dirty", "v0.1.0-3-gabc1234", Newer},
		{"v0.1.1", "v0.1.0-dirty", Newer},
	}

	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			if got := Compare(c.a, c.b); got != c.want {
				t.Errorf("Compare(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// `-dirty` says there are changes the version string does not name, so two identical
// dirty strings are not evidence of the same build — they are evidence that neither
// build can be identified. The person this costs an extra upload is whoever is
// changing the Shim, who is the last person a silent "already current" should reach.
func TestCompareWillNotCallTwoDirtyBuildsTheSame(t *testing.T) {
	for _, v := range []string{"v0.1.0-dirty", "v0.1.0-3-gabc1234-dirty", "abc1234-dirty"} {
		t.Run(v, func(t *testing.T) {
			if got := Compare(v, v); got != Unknown {
				t.Errorf("Compare(%q, %q) = %v, want Unknown", v, v, got)
			}
		})
	}
}

// A binary built outside a tagged tree has no place in an ordering. Saying so is the
// point: the caller has to decide what to do about two versions it cannot rank,
// rather than being handed a confident answer that was a coin toss.
func TestCompareReportsWhatItCannotOrder(t *testing.T) {
	cases := []struct{ a, b string }{
		{"dev", "v0.1.0"},
		{"v0.1.0", "dev"},
		{"abc1234", "v0.1.0"},
		{"abc1234-dirty", "v0.1.0"},
		{"", "v0.1.0"},
		{"v0.1", "v0.1.0"},
		{"v0.1.0.1", "v0.1.0"},
		{"not a version", "v0.1.0"},
	}

	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			if got := Compare(c.a, c.b); got != Unknown {
				t.Errorf("Compare(%q, %q) = %v, want Unknown", c.a, c.b, got)
			}
		})
	}
}

// Two builds stamped with the same unorderable string are still the same build, and
// must not be reported as a difference worth acting on.
func TestCompareMatchesIdenticalUnorderableVersions(t *testing.T) {
	for _, v := range []string{"dev", "abc1234"} {
		t.Run(v, func(t *testing.T) {
			if got := Compare(v, v); got != Same {
				t.Errorf("Compare(%q, %q) = %v, want Same", v, v, got)
			}
		})
	}
}

// Whitespace comes from reading a version off a Server's stdout.
func TestCompareTrimsWhitespace(t *testing.T) {
	if got := Compare("  v0.1.0\n", "v0.1.0"); got != Same {
		t.Errorf("Compare() = %v, want Same", got)
	}
}

func TestOrderingString(t *testing.T) {
	for ordering, want := range map[Ordering]string{
		Unknown: "unknown",
		Older:   "older",
		Same:    "same",
		Newer:   "newer",
	} {
		if got := ordering.String(); got != want {
			t.Errorf("Ordering.String() = %q, want %q", got, want)
		}
	}
}
