package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/op"
)

func TestStrokeDasharray(t *testing.T) {
	ops := &op.Ops{}
	ops.Add(op.RRectOp{
		Rect:            image.Rect(10, 10, 30, 30),
		Stroke:          color.RGBA{255, 0, 0, 255},
		StrokeWidth:     2,
		StrokeDasharray: []float64{5, 5},
	})
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	SoftwareRenderer{}.Render(ops, img)

	// The top edge should have dashes.
	// We expect (10,10) to (15,10) to be red, (15,10) to (20,10) to be blank.
	if img.RGBAAt(12, 10).R < 150 {
		t.Errorf("expected dashed red stroke at (12,10), got %v", img.RGBAAt(12, 10))
	}
	if img.RGBAAt(17, 10) != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("expected gap in dashed stroke at (17,10), got %v", img.RGBAAt(17, 10))
	}
}
