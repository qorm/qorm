package canvas

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

type observerDispatchWidget struct{}

func (observerDispatchWidget) Measure(*model.Node, *runtime.Runtime, map[string]any, int) (int, int) {
	return 40, 40
}
func (observerDispatchWidget) Record(ln *LayoutNode, _ *runtime.Runtime, _ int) graph.Node {
	r := graph.NewRect()
	r.Width, r.Height = float64(ln.Width), float64(ln.Height)
	r.Fill = color.RGBA{20, 20, 20, 255}
	return r
}
func (observerDispatchWidget) HandlePointer(_ *model.Node, rt *runtime.Runtime, p PointerInput, _ *Interaction, _ image.Rectangle) bool {
	if p.Type == PointerPress {
		rt.Dispatch("widget-action", nil) // deliberately bypass Engine.dispatch
		return true
	}
	return false
}

func measuredIDs(t *testing.T, e *Engine) map[string]bool {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(e.CollectMeasure(), &rows); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			out[id] = true
		}
	}
	return out
}

func TestQueuedRuntimeSwapTakesEffectBeforeFrameSnapshot(t *testing.T) {
	oldRoot := &model.Node{Type: "column", ID: "old-root"}
	oldRT := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": oldRoot}})
	newRoot := &model.Node{Type: "column", ID: "new-root", Children: []*model.Node{{Type: "text", ID: "fresh", Text: "new"}}}
	newRT := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": newRoot}})
	e := NewEngine(oldRT, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(200, 100))
	e.DrawFrame(surf)
	e.Inter.Focused = oldRoot

	done := make(chan struct{})
	go func() { e.EnqueueMutation(func() { e.SetRuntime(newRT) }); close(done) }()
	deadline := time.Now().Add(time.Second)
	for !e.Dirty() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.DrawFrame(surf)
	<-done
	ids := measuredIDs(t, e)
	if !ids["new-root"] || !ids["fresh"] || ids["old-root"] {
		t.Fatalf("runtime swap measure ids = %v", ids)
	}
	if e.Inter.Focused != nil {
		t.Fatal("new scene root must clear model-pointer interaction")
	}
}

func TestSetRuntimeSamePointerPreservesInteraction(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)
	e.Inter.Focused = btn
	e.SetRuntime(e.RT)
	e.DrawFrame(surf)
	if e.Inter.Focused != btn {
		t.Fatal("same-runtime state notification reset interaction")
	}
}

// engineFixture builds a headless engine around one centered button.
func engineFixture(t *testing.T) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	btn := &model.Node{Type: "button", ID: "b1", Props: map[string]any{"label": "Hi"}}
	root := &model.Node{
		Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{btn},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, btn
}

func buttonCenter(t *testing.T, e *Engine, btn *model.Node) (int, int) {
	t.Helper()
	g := e.findGroupByModel(btn)
	if g == nil {
		t.Fatal("button group not found in rendered graph")
	}
	bb := g.GetBBox()
	return int((bb.MinX + bb.MaxX) / 2), int((bb.MinY + bb.MaxY) / 2)
}

// buttonTextFreePoint returns a point inside the button clear of its label
// glyphs (8px in from the left edge, mid-height). Pixel assertions must not
// sample the label: under the default sfnt text engine, glyph antialiasing
// makes those pixels neither the plain nor the pressed background.
func buttonTextFreePoint(t *testing.T, e *Engine, btn *model.Node) (int, int) {
	t.Helper()
	g := e.findGroupByModel(btn)
	if g == nil {
		t.Fatal("button group not found in rendered graph")
	}
	bb := g.GetBBox()
	return int(bb.MinX) + 8, int((bb.MinY + bb.MaxY) / 2)
}

