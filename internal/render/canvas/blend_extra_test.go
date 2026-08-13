package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
)

func TestMixBlendDifference(t *testing.T) {
	// Green backdrop + red difference layer. CSS |Cs-Cb| → yellow (1,1,0),
	// not black (multiply). Raster through the software LayerOp path.
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	ops.Add(op.PaintOp{})
	ops.Add(op.LayerOp{Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1, BlendMode: "difference"})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, img)
	c := img.RGBAAt(16, 16)
	if c.R < 200 || c.G < 200 || c.B > 40 {
		t.Fatalf("difference red×green should be near yellow, got %v", c)
	}
}

func TestParseMixBlendColorDodge(t *testing.T) {
	n := &model.Node{Type: "box", Style: map[string]any{"mixBlendMode": "color-dodge"}}
	s := parseStyle(n, testRuntime(nil))
	if s.MixBlendMode != "color-dodge" {
		t.Errorf("mixBlendMode = %q, want color-dodge", s.MixBlendMode)
	}
}

func TestBlendModeExtraFormulas(t *testing.T) {
	// color-dodge: Cb=0.5, Cs=0.5 → min(1, 0.5/0.5) = 1
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	dst.SetRGBA(0, 0, color.RGBA{128, 128, 128, 255})
	blendModeOver(dst, 0, 0, color.RGBA{128, 128, 128, 255}, "color-dodge")
	if c := dst.RGBAAt(0, 0); c.R < 240 {
		t.Errorf("color-dodge mid-gray = %v, want near white", c)
	}

	// color-burn: Cb=1 → 1
	dst.SetRGBA(0, 0, color.RGBA{255, 255, 255, 255})
	blendModeOver(dst, 0, 0, color.RGBA{10, 10, 10, 255}, "color-burn")
	if c := dst.RGBAAt(0, 0); c.R < 250 {
		t.Errorf("color-burn on white = %v, want white", c)
	}

	// exclusion on 0/1 primaries matches difference → yellow
	dst.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	blendModeOver(dst, 0, 0, color.RGBA{255, 0, 0, 255}, "exclusion")
	if c := dst.RGBAAt(0, 0); c.R < 200 || c.G < 200 || c.B > 40 {
		t.Errorf("exclusion red×green = %v, want near yellow", c)
	}
}
