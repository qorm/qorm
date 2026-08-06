package expr

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// callBuiltin dispatches a function call. Unknown functions and out-of-range
// arguments yield nil/zero values rather than errors, so bindings never panic.
// env carries the guard-rail counters for builtins that evaluate string
// sub-expressions (map, filter, count).
func callBuiltin(name string, a []any, env *evalEnv) any {
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
	case "range":
		// range(n): the integers 0..n-1 as a list (games/scans). n clamps to
		// [0, 2^20] so a hostile or buggy size can never OOM the evaluation.
		n := int(num(arg(0)))
		if n < 0 {
			n = 0
		}
		if n > 1<<20 {
			n = 1 << 20
		}
		out := make([]any, n)
		for i := range out {
			out[i] = float64(i)
		}
		return out
	case "fill":
		// fill(n, v): n copies of v (board/state initialization).
		n := int(num(arg(0)))
		if n < 0 {
			n = 0
		}
		if n > 1<<20 {
			n = 1 << 20
		}
		out := make([]any, n)
		for i := range out {
			out[i] = arg(1)
		}
		return out
	case "concat":
		// concat(a, b, ...): lists joined in order; a bare (non-list) value
		// appends as one element, nils drop.
		out := []any{}
		for _, v := range a {
			switch t := v.(type) {
			case []any:
				out = append(out, t...)
			case nil:
			default:
				out = append(out, v)
			}
		}
		return out
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
	case "mod":
		// mod(a, b): the % operator as a function (LCG arithmetic in scripts).
		// Same truncation rules and zero-divisor guard as the operator.
		d := int64(num(arg(1)))
		if d == 0 {
			return 0.0
		}
		return float64(int64(num(arg(0))) % d)
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

	// ---- type checks (v2) ----
	case "typeof":
		// typeof(v): one of "number", "string", "boolean", "null", "array",
		// "object", "fn". Maps mirror qscript/js conventions so bindings can
		// dispatch on shape.
		return typeOfStr(arg(0))
	case "isArray", "isList":
		return isListArg(arg(0))
	case "isString":
		_, ok := arg(0).(string)
		return ok
	case "isNumber":
		_, ok := arg(0).(float64)
		return ok
	case "isBool":
		_, ok := arg(0).(bool)
		return ok
	case "isObject":
		_, ok := arg(0).(map[string]any)
		return ok
	case "isNull":
		return arg(0) == nil

	// ---- JSON (v2) ----
	case "jsonEncode", "JSON.stringify":
		// jsonEncode(v): any -> compact JSON string. NaN/Inf become null
		// (encoding/json marshalling) so output is always valid JSON.
		b, err := json.Marshal(normalizeForJSON(arg(0)))
		if err != nil {
			return "" // degrade silently — a binding failure is not fatal
		}
		return string(b)
	case "jsonDecode", "JSON.parse":
		// jsonDecode(s): JSON string -> any. Returns nil on parse error so
		// a malformed payload reads as an empty value, not a script error.
		s := Stringify(arg(0))
		if s == "" {
			return nil
		}
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil
		}
		return v

	// ---- array extras (v2) ----
	case "flatten":
		// flatten(list): one level deep. A nested list of lists becomes a
		// flat list; non-list elements pass through. Deeper nesting is left
		// alone (use flatten(flatten(x)) to recurse, like JS's flat()).
		arr, ok := arg(0).([]any)
		if !ok {
			return []any{}
		}
		out := make([]any, 0, len(arr))
		for _, v := range arr {
			if sub, ok := v.([]any); ok {
				out = append(out, sub...)
			} else {
				out = append(out, v)
			}
		}
		return out

	// ---- collection builtins ----
	// All degrade leniently: a non-list/non-map subject yields the zero result
	// (nil, 0, "", or an empty list) rather than an error.

	case "at":
		// at(list, i): element i; a negative i counts from the end (-1 = last).
		// Out-of-range (either direction) yields nil.
		return listAt(arg(0), arg(1))
	case "first":
		return listAt(arg(0), 0)
	case "last":
		return listAt(arg(0), -1)
	case "sum":
		// sum(list): numeric sum of the elements (each coerced via toNum).
		return sumList(arg(0))
	case "avg":
		// avg(list): arithmetic mean; an empty or non-list subject yields 0
		// (never NaN — renders must stay deterministic and printable).
		arr, ok := arg(0).([]any)
		if !ok || len(arr) == 0 {
			return float64(0)
		}
		return sumList(arg(0)) / float64(len(arr))
	case "count":
		// count(list) = len(list); count(list, "pred") counts elements whose
		// predicate (element bound as `it`) is truthy. A non-string predicate
		// degrades to the one-argument form; a non-list, non-map subject is 0.
		switch v := arg(0).(type) {
		case []any:
			pred, ok := arg(1).(string)
			if !ok {
				return float64(len(v))
			}
			n := 0
			for _, el := range v {
				if truthy(evalSub(pred, el, env)) {
					n++
				}
			}
			return float64(n)
		case map[string]any:
			return float64(len(v))
		}
		return float64(0)
	case "join":
		// join(list, sep): elements stringified and joined; non-list -> "".
		arr, ok := arg(0).([]any)
		if !ok {
			return ""
		}
		parts := make([]string, len(arr))
		for i, el := range arr {
			parts[i] = Stringify(el)
		}
		return strings.Join(parts, Stringify(arg(1)))
	case "split":
		// split(str, sep): list of substrings; an empty subject yields an
		// empty list (not [""]), and an empty sep splits into runes.
		s := Stringify(arg(0))
		if s == "" {
			return []any{}
		}
		fields := strings.Split(s, Stringify(arg(1)))
		out := make([]any, len(fields))
		for i, f := range fields {
			out[i] = f
		}
		return out
	case "push":
		// push(list, v, ...): a NEW list with the values appended (functional —
		// scripts assign the result back: state.items = push(state.items, x)).
		// A non-list subject becomes a one-element list; nils drop.
		arr, ok := arg(0).([]any)
		if !ok {
			arr = []any{}
		}
		out := append([]any{}, arr...)
		for _, v := range a[1:] {
			if v != nil {
				out = append(out, v)
			}
		}
		return out
	case "unshift":
		// unshift(list, v): a NEW list with v prepended. Non-list subject → [v].
		arr, ok := arg(0).([]any)
		if !ok {
			arr = []any{}
		}
		out := []any{}
		for _, v := range a[1:] {
			if v != nil {
				out = append(out, v)
			}
		}
		return append(out, arr...)
	case "pop":
		// pop(list): a NEW list without the last element (the popped value is
		// read with last() before/after). Empty/non-list → empty list.
		arr, _ := arg(0).([]any)
		if len(arr) <= 1 {
			return []any{}
		}
		return append([]any{}, arr[:len(arr)-1]...)
	case "shift":
		// shift(list): a NEW list without the first element. Empty/non-list → empty.
		arr, _ := arg(0).([]any)
		if len(arr) <= 1 {
			return []any{}
		}
		return append([]any{}, arr[1:]...)
	case "reverse":
		// reverse(list): a NEW list in reverse order. Non-list → empty list.
		arr, ok := arg(0).([]any)
		if !ok {
			return []any{}
		}
		out := make([]any, len(arr))
		for i, v := range arr {
			out[len(arr)-1-i] = v
		}
		return out
	case "sort":
		// sort(list): a NEW list sorted — numbers first ascending, then strings
		// lexically, booleans after (nil last). Stable within each class, so a
		// sort of mixed data never rearranges equal elements randomly.
		arr, ok := arg(0).([]any)
		if !ok {
			return []any{}
		}
		out := append([]any{}, arr...)
		sort.SliceStable(out, func(i, j int) bool { return lessForSort(out[i], out[j]) })
		return out
	case "indexOf":
		// indexOf(list, v): the first index of v (==-comparison), or -1.
		arr, ok := arg(0).([]any)
		if !ok {
			return float64(-1)
		}
		target := arg(1)
		for i, el := range arr {
			if valuesEqual(el, target) {
				return float64(i)
			}
		}
		return float64(-1)
	case "includes":
		// includes(list, v): whether v is an element (==-comparison). For
		// strings, includes(str, sub) is substring containment — mirroring
		// JS where Array#includes and String#includes share the name.
		if s, ok := arg(0).(string); ok {
			return strings.Contains(s, Stringify(arg(1)))
		}
		arr, ok := arg(0).([]any)
		if !ok {
			return false
		}
		target := arg(1)
		for _, el := range arr {
			if valuesEqual(el, target) {
				return true
			}
		}
		return false
	case "charAt":
		// charAt(s, i): the i-th rune of s as a string, "" out of range.
		s := Stringify(arg(0))
		i := int(num(arg(1)))
		if i < 0 {
			return ""
		}
		runes := []rune(s)
		if i >= len(runes) {
			return ""
		}
		return string(runes[i])
	case "substring":
		// substring(s, start[, end]): rune-clamped slice; negative/inverted
		// ranges collapse like slice().
		s := Stringify(arg(0))
		runes := []rune(s)
		lo, hi := 0, len(runes)
		if arg(1) != nil {
			lo = int(num(arg(1)))
		}
		if arg(2) != nil {
			hi = int(num(arg(2)))
		}
		if lo < 0 {
			lo = 0
		}
		if hi > len(runes) {
			hi = len(runes)
		}
		if lo > hi {
			lo = hi
		}
		return string(runes[lo:hi])
	case "repeat":
		// repeat(s, n): s repeated n times, "" for n<=0; n caps at 2^20 so a
		// hostile size cannot OOM.
		n := int(num(arg(1)))
		if n <= 0 {
			return ""
		}
		if n > 1<<20 {
			n = 1 << 20
		}
		return strings.Repeat(Stringify(arg(0)), n)
	case "padStart":
		// padStart(s, n[, ch]): left-pad to n RUNES with ch (default " ").
		return padRunes(Stringify(arg(0)), int(num(arg(1))), arg(2), false)
	case "padEnd":
		return padRunes(Stringify(arg(0)), int(num(arg(1))), arg(2), true)
	case "trimStart":
		return strings.TrimLeft(Stringify(arg(0)), " \t\n\r")
	case "trimEnd":
		return strings.TrimRight(Stringify(arg(0)), " \t\n\r")
	case "keys", "values":
		// keys(map) / values(map): keys sorted lexically, values in
		// sorted-key order — map iteration order is random in Go, and render
		// output must be deterministic, so the sort is a guarantee, not a
		// nicety. Non-map subjects yield an empty list.
		m, ok := arg(0).(map[string]any)
		if !ok {
			return []any{}
		}
		ks := sortedKeys(m)
		out := make([]any, len(ks))
		for i, k := range ks {
			if name == "keys" {
				out[i] = k
			} else {
				out[i] = m[k]
			}
		}
		return out
	case "map":
		// map(list, "expr"): evaluates the sub-expression (a string, element
		// bound as `it`) per element, e.g. map(state.items, "it.price").
		// Non-list or non-string arguments yield an empty list; a failing
		// sub-expression yields nil for that element.
		arr, aok := arg(0).([]any)
		sub, sok := arg(1).(string)
		if !aok || !sok {
			return []any{}
		}
		out := make([]any, len(arr))
		for i, el := range arr {
			out[i] = evalSub(sub, el, env)
		}
		return out
	case "filter":
		// filter(list, "expr"): keeps elements whose sub-expression (element
		// bound as `it`) is truthy, e.g. filter(state.items, "it.done").
		// Non-list or non-string arguments yield an empty list.
		arr, aok := arg(0).([]any)
		sub, sok := arg(1).(string)
		if !aok || !sok {
			return []any{}
		}
		out := []any{}
		for _, el := range arr {
			if truthy(evalSub(sub, el, env)) {
				out = append(out, el)
			}
		}
		return out
	case "format":
		// format(pattern, args...): minimal printf subset. Verbs: %s
		// (stringified), %d (truncated integer; NaN/out-of-int64-range -> 0),
		// %f (6 decimals), %.Nf (N decimals, N capped at 20), %% (literal %).
		// Any other %-sequence passes through literally; missing arguments
		// format as the zero value ("" / 0). No width/flags — this is a
		// placeholder formatter, not fmt.
		var rest []any
		if len(a) > 1 { // a zero-arg call has no a[1:] to slice
			rest = a[1:]
		}
		return formatPattern(Stringify(arg(0)), rest)
	case "now":
		// now(): wall-clock time in milliseconds since the Unix epoch. The
		// only entropy source scripts get — they have no clock/IO otherwise,
		// so the common pattern is to mod the value into the desired range
		// and use it to seed an LCG (e.g. state.rng = mod(now(), 2^31-1) + 1).
		// Goes through expr (not qscript) so bindings see the same value as
		// scripts — a {{ now() }} in a binding resolves to the same number.
		return float64(time.Now().UnixMilli())
	}
	return nil
}

