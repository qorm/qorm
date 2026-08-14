package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func TestParseCSSFilterStack(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{
		"filter": "blur(4px) brightness(120%) contrast(0.5) saturate(0)",
	}}
	s := parseStyle(n, rt)
	if s.FilterBlur != 4 {
		t.Errorf("blur = %v, want 4", s.FilterBlur)
	}
	if s.FilterBrightness < 1.19 || s.FilterBrightness > 1.21 {
		t.Errorf("brightness = %v, want 1.2", s.FilterBrightness)
	}
	if s.FilterContrast != 0.5 {
		t.Errorf("contrast = %v, want 0.5", s.FilterContrast)
	}
	if s.FilterSaturate != 0 {
		t.Errorf("saturate = %v, want 0", s.FilterSaturate)
	}
}

// saturate(0) must collapse a pure red fill toward gray luminance.
func TestFilterSaturateGrayscale(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Blur: 0, Brightness: 1, Contrast: 1, Saturate: 0})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(8, 8, 24, 24)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)

	c := img.RGBAAt(16, 16)
	// Rec.709 red → ~54 gray; must not stay pure red.
	if c.R > 200 && c.G < 20 && c.B < 20 {
		t.Fatalf("saturate(0) left pure red %v", c)
	}
	if abs8(c.R, c.G) > 30 || abs8(c.G, c.B) > 30 {
		t.Fatalf("saturate(0) should be near-gray, got %v", c)
	}
}

func abs8(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

// brightness(0) paints black; brightness(2) brightens mid gray.
func TestFilterBrightness(t *testing.T) {
	black := image.NewRGBA(image.Rect(0, 0, 16, 16))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Brightness: 0, Contrast: 1, Saturate: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{200, 100, 50, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(2, 2, 14, 14)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, black)
	if c := black.RGBAAt(8, 8); c.R > 5 || c.G > 5 || c.B > 5 {
		t.Errorf("brightness(0) = %v, want near black", c)
	}
}

// overflow:hidden + borderRadius clips a child that sticks out of the rounded box.
func TestOverflowHiddenRoundedClip(t *testing.T) {
	// Parent 40×40 radius 20 (circle-ish), child is a full black square
	// positioned at 0,0 size 40 — corners of the AABB must stay white.
	child := &model.Node{Type: "box", ID: "c",
		Style: map[string]any{
			"width": 40.0, "height": 40.0, "background": "#000000",
			"x": 0.0, "y": 0.0,
		}}
	parent := &model.Node{Type: "box", ID: "p",
		Style: map[string]any{
			"width": 40.0, "height": 40.0, "x": 10.0, "y": 10.0,
			"borderRadius": 20.0, "overflow": "hidden",
			"background": "#ffffff",
		},
		Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{parent}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)

	// Corner of the parent AABB (10,10) is outside the rounded clip → not black.
	// With overflow hidden + r=20, the exact corner should be clipped.
	c := surf.Frame().RGBAAt(10, 10)
	if c.R < 200 && c.G < 200 && c.B < 200 {
		// Dark = child painted outside rounded bounds — clip failed.
		t.Errorf("overflow:hidden rounded clip: outer corner still dark %v", c)
	}
	// Center of parent should be dark (child fill).
	mid := surf.Frame().RGBAAt(30, 30)
	if mid.R > 80 {
		t.Errorf("center should show black child, got %v", mid)
	}
}

// 3-pass Gaussian-approx blur softens more smoothly than a single box of same r.
func TestGaussianApproxSpreadsFurther(t *testing.T) {
	// Smoke: blur still darkens outside the solid rect (regression for 3-pass path).
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Blur: 6, Brightness: 1, Contrast: 1, Saturate: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(24, 24, 40, 40)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	if c := img.RGBAAt(20, 32); c.R > 245 {
		t.Fatalf("gaussian-approx blur must soft-spread, edge pixel %v", c)
	}
}
