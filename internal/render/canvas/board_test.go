package canvas

import (
	"image"
	"math"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// boardFixture builds a headless engine around a "board" scene: two notes
// absolutely placed at board (100,50) and (300,200), each carrying touch
// handlers so a press on a note drags the NOTE rather than panning the board.
// A bare background style keeps the board's fixed window-sized frame visible.
func boardFixture(t *testing.T) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	note := func(id string, x, y float64) *model.Node {
		return &model.Node{Type: "box", ID: id,
			Style: map[string]any{
				"x": x, "y": y, "width": 60.0, "height": 40.0, "background": "#FFCC00",
			},
			OnTouchStart: &model.Invoke{Name: "touchstart"},
			OnTouchMove:  &model.Invoke{Name: "touchmove"},
			OnTouchEnd:   &model.Invoke{Name: "touchend"},
		}
	}
	root := &model.Node{Type: "board", ID: "board",
		Style:    map[string]any{"background": "#EEEEEE"},
		Children: []*model.Node{note("n1", 100, 50), note("n2", 300, 200)}}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"touchstart": {Steps: []model.Step{{Type: "state.set", Path: "touched", Value: "true"}}},
			"touchmove":  {Steps: []model.Step{{Type: "state.set", Path: "moved", Value: "true"}}},
			"touchend":   {Steps: []model.Step{{Type: "state.set", Path: "ended", Value: "true"}}},
		},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, root
}

// A board scene activates the interaction sidecar, spans the viewport in BOTH
// axes (its children are out of flow), and applies pan/zoom to the content so
// a note's screen box follows the content matrix exactly.
func TestBoardViewportSpansAndTransforms(t *testing.T) {
	e, s, root := boardFixture(t)
	e.DrawFrame(s)
	if !e.Inter.Board.Active {
		t.Fatal("board scene must mark the interaction sidecar active")
	}
	if e.Inter.Board.Zoom != 1 {
		t.Errorf("default board zoom = %v, want 1", e.Inter.Board.Zoom)
	}
	groot := e.findGroupByModel(root)
	if groot == nil {
		t.Fatal("board root group missing")
	}
	if groot.Base().Width != 400 || groot.Base().Height != 400 {
		t.Errorf("board root = %vx%v, want the 400x400 viewport", groot.Base().Width, groot.Base().Height)
	}

	// Pan (10,5) + zoom 2: note at board (100,50) size 60x40 must land at
	// screen (10+2*100, 5+2*50)=(210,105) sized 120x80 — the rasterizer and
	// hit testing share this matrix (graph.GlobalTransform). Real gestures
	// reach this state through HandlePointer/HandleScroll, which mark the
	// engine dirty; a direct sidecar write needs an explicit nudge.
	e.Inter.Board.PanX, e.Inter.Board.PanY = 10, 5
	e.Inter.Board.Zoom = 2
	e.MarkDirty()
	e.DrawFrame(s)
	gn := e.findGroupByModel(root.Children[0])
	if gn == nil {
		t.Fatal("note group missing after zoom")
	}
	bb := gn.GetBBox()
	if int(bb.MinX) != 210 || int(bb.MinY) != 105 {
		t.Errorf("note screen origin = (%v,%v), want (210,105)", bb.MinX, bb.MinY)
	}
	if bb.MaxX-bb.MinX != 120 || bb.MaxY-bb.MinY != 80 {
		t.Errorf("note screen size = %vx%v, want 120x80", bb.MaxX-bb.MinX, bb.MaxY-bb.MinY)
	}
}

// A press on empty board space starts a pan and the canvas follows the pointer
// 1:1; release ends it.
func TestBoardBlankDragPans(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	if !e.HandlePointer(PointerInput{Type: PointerPress, X: 300, Y: 300, Buttons: 1}) {
		t.Fatal("blank press on a board must be handled")
	}
	if !e.Inter.Board.Panning {
		t.Fatal("blank press must start a pan")
	}
	e.HandlePointer(PointerInput{Type: PointerMove, X: 330, Y: 320, Buttons: 1})
	if e.Inter.Board.PanX != 30 || e.Inter.Board.PanY != 20 {
		t.Errorf("after drag pan = (%v,%v), want (30,20)", e.Inter.Board.PanX, e.Inter.Board.PanY)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 330, Y: 320})
	if e.Inter.Board.Panning {
		t.Error("release must end the pan")
	}
}

