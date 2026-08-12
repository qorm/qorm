package canvas

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// scrollFixture builds a headless engine around one scroll viewport (200x100)
// holding the given children, and returns the engine, surface and viewport.
func scrollFixture(t *testing.T, children []*model.Node) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	sv := &model.Node{
		Type: "scroll", ID: "sv",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: children,
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sv}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, sv
}

// tallChildren returns n plain rows of the given height (scrollable filler).
func tallChildren(n, h int) []*model.Node {
	out := make([]*model.Node, n)
	for i := range out {
		out[i] = &model.Node{Type: "column", Style: map[string]any{"height": float64(h)}}
	}
	return out
}

// The offset clamps to [0, contentHeight-viewportHeight] in both directions,
// and the clamped value is what the content actually shifts by.
func TestScrollOffsetClamp(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50)) // content 500, viewport 100
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	if !e.HandleScroll(ScrollInput{DY: 10000}) {
		t.Fatal("scroll over the viewport must be consumed")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 400 {
		t.Fatalf("offset after huge downward delta = %v, want 400 (500-100)", off.Y)
	}
	if e.HandleScroll(ScrollInput{DY: 10000}) {
		t.Fatal("already at the bottom: nothing left to consume")
	}
	if !e.HandleScroll(ScrollInput{DY: -10000}) {
		t.Fatal("scrolling back up must be consumed")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 0 {
		t.Fatalf("offset after huge upward delta = %v, want 0", off.Y)
	}

	// The content shifts by exactly the offset.
	first := sv.Children[0]
	before := e.findGroupByModel(first).GetBBox().MinY
	e.HandleScroll(ScrollInput{DY: 40})
	e.DrawFrame(surf)
	after := e.findGroupByModel(first).GetBBox().MinY
	if before-after != 40 {
		t.Fatalf("content shifted by %v, want 40 (the offset)", before-after)
	}
}

// A scroll viewport whose content is wider than the box scrolls horizontally:
// a row of fixed-width cards overflows the 200px viewport and DX drives the x
// offset, clamped to contentW-viewportW, shifting the content left.
func TestScrollHorizontal(t *testing.T) {
	cards := make([]*model.Node, 5)
	for i := range cards {
		cards[i] = &model.Node{Type: "column", Style: map[string]any{"width": 100.0, "height": 60.0}}
	}
	row := &model.Node{Type: "row", Children: cards}
	e, surf, sv := scrollFixture(t, []*model.Node{row})
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	if !e.HandleScroll(ScrollInput{DX: 100}) {
		t.Fatal("horizontal scroll over the viewport must be consumed")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.X != 100 {
		t.Fatalf("x offset = %v, want 100", off.X)
	}
	// Content is 5×100=500 wide, viewport 200 → max x offset 300.
	e.HandleScroll(ScrollInput{DX: 10000})
	if off := e.Inter.ScrollOffsets[sv]; off.X != 300 {
		t.Fatalf("x offset clamped = %v, want 300 (500-200)", off.X)
	}
	// The content shifts horizontally by the offset.
	e.DrawFrame(surf) // render the clamped frame before measuring `before`
	first := row.Children[0]
	before := e.findGroupByModel(first).GetBBox().MinX
	e.HandleScroll(ScrollInput{DX: -100})
	e.DrawFrame(surf)
	after := e.findGroupByModel(first).GetBBox().MinX
	if after-before != 100 {
		t.Fatalf("content shifted horizontally by %v, want 100 right (scrolling back)", after-before)
	}
}

// A scrollable viewport paints a scrollbar thumb at its right edge, sized to
// the visible fraction and sliding with the offset.
func TestScrollScrollbarPainted(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50)) // content 500, viewport 100
	e.DrawFrame(surf)
	thumb := func() *graph.Rect {
		g := e.findGroupByModel(sv).(*graph.Group)
		for _, c := range g.Children {
			if r, ok := c.(*graph.Rect); ok && r.NoHit && r.X == float64(200-6) {
				return r
			}
		}
		return nil
	}

	if tb := thumb(); tb == nil {
		t.Fatal("scrollable viewport must paint a scrollbar thumb")
	} else if tb.Height != 20 {
		t.Errorf("thumb height = %v, want 20 (viewport × visible fraction 100×100/500)", tb.Height)
	} else if tb.Y != 0 {
		t.Errorf("thumb y at top = %v, want 0", tb.Y)
	}

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	e.HandleScroll(ScrollInput{DY: 400}) // offset 400 → thumb at max (100-20)
	e.DrawFrame(surf)
	if tb := thumb(); tb == nil || tb.Y != 80 {
		t.Errorf("thumb y at bottom = %v, want 80 (100-thumbH)", tb.Y)
	}
}

