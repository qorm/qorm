package runtime

import "testing"

// FuzzFillMessage ensures the ICU-lite message formatter never panics on
// arbitrary catalog strings (translations come from app JSON).
func FuzzFillMessage(f *testing.F) {
	for _, s := range []string{
		"Hello {name}", "{n, plural, one {#} other {# x}}", "{g, select, a {x} other {y}}",
		"{v, number}", "{v, number, percent}", "{v, currency, USD}", "{d, date, long}",
		"{{{{", "}}}}", "{n, plural,", "{,}", "{a, b, c {d}}", "#", "{}", "",
		"{n, plural, =0 {} other {}}", "{x, number, ", "{d, date}",
	} {
		f.Add(s)
	}
	ctx := map[string]any{"name": "Ada", "n": float64(3), "g": "a", "v": float64(1234.5), "d": "2026-07-04", "__locale": "en"}
	f.Fuzz(func(t *testing.T, src string) {
		_ = fillMessage(src, ctx) // must not panic
	})
}
