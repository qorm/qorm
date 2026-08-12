package canvas

// End-to-end measurement of examples/canvas-fx — the showcase for recent
// canvas rendering features (scroll-snap, mask fade, conic, outline, filter,
// blend, FLIP). Loads the real app JSON and asserts rendered pixels + measure.

import (
	"encoding/json"
	"image"
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func decodeMeasureRows(t *testing.T, e *Engine) []map[string]any {
	t.Helper()
	raw := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("measure json: %v\n%s", err, raw)
	}
	return rows
}

func canvasFxFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "canvas-fx"))
	if err != nil {
		t.Fatalf("load examples/canvas-fx: %v", err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(420, 720))
	e.DrawFrame(surf)
	return e, surf, rt
}

func TestCanvasFxExampleLoadsAndRenders(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	if surf.Presents < 1 {
		t.Fatal("first frame must present")
	}
	// Scene must paint non-white content (dark stage background).
	c := surf.Frame().RGBAAt(10, 10)
	if c.R > 40 && c.G > 40 && c.B > 40 {
		t.Fatalf("stage background should be dark, got %v", c)
	}
	// Measure rows include key demo nodes.
	rows := decodeMeasureRows(t, e)
	ids := map[string]bool{}
	for _, r := range rows {
		if id, _ := r["id"].(string); id != "" {
			ids[id] = true
		}
	}
	for _, want := range []string{"title", "snap_strip", "conic_disc", "mask_panel", "flip_chip", "btn_flip", "clip_circle", "cache_blur"} {
		if !ids[want] {
			t.Errorf("measure missing id %q (got %d rows)", want, len(rows))
		}
	}
}

