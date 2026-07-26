package expr

import "testing"

// FuzzEval ensures the evaluator never panics on arbitrary binding expressions
// (they come from app JSON, so robustness matters). It may return errors, but
// must not crash.
func FuzzEval(f *testing.F) {
	seeds := []string{
		"1 + 2", "state.x", "a ? b : c", "matches(email, \"^x$\")",
		"len(trim(s)) >= 3", "((((", "1 +", "\"unterminated", "{{}}",
		"len()", "a.b.c.d.e", "-", "* /", "1e999", "!!!a", "min()",
		"default(x, y)", "1 == 1 == 1", "((1))", "a && b || c ? d : e",
		"", "   ", ".", "()", "a(", "a(,)", "\\", "0/0", "-9999999999999",
		// indexing / postfix member access
		"arr[0]", "arr[i]", "obj['k']", "obj[key]", "arr[0][1].b",
		"arr[arr[0]]", "arr[", "arr[]", "arr[0", "arr]", "arr[0].",
		"arr[.b]", "(arr)[0]", "arr[i > 0 ? 0 : 1]", "a . b", "arr[0/0]",
		"arr[-1]", "true[0]", "'s'[0]",
		// collection / format builtins and string sub-expressions
		"at(arr, -1)", "first(arr)", "last(arr)", "sum(arr)", "avg(arr)",
		"count(arr)", "count(arr, 'it > 1')", "join(arr, ',')",
		"split('a,b', ',')", "keys(obj)", "values(obj)[0]",
		"map(arr, 'it * 2')", "filter(arr, \"it.done\")",
		"map(arr, 'map(it, \"it\")')", "filter(arr, 'it +')",
		"map(arr, arr)", "sum(map(arr, 'it.price'))",
		"format('%s %d %.2f', 'x', 1, 2.5)", "format('%.999999f', 1)",
		"format('%', 1)", "format('%%')", "format('%.x', 1)",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	ctx := map[string]any{
		"state": map[string]any{"x": 1.0, "email": "a@b.co"},
		"a":     "hi", "b": true, "s": " x ", "email": "a@b.co",
		"arr": []any{1.0, "two", map[string]any{"done": true, "price": 2.0}},
		"obj": map[string]any{"k": "v", "n": 3.0},
		"i":   1.0, "key": "k",
	}
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = Eval(src, ctx) // must not panic
	})
}

// FuzzCheck ensures the static type checker never panics on arbitrary binding
// expressions (they come from app JSON, so robustness matters). It may report
// mismatches, but must not crash.
func FuzzCheck(f *testing.F) {
	seeds := []string{
		"state.x + 1", "a + 1", "len(a)", "((((", "",
		// new syntax and checked builtins
		"arr[0]", "arr[a]", "arr[arr]", "obj[a]", "arr[0].b * 2",
		"sum(a)", "at(arr)", "at(arr, a)", "count(arr, 'it', 3)",
		"map(arr, 'it +')", "filter(arr, \"sum()\")", "format()",
		"keys(arr)", "join(arr, obj)", "map(arr, 'map(it, \"it >\")')",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	vars := map[string]string{
		"state.x": "number", "a": "string",
		"arr": "array", "obj": "object",
	}
	f.Fuzz(func(t *testing.T, src string) {
		_ = Check(src, vars) // must not panic
	})
}
