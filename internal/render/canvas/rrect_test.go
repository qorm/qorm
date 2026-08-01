package canvas

// Pixel tests for op.RRectOp: per-pixel SDF coverage — antialiased rounded
// corners and smooth (not stepped) shadow falloff.

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/op"
)

func renderRRect(t *testing.T, size image.Point, rr op.RRectOp) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops := &op.Ops{}
	ops.Add(rr)
	SoftwareRenderer{}.Render(ops, img)
	return img
}

func TestRRectCornerAntialiasing(t *testing.T) {
	img := renderRRect(t, image.Pt(40, 40), op.RRectOp{
		Rect: image.Rect(4, 4, 36, 36), Radius: 8,
		Fill: color.RGBA{255, 0, 0, 255},
	})

	// The exact corner must be background (outside the rounded shape).
	if got := img.RGBAAt(4, 4); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("outer corner = %v, want background", got)
	}
	// Along the corner arc there must be intermediate-coverage pixels — the
	// antialiasing band the binary clip path never produced.
	mid := 0
	for y := 4; y < 14; y++ {
		for x := 4; x < 14; x++ {
			// Red fill over the white scene: R stays 255, so intermediate
			// coverage shows up in the green channel.
			if c := img.RGBAAt(x, y); c.G > 0 && c.G < 255 {
				mid++
			}
		}
	}
	if mid == 0 {
		t.Error("no intermediate-coverage pixels on the corner arc — edges are still binary")
	}
	// The body is fully opaque.
	if got := img.RGBAAt(20, 20); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("body = %v, want opaque fill", got)
	}
}

func TestRRectShadowIsSmoothNotStepped(t *testing.T) {
	img := renderRRect(t, image.Pt(60, 60), op.RRectOp{
		Rect: image.Rect(10, 10, 40, 40), Radius: 4,
		Fill:       color.RGBA{255, 255, 255, 255},
		Shadow:     color.RGBA{0, 0, 0, 128},
		ShadowBlur: 8,
		ShadowY:    2,
	})

	// The buffer is opaque, so shadow strength shows as DARKNESS: marching
	// outward below the bottom edge, luminance must INCREASE monotonically
	// (the shadow melts out) and take more than the old fake's 3 steps.
	var lum []uint32
	for y := 41; y < 55; y++ {
		r, _, _, _ := img.At(25, y).RGBA()
		lum = append(lum, r>>8)
	}
	distinct := map[uint32]bool{}
	for i, v := range lum {
		if v < 255 {
			distinct[v] = true
		}
		if i > 0 && v < lum[i-1] {
			t.Errorf("shadow gets darker outward at y=%d: %v", 41+i, lum)
		}
	}
	if len(distinct) < 5 {
		t.Errorf("shadow has only %d distinct levels (old fake had 3) — not smooth: %v", len(distinct), lum)
	}
	// The shadow hangs below the shape (ShadowY=2): nothing above the top.
	if r, _, _, _ := img.At(25, 2).RGBA(); r>>8 < 250 {
		t.Errorf("unexpected shadow above the shape (R=%d)", r>>8)
	}
}

func TestRRectStrokeRingWithAA(t *testing.T) {
	img := renderRRect(t, image.Pt(40, 40), op.RRectOp{
		Rect: image.Rect(5, 5, 35, 35), Radius: 6,
		Fill:        color.RGBA{255, 255, 255, 255},
		Stroke:      color.RGBA{0, 0, 255, 255},
		StrokeWidth: 2,
	})
	// Mid-edge: the ring is blue.
	if got := img.RGBAAt(20, 6); got.B < 200 {
		t.Errorf("stroke ring pixel = %v, want blue ring", got)
	}
	// Deep inside: white fill.
	if got := img.RGBAAt(20, 20); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("inner = %v, want fill", got)
	}
	// Outside: background.
	if got := img.RGBAAt(20, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("outer = %v, want background", got)
	}
}

func TestSDRoundBox(t *testing.T) {
	// 10x10 box at origin, radius 2.
	if d := sdRoundBox(0, 0, 0, 0, 5, 5, 2); d >= 0 {
		t.Errorf("center d=%v, want < 0", d)
	}
	if d := sdRoundBox(5, 0, 0, 0, 5, 5, 2); d != 0 {
		t.Errorf("edge midpoint d=%v, want 0", d)
	}
	if d := sdRoundBox(9, 9, 0, 0, 5, 5, 2); d <= 0 {
		t.Errorf("far corner d=%v, want > 0", d)
	}
	// Radius clamped to 0 behaves as a sharp box.
	if d := sdRoundBox(5.5, 5.5, 0, 0, 5, 5, 0); d <= 0 {
		t.Errorf("sharp corner point d=%v, want > 0 (outside sharp corner)", d)
	}
}
