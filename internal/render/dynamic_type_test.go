package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// These tests cover dynamic type binding (polymorphic rendering): a node whose
// `type` is a {{binding}} resolves to its widget name at RENDER time, against
// the current scope. The motivating case is a heterogeneous list — chat bubble
// / image / system notice — which previously needed one `when`/`if` branch per
// kind and grew unreadable past two kinds.

// renderAppState renders an app with components, a scene root and initial state.
func renderAppState(t *testing.T, components map[string]*model.Node, root *model.Node, state map[string]any) Result {
	t.Helper()
	return Render(runtime.New(&model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		Components:  components,
		GlobalState: model.GlobalState{Initial: state},
	}))
}

// polyItems is a heterogeneous feed: four kinds, each of which must render as a
// different widget from ONE template node.
func polyItems() []any {
	return []any{
		map[string]any{"kind": "text", "body": "hello there"},
		map[string]any{"kind": "image", "body": ""},
		map[string]any{"kind": "link", "body": "open docs"},
		map[string]any{"kind": "badge", "body": "system"},
	}
}

// polyTemplate is the single renderItem node whose type is bound to item.kind.
// It carries the union of the four widgets' props; each resolved renderer reads
// only the ones it knows, exactly as it would with a static type.
func polyTemplate() *model.Node {
	return &model.Node{
		Type: "{{ item.kind }}", ID: "cell", Text: "{{ item.body }}",
		Label: "{{ item.body }}",
		Props: map[string]any{
			"type": "{{ item.kind }}", "id": "cell",
			"src": "photo.png", "href": "https://example.com/docs",
		},
	}
}

// TestDynamicTypePolymorphicList is the headline case: one template, four
// widgets, chosen per item by the item's own data.
func TestDynamicTypePolymorphicList(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "feed", Data: "{{ state.feed }}",
		Template: polyTemplate(),
	}, map[string]any{"feed": polyItems()})

	for _, want := range []string{
		">hello there<",                   // text
		`<img id="cell" src="photo.png"`,  // image
		`href="https://example.com/docs"`, // link
		">open docs</a>",                  // link body
		">system</span>",                  // badge label
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("polymorphic list missing %q:\n%s", want, res.HTML)
		}
	}
	if len(res.Unknown) != 0 {
		t.Errorf("every kind resolves to a real widget, got unknown %v:\n%s", res.Unknown, res.HTML)
	}
	if strings.Contains(res.HTML, "{{") {
		t.Errorf("an unresolved binding leaked into the output:\n%s", res.HTML)
	}
	// The shared template node must not be mutated by the first item's
	// resolution — proof: item 1 is still an <img> and not a second <div>.
	if got := strings.Count(res.HTML, "<img "); got != 1 {
		t.Errorf("want exactly one image row, got %d:\n%s", got, res.HTML)
	}
}

// TestDynamicTypeUnknownDegrades guards the failure mode: a binding that
// resolves to a widget name nobody implements degrades through the same
// data-qorm-unknown path as a static typo, and is reported in Result.Unknown.
func TestDynamicTypeUnknownDegrades(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "{{ state.kind }}", ID: "dyn", Text: "x",
		Props: map[string]any{"type": "{{ state.kind }}", "id": "dyn"},
	}, map[string]any{"kind": "colunm"})
	if !strings.Contains(res.HTML, `data-qorm-unknown="colunm"`) {
		t.Errorf("unknown resolved type must be tagged with the RESOLVED name:\n%s", res.HTML)
	}
	if len(res.Unknown) != 1 || res.Unknown[0] != "colunm" {
		t.Errorf("Result.Unknown should report the resolved type, got %v", res.Unknown)
	}
}

// TestDynamicTypeUnresolvableDegrades guards the empty case: a binding over
// missing data evaluates to "" — the node keeps the raw binding as its type, so
// the unknown report names the offending expression instead of nothing at all.
func TestDynamicTypeUnresolvableDegrades(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "{{ state.missing }}", ID: "dyn",
		Props: map[string]any{"type": "{{ state.missing }}", "id": "dyn"},
	}, nil)
	if len(res.Unknown) != 1 || !strings.Contains(res.Unknown[0], "state.missing") {
		t.Errorf("an unresolvable type must be reported as unknown, got %v", res.Unknown)
	}
	if !strings.Contains(res.HTML, "data-qorm-unknown=") {
		t.Errorf("an unresolvable type must still render the inert unknown container:\n%s", res.HTML)
	}
	// Degraded, not crashed or vanished: the container is there.
	if !strings.Contains(res.HTML, `id="dyn"`) {
		t.Errorf("the unknown fallback must keep the node id:\n%s", res.HTML)
	}
}

