package expr

// Static checks for the new syntax and builtins: index-key inference, the
// callSigs arity/type table, and literal sub-expression validation. The
// checker's contract is unchanged: zero false positives, so everything not
// provably wrong passes.

import (
	"strings"
	"testing"
)

var callVars = map[string]string{
	"state.items": "array",
	"state.user":  "object",
	"state.name":  "string",
	"state.count": "number",
	"state.flag":  "bool",
}

// TestCheckIndexInference covers index-node inference: a declared-array base
// flags keys whose declared type toNum would silently zero, everything else
// passes, and subexpressions inside base and key are still checked.
func TestCheckIndexInference(t *testing.T) {
	legit := []string{
		"state.items[0]",
		"state.items[state.count]",
		"state.items[state.flag]",      // bool coerces numerically by design
		"state.items[state.count - 1]", // numeric expression key
		"state.items[state.other]",     // unknown key type
		"state.user[state.name]",       // object: any key stringifies
		"state.user['k']",
		"unknown[state.name]",      // unknown base: never reported
		"state.name[0]",            // non-array base: index not modeled
		"state.items[0].price * 2", // element type unknown -> * legal
		"len(state.items[0])",
		"state.items[0][1]", // chained: outer base is unknown
	}
	for _, src := range legit {
		if mm := Check(src, callVars); len(mm) != 0 {
			t.Errorf("Check(%q): false positive: %v", src, mm)
		}
	}

	flagged := []struct {
		src  string
		want string
	}{
		{"state.items[state.name]", "state.name is string, used as list index"},
		{"state.items['k']", `"k" is string, used as list index`},
		{"state.items[state.items]", "state.items is array, used as list index"},
		{"state.items[state.user]", "state.user is object, used as list index"},
	}
	for _, c := range flagged {
		mm := Check(c.src, callVars)
		if len(mm) != 1 || !strings.Contains(mm[0].Detail, c.want) {
			t.Errorf("Check(%q) = %v, want one mismatch containing %q", c.src, mm, c.want)
		}
	}

	// The key subexpression is still checked on its own terms: the mismatch
	// comes from the arithmetic, not a spurious index report.
	mm := Check("state.items[state.name - 1]", callVars)
	if len(mm) != 1 || !strings.Contains(mm[0].Detail, "state.name is string, used as number") {
		t.Errorf("key subexpr check = %v, want the arithmetic mismatch only", mm)
	}
}

// TestCheckCallArity covers each arity-mismatch wording: exact singular,
// exact plural, a min-max range, and a variadic minimum.
func TestCheckCallArity(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"sum(state.items, 1)", "sum() takes 1 argument, got 2"},
		{"sum()", "sum() takes 1 argument, got 0"},
		{"at(state.items)", "at() takes 2 arguments, got 1"},
		{"at(state.items, 1, 2)", "at() takes 2 arguments, got 3"},
		{"count(state.items, 'it', 3)", "count() takes 1 to 2 arguments, got 3"},
		{"format()", "format() takes at least 1 argument, got 0"},
		{"keys(state.user, 1)", "keys() takes 1 argument, got 2"},
		{"filter(state.items)", "filter() takes 2 arguments, got 1"},
	}
	for _, c := range cases {
		mm := Check(c.src, callVars)
		if len(mm) != 1 || !strings.Contains(mm[0].Detail, c.want) {
			t.Errorf("Check(%q) = %v, want one mismatch containing %q", c.src, mm, c.want)
		}
	}
}

// TestCheckCallArgTypes covers the per-parameter type checks for each wanted
// type, on both the flagged and the passing side.
func TestCheckCallArgTypes(t *testing.T) {
	flagged := []struct {
		src  string
		want string
	}{
		{"sum(state.name)", "state.name is string, sum() argument 1 wants array"},
		{"avg(state.user)", "state.user is object, avg() argument 1 wants array"},
		{"first(state.count)", "state.count is number, first() argument 1 wants array"},
		{"at(state.items, state.name)", "state.name is string, at() argument 2 wants number"},
		{"at(state.name, 0)", "state.name is string, at() argument 1 wants array"},
		{"join(state.name, ',')", "state.name is string, join() argument 1 wants array"},
		{"join(state.items, state.items)", "state.items is array, join() argument 2 wants string"},
		{"split(state.items, ',')", "state.items is array, split() argument 1 wants string"},
		{"split(state.name, state.user)", "state.user is object, split() argument 2 wants string"},
		{"keys(state.items)", "state.items is array, keys() argument 1 wants object"},
		{"values(state.name)", "state.name is string, values() argument 1 wants object"},
		{"map(state.items, state.count)", "state.count is number, map() argument 2 wants a string sub-expression"},
		{"filter(state.items, state.flag)", "state.flag is bool, filter() argument 2 wants a string sub-expression"},
		{"count(state.items, state.count)", "state.count is number, count() argument 2 wants a string sub-expression"},
		{"format(state.items)", "state.items is array, format() argument 1 wants string"},
	}
	for _, c := range flagged {
		mm := Check(c.src, callVars)
		if len(mm) != 1 || !strings.Contains(mm[0].Detail, c.want) {
			t.Errorf("Check(%q) = %v, want one mismatch containing %q", c.src, mm, c.want)
		}
	}

	legit := []string{
		"at(state.items, 2)",
		"at(state.items, state.flag)", // bool is numeric-compatible
		"first(state.items)",
		"last(state.items)",
		"sum(state.items)",
		"avg(state.items)",
		"count(state.items)",
		"count(state.items, 'it.done')",
		"join(state.items, ', ')",
		"join(state.items, 7)", // number separator stringifies
		"split(state.name, ',')",
		"split(state.count, 0)", // numbers stringify
		"keys(state.user)",
		"values(state.user)",
		"map(state.items, 'it.price * it.qty')",
		"filter(state.items, 'it.done')",
		"format('%s: %d', state.name, state.count)",
		"format(state.name)",
		"format(state.count)", // number pattern stringifies
		"sum(unknownList)",    // unknown types always pass
		"map(unknownList, unknownPred)",
		"someFunc(state.items)",             // unlisted builtins keep no-check behavior
		"sum(map(state.items, 'it.price'))", // call result is unknown
	}
	for _, src := range legit {
		if mm := Check(src, callVars); len(mm) != 0 {
			t.Errorf("Check(%q): false positive: %v", src, mm)
		}
	}
}

