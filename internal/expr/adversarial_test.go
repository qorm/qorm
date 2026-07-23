package expr

// Adversarial tests for the expression engine. Binding expressions come from
// app-authored JSON, so the lexer/parser/evaluator are probed with malformed
// and pathological inputs: they must return errors (never hang, never panic)
// and the guard rails (64 KB source cap, 256 depth cap) must hold.

import (
	"math"
	"strings"
	"testing"
)

// TestMalformedLexemesError asserts that structurally broken sources are
// rejected with an error rather than panicking or hanging.
func TestMalformedLexemesError(t *testing.T) {
	sources := []string{
		"",         // empty
		"   ",      // whitespace only
		".",        // lone dot operator
		"..",       // two dot operators
		"1 +",      // dangling operator
		"* /",      // leading operator
		"(",        // unbalanced open
		")",        // stray close
		"(((",      // unbalanced opens
		"1 2",      // two adjacent numbers
		"a b",      // two adjacent idents
		"a ? b",    // ternary missing ':'
		"a ? ",     // ternary missing then-branch
		"a ? b : ", // ternary missing else-branch
		"a ? : b",  // ternary missing then-branch (colon present)
		"f(1",      // call missing ')'
		"f(1,",     // call dangling comma
		"f(1,)",    // call empty trailing arg
		"f(1 +)",   // call malformed arg
		"(1",       // paren missing ')'
		"a)",       // trailing extra close
	}
	for _, src := range sources {
		if _, err := Eval(src, nil); err == nil {
			t.Errorf("Eval(%q): expected error, got nil", src)
		}
	}
}

// TestMalformedNumberLiteralErrors asserts that a malformed numeric literal —
// one the lexer's greedy digit-and-dot run accepts as a single lexeme but
// strconv.ParseFloat rejects (multiple dots) — is a parse error, not a silent
// 0. Binding expressions come from authored, untrusted JSON, so malformed
// input must fail loudly rather than coerce. The second half locks down the
// accepted set: every numeric form the lexer has always accepted still
// evaluates to the same value.
func TestMalformedNumberLiteralErrors(t *testing.T) {
	malformed := []string{"1.2.3", "1..5", "1.2.3.4", "9..", "0..1", "1.2.", "1.."}
	for _, src := range malformed {
		if _, err := Eval(src, nil); err == nil {
			t.Errorf("Eval(%q): expected error for malformed number literal, got nil", src)
		}
	}

	valid := []struct {
		src  string
		want float64
	}{
		{"1", 1.0},
		{"10", 10.0},
		{"1.5", 1.5},
		{".5", 0.5}, // leading-dot fraction
		{"1.", 1.0}, // trailing-dot integer
		{"0.25", 0.25},
	}
	for _, c := range valid {
		v, err := Eval(c.src, nil)
		if err != nil {
			t.Errorf("Eval(%q): unexpected error %v (legitimate literal must keep parsing)", c.src, err)
			continue
		}
		if v != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.src, v, c.want)
		}
	}
}

// TestUnterminatedStringErrors asserts the lexer reports an error when a
// string literal is never closed, instead of consuming to end-of-input and
// emitting the partial text as a valid string. Both quote styles, a trailing
// backslash, and an escaped closing quote (which does not terminate the
// literal) are covered.
func TestUnterminatedStringErrors(t *testing.T) {
	sources := []string{
		`"abc`,   // double-quoted, never closed
		`'xyz`,   // single-quoted, never closed
		`"a\`,    // trailing backslash, never closed
		`"a\"`,   // escaped quote does not close the literal
		`1 + "x`, // unterminated inside a larger expression
	}
	for _, src := range sources {
		if _, err := Eval(src, nil); err == nil {
			t.Errorf("Eval(%q): expected error for unterminated string, got nil", src)
		}
	}
}

