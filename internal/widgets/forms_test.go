package widgets

// Engine-level tests for the W9 form widgets (checkbox, switch, radio,
// slider, select, textarea): each widget is proven through the registry seam
// — rendered into a headless surface, driven by pointer/key events, and
// asserted on state write-back, onChange dispatch, bound initial values and
// disabled suppression. Record-level tests complement them where geometry
// (the caret, the displayed label) is the observable.

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

var (
	pxAccent  = color.RGBA{0, 122, 255, 255}   // theme primary (#007AFF)
	pxInputBg = color.RGBA{232, 232, 237, 255} // theme inputBg
	pxCardBg  = color.RGBA{255, 255, 255, 255} // theme cardBg
	pxInk2    = color.RGBA{134, 134, 139, 255} // theme textSecondary
)

// formEngine builds a headless engine around a plain column of the given
// children (laid out from the top-left corner at scale 1, so every widget's
// box is its measured size at a known origin).
func formEngine(t *testing.T, children ...*model.Node) (*canvas.Engine, *canvas.HeadlessSurface) {
	t.Helper()
	return formEngineActions(t, nil, children...)
}

func formEngineActions(t *testing.T, actions map[string]*model.Action, children ...*model.Node) (*canvas.Engine, *canvas.HeadlessSurface) {
	t.Helper()
	root := &model.Node{Type: "column", ID: "root", Children: children}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Actions: actions}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(400, 300))
}

func clickAt(e *canvas.Engine, x, y float64) {
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: x, Y: y})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: x, Y: y})
}

func typeText(e *canvas.Engine, s string) {
	for _, r := range s {
		e.HandleKey(canvas.KeyInput{Key: "rune", Rune: r, Down: true})
	}
}

func px(surf *canvas.HeadlessSurface, x, y int) color.RGBA {
	return surf.Frame().RGBAAt(x, y)
}

// recordOf invokes a registered widget's Record directly with a fabricated
// layout box (0,0,w,h) — the geometry-level half of the proof.
func recordOf(t *testing.T, name string, n *model.Node, rt *runtime.Runtime, w, h int) draw.Node {
	t.Helper()
	wgt, ok := canvas.LookupWidget(name)
	if !ok {
		t.Fatalf("%s not registered", name)
	}
	ln := &canvas.LayoutNode{Node: n, Width: w, Height: h}
	shape := wgt.Record(ln, rt, 1)
	if shape == nil {
		t.Fatalf("%s Record returned nil for a %dx%d box", name, w, h)
	}
	return shape
}

func overlayOf(t *testing.T, name string, n *model.Node, rt *runtime.Runtime, w, h int) draw.Node {
	t.Helper()
	wgt, ok := canvas.LookupWidget(name)
	if !ok {
		t.Fatalf("%s not registered", name)
	}
	ow, ok := wgt.(canvas.OverlayWidget)
	if !ok {
		t.Fatalf("%s does not implement canvas.OverlayWidget", name)
	}
	if !ow.OverlayOpen(n, rt) {
		t.Fatalf("%s overlay is not open", name)
	}
	ln := &canvas.LayoutNode{Node: n, Width: w, Height: h}
	shape := ow.OverlayRecord(ln, rt, 1, image.Point{})
	if shape == nil {
		t.Fatalf("%s OverlayRecord returned nil for a %dx%d box", name, w, h)
	}
	return shape
}

func TestFormWidgetsRegistered(t *testing.T) {
	for _, name := range []string{"checkbox", "switch", "radio", "slider", "select", "dropdown", "textarea"} {
		if _, ok := canvas.LookupWidget(name); !ok {
			t.Errorf("%s must be registered via this package's init", name)
		}
	}
}

// ---------------------------------------------------------------------------
// checkbox
// ---------------------------------------------------------------------------

