package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/op"
)

func TestFilterContrastAndHueRotate(t *testing.T) {
	ops := &op.Ops{}

	ops.Add(op.LayerOp{Contrast: 0.5, HueRotate: 180, Brightness: 1, Saturate: 1, Opacity: 1})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 10, 10)})

	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(5, 5)

	// HueRotate 180 on pure red should yield cyan, and contrast 0.5 reduces it.
	if c.R > 150 {
		t.Errorf("expected R to be low due to hue rotation and contrast, got %v", c.R)
	}
	if c.G == 0 || c.B == 0 {
		t.Errorf("expected G and B to be elevated, got %v", c)
	}
}