// TestStringEscaping documents the lexer's escape handling: a backslash before
// the quote lets the quote be embedded, but other escapes drop the backslash
// without interpreting the sequence (no \n, \t, etc.).
func TestStringEscaping(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"a\"b"`, `a"b`}, // escaped quote is embedded
		{`"a\nb"`, "anb"}, // \n is not interpreted; backslash dropped
		{`'it\'s'`, "it's"},
	}
	for _, c := range cases {
		v, err := Eval(c.src, nil)
		if err != nil {
			t.Fatalf("Eval(%q): %v", c.src, err)
		}
		if v != c.want {
			t.Errorf("Eval(%q) = %q, want %q", c.src, v, c.want)
		}
	}
}

// TestSourceLengthCap asserts the 64 KB source cap errors cleanly (no hang)
// and that a source exactly at the cap still parses.
func TestSourceLengthCap(t *testing.T) {
	// Exactly at the cap: "1" padded with spaces to maxExprLen bytes.
	atCap := "1" + strings.Repeat(" ", maxExprLen-1)
	if len(atCap) != maxExprLen {
		t.Fatalf("setup: len=%d want %d", len(atCap), maxExprLen)
	}
	if v, err := Eval(atCap, nil); err != nil || v != 1.0 {
		t.Fatalf("Eval(at cap) = %v, %v; want 1, nil", v, err)
	}

	// One byte over the cap errors with a clear message.
	over := atCap + " "
	_, err := Eval(over, nil)
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("Eval(over cap): want 'too long' error, got %v", err)
	}

	// A clearly oversized source also errors (and quickly, no hang).
	huge := strings.Repeat("1+", maxExprLen) + "1"
	if _, err := Eval(huge, nil); err == nil {
		t.Fatalf("Eval(huge): want error, got nil")
	}
}

// TestDepthCap asserts the 256 recursion cap errors cleanly instead of
// stack-overflowing, for both parenthesized and ternary nesting.
func TestDepthCap(t *testing.T) {
	// Just under the cap evaluates fine.
	okSrc := strings.Repeat("(", maxExprDepth-1) + "1" + strings.Repeat(")", maxExprDepth-1)
	if v, err := Eval(okSrc, nil); err != nil || v != 1.0 {
		t.Fatalf("Eval(depth %d) = %v, %v; want 1, nil", maxExprDepth-1, v, err)
	}

	// At and beyond the cap returns an error (never a crash).
	for _, depth := range []int{maxExprDepth, maxExprDepth + 1, 1000} {
		src := strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth)
		if _, err := Eval(src, nil); err == nil || !strings.Contains(err.Error(), "too deeply nested") {
			t.Errorf("Eval(parens depth %d): want 'too deeply nested' error, got %v", depth, err)
		}
	}

	// Deep right-nested ternary chains are bounded too.
	var b strings.Builder
	for i := 0; i < maxExprDepth+50; i++ {
		b.WriteString("1?")
	}
	b.WriteString("2")
	for i := 0; i < maxExprDepth+50; i++ {
		b.WriteString(":3")
	}
	if _, err := Eval(b.String(), nil); err == nil {
		t.Errorf("Eval(deep ternary): want error, got nil")
	}
}

