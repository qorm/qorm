package canvas

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// testRuntime builds a bare runtime with the given state: measure/layout only
// read State and Viewport, so no app or theme is needed.
func testRuntime(state map[string]any) *runtime.Runtime {
	if state == nil {
		state = map[string]any{}
	}
	return &runtime.Runtime{State: state}
}

// layoutScene runs the full Layout pipeline (measure + layout + record) and
// returns the graph root.
func layoutScene(root *model.Node, rt *runtime.Runtime, size image.Point) graph.Node {
	ops := &op.Ops{}
	g, _ := Layout(ops, root, size, rt, nil, 1)
	return g
}

// walkModel finds the graph node built from the model node with the given id.
func walkModel(n graph.Node, id string) graph.Node {
	if n == nil {
		return nil
	}
	if m := n.Base().Model; m != nil && m.ID == id {
		return n
	}
	if g, ok := n.(*graph.Group); ok {
		for _, c := range g.Children {
			if hit := walkModel(c, id); hit != nil {
				return hit
			}
		}
	}
	return nil
}

// findModel finds a model node by id, descending into children and the
// Then/Else branches of `when` nodes.
func findModel(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if hit := findModel(c, id); hit != nil {
			return hit
		}
	}
	if hit := findModel(n.Then, id); hit != nil {
		return hit
	}
	return findModel(n.Else, id)
}

// hasAncestor reports whether n sits inside ancestor's subtree (or is it).
func hasAncestor(n graph.Node, ancestor graph.Node) bool {
	for n != nil {
		if n == ancestor {
			return true
		}
		p := n.Base().Parent
		if p == nil {
			return false
		}
		n = p
	}
	return false
}

func childIDs(ln *LayoutNode) []string {
	var ids []string
	for _, c := range ln.Children {
		ids = append(ids, c.Node.ID)
	}
	return ids
}

// The canvas measure pass must honour the HTML conditional-rendering
// semantics (render.go:782 visible): `if`/`visible`/`show` hide the whole
// subtree, first key wins, same truthiness; flipping the state re-measures.
func TestMeasureConditionalVisibility(t *testing.T) {
	mk := func() *model.Node {
		return &model.Node{Type: "column", ID: "root", Children: []*model.Node{
			{Type: "text", ID: "a", Props: map[string]any{"text": "a"}},
			{Type: "text", ID: "b", Props: map[string]any{"text": "b", "if": "{{state.show}}"}},
			{Type: "text", ID: "c", Props: map[string]any{"text": "c", "visible": false}},
			{Type: "text", ID: "d", Props: map[string]any{"text": "d", "show": "false"}},
			{Type: "text", ID: "e", Props: map[string]any{"text": "e", "if": true}},
		}}
	}

	rt := testRuntime(map[string]any{"show": false})
	if got, want := childIDs(Measure(mk(), rt, nil, 1)), []string{"a", "e"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("hidden condition measured %v, want %v", got, want)
	}

	rt.State["show"] = true
	if got, want := childIDs(Measure(mk(), rt, nil, 1)), []string{"a", "b", "e"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("flipped condition measured %v, want %v", got, want)
	}

	// First matching key wins (if > visible > show), mirroring render.go:783.
	n := &model.Node{Type: "text", ID: "x", Props: map[string]any{"if": false, "visible": true, "show": true}}
	if nodeVisible(n, rt) {
		t.Error(`"if" must win over "visible"/"show"`)
	}

	// Truthiness matches asBool (render_style.go:1042): number 0 and missed
	// bindings are false, a non-zero number is true.
	rt2 := testRuntime(map[string]any{"zero": 0.0, "two": 2.0})
	for cond, want := range map[string]bool{"{{state.zero}}": false, "{{state.two}}": true, "{{state.missing}}": false} {
		cn := &model.Node{Type: "text", ID: "x", Props: map[string]any{"if": cond}}
		if got := nodeVisible(cn, rt2); got != want {
			t.Errorf("nodeVisible(if=%q) = %v, want %v", cond, got, want)
		}
	}
}