// evalSub evaluates a map/filter/count sub-expression against one element.
// The element is bound as `it` and is the ONLY visible binding: the outer
// scope (state etc.) is deliberately hidden, so an expression string that
// leaks into data cannot re-summon outer values into fresh evaluations. The
// parse goes through the shared bounded AST cache, so the same predicate
// across a list (and across renders) parses once. Parse errors, nesting past
// maxSubExprDepth, and total work past maxSubExprEvals all degrade to nil.
func evalSub(src string, elem any, env *evalEnv) any {
	if env == nil { // direct callBuiltin use outside Eval (tests)
		env = &evalEnv{}
	}
	env.subEvals++
	env.subDepth++
	defer func() { env.subDepth-- }()
	if env.subDepth > maxSubExprDepth || env.subEvals > maxSubExprEvals {
		return nil
	}
	n, err := parse(src)
	if err != nil {
		return nil
	}
	return evalNode(n, map[string]any{"it": elem}, env)
}

// lessForSort orders two values for sort(): numbers first (ascending, NaN
// treated as smallest and stable), then strings (lexical), then booleans
// (false < true), nil last — a total order over the value set so sort()
// never panics on mixed data.
func lessForSort(a, b any) bool {
	an, aNum := a.(float64)
	bn, bNum := b.(float64)
	if aNum && bNum {
		if math.IsNaN(an) {
			return !math.IsNaN(bn)
		}
		if math.IsNaN(bn) {
			return false
		}
		return an < bn
	}
	as, aStr := a.(string)
	bs, bStr := b.(string)
	if aStr && bStr {
		return as < bs
	}
	ab, aBool := a.(bool)
	bb, bBool := b.(bool)
	if aBool && bBool {
		return !ab && bb
	}
	// Numeric beats string beats bool beats nil.
	rank := func(v any) int {
		switch v.(type) {
		case float64:
			return 0
		case string:
			return 1
		case bool:
			return 2
		default:
			return 3
		}
	}
	return rank(a) < rank(b)
}

