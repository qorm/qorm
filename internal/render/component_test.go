package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// renderApp renders an app whose entry scene root holds the given children, with
// the supplied component definitions. Used to exercise component instantiation.
func renderApp(t *testing.T, components map[string]*model.Node, root *model.Node) Result {
	t.Helper()
	app := &model.App{
		Entry:      "main",
		Scenes:     map[string]*model.Node{"main": root},
		Components: components,
	}
	return Render(runtime.New(app))
}

// TestComponentPropScope guards that an instance's props/text/label/value become
// {{prop.x}} inside the component template, evaluated per instance.
func TestComponentPropScope(t *testing.T) {
	comps := map[string]*model.Node{
		"Field": {Type: "text", ID: "f", Text: "{{ prop.label }}={{ prop.value }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Field", ID: "f1", Label: "Name", Value: "Al"},
		{Type: "Field", ID: "f2", Label: "Age", Value: "30"},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"Name=Al", "Age=30"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component prop scope did not resolve per instance, lacks %q:\n%s", w, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "{{ prop.label }}") {
		t.Errorf("unresolved binding leaked:\n%s", res.HTML)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("component instance should not be unknown: %v", res.Unknown)
	}
}

// TestComponentPropsMap guards that explicit props (n.Props) are exposed as
// {{prop.x}} alongside the text/label/value shorthand.
func TestComponentPropsMap(t *testing.T) {
	comps := map[string]*model.Node{
		"Badge2": {Type: "text", ID: "b", Text: "{{ prop.kind }}:{{ prop.text }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Badge2", ID: "b1", Text: "HELLO", Props: map[string]any{"kind": "info"}},
	}}
	res := renderApp(t, comps, root)
	if !strings.Contains(res.HTML, "info:HELLO") {
		t.Errorf("component props map + text not resolved:\n%s", res.HTML)
	}
}

// TestComponentSlot guards that an instance's children fill a {type:slot} in the
// component template, per instance.
func TestComponentSlot(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel": {Type: "card", ID: "panel", Children: []*model.Node{
			{Type: "text", ID: "title", Text: "{{ prop.title }}"},
			{Type: "slot"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel", ID: "p1", Props: map[string]any{"title": "First"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "WORLD"}}},
		{Type: "Panel", ID: "p2", Props: map[string]any{"title": "Second"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "AGAIN"}}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"First", "Second", "WORLD", "AGAIN"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component slot/title not resolved, lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestComponentIdUniqueness guards that ids inside a component template are
// suffixed per instance, so two uses of the same component never collide on
// document.getElementById.
func TestComponentIdUniqueness(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel": {Type: "card", ID: "panel", Children: []*model.Node{
			{Type: "text", ID: "title", Text: "{{ prop.title }}"},
			{Type: "slot"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel", ID: "p1", Props: map[string]any{"title": "A"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "x"}}},
		{Type: "Panel", ID: "p2", Props: map[string]any{"title": "B"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "y"}}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{`id="title_p1"`, `id="title_p2"`, `id="body_p1"`, `id="body_p2"`} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component ids should be unique per instance, lacks %q:\n%s", w, res.HTML)
		}
	}
	// the bare (unsuffixed) template id must not appear, or lookups would collide
	if strings.Contains(res.HTML, `id="title"`) || strings.Contains(res.HTML, `id="body"`) {
		t.Errorf("unsuffixed template id leaked:\n%s", res.HTML)
	}
}

// TestComponentRecursionGuard guards that a self-referencing component terminates
// (the depth cap turns the runaway instance into an unknown node) rather than
// recursing forever.
func TestComponentRecursionGuard(t *testing.T) {
	// "Loop" instantiates itself as its only child.
	comps := map[string]*model.Node{
		"Loop": {Type: "column", ID: "loop", Children: []*model.Node{{Type: "Loop", ID: "inner"}}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Loop", ID: "top"},
	}}
	res := renderApp(t, comps, root) // must return, not hang/panic
	if len(res.Unknown) == 0 {
		t.Errorf("self-referencing component should bottom out as unknown at the depth cap")
	}
}