// A hidden node must leave no geometry behind: when the condition flips, the
// siblings below shift by exactly the node's height plus the gap.
func TestConditionalGeometryFlips(t *testing.T) {
	b := &model.Node{Type: "text", ID: "b", Props: map[string]any{"text": "bb", "if": "{{state.show}}"}}
	root := &model.Node{Type: "column", ID: "root",
		Style: map[string]any{"gap": 10},
		Children: []*model.Node{
			{Type: "text", ID: "a", Props: map[string]any{"text": "aa"}},
			b,
			{Type: "text", ID: "c", Props: map[string]any{"text": "cc"}},
		}}
	rt := testRuntime(map[string]any{"show": false})

	gHidden := layoutScene(root, rt, image.Pt(200, 200))
	if walkModel(gHidden, "b") != nil {
		t.Fatal("b must be absent from the graph while its condition is falsy")
	}
	aHidden, cHidden := walkModel(gHidden, "a"), walkModel(gHidden, "c")
	if aHidden == nil || cHidden == nil {
		t.Fatal("siblings of a hidden node must render")
	}

	rt.State["show"] = true
	gShown := layoutScene(root, rt, image.Pt(200, 200))
	bShown := walkModel(gShown, "b")
	if bShown == nil {
		t.Fatal("b must appear once its condition flips true")
	}
	cShown := walkModel(gShown, "c")

	if cShown.Base().Y <= cHidden.Base().Y {
		t.Fatalf("c must move down once b renders: hidden y=%v shown y=%v", cHidden.Base().Y, cShown.Base().Y)
	}
	wantShift := bShown.Base().Height + 10 // node + one gap
	if got := cShown.Base().Y - cHidden.Base().Y; got != wantShift {
		t.Errorf("c shifted by %v, want %v (b height + gap)", got, wantShift)
	}
	if cShown.Base().Y-bShown.Base().Y != bShown.Base().Height+10 {
		t.Errorf("c must sit one gap below b: b y=%v h=%v, c y=%v", bShown.Base().Y, bShown.Base().Height, cShown.Base().Y)
	}
}

// `when` nodes swap between the Then/Else subtrees (render.go:771): state-
// driven conditions follow state, viewport-driven conditions follow the
// surface size Layout wires into rt.Viewport, and a missing Else hides the
// node entirely.
func TestWhenNodeBranches(t *testing.T) {
	mk := func(cond string) *model.Node {
		return &model.Node{Type: "column", ID: "root", Children: []*model.Node{
			{Type: "when", ID: "w", Condition: cond,
				Then: &model.Node{Type: "text", ID: "then"},
				Else: &model.Node{Type: "text", ID: "else"}},
		}}
	}

	rt := testRuntime(map[string]any{"wide": false})
	g := layoutScene(mk("{{state.wide}}"), rt, image.Pt(200, 200))
	if walkModel(g, "then") != nil || walkModel(g, "else") == nil {
		t.Fatal("falsy condition must render the else branch")
	}
	rt.State["wide"] = true
	g = layoutScene(mk("{{state.wide}}"), rt, image.Pt(200, 200))
	if walkModel(g, "then") == nil || walkModel(g, "else") != nil {
		t.Fatal("truthy condition must render the then branch")
	}

	// Viewport condition (the dashboard pattern): narrow surface → else,
	// wide surface → then.
	vp := mk("{{ viewport.width >= 300 }}")
	g = layoutScene(vp, testRuntime(nil), image.Pt(200, 200))
	if walkModel(g, "then") != nil || walkModel(g, "else") == nil {
		t.Fatal("viewport.width >= 300 must be falsy on a 200px surface")
	}
	g = layoutScene(vp, testRuntime(nil), image.Pt(400, 200))
	if walkModel(g, "then") == nil || walkModel(g, "else") != nil {
		t.Fatal("viewport.width >= 300 must be truthy on a 400px surface")
	}

	// No else + falsy condition: the when node contributes nothing.
	noElse := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "a"},
		{Type: "when", ID: "w", Condition: "{{state.no}}", Then: &model.Node{Type: "text", ID: "then"}},
	}}
	if got := childIDs(Measure(noElse, testRuntime(nil), nil, 1)); fmt.Sprint(got) != fmt.Sprint([]string{"a"}) {
		t.Errorf("when without else measured %v, want [a]", got)
	}
}

