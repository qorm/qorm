package canvas

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestParseFilterGrayscaleHueOpacityDropShadow(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{
		"filter": "grayscale(100%) hue-rotate(90deg) opacity(0.5) drop-shadow(2px 3px 4px #000000)",
	}}
	s := parseStyle(n, rt)
	if s.FilterGrayscale != 1 {
		t.Errorf("grayscale = %v, want 1", s.FilterGrayscale)
	}
	if s.FilterHueRotate != 90 {
		t.Errorf("hue-rotate = %v, want 90", s.FilterHueRotate)
	}
	if s.FilterOpacity != 0.5 {
		t.Errorf("opacity filter = %v, want 0.5", s.FilterOpacity)
	}
	if s.DropShadowX != 2 || s.DropShadowY != 3 || s.DropShadowBlur != 4 {
		t.Errorf("drop-shadow nums = %v,%v,%v", s.DropShadowX, s.DropShadowY, s.DropShadowBlur)
	}
	if s.DropShadowColor.A == 0 {
		t.Error("drop-shadow color must parse")
	}
}

func TestFilterGrayscaleFull(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Grayscale: 1, Opacity: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(12, 12)
	if abs8(c.R, c.G) > 25 || abs8(c.G, c.B) > 25 {
		t.Fatalf("grayscale(1) must be near-gray, got %v", c)
	}
}

func TestMixBlendMultiply(t *testing.T) {
	// White background + red multiply layer → red-ish (multiply with white = source).
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ops := &op.Ops{}
	// Base green fill
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	ops.Add(op.PaintOp{})
	// Red multiply layer
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1, BlendMode: "multiply"})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(8, 8, 24, 24)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(16, 16)
	// multiply(red, green) → black-ish (0,0,0) channels product
	if c.R > 40 || c.G > 40 || c.B > 40 {
		t.Fatalf("multiply red×green should be near black, got %v", c)
	}
}

func TestDropShadowFilterDarkensOffset(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{
		Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1,
		DropShadowX: 4, DropShadowY: 4, DropShadowBlur: 2,
		DropShadowColor: color.RGBA{0, 0, 0, 200},
	})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 255, 255, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(10, 10, 26, 26)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	// Below-right of the white square should pick up shadow (darker than pure white).
	if c := img.RGBAAt(28, 28); c.R > 250 {
		t.Fatalf("drop-shadow should darken offset pixel, got %v", c)
	}
}

func TestLayoutFLIPTweensPosition(t *testing.T) {
	delete(globalFLIP, "flip-a")
	// Seed settled position.
	applyLayoutFLIP("flip-a", 0, 0, 20, 20, 200*time.Millisecond, "easeOut")
	// Jump to new box — should start animating.
	dx, dy, _, _, run := applyLayoutFLIP("flip-a", 100, 50, 20, 20, 200*time.Millisecond, "easeOut")
	if !run {
		t.Fatal("FLIP must run after a position jump")
	}
	// At t≈0 visual still near origin: dx ≈ 0-100 = -100
	if dx > -50 {
		t.Errorf("early FLIP dx = %v, want near -100 (still near from)", dx)
	}
	if dy > -20 {
		t.Errorf("early FLIP dy = %v, want near -50", dy)
	}
}

func TestDirtyRegionPartialClear(t *testing.T) {
	// Full render paints a red square; partial dirty only on left should leave
	// right-side red from the previous buffer when we re-render only left.
	buf := image.NewRGBA(image.Rect(0, 0, 40, 20))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 40, 20)})
	ops.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(ops, buf)
	if c := buf.RGBAAt(30, 10); c.R != 255 {
		t.Fatalf("setup: right pixel red, got %v", c)
	}
	// Partial: only clear/redraw left half as blue.
	ops2 := &op.Ops{}
	ops2.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
	ops2.Add(op.ClipOp{Rect: image.Rect(0, 0, 20, 20)})
	ops2.Add(op.PaintOp{})
	SoftwareRenderer{Dirty: image.Rect(0, 0, 20, 20)}.Render(ops2, buf)
	if c := buf.RGBAAt(10, 10); c.B < 200 {
		t.Errorf("dirty left should be blue, got %v", c)
	}
	if c := buf.RGBAAt(30, 10); c.R < 200 {
		t.Errorf("outside dirty must keep previous red, got %v", c)
	}
}

func TestMixBlendModeStyleParse(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{"mixBlendMode": "screen"}}
	s := parseStyle(n, rt)
	if s.MixBlendMode != "screen" {
		t.Errorf("mixBlendMode = %q", s.MixBlendMode)
	}
}
