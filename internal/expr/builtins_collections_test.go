package expr

// Tests for the collection/format builtins: at, first, last, sum, avg, count,
// join, split, keys, values, map, filter, format. All degrade leniently (nil /
// 0 / "" / empty list) on nil or type-mismatched input, keys/values are
// sorted for render determinism, and the map/filter/count sub-expression
// machinery is probed for isolation, cache reuse, and its recursion guards.

import (
	"reflect"
	"strings"
	"testing"
)

func collCtx() map[string]any {
	return map[string]any{
		"state": map[string]any{
			"nums":  []any{1.0, 2.0, 3.0, 4.0},
			"mixed": []any{1.0, "2", true, nil},
			"names": []any{"ada", "bo", "c"},
			"items": []any{
				map[string]any{"name": "pen", "price": 2.5, "qty": 2.0, "done": true},
				map[string]any{"name": "ink", "price": 10.0, "qty": 1.0, "done": false},
			},
			"empty": []any{},
			"user":  map[string]any{"b": 2.0, "a": 1.0, "c": 3.0},
			"x":     99.0,
		},
	}
}

func TestCollectionBuiltins(t *testing.T) {
	ctx := collCtx()
	cases := []struct {
		src  string
		want any
	}{
		// at: index with negative-from-end wrap
		{`at(state.nums, 0)`, 1.0},
		{`at(state.nums, 3)`, 4.0},
		{`at(state.nums, -1)`, 4.0},
		{`at(state.nums, -4)`, 1.0},
		{`at(state.nums, -5)`, nil},   // wraps past the front
		{`at(state.nums, 4)`, nil},    // out of range
		{`at(state.nums, 1.9)`, 2.0},  /* truncated */
		{`at(state.nums, 0/0)`, nil},  // NaN
		{`at(state.nums, null)`, nil}, // nil index
		{`at(state.x, 0)`, nil},       // non-list
		{`at(null, 0)`, nil},
		// first / last
		{`first(state.nums)`, 1.0},
		{`last(state.nums)`, 4.0},
		{`first(state.empty)`, nil},
		{`last(state.empty)`, nil},
		{`first(state.x)`, nil},
		{`last(null)`, nil},
		// sum / avg (toNum coercion per element)
		{`sum(state.nums)`, 10.0},
		{`sum(state.mixed)`, 4.0}, // 1 + 2 + 1 + 0
		{`sum(state.empty)`, 0.0},
		{`sum(state.x)`, 0.0}, // non-list
		{`sum(null)`, 0.0},
		{`avg(state.nums)`, 2.5},
		{`avg(state.empty)`, 0.0}, // empty: 0, never NaN
		{`avg(state.x)`, 0.0},     // non-list
		// count: one arg = len; two args = predicate count
		{`count(state.nums)`, 4.0},
		{`count(state.user)`, 3.0}, // map: len
		{`count(state.x)`, 0.0},    // non-collection
		{`count(null)`, 0.0},
		{`count(state.items, "it.done")`, 1.0},
		{`count(state.nums, "it > 2")`, 2.0},
		{`count(state.nums, "bogus +")`, 0.0}, // broken predicate: nothing matches
		{`count(state.nums, 5)`, 4.0},         // non-string predicate degrades to len
		// join / split
		{`join(state.names, ", ")`, "ada, bo, c"},
		{`join(state.mixed, "-")`, "1-2-true-"},
		{`join(state.empty, ",")`, ""},
		{`join(state.x, ",")`, ""}, // non-list
		{`join(state.names, 0)`, "ada0bo0c"},
		{`split("a,b,c", ",")`, []any{"a", "b", "c"}},
		{`split("abc", "")`, []any{"a", "b", "c"}}, // empty sep: runes
		{`split("", ",")`, []any{}},                // empty subject: empty list
		{`split(null, ",")`, []any{}},
		{`split(123, "2")`, []any{"1", "3"}}, // subject stringified
		// keys / values: sorted (determinism)
		{`keys(state.user)`, []any{"a", "b", "c"}},
		{`values(state.user)`, []any{1.0, 2.0, 3.0}},
		{`keys(state.nums)`, []any{}}, // non-map
		{`values(state.x)`, []any{}},  // non-map
		{`keys(null)`, []any{}},
		// map: sub-expression per element, bound as `it`
		{`map(state.nums, "it * 2")`, []any{2.0, 4.0, 6.0, 8.0}},
		{`map(state.items, "it.name")`, []any{"pen", "ink"}},
		{`map(state.items, "it.price * it.qty")`, []any{5.0, 10.0}},
		{`map(state.empty, "it")`, []any{}},
		{`map(state.x, "it")`, []any{}},                        // non-list
		{`map(state.nums, 7)`, []any{}},                        // non-string sub-expression
		{`map(state.nums, "it +")`, []any{nil, nil, nil, nil}}, // broken sub-expr: nil per element
		// filter: keeps truthy-predicate elements
		{`filter(state.nums, "it > 2")`, []any{3.0, 4.0}},
		{`filter(state.names, "len(it) > 1")`, []any{"ada", "bo"}},
		{`filter(state.empty, "it")`, []any{}},
		{`filter(state.x, "it")`, []any{}},    // non-list
		{`filter(state.nums, null)`, []any{}}, // non-string sub-expression
		{`filter(state.nums, "it +")`, []any{}},
		// composition: the shopping-cart total
		{`sum(map(state.items, "it.price * it.qty"))`, 15.0},
		{`join(map(state.items, "it.name"), "/")`, "pen/ink"},
		{`first(filter(state.items, "!it.done")).name`, "ink"},
		{`at(keys(state.user), -1)`, "c"},
		// isolation: the sub-expression sees only `it`, never the outer scope
		{`map(state.nums, "state.x")`, []any{nil, nil, nil, nil}},
		{`map(state.nums, "it")`, []any{1.0, 2.0, 3.0, 4.0}},
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Eval(%q) = %#v (%T), want %#v", c.src, got, got, c.want)
		}
	}
}