// A stack layers its children at one origin — declaration order is the
// z-order for both painting and hit testing — and align/justify position each
// child inside the stack's content box.
func TestStackLayersAtSameOrigin(t *testing.T) {
	mkBox := func(id string, w, h float64) *model.Node {
		return &model.Node{Type: "box", ID: id, Style: map[string]any{
			"width": w, "height": h, "background": "#FF0000",
		}}
	}
	back, front := mkBox("back", 60, 40), mkBox("front", 20, 10)
	stack := &model.Node{Type: "stack", ID: "s",
		Style:    map[string]any{"padding": 4.0},
		Children: []*model.Node{back, front}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{stack}}

	g := layoutScene(root, testRuntime(nil), image.Pt(200, 200))
	gs, gb, gf := walkModel(g, "s"), walkModel(g, "back"), walkModel(g, "front")
	if gs == nil || gb == nil || gf == nil {
		t.Fatal("stack and both children must render")
	}

	// Same origin: both children start at the stack's padding corner (their
	// graph coordinates are local to the stack group).
	for _, c := range []graph.Node{gb, gf} {
		if c.Base().X != 4 || c.Base().Y != 4 {
			t.Errorf("child %q origin = (%v,%v), want (4,4) — stack children share one origin",
				c.Base().Model.ID, c.Base().X, c.Base().Y)
		}
	}

	// The stack is a block-level container: its width stretches to the
	// container (the scene root spans the viewport, layout.go); its height
	// still sizes to the largest child plus padding.
	if gs.Base().Width != 200 || gs.Base().Height != 48 {
		t.Errorf("stack size = %vx%v, want 200x48 (block width stretches; height = largest child + 2×padding)", gs.Base().Width, gs.Base().Height)
	}

	// z-order: the later sibling is on top — a point inside both must hit
	// front's subtree, not back's.
	bb := gf.GetBBox()
	hit := g.HitTest(geom.Point{X: (bb.MinX + bb.MaxX) / 2, Y: (bb.MinY + bb.MaxY) / 2})
	if hit == nil || !hasAncestor(hit, gf) {
		t.Errorf("hit inside both layers = %v, want a node in front's subtree", hit)
	}

	// Alignment: an explicit-size stack with align/justify center centres
	// each child inside its content box.
	centered := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "stack", ID: "s",
			Style:    map[string]any{"width": 100.0, "height": 100.0},
			Layout:   map[string]any{"align": "center", "justify": "center"},
			Children: []*model.Node{mkBox("kid", 20, 10)}},
	}}
	g = layoutScene(centered, testRuntime(nil), image.Pt(200, 200))
	kid := walkModel(g, "kid")
	if kid == nil {
		t.Fatal("stack child must render")
	}
	if kid.Base().X != 40 || kid.Base().Y != 45 {
		t.Errorf("centered stack child at (%v,%v), want (40,45)", kid.Base().X, kid.Base().Y)
	}
}

