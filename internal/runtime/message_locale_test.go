package runtime

import (
	"testing"
	"time"
)

// msg is a shorthand for fillMessage with an explicit locale.
func msg(tmpl, locale string, ctx map[string]any) string {
	if ctx == nil {
		ctx = map[string]any{}
	}
	ctx["__locale"] = locale
	return fillMessage(tmpl, ctx)
}

func TestDateParsingVariants(t *testing.T) {
	// A fixed instant: 2026-07-04 12:30:00 UTC, as epoch seconds and millis.
	epoch := float64(time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC).Unix())

	cases := []struct {
		name string
		v    any
		want string
	}{
		{"epoch seconds", epoch, "2026-07-04"},
		{"epoch millis", epoch * 1000, "2026-07-04"},
		{"rfc3339", "2026-07-04T12:30:00Z", "2026-07-04"},
		{"iso no zone", "2026-07-04T12:30:00", "2026-07-04"},
		{"date only", "2026-07-04", "2026-07-04"},
		{"slashes", "2026/07/04", "2026-07-04"},
	}
	for _, c := range cases {
		if got := msg("{d, date}", "en", map[string]any{"d": c.v}); got != c.want {
			t.Errorf("date %s: got %q, want %q", c.name, got, c.want)
		}
	}

	// Unparseable values render as empty (never garbage, never a panic).
	for _, v := range []any{"garbage", "2026-13-99", true, nil, map[string]any{}} {
		if got := msg("{d, date}", "en", map[string]any{"d": v}); got != "" {
			t.Errorf("invalid date %v: got %q, want empty", v, got)
		}
	}

	// {d, time} on an invalid date is empty too; the date,time style uses 15:04.
	if got := msg("{d, time}", "en", map[string]any{"d": "nope"}); got != "" {
		t.Errorf("invalid time: got %q", got)
	}
	if got := msg("{d, time}", "en", map[string]any{"d": epoch}); got != "12:30" {
		t.Errorf("time from epoch: got %q", got)
	}
	if got := msg("{d, date, time}", "en", map[string]any{"d": "2026-07-04T12:30:00Z"}); got != "12:30" {
		t.Errorf("date,time style: got %q", got)
	}
}

func TestDateStylesByLocale(t *testing.T) {
	d := "2026-07-04"
	cases := []struct {
		locale, style, want string
	}{
		{"en", "short", "7/4/2026"},
		{"de", "short", "4.7.2026"},
		{"ja", "short", "2026/7/4"},
		{"ru", "short", "04.07.2026"},
		{"ko", "short", "2026. 7. 4."},
		// A locale without a short layout falls back to ISO.
		{"tr", "short", "2026-07-04"},
		// Regional tags strip down to their base language layout/names.
		{"pt-BR", "short", "4/7/2026"},
		{"zh-CN", "long", "2026年7月4日"},
		// Unknown style falls back to ISO as well.
		{"en", "medium", "2026-07-04"},
		// Long dates: CJK and fallback-to-English month names.
		{"ko", "long", "2026년 7월 4일"},
		{"ja", "long", "2026年7月4日"},
		{"es", "long", "4 julio 2026"},
		{"ru", "long", "4 июля 2026"},
		{"tr", "long", "4 July 2026"}, // unknown locale -> English month, generic order
	}
	for _, c := range cases {
		if got := msg("{d, date, "+c.style+"}", c.locale, map[string]any{"d": d}); got != c.want {
			t.Errorf("%s %s date: got %q, want %q", c.locale, c.style, got, c.want)
		}
	}
}