// A cancelled drag must not leave the board panning: a button-less move mid-pan
// clears the drag state, so the next note press+move drags the NOTE instead of
// resuming a stale pan from a dead PanStart.
func TestBoardCancelledDragReleasesPan(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	e.HandlePointer(PointerInput{Type: PointerPress, X: 300, Y: 300, Buttons: 1})
	if !e.Inter.Board.Panning {
		t.Fatal("blank press must start a pan")
	}
	// A button-less move mid-pan (drag cancelled/lost): must drop Panning.
	e.HandlePointer(PointerInput{Type: PointerMove, X: 310, Y: 310})
	if e.Inter.Board.Panning {
		t.Fatal("button-less move must cancel the pan")
	}
	// A subsequent note press+move drags the note, not the board.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 130, Y: 70, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 150, Y: 80, Buttons: 1})
	if e.Inter.Board.Panning {
		t.Fatal("note press must not start a pan")
	}
	if e.Inter.Board.PanX != 0 || e.Inter.Board.PanY != 0 {
		t.Errorf("stale pan resumed: board pan = (%v,%v), want (0,0)", e.Inter.Board.PanX, e.Inter.Board.PanY)
	}
}

// A press on a note must NOT pan the board — the note's own touch handler
// fires instead (its onTouchStart drags the note, not the canvas).
func TestBoardNotePressDoesNotPan(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	// Middle of n1: board (100,50) size 60x40, zoom 1, no pan.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 130, Y: 70, Buttons: 1})
	if e.Inter.Board.Panning {
		t.Fatal("press on a note must not pan the board")
	}
	if e.Inter.Board.PanX != 0 || e.Inter.Board.PanY != 0 {
		t.Errorf("note press moved the board: pan = (%v,%v)", e.Inter.Board.PanX, e.Inter.Board.PanY)
	}
	if v := e.RT.State["touched"]; v != "true" {
		t.Errorf("note onTouchStart not dispatched: state.touched = %v", v)
	}
}

// Control-modified scroll (trackpad pinch / Ctrl+wheel) zooms the board about
// the cursor so the content under it stays fixed (zoom-to-cursor).
func TestBoardCtrlScrollZoomsAtCursor(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	e.Inter.Board.PanX, e.Inter.Board.PanY = 100, 100
	e.HandlePointer(PointerInput{Type: PointerMove, X: 200, Y: 200})
	if !e.HandleScroll(ScrollInput{DY: 120, Ctrl: true}) {
		t.Fatal("ctrl scroll on a board must zoom")
	}
	if z := e.Inter.Board.Zoom; z < 1.09 || z > 1.11 {
		t.Errorf("zoom = %v, want ~1.1 (one 120px notch)", z)
	}
	// pan' = C − (C − pan)·ratio = 200 − 100·1.1 = 90 on both axes.
	wantPan := 200 - (200-100)*1.1
	if math.Abs(e.Inter.Board.PanX-wantPan) > 1e-6 || math.Abs(e.Inter.Board.PanY-wantPan) > 1e-6 {
		t.Errorf("pan after zoom = (%v,%v), want (%v,%v)", e.Inter.Board.PanX, e.Inter.Board.PanY, wantPan, wantPan)
	}
}