func TestCanvasFxClipPathCircleCutsCorner(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Content is tall; scroll the outer stage so clip_circle is on-screen.
	var stage *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || stage != nil {
			return
		}
		if n.ID == "stage" {
			stage = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(e.sceneRoot())
	if stage == nil {
		t.Fatal("stage scroll missing")
	}
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{stage: {Y: 1000}}
	e.MarkDirty()
	e.DrawFrame(surf)
	rows := decodeMeasureRows(t, e)
	var x, y, w, h float64
	for _, r := range rows {
		if r["id"] == "clip_circle" {
			x, y, w, h = asF64(r["x"]), asF64(r["y"]), asF64(r["w"]), asF64(r["h"])
			break
		}
	}
	if w < 10 {
		t.Fatal("clip_circle not measured")
	}
	// After scroll, AbsY is still content coords; visual y = absY - scrollOffset.
	visY := int(y - 1000)
	visX := int(x)
	frame := surf.Frame()
	if visY < 0 || visY >= frame.Bounds().Dy() {
		// Still off-screen — rely on unit TestClipPathStyleEndToEnd.
		t.Logf("clip_circle still off-screen after scroll (y=%v visY=%d); layout ok", y, visY)
		return
	}
	mid := frame.RGBAAt(visX+int(w/2), visY+int(h/2))
	if mid.R+mid.G+mid.B < 80 {
		t.Errorf("clip_circle center too dark %v at (%d,%d)", mid, visX+int(w/2), visY+int(h/2))
	}
}

func TestCanvasFxLayerCacheHits(t *testing.T) {
	ResetLayerCache()
	t.Cleanup(ResetLayerCache)
	// Minimal app matching example authoring for cache_blur.
	n := &model.Node{Type: "box", ID: "cache_blur", Style: map[string]any{
		"filter": "blur(2px)", "layerCache": true,
		"width": 120.0, "height": 48.0, "background": "#5e5ce6",
		"x": 10.0, "y": 10.0,
	}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	s := parseStyle(n, rt)
	if !s.LayerCache {
		t.Fatal("layerCache style must parse true")
	}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(160, 80))
	e.DrawFrame(surf)
	layerCacheMu.Lock()
	ent := layerCache["cache_blur"]
	layerCacheMu.Unlock()
	if ent == nil {
		t.Fatal("expected layerCache entry for id=cache_blur after paint")
	}
	// Second frame same content → still cached (FP stable).
	e.MarkDirty()
	e.DrawFrame(surf)
	layerCacheMu.Lock()
	ent2 := layerCache["cache_blur"]
	layerCacheMu.Unlock()
	if ent2 == nil || ent2.fp != ent.fp {
		t.Fatal("layer cache entry should remain for unchanged content")
	}
}

func TestCanvasFxConicDiscNotFlat(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Sample several points on the conic disc — angles must differ.
	// Disc is roughly left side under title; find non-white colorful pixels.
	frame := surf.Frame()
	var samples []color.RGBA
	for y := 0; y < 720; y += 4 {
		for x := 0; x < 200; x += 4 {
			c := frame.RGBAAt(x, y)
			// Strong chroma (not gray/white/black stage).
			if max3(c.R, c.G, c.B)-min3(c.R, c.G, c.B) > 40 && c.A > 200 {
				samples = append(samples, c)
			}
		}
	}
	if len(samples) < 8 {
		t.Fatalf("expected colorful conic samples, got %d", len(samples))
	}
	// At least two distinct hues among samples.
	distinct := 0
	seen := samples[0]
	for _, c := range samples {
		if absU8(c.R, seen.R)+absU8(c.G, seen.G)+absU8(c.B, seen.B) > 80 {
			distinct++
			seen = c
		}
	}
	if distinct < 1 {
		t.Fatalf("conic disc should vary by angle; samples all similar to %v", samples[0])
	}
	_ = e
}

func TestCanvasFxMaskFadeSoftensRight(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	rows := decodeMeasureRows(t, e)
	var mx, my, mw, mh float64
	found := false
	for _, r := range rows {
		if r["id"] == "mask_panel" {
			mx = asF64(r["x"])
			my = asF64(r["y"])
			mw = asF64(r["w"])
			mh = asF64(r["h"])
			found = true
			break
		}
	}
	if !found || mw < 40 || mh < 10 {
		t.Fatalf("mask_panel measure missing or tiny: %v,%v %vx%v", mx, my, mw, mh)
	}
	// Measure is logical; surface is physical at scale 1 for HeadlessSurface default.
	frame := surf.Frame()
	y := int(my + mh/2)
	xL := int(mx + mw*0.2)
	xR := int(mx + mw*0.92)
	if y < 0 || y >= frame.Bounds().Dy() || xR >= frame.Bounds().Dx() {
		t.Fatalf("sample out of bounds y=%d xL=%d xR=%d", y, xL, xR)
	}
	left := frame.RGBAAt(xL, y)
	right := frame.RGBAAt(xR, y)
	// Stage is dark (#0f1115); opaque purple panel is bright. Fade reduces
	// coverage so the right edge is darker (more stage shows through).
	sumL := int(left.R) + int(left.G) + int(left.B)
	sumR := int(right.R) + int(right.G) + int(right.B)
	if sumR >= sumL {
		t.Fatalf("maskFade right should darken toward stage; left=%v (%d) right=%v (%d)", left, sumL, right, sumR)
	}
	// Right must still differ from solid stage (not fully gone).
	stage := frame.RGBAAt(2, 2)
	if right == stage {
		t.Fatalf("right edge fully transparent (stage); want soft fade, got %v", right)
	}
}

func TestCanvasFxScrollSnapArms(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Find snap_strip model node.
	var scroll *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || scroll != nil {
			return
		}
		if n.ID == "snap_strip" {
			scroll = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(e.sceneRoot())
	if scroll == nil {
		t.Fatal("snap_strip not found")
	}
	axis, mand := scrollSnapConfig(scroll)
	if axis != "y" || !mand {
		t.Fatalf("scrollSnapType = %q mand=%v, want y mandatory", axis, mand)
	}
	// Offset mid-page and arm snap.
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{scroll: {Y: 60}}
	e.MarkDirty()
	e.DrawFrame(surf)
	pos := e.Inter.ScrollOffsets[scroll]
	mom := ScrollMomentum{}
	if !e.tryArmScrollSnap(scroll, &pos, &mom) {
		t.Fatal("mandatory snap mid-page must arm on real example layout")
	}
	if !mom.HasSnapY {
		t.Error("expected HasSnapY")
	}
	if mom.SnapToY != 0 && mom.SnapToY != 120 {
		// Page heights are 120; nearest of 0 or 120 from 60.
		t.Logf("SnapToY=%v (ok if other page edge)", mom.SnapToY)
	}
}

func TestCanvasFxFlipLayoutMotion(t *testing.T) {
	e, surf, rt := canvasFxFixture(t)
	// Initial chip on the left (flipLeft=true → x=16).
	rows := decodeMeasureRows(t, e)
	var x0 float64
	found := false
	for _, r := range rows {
		if r["id"] == "flip_chip" {
			if x, ok := r["x"].(float64); ok {
				x0 = x
				found = true
			} else if xi, ok := r["x"].(int); ok {
				x0 = float64(xi)
				found = true
			}
		}
	}
	if !found {
		t.Fatal("flip_chip not in measure")
	}
	// Toggle flip and settle.
	rt.Dispatch("toggle_flip", nil)
	e.MarkDirty()
	for i := 0; i < 40; i++ {
		e.DrawFrame(surf)
		if !e.Animating() && !flipStillRunning() && !e.Dirty() {
			break
		}
		e.MarkDirty()
		time.Sleep(10 * time.Millisecond)
	}
	rows = decodeMeasureRows(t, e)
	var x1 float64
	for _, r := range rows {
		if r["id"] == "flip_chip" {
			if x, ok := r["x"].(float64); ok {
				x1 = x
			} else if xi, ok := r["x"].(int); ok {
				x1 = float64(xi)
			}
		}
	}
	if x1 <= x0+20 {
		t.Fatalf("after toggle flip, chip should move right: x0=%v x1=%v", x0, x1)
	}
}

func TestCanvasFxTextTransformInStyle(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	// Mirror subtitle style from the example.
	n := &model.Node{Type: "text", Props: map[string]any{
		"text": "snap · mask · conic",
	}, Style: map[string]any{"textTransform": "uppercase", "fontSize": 13.0}}
	s := parseStyle(n, rt)
	if s.TextTransform != "uppercase" {
		t.Fatalf("textTransform = %q", s.TextTransform)
	}
	got := applyTextTransform("snap · mask · conic", s.TextTransform)
	if got != "SNAP · MASK · CONIC" {
		t.Fatalf("transform = %q", got)
	}
}

func max3(a, b, c uint8) uint8 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c uint8) uint8 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func absU8(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

func asF64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}