// Keyboard focus that lands on a clipped node scrolls the viewport so the
// node becomes visible: tabbing past the fold advances the offset.
func TestScrollFocusIntoView(t *testing.T) {
	btns := make([]*model.Node, 10)
	for i := range btns {
		btns[i] = &model.Node{Type: "button", ID: fmt.Sprintf("b%d", i)}
	}
	col := &model.Node{Type: "column", Children: btns}
	sv := &model.Node{Type: "scroll", ID: "sv",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: []*model.Node{col}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sv}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	svBBox := func() float64 { return e.findGroupByModel(sv).GetBBox().MaxY }
	scrolled := false
	for i := 0; i < 10; i++ {
		e.HandleKey(KeyInput{Key: "tab", Down: true})
		e.DrawFrame(surf)
		f := e.Inter.Focused
		if f == nil {
			break
		}
		gb := e.findGroupByModel(f)
		if gb == nil {
			continue
		}
		if bb := gb.GetBBox(); bb.MaxY > svBBox() {
			t.Fatalf("button %s still below the viewport fold after tab %d (maxY %v)", f.ID, i, bb.MaxY)
		}
		if off := e.Inter.ScrollOffsets[sv]; off.Y > 0 {
			scrolled = true
		}
	}
	if !scrolled {
		t.Error("focusing below the fold must scroll the viewport, but the offset never advanced")
	}
}

// Focus scrolling through NESTED scroll viewports must keep the node visible
// in BOTH: scrolling the inner shifts the node's outer position, so the outer's
// overshoot is computed against the post-inner-scroll position — otherwise it
// over-scrolls by the whole inner delta and pushes the node back out of view.
func TestScrollFocusIntoViewNested(t *testing.T) {
	btns := make([]*model.Node, 10)
	for i := range btns {
		btns[i] = &model.Node{Type: "button", ID: fmt.Sprintf("b%d", i)}
	}
	col := &model.Node{Type: "column", Children: btns}
	inner := &model.Node{Type: "scroll", ID: "inner",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: []*model.Node{col}}
	below := &model.Node{Type: "column", ID: "below", Style: map[string]any{"height": 300.0}}
	outer := &model.Node{Type: "scroll", ID: "outer",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: []*model.Node{inner, below}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{outer}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	for i := 0; i < 10; i++ {
		e.HandleKey(KeyInput{Key: "tab", Down: true})
		e.DrawFrame(surf)
		f := e.Inter.Focused
		if f == nil {
			break
		}
		gb := e.findGroupByModel(f)
		if gb == nil {
			continue
		}
		bb := gb.GetBBox()
		ib := e.findGroupByModel(inner).GetBBox()
		ob := e.findGroupByModel(outer).GetBBox()
		if bb.MinY < ib.MinY-1 || bb.MaxY > ib.MaxY+1 {
			t.Fatalf("button %s outside the inner viewport after tab %d: [%v,%v] vs [%v,%v]", f.ID, i, bb.MinY, bb.MaxY, ib.MinY, ib.MaxY)
		}
		if bb.MinY < ob.MinY-1 || bb.MaxY > ob.MaxY+1 {
			t.Fatalf("button %s outside the outer viewport after tab %d: [%v,%v] vs [%v,%v]", f.ID, i, bb.MinY, bb.MaxY, ob.MinY, ob.MaxY)
		}
	}
}

// A wheel gesture only scrolls the viewport under the pointer: over empty
// space nothing is consumed and the engine stays clean.
func TestScrollWheelHitAndMiss(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerMove, X: 350, Y: 350}) // outside the 200x100 viewport
	if e.HandleScroll(ScrollInput{DY: 30}) {
		t.Fatal("scroll outside any viewport must not be consumed")
	}
	if e.Dirty() {
		t.Fatal("an unconsumed scroll must not dirty the engine")
	}
	if len(e.Inter.ScrollOffsets) != 0 {
		t.Fatal("an unconsumed scroll must not create offset state")
	}

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50}) // inside
	if !e.HandleScroll(ScrollInput{DY: 30}) {
		t.Fatal("scroll over the viewport must be consumed")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 30 {
		t.Fatalf("offset = %v, want 30", off.Y)
	}
}

