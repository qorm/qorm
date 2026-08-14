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

func TestParseTint(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{"tint": "#00ff00"}}
	s := parseStyle(n, rt)
	if s.Tint.A == 0 || s.Tint.G < 250 || s.Tint.R > 5 || s.Tint.B > 5 {
		t.Fatalf("tint #00ff00 = %v, want green", s.Tint)
	}
	n2 := &model.Node{Type: "box", Style: map[string]any{"tint": "rgb(255, 0, 0)"}}
	s2 := parseStyle(n2, rt)
	if s2.Tint.R < 250 || s2.Tint.G > 5 || s2.Tint.B > 5 || s2.Tint.A < 250 {
		t.Fatalf("tint rgb(255,0,0) = %v, want red", s2.Tint)
	}
}

func TestTintRedTimesGreenIsBlack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{
		Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
		Tint: color.RGBA{0, 255, 0, 255},
	})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(12, 12)
	if c.R > 15 || c.G > 15 || c.B > 15 {
		t.Fatalf("red × tint #00ff00 = %v, want near black", c)
	}
}

func TestTintWhiteTimesRedIsRed(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{
		Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
		Tint: color.RGBA{255, 0, 0, 255},
	})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 255, 255, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(12, 12)
	if c.R < 230 || c.G > 20 || c.B > 20 {
		t.Fatalf("white × tint #ff0000 = %v, want red", c)
	}
}

func TestTintStyleOpensLayer(t *testing.T) {
	box := &model.Node{Type: "box", ID: "tintbox",
		Style: map[string]any{
			"width": 20.0, "height": 20.0, "x": 4.0, "y": 4.0,
			"background": "#ffffff",
			"tint":       "#ff0000",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(40, 40))
	e.DrawFrame(surf)
	c := surf.Frame().RGBAAt(14, 14)
	if c.R < 220 || c.G > 30 || c.B > 30 {
		t.Fatalf("white fill + tint #ff0000 = %v, want red", c)
	}
}