// An unconsumed wheel/trackpad scroll over a board pans the canvas instead of
// doing nothing; it must not change the zoom.
func TestBoardPlainScrollPans(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	e.HandlePointer(PointerInput{Type: PointerMove, X: 200, Y: 200})
	if !e.HandleScroll(ScrollInput{DY: 50}) {
		t.Fatal("scroll over a board must pan")
	}
	if e.Inter.Board.PanY != -50 {
		t.Errorf("panY = %v, want -50 (positive dy scrolls toward content bottom)", e.Inter.Board.PanY)
	}
	if e.Inter.Board.Zoom != 1 {
		t.Errorf("plain scroll must not zoom; zoom = %v", e.Inter.Board.Zoom)
	}
	// A horizontal-only swipe pans X even though the scroll-viewport walk only
	// consumes vertical deltas.
	if !e.HandleScroll(ScrollInput{DX: 30}) {
		t.Fatal("horizontal scroll over a board must pan")
	}
	if e.Inter.Board.PanX != -30 {
		t.Errorf("panX after horizontal scroll = %v, want -30", e.Inter.Board.PanX)
	}
}

// Off-screen board children are culled at record time: no graph subtree, no
// raster. A note beyond the viewport edge (and its pan/zoom) simply isn't in
// the graph; panning it into view builds it again.
func TestBoardCullsOffscreenChildren(t *testing.T) {
	e, s, root := boardFixture(t)
	e.DrawFrame(s)

	// n1 at board (100,50) is visible; n2 at (300,200) too. Add an implicit
	// check that a note far outside the viewport is absent from the graph.
	if g := e.findGroupByModel(root.Children[0]); g == nil {
		t.Fatal("on-screen note must be in the graph")
	}
	if g := e.findGroupByModel(root.Children[1]); g == nil {
		t.Fatal("second on-screen note must be in the graph")
	}

	// Pan far enough that n1 (at board 100,50, 60x40) leaves the 400x400
	// viewport; its group must vanish from the graph.
	e.Inter.Board.PanX, e.Inter.Board.PanY = -500, -500
	e.MarkDirty()
	e.DrawFrame(s)
	if g := e.findGroupByModel(root.Children[0]); g != nil {
		t.Fatalf("off-screen note must be culled, but a group exists at (%v,%v)", g.Base().X, g.Base().Y)
	}

	// Pan it back: the note reappears.
	e.Inter.Board.PanX, e.Inter.Board.PanY = 0, 0
	e.MarkDirty()
	e.DrawFrame(s)
	if g := e.findGroupByModel(root.Children[0]); g == nil {
		t.Fatal("note must reappear after panning it back into view")
	}
}

// Follow-cam / drag pan is a transform, not a layout-box move. Dirty
// tracking that only unions AbsX would leave the world frozen while the
// sprite slides — the hitch Mario showed when walking.
func TestBoardPanRepaintsWorldPixels(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	// n1 is yellow at board (100,50) 60x40. Sample its interior.
	before := s.Frame().RGBAAt(120, 70)
	if before.R < 200 || before.G < 180 {
		t.Fatalf("expected yellow note at (120,70), got %v", before)
	}
	e.Inter.Board.PanX = -80
	e.MarkDirty()
	e.DrawFrame(s)
	after := s.Frame().RGBAAt(120, 70)
	if after == before {
		t.Fatal("after pan the old note pixel must be cleared (full-frame dirty)")
	}
	moved := s.Frame().RGBAAt(40, 70) // 120-80
	if moved.R < 200 || moved.G < 180 {
		t.Fatalf("note should now sit at (40,70), got %v", moved)
	}
}

