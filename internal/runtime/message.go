package runtime

import (
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/expr"
)

// fillMessage expands an ICU-MessageFormat-lite catalog string against ctx:
//
//	simple param:  "Hello, {name}"                         -> "Hello, Ada"
//	plural:        "{n, plural, one {# item} other {# items}}"
//	select:        "{g, select, male {he} female {she} other {they}}"
//
// Inside plural/select forms, `#` is the number and nested {params} expand too.
// Plural categories use English/default CLDR rules (one/other) plus exact =N
// matches; other locales' rules are not yet modelled.
// maxMessageLen bounds ICU message expansion. resolveChoice recurses via
// fillMessage on a substring of s, so capping length at entry also caps the
// recursion depth — a pathological deeply-nested {a,plural,…} catalog value
// can't hang the O(n) brace scan or overflow the stack.
const maxMessageLen = 256 << 10

func fillMessage(s string, ctx map[string]any) string {
	if len(s) > maxMessageLen {
		return s // pathological catalog value — leave it unexpanded rather than hang
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '{' {
			j, inner := matchBrace(s, i)
			b.WriteString(resolvePlaceholder(inner, ctx))
			i = j + 1
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// matchBrace returns the index of the '}' balancing the '{' at start, and the
// content between them.
func matchBrace(s string, start int) (int, string) {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, s[start+1 : i]
			}
		}
	}
	return len(s) - 1, s[start+1:]
}

func resolvePlaceholder(inner string, ctx map[string]any) string {
	comma := strings.IndexByte(inner, ',')
	if comma >= 0 {
		varName := strings.TrimSpace(inner[:comma])
		rest := strings.TrimSpace(inner[comma+1:])
		switch {
		case strings.HasPrefix(rest, "plural,"), strings.HasPrefix(rest, "select,"):
			return resolveChoice(varName, rest, ctx)
		case rest == "number" || strings.HasPrefix(rest, "number,"):
			return formatNumber(toNumber(lookupPath(varName, ctx)), localeOf(ctx), argOf(rest))
		case rest == "currency" || strings.HasPrefix(rest, "currency,"):
			code := argOf(rest)
			if code == "" {
				code = "USD"
			}
			return formatCurrency(toNumber(lookupPath(varName, ctx)), localeOf(ctx), code)
		case rest == "date" || strings.HasPrefix(rest, "date,"):
			return formatDate(lookupPath(varName, ctx), localeOf(ctx), argOf(rest))
		case rest == "time" || strings.HasPrefix(rest, "time,"):
			if tm, ok := parseDate(lookupPath(varName, ctx)); ok {
				return tm.Format("15:04")
			}
			return ""
		}
	}
	// simple {path} substitution
	name := strings.TrimSpace(inner)
	if v := lookupPath(name, ctx); v != nil {
		return expr.Stringify(v)
	}
	return "{" + inner + "}" // unresolved: leave visible
}

// numSeps maps a base language to its {group, decimal} separators.
var numSeps = map[string][2]string{
	"en": {",", "."}, "zh": {",", "."}, "ja": {",", "."}, "ko": {",", "."},
	"de": {".", ","}, "es": {".", ","}, "it": {".", ","}, "nl": {".", ","},
	"fr": {" ", ","}, "ru": {" ", ","}, "pl": {" ", ","},
	"ar": {",", "."}, "he": {",", "."}, "pt": {".", ","},
}

func localeOf(ctx map[string]any) string {
	if l, ok := ctx["__locale"].(string); ok {
		return l
	}
	return "en"
}

func separators(locale string) [2]string {
	base := locale
	if i := strings.IndexAny(locale, "-_"); i > 0 {
		base = locale[:i]
	}
	if s, ok := numSeps[strings.ToLower(base)]; ok {
		return s
	}
	return [2]string{",", "."}
}

// formatNumber formats n with locale grouping/decimal separators. style
// "percent" scales by 100 and appends "%".
func formatNumber(n float64, locale, style string) string {
	seps := separators(locale)
	suffix := ""
	if style == "percent" {
		n *= 100
		suffix = "%"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatFloat(n, 'f', -1, 64)
	intPart, frac := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart, frac = s[:dot], s[dot+1:]
	}
	out := groupDigits(intPart, seps[0])
	if frac != "" {
		out += seps[1] + frac
	}
	if neg {
		out = "-" + out
	}
	return out + suffix
}

