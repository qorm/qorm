package canvas

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestRaidenPlayerLoads(t *testing.T) {
	// Fresh image cache.
	imageCacheMu.Lock()
	imageCache = map[string]*image.RGBA{}
	imageWarned = map[string]bool{}
	imageCacheMu.Unlock()

	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "raiden"))
	if err != nil {
		t.Fatal(err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(320, 560))
	e.DrawFrame(surf)
	img := surf.Frame()
	// Player at (132,404) 56x56. Count opaque non-bg in that box.
	nonbg, blue, grey := 0, 0, 0
	for y := 404; y < 460; y++ {
		for x := 132; x < 188; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			if !(absi(int(c.R), 2) < 10 && absi(int(c.G), 8) < 10 && absi(int(c.B), 23) < 10) {
				nonbg++
			}
			if absi(int(c.R), 60) < 25 && absi(int(c.G), 130) < 25 && absi(int(c.B), 230) < 25 {
				blue++
			}
			if int(c.R) < 90 && int(c.G) > 100 && int(c.B) > 180 && c.A > 150 {
				blue++
			}
			if absi(int(c.R), 229) < 12 && absi(int(c.G), 229) < 12 && absi(int(c.B), 234) < 12 {
				grey++
			}
		}
	}
	t.Logf("player box: nonbg=%d blue=%d grey(placeholder)=%d", nonbg, blue, grey)
	if grey > 1000 {
		t.Error("player renders as placeholder grey box — image failed to load")
	}
	// The cockpit is a small region in the redesigned sprite (a single
	// bright blue highlight pixel plus the canopy band). Just require
	// the canopy's mid blue to be present, not a fixed pixel count.
	if blue < 10 {
		t.Error("player has no cockpit blue pixels — sprite not rendering correctly")
	}
}

func absi(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
