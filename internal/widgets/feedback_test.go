package widgets

// Engine-level tests for the W10 feedback/structure widgets: progress,
// spinner, avatar, tabs, animatedopacity — all rendered through the canvas
// registry seam into a HeadlessSurface, with pixel / geometry / state
// assertions.

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// feedbackEngine builds an engine + headless surface for one scene subtree.
func feedbackEngine(t *testing.T, root *model.Node) (*canvas.Engine, *canvas.HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(240, 160)), rt
}

func isAccent(c color.RGBA) bool { return c.R == 0 && c.G == 122 && c.B == 255 }

// ---------------------------------------------------------------- progress

func TestProgressRegistered(t *testing.T) {
	if _, ok := canvas.LookupWidget("progress"); !ok {
		t.Fatal("progress must be registered via this package's init")
	}
}

func TestProgressFillFractions(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "progress", ID: "p", Value: "{{state.v}}", Style: map[string]any{"width": float64(200)}},
	}}
	e, surf, rt := feedbackEngine(t, root)

	accentAt := func(x int) bool {
		for y := 0; y < 8; y++ {
			if isAccent(surf.Frame().RGBAAt(x, y)) {
				return true
			}
		}
		return false
	}

	rt.State["v"] = float64(0)
	e.MarkDirty()
	e.DrawFrame(surf)
	if accentAt(5) || accentAt(195) {
		t.Error("value 0 must paint no fill")
	}

	rt.State["v"] = float64(50)
	e.MarkDirty()
	e.DrawFrame(surf)
	if !accentAt(5) {
		t.Error("value 50 must fill the left half")
	}
	if accentAt(195) {
		t.Error("value 50 must not reach the right edge")
	}

	// HTML parity: a 0..1 fraction reads as a percentage.
	rt.State["v"] = float64(0.5)
	e.MarkDirty()
	e.DrawFrame(surf)
	if !accentAt(5) || accentAt(195) {
		t.Error("value 0.5 (fraction) must render like 50%%")
	}

	rt.State["v"] = float64(100)
	e.MarkDirty()
	e.DrawFrame(surf)
	if !accentAt(195) {
		t.Error("value 100 must fill to the right edge")
	}
}

func TestProgressMaxProp(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "progress", ID: "p", Value: "10", Props: map[string]any{"max": float64(20)},
			Style: map[string]any{"width": float64(200)}},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	// 10/20 = 50%: left half accent, right half track.
	if !isAccent(surf.Frame().RGBAAt(5, 4)) {
		t.Error("value 10 with max 20 must fill the left half")
	}
	if isAccent(surf.Frame().RGBAAt(195, 4)) {
		t.Error("value 10 with max 20 must not fill the right half")
	}
}

// ----------------------------------------------------------------- spinner

func TestSpinnerKeepsFrameLoopAlive(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "spinner", ID: "s"},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("a mounted spinner (AnimatedWidget) must keep the frame loop alive")
	}
	presents := surf.Presents
	e.DrawFrame(surf) // no dirty flag, no input — the animation alone ticks
	if surf.Presents != presents+1 {
		t.Error("spinner did not drive a second frame without a dirty flag")
	}
}

func TestSpinnerAngleFunction(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0},
		{250 * time.Millisecond, math.Pi / 2},
		{500 * time.Millisecond, math.Pi},
		{750 * time.Millisecond, 3 * math.Pi / 2},
		{time.Second, 0}, // wraps: 1s per revolution
	} {
		if got := spinAngle(tc.elapsed); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("spinAngle(%v) = %v, want %v", tc.elapsed, got, tc.want)
		}
	}
}

func TestSpinnerArcFollowsClock(t *testing.T) {
	frozen := time.Now()
	spinNow = func() time.Time { return frozen }
	defer func() { spinNow = time.Now }()

	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "spinner", ID: "s", Props: map[string]any{"size": float64(24)}},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)

	track := color.RGBA{198, 198, 200, 255}
	top := surf.Frame().RGBAAt(12, 0)
	bottom := surf.Frame().RGBAAt(12, 23)
	if !isAccent(top) {
		t.Errorf("first frame: arc must sit at the top (HTML border-top); top pixel = %v", top)
	}
	if bottom != track {
		t.Errorf("first frame: bottom of the ring must be the track colour; got %v", bottom)
	}

	// Half a revolution later the accent arc sits at the bottom.
	frozen = frozen.Add(500 * time.Millisecond)
	e.DrawFrame(surf)
	if top := surf.Frame().RGBAAt(12, 0); top != track {
		t.Errorf("t+500ms: top must be the track colour; got %v", top)
	}
	if bottom := surf.Frame().RGBAAt(12, 23); !isAccent(bottom) {
		t.Errorf("t+500ms: arc must sit at the bottom; got %v", bottom)
	}
}

