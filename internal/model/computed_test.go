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
		{"state.computed", false}, // step paths are state-rooted already
		{"total", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsComputedPath(tt.path); got != tt.want {
			t.Errorf("IsComputedPath(%q) = %v, want %v", tt.path, got, tt.want)
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

func TestComputedRefsInsideStringLiteralIsSafelyOverBroad(t *testing.T) {
	// The scanner does not parse, so a name inside a string literal counts as a
	// reference. That can only ADD an edge — a stricter order or a cycle
	// report — never drop one, which is what keeps the recursion guard sound.
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