// A bound checkbox flips state on click, repaints accent-filled, and a second
// click flips back — the whole loop verified down to pixels.
func TestCheckboxToggleWritesState(t *testing.T) {
	cb := &model.Node{Type: "checkbox", ID: "cb", Value: "{{state.agree}}"}
	e, surf := formEngine(t, cb)
	e.DrawFrame(surf)

	// Unchecked: the box interior is the card background, not the accent.
	if got := px(surf, 9, 9); got != pxCardBg {
		t.Fatalf("unchecked box = %v, want cardBg %v", got, pxCardBg)
	}

	clickAt(e, 9, 9)
	if got := e.RT.State["agree"]; got != true {
		t.Fatalf("state.agree = %v, want true after click", got)
	}
	if !e.Dirty() {
		t.Fatal("the toggle must request a redraw")
	}
	e.DrawFrame(surf)
	if got := px(surf, 9, 9); got != pxAccent {
		t.Fatalf("checked box = %v, want accent %v", got, pxAccent)
	}

	clickAt(e, 9, 9)
	if got := e.RT.State["agree"]; got != false {
		t.Fatalf("state.agree = %v, want false after second click", got)
	}
	e.DrawFrame(surf)
	if got := px(surf, 9, 9); got != pxCardBg {
		t.Fatalf("unchecked box = %v, want cardBg %v", got, pxCardBg)
	}
}

