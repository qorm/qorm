package expr

// Tests for postfix indexing: arr[0], arr[i], obj[key], chained brackets, and
// dotted member access resuming after a bracket (users[0].name). Indexing is
// lenient at runtime — out-of-range, missing keys, and non-indexable bases
// yield nil, never a panic — and strict in the parser (malformed brackets are
// errors, since bindings are authored JSON).

import (
	"reflect"
	"strings"
	"testing"
)

func indexCtx() map[string]any {
	return map[string]any{
		"state": map[string]any{
			"items": []any{10.0, 20.0, 30.0},
			"grid":  []any{[]any{1.0, 2.0}, []any{3.0}},
			"users": []any{
				map[string]any{"name": "Ada", "pet": map[string]any{"name": "cat"}},
				map[string]any{"name": "Bob"},
			},
			"user":  map[string]any{"name": "Ada", "age": 30.0},
			"idx":   1.0,
			"key":   "name",
			"count": 5.0,
		},
		"a": map[string]any{"b": 5.0},
	}
}

func TestIndexing(t *testing.T) {
	ctx := indexCtx()
	cases := []struct {
		src  string
		want any
	}{
		// list indexing
		{`state.items[0]`, 10.0},
		{`state.items[2]`, 30.0},
		{`state.items[state.idx]`, 20.0},    // dynamic index
		{`state.items[1 + 1]`, 30.0},        // expression index
		{`state.items[true ? 0 : 1]`, 10.0}, /* ternary index */
		{`state.items[1.9]`, 20.0},          // fractional index truncates
		{`state.items['1']`, 20.0},          // numeric-string index coerces
		{`state.items[3]`, nil},             // out of range
		{`state.items[-1]`, nil},            // plain indexing has no negative wrap
		{`state.items[0/0]`, nil},           // NaN index
		{`state.items[null]`, nil},          // nil index
		// object indexing
		{`state.user['name']`, "Ada"},
		{`state.user[state.key]`, "Ada"}, // dynamic key
		{`state.user['missing']`, nil},
		{`state.user[null]`, nil},
		{`state.user[0]`, nil}, // numeric key stringifies to "0": missing
		// non-indexable bases
		{`state.count[0]`, nil},
		{`'abc'[0]`, nil}, // strings are not indexable
		{`state.missing[0]`, nil},
		{`true[0]`, nil},
		{`(1 + 2)[0]`, nil},
		// chaining and member access after brackets
		{`state.grid[1][0]`, 3.0},
		{`state.users[0].name`, "Ada"},
		{`state.users[0].pet.name`, "cat"}, // dotted resume is multi-part
		{`state.users[1].pet.name`, nil},   // missing intermediate degrades
		{`state.users[state.idx].name`, "Bob"},
		// postfix on call results and parens
		{`split('a,b,c', ',')[1]`, "b"},
		{`keys(state.user)[0]`, "age"}, // sorted keys
		{`(state.items)[0]`, 10.0},
		// indexing composes with operators
		{`state.items[0] + state.items[1]`, 30.0},
		{`-state.items[1]`, -20.0},
		{`!state.items[0]`, false},
		{`state.grid[0][1] * 2`, 4.0},
		// a standalone '.' token between primaries is member access
		{`a . b`, 5.0},
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

	// 'abc' coerces to 0 under toNum, so it addresses element 0 — pin the
	// lenient coercion so a refactor cannot silently change it.
	if got, _ := Eval(`state.items['abc']`, ctx); got != 10.0 {
		t.Errorf("state.items['abc'] = %v, want 10 (toNum coercion to 0)", got)
	}
}

// TestIndexSyntaxErrors asserts malformed bracket/member syntax is a parse
// error (authored JSON must fail loudly), and that the error is stable.
func TestIndexSyntaxErrors(t *testing.T) {
	cases := []string{
		"a[",     // unterminated index
		"a[]",    // empty index expression
		"a[0",    // missing ']'
		"a[0 5]", // two expressions in one index
		"a[0)",   // mismatched close
		"[0]",    // index with no base
		"a]",     // stray close bracket
		"a[0].",  // dangling dot after bracket
		"a[0].2", // member name must be an identifier
		"(a).",   // dangling dot after paren
		"a[0] .", // spaced dangling dot
		"a[[0]",  // nested unterminated
		"a[0]]",  // extra close
		"a[1,2]", // comma is not an index operator
		"a[0][",  // chain ending unterminated
		"a[.b]",  // dot cannot start an index expression
	}
	for _, src := range cases {
		if _, err := Eval(src, nil); err == nil {
			t.Errorf("Eval(%q): expected parse error, got nil", src)
		}
	}
}

// TestIndexDepthCap asserts nested index expressions (a[a[a[...]]]) count
// into the parser depth guard, while long *chains* (a[0][0]...) are iterative
// like binary-operator chains and stay legal.
func TestIndexDepthCap(t *testing.T) {
	ctx := map[string]any{"a": []any{[]any{}}}

	// Nested: each bracket recurses through parseExpr.
	nested := strings.Repeat("a[", maxExprDepth+10) + "0" + strings.Repeat("]", maxExprDepth+10)
	if _, err := Eval(nested, ctx); err == nil || !strings.Contains(err.Error(), "too deeply nested") {
		t.Errorf("nested index past cap: want 'too deeply nested', got %v", err)
	}

	// Under the cap parses and evaluates (leniently to nil for OOB).
	small := strings.Repeat("a[", 20) + "0" + strings.Repeat("]", 20)
	if _, err := Eval(small, ctx); err != nil {
		t.Errorf("nested index under cap: unexpected error %v", err)
	}

	// A long flat chain is bounded only by the source cap, like `1+1+1+...`.
	chain := "a" + strings.Repeat("[0]", 2000)
	if v, err := Eval(chain, ctx); err != nil || v != nil {
		t.Errorf("flat chain: got %v, %v; want nil, nil", v, err)
	}
}

// TestIndexValueDirect drives indexValue's remaining branches directly.
func TestIndexValueDirect(t *testing.T) {
	if got := indexValue(nil, 0.0); got != nil {
		t.Errorf("indexValue(nil base) = %v, want nil", got)
	}
	if got := indexValue([]any{1.0}, nil); got != nil {
		t.Errorf("indexValue(nil key) = %v, want nil", got)
	}
	if got := indexValue(map[string]any{"1": "x"}, 1.0); got != "x" {
		t.Errorf("indexValue(map, 1.0) = %v, want x (stringified key)", got)
	}
	if got := indexValue("str", 0.0); got != nil {
		t.Errorf("indexValue(string base) = %v, want nil", got)
	}
}
