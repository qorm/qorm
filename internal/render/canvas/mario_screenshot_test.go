package canvas

// Mario canvas-engine screenshot sequence. Manual diagnostic — set
// MARIO_SCREENSHOT=1 (or a name like T0/T1/T2/WALK/JUMP for individual
// frames) to render the mario app via the canvas engine's HeadlessSurface
// and write the result to /tmp/mario_canvas_<name>.png. Used to compare
// canvas engine output to the web HTML renderer when the web view doesn't
// match expectations.

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func screenshotMario(t *testing.T, name string, tickSteps int, pressKeys ...string) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "mario"))
	if err != nil {
		t.Fatal(err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(1024, 480))
	e.DrawFrame(surf)
	// Press keys
	for _, k := range pressKeys {
		e.HandleKey(KeyInput{Key: k, Down: true})
	}
	// Advance physics
	for i := 0; i < tickSteps; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	img := surf.Frame()
	out := "/tmp/mario_canvas_" + name + ".png"
	f, _ := os.Create(out)
	defer f.Close()
	png.Encode(f, img)
	m := rt.State["mario"].(map[string]any)
	t.Logf("wrote %s — mario.x=%v y=%v coins=%v status=%v",
		out, m["x"], m["y"], rt.State["coins"], rt.State["status"])
}

func TestMarioCanvasT0(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	screenshotMario(t, "t0", 0)
}
func TestMarioCanvasT1(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	screenshotMario(t, "t1", 60) // ~1s of physics
}
func TestMarioCanvasWalk(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	screenshotMario(t, "walk", 120, "right") // walk right for ~2s
}
func TestMarioCanvasJump(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	screenshotMario(t, "jump", 60, "right", "space") // walk + jump 1s
}