// ------------------------------------------------------------------ avatar

func TestAvatarInitials(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "avatar", ID: "a", Props: map[string]any{"name": "ada"}},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	img := surf.Frame()

	// The disc is the theme secondary colour (HTML: #6366f1 behind initials).
	if c := img.RGBAAt(20, 6); c != (color.RGBA{88, 86, 214, 255}) {
		t.Errorf("disc pixel = %v, want theme secondary #5856D6", c)
	}
	// White initials ink somewhere in the centre band.
	white := 0
	for y := 12; y < 28; y++ {
		for x := 8; x < 32; x++ {
			if c := img.RGBAAt(x, y); c.R > 240 && c.G > 240 && c.B > 240 {
				white++
			}
		}
	}
	if white == 0 {
		t.Error("no white initials pixels — the avatar text did not render")
	}
}

func TestAvatarFallbackGreyDisc(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "avatar", ID: "a"},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	img := surf.Frame()

	if c := img.RGBAAt(20, 6); c != (color.RGBA{198, 198, 200, 255}) {
		t.Errorf("fallback disc pixel = %v, want separator grey", c)
	}
	// The "?" placeholder paints ink in the centre band.
	ink := 0
	for y := 12; y < 30; y++ {
		for x := 12; x < 28; x++ {
			if c := img.RGBAAt(x, y); c.R > 240 && c.G > 240 && c.B > 240 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Error("no placeholder glyph pixels on the fallback disc")
	}
}

func TestAvatarImageSrc(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "avatar", ID: "a", Props: map[string]any{"src": "a.png"}},
	}}
	e, surf, _ := feedbackEngine(t, root)
	e.RT.App.BaseDir = dir
	e.DrawFrame(surf)

	if c := surf.Frame().RGBAAt(20, 20); c.R < 200 || c.G > 60 || c.B > 60 {
		t.Errorf("avatar centre = %v, want the red source image (circular cover crop)", c)
	}
}

// -------------------------------------------------------------------- tabs

func tabsScene(active any) *model.Node {
	props := map[string]any{"tabs": []any{"One", "Two"}}
	if active != nil {
		props["active"] = active
	}
	return &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "tabs", ID: "tb", Props: props, Children: []*model.Node{
			{Type: "text", ID: "pa", Props: map[string]any{"text": "AAAA"}, Style: map[string]any{"color": "#ff0000"}},
			{Type: "text", ID: "pb", Props: map[string]any{"text": "BBBB"}, Style: map[string]any{"color": "#0000ff"}},
		}},
	}}
}

func countPixels(img *image.RGBA, match func(color.RGBA) bool) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if match(img.RGBAAt(x, y)) {
				n++
			}
		}
	}
	return n
}

func isRed(c color.RGBA) bool  { return c.R > 200 && c.G < 80 && c.B < 80 }
func isBlue(c color.RGBA) bool { return c.B > 200 && c.R < 80 && c.G < 80 }

// pressSecondTab taps the centre of the second tab in the bar. The consumed
// press returns redraw=true, but engine.HandlePointer's InteractiveWidget
// branch (canvas/engine.go:362) returns that bool to the HOST without storing
// the dirty flag its own frame gate checks (the generic path stores it at
// engine.go:473) — so a headless DrawFrame would render nothing. MarkDirty
// stands in for the missing store, the same host-side seam the canvas tests
// use; the pixel assertions below still prove the widget's state flip and
// Record path. (Engine-side gap, reported for the canvas owner to fix.)
func pressSecondTab(e *canvas.Engine) bool {
	w1 := tabWidth("One", 1)
	w2 := tabWidth("Two", 1)
	x := float64(w1 + w2/2)
	y := float64(tabsBarH(1) / 2)
	ok := e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: x, Y: y})
	e.MarkDirty()
	return ok
}