func TestPluralCategoriesByLocale(t *testing.T) {
	check := func(locale, tmpl string, n any, want string) {
		t.Helper()
		if got := msg(tmpl, locale, map[string]any{"n": n}); got != want {
			t.Errorf("%s n=%v: got %q, want %q", locale, n, got, want)
		}
	}

	// Polish: 1 one, 2-4 few (but 12-14 many), 5+ many.
	pl := "{n, plural, one {o} few {f} many {m} other {?}}"
	check("pl", pl, 1.0, "o")
	check("pl", pl, 2.0, "f")
	check("pl", pl, 5.0, "m")
	check("pl", pl, 12.0, "m")
	check("pl", pl, 22.0, "f")
	check("pl", pl, 25.0, "m")

	// Czech: 1 one, 2-4 few, everything else other.
	cs := "{n, plural, one {o} few {f} other {r}}"
	check("cs", cs, 1.0, "o")
	check("cs", cs, 3.0, "f")
	check("cs", cs, 4.0, "f")
	check("cs", cs, 5.0, "r")

	// East-Asian locales have a single "other" form.
	check("zh", "{n, plural, other {#个}}", 1.0, "1个")
	check("ko", "{n, plural, other {#개}}", 7.0, "7개")
	check("vi", "{n, plural, other {x}}", 1.0, "x")

	// Arabic covers the zero/one/two/few/many/other spectrum.
	ar := "{n, plural, zero {z} one {o} two {t} few {f} many {m} other {r}}"
	check("ar", ar, 0.0, "z")
	check("ar", ar, 1.0, "o")
	check("ar", ar, 2.0, "t")
	check("ar", ar, 5.0, "f")
	check("ar", ar, 11.0, "m")
	check("ar", ar, 100.0, "r")

	// Russian 11-14 are "many" even though they end in 1-4.
	ru := "{n, plural, one {o} few {f} many {m} other {?}}"
	check("ru", ru, 11.0, "m")
	check("ru", ru, 14.0, "m")

	// The number may arrive as a string or int (coerced via toNumber).
	check("en", "{n, plural, one {a} other {b}}", "1", "a")
	check("en", "{n, plural, one {a} other {b}}", int(3), "b")
}

func TestPluralExactAndHash(t *testing.T) {
	// Exact =N matches win over the category, including fractional values.
	tmpl := "{n, plural, =1.5 {one-and-a-half} =2 {exactly two} one {# thing} other {# things}}"
	if got := msg(tmpl, "en", map[string]any{"n": 1.5}); got != "one-and-a-half" {
		t.Errorf("=1.5 exact: got %q", got)
	}
	if got := msg(tmpl, "en", map[string]any{"n": 2.0}); got != "exactly two" {
		t.Errorf("=2 exact beats one/other: got %q", got)
	}
	// # is the stringified number (integer form when whole).
	if got := msg(tmpl, "en", map[string]any{"n": 5.0}); got != "5 things" {
		t.Errorf("# substitution: got %q", got)
	}
	// Trailing garbage after the last form is skipped without hanging.
	if got := msg("{n, plural, other {done} garbage", "en", map[string]any{"n": 2.0}); got != "done" {
		t.Errorf("trailing junk after forms: got %q", got)
	}
}

// TestNestedPluralHash is a regression test: inside a nested plural, `#` is the
// INNERMOST plural's argument (ICU semantics), not the enclosing one. The old
// code did a blanket ReplaceAll of `#` on the chosen branch before recursing, so
// an inner plural's `#` was clobbered with the outer value. A `#` inside a
// nested select must keep referring to the enclosing plural (select does not
// shadow `#`), so that case is asserted too.
func TestNestedPluralHash(t *testing.T) {
	// Inner # takes the inner argument (m=3), not the outer (n=5).
	got := msg("{n, plural, other {{m, plural, other {# inner}}}}", "en",
		map[string]any{"n": float64(5), "m": float64(3)})
	if got != "3 inner" {
		t.Errorf("nested plural #: got %q, want \"3 inner\"", got)
	}
	// The outer # (outside the nested block) still resolves to the outer value.
	got = msg("{n, plural, other {# and {m, plural, other {# inner}}}}", "en",
		map[string]any{"n": float64(5), "m": float64(3)})
	if got != "5 and 3 inner" {
		t.Errorf("outer and inner #: got %q, want \"5 and 3 inner\"", got)
	}
	// # inside a nested select keeps referring to the enclosing plural value.
	got = msg("{n, plural, other {{g, select, m {# he} f {# she}}}}", "en",
		map[string]any{"n": float64(5), "g": "m"})
	if got != "5 he" {
		t.Errorf("# in nested select: got %q, want \"5 he\"", got)
	}
}

