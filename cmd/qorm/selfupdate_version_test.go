package main

import (
	"testing"
)

// TestCompareSemver pins the ordering rules the self-update version gate
// relies on: numeric (never lexicographic) core comparison, an optional
// leading "v" on either side, prereleases sorting OLDER than their release,
// semver prerelease-identifier precedence, and build metadata ignored.
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// core ordering is numeric, not string comparison
		{"1.2.3", "1.2.4", -1},
		{"1.2.4", "1.2.3", 1},
		{"1.2.10", "1.2.9", 1}, // "1.2.10" < "1.2.9" as strings; numerically newer
		{"1.2.9", "1.2.10", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"10.0.0", "9.0.0", 1},
		{"0.3.3", "0.3.3", 0},

		// optional leading "v" on either or both sides
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v2.0.0", "v1.9.9", 1},
		{" v1.2.3 ", "1.2.3", 0}, // surrounding whitespace tolerated

		// a prerelease of the same X.Y.Z sorts OLDER than the release
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"v9.9.9-rc1", "v9.9.9", -1},
		{"1.0.0-rc1", "1.0.0-rc1", 0},

		// prerelease precedence per semver.org section 11: numeric identifiers
		// compare as numbers and sort before alphanumeric ones; alphanumeric
		// compare ASCII-lexicographically; a shorter identifier list sorts
		// first when all shared identifiers are equal
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-beta.2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1}, // numeric: 2 < 11
		{"1.0.0-beta.11", "1.0.0-rc.1", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-2", "1.0.0-10", -1},
		{"1.0.0-10", "1.0.0-2", 1},
		{"1.0.0-RC1", "1.0.0-rc1", -1}, // ASCII: uppercase sorts first

		// build metadata carries no precedence
		{"1.0.0+build.5", "1.0.0", 0},
		{"1.0.0+aaa", "1.0.0+zzz", 0},
		{"1.0.0-rc1+build", "1.0.0-rc1", 0},
		{"1.0.0-rc1+build", "1.0.0", -1},
	}
	for _, c := range cases {
		got, err := compareSemver(c.a, c.b)
		if err != nil {
			t.Errorf("compareSemver(%q, %q): unexpected error: %v", c.a, c.b, err)
			continue
		}
		if got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCompareSemverMalformed pins fail-closed parsing: anything that is not a
// clean [v]X.Y.Z[-pre][+build] tag is an error, which the update gate treats
// as "not newer" and refuses to install. A hostile or sloppy release endpoint
// must not be able to smuggle past the gate with an odd tag.
func TestCompareSemverMalformed(t *testing.T) {
	bad := []string{
		"",
		"banana",
		"v",
		"V1.2.3", // only a lowercase "v" prefix is recognized
		"+1.2.3",
		"1",
		"1.2",
		"1.2.3.4",
		"1.2.x",
		"a.b.c",
		"-1.2.3",  // negative component
		"1.-2.3",  // negative component
		"1.02.3",  // leading zeros in core components
		"1.2.3-",  // empty prerelease
		"1.2.3-.", // empty identifier
		"1.2.3-rc..1",
		"1.2.3-01",                       // numeric prerelease identifier with a leading zero
		"1.2.3_rc1",                      // underscore is not a legal identifier character
		"1.2.3 rc1",                      // internal space
		"v 1.2.3",                        // space after the v prefix
		"99999999999999999999999999.0.0", // component overflows uint64
	}
	// Contrast: a valid core plus build metadata IS well-formed (build
	// metadata is dropped) and compares equal to the bare core.
	got, err := compareSemver("1.2.3+only.build", "1.2.3")
	if err != nil || got != 0 {
		t.Errorf("compareSemver(\"1.2.3+only.build\", \"1.2.3\") = %d, %v; want 0, nil", got, err)
	}

	for _, s := range bad {
		if _, err := compareSemver(s, "1.0.0"); err == nil {
			t.Errorf("compareSemver(%q, \"1.0.0\"): want an error for a malformed version", s)
		}
		if _, err := compareSemver("1.0.0", s); err == nil {
			t.Errorf("compareSemver(\"1.0.0\", %q): want an error for a malformed version", s)
		}
	}
}
