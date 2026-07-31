package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

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
	return NewEngine(rt, SoftwareRenderer{}, surf), surf, btn
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

// The default theme paints buttons with the primary color; pressing must
// switch pixels to the theme's pressedBackgroundColor (#0062CC) and release
// must restore — the whole feedback chain verified at the pixel level, no
// window required.
func TestEnginePressFeedbackPixels(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame()

	cx, cy := buttonCenter(t, e, btn)
	primary := color.RGBA{0x00, 0x7A, 0xFF, 255} // #007AFF
	// #0062CC (theme pressedBackgroundColor) at the theme's pressedOpacity
	// 0.9 → alpha 229: the press overlay covers color AND opacity.
	pressed := color.RGBA{0x00, 0x62, 0xCC, 229}

	if got := surf.Frame().RGBAAt(cx, cy); got != primary {
		t.Fatalf("normal button pixel = %v, want primary %v", got, primary)
	}

	if !e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)}) {
		t.Fatal("press must request a redraw")
	}
	if e.Inter.Pressed != btn {
		t.Fatal("press must set the pressed identity")
	}
	e.DrawFrame()
	if got := surf.Frame().RGBAAt(cx, cy); got != pressed {
		t.Fatalf("pressed button pixel = %v, want %v", got, pressed)
	}

	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	e.DrawFrame()
	if got := surf.Frame().RGBAAt(cx, cy); got != primary {
		t.Fatalf("released button pixel = %v, want primary again", got)
	}
	if surf.Presents != 3 {
		t.Errorf("presents = %d, want 3", surf.Presents)
	}
}

// Hover must swap to hoveredBackgroundColor (#1A86FF) and revert on leave.
func TestEngineHoverPixels(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame()
	cx, cy := buttonCenter(t, e, btn)
	hovered := color.RGBA{0x1A, 0x86, 0xFF, 255}

	e.HandlePointer(PointerInput{Type: PointerMove, X: float64(cx), Y: float64(cy)})
	e.DrawFrame()
	if got := surf.Frame().RGBAAt(cx, cy); got != hovered {
		t.Fatalf("hovered pixel = %v, want %v", got, hovered)
	}

	e.HandlePointer(PointerInput{Type: PointerMove, X: 5, Y: 5})
	e.DrawFrame()
	if got := surf.Frame().RGBAAt(cx, cy); got == hovered {
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
	e.DrawFrame()
	if !e.HandleKey(KeyInput{Key: "tab", Down: true}) {
		t.Fatal("tab must request a redraw")
	}
	e.DrawFrame()
	if n := ringPixels(e, surf, btn); n == 0 {
		t.Fatal("keyboard focus must draw the ring")
	}

	// Pointer focus → no ring (focus-visible semantics).
	e2, surf2, btn2 := engineFixture(t)
	e2.DrawFrame()
	cx, cy := buttonCenter(t, e2, btn2)
	e2.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e2.DrawFrame()
	if e2.Inter.Focused != btn2 || e2.Inter.FocusVisible {
		t.Fatal("pointer press must focus without a visible ring")
	}
	if n := ringPixels(e2, surf2, btn2); n != 0 {
		t.Fatalf("pointer focus drew %d ring pixels, want 0", n)
	}
}

// Enter activates the focused node's OnPress.
func TestEngineEnterActivatesFocused(t *testing.T) {
	e, _, btn := engineFixture(t)
	e.RT.App.Actions = map[string]*model.Action{
		"ping": {ID: "ping", Steps: []model.Step{{Type: "state.set", Path: "pinged", Value: "{{ 'yes' }}"}}},
	}
	btn.OnPress = &model.Invoke{Name: "ping"}

	e.DrawFrame()
	e.HandleKey(KeyInput{Key: "tab", Down: true})
	e.HandleKey(KeyInput{Key: "return", Down: true})
	if v := e.RT.State["pinged"]; v != "yes" {
		t.Fatalf("Enter on focused button did not dispatch OnPress (state=%v)", v)
	}
}

// onKeyDown on the focused node receives the pressed key: the engine seeds
// the "key" arg, which action expressions read as a bare scope name.
func TestEngineKeyDownDispatch(t *testing.T) {
	e, _, btn := engineFixture(t)
	e.RT.App.Actions = map[string]*model.Action{
		"onkey": {ID: "onkey", Steps: []model.Step{{Type: "state.set", Path: "lastKey", Value: "{{ key }}"}}},
	}
	btn.OnKeyDown = &model.Invoke{Name: "onkey"}

	e.DrawFrame()
	e.HandleKey(KeyInput{Key: "tab", Down: true}) // focus the button
	e.HandleKey(KeyInput{Key: "a", Down: true})
	if v := e.RT.State["lastKey"]; v != "a" {
		t.Fatalf("onKeyDown key = %v, want \"a\"", v)
	}
}

// The display list must be rebuilt each frame — drawing the same scene twice
// yields identical pixels (no accumulation of stale ops).
func TestEngineDisplayListRebuiltEachFrame(t *testing.T) {
	e, surf, _ := engineFixture(t)
	e.DrawFrame()
	first := image.NewRGBA(surf.Frame().Rect)
	copy(first.Pix, surf.Frame().Pix)
	e.DrawFrame()
	for i := range first.Pix {
		if first.Pix[i] != surf.Frame().Pix[i] {
			t.Fatalf("frame differs at byte %d — display list accumulation?", i)
		}
	}
}