// valuesEqual is the ==-comparison for indexOf/includes: numbers compare
// numerically, everything else by equality of concrete value. nil never
// equals anything (a nil lookup can't match, mirroring binding semantics).
func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	an, aNum := a.(float64)
	bn, bNum := b.(float64)
	if aNum && bNum {
		return an == bn
	}
	return a == b
}

// padRunes pads s with ch (default " ") to n runes, left or right.
func padRunes(s string, n int, ch any, right bool) string {
	if n <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) >= n {
		return s
	}
	padCh := " "
	if ch != nil {
		padCh = Stringify(ch)
		if padCh == "" {
			padCh = " "
		}
	}
	pad := strings.Repeat(padCh, n-len(runes))
	if right {
		return s + pad
	}
	return pad + s
}

// listAt implements at()/first()/last(): index into a list with
// negative-from-end wrapping. Non-lists, nil/NaN indexes, and out-of-range
// indexes (after wrapping) yield nil.
func listAt(list, idx any) any {
	arr, ok := list.([]any)
	if !ok || idx == nil {
		return nil
	}
	f := math.Trunc(toNum(idx))
	if f < 0 {
		f += float64(len(arr))
	}
	// Float-domain bounds check: rejects NaN (both comparisons false) and
	// out-of-range values before the int conversion can misbehave.
	if !(f >= 0 && f < float64(len(arr))) {
		return nil
	}
	return arr[int(f)]
}

