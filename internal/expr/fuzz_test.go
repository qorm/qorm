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
	}
	for _, s := range seeds {
		f.Add(s)
	}
	ctx := map[string]any{
		"state": map[string]any{"x": 1.0, "email": "a@b.co"},
		"a":     "hi", "b": true, "s": " x ", "email": "a@b.co",
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
	}
	for _, s := range seeds {
		f.Add(s)
	}
	vars := map[string]string{"state.x": "number", "a": "string"}
	f.Fuzz(func(t *testing.T, src string) {
		_ = Check(src, vars) // must not panic
	})
}