// TestKeysValuesDeterministic evaluates keys()/values() repeatedly over a
// many-key map and asserts an identical (sorted) result every time — map
// iteration order is random in Go, and the render-determinism guard depends
// on these being stable.
func TestKeysValuesDeterministic(t *testing.T) {
	m := map[string]any{}
	for _, k := range []string{"zeta", "alpha", "mu", "beta", "omega", "kappa", "iota", "nu"} {
		m[k] = "v-" + k
	}
	ctx := map[string]any{"m": m}

	wantKeys, _ := Eval(`join(keys(m), ",")`, ctx)
	if wantKeys != "alpha,beta,iota,kappa,mu,nu,omega,zeta" {
		t.Fatalf("keys(m) joined = %q, want sorted order", wantKeys)
	}
	wantVals, _ := Eval(`join(values(m), ",")`, ctx)
	for i := 0; i < 50; i++ {
		gotK, _ := Eval(`join(keys(m), ",")`, ctx)
		gotV, _ := Eval(`join(values(m), ",")`, ctx)
		if gotK != wantKeys || gotV != wantVals {
			t.Fatalf("iteration %d: keys/values order changed: %q / %q", i, gotK, gotV)
		}
	}
}

// TestSubExprUsesASTCache proves map/filter sub-expressions parse through the
// shared bounded AST cache: a sentinel AST stored under a fake source string
// is what evaluation returns, so the cache must have been consulted.
func TestSubExprUsesASTCache(t *testing.T) {
	const sentinel = "__subexpr_cache_sentinel__"
	astCache.Store(sentinel, parsed{node: strLit{"cached"}, err: nil})
	defer astCache.Delete(sentinel)

	ctx := map[string]any{"arr": []any{1.0, 2.0}}
	got, err := Eval(`map(arr, '`+sentinel+`')`, ctx)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !reflect.DeepEqual(got, []any{"cached", "cached"}) {
		t.Fatalf("map with sentinel source = %#v, want the cached AST's value", got)
	}
}

