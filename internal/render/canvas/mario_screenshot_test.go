package canvas

// Mario canvas-engine screenshot sequence. Manual diagnostic — set
// MARIO_SCREENSHOT=1 to render the mario app via the canvas engine's
// HeadlessSurface and write a sequence of PNGs to /tmp/mario_canvas_*.png.
// Used to compare canvas engine output to the web HTML renderer.

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

// shootMario boots the mario app, presses the requested keys, advances
// physicsSteps render frames (each forced to fire immediately so the test
// is fast), and writes the result to /tmp/mario_canvas_<name>.png.
func shootMario(t *testing.T, name string, physicsSteps int, pressKeys ...string) {
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
	for _, k := range pressKeys {
		e.HandleKey(KeyInput{Key: k, Down: true})
	}
	// Real-time scaling: at 60fps each step is 16.67ms. The test fixture
	// forces nextFire into the past, but physicsStep reads wall-clock dt
	// — running physicsSteps ticks in a tight loop only advances ~1ms of
	// simulated time per tick, so 1200 ticks ≈ 20 frames ≈ 0.33s of
	// perceived motion. We bump to 4000 ticks (~70 frames ≈ 1.2s) to
	// get visible movement in the static screenshot.
	for i := 0; i < physicsSteps; i++ {
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
	t.Logf("%s — mario.x=%v y=%v coins=%v status=%v camX=%v camY=%v",
		out, m["x"], m["y"], rt.State["coins"], rt.State["status"],
		rt.State["cameraX"], rt.State["cameraY"])
}

func TestMarioCanvasT0(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	shootMario(t, "t0", 0)
}
func TestMarioCanvasWalk(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	shootMario(t, "walk", 4000, "right")
}
func TestMarioCanvasJump(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	// walk for a bit, then jump
	shootMario(t, "jump", 4000, "right", "space")
}
func TestMarioCanvasGoombas(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
	// let the world settle and goombas move
	shootMario(t, "goombas", 8000)
}
