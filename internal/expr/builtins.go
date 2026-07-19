package expr

import (
	"math"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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
		re := compileCached(Stringify(arg(1)))
		if re == nil {
			return false
		}
		return re.MatchString(Stringify(arg(0)))
	case "str":
		return Stringify(arg(0))
	case "slice":
		arr, _ := arg(0).([]any)
		if arr == nil {
			return []any{}
		}
		lo, hi := 0, len(arr)
		if arg(1) != nil {
			lo = int(num(arg(1)))
		}
		if arg(2) != nil {
			hi = int(num(arg(2)))
		}
		// bounds-clamped: a negative start reads as 0, an oversized end as len,
		// and an inverted range collapses to empty — bindings never panic.
		if lo < 0 {
			lo = 0
		}
		if hi > len(arr) {
			hi = len(arr)
		}
		if lo > hi {
			lo = hi
		}
		return arr[lo:hi]
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

// compileCached compiles a regex once per pattern (matches() is used inside
// bindings evaluated on every render). A previously-bad pattern caches as nil so
// it isn't recompiled; the cache is bounded.
var (
	reCache sync.Map // pattern -> *regexp.Regexp (typed-nil for a bad pattern)
	reCount atomic.Int64
)

func compileCached(pat string) *regexp.Regexp {
	if v, ok := reCache.Load(pat); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		re = nil
	}
	if reCount.Load() < 1024 {
		if _, loaded := reCache.LoadOrStore(pat, re); !loaded {
			reCount.Add(1)
		}
	}
	return re
}
