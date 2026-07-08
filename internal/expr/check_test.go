package expr

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	numVars := map[string]string{"state.count": "number"}
	strVars := map[string]string{"state.name": "string"}

	tests := []struct {
		name string
		src  string
		vars map[string]string
		want []string // required substrings of each mismatch Detail, in order
	}{
		{"number minus number ok", "state.count - 1", numVars, nil},
		{"number arithmetic ok", "(state.count * 2 + 1) % 3", numVars, nil},
		{"string in minus reported", "state.name - 1", strVars,
			[]string{"state.name is string, used as number"}},
		{"string in times reported", "state.name * 2", strVars,
			[]string{"state.name is string, used as number"}},
		{"string in divide reported", "10 / state.name", strVars,
			[]string{"state.name is string, used as number"}},
		{"string in modulo reported", "state.name % 2", strVars,
			[]string{"state.name is string, used as number"}},
		{"unary minus on string reported", "-state.name", strVars,
			[]string{"state.name is string, used as number"}},
		{"string literal in times reported", `"a" * 2`, nil,
			[]string{`"a" is string, used as number`}},
		{"plus with string is concat", "state.name + '!'", strVars, nil},
		{"plus mixing string and number is concat", "state.count + state.name",
			map[string]string{"state.count": "number", "state.name": "string"}, nil},
		{"unknown identifier passes", "state.other * 2", nil, nil},
		{"unknown identifier passes with vars", "state.other * 2", numVars, nil},
		{"bool operand allowed", "state.flag * 2",
			map[string]string{"state.flag": "bool"}, nil},
		{"array in arithmetic reported", "state.items / 2",
			map[string]string{"state.items": "array"}, []string{"state.items is array, used as number"}},
		{"object in arithmetic reported", "state.user - 1",
			map[string]string{"state.user": "object"}, []string{"state.user is object, used as number"}},
		{"builtin call result unknown", "len(state.items) * 2",
			map[string]string{"state.items": "array"}, nil},
		{"builtin argument still checked", "abs(state.name - 1)", strVars,
			[]string{"state.name is string, used as number"}},
		{"comparison never reported", "state.name > 3", strVars, nil},
		{"logic never reported", "state.name && state.count", strVars, nil},
		{"ternary result unknown", "(state.flag ? 1 : 'a') * 2",
			map[string]string{"state.flag": "bool"}, nil},
		{"ternary branches still checked", "state.flag ? state.name - 1 : 0",
			map[string]string{"state.flag": "bool", "state.name": "string"},
			[]string{"state.name is string, used as number"}},
		{"nested mixed reports only the string side", "(state.count + 1) * state.name",
			map[string]string{"state.count": "number", "state.name": "string"},
			[]string{"state.name is string, used as number"}},
		{"both operands reported", "state.a - state.b",
			map[string]string{"state.a": "string", "state.b": "array"},
			[]string{"state.a is string", "state.b is array"}},
		{"boolean schema alias", "state.on * 1",
			map[string]string{"state.on": "boolean"}, nil},
		{"unrecognized schema type is unknown", "state.x * 1",
			map[string]string{"state.x": "whatever"}, nil},
		{"parse error yields nothing", "state.count - ", numVars, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.src, tt.vars)
			if len(got) != len(tt.want) {
				t.Fatalf("Check(%q) = %v, want %d mismatches", tt.src, got, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.Contains(got[i].Detail, want) {
					t.Errorf("mismatch %d detail %q, want substring %q", i, got[i].Detail, want)
				}
				if got[i].Expr != tt.src {
					t.Errorf("mismatch %d Expr = %q, want %q", i, got[i].Expr, tt.src)
				}
			}
		})
	}
}