func TestSelectNumericAndNested(t *testing.T) {
	// Numeric select keys compare via Stringify (2.0 -> "2").
	if got := msg("{n, select, 2 {two} other {many}}", "en", map[string]any{"n": float64(2)}); got != "two" {
		t.Errorf("numeric select: got %q", got)
	}
	// The chosen form is recursively expanded (nested {params}).
	got := msg("{g, select, m {Mr {name}} f {Ms {name}} other {{name}}}", "en",
		map[string]any{"g": "f", "name": "Ada"})
	if got != "Ms Ada" {
		t.Errorf("nested select expansion: got %q", got)
	}
	// Missing forms map: a select with no other and no match yields "".
	if got := msg("{g, select, a {x}}", "en", map[string]any{"g": "zzz"}); got != "" {
		t.Errorf("select with no matching form: got %q", got)
	}
}

func TestNumberLocaleSeparators(t *testing.T) {
	cases := []struct {
		locale, want string
		v            float64
	}{
		// French uses a non-breaking space (U+00A0) for grouping and "," decimal.
		{"fr", "1\u00a0234\u00a0567", 1234567},
		{"fr", "1\u00a0234,5", 1234.5},
		// A regional tag resolves via its base language.
		{"de-AT", "1.234,5", 1234.5},
		{"de_AT", "1.234,5", 1234.5},
		{"es-419", "1.234,5", 1234.5},
		// An unknown locale falls back to "," / ".".
		{"xx", "1,234.5", 1234.5},
		// Large grouping exercises multiple groups.
		{"en", "1,000,000,000", 1e9},
		// Negative percent.
		{"en", "-50%", -0.5},
		// Fractional percent.
		{"en", "12.5%", 0.125},
		// Small numbers need no grouping.
		{"en", "999", 999},
	}
	for _, c := range cases {
		tmpl := "{v, number}"
		if c.want[len(c.want)-1] == '%' {
			tmpl = "{v, number, percent}"
		}
		if got := msg(tmpl, c.locale, map[string]any{"v": c.v}); got != c.want {
			t.Errorf("number %v in %s: got %q, want %q", c.v, c.locale, got, c.want)
		}
	}
}

func TestCurrencyVariants(t *testing.T) {
	cur := func(v float64, locale, code string) string {
		return msg("{v, currency, "+code+"}", locale, map[string]any{"v": v})
	}
	// Known symbols, zero-decimal currencies, symbol-after locales.
	if got := cur(5, "en", "GBP"); got != "£5.00" {
		t.Errorf("GBP/en = %q", got)
	}
	if got := cur(1000, "ko", "KRW"); got != "₩1,000" {
		t.Errorf("KRW/ko (zero-decimal) = %q", got)
	}
	if got := cur(1234.5, "ru", "RUB"); got != "1\u00a0234,50 ₽" {
		t.Errorf("RUB/ru (symbol after, NBSP grouping) = %q", got)
	}
	// A code with no symbol table entry renders the ISO code as prefix (en).
	if got := cur(1234.5, "en", "abc"); got != "ABC 1,234.50" {
		t.Errorf("unknown code = %q", got)
	}
	// No code at all defaults to USD.
	if got := msg("{v, currency}", "en", map[string]any{"v": 9.5}); got != "$9.50" {
		t.Errorf("default code USD = %q", got)
	}
	// Negative amounts: the leading minus precedes the symbol (CLDR convention),
	// never landing between the symbol and the digits.
	if got := cur(-1234.5, "en", "USD"); got != "-$1,234.50" {
		t.Errorf("negative currency = %q", got)
	}
	// Symbol-after locales keep the sign at the very front too.
	if got := cur(-1234.5, "de", "EUR"); got != "-1.234,50 €" {
		t.Errorf("negative currency (symbol after) = %q", got)
	}
}