// argOf returns the style/arg after the first comma in a placeholder spec.
func argOf(rest string) string {
	if c := strings.IndexByte(rest, ','); c >= 0 {
		return strings.TrimSpace(rest[c+1:])
	}
	return ""
}

var currencySymbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥",
	"KRW": "₩", "INR": "₹", "RUB": "₽", "BRL": "R$", "CHF": "CHF",
}

// symbolAfterLocales place the currency symbol after the amount.
var symbolAfterLocales = map[string]bool{
	"de": true, "fr": true, "es": true, "it": true, "pt": true,
	"nl": true, "pl": true, "ru": true, "fi": true, "sv": true,
}

func baseLocale(locale string) string {
	if i := strings.IndexAny(locale, "-_"); i > 0 {
		return strings.ToLower(locale[:i])
	}
	return strings.ToLower(locale)
}

// zeroDecimalCurrencies use no minor units (whole amounts only).
var zeroDecimalCurrencies = map[string]bool{"JPY": true, "KRW": true, "VND": true, "CLP": true, "ISK": true}

// formatNumberFixed formats with exactly `frac` decimal places, locale-aware.
func formatNumberFixed(n float64, locale string, frac int) string {
	seps := separators(locale)
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatFloat(n, 'f', frac, 64)
	intPart, fp := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart, fp = s[:dot], s[dot+1:]
	}
	out := groupDigits(intPart, seps[0])
	if fp != "" {
		out += seps[1] + fp
	}
	if neg {
		out = "-" + out
	}
	return out
}

// formatCurrency renders v with the currency's symbol, locale-aware. The sign
// leads the whole formatted amount (CLDR convention): a symbol-before locale
// renders "-$1,234.50" (minus before the symbol), a symbol-after locale renders
// "-1.234,50 €" (minus before the digits) — never "$-1,234.50".
func formatCurrency(v float64, locale, code string) string {
	sym := currencySymbols[strings.ToUpper(code)]
	if sym == "" {
		sym = strings.ToUpper(code) + " "
	}
	frac := 2
	if zeroDecimalCurrencies[strings.ToUpper(code)] {
		frac = 0
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	amount := formatNumberFixed(v, locale, frac)
	if symbolAfterLocales[baseLocale(locale)] {
		return sign + amount + " " + sym
	}
	return sign + sym + amount
}

// parseDate accepts an epoch number (seconds or millis) or an ISO/RFC3339 string.
func parseDate(v any) (time.Time, bool) {
	switch t := v.(type) {
	case float64:
		sec := int64(t)
		if t > 1e11 { // looks like milliseconds
			sec = int64(t) / 1000
		}
		return time.Unix(sec, 0).UTC(), true
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02", "2006/01/02"} {
			if tm, err := time.Parse(layout, t); err == nil {
				return tm, true
			}
		}
	}
	return time.Time{}, false
}

// dateShortLayouts maps a base locale to a Go reference layout for short dates.
var dateShortLayouts = map[string]string{
	"en": "1/2/2006", "de": "2.1.2006", "at": "2.1.2006",
	"fr": "2/1/2006", "es": "2/1/2006", "it": "2/1/2006", "pt": "2/1/2006",
	"nl": "2-1-2006", "ru": "02.01.2006", "pl": "2.01.2006",
	"zh": "2006/1/2", "ja": "2006/1/2", "ko": "2006. 1. 2.",
}

func formatDate(v any, locale, style string) string {
	tm, ok := parseDate(v)
	if !ok {
		return ""
	}
	switch style {
	case "short":
		if layout, ok := dateShortLayouts[baseLocale(locale)]; ok {
			return tm.Format(layout)
		}
		return tm.Format("2006-01-02")
	case "long":
		return formatLongDate(tm, locale)
	case "time":
		return tm.Format("15:04")
	default: // ISO
		return tm.Format("2006-01-02")
	}
}

