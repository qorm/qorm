//go:build !qorm_nocjk

package canvas

import (
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/qorm/qorm/internal/op"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// requireEngine skips when the embedded subset font is absent (checkout
// before CI ran scripts/subset-font.sh): the bitmap fallback is correct
// behaviour then, and the asset-dependent assertions have nothing to test.
func requireEngine(t *testing.T) *sfntEngine {
	t.Helper()
	e := sharedEngine()
	if e == nil {
		t.Skip("subset font asset not embedded; bitmap fallback in effect")
	}
	return e
}

// swapEngine replaces the lazy-load globals for one test and restores them
// after. Tests using it must not run in parallel. The Once is replaced (not
// copied back): its only state is "done", and restoring a fresh one plus the
// old instance is equivalent.
func swapEngine(t *testing.T, parse func() (*sfnt.Font, error)) {
	t.Helper()
	oldInst, oldParse := engineInst, parseFont
	engineOnce, engineInst, parseFont = sync.Once{}, nil, parse
	t.Cleanup(func() {
		engineOnce, engineInst, parseFont = sync.Once{}, oldInst, oldParse
	})
}

func TestSFNTEngineAvailable(t *testing.T) {
	e := requireEngine(t) // skips when the asset is absent
	if e.font == nil {
		t.Fatal("engine parsed but holds no font")
	}
}

func TestSFNTLazyLoadsOnce(t *testing.T) {
	origParse := parseFont
	if _, err := origParse(); err != nil {
		t.Skip("subset font asset not embedded; nothing to lazy-load")
	}
	parses := 0
	swapEngine(t, func() (*sfnt.Font, error) {
		parses++
		return origParse()
	})
	for i := 0; i < 3; i++ {
		if sharedEngine() == nil {
			t.Fatal("engine must load from the checked-in asset")
		}
	}
	if parses != 1 {
		t.Errorf("font parsed %d times, want exactly 1 (lazy once)", parses)
	}
}

func TestSFNTParseFailureIsNegativelyCached(t *testing.T) {
	parses := 0
	swapEngine(t, func() (*sfnt.Font, error) {
		parses++
		return nil, errors.New("boom")
	})
	for i := 0; i < 3; i++ {
		if sharedEngine() != nil {
			t.Fatal("engine must stay nil after a parse failure")
		}
	}
	if parses != 1 {
		t.Errorf("parse attempted %d times, want exactly 1 (negative cache)", parses)
	}
	// With no engine the public entry points must keep the bitmap behaviour.
	if w := MeasureText("AB", 10); w != 12 {
		t.Errorf("bitmap fallback MeasureText(AB, 10) = %v, want 12", w)
	}
}

func TestSFNTMeasureCJKAndASCII(t *testing.T) {
	requireEngine(t)
	// CJK runes are full-width in the subset font — same answer the bitmap
	// metrics give, so layout does not shift when the engine turns on.
	if w := MeasureText("中文", 16); math.Abs(w-32) > 0.5 {
		t.Errorf("MeasureText(中文, 16) = %v, want ~32", w)
	}
	if got, want := MeasureText("中文", 16), (bitmapMeasurer{}).Measure("中文", 16); got != want {
		t.Errorf("sfnt %v != bitmap %v for full-width CJK", got, want)
	}
	// ASCII advance comes from the font now (~0.5-0.7em), not the bitmap
	// constant 0.6em — assert the sane band, not an exact font-version value.
	if w := MeasureText("A", 16); w < 8 || w > 12 {
		t.Errorf("MeasureText(A, 16) = %v, want within [8,12]", w)
	}
	// Clamp still applies on the sfnt path.
	if w := MeasureText("A", 1e6); w != MeasureText("A", maxFontSize) {
		t.Errorf("MeasureText(A, 1e6) = %v, want clamped %v", w, MeasureText("A", maxFontSize))
	}
}

func TestSFNTMissingGlyphMeasuresFullWidth(t *testing.T) {
	requireEngine(t)
	// U+2000B (CJK Ext B) is outside the GB2312 subset.
	if w := MeasureText("\U0002000B", 16); math.Abs(w-16) > 0.01 {
		t.Errorf("MeasureText(U+2000B, 16) = %v, want 16 (full-width fallback)", w)
	}
}

func TestSFNTDrawsRealCJKGlyph(t *testing.T) {
	requireEngine(t)
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	DrawText(img, "中", image.Pt(2, 2), color.RGBA{0, 0, 0, 255}, 1.6, nil) // fontSize 16

	inked := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if img.RGBAAt(x, y) != (color.RGBA{}) {
				inked++
			}
		}
	}
	// A real 中 at 16px inks dozens of pixels; the bitmap '?' would ink 9-25.
	if inked < 40 {
		t.Errorf("中 inked %d px, want >= 40 (real glyph, not the '?' fallback)", inked)
	}
}

func TestSFNTMissingGlyphDrawsBitmapQuestionMark(t *testing.T) {
	requireEngine(t)
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	DrawText(img, "\U0002000B", image.Pt(0, 0), color.RGBA{0, 0, 0, 255}, 1.0, nil) // fontSize 10

	// Exactly the phase-1 '?' bitmap: 9 pixels in the 5x7 cell at the origin.
	inked := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			if img.RGBAAt(x, y) != (color.RGBA{}) {
				inked++
			}
		}
	}
	if inked != 9 {
		t.Errorf("missing glyph inked %d px, want the 9 px of the bitmap '?'", inked)
	}
	if !inkedAt(img, 0, 1) || inkedAt(img, 0, 0) {
		t.Error("fallback glyph must sit at the bitmap '?' position")
	}
}