// The default theme paints buttons with the primary color; pressing must
// switch pixels to the theme's pressedBackgroundColor (#0062CC) and release
// must restore — the whole feedback chain verified at the pixel level, no
// window required.
func TestEnginePressFeedbackPixels(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)

	cx, cy := buttonCenter(t, e, btn)
	px, py := buttonTextFreePoint(t, e, btn)
	primary := color.RGBA{0x00, 0x7A, 0xFF, 255} // #007AFF
	// The press overlay sets #0062CC at pressedOpacity 0.9, which the rasterizer
	// alpha-composites over the white scene background → (26,114,209,255).
	pressed := color.RGBA{26, 114, 209, 255}

	if got := surf.Frame().RGBAAt(px, py); got != primary {
		t.Fatalf("normal button pixel = %v, want primary %v", got, primary)
	}

	if !e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)}) {
		t.Fatal("press must request a redraw")
	}
	if e.Inter.Pressed != btn {
		t.Fatal("press must set the pressed identity")
	}
	e.DrawFrame(surf)
	if got := surf.Frame().RGBAAt(px, py); got != pressed {
		t.Fatalf("pressed button pixel = %v, want %v", got, pressed)
	}

	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	e.DrawFrame(surf)
	if got := surf.Frame().RGBAAt(px, py); got != primary {
		t.Fatalf("released button pixel = %v, want primary again", got)
	}
	if surf.Presents != 3 {
		t.Errorf("presents = %d, want 3", surf.Presents)
	}
}

// Hover must swap to hoveredBackgroundColor (#1A86FF) and revert on leave.
func TestEngineHoverPixels(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)
	cx, cy := buttonCenter(t, e, btn)
	px, py := buttonTextFreePoint(t, e, btn)
	hovered := color.RGBA{0x1A, 0x86, 0xFF, 255}

	e.HandlePointer(PointerInput{Type: PointerMove, X: float64(cx), Y: float64(cy)})
	e.DrawFrame(surf)
	if got := surf.Frame().RGBAAt(px, py); got != hovered {
		t.Fatalf("hovered pixel = %v, want %v", got, hovered)
	}

	e.HandlePointer(PointerInput{Type: PointerMove, X: 5, Y: 5})
	e.DrawFrame(surf)
	if got := surf.Frame().RGBAAt(px, py); got == hovered {
		t.Fatal("hover must revert when the pointer leaves")
	}
}

// Tab must draw the keyboard focus ring (theme focus color) offset outside
// the button; a pointer click moves focus WITHOUT a ring.
func TestEngineFocusRingPixels(t *testing.T) {
	focus := color.RGBA{0x00, 0x7A, 0xFF, 255} // #007AFF
	ringPixels := func(e *Engine, surf *HeadlessSurface, btn *model.Node) int {
		g := e.findGroupByModel(btn)
		bb := g.GetBBox()
		ringY := int(bb.MinY) - 3 // the ring's top edge, outside the button body
		n := 0
		for x := int(bb.MinX); x < int(bb.MaxX); x++ {
			if surf.Frame().RGBAAt(x, ringY) == focus {
				n++
			}
		}
		return n
	}

	// Keyboard focus → ring visible.
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)
	if !e.HandleKey(KeyInput{Key: "tab", Down: true}) {
		t.Fatal("tab must request a redraw")
	}
	e.DrawFrame(surf)
	if n := ringPixels(e, surf, btn); n == 0 {
		t.Fatal("keyboard focus must draw the ring")
	}

	// Pointer focus → no ring (focus-visible semantics).
	e2, surf2, btn2 := engineFixture(t)
	e2.DrawFrame(surf2)
	cx, cy := buttonCenter(t, e2, btn2)
	e2.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e2.DrawFrame(surf2)
	if e2.Inter.Focused != btn2 || e2.Inter.FocusVisible {
		t.Fatal("pointer press must focus without a visible ring")
	}
	if n := ringPixels(e2, surf2, btn2); n != 0 {
		t.Fatalf("pointer focus drew %d ring pixels, want 0", n)
	}
}

// Enter activates the focused node's OnPress.
func TestEngineEnterActivatesFocused(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.RT.App.Actions = map[string]*model.Action{
		"ping": {ID: "ping", Steps: []model.Step{{Type: "state.set", Path: "pinged", Value: "{{ 'yes' }}"}}},
	}
	btn.OnPress = &model.Invoke{Name: "ping"}

	e.DrawFrame(surf)
	e.HandleKey(KeyInput{Key: "tab", Down: true})
	e.HandleKey(KeyInput{Key: "return", Down: true})
	if v := e.RT.State["pinged"]; v != "yes" {
		t.Fatalf("Enter on focused button did not dispatch OnPress (state=%v)", v)
	}
}