// The `checked` prop renders the initial on-state (HTML checkedState), and a
// declared onChange dispatches with the new {value} alongside the write-back.
func TestCheckboxCheckedPropAndOnChange(t *testing.T) {
	cb := &model.Node{
		Type: "checkbox", ID: "cb",
		Props:    map[string]any{"checked": true},
		OnChange: &model.Invoke{Name: "changed"},
	}
	actions := map[string]*model.Action{
		"changed": {ID: "changed", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e, surf := formEngineActions(t, actions, cb)
	e.DrawFrame(surf)
	if got := px(surf, 9, 9); got != pxAccent {
		t.Fatalf("checked-prop box = %v, want accent %v", got, pxAccent)
	}

	// Unbound: the toggle lands in the uncontrolled store and onChange still
	// fires with the new bool.
	clickAt(e, 9, 9)
	if got := e.RT.State["seen"]; got != false {
		t.Fatalf("onChange value = %v, want false (toggled off)", got)
	}
	e.DrawFrame(surf)
	if got := px(surf, 9, 9); got != pxCardBg {
		t.Fatalf("locally toggled box = %v, want cardBg %v", got, pxCardBg)
	}
}

// A disabled checkbox ignores clicks entirely: no state, no focus.
func TestCheckboxDisabledSuppresses(t *testing.T) {
	cb := &model.Node{Type: "checkbox", ID: "cb", Value: "{{state.agree}}",
		Style: map[string]any{"disabled": true}}
	e, surf := formEngine(t, cb)
	e.DrawFrame(surf)

	clickAt(e, 9, 9)
	if v := e.RT.State["agree"]; v != nil {
		t.Errorf("disabled checkbox wrote state.agree = %v, want untouched", v)
	}
	if e.Inter.Focused != nil {
		t.Error("disabled checkbox must not take focus")
	}
}

// Record geometry: the checked box carries the rasterized check icon, the
// unchecked one does not.
func TestCheckboxRecordGeometry(t *testing.T) {
	n := &model.Node{Type: "checkbox", ID: "cb", Props: map[string]any{"checked": true}}
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	on := recordOf(t, "checkbox", n, rt, 18, 18)
	var onRects int
	var onImages int
	walkRects(on, &onRects)
	walkImages(on, &onImages)
	if onRects != 1 || onImages != 1 {
		t.Errorf("checked checkbox drew %d rects and %d images, want box + check icon", onRects, onImages)
	}

	n2 := &model.Node{Type: "checkbox", ID: "cb2"}
	off := recordOf(t, "checkbox", n2, rt, 18, 18)
	var offRects int
	var offImages int
	walkRects(off, &offRects)
	walkImages(off, &offImages)
	if offRects != 1 || offImages != 0 {
		t.Errorf("unchecked checkbox drew %d rects and %d images, want just the box", offRects, offImages)
	}
}

func walkRects(n draw.Node, count *int) {
	if n == nil {
		return
	}
	if _, ok := n.(*draw.Rect); ok {
		*count++
	}
	if g, ok := n.(*draw.Group); ok {
		for _, c := range g.Children {
			walkRects(c, count)
		}
	}
}

func walkImages(n draw.Node, count *int) {
	if n == nil {
		return
	}
	if _, ok := n.(*draw.Image); ok {
		*count++
	}
	if g, ok := n.(*draw.Group); ok {
		for _, c := range g.Children {
			walkImages(c, count)
		}
	}
}

// ---------------------------------------------------------------------------
// switch
// ---------------------------------------------------------------------------

// A bound switch flips state; the track paints accent while on and the thumb
// rides the corresponding end — verified per pixel.
func TestSwitchToggleWritesState(t *testing.T) {
	sw := &model.Node{Type: "switch", ID: "sw", Value: "{{state.on}}"}
	e, surf := formEngine(t, sw)
	e.DrawFrame(surf)

	// Off: track in the input gray (sampled left of the thumb), thumb white
	// at the LEFT end (its center is (13,13)).
	if got := px(surf, 40, 13); got != pxInputBg {
		t.Fatalf("off track = %v, want inputBg %v", got, pxInputBg)
	}
	if got := px(surf, 13, 13); got != pxCardBg {
		t.Fatalf("off thumb = %v, want white %v", got, pxCardBg)
	}

	clickAt(e, 22, 13)
	if got := e.RT.State["on"]; got != true {
		t.Fatalf("state.on = %v, want true after click", got)
	}
	// The thumb SLIDES between ends on toggle; let the tween settle before
	// asserting the on-state pixels.
	time.Sleep(switchSlideDuration + 20*time.Millisecond)
	e.DrawFrame(surf)
	if got := px(surf, 5, 13); got != pxAccent {
		t.Fatalf("on track = %v, want accent %v", got, pxAccent)
	}
	if got := px(surf, 31, 13); got != pxCardBg {
		t.Fatalf("on thumb must ride the right end, pixel = %v, want white %v", got, pxCardBg)
	}
}

// The thumb SLIDES between ends: right after a toggle the switch reports
// Animating (tween in flight) and the thumb is NOT yet at the target end.
func TestSwitchThumbSlides(t *testing.T) {
	sw := &model.Node{Type: "switch", ID: "sw", Value: "{{state.on}}"}
	e, surf := formEngine(t, sw)
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("switch")
	swW := w.(*Switch)

	clickAt(e, 22, 13)
	if !swW.Animating() {
		t.Error("a just-toggled switch must report Animating (thumb slide in flight)")
	}
	// One frame later the thumb is still travelling, not snapped to the on-end.
	e.DrawFrame(surf)
	if got := px(surf, 31, 13); got == pxCardBg {
		t.Error("the thumb must not be at the on-end on the first post-toggle frame")
	}
}

// The `checked` prop gives the initial on-state; disabled suppresses the tap.
func TestSwitchCheckedInitialAndDisabled(t *testing.T) {
	sw := &model.Node{Type: "switch", ID: "sw", Value: "{{state.on}}",
		Props: map[string]any{"checked": true}}
	e, surf := formEngine(t, sw)
	e.DrawFrame(surf)
	// HTML checkedState: the value BINDING wins over the prop, and an unset
	// state reads false — so the prop alone does not light a bound switch.
	// (Sample the right half: the off-thumb covers the left end.)
	if got := px(surf, 40, 13); got != pxInputBg {
		t.Fatalf("bound switch with unset state = %v, want inputBg (binding wins)", got)
	}

	sw2 := &model.Node{Type: "switch", ID: "sw2", Props: map[string]any{"checked": true},
		Style: map[string]any{"disabled": true}}
	e2, surf2 := formEngine(t, sw2)
	e2.DrawFrame(surf2)
	if got := px(surf2, 5, 13); got != pxAccent {
		t.Fatalf("unbound checked switch = %v, want accent %v", got, pxAccent)
	}
	clickAt(e2, 22, 13)
	e2.DrawFrame(surf2)
	if got := px(surf2, 5, 13); got != pxAccent {
		t.Errorf("disabled switch toggled: track = %v, want still accent", got)
	}
	if e2.Inter.Focused != nil {
		t.Error("disabled switch must not take focus")
	}
}

// ---------------------------------------------------------------------------
// radio
// ---------------------------------------------------------------------------

// Clicking a row selects that option into the bound state; the previously
// selected row's dot clears — mutual exclusion through the shared binding.
func TestRadioSelectWritesStateExclusive(t *testing.T) {
	ra := &model.Node{Type: "radio", ID: "ra", Value: "{{state.pick}}",
		Props: map[string]any{"options": []any{"a", "b", "c"}}}
	e, surf := formEngine(t, ra)
	e.RT.State["pick"] = "a"
	e.DrawFrame(surf)

	// Option "a" selected: accent dot at the row-0 circle center (8,8).
	if got := px(surf, 8, 8); got != pxAccent {
		t.Fatalf("selected dot = %v, want accent %v", got, pxAccent)
	}
	if got := px(surf, 8, 30); got == pxAccent {
		t.Fatal("unselected row must not paint the accent dot")
	}

	// Row 1 ("b") circle center is (8, 8+22=30).
	clickAt(e, 8, 30)
	if got := e.RT.State["pick"]; got != "b" {
		t.Fatalf("state.pick = %v, want %q", got, "b")
	}
	e.DrawFrame(surf)
	if got := px(surf, 8, 30); got != pxAccent {
		t.Fatalf("new selection = %v, want accent %v", got, pxAccent)
	}
	if got := px(surf, 8, 8); got != pxCardBg {
		t.Fatalf("old selection must clear (exclusivity), pixel = %v, want cardBg %v", got, pxCardBg)
	}
}

// A radio nested under a container (scene Y differs from its local Y) must
// still map a press to the correct row: the hit geometry is stored in
// ABSOLUTE scene px, or every press lands on a wrong row (the gallery bug).
func TestRadioNestedRowMapping(t *testing.T) {
	ra := &model.Node{Type: "radio", ID: "ra", Value: "{{state.pick}}",
		Props: map[string]any{"options": []any{"a", "b", "c"}}}
	spacer := &model.Node{Type: "box", ID: "sp", Style: map[string]any{"height": 80.0}}
	inner := &model.Node{Type: "column", ID: "inner", Children: []*model.Node{ra}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{spacer, inner}}
	e, surf := formEngine(t, root)
	e.RT.State["pick"] = "a"
	e.DrawFrame(surf)

	// Row 1 ("b") circle center is at scene (8, 80+30=110).
	clickAt(e, 8, 110)
	if got := e.RT.State["pick"]; got != "b" {
		t.Fatalf("state.pick = %v, want %q (a nested radio must map rows in scene px)", got, "b")
	}
}

// Radio measures its option stack; onChange carries the selected value; a
// disabled radio ignores presses.
func TestRadioMeasureOnChangeDisabled(t *testing.T) {
	ra := &model.Node{Type: "radio", ID: "ra", Value: "{{state.pick}}",
		Props:    map[string]any{"options": []any{"a", "b", "c"}},
		OnChange: &model.Invoke{Name: "picked"}}
	actions := map[string]*model.Action{
		"picked": {ID: "picked", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e, surf := formEngineActions(t, actions, ra)
	e.DrawFrame(surf)

	w, ok := canvas.LookupWidget("radio")
	if !ok {
		t.Fatal("radio not registered")
	}
	if _, h := w.Measure(ra, e.RT, nil, 1); h != 3*16+2*6 {
		t.Errorf("radio height = %d, want 3 rows of 16 + 2 gaps of 6 = %d", h, 3*16+2*6)
	}

	clickAt(e, 8, 52) // row 2: center y = 8 + 2*22 = 52
	if got := e.RT.State["seen"]; got != "c" {
		t.Fatalf("onChange value = %v, want %q", got, "c")
	}

	off := &model.Node{Type: "radio", ID: "off", Value: "{{state.x}}",
		Props: map[string]any{"options": []any{"a"}}, Style: map[string]any{"disabled": true}}
	e2, surf2 := formEngine(t, off)
	e2.DrawFrame(surf2)
	clickAt(e2, 8, 8)
	if v := e2.RT.State["x"]; v != nil {
		t.Errorf("disabled radio wrote state.x = %v, want untouched", v)
	}
}

// ---------------------------------------------------------------------------
// slider
// ---------------------------------------------------------------------------

// Press-drag moves the thumb with continuous write-back; release dispatches
// onChange once with the final value.
func TestSliderDragWritesState(t *testing.T) {
	sl := &model.Node{Type: "slider", ID: "sl", Value: "{{state.vol}}",
		Style:    map[string]any{"width": 200.0},
		OnChange: &model.Invoke{Name: "volChanged"}}
	actions := map[string]*model.Action{
		"volChanged": {ID: "volChanged", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e, surf := formEngineActions(t, actions, sl)
	e.RT.State["vol"] = float64(50)
	e.DrawFrame(surf)

	// vol=50 → thumb center at 8 + 0.5*(200-16) = 100: the fill reaches it.
	if got := px(surf, 60, 12); got != pxAccent {
		t.Fatalf("filled track = %v, want accent %v", got, pxAccent)
	}
	if got := px(surf, 150, 12); got != pxInputBg {
		t.Fatalf("empty track = %v, want inputBg %v", got, pxInputBg)
	}

	// Press jumps the thumb: (150-8)/184 = 0.772 → 77.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 150, Y: 12})
	if got := e.RT.State["vol"]; got != float64(77) {
		t.Fatalf("state.vol after press = %v, want 77", got)
	}
	// Drag follows the pointer.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 50, Y: 12})
	if got := e.RT.State["vol"]; got != float64(23) {
		t.Fatalf("state.vol after drag = %v, want 23", got)
	}
	if !e.Dirty() {
		t.Fatal("the drag must keep requesting frames")
	}
	// Release fires onChange once with the final value.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 50, Y: 12})
	if got := e.RT.State["seen"]; got != float64(23) {
		t.Fatalf("onChange value = %v, want 23", got)
	}
	if e.Inter.Pressed != nil {
		t.Error("release must drop the capture")
	}
	e.DrawFrame(surf)
	if got := px(surf, 30, 12); got != pxAccent {
		t.Fatalf("track after drag = %v, want accent %v", got, pxAccent)
	}
}

// Pressing the thumb keeps its current value until movement begins, and the
// pointer keeps the original grab offset instead of snapping the thumb center
// under the cursor.
func TestSliderThumbPressPreservesValueAndOffset(t *testing.T) {
	sl := &model.Node{Type: "slider", ID: "sl", Value: "{{state.vol}}",
		Style: map[string]any{"width": 200.0}}
	e, surf := formEngine(t, sl)
	e.RT.State["vol"] = float64(50)
	e.DrawFrame(surf)

	// At value 50 the thumb center is x=100 and its radius is 8. Grab four
	// pixels to the right of center; a press must not change the value.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 104, Y: 12})
	if got := e.RT.State["vol"]; got != float64(50) {
		t.Fatalf("state.vol after thumb press = %v, want unchanged 50", got)
	}

	// Move ten pixels right. The preserved four-pixel grab offset means the
	// thumb center moves to x=110, which maps to 55 (not the direct x=114 map).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 114, Y: 12})
	if got := e.RT.State["vol"]; got != float64(55) {
		t.Fatalf("state.vol after offset drag = %v, want 55", got)
	}
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 114, Y: 12})
}

// Step snapping and [min,max] clamping apply before every write-back.
func TestSliderStepAndClamp(t *testing.T) {
	sl := &model.Node{Type: "slider", ID: "sl", Value: "{{state.v}}",
		Props: map[string]any{"min": 0.0, "max": 10.0, "step": 2.0},
		Style: map[string]any{"width": 200.0}}
	e, surf := formEngine(t, sl)
	e.DrawFrame(surf)

	// (100-8)/184 = 0.5 → 5.0, snapped to the step grid → 4.0 or 6.0 (round):
	// 5/2 = 2.5 rounds to even? math.Round rounds half away from zero → 6/2...
	// 5/2=2.5 → round → 3 → 3*2 = 6.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 100, Y: 12})
	if got := e.RT.State["v"]; got != float64(6) {
		t.Fatalf("step snap: state.v = %v, want 6", got)
	}
	// Drag beyond the right end clamps to max.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 500, Y: 12})
	if got := e.RT.State["v"]; got != float64(10) {
		t.Fatalf("clamp max: state.v = %v, want 10", got)
	}
	// And beyond the left end to min.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: -50, Y: 12})
	if got := e.RT.State["v"]; got != float64(0) {
		t.Fatalf("clamp min: state.v = %v, want 0", got)
	}
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: -50, Y: 12})
}

// A disabled slider ignores the gesture; a bound initial value places the
// thumb (pixel-proven above).
func TestSliderDisabledSuppresses(t *testing.T) {
	sl := &model.Node{Type: "slider", ID: "sl", Value: "{{state.v}}",
		Style: map[string]any{"width": 200.0, "disabled": true}}
	e, surf := formEngine(t, sl)
	e.DrawFrame(surf)
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 150, Y: 12})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 150, Y: 12})
	if v := e.RT.State["v"]; v != nil {
		t.Errorf("disabled slider wrote state.v = %v, want untouched", v)
	}
	if e.Inter.Pressed != nil || e.Inter.Focused != nil {
		t.Error("disabled slider must not capture or focus")
	}
}