// sumList sums a list's elements under toNum coercion; non-lists sum to 0.
func sumList(v any) float64 {
	arr, _ := v.([]any)
	var s float64
	for _, el := range arr {
		s += toNum(el)
	}
	return s
}

// sortedKeys returns m's keys sorted lexically (the determinism contract of
// keys()/values()).
func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// formatPattern implements the format() builtin's printf subset (%s, %d, %f,
// %.Nf, %%; anything else after '%' passes through literally). Iteration is
// byte-wise: the verbs are ASCII, and non-verb bytes are copied verbatim, so
// UTF-8 text in the pattern survives untouched.
func formatPattern(pat string, args []any) string {
	var sb strings.Builder
	argi := 0
	next := func() any {
		if argi < len(args) {
			v := args[argi]
			argi++
			return v
		}
		argi++
		return nil
	}
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		if c != '%' {
			sb.WriteByte(c)
			continue
		}
		if i+1 >= len(pat) { // trailing bare '%'
			sb.WriteByte('%')
			break
		}
		i++
		switch pat[i] {
		case '%':
			sb.WriteByte('%')
		case 's':
			sb.WriteString(Stringify(next()))
		case 'd':
			sb.WriteString(formatInt(toNum(next())))
		case 'f':
			sb.WriteString(strconv.FormatFloat(toNum(next()), 'f', 6, 64))
		case '.':
			// %.Nf — precision digits then 'f'. Anything else (%.x, %.f with
			// no digits) is not a verb and passes through literally.
			j := i + 1
			for j < len(pat) && pat[j] >= '0' && pat[j] <= '9' {
				j++
			}
			if j > i+1 && j < len(pat) && pat[j] == 'f' {
				n, _ := strconv.Atoi(pat[i+1 : j])
				if n > 20 {
					n = 20 // cap precision: %.999999f must not allocate wildly
				}
				sb.WriteString(strconv.FormatFloat(toNum(next()), 'f', n, 64))
				i = j
			} else {
				sb.WriteByte('%')
				sb.WriteByte('.')
			}
		default:
			sb.WriteByte('%')
			sb.WriteByte(pat[i])
		}
	}
	return sb.String()
}