// TestOperatorPrecedence locks down precedence and associativity so a refactor
// of the precedence table cannot silently change evaluation.
func TestOperatorPrecedence(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"1 + 2 * 3", 7.0},
		{"(1 + 2) * 3", 9.0},
		{"2 + 3 * 4 - 5", 9.0},
		{"10 - 2 - 3", 5.0},   // left-associative
		{"100 / 10 / 2", 5.0}, // left-associative
		{"2 * 3 % 4", 2.0},    // 6 % 4
		{"1 + 2 == 3", true},  // arithmetic binds tighter than ==
		{"1 < 2 && 2 < 3", true},
		{"true || false && false", true}, // && binds tighter than ||
		{"1 == 1 == 1", true},            // left-assoc; (1==1)==1 -> true==1 -> true
		{"-2 * 3", -6.0},
		{"-(2 + 3)", -5.0},
		{"!true", false},
		{"!!true", true},
		{"- -5", 5.0},
	}
	for _, c := range cases {
		got, err := Eval(c.src, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestTernaryAndLiterals covers ternary branch selection, nested/right-assoc
// ternaries, truthiness of literals, and the literal node kinds.
func TestTernaryAndLiterals(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"true", true},
		{"false", false},
		{"null", nil},
		{"nil", nil},
		{"true ? 1 : 2", 1.0},
		{"false ? 1 : 2", 2.0}, // exercises the else branch
		{"0 ? 'y' : 'n'", "n"},
		{"'' ? 'y' : 'n'", "n"},
		{"'x' ? 'y' : 'n'", "y"},
		{"null ? 1 : 2", 2.0},
		{"2 > 1 ? 10 : 20", 10.0},
		{"2 < 1 ? 10 : 20", 20.0},
		{"1 > 0 ? 2 > 1 ? 'a' : 'b' : 'c'", "a"}, // nested then-branch ternary
		{"false ? 1 : 0 ? 2 : 3", 3.0},           // right-assoc: false ? 1 : (0 ? 2 : 3)
	}
	for _, c := range cases {
		got, err := Eval(c.src, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestArithmeticAndModulo covers the string-concat branch of '+', integer
// modulo (including the zero-divisor guard), and division producing IEEE
// inf/nan without panicking.
func TestArithmeticAndModulo(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"'a' + 'b'", "ab"},
		{"'n=' + 5", "n=5"}, // string + number -> concatenation
		{"5 + 'x'", "5x"},   // number + string -> concatenation
		{"1 + 2", 3.0},      // number + number -> addition
		{"7 % 3", 1.0},
		{"10 % 3", 1.0},
		{"-7 % 3", -1.0},  // dividend sign preserved (Go semantics)
		{"10 % 0", 0.0},   // zero divisor guarded -> 0, no panic
		{"10 % 0.5", 0.0}, // divisor truncates to 0 -> guarded
	}
	for _, c := range cases {
		got, err := Eval(c.src, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}

	// Division by zero yields IEEE values, never a panic. Documented here so a
	// future guard (or crash) is noticed. See notes on the / vs % asymmetry.
	if v, _ := Eval("1 / 0", nil); v != math.Inf(1) {
		t.Errorf("1/0 = %v, want +Inf", v)
	}
	if v, _ := Eval("-1 / 0", nil); v != math.Inf(-1) {
		t.Errorf("-1/0 = %v, want -Inf", v)
	}
	if v, _ := Eval("0 / 0", nil); !math.IsNaN(v.(float64)) {
		t.Errorf("0/0 = %v, want NaN", v)
	}
}

// TestComparisonOperators covers the <, <=, >, >=, ==, != evaluation branches
// over both numbers and strings.
func TestComparisonOperators(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"1 < 2", true},
		{"2 < 1", false},
		{"1 <= 1", true},
		{"2 <= 1", false},
		{"2 > 1", true},
		{"1 > 2", false},
		{"1 >= 1", true},
		{"0 >= 1", false},
		{"1 != 2", true},
		{"1 != 1", false},
		{"'a' < 'b'", true},
		{"'b' <= 'a'", false},
		{"'z' > 'a'", true},
		{"'a' == 'a'", true},
		{"'3' == 3", true}, // string operand -> Stringify comparison
	}
	for _, c := range cases {
		got, err := Eval(c.src, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

// TestLookupDotted exercises dotted member access, missing keys, and
// traversal through a non-map intermediate (must yield nil, not panic).
func TestLookupDotted(t *testing.T) {
	ctx := map[string]any{
		"state": map[string]any{
			"count": 5.0,
			"nested": map[string]any{
				"deep": "value",
			},
		},
		"scalar": "not-a-map",
	}
	cases := []struct {
		src  string
		want any
	}{
		{"state.count", 5.0},
		{"state.nested.deep", "value"},
		{"state.missing", nil},
		{"state.nested.missing", nil},
		{"scalar.anything", nil},    // intermediate is not a map
		{"state.count.deeper", nil}, // number is not a map
		{"unknown", nil},
		{"unknown.x.y", nil},
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

// TestToNum covers every branch of the numeric coercion helper.
func TestToNum(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{3.5, 3.5},
		{7, 7.0},        // int
		{true, 1.0},     // bool true
		{false, 0.0},    // bool false
		{"42", 42.0},    // numeric string
		{"3.5", 3.5},    // float string
		{"abc", 0.0},    // non-numeric string -> 0
		{"", 0.0},       // empty string -> 0
		{[]any{1}, 0.0}, // other -> 0
		{nil, 0.0},
		{struct{}{}, 0.0},
	}
	for _, c := range cases {
		if got := toNum(c.in); got != c.want {
			t.Errorf("toNum(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestTruthy covers every branch of the truthiness helper.
func TestTruthy(t *testing.T) {
	trueCases := []any{true, 1.0, -1.0, "x", []any{1}, map[string]any{"k": 1}, struct{}{}}
	for _, v := range trueCases {
		if !truthy(v) {
			t.Errorf("truthy(%#v) = false, want true", v)
		}
	}
	falseCases := []any{nil, false, 0.0, "", []any{}, map[string]any{}}
	for _, v := range falseCases {
		if truthy(v) {
			t.Errorf("truthy(%#v) = true, want false", v)
		}
	}
}

// TestEquals covers the string, bool (both sides), and numeric paths.
func TestEquals(t *testing.T) {
	cases := []struct {
		l, r any
		want bool
	}{
		{"3", 3.0, true}, // string operand -> Stringify equality
		{"3", 4.0, false},
		{3.0, "3", true},  // string on the right too
		{1.0, true, true}, // right bool -> truthy equality
		{0.0, true, false},
		{true, 1.0, true}, // left bool -> truthy equality
		{true, 0.0, false},
		{2.0, 2.0, true}, // pure numeric
		{2.0, 3.0, false},
		{nil, nil, true}, // both non-string non-bool -> toNum(0)==toNum(0)
	}
	for _, c := range cases {
		if got := equals(c.l, c.r); got != c.want {
			t.Errorf("equals(%#v, %#v) = %v, want %v", c.l, c.r, got, c.want)
		}
	}
}

// TestCompare covers string (lexical) and numeric comparison.
func TestCompare(t *testing.T) {
	if got := compare("a", "b"); got >= 0 {
		t.Errorf("compare(a,b) = %d, want < 0", got)
	}
	if got := compare("b", "a"); got <= 0 {
		t.Errorf("compare(b,a) = %d, want > 0", got)
	}
	if got := compare("a", "a"); got != 0 {
		t.Errorf("compare(a,a) = %d, want 0", got)
	}
	// String comparison is lexical, not numeric: "10" < "9".
	if got := compare("10", "9"); got >= 0 {
		t.Errorf("compare(\"10\",\"9\") = %d, want < 0 (lexical)", got)
	}
	if got := compare(1.0, 2.0); got >= 0 {
		t.Errorf("compare(1,2) = %d, want < 0", got)
	}
	if got := compare(2.0, 1.0); got <= 0 {
		t.Errorf("compare(2,1) = %d, want > 0", got)
	}
	if got := compare(2.0, 2.0); got != 0 {
		t.Errorf("compare(2,2) = %d, want 0", got)
	}
}

// TestStringifyAllBranches covers every case of the string interpolation
// helper, including the large-float and non-scalar default paths.
func TestStringifyAllBranches(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{true, "true"},
		{false, "false"},
		{3.0, "3"}, // integral float -> int format
		{-2.0, "-2"},
		{0.5, "0.5"},    // fractional float -> %g
		{1e20, "1e+20"}, // integral but overflows int64 -> %g
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{[]any{"a", "b"}, "[a b]"},             // default %v
		{map[string]any{"k": "v"}, "map[k:v]"}, // default %v (single key: stable)
	}
	for _, c := range cases {
		if got := Stringify(c.in); got != c.want {
			t.Errorf("Stringify(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEvalNodeAndBinaryFallthrough drives the unreachable-by-parsing default
// branches directly so they are covered and pinned to nil.
func TestEvalNodeAndBinaryFallthrough(t *testing.T) {
	// An unknown node type yields nil (evalNode default).
	if got := evalNode(struct{}{}, nil); got != nil {
		t.Errorf("evalNode(unknown) = %v, want nil", got)
	}
	// An unknown operator yields nil (evalBinary default).
	if got := evalBinary(binary{op: "??", l: numLit{1}, r: numLit{2}}, nil); got != nil {
		t.Errorf("evalBinary(unknown op) = %v, want nil", got)
	}
}
