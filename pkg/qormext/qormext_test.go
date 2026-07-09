package qormext

import (
	"strconv"
	"testing"
)

func TestCompatibleABI(t *testing.T) {
	cur := strconv.Itoa(ABIVersion)
	cases := []struct {
		declared string
		want     bool
	}{
		{"", true},                      // unset — no versioned middle-layer
		{"  ", true},                    // blank
		{cur, true},                     // exact current major
		{cur + ".3", true},              // same major, any minor
		{strconv.Itoa(ABIVersion + 1), false}, // next major — incompatible
		{"999", false},                  // far future major
		{"0", ABIVersion == 0},          // major 0 only ok if runtime is 0
		{"abc", false},                  // unparseable
		{"1.x", true},                   // major "1" parses; ".x" ignored (when ABIVersion==1)
	}
	for _, c := range cases {
		// "1.x" only compatible when ABIVersion==1
		want := c.want
		if c.declared == "1.x" {
			want = ABIVersion == 1
		}
		if got := CompatibleABI(c.declared); got != want {
			t.Errorf("CompatibleABI(%q) = %v, want %v (ABIVersion=%d)", c.declared, got, want, ABIVersion)
		}
	}
}
