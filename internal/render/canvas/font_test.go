//go:build qorm_nocjk

// These cases pin the phase-1 bitmap font behaviour, which holds when no TTF
// engine is active — i.e. exactly the builds with the qorm_nocjk tag (the
// sfnt opt-out). The default-build equivalents live in font_sfnt_test.go.
package canvas

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"
)

// Compile-time check: the built-in bitmap metrics satisfy the TextMeasurer
// seam phase 2 will plug a TTF-backed implementation into.
var _ TextMeasurer = bitmapMeasurer{}

func TestMeasureTextASCII(t *testing.T) {
	// ASCII runes are half-width: 0.6 * fontSize each, counted per rune.
	if w := MeasureText("hello", 10); w != 30 {
		t.Errorf("MeasureText(hello, 10) = %v, want 30", w)
	}
	if w := MeasureText("", 14); w != 0 {
		t.Errorf("MeasureText(empty, 14) = %v, want 0", w)
	}
}

func TestMeasureTextCJKMixedWidth(t *testing.T) {
	// East Asian wide runes are full-width (1.0 * fontSize), one advance per
	// rune — not per byte (len("中") == 3 was the old ~5x overestimate).
	if w := MeasureText("中文", 10); w != 20 {
		t.Errorf("MeasureText(中文, 10) = %v, want 20", w)
	}
	// Mixed CJK + ASCII: 10 + 6 + 10 + 6.
	if w := MeasureText("中A文B", 10); w != 32 {
		t.Errorf("MeasureText(中A文B, 10) = %v, want 32", w)
	}
}

func TestMeasureTextClampsFontSize(t *testing.T) {
	start := time.Now()
	if w := MeasureText("A", 1e6); w != 0.6*maxFontSize {
		t.Errorf("MeasureText(A, 1e6) = %v, want %v (clamped)", w, 0.6*maxFontSize)
	}
	if w := MeasureText("A", -100); w != 0.6*minFontSize {
		t.Errorf("MeasureText(A, -100) = %v, want %v (clamped)", w, 0.6*minFontSize)
	}
	if w := MeasureText("A", math.NaN()); w != 0.6*minFontSize {
		t.Errorf("MeasureText(A, NaN) = %v, want %v (clamped)", w, 0.6*minFontSize)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("clamped measure took %v, want milliseconds", d)
	}
}

// inked reports whether DrawText wrote pixel (x, y) (buffer starts zeroed).
func inked(img *image.RGBA, x, y int) bool {
	return img.RGBAAt(x, y) != (color.RGBA{})
}

// The drawn pen advance and the layout measurement must come from the same
// source: glyph origins sit at floor(k * per-rune advance), and MeasureText
// is exactly the sum of those advances.
func TestDrawTextAdvanceMatchesMeasure(t *testing.T) {
	// fontSize 14 (not a multiple of 10) used to quantize the drawn advance
	// to 6px while the layout measured 8.4px — the old draw/measure split.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	black := color.RGBA{0, 0, 0, 255}
	DrawText(img, "AB", image.Pt(0, 0), black, 1.4, nil) // fontSize 14

	// 'B' column 0 is 0x7f (all rows lit); with the shared metric it starts
	// at x = floor(0.6*14) = 8. The old quantized advance started it at 6.
	if !inked(img, 8, 1) {
		t.Error("glyph B must start at x=8 (floor of the MeasureText advance)")
	}
	if inked(img, 6, 1) {
		t.Error("x=6 must be blank: advance is 8.4px/rune, not the old 6px quantization")
	}

	// Measured width is the sum of the same per-rune advances the pen used.
	if w, want := MeasureText("AB", 14), runeAdvance('A', 14)+runeAdvance('B', 14); w != want {
		t.Errorf("MeasureText(AB, 14) = %v, want %v (sum of drawn advances)", w, want)
	}
}

// Phase 1 still renders non-ASCII as '?', but a CJK rune is ONE '?' at a
// full-width advance — not three '?' glyphs at byte offsets.
func TestDrawTextCJKRuneAdvance(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	black := color.RGBA{0, 0, 0, 255}
	DrawText(img, "中A", image.Pt(0, 0), black, 1.0, nil) // fontSize 10

	// 'A' (column 4 is 0x7e, rows 1-6 lit) must sit at x=10..14, right after
	// one full-width advance for 中. Byte-based advance placed it at x=18.
	if !inked(img, 14, 1) {
		t.Error("A after 中 must start at x=10 (one full-width rune advance)")
	}
	// The '?' for 中 inks x=0..4 only; x=6 must be blank (the old byte loop
	// inked a second '?' there for the UTF-8 continuation bytes).
	if inked(img, 6, 1) {
		t.Error("x=6 must be blank: 中 is one rune, not three bytes")
	}
}

// Redteam R1 P1-1: fontSize=1e6 pushed one frame past the 5s watchdog
// (intScale^2 pixel loop, no upper bound). Clamped to maxFontSize the same
// call returns in milliseconds and inks a bounded area.
func TestDrawTextFontSizeClamp(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2000, 500))
	black := color.RGBA{0, 0, 0, 255}

	start := time.Now()
	DrawText(img, "Hi", image.Pt(0, 0), black, 1e5, nil) // fontSize 1e6
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("DrawText with fontSize=1e6 took %v; clamp must keep it in milliseconds", d)
	}

	// Drew something at the clamped size (intScale = maxFontSize/10 = 51).
	if !inked(img, 10, 10) {
		t.Error("clamped text must still render")
	}
	// Nothing beyond the clamped extent: 2 full-width advances + a glyph.
	maxX := int(2*maxFontSize) + 5*(int(maxFontSize)/10)
	for y := 0; y < 500; y++ {
		for x := maxX + 1; x < 2000; x++ {
			if inked(img, x, y) {
				t.Fatalf("pixel (%d,%d) inked beyond clamped extent %d", x, y, maxX)
			}
		}
	}
}

func TestDefaultTextMeasurer(t *testing.T) {
	var m TextMeasurer = DefaultTextMeasurer
	if w := m.Measure("AB", 10); w != 12 {
		t.Errorf("DefaultTextMeasurer.Measure(AB, 10) = %v, want 12", w)
	}
}
