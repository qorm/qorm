package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestParseClipPathCircleEllipseInset(t *testing.T) {
	kind, rx, ry, _, _, ok := parseClipPath("circle(50%)", 100, 80)
	if !ok || kind != "ellipse" || rx != 40 || ry != 40 {
		// min(100,80)*0.5 = 40
		t.Fatalf("circle(50%%) = %s rx=%v ry=%v ok=%v", kind, rx, ry, ok)
	}
	kind, rx, ry, _, _, ok = parseClipPath("ellipse(50% 25%)", 100, 80)
	if !ok || kind != "ellipse" || rx != 50 || ry != 20 {
		t.Fatalf("ellipse = %s rx=%v ry=%v", kind, rx, ry)
	}
	kind, _, _, inset, rad, ok := parseClipPath("inset(10px round 5px)", 100, 100)
	if !ok || kind != "inset" || inset != (image.Rectangle{Min: image.Pt(10, 10), Max: image.Pt(90, 90)}) || rad != 5 {
		t.Fatalf("inset = %s %v rad=%v", kind, inset, rad)
	}
}

func TestClipPathCircleKeepsCornersEmpty(t *testing.T) {
	// Full red rect under a circle clip — corners of the AABB stay white.
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	ops := &op.Ops{}
	ops.Add(op.SaveOp{})
	ops.Add(op.ClipOp{
		Rect:      image.Rect(4, 4, 36, 36),
		EllipseRX: 16, EllipseRY: 16,
	})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 36, 36)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})
	SoftwareRenderer{}.Render(ops, img)
	// Outer corner of AABB (4,4) is outside the circle (center 20,20 r=16).
	// White background (no red fill) is correct.
	if c := img.RGBAAt(4, 4); c.R > 200 && c.G < 50 {
		t.Errorf("circle clip corner should not be red fill, got %v", c)
	}
	// Center is red.
	if c := img.RGBAAt(20, 20); c.R < 200 {
		t.Errorf("circle clip center should be red, got %v", c)
	}
}

func TestLayerCacheReusesBitmap(t *testing.T) {
	ResetLayerCache()
	t.Cleanup(ResetLayerCache)

	build := func() *op.Ops {
		ops := &op.Ops{}
		ops.Add(op.LayerOp{
			Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
			CacheKey: "demo", CacheFP: 42,
		})
		ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
		ops.Add(op.ClipOp{Rect: image.Rect(10, 10, 30, 30)})
		ops.Add(op.PaintOp{})
		ops.Add(op.EndLayerOp{})
		return ops
	}

	img1 := image.NewRGBA(image.Rect(0, 0, 40, 40))
	SoftwareRenderer{}.Render(build(), img1)
	if c := img1.RGBAAt(20, 20); c.B < 200 {
		t.Fatalf("first paint center = %v", c)
	}

	// Second paint with same CacheKey/FP — hits cache. Mutate would-be child
	// ops to red; cache hit must still show blue.
	ops2 := &op.Ops{}
	ops2.Add(op.LayerOp{
		Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
		CacheKey: "demo", CacheFP: 42,
	})
	ops2.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}}) // ignored on cache hit
	ops2.Add(op.ClipOp{Rect: image.Rect(10, 10, 30, 30)})
	ops2.Add(op.PaintOp{})
	ops2.Add(op.EndLayerOp{})
	img2 := image.NewRGBA(image.Rect(0, 0, 40, 40))
	SoftwareRenderer{}.Render(ops2, img2)
	if c := img2.RGBAAt(20, 20); c.B < 200 || c.R > 50 {
		t.Fatalf("cache hit must keep blue, got %v", c)
	}

	// New FP invalidates cache.
	ops3 := &op.Ops{}
	ops3.Add(op.LayerOp{
		Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
		CacheKey: "demo", CacheFP: 99,
	})
	ops3.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops3.Add(op.ClipOp{Rect: image.Rect(10, 10, 30, 30)})
	ops3.Add(op.PaintOp{})
	ops3.Add(op.EndLayerOp{})
	img3 := image.NewRGBA(image.Rect(0, 0, 40, 40))
	SoftwareRenderer{}.Render(ops3, img3)
	if c := img3.RGBAAt(20, 20); c.G < 200 {
		t.Fatalf("new FP must repaint green, got %v", c)
	}
}

func TestClipPathStyleEndToEnd(t *testing.T) {
	box := &model.Node{Type: "box", ID: "clip1",
		Style: map[string]any{
			"width": 40.0, "height": 40.0, "x": 10.0, "y": 10.0,
			"background": "#ff0000",
			"clipPath":   "circle(50%)",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)
	// Corner of the box AABB should not be solid red.
	if c := surf.Frame().RGBAAt(10, 10); c.R > 200 && c.G < 20 {
		t.Errorf("clipPath circle must cut corner, got %v", c)
	}
	// Near center of box should be red.
	if c := surf.Frame().RGBAAt(30, 30); c.R < 150 {
		t.Errorf("clipPath center should be red, got %v", c)
	}
}