// formatInt renders %d: the float truncated to an integer, with NaN and
// values outside int64's exact range degrading to "0" (int conversion of
// such floats is platform-defined, and rendered output must be deterministic).
func formatInt(f float64) string {
	if f != f || f < math.MinInt64 || f >= math.MaxInt64 {
		return "0"
	}
	return strconv.FormatInt(int64(f), 10)
}

// num coerces a value to float64 (re-uses toNum's rules).
func num(v any) float64 { return toNum(v) }

// CallBuiltin invokes a built-in function by name with already-evaluated
// arguments — the very dispatch bindings use — exposed so the script
// interpreter (internal/qscript) evaluates builtin calls with binding
// semantics exactly, rather than growing a second implementation that could
// drift. Callers get the same leniency bindings have: an unknown name or
// out-of-range arguments yield nil/zero values, never an error.
func CallBuiltin(name string, args []any) any {
	return callBuiltin(name, args, &evalEnv{})
}

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

// typeOfStr returns the type tag for v, mirroring qscript's convention:
// "number"/"string"/"boolean"/"null"/"array"/"object". A function value
// has its own "fn" tag (engine-level; bindings never see a function, but
// tests do).
func typeOfStr(v any) string {
	switch v.(type) {
	case float64:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "object"
}

// isListArg reports whether v is a list — the v2 array builtins (push,
// pop, etc.) all share this check.
func isListArg(v any) bool {
	_, ok := v.([]any)
	return ok
}

// normalizeForJSON rewrites qscript values into shapes encoding/json
// understands: numbers stay float64 (JSON numbers); strings and bools are
// already JSON-shaped; nil becomes JSON null. A map[string]any whose values
// are themselves qscript values recurses — the function returns the same
// tree with any non-JSON-compatible values zeroed.
func normalizeForJSON(v any) any {
	switch x := v.(type) {
	case nil, bool, string, float64, int:
		return v
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = normalizeForJSON(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = normalizeForJSON(vv)
		}
		return out
	}
	// Unknown kinds (e.g. fn values) become null.
	return nil
}
