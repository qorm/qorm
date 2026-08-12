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

func TestParseFilterInvertSepia(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{
		"filter": "invert(100%) sepia(80%)",
	}}
	s := parseStyle(n, rt)
	if s.FilterInvert != 1 {
		t.Errorf("invert = %v, want 1", s.FilterInvert)
	}
	if s.FilterSepia < 0.79 || s.FilterSepia > 0.81 {
		t.Errorf("sepia = %v, want 0.8", s.FilterSepia)
	}
}

func TestFilterInvertRedIsCyan(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Invert: 1, Opacity: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(12, 12)
	// invert(1) on red → near-cyan (0, 255, 255).
	if c.R > 20 || c.G < 230 || c.B < 230 {
		t.Fatalf("invert(1) red = %v, want near cyan", c)
	}
}

func TestFilterSepiaRedIsBrownish(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Sepia: 1, Opacity: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(12, 12)
	// CSS sepia(1) on red: r=0.393, g=0.349, b=0.272 → brownish, R>G>B.
	if c.R < 80 || c.G >= c.R || c.B >= c.G {
		t.Fatalf("sepia(1) red = %v, want brownish R>G>B", c)
	}
	if c.R > 140 || c.G < 60 || c.B > 100 {
		t.Fatalf("sepia(1) red = %v, want ~100/89/69", c)
	}
}

func TestFilterInvertStyleEndToEnd(t *testing.T) {
	box := &model.Node{Type: "box", ID: "inv",
		Style: map[string]any{
			"width": 20.0, "height": 20.0, "x": 4.0, "y": 4.0,
			"background": "#ff0000",
			"filter":     "invert(100%)",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(40, 40))
	e.DrawFrame(surf)
	c := surf.Frame().RGBAAt(14, 14)
	if c.R > 30 || c.G < 220 || c.B < 220 {
		t.Fatalf("style invert(100%%) red = %v, want near cyan", c)
	}
}
