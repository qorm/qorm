package expr

import (
	"math"
	"reflect"
	"testing"
)

func TestBuiltins(t *testing.T) {
	ctx := map[string]any{
		"state": map[string]any{
			"name":  "  Ada  ",
			"email": "ada@example.com",
			"bad":   "nope",
			"age":   float64(30),
			"list":  []any{"a", "b", "c"},
			"page":  float64(2),
		},
	}
	cases := []struct {
		src  string
		want any
	}{
		{`len(state.name)`, float64(7)},
		{`len(trim(state.name))`, float64(3)},
		{`upper(trim(state.name))`, "ADA"},
		{`contains(state.email, "@")`, true},
		{`matches(state.email, "^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$")`, true},
		{`matches(state.bad, "^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$")`, false},
		{`len(trim(state.name)) >= 3`, true},
		{`min(3, 7, 2)`, float64(2)},
		{`max(3, 7, 2)`, float64(7)},
		{`round(4.6)`, float64(5)},
		{`not(contains(state.email, "@"))`, false},
		{`default(state.missing, "fallback")`, "fallback"},
		{`state.age >= 18 ? "adult" : "minor"`, "adult"},
		{`len(slice(state.list, 0, 2))`, float64(2)},
		{`len(slice(state.list, (state.page-1)*2, state.page*2))`, float64(1)},
		{`slice(state.list, 0, 2)`, []any{"a", "b"}},
		{`slice(state.list, -3, 99)`, []any{"a", "b", "c"}},
		{`len(slice(state.list, 2, 1))`, float64(0)},
		{`len(slice(state.list, 0, 0))`, float64(0)},
		{`slice(state.list, 5)`, []any{}},
		{`slice(state.list, 1)`, []any{"b", "c"}},
		{`slice(state.missing, 0, 2)`, []any{}},
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

func TestGameBuiltins(t *testing.T) {
	if got, _ := Eval(`len(range(5))`, nil); got != 5.0 {
		t.Errorf("len(range(5)) = %v", got)
	}
	r, _ := Eval(`range(3)`, nil)
	if arr, ok := r.([]any); !ok || len(arr) != 3 || arr[2] != 2.0 {
		t.Errorf("range(3) = %v", r)
	}
	f, _ := Eval(`fill(4, 7)`, nil)
	if arr, ok := f.([]any); !ok || len(arr) != 4 || arr[3] != 7.0 {
		t.Errorf("fill(4,7) = %v", f)
	}
	c, _ := Eval(`concat(range(2), fill(2, 9), 4)`, nil)
	if arr, ok := c.([]any); !ok || len(arr) != 5 || arr[0] != 0.0 || arr[1] != 1.0 || arr[3] != 9.0 || arr[4] != 4.0 {
		t.Errorf("concat(range(2),fill(2,9),4) = %v", c)
	}
	if got, _ := Eval(`len(range(-3))`, nil); got != 0.0 {
		t.Errorf("range(-3) must clamp to empty: %v", got)
	}
	if got, _ := Eval(`len(concat(range(3), fill(2, 0)))`, nil); got != 5.0 {
		t.Errorf("concat(range,fill) len = %v", got)
	}
}

// v2 array methods (functional) — direct CallBuiltin (Eval has no array
// literals; that is a qscript feature, covered by the qscript parity tests).
func TestArrayMethodsV2(t *testing.T) {
	// Values must be float64 (the expression language's number type), not
	// Go ints — indexOf/includes/sort compare numeric values numerically.
	l := func(vals ...any) []any {
		out := make([]any, len(vals))
		for i, v := range vals {
			if n, ok := v.(int); ok {
				out[i] = float64(n)
			} else {
				out[i] = v
			}
		}
		return out
	}
	join := func(a []any) string { return Stringify(CallBuiltin("join", []any{a, ","})) }
	cases := []struct {
		name string
		args []any
		want string
	}{
		{"push", []any{l(1, 2), float64(3)}, "1,2,3"},
		{"unshift", []any{l(1, 2), float64(0)}, "0,1,2"},
		{"pop", []any{l(1, 2, 3)}, "1,2"},
		{"shift", []any{l(1, 2, 3)}, "2,3"},
		{"reverse", []any{l(1, 2, 3)}, "3,2,1"},
		{"sort", []any{l(3, 1, 2)}, "1,2,3"},
		{"indexOf", []any{l(5, 9, 2), float64(9)}, "1"},
		{"indexOf", []any{l(5, 9, 2), float64(42)}, "-1"},
		{"includes", []any{l(5, 9), float64(9)}, "true"},
		{"includes", []any{l(5, 9), float64(8)}, "false"},
		{"includes", []any{"hello", "ell"}, "true"},
		{"charAt", []any{"hello", float64(1)}, "e"},
		{"charAt", []any{"hello", float64(99)}, ""},
		{"substring", []any{"abcdef", float64(2), float64(4)}, "cd"},
		{"substring", []any{"abcdef", float64(4), float64(2)}, ""},
		{"repeat", []any{"ab", float64(3)}, "ababab"},
		{"padStart", []any{"5", float64(3), "0"}, "005"},
		{"padEnd", []any{"5", float64(3), "0"}, "500"},
		{"trimStart", []any{"  x  "}, "x  "},
		{"trimEnd", []any{"  x  "}, "  x"},
	}
	for _, c := range cases {
		res := CallBuiltin(c.name, c.args)
		// Array-returning methods join with "," for comparison; scalar
		// results (indexOf, includes, strings) stringify directly.
		var got string
		if a, ok := res.([]any); ok {
			got = join(a)
		} else {
			got = Stringify(res)
		}
		if got != c.want {
			t.Errorf("%s%v → %q, want %q", c.name, c.args, got, c.want)
		}
	}
	// sort class ordering: numbers, strings, bools, nil.
	mixed := join(CallBuiltin("sort", []any{l("b", float64(2), "a", float64(1), true, false, nil)}).([]any))
	if mixed != "1,2,a,b,false,true," {
		t.Errorf("sort mixed → %q", mixed)
	}
	// Functional purity: push/pop must not mutate their subject.
	subj := []any{float64(1), float64(2)}
	CallBuiltin("push", []any{subj, float64(3)})
	CallBuiltin("pop", []any{subj})
	if len(subj) != 2 {
		t.Errorf("builtins mutated their subject: len=%d want 2", len(subj))
	}
}

// Edge branches of the v2 helpers: NaN ordering, nil equality, empty pad.
func TestArrayMethodsV2Edges(t *testing.T) {
	l := func(vals ...any) []any {
		out := make([]any, len(vals))
		for i, v := range vals {
			if n, ok := v.(int); ok {
				out[i] = float64(n)
			} else {
				out[i] = v
			}
		}
		return out
	}
	// NaN sorts before any number (smallest) and stays stable.
	nan := math.NaN()
	sorted := CallBuiltin("sort", []any{l(nan, float64(3), float64(1))}).([]any)
	if len(sorted) != 3 || !math.IsNaN(sorted[0].(float64)) || sorted[1] != float64(1) || sorted[2] != float64(3) {
		t.Errorf("sort with NaN → %v", sorted)
	}
	// indexOf/includes never match nil.
	if got := CallBuiltin("indexOf", []any{l(1, nil), nil}); got != float64(-1) {
		t.Errorf("indexOf with nil → %v, want -1", got)
	}
	if got := CallBuiltin("includes", []any{l(1, nil), nil}); got != false {
		t.Errorf("includes with nil → %v, want false", got)
	}
	// Empty subject list.
	if got := CallBuiltin("pop", []any{[]any{}}).([]any); len(got) != 0 {
		t.Errorf("pop([]) → %v", got)
	}
	if got := CallBuiltin("shift", []any{[]any{}}).([]any); len(got) != 0 {
		t.Errorf("shift([]) → %v", got)
	}
	// Non-list subjects degrade to zero results.
	if got := CallBuiltin("reverse", []any{"nope"}).([]any); len(got) != 0 {
		t.Errorf("reverse(non-list) → %v", got)
	}
	if got := CallBuiltin("sort", []any{"nope"}).([]any); len(got) != 0 {
		t.Errorf("sort(non-list) → %v", got)
	}
	// pad with empty pad char falls back to space; n<=0 returns unchanged.
	if got := CallBuiltin("padStart", []any{"x", float64(3), ""}); got != "  x" {
		t.Errorf("padStart empty pad → %q", got)
	}
	if got := CallBuiltin("padStart", []any{"x", float64(0), "0"}); got != "x" {
		t.Errorf("padStart n=0 → %q", got)
	}
	if got := CallBuiltin("padEnd", []any{"x", float64(1), "0"}); got != "x" {
		t.Errorf("padEnd already-long → %q", got)
	}
	// repeat caps at 2^20 and 0 → "".
	if got := CallBuiltin("repeat", []any{"a", float64(0)}); got != "" {
		t.Errorf("repeat(0) → %q", got)
	}
}