// An x/y style key places a child out of flow at the container origin +
// (x, y) — the infinite-canvas board's coordinate model. Coordinates may be
// negative or past the container edge (an unbounded plane has no default
// clip); left/top are the HTML aliases.
func TestAbsolutePositionXY(t *testing.T) {
	mkNote := func(id string, x, y float64) *model.Node {
		return &model.Node{Type: "box", ID: id, Style: map[string]any{
			"width": 60.0, "height": 40.0, "background": "#FFCC00", "x": x, "y": y,
		}}
	}
	root := &model.Node{Type: "column", ID: "root",
		Style:    map[string]any{"width": 200.0, "height": 200.0},
		Children: []*model.Node{mkNote("a", 10, 20), mkNote("b", 150, -30)}}

	g := layoutScene(root, testRuntime(nil), image.Pt(200, 200))
	ga, gb := walkModel(g, "a"), walkModel(g, "b")
	if ga == nil || gb == nil {
		t.Fatal("absolute children must render")
	}
	if ga.Base().X != 10 || ga.Base().Y != 20 {
		t.Errorf("note a at (%v,%v), want (10,20)", ga.Base().X, ga.Base().Y)
	}
	if gb.Base().X != 150 || gb.Base().Y != -30 {
		t.Errorf("note b at (%v,%v), want (150,-30)", gb.Base().X, gb.Base().Y)
	}

	// left/top are HTML aliases for x/y.
	root2 := &model.Node{Type: "column", ID: "root2",
		Style:    map[string]any{"width": 200.0, "height": 200.0},
		Children: []*model.Node{{Type: "box", ID: "c", Style: map[string]any{"width": 30.0, "height": 20.0, "left": 5.0, "top": 6.0}}}}
	g2 := layoutScene(root2, testRuntime(nil), image.Pt(200, 200))
	gc := walkModel(g2, "c")
	if gc == nil || gc.Base().X != 5 || gc.Base().Y != 6 {
		t.Errorf("left/top alias: c at (%v,%v), want (5,6)", gc.Base().X, gc.Base().Y)
	}
}

// A grid lays children out in N equal columns (HTML repeat(N,1fr) with the
// `columns` prop, render_style.go:103), gap between tracks, row height from
// the tallest child in the row.
func TestGridLayout(t *testing.T) {
	mkBox := func(id string) *model.Node {
		return &model.Node{Type: "box", ID: id, Style: map[string]any{"width": 40.0, "height": 20.0}}
	}
	grid := &model.Node{Type: "grid", ID: "g",
		Props: map[string]any{"columns": 3.0},
		Style: map[string]any{"gap": 12.0, "width": 300.0},
		Children: []*model.Node{
			mkBox("a"), mkBox("b"), mkBox("c"), mkBox("d"),
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{grid}}

	g := layoutScene(root, testRuntime(nil), image.Pt(400, 400))
	pos := map[string][2]float64{}
	for _, id := range []string{"a", "b", "c", "d"} {
		n := walkModel(g, id)
		if n == nil {
			t.Fatalf("grid child %q must render", id)
		}
		pos[id] = [2]float64{n.Base().X, n.Base().Y}
	}

	// colW = (300 - 2×12) / 3 = 92 → cells at x = 0, 104, 208; row height 20,
	// second row at y = 20 + 12 = 32.
	want := map[string][2]float64{"a": {0, 0}, "b": {104, 0}, "c": {208, 0}, "d": {0, 32}}
	for id, w := range want {
		if pos[id] != w {
			t.Errorf("grid child %q at %v, want %v", id, pos[id], w)
		}
	}

	// Default columns is 2 (HTML propNum(n, "columns", 2)); an auto-width grid
	// spans the container (the scene root spans the viewport), so the tracks
	// split it equally: colW = (400-10)/2 = 195, b at x = 195+10 = 205.
	def := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "grid", ID: "g2",
			Style:    map[string]any{"gap": 10.0},
			Children: []*model.Node{mkBox("a"), mkBox("b"), mkBox("c")}},
	}}
	g = layoutScene(def, testRuntime(nil), image.Pt(400, 400))
	pa, pb, pc := walkModel(g, "a"), walkModel(g, "b"), walkModel(g, "c")
	if pa == nil || pb == nil || pc == nil {
		t.Fatal("default-columns grid children must render")
	}
	if pa.Base().X != 0 || pa.Base().Y != 0 || pb.Base().X != 205 || pb.Base().Y != 0 || pc.Base().X != 0 || pc.Base().Y != 30 {
		t.Errorf("default grid geometry a=%v b=%v c=%v, want (0,0) (205,0) (0,30)",
			[2]float64{pa.Base().X, pa.Base().Y}, [2]float64{pb.Base().X, pb.Base().Y}, [2]float64{pc.Base().X, pc.Base().Y})
	}
}