// TestCurrencyNegativeSignPlacement is a regression test: the minus sign must
// lead the whole formatted amount, so a symbol-before locale renders "-$1,234.50"
// (not "$-1,234.50") and an unknown code's ISO prefix stays behind the sign.
func TestCurrencyNegativeSignPlacement(t *testing.T) {
	cur := func(v float64, locale, code string) string {
		return msg("{v, currency, "+code+"}", locale, map[string]any{"v": v})
	}
	cases := []struct {
		v            float64
		locale, code string
		want         string
	}{
		{-1234.5, "en", "USD", "-$1,234.50"},    // symbol before: sign leads the symbol
		{-5, "en", "GBP", "-£5.00"},             // symbol before, small amount
		{-1000, "ko", "KRW", "-₩1,000"},         // symbol before, zero-decimal
		{-1234.5, "de", "EUR", "-1.234,50 €"},   // symbol after: sign leads the digits
		{-1234.5, "ru", "RUB", "-1 234,50 ₽"},   // symbol after, NBSP grouping
		{-1234.5, "en", "abc", "-ABC 1,234.50"}, // unknown code: sign before the ISO prefix
	}
	for _, c := range cases {
		if got := cur(c.v, c.locale, c.code); got != c.want {
			t.Errorf("cur(%v, %s, %s) = %q, want %q", c.v, c.locale, c.code, got, c.want)
		}
	}
}

func TestMessageSimpleParamVariants(t *testing.T) {
	// Dotted params resolve through nested maps.
	got := msg("Hi {user.name}", "en", map[string]any{"user": map[string]any{"name": "Ada"}})
	if got != "Hi Ada" {
		t.Errorf("dotted param: got %q", got)
	}
	// A dotted path through a non-map resolves to nothing (left visible).
	got = msg("Hi {user.name}", "en", map[string]any{"user": "scalar"})
	if got != "Hi {user.name}" {
		t.Errorf("dotted param through scalar: got %q", got)
	}
	// Whitespace inside the braces is tolerated.
	if got := msg("Hi { name }", "en", map[string]any{"name": "Ada"}); got != "Hi Ada" {
		t.Errorf("padded placeholder: got %q", got)
	}
	// A placeholder whose value is 0/false still renders (not treated as missing).
	if got := msg("n={n}", "en", map[string]any{"n": float64(0)}); got != "n=0" {
		t.Errorf("zero value: got %q", got)
	}
	if got := msg("b={b}", "en", map[string]any{"b": false}); got != "b=false" {
		t.Errorf("false value: got %q", got)
	}
}

func TestMessageBoundsAndMalformed(t *testing.T) {
	// An oversized catalog value is returned unexpanded rather than scanned.
	big := "{" + string(make([]byte, maxMessageLen+1)) + "}"
	if got := fillMessage(big, map[string]any{}); got != big {
		t.Errorf("oversized message should pass through unchanged (len %d)", len(got))
	}

	// Unbalanced braces must not hang or panic; the tail is consumed as inner.
	if got := fillMessage("{name", map[string]any{"name": "Ada"}); got != "Ada" {
		t.Errorf("unbalanced open brace: got %q", got)
	}
	// A lone opening brace with no content resolves as an empty placeholder.
	if got := fillMessage("{", map[string]any{}); got != "{}" {
		t.Errorf("lone brace: got %q", got)
	}
	// An empty placeholder is left visible.
	if got := fillMessage("{}", map[string]any{}); got != "{}" {
		t.Errorf("empty placeholder: got %q", got)
	}
}