// TestDynamicTypeComponent guards that a binding resolving to an APP COMPONENT
// name instantiates the component (renderInner tries components before the
// built-in switch), props and all.
func TestDynamicTypeComponent(t *testing.T) {
	comps := map[string]*model.Node{
		"Bubble": {Type: "text", ID: "b", Text: "bubble:{{ prop.body }}"},
		"Notice": {Type: "text", ID: "n", Text: "notice:{{ prop.body }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{{
		Type: "list", ID: "feed", Data: "{{ state.feed }}",
		Template: &model.Node{
			Type:  "{{ item.kind }}",
			ID:    "cell",
			Props: map[string]any{"type": "{{ item.kind }}", "id": "cell", "body": "{{ item.body }}"},
		},
	}}}
	res := renderAppState(t, comps, root, map[string]any{"feed": []any{
		map[string]any{"kind": "Bubble", "body": "hi"},
		map[string]any{"kind": "Notice", "body": "joined"},
		map[string]any{"kind": "Bubble", "body": "bye"},
	}})
	for _, want := range []string{"bubble:hi", "notice:joined", "bubble:bye"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("dynamic type must instantiate the named component, missing %q:\n%s", want, res.HTML)
		}
	}
	if len(res.Unknown) != 0 {
		t.Errorf("component names are not unknown widgets, got %v", res.Unknown)
	}
}

// TestDynamicTypeStaticUnchanged is the backward-compatibility guard: a type
// with no binding is never evaluated, so a literal name (even a weird one)
// dispatches exactly as before.
func TestDynamicTypeStaticUnchanged(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "text", ID: "t", Text: "plain",
	}, map[string]any{"kind": "badge"})
	if !strings.Contains(res.HTML, ">plain<") || len(res.Unknown) != 0 {
		t.Errorf("static type must render unchanged: unknown=%v\n%s", res.Unknown, res.HTML)
	}
	// A static unknown type still reports itself verbatim.
	res = renderWidgetState(t, &model.Node{Type: "colunm", ID: "c"}, nil)
	if len(res.Unknown) != 1 || res.Unknown[0] != "colunm" {
		t.Errorf("static unknown type reporting changed: %v", res.Unknown)
	}
}

// TestDynamicTypeIsNotAnInjectionVector guards that a hostile resolved type is
// entity-encoded like any other attribute value: the binding buys no escape
// from the escaping every renderer applies.
func TestDynamicTypeIsNotAnInjectionVector(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "{{ state.kind }}", ID: `d"><script>alert(1)</script>`,
		Props: map[string]any{"type": "{{ state.kind }}"},
	}, map[string]any{"kind": `x" onmouseover="alert(1)`})
	if strings.Contains(res.HTML, `onmouseover="alert(1)"`) || strings.Contains(res.HTML, "<script>") {
		t.Errorf("dynamic type must not break out of the attribute:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "&#34;") {
		t.Errorf("the hostile quote should be entity-encoded:\n%s", res.HTML)
	}
}

// TestDynamicTypeRespectsVisibilityAndAnimation guards the dispatch order in
// node(): `if` still hides the node without evaluating its type, and the
// animation wrap keys off the RESOLVED type (so a resolved motion widget is not
// double-animated).
func TestDynamicTypeRespectsVisibilityAndAnimation(t *testing.T) {
	hidden := renderWidgetState(t, &model.Node{
		Type: "{{ state.kind }}", ID: "dyn", Text: "nope",
		Props: map[string]any{"type": "{{ state.kind }}", "if": "{{ false }}"},
	}, map[string]any{"kind": "text"})
	if strings.Contains(hidden.HTML, "nope") || len(hidden.Unknown) != 0 {
		t.Errorf("if:false must hide a dynamically typed node too:\n%s", hidden.HTML)
	}

	anim := renderWidgetState(t, &model.Node{
		Type: "{{ state.kind }}", ID: "dyn", Text: "fade me",
		Props: map[string]any{"type": "{{ state.kind }}", "animation": "fadeup"},
	}, map[string]any{"kind": "text"})
	if !strings.Contains(anim.HTML, "animation:") || !strings.Contains(anim.HTML, ">fade me<") {
		t.Errorf("a dynamically typed node must still get the animation wrap:\n%s", anim.HTML)
	}

	// motion consumes `animation` itself: resolving to it must skip the wrap
	// exactly as a static motion node does.
	dyn := renderWidgetState(t, &model.Node{
		Type: "{{ state.kind }}", ID: "dyn", Text: "m",
		Props: map[string]any{"type": "{{ state.kind }}", "animation": "fadeup"},
	}, map[string]any{"kind": "motion"})
	static := renderWidgetState(t, &model.Node{
		Type: "motion", ID: "dyn", Text: "m",
		Props: map[string]any{"type": "motion", "animation": "fadeup"},
	}, nil)
	if dyn.HTML != static.HTML {
		t.Errorf("resolved type must dispatch identically to the static one:\ndyn:    %s\nstatic: %s", dyn.HTML, static.HTML)
	}
}