func TestHumanActionSinkAttributesNativeInputOnly(t *testing.T) {
	t.Run("pointer keyboard and disabled", func(t *testing.T) {
		e, surf, btn := engineFixture(t)
		e.RT.App.Actions = map[string]*model.Action{
			"tap": {ID: "tap"}, "key": {ID: "key"}, "timer": {ID: "timer"},
		}
		btn.OnPress = &model.Invoke{Name: "tap"}
		var got []string
		e.SetHumanActionSink(func(action string) { got = append(got, action) })
		e.DrawFrame(surf)
		cx, cy := buttonCenter(t, e, btn)
		e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
		if fmt.Sprint(got) != "[tap]" {
			t.Fatalf("pointer activity = %v, want [tap]", got)
		}

		// A non-input dispatch (timer/physics/onComplete class) is not human.
		e.RT.Dispatch("timer", nil)
		if fmt.Sprint(got) != "[tap]" {
			t.Fatalf("programmatic dispatch was attributed to human: %v", got)
		}

		btn.OnPress = &model.Invoke{Name: "key"}
		e.Inter.Focused = btn
		e.HandleKey(KeyInput{Key: "return", Down: true})
		if fmt.Sprint(got) != "[tap key]" {
			t.Fatalf("keyboard activity = %v, want [tap key]", got)
		}

		btn.Style = map[string]any{"disabled": true}
		e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
		if fmt.Sprint(got) != "[tap key]" {
			t.Fatalf("disabled input emitted activity: %v", got)
		}
	})

	t.Run("interactive widget direct runtime dispatch", func(t *testing.T) {
		const typ = "observer-dispatch-widget"
		RegisterWidget(typ, observerDispatchWidget{})
		defer UnregisterWidget(typ)
		n := &model.Node{Type: typ, ID: "w", Props: map[string]any{"class": "off"}}
		rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", Children: []*model.Node{n}}}, Actions: map[string]*model.Action{"widget-action": {ID: "widget-action"}}})
		e := NewEngine(rt, SoftwareRenderer{})
		surf := NewHeadlessSurface(image.Pt(100, 100))
		var got []string
		e.SetHumanActionSink(func(action string) { got = append(got, action) })
		e.DrawFrame(surf)
		e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})
		if fmt.Sprint(got) != "[widget-action]" {
			t.Fatalf("direct widget dispatch activity = %v", got)
		}

		rt.App.Styles = []model.StyleRule{{Kind: model.StyleRuleClass, Name: "off", Style: map[string]any{"disabled": true}}}
		got = nil
		e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})
		if len(got) != 0 {
			t.Fatalf("QSS-disabled interactive widget dispatched/logged: %v", got)
		}
	})
}

// onKeyDown on the focused node receives the pressed key: the engine seeds
// the "key" arg, which action expressions read as a bare scope name.
func TestEngineKeyDownDispatch(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.RT.App.Actions = map[string]*model.Action{
		"onkey": {ID: "onkey", Steps: []model.Step{{Type: "state.set", Path: "lastKey", Value: "{{ key }}"}}},
	}
	btn.OnKeyDown = &model.Invoke{Name: "onkey"}

	e.DrawFrame(surf)
	e.HandleKey(KeyInput{Key: "tab", Down: true}) // focus the button
	e.HandleKey(KeyInput{Key: "a", Down: true})
	if v := e.RT.State["lastKey"]; v != "a" {
		t.Fatalf("onKeyDown key = %v, want \"a\"", v)
	}
}

// OS key-repeat is extra KeyDown with no KeyUp. Scene bindings must fire once.
func TestHandleKeyIgnoresRepeat(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root"}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		SceneKeys: map[string]map[string]string{
			"main": {"r": "bump"},
		},
		SceneKeyReleases: map[string]map[string]string{
			"main": {"r": "noop"},
		},
		Actions: map[string]*model.Action{
			"bump": {ID: "bump", Script: "state.n = state.n + 1"},
			"noop": {ID: "noop", Script: ""},
		},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["n"] = 0.0
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)
	e.HandleKey(KeyInput{Key: "r", Down: true})
	e.HandleKey(KeyInput{Key: "r", Down: true}) // OS repeat
	e.HandleKey(KeyInput{Key: "r", Down: true})
	if got := rt.State["n"]; got != 1.0 {
		t.Fatalf("keydown+repeats n = %v, want 1 (repeat swallowed)", got)
	}
	e.HandleKey(KeyInput{Key: "r", Down: false})
	e.HandleKey(KeyInput{Key: "r", Down: true})
	if got := rt.State["n"]; got != 2.0 {
		t.Fatalf("after release+press n = %v, want 2", got)
	}
}