// Auto-sized grid children stretch to their equal-width tracks. Without this
// top-down resolution, cards shrink back to their intrinsic text width even
// though the grid itself has already computed full-width columns.
func TestGridAutoItemsStretchToTracks(t *testing.T) {
	grid := &model.Node{Type: "grid", ID: "g",
		Props: map[string]any{"columns": 3.0},
		Style: map[string]any{"gap": 12.0, "width": 300.0},
		Children: []*model.Node{
			{Type: "text", ID: "a", Props: map[string]any{"text": "1.2k"}},
			{Type: "text", ID: "b", Props: map[string]any{"text": "348"}},
			{Type: "text", ID: "c", Props: map[string]any{"text": "27"}},
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{grid}}

	g := layoutScene(root, testRuntime(nil), image.Pt(400, 200))
	wantW := float64((300 - 2*12) / 3)
	for _, id := range []string{"a", "b", "c"} {
		child := walkModel(g, id)
		if child == nil {
			t.Fatalf("grid child %q must render", id)
		}
		if got := child.Base().Width; got != wantW {
			t.Errorf("grid child %q width = %v, want track width %v", id, got, wantW)
		}
	}
}

// The `opacity` style key must multiply into the drawn pixels (it was
// silently ignored before): 0.5 alpha-composites over the white frame, 0
// hides, values clamp to [0,1] like CSS, a binding resolves, and a parent's
// opacity multiplies its whole subtree.
func TestOpacityStyleKeyPixels(t *testing.T) {
	renderBox := func(style map[string]any) *image.RGBA {
		box := &model.Node{Type: "box", ID: "box", Style: style}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
		ops := &op.Ops{}
		Layout(ops, root, image.Pt(60, 60), testRuntime(nil), nil, 1)
		return Rasterize(ops, image.Pt(60, 60))
	}
	mkStyle := func(extra map[string]any) map[string]any {
		s := map[string]any{"width": 20.0, "height": 20.0, "background": "#FF0000"}
		for k, v := range extra {
			s[k] = v
		}
		return s
	}
	blended := color.RGBA{255, 128, 128, 255} // straight red@127 over white
	red := color.RGBA{255, 0, 0, 255}
	white := color.RGBA{255, 255, 255, 255}

	if got := renderBox(mkStyle(map[string]any{"opacity": 0.5})).RGBAAt(10, 10); got != blended {
		t.Errorf("opacity 0.5 pixel = %v, want %v", got, blended)
	}
	if got := renderBox(mkStyle(map[string]any{"opacity": 0.0})).RGBAAt(10, 10); got != white {
		t.Errorf("opacity 0 pixel = %v, want white (fully transparent)", got)
	}
	if got := renderBox(mkStyle(nil)).RGBAAt(10, 10); got != red {
		t.Errorf("no opacity pixel = %v, want opaque red", got)
	}
	if got := renderBox(mkStyle(map[string]any{"opacity": 2.0})).RGBAAt(10, 10); got != red {
		t.Errorf("opacity 2 must clamp to 1: pixel = %v, want red", got)
	}

	// Bound opacity (the examples/components pattern "{{ state.faded ? ... }}").
	rt := testRuntime(map[string]any{"o": 0.5})
	box := &model.Node{Type: "box", ID: "box",
		Style: mkStyle(map[string]any{"opacity": "{{state.o}}"})}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	ops := &op.Ops{}
	Layout(ops, root, image.Pt(60, 60), rt, nil, 1)
	if got := Rasterize(ops, image.Pt(60, 60)).RGBAAt(10, 10); got != blended {
		t.Errorf("bound opacity pixel = %v, want %v", got, blended)
	}

	// Group semantics: a parent's opacity multiplies the child subtree,
	// matching CSS opacity on an element.
	parent := &model.Node{Type: "column", ID: "root",
		Style:    map[string]any{"opacity": 0.5},
		Children: []*model.Node{{Type: "box", ID: "box", Style: mkStyle(nil)}}}
	ops = &op.Ops{}
	Layout(ops, parent, image.Pt(60, 60), testRuntime(nil), nil, 1)
	if got := Rasterize(ops, image.Pt(60, 60)).RGBAAt(10, 10); got != blended {
		t.Errorf("parent-opacity pixel = %v, want %v (subtree must inherit the alpha)", got, blended)
	}
}

// Style keys the canvas renderer does not consume must warn once per key per
// scene — never silently, never per frame.
func TestWarnUnsupportedStyleKeys(t *testing.T) {
	var buf bytes.Buffer
	styleWarnMu.Lock()
	oldOut, oldRoot, oldSeen := styleWarnOut, styleWarnRoot, styleWarnSeen
	styleWarnOut, styleWarnRoot, styleWarnSeen = &buf, nil, map[string]bool{}
	styleWarnMu.Unlock()
	defer func() {
		styleWarnMu.Lock()
		styleWarnOut, styleWarnRoot, styleWarnSeen = oldOut, oldRoot, oldSeen
		styleWarnMu.Unlock()
	}()

	// Use keys that remain intentionally unimplemented on canvas so the
	// one-shot warn path stays covered (gradient/flexGrow are supported now).
	mk := func() *model.Node {
		return &model.Node{Type: "column", ID: "root", Children: []*model.Node{
			{Type: "text", ID: "a", Style: map[string]any{"backdropBlur": 12.0, "letterSpacing": 1.0, "fontSize": 14.0}},
			{Type: "text", ID: "b", Style: map[string]any{"backdropBlur": 8.0}},
		}}
	}
	layoutScene(mk(), testRuntime(nil), image.Pt(100, 100))
	out := buf.String()
	for _, key := range []string{`"backdropBlur"`, `"letterSpacing"`} {
		if !strings.Contains(out, key) {
			t.Errorf("unsupported key %s must warn, got:\n%s", key, out)
		}
	}
	if strings.Contains(out, `"fontSize"`) {
		t.Errorf("supported key fontSize must not warn, got:\n%s", out)
	}
	if !strings.Contains(out, `node id: "a"`) {
		t.Errorf("warning must name the node, got:\n%s", out)
	}
	if c := strings.Count(out, `"backdropBlur"`); c != 1 {
		t.Errorf("backdropBlur warned %d times, want 1 (one-shot per key per scene)", c)
	}

	// Same scene tree (same root pointer, as the engine reuses it across
	// frames): the first frame arms the one-shot warnings, the next is silent.
	scene := mk()
	layoutScene(scene, testRuntime(nil), image.Pt(100, 100))
	buf.Reset()
	layoutScene(scene, testRuntime(nil), image.Pt(100, 100))
	if buf.Len() != 0 {
		t.Errorf("warnings must not repeat per frame, got:\n%s", buf.String())
	}

	// A different scene root re-arms the same key.
	layoutScene(mk(), testRuntime(nil), image.Pt(100, 100))
	if !strings.Contains(buf.String(), `"backdropBlur"`) {
		t.Error("a new scene tree must re-arm the warning")
	}
}

// Real scene (examples/gallery): the `thanks` text carries
// if="{{state.agree}}" — it must stay out of the rendered graph while agree
// is falsy and appear once the state flips, with the rest of the scene
// intact.
func TestExampleGalleryConditionalRender(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "gallery"))
	if err != nil {
		t.Fatalf("load gallery: %v", err)
	}
	root := app.EntryRoot()
	if findModel(root, "thanks") == nil {
		t.Fatal("fixture drift: examples/gallery no longer has the conditional #thanks node")
	}
	rt := runtime.New(app)
	rt.State["agree"] = false

	g := layoutScene(root, rt, image.Pt(400, 800))
	if walkModel(g, "thanks") != nil {
		t.Fatal(`if="{{state.agree}}" must hide #thanks while agree is falsy`)
	}

	rt.State["agree"] = true
	g = layoutScene(root, rt, image.Pt(400, 800))
	if walkModel(g, "thanks") == nil {
		t.Fatal("#thanks must render once agree flips true")
	}
}

