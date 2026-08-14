package render

import (
	"math"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func TestComputeBoardCameraPan_NoCenter(t *testing.T) {
	px, py := ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 100, TargetY: 40,
		ViewW: 512, ViewH: 384,
		MaxX: math.Inf(1), MaxY: math.Inf(1),
	})
	if px != -100 || py != -40 {
		t.Errorf("no-center pan = (%v,%v), want (-100,-40)", px, py)
	}
}

func TestComputeBoardCameraPan_CenterBoth(t *testing.T) {
	// Same numbers as canvas TestBoardCameraFollow at 512x384:
	// pan = -target + view/2 → (-128+256, -192+192) = (128, 0)
	px, py := ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 128, TargetY: 192,
		CenterX: true, CenterY: true,
		ViewW: 512, ViewH: 384,
		MaxX: math.Inf(1), MaxY: math.Inf(1),
	})
	if px != 128 || py != 0 {
		t.Errorf("center pan = (%v,%v), want (128,0)", px, py)
	}
	px, py = ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 320, TargetY: 192,
		CenterX: true, CenterY: true,
		ViewW: 512, ViewH: 384,
		MaxX: math.Inf(1), MaxY: math.Inf(1),
	})
	if px != -64 || py != 0 {
		t.Errorf("after move pan = (%v,%v), want (-64,0)", px, py)
	}
}

func TestComputeBoardCameraPan_CenterXOnly(t *testing.T) {
	px, py := ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 200, TargetY: 99,
		CenterX: true, CenterY: false,
		ViewW: 400, ViewH: 300,
		MaxX: math.Inf(1), MaxY: math.Inf(1),
	})
	// X centred: -200+200=0; Y pinned to world top when not centreY.
	if px != 0 || py != 0 {
		t.Errorf("centerX pan = (%v,%v), want (0,0)", px, py)
	}
}

func TestComputeBoardCameraPan_DeadZone(t *testing.T) {
	// Target still inside left band → keep CurPanX (0).
	px, _ := ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 50, TargetY: 0,
		CenterX: true, CenterY: false,
		ViewW: 510, ViewH: 300,
		DeadZone: 170,
		MaxX:     math.Inf(1), MaxY: math.Inf(1),
		CurPanX: 0,
	})
	if px != 0 {
		t.Errorf("inside dead-zone panX = %v, want 0", px)
	}
	// Target past the band → park target on dead-zone edge.
	px, _ = ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 300, TargetY: 0,
		CenterX: true, CenterY: false,
		ViewW: 510, ViewH: 300,
		DeadZone: 170,
		MaxX:     math.Inf(1), MaxY: math.Inf(1),
		CurPanX: 0,
	})
	if want := 170.0 - 300; px != want {
		t.Errorf("past dead-zone panX = %v, want %v", px, want)
	}
}

func TestComputeBoardCameraPan_CameraMax(t *testing.T) {
	px, py := ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 1000, TargetY: 800,
		CenterX: true, CenterY: true,
		ViewW: 400, ViewH: 300,
		MaxX: 200, MaxY: 100,
	})
	// Unclamped would be (-1000+200, -800+150)=(-800,-650); clamp to (-200,-100).
	if px != -200 || py != -100 {
		t.Errorf("cameraMax pan = (%v,%v), want (-200,-100)", px, py)
	}
	// Positive pan (target near origin while centred) clamps to 0.
	px, py = ComputeBoardCameraPan(BoardCameraParams{
		TargetX: 50, TargetY: 40,
		CenterX: true, CenterY: true,
		ViewW: 400, ViewH: 300,
		MaxX: 200, MaxY: 100,
	})
	if px != 0 || py != 0 {
		t.Errorf("left-edge clamp pan = (%v,%v), want (0,0)", px, py)
	}
}

func TestHTMLBoard_CameraTargetBinding(t *testing.T) {
	root := &model.Node{
		Type: "board", ID: "board",
		Style: map[string]any{"width": 512.0, "height": 384.0, "background": "#000"},
		Props: map[string]any{
			"cameraTarget": "{{state.player}}",
			"cameraCenter": true,
			"cameraCell":   32.0,
			// Explicit viewport so math matches canvas TestBoardCameraFollow
			// (view = 16*32 × proportional height).
			"cameraViewport": 16.0,
		},
		Children: []*model.Node{
			{Type: "box", ID: "p", Style: map[string]any{"x": 128.0, "y": 192.0, "width": 32.0, "height": 32.0}},
		},
	}
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{"player": map[string]any{"x": 128.0, "y": 192.0}}},
	}
	res := Render(runtime.New(app))
	// viewW = 16*32 = 512; panX = -128 + 256 = 128; panY = -192 + 192 = 0
	// (viewH = 16 * 384/512 * 32 = 384 after cell multiply)
	if !strings.Contains(res.HTML, "translate(128.0000px,0.0000px)") {
		t.Fatalf("expected cameraCenter pan translate(128,0) in HTML, got:\n%s", res.HTML)
	}
}

func TestHTMLBoard_EngineOptsPrecedeCameraProps(t *testing.T) {
	root := &model.Node{
		Type: "board", ID: "board",
		Props: map[string]any{
			"cameraTarget": map[string]any{"x": 100.0, "y": 50.0},
			"cameraCenter": true,
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	res := RenderWithOpts(rt, RenderOpts{Board: BoardRender{Active: true, PanX: 12, PanY: 34, Zoom: 1}})
	if !strings.Contains(res.HTML, "translate(12.0000px,34.0000px)") {
		t.Fatalf("Active engine Board must win over cameraTarget, got:\n%s", res.HTML)
	}
	// Non-zero pan without Active still wins (legacy host signal).
	res = RenderWithOpts(rt, RenderOpts{Board: BoardRender{PanX: 7, PanY: 8, Zoom: 1}})
	if !strings.Contains(res.HTML, "translate(7.0000px,8.0000px)") {
		t.Fatalf("non-zero opts.Board pan must win, got:\n%s", res.HTML)
	}
}

func TestHTMLBoard_CameraTargetBeforeStateCamera(t *testing.T) {
	root := &model.Node{
		Type: "board", ID: "board",
		Props: map[string]any{
			"cameraTarget": map[string]any{"x": 40.0, "y": 10.0},
			// no cameraCenter → pan = -target
		},
	}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"cameraX": -999.0,
			"cameraY": -888.0,
		}},
	}
	res := Render(runtime.New(app))
	if !strings.Contains(res.HTML, "translate(-40.0000px,-10.0000px)") {
		t.Fatalf("cameraTarget must beat state.cameraX/Y, got:\n%s", res.HTML)
	}
}

func TestHTMLBoard_StateCameraFallback(t *testing.T) {
	root := &model.Node{Type: "board", ID: "board"}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"cameraX": -55.0,
			"cameraY": -66.0,
		}},
	}
	res := Render(runtime.New(app))
	if !strings.Contains(res.HTML, "translate(-55.0000px,-66.0000px)") {
		t.Fatalf("state cameraX/Y fallback missing, got:\n%s", res.HTML)
	}
}

func TestBoardViewportPx_FromCameraViewport(t *testing.T) {
	n := &model.Node{Props: map[string]any{"cameraViewport": 16.0}}
	w, h := boardViewportPx(n, 512, 384)
	if w != 16 || h != 16*384/512 {
		t.Errorf("viewport = (%v,%v), want (16,12)", w, h)
	}
	w, h = boardViewportPx(n, 0, 0)
	if w != 16 || h != 12 {
		t.Errorf("headless viewport = (%v,%v), want (16,12)", w, h)
	}
}
