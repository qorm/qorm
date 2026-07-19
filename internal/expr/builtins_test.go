package expr

import (
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