// A board list with absolute x/y items only records instances inside the
// camera frustum. Far tiles stay in state but never enter the graph.
func TestBoardListFrustumCullsItems(t *testing.T) {
	tmpl := &model.Node{Type: "box", ID: "tile",
		Style: map[string]any{
			"x": "{{item.x}}", "y": "{{item.y}}",
			"width": 32.0, "height": 32.0, "background": "#FF0000",
		}}
	list := &model.Node{Type: "list", ID: "tiles", Data: "{{state.items}}", Template: tmpl}
	root := &model.Node{Type: "board", ID: "world",
		Style:    map[string]any{"width": 400.0, "height": 400.0, "background": "#111111"},
		Children: []*model.Node{list}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["items"] = []any{
		map[string]any{"x": 40.0, "y": 40.0},
		map[string]any{"x": 8000.0, "y": 40.0},
	}
	e := NewEngine(rt, SoftwareRenderer{})
	s := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(s)

	inst := instanceGroups(t, e, tmpl)
	if len(inst) != 1 {
		t.Fatalf("board list instances = %d, want 1 (far item culled)", len(inst))
	}
}

// The board zoom clamps to the [0.25, 4] range regardless of gesture size.
func TestBoardZoomClamped(t *testing.T) {
	e, s, _ := boardFixture(t)
	e.DrawFrame(s)
	e.Inter.Board.PanX, e.Inter.Board.PanY = 50, 50
	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 100})
	for i := 0; i < 30; i++ {
		e.HandleScroll(ScrollInput{DY: 120, Ctrl: true}) // 1.1^30 ≈ 17 → clamp 4
	}
	if z := e.Inter.Board.Zoom; z != maxBoardZoom {
		t.Errorf("zoom = %v, want clamped %v", z, maxBoardZoom)
	}
	for i := 0; i < 60; i++ {
		e.HandleScroll(ScrollInput{DY: -120, Ctrl: true})
	}
	if z := e.Inter.Board.Zoom; z != minBoardZoom {
		t.Errorf("zoom = %v, want clamped %v", z, minBoardZoom)
	}
}

// A board root with `cameraTarget` / `cameraCenter` / `cameraCell` rewrites
// PanX/PanY every layout pass so the target lands at the configured spot.
// Side-scroller games (mario / metroid / sonic) use this to make the
// engine own the camera follow instead of having qscript fight PanX.
//
// cameraTarget is in PHYSICAL pixels (same as the rest of the app) — the
// cameraCell/cameraViewport pair is multiplied together to derive the
// center offset, so a `cameraViewport: 16` + `cameraCell: 32` reads as
// "center the target inside a 512-px window". This test runs at 512x384
// (16 cells × 32 px wide), so target x=128 lands at screen center.
func TestBoardCameraFollow(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board",
		Style: map[string]any{"background": "#000000"},
		Props: map[string]any{
			"cameraTarget": "{{state.player}}",
			"cameraCenter": true,
			"cameraCell":   32.0,
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["player"] = map[string]any{"x": 128.0, "y": 192.0} // 4 cells across, 6 cells down
	surf := NewHeadlessSurface(image.Pt(512, 384))              // 16x12 cells @ 32px
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)
	if !e.Inter.Board.Active {
		t.Fatal("board must be active")
	}
	// Centered: pan = -target + (cameraViewport * cameraCell) / 2
	// = -128 + (16*32)/2 = -128 + 256 = 128
	if got, want := e.Inter.Board.PanX, 128.0; got != want {
		t.Errorf("PanX = %v, want %v (cameraCenter=true, cell=32, target x=128)", got, want)
	}
	// PanY = -192 + (12*32)/2 = -192 + 192 = 0 (target is mid-screen vertically)
	if got, want := e.Inter.Board.PanY, 0.0; got != want {
		t.Errorf("PanY = %v, want %v", got, want)
	}
	// Move the player: re-render and the camera follows.
	rt.State["player"] = map[string]any{"x": 320.0, "y": 192.0} // 10 cells across
	e.MarkDirty()
	e.DrawFrame(surf)
	if got, want := e.Inter.Board.PanX, -64.0; got != want {
		t.Errorf("after move PanX = %v, want %v (cameraCenter, cell=32, target x=320)", got, want)
	}
}

// NES side-scrollers never rewind the camera. cameraLockLeft keeps the
// furthest-right pan once the player has walked past the dead zone.
func TestBoardCameraLockLeft(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board",
		Style: map[string]any{"background": "#000000"},
		Props: map[string]any{
			"cameraTarget":   "{{state.player}}",
			"cameraCenter":   true,
			"cameraCell":     32.0,
			"cameraLockLeft": true,
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["player"] = map[string]any{"x": 400.0, "y": 384.0}
	surf := NewHeadlessSurface(image.Pt(512, 480))
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)
	rightPan := e.Inter.Board.PanX
	if rightPan >= 0 {
		t.Fatalf("expected a negative pan after walking right, got %v", rightPan)
	}
	rt.State["player"] = map[string]any{"x": 80.0, "y": 384.0}
	e.MarkDirty()
	e.DrawFrame(surf)
	if e.Inter.Board.PanX != rightPan {
		t.Errorf("lock-left: PanX = %v after walking back, want %v", e.Inter.Board.PanX, rightPan)
	}
}

