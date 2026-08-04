package canvas

import (
	"fmt"
	"image"
	"sort"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// listFixture builds a headless engine around one list bound to state.items,
// with the given renderItem template (and optional list props, e.g. "as").
func listFixture(t *testing.T, items any, template *model.Node, props map[string]any) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	list := &model.Node{Type: "list", ID: "l1", Data: "{{state.items}}", Template: template, Props: props}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{list}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.State["items"] = items
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, list
}

// collectTexts gathers every text shape's content in draw (declaration)
// order.
func collectTexts(n graph.Node, out *[]string) {
	if t, ok := n.(*graph.Text); ok {
		*out = append(*out, t.Content)
	}
	if g, ok := n.(*graph.Group); ok {
		for _, c := range g.Children {
			collectTexts(c, out)
		}
	}
}

func renderedTexts(t *testing.T, e *Engine) []string {
	t.Helper()
	var out []string
	collectTexts(e.graphRoot, &out)
	return out
}

// instanceGroups returns the graph groups built from a repeat template's
// model node, ordered by their vertical position (document order).
func instanceGroups(t *testing.T, e *Engine, template *model.Node) []graph.Node {
	t.Helper()
	var out []graph.Node
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		if n.Base().Model == template {
			out = append(out, n)
		}
		if g, ok := n.(*graph.Group); ok {
			for _, c := range g.Children {
				walk(c)
			}
		}
	}
	walk(e.graphRoot)
	sort.Slice(out, func(i, j int) bool { return out[i].GetBBox().MinY < out[j].GetBBox().MinY })
	return out
}

func names(xs ...string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = map[string]any{"name": x}
	}
	return out
}

// Three items render three template instances at column geometry, each
// evaluated under its own item scope (item/index/first/last).
func TestListRendersItemsWithItemScope(t *testing.T) {
	template := &model.Node{Type: "column", ID: "row", Children: []*model.Node{
		{Type: "text", Props: map[string]any{"text": "{{index}}:{{item.name}}"}},
		{Type: "text", Props: map[string]any{"text": "first", "if": "{{first}}"}},
		{Type: "text", Props: map[string]any{"text": "last", "if": "{{last}}"}},
	}}
	e, surf, _ := listFixture(t, names("a", "b", "c"), template, nil)
	e.DrawFrame(surf)

	want := []string{"0:a", "first", "1:b", "2:c", "last"}
	if got := renderedTexts(t, e); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rendered texts = %v, want %v", got, want)
	}

	// Geometry: three instances stack as a column from y 0, each directly
	// below the previous one; the conditional rows make items 0 and 2 exactly
	// twice as tall as the single-row item 1 (runtime.New installs the default
	// theme, so absolute row heights are the theme's business, not this
	// test's).
	groups := instanceGroups(t, e, template)
	if len(groups) != 3 {
		t.Fatalf("instances = %d, want 3", len(groups))
	}
	height := func(i int) float64 {
		bb := groups[i].GetBBox()
		return bb.MaxY - bb.MinY
	}
	if y := groups[0].GetBBox().MinY; y != 0 {
		t.Errorf("instance 0 y = %v, want 0", y)
	}
	if y := groups[1].GetBBox().MinY; y != height(0) {
		t.Errorf("instance 1 y = %v, want %v (stacked below instance 0)", y, height(0))
	}
	if y := groups[2].GetBBox().MinY; y != height(0)+height(1) {
		t.Errorf("instance 2 y = %v, want %v", y, height(0)+height(1))
	}
	if height(0) != 2*height(1) || height(2) != height(0) {
		t.Errorf("heights = %v/%v/%v, want item 0 == item 2 == 2×item 1 (the conditional rows)", height(0), height(1), height(2))
	}
}

// A focusable inside a list's renderItem template joins the Tab order (the
// template is walked, not just Children), and Tab lands on the FIRST instance
// whose graph node carries the focus flag. Cycling through instances is a
// later milestone.
func TestListItemsKeyboardFocusable(t *testing.T) {
	tpl := &model.Node{Type: "button", ID: "item", Props: map[string]any{"label": "x"}}
	e, surf, _ := listFixture(t, names("a", "b", "c"), tpl, nil)
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != tpl {
		t.Fatal("tab must reach the list item template's button")
	}
	if e.Inter.FocusedItem != 0 {
		t.Fatalf("focus must land on the first instance, got item %d", e.Inter.FocusedItem)
	}
	e.DrawFrame(surf) // re-layout stamps the focused instance's graph node
	g := e.findGroupByModelIndex(tpl, 0)
	if g == nil || !g.Base().Focused {
		t.Error("the first instance's graph node must carry the focus flag")
	}
}