// ---------------------------------------------------------------------------
// select
// ---------------------------------------------------------------------------

// Clicking opens the dropdown; the popup overlays later siblings, and
// clicking an option selects it, writes the new value back and dispatches
// onChange.
func TestSelectExpandsAndChoosesOptions(t *testing.T) {
	se := &model.Node{Type: "select", ID: "se", Value: "{{state.fruit}}",
		Props: map[string]any{"options": []any{
			map[string]any{"value": "a", "label": "Alpha"},
			map[string]any{"value": "b", "label": "Beta"},
		}},
		OnChange: &model.Invoke{Name: "fruitChanged"}}
	sibling := &model.Node{
		Type:  "row",
		ID:    "sibling",
		Style: map[string]any{"width": 220.0, "height": 84.0, "background": "#FF0000"},
	}
	actions := map[string]*model.Action{
		"fruitChanged": {ID: "fruitChanged", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e, surf := formEngineActions(t, actions, se, sibling)
	e.RT.State["fruit"] = "a"
	e.DrawFrame(surf)

	w, ok := canvas.LookupWidget("select")
	if !ok {
		t.Fatal("select not registered")
	}
	baseW, baseH := w.Measure(se, e.RT, nil, 1)
	if baseW <= 0 || baseH <= 0 {
		t.Fatalf("select collapsed measure = %dx%d, want positive", baseW, baseH)
	}

	clickAt(e, 10, 10)
	if got := e.RT.State["fruit"]; got != "a" {
		t.Fatalf("state.fruit = %v, want %q after opening press", got, "a")
	}
	if got := e.RT.State["seen"]; got != nil {
		t.Fatalf("onChange fired on open press: %v, want untouched", got)
	}
	if !e.Dirty() {
		t.Fatal("opening the dropdown must request a redraw")
	}

	e.DrawFrame(surf)
	openW, openH := w.Measure(se, e.RT, nil, 1)
	if openW != baseW || openH != baseH {
		t.Fatalf("open select measure = %dx%d, want collapsed %dx%d", openW, openH, baseW, baseH)
	}
	shape := recordOf(t, "select", se, e.RT, openW, openH)
	overlay := overlayOf(t, "select", se, e.RT, openW, openH)
	var texts []*draw.Text
	walkTexts(shape, &texts)
	walkTexts(overlay, &texts)
	wantLabels := map[string]bool{"Alpha": false, "Beta": false}
	for _, txt := range texts {
		if _, ok := wantLabels[txt.Content]; ok {
			wantLabels[txt.Content] = true
		}
	}
	for label, seen := range wantLabels {
		if !seen {
			t.Fatalf("open select missing label %q: %+v", label, texts)
		}
	}

	if got := px(surf, baseW-18, baseH+6); got == (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("dropdown failed to overlay the sibling row: pixel = %v", got)
	}

	rowH := selectRowHeight(se, 1)
	y := float64(baseH + selectMenuGap + selectMenuPad + rowH + rowH/2)
	clickAt(e, 10, y)
	if got := e.RT.State["fruit"]; got != "b" {
		t.Fatalf("state.fruit = %v, want %q after picking the second row", got, "b")
	}
	if got := e.RT.State["seen"]; got != "b" {
		t.Fatalf("onChange value = %v, want %q", got, "b")
	}
	e.DrawFrame(surf)
	closedW, closedH := w.Measure(se, e.RT, nil, 1)
	if closedW != baseW || closedH != baseH {
		t.Fatalf("closed select measure = %dx%d, want original %dx%d", closedW, closedH, baseW, baseH)
	}
}

// The box shows the selected option's label; with an empty value it shows the
// first option (the browser's default display), and Record paints the chrome
// plus the rasterized chevron indicator.
func TestSelectRecordDisplay(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	rt.State["fruit"] = "b"
	se := &model.Node{Type: "select", ID: "se", Value: "{{state.fruit}}",
		Props: map[string]any{"options": []any{
			map[string]any{"value": "a", "label": "Alpha"},
			map[string]any{"value": "b", "label": "Beta"},
		}}}
	shape := recordOf(t, "select", se, rt, 140, 32)
	var texts []*draw.Text
	walkTexts(shape, &texts)
	if len(texts) == 0 || texts[0].Content != "Beta" {
		t.Fatalf("select display = %+v, want the selected option's label %q", texts, "Beta")
	}
	var bars int
	var images int
	walkRects(shape, &bars)
	walkImages(shape, &images)
	if bars != 1 || images != 1 {
		t.Errorf("select drew %d rects and %d images, want chrome + chevron icon", bars, images)
	}

	// Empty value: the first option's label shows (browser default).
	rt.State["fruit"] = ""
	shape = recordOf(t, "select", se, rt, 140, 32)
	texts = nil
	walkTexts(shape, &texts)
	if len(texts) == 0 || texts[0].Content != "Alpha" {
		t.Errorf("empty-value display = %+v, want the first option %q", texts, "Alpha")
	}
}

func walkTexts(n draw.Node, out *[]*draw.Text) {
	if n == nil {
		return
	}
	if t, ok := n.(*draw.Text); ok {
		*out = append(*out, t)
	}
	if g, ok := n.(*draw.Group); ok {
		for _, c := range g.Children {
			walkTexts(c, out)
		}
	}
}

// A disabled select ignores presses.
func TestSelectDisabledSuppresses(t *testing.T) {
	se := &model.Node{Type: "select", ID: "se", Value: "{{state.fruit}}",
		Props: map[string]any{"options": []any{"a", "b"}},
		Style: map[string]any{"disabled": true}}
	e, surf := formEngine(t, se)
	e.DrawFrame(surf)
	clickAt(e, 10, 10)
	if v := e.RT.State["fruit"]; v != nil {
		t.Errorf("disabled select wrote state.fruit = %v, want untouched", v)
	}
}

// ---------------------------------------------------------------------------
// textarea
// ---------------------------------------------------------------------------

// Clicking focuses the textarea and opens the shared edit session (the
// engine's widened routing); typing, Enter and write-back behave like the
// single-line input, and onChange dispatches per edit.
func TestTextareaClickFocusesAndEdits(t *testing.T) {
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}",
		OnChange: &model.Invoke{Name: "noted"}}
	actions := map[string]*model.Action{
		"noted": {ID: "noted", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e, surf := formEngineActions(t, actions, ta)
	e.DrawFrame(surf)

	clickAt(e, 10, 10)
	if e.Inter.Focused != ta {
		t.Fatalf("click must focus the textarea, focused=%v", e.Inter.Focused)
	}
	s := e.Inter.Input
	if s == nil || s.Node != ta {
		t.Fatal("click must open the edit session (input.go session, widened routing)")
	}

	typeText(e, "hi")
	e.HandleKey(canvas.KeyInput{Key: "return", Down: true})
	typeText(e, "x")
	if got := e.RT.State["note"]; got != "hi\nx" {
		t.Fatalf("state.note = %v, want %q (Enter = newline, per-keystroke write-back)", got, "hi\nx")
	}
	if got := e.RT.State["seen"]; got != "hi\nx" {
		t.Fatalf("onChange value = %v, want %q", got, "hi\nx")
	}

	// Clicking blank space blurs and closes the session (input parity).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 350, Y: 280})
	if e.Inter.Focused != nil || e.Inter.Input != nil {
		t.Error("blank press must blur the textarea and close the session")
	}
}

// Record renders one text run per line at fs*1.2, the placeholder in the
// secondary color, and — with a live session — the caret at its (line, col).
func TestTextareaRecordLinesCaret(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	rt.State["note"] = "ab\ncd"
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}"}

	// Bound initial value: two text runs, second line one row down.
	shape := recordOf(t, "textarea", ta, rt, 160, 72)
	var texts []*draw.Text
	walkTexts(shape, &texts)
	if len(texts) != 2 || texts[0].Content != "ab" || texts[1].Content != "cd" {
		t.Fatalf("textarea lines = %+v, want [ab cd]", texts)
	}
	if texts[1].Y-texts[0].Y != float64(lineHeight(14)) {
		t.Errorf("line stride = %v, want fs*1.2 = %v", texts[1].Y-texts[0].Y, lineHeight(14))
	}

	// A live session drives the caret: buffer "ab\ncd", cursor 4 = line 1,
	// column 1 → caret at pad + width("c"), one row down.
	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	inter := &canvas.Interaction{Input: &canvas.InputState{Node: ta, Runes: []rune("ab\ncd"), Cursor: 4}}
	tw.mu.Lock()
	tw.inters[ta] = inter
	tw.mu.Unlock()
	ln := &canvas.LayoutNode{Node: ta, Width: 160, Height: 72}
	shape = tw.Record(ln, rt, 1)
	var caret *draw.Rect
	var findCaret func(n draw.Node)
	findCaret = func(n draw.Node) {
		if r, ok := n.(*draw.Rect); ok && r.NoHit && r.Width == 1 && r.StrokeWidth == 0 {
			caret = r
		}
		if g, ok := n.(*draw.Group); ok {
			for _, c := range g.Children {
				findCaret(c)
			}
		}
	}
	findCaret(shape)
	if caret == nil {
		t.Fatal("an editing textarea must paint the caret")
	}
	wantX := float64(12 + int(canvas.MeasureText("c", 14)))
	if caret.X != wantX {
		t.Errorf("caret x = %v, want pad + width(c) = %v", caret.X, wantX)
	}
	if caret.Y != float64(12+lineHeight(14)) {
		t.Errorf("caret y = %v, want pad + one line = %v", caret.Y, 12+lineHeight(14))
	}

	// Empty value: the placeholder shows in the secondary color.
	ta2 := &model.Node{Type: "textarea", ID: "ta2", Placeholder: "Notes"}
	shape = recordOf(t, "textarea", ta2, rt, 160, 72)
	texts = nil
	walkTexts(shape, &texts)
	if len(texts) != 1 || texts[0].Content != "Notes" || texts[0].Fill != pxInk2 {
		t.Errorf("placeholder = %+v, want one run %q in textSecondary %v", texts, "Notes", pxInk2)
	}
}

// Lines overflowing the box vertically are clipped at the pixel level: a
// 1-row box (40px tall) over a 3-line value shows the top of line 2 but
// nothing below the box edge.
func TestTextareaOverflowClipped(t *testing.T) {
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}",
		Props: map[string]any{"rows": 1.0}}
	e, surf := formEngine(t, ta)
	e.RT.State["note"] = "aaaa\nbbbb\ncccc"
	e.DrawFrame(surf)

	// Box: h = 1*16 + 2*12 = 40. Line 2 starts at y = 12+16 = 28: its glyphs
	// (cap height ~10px) cross the box edge — ink above 40 must exist, and
	// nothing may paint below it. Ink is coverage-blended by the sfnt engine,
	// so dark is a threshold, not an exact color.
	darkAbove, darkBelow := 0, 0
	for y := 28; y < 60; y++ {
		for x := 12; x < 60; x++ {
			c := px(surf, x, y)
			if c.R < 200 && c.G < 200 && c.B < 200 {
				if y < 40 {
					darkAbove++
				} else {
					darkBelow++
				}
			}
		}
	}
	if darkAbove == 0 {
		t.Error("line 2's visible top must paint inside the box (clip, not line-cull)")
	}
	if darkBelow != 0 {
		t.Errorf("%d text pixels below the box edge — vertical overflow must be clipped", darkBelow)
	}
}

// A disabled textarea swallows the click: no focus, no session, no edits.
func TestTextareaDisabledSuppresses(t *testing.T) {
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}",
		Style: map[string]any{"disabled": true}}
	e, surf := formEngine(t, ta)
	e.DrawFrame(surf)
	clickAt(e, 10, 10)
	if e.Inter.Focused != nil {
		t.Error("disabled textarea must not take focus")
	}
	if e.Inter.Input != nil {
		t.Error("disabled textarea must not open an edit session")
	}
	typeText(e, "zz")
	if v := e.RT.State["note"]; v != nil {
		t.Errorf("disabled textarea edited state.note = %v, want untouched", v)
	}
}