// monthNames holds localized full month names (index 0 = January).
var monthNames = map[string][]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"de": {"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
	"fr": {"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
	"es": {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
	"it": {"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
	"pt": {"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"},
	"nl": {"januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"},
	"ru": {"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"},
}

func formatLongDate(tm time.Time, locale string) string {
	base := baseLocale(locale)
	y, m, d := tm.Year(), int(tm.Month()), tm.Day()
	switch base {
	case "zh", "ja":
		return strconv.Itoa(y) + "年" + strconv.Itoa(m) + "月" + strconv.Itoa(d) + "日"
	case "ko":
		return strconv.Itoa(y) + "년 " + strconv.Itoa(m) + "월 " + strconv.Itoa(d) + "일"
	}
	months, ok := monthNames[base]
	if !ok {
		months = monthNames["en"]
	}
	name := months[m-1]
	switch base {
	case "en":
		return name + " " + strconv.Itoa(d) + ", " + strconv.Itoa(y)
	case "de":
		return strconv.Itoa(d) + ". " + name + " " + strconv.Itoa(y)
	default: // fr/es/it/pt/nl/ru: "2 janvier 2026"
		return strconv.Itoa(d) + " " + name + " " + strconv.Itoa(y)
	}
}

func groupDigits(digits, group string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	first := n % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < n; i += 3 {
		b.WriteString(group)
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func resolveChoice(varName, rest string, ctx map[string]any) string {
	kind := "plural"
	if strings.HasPrefix(rest, "select,") {
		kind = "select"
	}
	forms := parseForms(rest[strings.IndexByte(rest, ',')+1:])
	val := lookupPath(varName, ctx)

	var chosen string
	if kind == "plural" {
		n := toNumber(val)
		if f, ok := forms["="+trimNum(n)]; ok {
			chosen = f
		} else if f, ok := forms[pluralCategory(n, localeOf(ctx))]; ok {
			chosen = f
		} else {
			chosen = forms["other"]
		}
		chosen = strings.ReplaceAll(chosen, "#", trimNum(n))
	} else {
		key := expr.Stringify(val)
		if f, ok := forms[key]; ok {
			chosen = f
		} else {
			chosen = forms["other"]
		}
	}
	return fillMessage(chosen, ctx) // expand nested params
}

// parseForms reads "cat {text} cat {text} ..." into a map.
func parseForms(s string) map[string]string {
	forms := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t') {
			i++
		}
		start := i
		for i < len(s) && s[i] != '{' && s[i] != ' ' {
			i++
		}
		key := s[start:i]
		for i < len(s) && s[i] != '{' {
			i++
		}
		if i >= len(s) {
			break
		}
		j, text := matchBrace(s, i)
		if key != "" {
			forms[key] = text
		}
		i = j + 1
	}
	return forms
}

// pluralCategory returns the CLDR plural category for n in the given locale.
// Rules cover the major families; unlisted locales use the English rule.
func pluralCategory(n float64, locale string) string {
	i := int64(n)
	mod10, mod100 := i%10, i%100
	switch baseLocale(locale) {
	case "zh", "ja", "ko", "th", "vi", "id", "ms", "lo", "km", "my":
		return "other" // no plural distinction
	case "fr", "pt", "hy", "ff", "kab":
		if n == 0 || n == 1 {
			return "one"
		}
		return "other"
	case "ru", "uk", "be", "sr", "hr", "bs":
		if mod10 == 1 && mod100 != 11 {
			return "one"
		}
		if mod10 >= 2 && mod10 <= 4 && !(mod100 >= 12 && mod100 <= 14) {
			return "few"
		}
		return "many"
	case "pl":
		if i == 1 {
			return "one"
		}
		if mod10 >= 2 && mod10 <= 4 && !(mod100 >= 12 && mod100 <= 14) {
			return "few"
		}
		return "many"
	case "cs", "sk":
		if i == 1 {
			return "one"
		}
		if i >= 2 && i <= 4 {
			return "few"
		}
		return "other"
	case "ar":
		switch {
		case i == 0:
			return "zero"
		case i == 1:
			return "one"
		case i == 2:
			return "two"
		case mod100 >= 3 && mod100 <= 10:
			return "few"
		case mod100 >= 11 && mod100 <= 99:
			return "many"
		default:
			return "other"
		}
	default: // en, de, es, it, nl, sv, ...
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

func lookupPath(name string, ctx map[string]any) any {
	parts := strings.Split(name, ".")
	var cur any = ctx[parts[0]]
	for _, p := range parts[1:] {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func toNumber(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func trimNum(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
