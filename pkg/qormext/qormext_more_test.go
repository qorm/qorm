package qormext

import (
	"strconv"
	"testing"
)

// TestCompatibleABIEdgeCases extends TestCompatibleABI with parsing edge
// cases: surrounding whitespace, dotted/empty/negative/overflow majors, and
// the inner TrimSpace of the major component.
func TestCompatibleABIEdgeCases(t *testing.T) {
	cur := strconv.Itoa(ABIVersion)
	cases := []struct {
		declared string
		want     bool
	}{
		{"\t" + cur + "\n", true},       // whitespace around a bare major
		{" " + cur + ".2.3 ", true},     // major.minor.patch, padded
		{cur + ".", true},               // trailing dot: major only
		{cur + ".x.y", true},            // everything past the first dot is ignored
		{cur + " .2", true},             // space before the dot: inner TrimSpace
		{"1 .2", ABIVersion == 1},       // major "1 " trims to "1"
		{"+1", ABIVersion == 1},         // strconv.Atoi accepts a leading +
		{"-1", ABIVersion == -1},        // negative major parses, never matches
		{".5", false},                   // leading dot: empty major is unparseable
		{".", false},                    // dot alone: empty major
		{"99999999999999999999", false}, // int overflow: unparseable
		{"abc.def", false},              // non-numeric major
	}
	for _, c := range cases {
		if got := CompatibleABI(c.declared); got != c.want {
			t.Errorf("CompatibleABI(%q) = %v, want %v (ABIVersion=%d)", c.declared, got, c.want, ABIVersion)
		}
	}
}

// TestRegister verifies Register adds ops to the shared Ops registry, that a
// re-registration replaces the previous handler, and that distinct ops
// coexist without disturbing one another.
func TestRegister(t *testing.T) {
	const (
		opA = "qormextTestOpA"
		opB = "qormextTestOpB"
	)
	t.Cleanup(func() {
		delete(Ops, opA)
		delete(Ops, opB)
	})

	if _, ok := Ops[opA]; ok {
		t.Fatalf("precondition failed: Ops already contains %q", opA)
	}

	Register(opA, func(data map[string]any) string {
		return "first:" + data["v"].(string)
	})
	fn, ok := Ops[opA]
	if !ok {
		t.Fatalf("Register(%q, ...) did not add the op to Ops", opA)
	}
	if got := fn(map[string]any{"v": "x"}); got != "first:x" {
		t.Errorf("registered op returned %q, want %q", got, "first:x")
	}

	// Re-registering the same name replaces the handler.
	Register(opA, func(data map[string]any) string { return "second" })
	if got := Ops[opA](nil); got != "second" {
		t.Errorf("after re-register, op %q returned %q, want %q", opA, got, "second")
	}

	// A second op coexists and leaves the first untouched.
	Register(opB, func(data map[string]any) string { return "b" })
	if got := Ops[opA](nil); got != "second" {
		t.Errorf("op %q changed after registering %q: got %q, want %q", opA, opB, got, "second")
	}
	if got := Ops[opB](nil); got != "b" {
		t.Errorf("op %q returned %q, want %q", opB, got, "b")
	}
}
