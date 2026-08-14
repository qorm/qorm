package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/op"
)

func TestPlusLighterRedOverGreen(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	ops.Add(op.PaintOp{})
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1, BlendMode: "plus-lighter"})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(16, 16)
	if c.R < 200 || c.G < 200 || c.B > 40 {
		t.Fatalf("plus-lighter red over green should be yellow (1,1,0), got %v", c)
	}
}

func TestLighterAliasMatchesPlusLighter(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	dst.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	blendModeOver(dst, 0, 0, color.RGBA{255, 0, 0, 255}, "lighter")
	c := dst.RGBAAt(0, 0)
	if c.R < 200 || c.G < 200 || c.B > 40 {
		t.Fatalf("lighter alias red over green = %v, want yellow", c)
	}
}
