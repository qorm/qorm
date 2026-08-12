package canvas

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestTextDecorationUnderlineInks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 24))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.TextOp{Text: "Hi", Pos: image.Pt(4, 4), Scale: 1.4, Underline: true})
	SoftwareRenderer{}.Render(ops, img)
	// Underline sits near the bottom of the glyph box — scan for ink below top.
	ink := 0
	for y := 14; y < 22; y++ {
		for x := 4; x < 30; x++ {
			if c := img.RGBAAt(x, y); c.R < 200 {
				ink++
			}
		}
	}
	if ink < 4 {
		t.Fatalf("underline must ink pixels under the glyph; ink=%d", ink)
	}
}

func TestTextTransformUppercase(t *testing.T) {
	if got := applyTextTransform("hello", "uppercase"); got != "HELLO" {
		t.Errorf("uppercase = %q", got)
	}
	if got := applyTextTransform("Hello World", "lowercase"); got != "hello world" {
		t.Errorf("lowercase = %q", got)
	}
	if got := applyTextTransform("hello world", "capitalize"); got != "Hello World" {
		t.Errorf("capitalize = %q", got)
	}
}

func TestParseTextDecorationAndOutline(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "text", Style: map[string]any{
		"textDecoration": "underline line-through",
		"textTransform":  "uppercase",
		"outline":        "2px solid #ff0000",
		"lineClamp":      3.0,
	}}
	s := parseStyle(n, rt)
	if !strings.Contains(s.TextDecoration, "underline") || !strings.Contains(s.TextDecoration, "line-through") {
		t.Errorf("textDecoration = %q", s.TextDecoration)
	}
	if s.TextTransform != "uppercase" {
		t.Errorf("textTransform = %q", s.TextTransform)
	}
	if s.OutlineWidth != 2 || s.OutlineColor.R != 255 {
		t.Errorf("outline = w=%v c=%v", s.OutlineWidth, s.OutlineColor)
	}
	if s.LineClamp != 3 {
		t.Errorf("lineClamp = %d", s.LineClamp)
	}
}

func TestOutlineOutsideBox(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	ops := &op.Ops{}
	ops.Add(op.RRectOp{
		Rect:    image.Rect(10, 10, 30, 30),
		Fill:    color.RGBA{255, 255, 255, 255},
		Outline: color.RGBA{255, 0, 0, 255}, OutlineWidth: 2, OutlineOffset: 1,
	})
	SoftwareRenderer{}.Render(ops, img)
	// Just outside the fill (with offset 1) should pick up red outline.
	if c := img.RGBAAt(8, 20); c.R < 100 {
		t.Errorf("outline outside box = %v, want red-ish", c)
	}
	// Far outside stays white.
	if c := img.RGBAAt(2, 2); c.R < 250 {
		t.Errorf("far corner should stay white, got %v", c)
	}
}

func TestConicGradientVariesByAngle(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	ops := &op.Ops{}
	ops.Add(op.RRectOp{
		Rect: image.Rect(4, 4, 36, 36),
		GradientStops: []color.RGBA{
			{255, 0, 0, 255},
			{0, 0, 255, 255},
		},
		GradientConic: true, GradientAngle: 0,
		Fill: color.RGBA{255, 0, 0, 255},
	})
	SoftwareRenderer{}.Render(ops, img)
	// Top vs right should differ in hue for a red→blue conic.
	top := img.RGBAAt(20, 8)
	right := img.RGBAAt(32, 20)
	if top == right {
		t.Fatalf("conic gradient must vary by angle; top=%v right=%v", top, right)
	}
}

func TestLineClampParseAndMeasure(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	// Long text with small width and lineClamp 2.
	n := &model.Node{Type: "text", ID: "t",
		Props: map[string]any{"text": "abcdefghijklmnop qrstuvwxyz 1234567890"},
		Style: map[string]any{"fontSize": 12.0, "width": 40.0, "lineClamp": 2.0}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt2 := runtime.New(app)
	rt2.Theme = theme.GetDefault()
	e := NewEngine(rt2, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)
	// Smoke: rendered without panic and some ink exists.
	ink := 0
	frame := surf.Frame()
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if c := frame.RGBAAt(x, y); c.R < 250 || c.G < 250 || c.B < 250 {
				ink++
			}
		}
	}
	if ink < 4 {
		t.Fatalf("lineClamp text must paint something; ink=%d", ink)
	}
}