// Real scene (examples/components): the #stats2 grid (columns 3, gap 12)
// must place its first three children on one row at equally spaced,
// increasing x positions — not degrade them into a vertical column.
func TestExampleComponentsGrid(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "components"))
	if err != nil {
		t.Fatalf("load components: %v", err)
	}
	root := app.EntryRoot()
	gridNode := findModel(root, "stats2")
	if gridNode == nil || gridNode.Type != "grid" {
		t.Fatal("fixture drift: examples/components no longer has the #stats2 grid")
	}
	rt := runtime.New(app)

	g := layoutScene(root, rt, image.Pt(800, 1200))
	gg := walkModel(g, "stats2")
	if gg == nil {
		t.Fatal("#stats2 must render")
	}
	var kids []graph.Node
	for _, c := range gg.(*graph.Group).Children {
		if c.Base().Model != nil { // skip the container's own background rect
			kids = append(kids, c)
		}
	}
	if len(kids) < 3 {
		t.Fatalf("#stats2 rendered %d children, want at least 3", len(kids))
	}
	y0 := kids[0].Base().Y
	x0 := kids[0].Base().X
	spacing := kids[1].Base().X - x0
	for i, k := range kids[1:3] {
		if k.Base().Y != y0 {
			t.Errorf("grid row-1 child %d y = %v, want %v (same row)", i+1, k.Base().Y, y0)
		}
		if want := x0 + float64(i+1)*spacing; k.Base().X != want || spacing <= 0 {
			t.Errorf("grid row-1 child %d x = %v, want %v (equal columns, increasing)", i+1, k.Base().X, want)
		}
	}
}