// An EMPTY list's template focusable is skipped by Tab: focus must land on a
// rendered instance (the template has none), so Tab moves on to the button.
func TestListEmptyTemplateSkipped(t *testing.T) {
	tpl := &model.Node{Type: "button", ID: "item", Props: map[string]any{"label": "x"}}
	emptyList := &model.Node{Type: "list", ID: "l", Data: "{{state.items}}", Template: tpl}
	btn := &model.Node{Type: "button", ID: "btn"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{emptyList, btn}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.State["items"] = []any{}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != btn {
		t.Fatalf("tab must skip the empty list's template and focus the button, got %v", e.Inter.Focused)
	}
}

// Enter activates a focused list item: nodeMounted descends the renderItem
// template, so the activation guard (which re-checks mounted) passes.
func TestListItemEnterActivates(t *testing.T) {
	tpl := &model.Node{Type: "button", ID: "item", OnPress: &model.Invoke{Name: "go"}}
	list := &model.Node{Type: "list", ID: "l", Data: "{{state.items}}", Template: tpl}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{list}}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"go": {ID: "go", Steps: []model.Step{{Type: "state.set", Path: "hit", Value: "yes"}}},
		},
	}
	rt := runtime.New(app)
	rt.State["items"] = names("a", "b", "c")
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != tpl {
		t.Fatalf("tab must focus the list item template, got %v", e.Inter.Focused)
	}
	e.HandleKey(KeyInput{Key: "return", Down: true})
	if rt.State["hit"] != "yes" {
		t.Error("Enter must activate the focused list item (dispatch its onPress)")
	}
}

// A reorderable list claims a press on one of its items and tracks the drag
// to a new slot: dragging item 0 down 1.5 item-heights lands at To=2, and the
// release dispatches onReorder {from, to}.
func TestListReorderDrag(t *testing.T) {
	tpl := &model.Node{Type: "box", ID: "item", Style: map[string]any{"height": 40.0}}
	list := &model.Node{Type: "list", ID: "l1", Data: "{{state.items}}", Template: tpl,
		Props: map[string]any{"reorderable": true, "onReorder": map[string]any{"name": "reorder"}}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{list}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"reorder": {ID: "reorder", Steps: []model.Step{
				{Type: "state.set", Path: "lastFrom", Value: "{{from}}"},
				{Type: "state.set", Path: "lastTo", Value: "{{to}}"},
			}},
		}}
	rt := runtime.New(app)
	rt.State["items"] = names("a", "b", "c")
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Item 0 spans y 0..40; drag it down 60px (1.5 item heights) → To=2.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 20, Y: 20, Buttons: 1})
	if !e.Inter.Reorder.Active {
		t.Fatal("a press on a reorderable list item must arm the gesture")
	}
	e.HandlePointer(PointerInput{Type: PointerMove, X: 20, Y: 80, Buttons: 1})
	if e.Inter.Reorder.To != 2 {
		t.Fatalf("target slot = %d, want 2 (60px / 40px item)", e.Inter.Reorder.To)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 20, Y: 80})
	if rt.State["lastFrom"] != 0 || rt.State["lastTo"] != 2 {
		t.Errorf("reorder dispatch = from %v to %v, want 0→2", rt.State["lastFrom"], rt.State["lastTo"])
	}
	if e.Inter.Reorder.Active {
		t.Error("the release must clear the reorder gesture")
	}
}

// A list WITHOUT reorderable leaves item presses alone (the normal press path
// still works — the reorder claim is opt-in).
func TestListNotReorderablePassesPress(t *testing.T) {
	tpl := &model.Node{Type: "button", ID: "item", Props: map[string]any{"label": "x"},
		OnPress: &model.Invoke{Name: "hit"}}
	list := &model.Node{Type: "list", ID: "l1", Data: "{{state.items}}", Template: tpl}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{list}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"hit": {ID: "hit", Steps: []model.Step{{Type: "state.set", Path: "pressed", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.State["items"] = names("a", "b", "c")
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerPress, X: 20, Y: 20})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 20, Y: 20})
	if rt.State["pressed"] != "yes" {
		t.Error("a non-reorderable list must leave item presses to the item's handler")
	}
	if e.Inter.Reorder.Active {
		t.Error("a non-reorderable list must not arm the reorder gesture")
	}
}

// A state change re-expands the repeat on the next frame: items added and
// removed show up without any identity bookkeeping going stale.
func TestListDataChangeRerenders(t *testing.T) {
	template := &model.Node{Type: "text", ID: "row", Props: map[string]any{"text": "{{item.name}}"}}
	e, surf, _ := listFixture(t, names("a", "b", "c"), template, nil)
	e.DrawFrame(surf)
	if n := len(instanceGroups(t, e, template)); n != 3 {
		t.Fatalf("instances = %d, want 3", n)
	}

	e.RT.State["items"] = names("only")
	e.MarkDirty()
	e.DrawFrame(surf)
	if got := renderedTexts(t, e); fmt.Sprint(got) != "[only]" {
		t.Fatalf("after shrink texts = %v, want [only]", got)
	}

	e.RT.State["items"] = names("w", "x", "y", "z")
	e.MarkDirty()
	e.DrawFrame(surf)
	if n := len(instanceGroups(t, e, template)); n != 4 {
		t.Fatalf("instances after grow = %d, want 4", n)
	}
}