// At device scale 2 the same logical design must occupy twice the physical
// pixels: the layout DPI plumbing multiplies design pixels by Scale() while
// the surface reports physical dimensions. (Scale 1 must stay bit-identical.)
func TestEngineHiDPIScalesGeometry(t *testing.T) {
	// build lays the SAME 200x200 logical design into a physical buffer of
	// 200*scale, so scale 2 must produce 2x the geometry of scale 1.
	build := func(scale int) (*Engine, *HeadlessSurface, *model.Node) {
		btn := newButton("b")
		root := &model.Node{Type: "column", ID: "root",
			Layout:   map[string]any{"align": "center", "justify": "center"},
			Children: []*model.Node{btn}}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
		rt := runtime.New(app)
		rt.Theme = theme.GetDefault()
		phys := 200 * scale
		surf := NewHeadlessSurface(image.Pt(phys, phys)) // physical buffer
		surf.Logical = image.Pt(200, 200)                // logical layout size
		surf.ScaleFactor = scale
		return NewEngine(rt, SoftwareRenderer{}), surf, btn
	}
	widthOf := func(e *Engine, surf *HeadlessSurface, btn *model.Node) float64 {
		e.DrawFrame(surf)
		bb := e.findGroupByModel(btn).GetBBox()
		return bb.MaxX - bb.MinX
	}

	e1, s1, btn1 := build(1)
	w1 := widthOf(e1, s1, btn1)
	e2, s2, btn2 := build(2)
	w2 := widthOf(e2, s2, btn2)

	if w1 <= 0 {
		t.Fatalf("scale-1 button width = %v", w1)
	}
	if got, want := w2, w1*2; got != want {
		t.Errorf("scale-2 button width = %v, want %v (2× scale-1)", got, want)
	}
}

// The display list must be rebuilt each frame — drawing the same scene twice
// yields identical pixels (no accumulation of stale ops).
func TestEngineDisplayListRebuiltEachFrame(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame(surf)
	first := image.NewRGBA(surf.Frame().Rect)
	copy(first.Pix, surf.Frame().Pix)
	e.MarkDirty()
	e.DrawFrame(surf)
	for i := range first.Pix {
		if first.Pix[i] != surf.Frame().Pix[i] {
			t.Fatalf("frame differs at byte %d — display list accumulation?", i)
		}
	}
}

// Idle frames (no input or state change) must be skipped: a second DrawFrame
// without MarkDirty renders nothing, presents nothing and reports zero stats.
func TestEngineSkipsIdleFrames(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame(surf)
	if surf.Presents != 1 {
		t.Fatalf("first frame presents = %d, want 1", surf.Presents)
	}

	st := e.DrawFrame(surf)
	if surf.Presents != 1 {
		t.Fatalf("idle frame presented (presents = %d), want skip", surf.Presents)
	}
	if st.Total != 0 {
		t.Errorf("idle frame stats = %+v, want zero value", st)
	}
}

// MarkDirty forces the next frame to render even without input.
func TestEngineMarkDirtyRenders(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame(surf)
	e.DrawFrame(surf) // idle → skipped
	e.MarkDirty()
	e.DrawFrame(surf)
	if surf.Presents != 2 {
		t.Fatalf("presents = %d, want 2 (MarkDirty must drive a render)", surf.Presents)
	}
}

// Input handlers dirty the engine themselves: the next DrawFrame renders
// without the host ever calling MarkDirty.
func TestEngineHandlerSetsDirty(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)
	if surf.Presents != 1 {
		t.Fatalf("first frame presents = %d, want 1", surf.Presents)
	}
	cx, cy := buttonCenter(t, e, btn)
	if !e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)}) {
		t.Fatal("press must request a redraw")
	}
	e.DrawFrame(surf)
	if surf.Presents != 2 {
		t.Fatalf("presents = %d, want 2 (the handler's dirty flag must drive the render)", surf.Presents)
	}
}

