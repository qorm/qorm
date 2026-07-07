package runtime

import "testing"

func TestFillMessage(t *testing.T) {
	cases := []struct {
		tmpl string
		ctx  map[string]any
		want string
	}{
		{"Hello, {name}!", map[string]any{"name": "Ada"}, "Hello, Ada!"},
		{"{n, plural, one {# item} other {# items}}", map[string]any{"n": float64(1)}, "1 item"},
		{"{n, plural, one {# item} other {# items}}", map[string]any{"n": float64(4)}, "4 items"},
		{"{n, plural, =0 {none} other {# left}}", map[string]any{"n": float64(0)}, "none"},
		{"{n, plural, other {# for {name}}}", map[string]any{"n": float64(2), "name": "Ada"}, "2 for Ada"},
		{"{g, select, male {he} female {she} other {they}}", map[string]any{"g": "female"}, "she"},
		{"{g, select, male {he} other {they}}", map[string]any{"g": "x"}, "they"},
		{"Hi {missing}", map[string]any{}, "Hi {missing}"},
	}
	for _, c := range cases {
		if got := fillMessage(c.tmpl, c.ctx); got != c.want {
			t.Errorf("fillMessage(%q) = %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestNumberFormatting(t *testing.T) {
	// en groups with "," / "."; de with "." / "," — proves locale separators.
	cases := []struct {
		tmpl, locale, want string
		n                  float64
	}{
		{"{v, number}", "en", "1,234,567", 1234567},
		{"{v, number}", "de", "1.234.567", 1234567},
		{"{v, number}", "en", "1,234.5", 1234.5},
		{"{v, number}", "de", "1.234,5", 1234.5},
		{"{v, number, percent}", "en", "42%", 0.42},
		{"{v, number}", "en", "-2,500", -2500},
		{"{v, number}", "en", "42", 42},
	}
	for _, c := range cases {
		ctx := map[string]any{"v": c.n, "__locale": c.locale}
		if got := fillMessage(c.tmpl, ctx); got != c.want {
			t.Errorf("fillMessage(%q, %s) = %q, want %q", c.tmpl, c.locale, got, c.want)
		}
	}
}

func TestCurrencyAndDate(t *testing.T) {
	cur := func(v float64, loc, code string) string {
		return fillMessage("{v, currency, "+code+"}", map[string]any{"v": v, "__locale": loc})
	}
	if got := cur(1234.5, "en", "USD"); got != "$1,234.50" {
		t.Errorf("USD/en = %q", got)
	}
	if got := cur(1234.5, "de", "EUR"); got != "1.234,50 €" {
		t.Errorf("EUR/de = %q", got)
	}
	if got := cur(1000, "ja", "JPY"); got != "¥1,000" {
		t.Errorf("JPY/ja = %q", got)
	}

	dt := func(v any, loc, style string) string {
		spec := "{d, date}"
		if style != "" {
			spec = "{d, date, " + style + "}"
		}
		return fillMessage(spec, map[string]any{"d": v, "__locale": loc})
	}
	if got := dt("2026-07-04", "en", ""); got != "2026-07-04" {
		t.Errorf("iso date = %q", got)
	}
	if got := dt("2026-07-04", "en", "short"); got != "7/4/2026" {
		t.Errorf("en short = %q", got)
	}
	if got := dt("2026-07-04", "de", "short"); got != "4.7.2026" {
		t.Errorf("de short = %q", got)
	}
	if got := fillMessage("{d, time}", map[string]any{"d": "2026-07-04T09:05:00Z", "__locale": "en"}); got != "09:05" {
		t.Errorf("time = %q", got)
	}
}

func TestCLDRPluralRules(t *testing.T) {
	// Russian: 1->one, 2..4->few, 5..20->many, 21->one, 22->few
	ru := "{n, plural, one {штука} few {штуки} many {штук} other {?}}"
	check := func(loc, tmpl string, n float64, want string) {
		got := fillMessage(tmpl, map[string]any{"n": n, "__locale": loc})
		if got != want {
			t.Errorf("%s n=%g = %q, want %q", loc, n, got, want)
		}
	}
	check("ru", ru, 1, "штука")
	check("ru", ru, 2, "штуки")
	check("ru", ru, 5, "штук")
	check("ru", ru, 21, "штука")
	check("ru", ru, 22, "штуки")
	// French: 0 and 1 are "one"
	fr := "{n, plural, one {jour} other {jours}}"
	check("fr", fr, 0, "jour")
	check("fr", fr, 1, "jour")
	check("fr", fr, 2, "jours")
	// Arabic: 0->zero, 2->two, 3->few
	ar := "{n, plural, zero { zero } one { one } two { two } few { few } many { many } other { other }}"
	check("ar", ar, 0, " zero ")
	check("ar", ar, 2, " two ")
	check("ar", ar, 3, " few ")
	// English fallback unchanged
	check("en", "{n, plural, one {a} other {b}}", 1, "a")
	check("en", "{n, plural, one {a} other {b}}", 3, "b")
}

func TestLongDate(t *testing.T) {
	d := "2026-07-04"
	cases := map[string]string{"en": "July 4, 2026", "de": "4. Juli 2026", "fr": "4 juillet 2026", "zh": "2026年7月4日"}
	for loc, want := range cases {
		got := fillMessage("{d, date, long}", map[string]any{"d": d, "__locale": loc})
		if got != want {
			t.Errorf("long date %s = %q, want %q", loc, got, want)
		}
	}
}
