package canvas

// Mario canvas-engine screenshot. Manual diagnostic — set MARIO_SCREENSHOT=1
// to render the mario app via the canvas engine's HeadlessSurface and write
// the result to /tmp/mario_canvas.png. Used to compare canvas engine output
// to the web HTML renderer when the web view doesn't match expectations.

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestMarioCanvasScreenshot(t *testing.T) {
	if os.Getenv("MARIO_SCREENSHOT") == "" {
		t.Skip("set MARIO_SCREENSHOT=1 to render")
	}
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
	img := surf.Frame()
	f, _ := os.Create("/tmp/mario_canvas.png")
	defer f.Close()
	png.Encode(f, img)
	t.Logf("wrote /tmp/mario_canvas.png (%dx%d)", img.Bounds().Dx(), img.Bounds().Dy())
}