// Regression (red-team R1 P0-2 / R2 P0-1): an external dispatch (HTTP/MCP
// goroutine) mutating rt.State concurrently with RenderInto was a cross-thread
// map read/write — fatal at runtime, flagged by -race. The mutation queue
// serializes external work onto the render thread: run under -race, this must
// stay clean AND every queued mutation must land exactly once.
func TestEngineEnqueueMutationSerializesExternalDispatch(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.RT.App.Actions = map[string]*model.Action{
		"inc": {ID: "inc", Steps: []model.Step{{Type: "state.set", Path: "count", Value: "{{ count + 1 }}"}}},
	}
	e.RT.State["count"] = 0

	const mutations = 100
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < mutations; i++ {
			e.EnqueueMutation(func() { e.RT.Dispatch("inc", nil) })
		}
	}()

	// The render thread: keep framing until every queued mutation has been
	// applied (the last EnqueueMutation returns only after its closure ran).
	for {
		e.DrawFrame(surf)
		select {
		case <-done:
			if got := fmt.Sprint(e.RT.State["count"]); got != "100" {
				t.Fatalf("count = %v, want %d — every queued mutation must run exactly once", got, mutations)
			}
			return
		default:
		}
	}
}

// EnqueueMutation preserves request/response semantics: it blocks the caller
// until the closure ran at a frame boundary, and the closure's effect is
// visible to it afterwards.
func TestEngineEnqueueMutationBlocksUntilApplied(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame(surf) // first frame renders; engine goes idle

	applied := make(chan int, 1)
	go func() {
		e.EnqueueMutation(func() {
			e.RT.State["answer"] = 42
			applied <- 1
		})
	}()

	// Wait until the mutation is actually queued (dirty is stored after the
	// queue append), otherwise the frame below could run before the goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for !e.Dirty() {
		if time.Now().After(deadline) {
			t.Fatal("mutation was not queued")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-applied:
		t.Fatal("mutation applied before any frame ran — the queue must drain at a frame boundary")
	default:
	}
	e.DrawFrame(surf) // frame boundary: the queued mutation drains here
	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("EnqueueMutation did not return after the frame drained its mutation")
	}
	if got := e.RT.State["answer"]; got != 42 {
		t.Fatalf("answer = %v, want 42", got)
	}
}