// cameraResetToken changing must rewind lock-left (Mario R / restart).
func TestBoardCameraResetTokenRewindsLockLeft(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board",
		Style: map[string]any{"background": "#000000"},
		Props: map[string]any{
			"cameraTarget":     "{{state.player}}",
			"cameraCenter":     "x",
			"cameraDeadZone":   160.0,
			"cameraLockLeft":   true,
			"cameraResetToken": "{{state.cameraGen}}",
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["player"] = map[string]any{"x": 400.0, "y": 384.0}
	rt.State["cameraGen"] = 0.0
	surf := NewHeadlessSurface(image.Pt(512, 480))
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)
	if e.Inter.Board.PanX >= 0 {
		t.Fatalf("expected scrolled pan, got %v", e.Inter.Board.PanX)
	}
	rt.State["player"] = map[string]any{"x": 32.0, "y": 384.0}
	rt.State["cameraGen"] = 1.0
	e.MarkDirty()
	e.DrawFrame(surf)
	if e.Inter.Board.PanX < -1 {
		t.Errorf("after reset token, PanX = %v, want ~0 (camera back at start)", e.Inter.Board.PanX)
	}
}

// A board with `disablePan: true` ignores blank-space pointer drags — a
// follow-cam game has no pan affordance for the user, and a stray drag
// would fight the camera. The whiteboard example leaves the prop off.
func TestBoardDisablePanSuppressesBlankDrag(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board",
		Style: map[string]any{"background": "#EEEEEE"},
		Props: map[string]any{"disablePan": true},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)
	if !e.Inter.Board.PanDisabled {
		t.Fatal("disablePan=true must set Board.PanDisabled")
	}
	e.HandlePointer(PointerInput{Type: PointerPress, X: 200, Y: 200, Buttons: 1})
	if e.Inter.Board.Panning {
		t.Error("blank-space press on a disablePan board must NOT start a pan")
	}
}

// A board child with a fractional style x/y lands between pixels: the
// sub-pixel contract is a 60fps game authoring `x: state.player * 32` and
// expecting the sprite to drift smoothly, not snap to integer cells. The
// renderer rounds at draw (measure.go), so the final bbox is int but the
// pre-round value is float — here we exercise the parser keeps the float
// through the layout pass.
func TestBoardChildSubPixelPosition(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board",
		Style: map[string]any{"background": "#000000"},
		Children: []*model.Node{
			{Type: "box", ID: "n1", Style: map[string]any{
				"x": 100.5, "y": 50.25, "width": 60, "height": 40, "background": "#FF0000",
			}},
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(surf)
	// The style parser must keep the fractional part on PosX/PosY; rounding
	// happens at the layout step. We re-parse the child to assert the float
	// survived the pass — a regression to int(100.5) would break physics.
	ps := parseStyle(root.Children[0], rt)
	if ps.PosX != 100.5 {
		t.Errorf("PosX = %v, want 100.5 (sub-pixel must survive parse)", ps.PosX)
	}
	if ps.PosY != 50.25 {
		t.Errorf("PosY = %v, want 50.25", ps.PosY)
	}
	// And the rendered bbox snaps to the nearest pixel.
	gn := e.findGroupByModel(root.Children[0])
	if gn == nil {
		t.Fatal("note group missing")
	}
	bb := gn.GetBBox()
	// math.Round: 100.5 -> 101 (half away from zero), 50.25 -> 50.
	if int(bb.MinX) != 101 || int(bb.MinY) != 50 {
		t.Errorf("rendered bbox = (%v,%v), want (101,50) — round of 100.5/50.25", bb.MinX, bb.MinY)
	}
}