// TestCheckSubExprLiteral covers static validation of literal sub-expression
// arguments: parse errors are reported, nested provable mistakes are lifted
// with context, and valid predicates (with `it` unknown) pass.
func TestCheckSubExprLiteral(t *testing.T) {
	mm := Check(`filter(state.items, "it >")`, callVars)
	if len(mm) != 1 || !strings.Contains(mm[0].Detail, `filter() argument 2 sub-expression "it >" does not parse`) {
		t.Errorf("parse error not reported: %v", mm)
	}
	if len(mm) == 1 && mm[0].Expr != `filter(state.items, "it >")` {
		t.Errorf("Expr = %q, want the outer source", mm[0].Expr)
	}

	mm = Check(`map(state.items, "sum()")`, callVars)
	if len(mm) != 1 || !strings.Contains(mm[0].Detail, `in map() sub-expression "sum()": sum() takes 1 argument, got 0`) {
		t.Errorf("nested mismatch not lifted: %v", mm)
	}

	mm = Check(`count(state.items, "'a' * 2")`, callVars)
	if len(mm) != 1 || !strings.Contains(mm[0].Detail, `in count() sub-expression`) {
		t.Errorf("nested literal misuse not lifted: %v", mm)
	}

	legit := []string{
		`filter(state.items, "it.done")`,
		`map(state.items, "it.price * it.qty")`,
		`count(state.items, "it.qty > 1")`,
		`map(state.items, "filter(it.tags, 'len(it) > 1')")`, // nested literal, valid
	}
	for _, src := range legit {
		if got := Check(src, callVars); len(got) != 0 {
			t.Errorf("Check(%q): false positive: %v", src, got)
		}
	}

	// An arity mismatch stops positional checks (they would mislead), so a
	// wrong-arity call with a broken literal reports the arity alone.
	mm = Check(`map(state.items, "it >", 3)`, callVars)
	if len(mm) != 1 || !strings.Contains(mm[0].Detail, "map() takes 2 arguments, got 3") {
		t.Errorf("arity-first contract broken: %v", mm)
	}
}

// TestExprTextIndex pins the diagnostic rendering of index nodes.
func TestExprTextIndex(t *testing.T) {
	n := index{index{ident{"a"}, numLit{0}}, strLit{"k"}}
	if got := exprText(n); got != `a[0]["k"]` {
		t.Errorf("exprText(index chain) = %q, want %q", got, `a[0]["k"]`)
	}
}

// TestArgCompatibleAndPlural covers the helpers' remaining branches directly.
func TestArgCompatibleAndPlural(t *testing.T) {
	cases := []struct {
		want, got string
		ok        bool
	}{
		{typeNumber, typeUnknown, true},
		{typeNumber, typeNumber, true},
		{typeNumber, typeBool, true},
		{typeNumber, typeString, false},
		{typeArray, typeArray, true},
		{typeArray, typeObject, false},
		{typeObject, typeObject, true},
		{typeObject, typeArray, false},
		{typeString, typeNumber, true},
		{typeString, typeBool, true},
		{typeString, typeArray, false},
		{typeString, typeObject, false},
		{typeExpr, typeString, true},
		{typeExpr, typeNumber, false},
	}
	for _, c := range cases {
		if got := argCompatible(c.want, c.got); got != c.ok {
			t.Errorf("argCompatible(%q, %q) = %v, want %v", c.want, c.got, got, c.ok)
		}
	}
	if plural(1) != "" || plural(2) != "s" {
		t.Errorf("plural: got %q/%q, want \"\"/\"s\"", plural(1), plural(2))
	}
}