// An item's onPress dispatches with THAT instance's scope: arg bindings see
// the item's data and its index, and the pressed identity lands on the one
// instance — never on its same-template siblings.
func TestListItemEventDispatch(t *testing.T) {
	template := &model.Node{
		Type: "button", ID: "rowbtn",
		Props:   map[string]any{"label": "{{item.name}}"},
		OnPress: &model.Invoke{Name: "pick", Args: map[string]string{"id": "{{item.name}}", "i": "{{index}}"}},
	}
	e, surf, _ := listFixture(t, names("a", "b", "c"), template, nil)
	e.RT.App.Actions = map[string]*model.Action{
		"pick": {ID: "pick", Steps: []model.Step{
			{Type: "state.set", Path: "picked", Value: "{{id}}"},
			{Type: "state.set", Path: "pidx", Value: "{{i}}"},
		}},
	}
	e.DrawFrame(surf)

	groups := instanceGroups(t, e, template)
	if len(groups) != 3 {
		t.Fatalf("instances = %d, want 3", len(groups))
	}
	bb := groups[1].GetBBox()
	cx, cy := (bb.MinX+bb.MaxX)/2, (bb.MinY+bb.MaxY)/2
	e.HandlePointer(PointerInput{Type: PointerPress, X: cx, Y: cy})

	if e.RT.State["picked"] != "b" {
		t.Errorf("picked = %v, want b (the second item)", e.RT.State["picked"])
	}
	if fmt.Sprint(e.RT.State["pidx"]) != "1" {
		t.Errorf("pidx = %v, want 1", e.RT.State["pidx"])
	}
	if e.Inter.Pressed != template || e.Inter.PressedItem != 1 {
		t.Errorf("pressed identity = (%v, %d), want the template node at index 1", e.Inter.Pressed == template, e.Inter.PressedItem)
	}

	// The pressed flag stamps back onto instance 1 only.
	e.DrawFrame(surf)
	groups = instanceGroups(t, e, template)
	for i, g := range groups {
		if got := g.Base().Pressed; got != (i == 1) {
			t.Errorf("instance %d Pressed = %v, want %v", i, got, i == 1)
		}
	}
}

// An empty array, a non-array binding and a missed binding all render zero
// items without complaint (the HTML quiet degradation); a template-less list
// is a plain container for its children.
func TestListEmptyAndNonArrayDataDegradeQuietly(t *testing.T) {
	template := &model.Node{Type: "text", ID: "row", Props: map[string]any{"text": "{{item.name}}"}}
	for name, items := range map[string]any{
		"empty":     []any{},
		"non-array": "nope",
		"object":    map[string]any{"name": "x"},
		"missing":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			e, surf, list := listFixture(t, items, template, nil)
			e.DrawFrame(surf) // must not panic
			if n := len(instanceGroups(t, e, template)); n != 0 {
				t.Fatalf("instances = %d, want 0", n)
			}
			if e.findGroupByModel(list) == nil {
				t.Fatal("the list container itself must still render")
			}
		})
	}

	// A template-less list degrades to a plain container for its children
	// (HTML render_data.go:42-45).
	child := &model.Node{Type: "text", Props: map[string]any{"text": "plain"}}
	list := &model.Node{Type: "list", ID: "l2", Data: "{{state.items}}", Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{list}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(NewHeadlessSurface(image.Pt(400, 400)))
	if got := renderedTexts(t, e); fmt.Sprint(got) != "[plain]" {
		t.Fatalf("template-less list texts = %v, want [plain]", got)
	}
}

// `as` renames the four scope keys (HTML ListAliasNames parity).
func TestListAsAlias(t *testing.T) {
	template := &model.Node{Type: "text", ID: "row", Props: map[string]any{"text": "{{rowIndex}}={{row.name}}"}}
	e, surf, _ := listFixture(t, names("a", "b"), template, map[string]any{"as": "row"})
	e.DrawFrame(surf)
	if got := renderedTexts(t, e); fmt.Sprint(got) != "[0=a 1=b]" {
		t.Fatalf("rendered texts = %v, want [0=a 1=b]", got)
	}
}

// The canvas alias resolution must never drift from the HTML renderer's
// ListAliasNames — the loader validates against the HTML one (render_data.go),
// so a divergence would make the two paths disagree on accepted JSON.
func TestListAliasNamesMatchHTMLRenderer(t *testing.T) {
	for _, as := range []string{"", "item", "rows", "state", "t", "viewport", "route", "prop", "my alias", "2rows", "ok_1"} {
		gotA, gotI, gotF, gotL := listAliasNames(as)
		wantA, wantI, wantF, wantL := render.ListAliasNames(as)
		if gotA != wantA || gotI != wantI || gotF != wantF || gotL != wantL {
			t.Errorf("as=%q: canvas (%q,%q,%q,%q) != HTML (%q,%q,%q,%q)",
				as, gotA, gotI, gotF, gotL, wantA, wantI, wantF, wantL)
		}
	}
}