// A child scrolled out of the viewport is clipped from hit testing exactly
// like it is clipped from painting: pressing its off-viewport position does
// nothing; after scrolling it into view the same press dispatches.
func TestScrollClippedChildNotHittable(t *testing.T) {
	mk := func(id string) *model.Node {
		return &model.Node{
			Type: "column", ID: id,
			Style:   map[string]any{"width": 200.0, "height": 50.0},
			OnPress: &model.Invoke{Name: "hit"},
		}
	}
	children := []*model.Node{mk("b0"), mk("b1"), mk("b2"), mk("b3")}
	e, surf, _ := scrollFixture(t, children)
	e.RT.App.Actions = map[string]*model.Action{
		"hit": {ID: "hit", Steps: []model.Step{{Type: "state.set", Path: "hits", Value: "{{1}}"}}},
	}
	e.DrawFrame(surf)

	// b3 sits at content y 150..200 — entirely below the 100px viewport.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 100, Y: 175})
	if e.RT.State["hits"] != nil {
		t.Fatal("press on a clipped child must not dispatch")
	}
	if e.Inter.Pressed != nil {
		t.Fatal("press on a clipped child must not set the pressed identity")
	}

	// Scroll it into view (offset 100: b3 now at viewport y 50..100) and the
	// very same node becomes pressable.
	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 75})
	e.HandleScroll(ScrollInput{DY: 100})
	e.DrawFrame(surf)
	e.HandlePointer(PointerInput{Type: PointerPress, X: 100, Y: 75})
	if fmt.Sprint(e.RT.State["hits"]) != "1" {
		t.Fatalf("hits = %v, want 1 after scrolling the child into view", e.RT.State["hits"])
	}
	if e.Inter.Pressed != children[3] {
		t.Fatal("the scrolled-into-view child must take the pressed identity")
	}
}

// Nested viewports chain the gesture: the inner consumes what it can, the
// remainder bubbles to the outer (the web's scroll chaining).
func TestScrollNestedBubbling(t *testing.T) {
	inner := &model.Node{
		Type: "scroll", ID: "inner",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: tallChildren(6, 50), // content 300, range 200
	}
	outer := &model.Node{
		Type: "scroll", ID: "outer",
		Style: map[string]any{"width": 200.0, "height": 100.0},
		Children: []*model.Node{
			inner,
			{Type: "column", Style: map[string]any{"height": 200.0}}, // outer content 300, range 200
		},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{outer}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50}) // over the inner viewport
	if !e.HandleScroll(ScrollInput{DY: 250}) {
		t.Fatal("a 250 delta across two ranges must be consumed")
	}
	if off := e.Inter.ScrollOffsets[inner]; off.Y != 200 {
		t.Fatalf("inner offset = %v, want 200 (its full range)", off.Y)
	}
	if off := e.Inter.ScrollOffsets[outer]; off.Y != 50 {
		t.Fatalf("outer offset = %v, want 50 (the bubbled remainder)", off.Y)
	}

	// Inner exhausted: a further gesture lands entirely on the outer.
	e.HandleScroll(ScrollInput{DY: 1000})
	if off := e.Inter.ScrollOffsets[outer]; off.Y != 200 {
		t.Fatalf("outer offset = %v, want 200 (its full range)", off.Y)
	}
	// Both at their ends: nothing left to consume anywhere.
	if e.HandleScroll(ScrollInput{DY: 1000}) {
		t.Fatal("fully exhausted nesting must report no change")
	}
}

// The paint side of clipping: content past the viewport edge never reaches
// the framebuffer, and scrolling reveals it.
func TestScrollRenderClipPixels(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	body := &model.Node{
		Type: "column", ID: "body",
		Style: map[string]any{"width": 200.0, "height": 200.0, "background": "#FF0000"},
	}
	e, surf, _ := scrollFixture(t, []*model.Node{body})
	e.DrawFrame(surf)

	if got := surf.Frame().RGBAAt(50, 50); got != red {
		t.Fatalf("in-viewport pixel = %v, want the red content", got)
	}
	if got := surf.Frame().RGBAAt(50, 150); got == red {
		t.Fatal("content past the viewport edge must be clipped from the framebuffer")
	}

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	e.HandleScroll(ScrollInput{DY: 100}) // body now spans viewport y -100..100
	e.DrawFrame(surf)
	if got := surf.Frame().RGBAAt(50, 99); got != red {
		t.Fatalf("scrolled content pixel = %v, want red (offset reveals it)", got)
	}
	if got := surf.Frame().RGBAAt(50, 120); got == red {
		t.Fatal("content below the viewport must stay clipped after scrolling")
	}
}