// Regression (red-team R1 P1-2): the physics pass dispatches a collision
// action AFTER layout; its render step calls Commit → RequestDraw mid-frame.
// The old tail-side dirty.Store(false) ate that flag, so the frame carrying
// the collision state was never presented. The next frame must render.
func TestPhysicsCollisionStateRendersNextFrame(t *testing.T) {
	// Two boxes overlapping via a negative margin, both with OnCollide.
	mk := func(id string, marginTop float64) *model.Node {
		return &model.Node{
			Type: "button", ID: id, Props: map[string]any{"label": id},
			Style:     map[string]any{"width": 100, "height": 100, "margin": map[string]any{"top": marginTop}},
			OnCollide: &model.Invoke{Name: "collide"},
		}
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{mk("a", 0), mk("b", -50)}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	app.Actions = map[string]*model.Action{
		"collide": {ID: "collide", Steps: []model.Step{
			{Type: "state.set", Path: "collided", Value: "{{ 'yes' }}"},
			{Type: "render"},
		}},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	// Faithful to the server wiring: a render step's Commit asks the engine
	// for a new frame.
	rt.Commit = e.RequestDraw
	surf := NewHeadlessSurface(image.Pt(400, 400))

	e.DrawFrame(surf)
	if got := rt.State["collided"]; got != "yes" {
		t.Fatalf("collision did not dispatch mid-frame (collided=%v)", got)
	}
	if surf.Presents != 1 {
		t.Fatalf("presents after frame 1 = %d, want 1", surf.Presents)
	}

	// The mid-frame RequestDraw must survive to drive frame 2 — the frame that
	// actually carries collided=yes. The lost-frame bug kept this at 1.
	e.DrawFrame(surf)
	if surf.Presents != 2 {
		t.Fatalf("presents after frame 2 = %d, want 2 — the frame carrying the collision state was lost", surf.Presents)
	}
}

// Regression (red-team R2 P1-1): state.theme is writable by actions/MCP, and
// was spliced straight into a file path — "../../evil" loaded arbitrary JSON
// as a skin. Traversal names must be rejected without touching the disk.
func TestResolveThemeRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	// The payload a traversal would reach: themes/../../evil resolves outside
	// the themes dir. If the name is sanitized, this file is never read.
	evil := []byte(`{"name":"PWNED","colors":{}}`)
	if err := os.WriteFile(filepath.Join(dir, "evil.json"), evil, 0o644); err != nil {
		t.Fatal(err)
	}

	e, surf, _ := engineFixture(t)
	e.RT.App.BaseDir = dir
	e.RT.State["theme"] = "../../evil"
	e.DrawFrame(surf)

	if e.RT.Theme != nil && e.RT.Theme.Name == "PWNED" {
		t.Fatal("path traversal: engine loaded a theme outside the themes directory")
	}
	if e.themeFailed != "../../evil" {
		t.Errorf("themeFailed = %q, want the rejected name cached", e.themeFailed)
	}
	if e.RT.Theme == nil || e.RT.Theme.Name != "apple-light" {
		t.Errorf("theme after rejection = %v, want the default kept", e.RT.Theme)
	}
}

// Regression (red-team R2 P1-1): a theme that fails to load must be
// negatively cached — logged once, not retried every frame — and themes must
// resolve against the app's own themes/ directory so a cwd without one no
// longer silently kills theme switching.
func TestResolveThemeAppDirAndNegativeCache(t *testing.T) {
	dir := t.TempDir()
	e, surf, _ := engineFixture(t)
	e.RT.App.BaseDir = dir
	e.RT.State["theme"] = "apple-dark"

	// Frame 1: the file does not exist yet → failure, negatively cached.
	e.DrawFrame(surf)
	if e.themeFailed != "apple-dark" {
		t.Fatalf("themeFailed = %q, want \"apple-dark\" after the failed load", e.themeFailed)
	}

	// The skin appears on disk now, but the cached failure must NOT be
	// retried while state.theme keeps the same name (that retry was the
	// per-frame disk + stderr spin).
	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skin := []byte(`{"name":"apple-dark","colors":{"primary":"#000000"}}`)
	if err := os.WriteFile(filepath.Join(themesDir, "apple-dark.json"), skin, 0o644); err != nil {
		t.Fatal(err)
	}
	e.MarkDirty()
	e.DrawFrame(surf)
	if e.RT.Theme.Name == "apple-dark" {
		t.Fatal("failed theme was retried while state.theme was unchanged — negative cache broken")
	}

	// A name change clears the cache: switching away and back loads the skin —
	// from the APP's themes dir, with the cwd (this package) having none.
	e.RT.State["theme"] = "win11-dark"
	e.MarkDirty()
	e.DrawFrame(surf)
	e.RT.State["theme"] = "apple-dark"
	e.MarkDirty()
	e.DrawFrame(surf)
	if e.RT.Theme.Name != "apple-dark" {
		t.Fatalf("theme = %q, want \"apple-dark\" loaded from the app dir after the name changed", e.RT.Theme.Name)
	}
	if got := e.RT.Theme.ParsedColors["primary"]; got.A != 255 {
		t.Errorf("loaded theme primary = %v, want the app-dir skin's color", got)
	}
}

// ScrollInput is routed into the engine instead of being silently dropped by
// the host. With scroll containers landed (scroll.go), a gesture can only
// take effect where the pointer rests over a scroll viewport — and this
// fixture has neither a known pointer position nor a viewport, so the handler
// must stay a no-op: no dirty flag, no interaction change.
func TestEngineHandleScrollIsExplicitNoOp(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame(surf)
	if e.HandleScroll(ScrollInput{DX: 0, DY: -24}) {
		t.Error("HandleScroll must report no visual change until scroll containers exist")
	}
	if e.Dirty() {
		t.Error("HandleScroll must not dirty the engine")
	}
}
