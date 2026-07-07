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