// Touch-drag on a scroll viewport moves the content (finger down → offset
// decreases in scroll space when pulling content down from top is rubber;
// dragging up increases offset like scrolling down).
func TestScrollTouchDrag(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50)) // content 500, viewport 100
	e.DrawFrame(surf)

	// Press in the viewport and drag upward 40px (finger up → scroll down).
	e.HandlePointer(PointerInput{Type: PointerPress, X: 100, Y: 50, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 40, Buttons: 1}) // past slop
	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 10, Buttons: 1})
	if !e.Inter.ScrollDrag.Active {
		t.Fatal("drag past slop must activate ScrollDrag")
	}
	off := e.Inter.ScrollOffsets[sv]
	if off.Y <= 0 {
		t.Fatalf("finger drag up must increase scroll offset Y, got %v", off)
	}
	// Release should clear active drag and seed momentum or settle.
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 100, Y: 10})
	if e.Inter.ScrollDrag.Active || e.Inter.ScrollDrag.Pending {
		t.Fatal("release must clear ScrollDrag")
	}
}

// Pulling past the top rubber-bands (negative offset allowed while dragging)
// and springs back after release.
func TestScrollRubberBandAndSpring(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50))
	e.DrawFrame(surf)

	// At top: drag finger down hard to overscroll.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 100, Y: 20, Buttons: 1})
	for y := 20.0; y <= 120; y += 10 {
		e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: y, Buttons: 1})
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y >= 0 {
		t.Fatalf("pull past top must rubber-band to negative Y, got %v", off.Y)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 100, Y: 120})
	mom := e.Inter.ScrollMomentum[sv]
	if !mom.Spring && !mom.Active {
		// Spring should be armed; if overscroll was tiny maybe already cleared.
		if off := e.Inter.ScrollOffsets[sv]; off.Y < 0 {
			t.Fatal("overscrolled release must arm spring momentum")
		}
	}
	// Advance spring frames until settled.
	for i := 0; i < 60; i++ {
		e.DrawFrame(surf)
		if off := e.Inter.ScrollOffsets[sv]; off.Y >= -0.5 {
			if off.Y < 0 {
				// still settling
				continue
			}
			return
		}
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y < -0.5 {
		t.Fatalf("spring did not settle, offset Y = %v", off.Y)
	}
}

// Nested scroll: when the inner viewport is at its bottom edge, further
// drag-up bubbles to the outer viewport.
func TestScrollNestedDragBubbles(t *testing.T) {
	// Inner: 200x80 viewport, content 200 tall → maxY 120
	innerKids := tallChildren(5, 40)
	inner := &model.Node{
		Type: "scroll", ID: "inner",
		Style:    map[string]any{"width": 180.0, "height": 80.0},
		Children: innerKids,
	}
	// Outer: holds inner + filler
	outerKids := append([]*model.Node{inner}, tallChildren(6, 50)...)
	outer := &model.Node{
		Type: "scroll", ID: "outer",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: outerKids,
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{outer}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Pin inner at its bottom without chaining the outer (direct offset).
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
	}
	e.Inter.ScrollOffsets[inner] = ScrollPos{Y: 200} // past max; layout clamps
	e.Inter.ScrollOffsets[outer] = ScrollPos{Y: 0}
	e.DrawFrame(surf)
	innerOff := e.Inter.ScrollOffsets[inner]
	if innerOff.Y < 50 {
		t.Fatalf("inner should be scrolled, Y=%v", innerOff.Y)
	}
	outerBefore := e.Inter.ScrollOffsets[outer].Y

	// Drag finger up (content follows → offset increases) at inner bottom → bubble.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 50, Y: 40, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 50, Y: 20, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 50, Y: -80, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 50, Y: -80})

	outerAfter := e.Inter.ScrollOffsets[outer].Y
	if outerAfter <= outerBefore {
		t.Fatalf("nested drag at inner bottom should bubble to outer: before=%v after=%v inner=%v",
			outerBefore, outerAfter, e.Inter.ScrollOffsets[inner])
	}
}

// A short tap (no drag past slop) must not leave ScrollDrag armed and must
// not change the offset.
func TestScrollTapDoesNotScroll(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50))
	e.DrawFrame(surf)
	e.HandlePointer(PointerInput{Type: PointerPress, X: 100, Y: 50, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 101, Y: 51, Buttons: 1}) // under slop
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 101, Y: 51})
	if e.Inter.ScrollDrag.Active || e.Inter.ScrollDrag.Pending {
		t.Fatal("tap must clear pending ScrollDrag")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 0 {
		t.Fatalf("tap must not scroll, offset = %v", off)
	}
}
