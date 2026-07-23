package expr

// Additional coverage for the builtin dispatch table and the static
// type-checker (Check). The checker's contract is ZERO false positives: it must
// flag non-numeric operands of numeric operators, and must never flag a
// legitimate expression.

import (
	"reflect"
	"strings"
	"testing"
)

// TestBuiltinsExtra covers the builtin branches not exercised by TestBuiltins:
// lower, startsWith, endsWith, replace, str, number/num, int, abs, floor, ceil,
// empty, default/coalesce with a truthy first arg, unknown functions, and the
// various len() argument kinds (map, number, bool, nil, missing).
func TestBuiltinsExtra(t *testing.T) {
	ctx := map[string]any{
		"m":    map[string]any{"a": 1.0, "b": 2.0},
		"arr":  []any{1.0, 2.0, 3.0},
		"num":  42.0,
		"flag": true,
	}
	cases := []struct {
		src  string
		want any
	}{
		{"lower('ABC')", "abc"},
		{"startsWith('hello', 'he')", true},
		{"startsWith('hello', 'lo')", false},
		{"endsWith('hello', 'lo')", true},
		{"endsWith('hello', 'he')", false},
		{"replace('aaa', 'a', 'b')", "bbb"},
		{"replace('hello', 'l', 'L')", "heLLo"},
		{"str(5)", "5"},
		{"str(true)", "true"},
		{"number('42')", 42.0},
		{"num('3.5')", 3.5},
		{"num('abc')", 0.0}, // non-numeric string coerces to 0
		{"int(3.9)", 3.0},
		{"int(-3.9)", -3.0},
		{"abs(-4)", 4.0},
		{"abs(4)", 4.0},
		{"floor(2.7)", 2.0},
		{"floor(-2.2)", -3.0},
		{"ceil(2.1)", 3.0},
		{"ceil(-2.7)", -2.0},
		{"empty('')", true},
		{"empty('x')", false},
		{"empty(0)", true},
		{"empty(arr)", false},
		{"default('v', 'fb')", "v"}, // truthy first arg wins
		{"default('', 'fb')", "fb"}, // falsy falls back
		{"coalesce(0, 'fb')", "fb"},
		{"coalesce('keep', 'fb')", "keep"},
		{"unknownfn(1)", nil}, // unknown builtin -> nil
		{"len(m)", 2.0},       // map
		{"len(arr)", 3.0},     // slice
		{"len(num)", 2.0},     // default: RuneCount(Stringify(42)="42")
		{"len(flag)", 4.0},    // default: Stringify(true)="true"
		{"len(null)", 0.0},    // nil arg
		{"len()", 0.0},        // missing arg -> nil -> 0
		{"min()", 0.0},        // no args -> 0
		{"max()", 0.0},
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}

	// slice with clamped bounds (negative start, oversized end).
	sliceCases := []struct {
		src  string
		want any
	}{
		{"slice(arr, 1, 3)", []any{2.0, 3.0}},
		{"slice(arr, -5, 2)", []any{1.0, 2.0}},
		{"slice(arr, 2, 99)", []any{3.0}},
		{"slice(num, 0, 2)", []any{}}, // non-array -> empty
	}
	for _, c := range sliceCases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Eval(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

// TestCheckZeroFalsePositives feeds a battery of legitimate expressions with a
// richly-typed var map and asserts the checker reports nothing. This is the
// core contract: no false positives.
func TestCheckZeroFalsePositives(t *testing.T) {
	vars := map[string]string{
		"s":   "string",
		"n":   "number",
		"b":   "bool",
		"arr": "array",
		"obj": "object",
	}
	legit := []string{
		"s + 'x'", // string concatenation
		"n + 1",   // numeric +
		"n - 1", "n * 2", "n / 3", "n % 4",
		"-n",               // unary minus on number
		"b ? s : n",        // ternary result unknown
		"n > 1 && b || !b", // comparisons and logic
		"len(arr)",         // builtin call
		"len(s) * 2",       // builtin result is unknown -> * legal
		"s == 'x'",
		"s != n", // comparisons never reported
		"s < n",
		"default(s, 'x')",
		"matches(s, '^a$')",
		"arr",         // bare array identifier
		"obj",         // bare object identifier
		"unknown * 2", // unlisted identifier is unknown
		"n + unknown",
		"str(n) + s",
		"someFunc(n - n)", // numeric subexpr inside a call is fine
		"b * 2",           // bool operand allowed (toNum: true=1)
	}
	for _, src := range legit {
		if mm := Check(src, vars); len(mm) != 0 {
			t.Errorf("Check(%q): false positive: %v", src, mm)
		}
	}
}

// TestCheckFlagsNonNumeric confirms the true positives: string/array/object
// operands of numeric operators (and unary minus) on either side are flagged.
func TestCheckFlagsNonNumeric(t *testing.T) {
	vars := map[string]string{
		"s":   "string",
		"arr": "array",
		"obj": "object",
	}
	cases := []struct {
		src  string
		want string // required Detail substring
	}{
		{"s - 1", "s is string"},
		{"arr * 2", "arr is array"},
		{"obj / 2", "obj is object"},
		{"1 - s", "s is string"}, // right operand
		{"10 % s", "s is string"},
		{"-s", "s is string"},            // unary minus
		{`"lit" * 2`, `"lit" is string`}, // string literal operand
	}
	for _, c := range cases {
		mm := Check(c.src, vars)
		if len(mm) == 0 {
			t.Errorf("Check(%q): expected a mismatch, got none", c.src)
			continue
		}
		found := false
		for _, m := range mm {
			if strings.Contains(m.Detail, c.want) && m.Expr == c.src {
				found = true
			}
		}
		if !found {
			t.Errorf("Check(%q) = %v, want a detail containing %q", c.src, mm, c.want)
		}
	}
}

// TestCheckStringConcatOperand covers the checker flagging a string-typed
// subexpression (a '+' concatenation) when it is later used numerically. This
// also exercises exprText's binary-node rendering in the diagnostic.
func TestCheckStringConcatOperand(t *testing.T) {
	vars := map[string]string{"s": "string", "n": "number"}
	mm := Check("(s + 'x') - n", vars)
	if len(mm) != 1 {
		t.Fatalf("Check: got %d mismatches, want 1: %v", len(mm), mm)
	}
	// Detail renders the binary '+' node: s + "x"
	if !strings.Contains(mm[0].Detail, `s + "x"`) {
		t.Errorf("Detail = %q, want it to render the concat subexpr `s + \"x\"`", mm[0].Detail)
	}
	if !strings.Contains(mm[0].Detail, "is string, used as number") {
		t.Errorf("Detail = %q, want 'is string, used as number'", mm[0].Detail)
	}
}

// TestCheckInferBranches exercises the checker's inference over node shapes
// (bool/null literals, unary '!', the three '+' type outcomes) and asserts the
// inferred type never produces a spurious mismatch.
func TestCheckInferBranches(t *testing.T) {
	vars := map[string]string{"s": "string", "b": "bool"}
	cases := []string{
		"true && false", // boolLit operands
		"null - 1",      // nullLit operand is unknown -> not flagged
		"!b",            // unary '!' infers bool
		"s + 'x'",       // '+' string outcome
		"1 + 2",         // '+' number outcome
		"unknown + 1",   // '+' unknown outcome
		"b ? 1 : 'x'",   // ternary infers unknown
	}
	for _, src := range cases {
		if mm := Check(src, vars); len(mm) != 0 {
			t.Errorf("Check(%q): unexpected mismatch: %v", src, mm)
		}
	}

	// infer's default branch (an unknown node type) returns typeUnknown.
	c := &checker{vars: vars}
	if got := c.infer(struct{}{}); got != typeUnknown {
		t.Errorf("infer(unknown node) = %q, want %q", got, typeUnknown)
	}
	if len(c.mismatches) != 0 {
		t.Errorf("infer(unknown node) recorded mismatches: %v", c.mismatches)
	}
}

// TestNormalizeType covers every schema-type alias (plus trim/case handling
// and the unknown fallback).
func TestNormalizeType(t *testing.T) {
	cases := map[string]string{
		"number": typeNumber, "num": typeNumber, "int": typeNumber,
		"integer": typeNumber, "float": typeNumber, "double": typeNumber,
		" NUMBER ": typeNumber, // trimmed and lower-cased
		"string":   typeString, "str": typeString, "text": typeString,
		"bool": typeBool, "boolean": typeBool, "BOOL": typeBool,
		"array": typeArray, "list": typeArray,
		"object": typeObject, "map": typeObject,
		"whatever": typeUnknown, "": typeUnknown,
	}
	for in, want := range cases {
		if got := normalizeType(in); got != want {
			t.Errorf("normalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExprText covers every branch of the diagnostic renderer, including the
// unknown-node fallback.
func TestExprText(t *testing.T) {
	cases := []struct {
		n    node
		want string
	}{
		{numLit{3}, "3"},
		{numLit{0.5}, "0.5"},
		{strLit{"hi"}, `"hi"`},
		{boolLit{true}, "true"},
		{boolLit{false}, "false"},
		{nullLit{}, "null"},
		{ident{"state.x"}, "state.x"},
		{unary{"-", numLit{5}}, "-5"},
		{unary{"!", ident{"b"}}, "!b"},
		{binary{"+", numLit{1}, numLit{2}}, "1 + 2"},
		{ternary{boolLit{true}, numLit{1}, numLit{2}}, "true ? 1 : 2"},
		{call{"len", []node{ident{"arr"}}}, "len(arr)"},
		{call{"f", []node{numLit{1}, numLit{2}}}, "f(1, 2)"},
		{struct{}{}, "?"}, // unknown node fallback
	}
	for _, c := range cases {
		if got := exprText(c.n); got != c.want {
			t.Errorf("exprText(%#v) = %q, want %q", c.n, got, c.want)
		}
	}
}