// min/max size constraints clamp the resolved box after content and explicit
// sizes (CSS resolution order) — and the keys no longer warn as unsupported.
func TestMinMaxSizeConstraints(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	mk := func(style map[string]any, text string) *model.Node {
		return &model.Node{Type: "text", ID: "t", Props: map[string]any{"text": text}, Style: style}
	}

	// maxWidth clamps a wide content box.
	ln := Measure(mk(map[string]any{"maxWidth": 40.0}, "a much much wider label"), rt, &Interaction{}, 1)
	if ln.Width > 40 {
		t.Errorf("maxWidth: width = %d, want clamped to 40", ln.Width)
	}
	// minWidth grows a narrow content box.
	ln = Measure(mk(map[string]any{"minWidth": 200.0}, "hi"), rt, &Interaction{}, 1)
	if ln.Width < 200 {
		t.Errorf("minWidth: width = %d, want at least 200", ln.Width)
	}
	// Explicit width resolves first, then the constraint clamps it.
	ln = Measure(mk(map[string]any{"width": 300.0, "maxWidth": 100.0}, "x"), rt, &Interaction{}, 1)
	if ln.Width != 100 {
		t.Errorf("width+maxWidth: width = %d, want 100", ln.Width)
	}
	// Height pair.
	ln = Measure(mk(map[string]any{"minHeight": 60.0, "maxHeight": 60.0}, "x"), rt, &Interaction{}, 1)
	if ln.Height != 60 {
		t.Errorf("min+maxHeight: height = %d, want 60", ln.Height)
	}
	// Bound values evaluate.
	rt.State["cap"] = float64(50)
	ln = Measure(mk(map[string]any{"maxWidth": "{{state.cap}}"}, "a much much wider label"), rt, &Interaction{}, 1)
	if ln.Width > 50 {
		t.Errorf("bound maxWidth: width = %d, want clamped to 50", ln.Width)
	}
	// The four keys are consumed, so the one-shot warning must stay silent
	// for them (style.go whitelist).
	for _, k := range []string{"minWidth", "maxWidth", "minHeight", "maxHeight"} {
		if !canvasStyleKeys[k] {
			t.Errorf("style key %q missing from canvasStyleKeys", k)
		}
	}
}
