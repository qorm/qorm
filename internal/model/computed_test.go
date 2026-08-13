package model

// Tests for the DERIVED-VALUE dependency machinery: the reserved namespace
// predicate and the evaluation order the runtime and the loader both drive off.
//
// ComputedOrder is the one place that decides (a) which derived value may read
// which other one and (b) which declarations are unevaluatable because they
// close a cycle. Both answers are load-bearing: the runtime evaluates `order`
// front to back with no recursion at all, and reports `cyclic` as nothing —
// which is precisely what makes `a = b + 1, b = a + 1` a diagnosable mistake
// rather than a stack overflow.

import (
	"reflect"
	"testing"
)

func TestIsComputedPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"computed", true},
		{"computed.total", true},
		{"computed.a.b", true},
		{"  computed.total  ", true}, // trimmed: a padded path still targets it
		{"computedish", false},       // prefix without the dot boundary
		{"computedish.x", false},
		// The `state.`-rooted spelling counts too. A step path is already
		// relative to the state root, so `state.computed.x` is the binding
		// spelling copied into an action by mistake — and taken literally it
		// creates a top-level key named "state". Both spellings are refused.
		{"state.computed", true},
		{"state.computed.total", true},
		{"  state.computed.total  ", true},
		{"state.computedish", false}, // the dot boundary still decides
		{"state.total", false},       // an ordinary (if mis-rooted) write
		{"item.computed.x", false},   // somebody else's field named computed
		{"state", false},
		{"total", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsComputedPath(tt.path); got != tt.want {
			t.Errorf("IsComputedPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestComputedDynamicKeyRefs(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		// Dynamic keys AT the namespace-name position — exactly what hides a
		// real cycle from the dependency scan.
		{"{{ computed[state.k] }}", []string{"computed[state.k]"}},
		{"{{ computed [ state.k ] }}", []string{"computed [ state.k ]"}}, // verbatim spelling
		{"{{ computed[0] }}", []string{"computed[0]"}},
		{"{{ computed[] }}", nil}, // empty brackets: a parse error later, no key to cycle-check
		{"{{ state.computed[state.k] }}", []string{"state.computed[state.k]"}},
		{"{{ state['computed'][state.k] }}", []string{"state['computed'][state.k]"}},
		{"{{ computed[key] + computed['b'] }}", []string{"computed[key]"}},
		{"{{ computed[state.m[k]] }}", []string{"computed[state.m[k]]"}},
		{"{{ computed['a' + 'b'] }}", []string{"computed['a' + 'b']"}}, // concatenation is not a plain key
		{"{{ computed[a\\b] }}", []string{"computed[a\\b]"}},           // a backslash inside the brackets: still a dynamic key (the quoted-literal rule decides)
		{"{{ computed['abc ", []string{"computed['abc "}},              // unterminated quote: the bracket runs to the end, still reported as dynamic
		// Static keys and statically-rooted accesses are never reported.
		{"{{ computed }}", nil}, // the bare namespace names no value: the path just terminates
		{"computed", nil},       // ditto with nothing after the run
		{"{{ computed.total }}", nil},
		{"{{ computed['total'] }}", nil},
		{"{{ computed['a'] + 42 }}", nil}, // a number is not an access
		{"{{ computed['a\\'b'] }}", nil},  // an escaped quote inside the literal: still exactly one string literal, so `a'b` is a static key
		{"{{ computed['a']['b'] }}", nil},
		{"{{ state.computed.total }}", nil},
		{"{{ state['computed']['total'] }}", nil},
		{"{{ state['total'] }}", nil}, // a lone `state` run whose first postfix is not the namespace: plain state
		{"{{ (computed)['total'] }}", nil},
		{"{{ computed . total }}", nil},
		{"{{ computed . ['total'] }}", nil},    // the dot before a bracket is a parse error: nothing to report
		{"{{ computed.items[state.i] }}", nil}, // deeper bracket on a static value
		{"{{ state.total }}", nil},             // ordinary state
		{"{{ item.computed.key }}", nil},       // someone else's field named computed
		{"{{ computedTotal }}", nil},
		{"{{ 'read computed[state.k] first' }}", nil}, // decorative string is data
		{"{{ 'it\\'s not a binding' }}", nil},         // ...even when it contains an escaped quote
		{"{{ sum(computed.rows) }}", nil},
		{"no bindings", nil},
	}
	for _, tt := range tests {
		if got := ComputedDynamicKeyRefs(tt.src); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ComputedDynamicKeyRefs(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestComputedOrderIndependentValuesAreNameSorted(t *testing.T) {
	app := &App{Computed: map[string]string{
		"zeta":  "{{ state.a }}",
		"alpha": "{{ state.b }}",
		"mid":   "{{ state.c }}",
	}}
	order, cyclic := app.ComputedOrder()
	if want := []string{"alpha", "mid", "zeta"}; !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v (independent values must be name-sorted, never map order)", order, want)
	}
	if len(cyclic) != 0 {
		t.Errorf("cyclic = %v, want none", cyclic)
	}
	// Determinism: the same app must produce the same order every time.
	for i := 0; i < 20; i++ {
		if got, _ := app.ComputedOrder(); !reflect.DeepEqual(got, order) {
			t.Fatalf("ComputedOrder is not deterministic: %v vs %v", got, order)
		}
	}
}

func TestComputedOrderRespectsDependencies(t *testing.T) {
	// `total` reads two other derived values; `tax` reads one. Whatever the
	// name order, a value must never come before something it reads.
	app := &App{Computed: map[string]string{
		"subtotal": "{{ sum(map(state.items, \"it.price * it.qty\")) }}",
		"tax":      "{{ computed.subtotal * 0.2 }}",
		"total":    "{{ state.computed.subtotal + computed.tax }}",
	}}
	order, cyclic := app.ComputedOrder()
	if len(cyclic) != 0 {
		t.Fatalf("cyclic = %v, want none", cyclic)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if len(pos) != 3 {
		t.Fatalf("order = %v, want all three names", order)
	}
	if pos["subtotal"] > pos["tax"] || pos["tax"] > pos["total"] {
		t.Errorf("order = %v, want subtotal before tax before total (both spellings must create the edge)", order)
	}
}

func TestComputedOrderDetectsCycles(t *testing.T) {
	tests := []struct {
		name       string
		computed   map[string]string
		wantOrder  []string
		wantCyclic []string
	}{
		{
			name:       "self reference",
			computed:   map[string]string{"loop": "{{ computed.loop + 1 }}"},
			wantCyclic: []string{"loop"},
		},
		{
			name: "two-value cycle",
			computed: map[string]string{
				"a": "{{ computed.b + 1 }}",
				"b": "{{ computed.a + 1 }}",
			},
			wantCyclic: []string{"a", "b"},
		},
		{
			name: "downstream of a cycle is equally unevaluatable",
			computed: map[string]string{
				"a":    "{{ computed.b }}",
				"b":    "{{ computed.a }}",
				"safe": "{{ state.x }}",
				"down": "{{ computed.a + 1 }}",
			},
			wantOrder:  []string{"safe"},
			wantCyclic: []string{"a", "b", "down"},
		},
		{
			name: "a three-hop chain is not a cycle",
			computed: map[string]string{
				"a": "{{ state.x }}",
				"b": "{{ computed.a }}",
				"c": "{{ computed.b }}",
			},
			wantOrder: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, cyclic := (&App{Computed: tt.computed}).ComputedOrder()
			if !reflect.DeepEqual(order, tt.wantOrder) {
				t.Errorf("order = %v, want %v", order, tt.wantOrder)
			}
			if !reflect.DeepEqual(cyclic, tt.wantCyclic) {
				t.Errorf("cyclic = %v, want %v", cyclic, tt.wantCyclic)
			}
		})
	}
}

func TestComputedOrderEmpty(t *testing.T) {
	var nilApp *App
	if order, cyclic := nilApp.ComputedOrder(); order != nil || cyclic != nil {
		t.Errorf("nil app: got %v / %v, want nil / nil", order, cyclic)
	}
	if order, cyclic := (&App{}).ComputedOrder(); order != nil || cyclic != nil {
		t.Errorf("no declarations: got %v / %v, want nil / nil", order, cyclic)
	}
}

func TestComputedRefs(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"{{ computed.total }}", []string{"total"}},
		{"{{ state.computed.total }}", []string{"total"}},
		{"{{ computed.a + computed.b }}", []string{"a", "b"}},
		{"{{ state.total }}", nil},         // an ordinary state key, not derived
		{"{{ item.computed.total }}", nil}, // someone else's field named computed
		{"{{ computed }}", nil},            // the namespace itself names no value
		{"{{ computed. }}", nil},           // nothing after the dot
		{"{{ computedTotal }}", nil},       // no dot boundary
		{"{{ sum(computed.rows) }}", []string{"rows"}},
		{"no bindings at all", nil},
	}
	for _, tt := range tests {
		if got := computedRefs(tt.src); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("computedRefs(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestComputedRefsSeesBracketSpelling(t *testing.T) {
	// Every accessor spelling the expr grammar accepts must create the same
	// dependency edge — a cycle written in any of them is a real cycle.
	tests := []struct {
		src  string
		want []string
	}{
		{"{{ computed['total'] }}", []string{"total"}},
		{"{{ computed[\"total\"] }}", []string{"total"}},
		{"{{ state.computed['total'] }}", []string{"total"}},
		{"{{ state['computed']['total'] }}", []string{"total"}},
		{"{{ state['computed'].total }}", []string{"total"}},
		{"{{ computed ['total'] }}", []string{"total"}}, // whitespace before the bracket
		{"{{ computed['a'].b }}", []string{"a"}},        // mixed: dot access resumes after a bracket
		{"{{ computed['a']['b'] }}", []string{"a"}},     // chained brackets still name the first key
		{"{{ computed['a'] + computed.b }}", []string{"a", "b"}},
		{"{{ computed . total }}", []string{"total"}},               // whitespace around the dot is still member access
		{"{{ computed . ['total'] }}", nil},                         // a dot before a bracket is a parse error, no static name
		{"{{ (computed)['total'] }}", []string{"total"}},            // parens do not change the root
		{"{{ map(state.rows, \"computed['b']\") }}", []string{"b"}}, // bracket spelling inside a predicate string
		{"{{ rows['computed.x'] }}", nil},                           // a string index key is app data, not a reference
		{"{{ computed[key] }}", nil},                                // a dynamic key names nothing statically
		{"{{ item['computed'].x }}", nil},                           // someone else's bracketed field named computed
		{"{{ computed['a\\'b'] }}", []string{"a'b"}},                // an escaped quote in a static key is still a static key
	}
	for _, tt := range tests {
		if got := computedRefs(tt.src); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("computedRefs(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}

	// The point of the edges: a cycle written in bracket spelling must be
	// REPORTED as cyclic, not silently ordered (and published as garbage).
	app := &App{Computed: map[string]string{
		"a": "{{ computed['b'] + 1 }}",
		"b": "{{ computed['a'] + 1 }}",
	}}
	order, cyclic := app.ComputedOrder()
	if len(order) != 0 {
		t.Errorf("order = %v, want none (the whole graph is one cycle)", order)
	}
	if !reflect.DeepEqual(cyclic, []string{"a", "b"}) {
		t.Errorf("cyclic = %v, want [a b] — a bracket-spelled cycle must not pass silently", cyclic)
	}
}

func TestComputedRefsInsideStringLiteralIsSafelyOverBroad(t *testing.T) {
	// A string literal is scanned for references because a map/filter/count
	// predicate is re-parsed by evalSub at runtime: `computed.b` inside one is
	// a genuine read of b. Scanning every string keeps that edge; where the
	// scan is broader than the runtime it can only ADD an edge — a stricter
	// order or a cycle report — never drop one, which is what keeps the
	// recursion guard sound.
	app := &App{Computed: map[string]string{
		"a": "{{ map(state.rows, \"computed.b\") }}",
		"b": "{{ state.x }}",
	}}
	order, cyclic := app.ComputedOrder()
	if len(cyclic) != 0 {
		t.Fatalf("cyclic = %v, want none", cyclic)
	}
	if !reflect.DeepEqual(order, []string{"b", "a"}) {
		t.Errorf("order = %v, want b before a (the literal created the edge)", order)
	}
}
