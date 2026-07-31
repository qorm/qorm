package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/op"
)

// SoftwareRenderer must satisfy the Renderer contract.
var _ Renderer = SoftwareRenderer{}

// buildTestOps records a display list exercising the main op kinds:
// filled rect, rounded stroke, opacity and text.
func buildTestOps() *op.Ops {
	ops := &op.Ops{}

	// Filled blue square at (4,4)-(20,20).
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})

	// Rounded red stroke ring around (30,4)-(50,24), radius 6, width 2.
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.StrokeOp{Width: 2})
	ops.Add(op.ClipOp{Rect: image.Rect(30, 4, 50, 24), Radius: 6})
	ops.Add(op.StrokePaintOp{})
	ops.Add(op.RestoreOp{})

	// Half-transparent green fill at (4,30)-(60,44).
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.OpacityOp{Alpha: 0.5})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 30, 60, 44)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})

	// Text.
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.TextOp{Text: "AB", Pos: image.Pt(4, 46), Scale: 1})

	return ops
}

// The SoftwareRenderer.Render path must produce exactly the same pixels as
// the Rasterize convenience wrapper — the frame loop switches from one to the
// other without any visible change.
func TestSoftwareRendererMatchesRasterize(t *testing.T) {
	size := image.Pt(64, 56)
	want := Rasterize(buildTestOps(), size)

	got := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	SoftwareRenderer{}.Render(buildTestOps(), got)

	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

// Render must start every frame from a clean white buffer — a persistent
// Surface buffer is reused across frames and may never carry residue.
func TestRenderCleansTargetEachFrame(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 32, 32))

	frame1 := &op.Ops{}
	frame1.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	frame1.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	frame1.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(frame1, buf)
	if c := buf.RGBAAt(16, 16); c.R != 255 || c.G != 0 {
		t.Fatalf("frame 1 should paint red, got %v", c)
	}

	// Frame 2 draws nothing: the buffer must come back pure white.
	SoftwareRenderer{}.Render(&op.Ops{}, buf)
	if c := buf.RGBAAt(16, 16); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("frame 2 left residue at (16,16): %v", c)
	}
}

// Rendering into a caller-provided buffer must not allocate a second image:
// the returned target is the same memory that was passed in.
func TestRenderDrawsIntoProvidedBuffer(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 8, 8))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{1, 2, 3, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 8, 8)})
	ops.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(ops, buf)
	if c := buf.RGBAAt(0, 0); c != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("provided buffer not drawn into: %v", c)
	}
}
