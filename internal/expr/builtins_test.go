package expr

import "testing"

func TestBuiltins(t *testing.T) {
	ctx := map[string]any{
		"state": map[string]any{
			"name":  "  Ada  ",
			"email": "ada@example.com",
			"bad":   "nope",
			"age":   float64(30),
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
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}