func inkedAt(img *image.RGBA, x, y int) bool {
	return img.RGBAAt(x, y) != (color.RGBA{})
}

func TestSFNTDrawWithinMeasuredWidth(t *testing.T) {
	requireEngine(t)
	const text = "Hello,世界"
	const fontSize = 14.0
	img := image.NewRGBA(image.Rect(0, 0, 200, 40))
	DrawText(img, text, image.Pt(0, 0), color.RGBA{0, 0, 0, 255}, fontSize/10, nil)

	maxX := -1
	for y := 0; y < 40; y++ {
		for x := 0; x < 200; x++ {
			if img.RGBAAt(x, y) != (color.RGBA{}) && x > maxX {
				maxX = x
			}
		}
	}
	if maxX < 0 {
		t.Fatal("nothing drawn")
	}
	// The drawn ink must stay inside the measured advance (allowing the last
	// glyph's right side bearing to be one pixel tight).
	if limit := int(math.Ceil(MeasureText(text, fontSize))) + 1; maxX > limit {
		t.Errorf("ink reaches x=%d, past measured width %d", maxX, limit)
	}
}

func TestSFNTDrawClippedAndOffscreen(t *testing.T) {
	requireEngine(t)
	black := color.RGBA{0, 0, 0, 255}

	// Partially offscreen (negative origin) must not panic and must keep the
	// on-screen pixels identical to the unclipped render — the mask index
	// math must use the glyph rect, not the clipped one.
	full := image.NewRGBA(image.Rect(0, 0, 40, 40))
	DrawText(full, "中", image.Pt(2, 2), black, 1.6, nil)
	off := image.NewRGBA(image.Rect(0, 0, 40, 40))
	DrawText(off, "中", image.Pt(0, 2), black, 1.6, nil)
	DrawText(off, "中", image.Pt(-100, -100), black, 1.6, nil) // fully offscreen: no panic
	for y := 0; y < 40; y++ {
		for x := 2; x < 40; x++ {
			if full.RGBAAt(x, y) != off.RGBAAt(x-2, y) {
				t.Fatalf("pixel (%d,%d) mismatch vs shifted render", x, y)
			}
		}
	}

	// A clip rect must bound the ink: nothing outside it.
	clipped := image.NewRGBA(image.Rect(0, 0, 40, 40))
	clip := image.Rect(8, 8, 20, 20)
	DrawText(clipped, "中", image.Pt(2, 2), black, 1.6, []op.ClipOp{{Rect: clip}})
	inside := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if clipped.RGBAAt(x, y) != (color.RGBA{}) {
				if !(image.Pt(x, y).In(clip)) {
					t.Fatalf("ink at (%d,%d) outside clip %v", x, y, clip)
				}
				inside++
			}
		}
	}
	if inside == 0 {
		t.Error("clip interior must still be inked")
	}
}

func TestDefaultTextMeasurerUsesSFNT(t *testing.T) {
	requireEngine(t)
	if _, isBitmap := DefaultTextMeasurer.(bitmapMeasurer); isBitmap {
		t.Fatal("DefaultTextMeasurer must be swapped off the bitmap measurer")
	}
	if got, want := DefaultTextMeasurer.Measure("中文", 16), MeasureText("中文", 16); got != want {
		t.Errorf("DefaultTextMeasurer %v != MeasureText %v", got, want)
	}
}

// The sfnt pipeline must not be welded to the embedded asset: any TTF/OTF on
// disk parses the same way. Uses a macOS system font purely at test time
// (never shipped); skips where it is absent.
func TestSFNTParsesArbitrarySystemFont(t *testing.T) {
	data, err := os.ReadFile("/System/Library/Fonts/Supplemental/Arial.ttf")
	if err != nil {
		t.Skip("system test font not present")
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatalf("sfnt.Parse(system font): %v", err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 16, DPI: 72})
	if err != nil {
		t.Fatalf("NewFace: %v", err)
	}
	defer face.Close()
	if adv, ok := face.GlyphAdvance('A'); !ok || adv <= 0 {
		t.Errorf("GlyphAdvance(A) = %v, %v; want positive", adv, ok)
	}
}

// fontWeight >= 600 emboldens synthetically (faux-bold second pass): bold
// text carries measurably more ink than the same string at 400.
func TestDrawTextWeightedEmboldens(t *testing.T) {
	ink := func(weight int) int {
		img := image.NewRGBA(image.Rect(0, 0, 120, 40))
		DrawTextWeighted(img, "Bold", image.Pt(2, 2), color.RGBA{0, 0, 0, 255}, 1.4, weight, nil)
		n := 0
		for y := 0; y < 40; y++ {
			for x := 0; x < 120; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a > 0 {
					n++
				}
			}
		}
		return n
	}
	normal, bold := ink(400), ink(700)
	if bold <= normal {
		t.Errorf("bold ink %d <= normal %d — emboldening did nothing", bold, normal)
	}
	// 500 stays normal (CSS's semi-bold threshold starts at 600 here).
	if ink(500) != normal {
		t.Errorf("weight 500 should not embolden: %d vs %d", ink(500), normal)
	}
}