// TestSubExprDepthGuard builds cyclic data (a list that contains itself) plus
// a self-referential predicate drawn from the data — the one shape that could
// recurse without bound — and asserts evaluation terminates leniently.
func TestSubExprDepthGuard(t *testing.T) {
	pred := `count(it, it[1])`
	cyc := []any{nil, pred}
	cyc[0] = cyc
	ctx := map[string]any{"c": cyc}

	v, err := Eval(`count(c, "count(it, it[1])")`, ctx)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if _, ok := v.(float64); !ok {
		t.Fatalf("got %#v (%T), want a float64 count", v, v)
	}
}

// TestSubExprWorkGuard is the branching version of the recursion bomb: two
// self-references double the work per level, so without the total-evaluation
// cap this would take ~2^depth predicate evaluations. The test passing at all
// (within the suite timeout) is the assertion; the value check pins the
// degrade-to-nil behavior.
func TestSubExprWorkGuard(t *testing.T) {
	pred := `count(it, it[2])`
	cyc := []any{nil, nil, pred}
	cyc[0] = cyc
	cyc[1] = cyc
	ctx := map[string]any{"c": cyc}

	v, err := Eval(`count(c, "count(it, it[2])")`, ctx)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if _, ok := v.(float64); !ok {
		t.Fatalf("got %#v (%T), want a float64 count", v, v)
	}
}

// TestEvalSubDirect drives evalSub's nil-env branch (reachable only by direct
// callBuiltin use outside Eval) and its parse-error degrade.
func TestEvalSubDirect(t *testing.T) {
	if got := callBuiltin("map", []any{[]any{1.0}, "it + 1"}, nil); !reflect.DeepEqual(got, []any{2.0}) {
		t.Errorf("callBuiltin(map, nil env) = %#v, want [2]", got)
	}
	if got := evalSub("it +", 1.0, nil); got != nil {
		t.Errorf("evalSub(broken) = %#v, want nil", got)
	}
}

// TestFormat covers the format() printf subset verb by verb, including the
// degrade paths (missing args, NaN/huge %d, precision cap, unknown verbs).
func TestFormat(t *testing.T) {
	ctx := map[string]any{"state": map[string]any{"n": 3.0, "s": "ok", "f": 2.5}}
	cases := []struct {
		src  string
		want any
	}{
		{`format("%s", state.s)`, "ok"},
		{`format("%s items", 7)`, "7 items"},
		{`format("%d", 3.9)`, "3"}, // truncates
		{`format("%d", -3.9)`, "-3"},
		{`format("%d", "7.5")`, "7"}, // toNum coercion
		{`format("%f", 2.5)`, "2.500000"},
		{`format("%.2f", 2.5)`, "2.50"},
		{`format("%.0f", 2.5)`, "2"},
		{`format("%.2f + %.1f", 1, 2)`, "1.00 + 2.0"},
		{`format("100%%")`, "100%"},
		{`format("%x", 5)`, "%x"},   // unknown verb passes through
		{`format("50%")`, "50%"},    // trailing bare %
		{`format("%.f", 1)`, "%.f"}, // no precision digits: literal
		{`format("%.2x", 1)`, "%.2x"},
		{`format("%s %d %.1f", "a", 2, 3.25)`, "a 2 3.2"},
		{`format("%s and %s", "one")`, "one and "},        // missing arg -> ""
		{`format("%d", 0/0)`, "0"},                        // NaN -> 0
		{`format("%d", 99999999999999999999999999)`, "0"}, // out of int64 range -> 0
		{`format("%d", -99999999999999999999999999)`, "0"},
		{`format()`, ""},                    // no args at all (fuzz-found: must not panic)
		{`format(null)`, ""},                // nil pattern
		{`format(42)`, "42"},                // non-string pattern stringifies
		{`format("café %s", 1)`, "café 1"},  // UTF-8 survives
		{`format("%s", "x", "extra")`, "x"}, // extra args ignored
	}
	for _, c := range cases {
		got, err := Eval(c.src, ctx)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %q, want %q", c.src, got, c.want)
		}
	}

	// Precision is capped at 20 so a hostile %.999999f cannot allocate wildly.
	v, err := Eval(`format("%.999999f", 1.5)`, nil)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	s, _ := v.(string)
	if !strings.HasPrefix(s, "1.5") || len(s) != 2+20 {
		t.Errorf("capped precision: got %q (len %d), want 1.5 padded to 20 decimals", s, len(s))
	}
}