func TestTabsDefaultFirstActiveAndClickSwitches(t *testing.T) {
	e, surf, _ := feedbackEngine(t, tabsScene(nil))
	e.DrawFrame(surf)

	red := countPixels(surf.Frame(), isRed)
	blue := countPixels(surf.Frame(), isBlue)
	if red == 0 {
		t.Error("default: first tab's panel must be visible (no red text pixels)")
	}
	if blue != 0 {
		t.Error("default: second tab's panel must be hidden (blue text pixels present)")
	}

	if !pressSecondTab(e) {
		t.Fatal("press on the second tab was not consumed by the tabs widget")
	}
	e.DrawFrame(surf)
	red = countPixels(surf.Frame(), isRed)
	blue = countPixels(surf.Frame(), isBlue)
	if blue == 0 {
		t.Error("after tapping tab 2: its panel must be visible")
	}
	if red != 0 {
		t.Error("after tapping tab 2: the first panel must be hidden")
	}
}

func TestTabsControlledWritesState(t *testing.T) {
	e, surf, rt := feedbackEngine(t, tabsScene("{{state.tab}}"))
	rt.State["tab"] = float64(0)
	e.DrawFrame(surf)
	if countPixels(surf.Frame(), isRed) == 0 {
		t.Fatal("controlled tab 0: first panel must be visible")
	}

	if !pressSecondTab(e) {
		t.Fatal("press on the second tab was not consumed")
	}
	if v, ok := rt.State["tab"].(float64); !ok || v != 1 {
		t.Fatalf("controlled tap must write the index to state.tab; got %v", rt.State["tab"])
	}
	e.DrawFrame(surf)
	if countPixels(surf.Frame(), isBlue) == 0 {
		t.Error("controlled: after the state write the second panel must show")
	}
}

func TestTabsLiteralActiveSeedsDefault(t *testing.T) {
	e, surf, _ := feedbackEngine(t, tabsScene(float64(1)))
	e.DrawFrame(surf)
	if countPixels(surf.Frame(), isBlue) == 0 {
		t.Error("literal active:1 must seed the second tab as active")
	}
}

// ---------------------------------------------------------- animatedopacity

func animOpacityScene(id string, opacity any) *model.Node {
	return &model.Node{Type: "column", ID: "root",
		Style: map[string]any{"background": "#ffffff"},
		Children: []*model.Node{
			{Type: "animatedopacity", ID: id, Props: map[string]any{"opacity": opacity}, Children: []*model.Node{
				{Type: "column", ID: "box", Style: map[string]any{
					"width": float64(20), "height": float64(20), "background": "#000000"}},
			}},
		}}
}

func TestAnimatedOpacityFirstFramePaintsTarget(t *testing.T) {
	e, surf, _ := feedbackEngine(t, animOpacityScene("ao1", float64(0.5)))
	e.DrawFrame(surf)
	// Black box at 50% over the white scene background ≈ 127 per channel.
	c := surf.Frame().RGBAAt(10, 10)
	if c.R < 110 || c.R > 145 {
		t.Errorf("opacity 0.5 over white: centre pixel = %v, want ~127 grey", c)
	}
}

func TestAnimatedOpacityFullAndZero(t *testing.T) {
	e, surf, _ := feedbackEngine(t, animOpacityScene("ao2", float64(1)))
	e.DrawFrame(surf)
	if c := surf.Frame().RGBAAt(10, 10); c.R != 0 || c.G != 0 || c.B != 0 {
		t.Errorf("opacity 1: centre pixel = %v, want black", c)
	}

	e2, surf2, _ := feedbackEngine(t, animOpacityScene("ao3", float64(0)))
	e2.DrawFrame(surf2)
	if c := surf2.Frame().RGBAAt(10, 10); c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("opacity 0: centre pixel = %v, want the white background", c)
	}
}

func TestAnimatedOpacityTweenKeepsLoopAlive(t *testing.T) {
	e, surf, rt := feedbackEngine(t, animOpacityScene("ao4", "{{state.o}}"))
	rt.State["o"] = float64(1)
	e.DrawFrame(surf)
	if e.Animating() {
		t.Fatal("a settled opacity (first frame at target) must not animate")
	}

	// Retarget the tween: the next frame starts it, and the AnimatedWidget
	// seam keeps the loop alive without any dirty flag afterwards.
	rt.State["o"] = float64(0.2)
	e.MarkDirty()
	e.DrawFrame(surf)
	if !e.Animating() {
		t.Error("mid-tween: the frame loop must stay alive (AnimatedWidget)")
	}
}
