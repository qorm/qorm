package expr

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// callBuiltin dispatches a function call. Unknown functions and out-of-range
// arguments yield nil/zero values rather than errors, so bindings never panic.
func callBuiltin(name string, a []any) any {
	arg := func(i int) any {
		if i < len(a) {
			return a[i]
		}
		return nil
	}
	switch name {
	case "len":
		switch v := arg(0).(type) {
		case string:
			return float64(utf8.RuneCountInString(v))
		case []any:
			return float64(len(v))
		case map[string]any:
			return float64(len(v))
		case nil:
			return float64(0)
		default:
			return float64(utf8.RuneCountInString(Stringify(v)))
		}
	case "trim":
		return strings.TrimSpace(Stringify(arg(0)))
	case "upper":
		return strings.ToUpper(Stringify(arg(0)))
	case "lower":
		return strings.ToLower(Stringify(arg(0)))
	case "contains":
		return strings.Contains(Stringify(arg(0)), Stringify(arg(1)))
	case "startsWith":
		return strings.HasPrefix(Stringify(arg(0)), Stringify(arg(1)))
	case "endsWith":
		return strings.HasSuffix(Stringify(arg(0)), Stringify(arg(1)))
	case "replace":
		return strings.ReplaceAll(Stringify(arg(0)), Stringify(arg(1)), Stringify(arg(2)))
	case "matches":
		re, err := regexp.Compile(Stringify(arg(1)))
		if err != nil {
			return false
		}
		return re.MatchString(Stringify(arg(0)))
	case "str":
		return Stringify(arg(0))
	case "number", "num":
		return num(arg(0))
	case "int":
		return math.Trunc(num(arg(0)))
	case "abs":
		return math.Abs(num(arg(0)))
	case "round":
		return math.Round(num(arg(0)))
	case "floor":
		return math.Floor(num(arg(0)))
	case "ceil":
		return math.Ceil(num(arg(0)))
	case "min":
		return reduceNums(a, math.Min)
	case "max":
		return reduceNums(a, math.Max)
	case "not":
		return !truthy(arg(0))
	case "empty":
		return !truthy(arg(0))
	case "default", "coalesce":
		if truthy(arg(0)) {
			return arg(0)
		}
		return arg(1)
	}
	return nil
}

// num coerces a value to float64 (re-uses toNum's rules).
func num(v any) float64 { return toNum(v) }

func reduceNums(a []any, f func(x, y float64) float64) any {
	if len(a) == 0 {
		return float64(0)
	}
	acc := toNum(a[0])
	for _, v := range a[1:] {
		acc = f(acc, toNum(v))
	}
	return acc
}
