package canvas

import (
	"image"
	"image/color"
	"math"

	"github.com/qorm/qorm/internal/op"
)

// Font5x7 is a simple built-in 5x7 ASCII bitmap font.
// Each character is 5 pixels wide and 7 pixels high.
// The array contains 96 characters, starting from ASCII 32 (space) to 127.
var font5x7 = [96][5]uint8{
	// 32: Space
	{0x00, 0x00, 0x00, 0x00, 0x00},
	// 33: !
	{0x00, 0x00, 0x5f, 0x00, 0x00},
	// 34: "
	{0x00, 0x07, 0x00, 0x07, 0x00},
	// 35: #
	{0x14, 0x7f, 0x14, 0x7f, 0x14},
	// 36: $
	{0x24, 0x2a, 0x7f, 0x2a, 0x12},
	// 37: %
	{0x23, 0x13, 0x08, 0x64, 0x62},
	// 38: &
	{0x36, 0x49, 0x55, 0x22, 0x50},
	// 39: '
	{0x00, 0x05, 0x03, 0x00, 0x00},
	// 40: (
	{0x00, 0x1c, 0x22, 0x41, 0x00},
	// 41: )
	{0x00, 0x41, 0x22, 0x1c, 0x00},
	// 42: *
	{0x08, 0x2a, 0x1c, 0x2a, 0x08},
	// 43: +
	{0x08, 0x08, 0x3e, 0x08, 0x08},
	// 44: ,
	{0x00, 0x50, 0x30, 0x00, 0x00},
	// 45: -
	{0x08, 0x08, 0x08, 0x08, 0x08},
	// 46: .
	{0x00, 0x60, 0x60, 0x00, 0x00},
	// 47: /
	{0x20, 0x10, 0x08, 0x04, 0x02},
	// 48: 0
	{0x3e, 0x51, 0x49, 0x45, 0x3e},
	// 49: 1
	{0x00, 0x42, 0x7f, 0x40, 0x00},
	// 50: 2
	{0x42, 0x61, 0x51, 0x49, 0x46},
	// 51: 3
	{0x21, 0x41, 0x45, 0x4b, 0x31},
	// 52: 4
	{0x18, 0x14, 0x12, 0x7f, 0x10},
	// 53: 5
	{0x27, 0x45, 0x45, 0x45, 0x39},
	// 54: 6
	{0x3c, 0x4a, 0x49, 0x49, 0x30},
	// 55: 7
	{0x01, 0x71, 0x09, 0x05, 0x03},
	// 56: 8
	{0x36, 0x49, 0x49, 0x49, 0x36},
	// 57: 9
	{0x06, 0x49, 0x49, 0x29, 0x1e},
	// 58: :
	{0x00, 0x36, 0x36, 0x00, 0x00},
	// 59: ;
	{0x00, 0x56, 0x36, 0x00, 0x00},
	// 60: <
	{0x00, 0x08, 0x14, 0x22, 0x41},
	// 61: =
	{0x14, 0x14, 0x14, 0x14, 0x14},
	// 62: >
	{0x41, 0x22, 0x14, 0x08, 0x00},
	// 63: ?
	{0x02, 0x01, 0x51, 0x09, 0x06},
	// 64: @
	{0x32, 0x49, 0x79, 0x41, 0x3e},
	// 65: A
	{0x7e, 0x11, 0x11, 0x11, 0x7e},
	// 66: B
	{0x7f, 0x49, 0x49, 0x49, 0x36},
	// 67: C
	{0x3e, 0x41, 0x41, 0x41, 0x22},
	// 68: D
	{0x7f, 0x41, 0x41, 0x22, 0x1c},
	// 69: E
	{0x7f, 0x49, 0x49, 0x49, 0x41},
	// 70: F
	{0x7f, 0x09, 0x09, 0x01, 0x01},
	// 71: G
	{0x3e, 0x41, 0x41, 0x51, 0x32},
	// 72: H
	{0x7f, 0x08, 0x08, 0x08, 0x7f},
	// 73: I
	{0x00, 0x41, 0x7f, 0x41, 0x00},
	// 74: J
	{0x20, 0x40, 0x41, 0x3f, 0x01},
	// 75: K
	{0x7f, 0x08, 0x14, 0x22, 0x41},
	// 76: L
	{0x7f, 0x40, 0x40, 0x40, 0x40},
	// 77: M
	{0x7f, 0x02, 0x04, 0x02, 0x7f},
	// 78: N
	{0x7f, 0x04, 0x08, 0x10, 0x7f},
	// 79: O
	{0x3e, 0x41, 0x41, 0x41, 0x3e},
	// 80: P
	{0x7f, 0x09, 0x09, 0x09, 0x06},
	// 81: Q
	{0x3e, 0x41, 0x51, 0x21, 0x5e},
	// 82: R
	{0x7f, 0x09, 0x19, 0x29, 0x46},
	// 83: S
	{0x46, 0x49, 0x49, 0x49, 0x31},
	// 84: T
	{0x01, 0x01, 0x7f, 0x01, 0x01},
	// 85: U
	{0x3f, 0x40, 0x40, 0x40, 0x3f},
	// 86: V
	{0x1f, 0x20, 0x40, 0x20, 0x1f},
	// 87: W
	{0x7f, 0x20, 0x18, 0x20, 0x7f},
	// 88: X
	{0x63, 0x14, 0x08, 0x14, 0x63},
	// 89: Y
	{0x03, 0x04, 0x78, 0x04, 0x03},
	// 90: Z
	{0x61, 0x51, 0x49, 0x45, 0x43},
	// 91: [
	{0x00, 0x00, 0x7f, 0x41, 0x41},
	// 92: \
	{0x02, 0x04, 0x08, 0x10, 0x20},
	// 93: ]
	{0x41, 0x41, 0x7f, 0x00, 0x00},
	// 94: ^
	{0x04, 0x02, 0x01, 0x02, 0x04},
	// 95: _
	{0x40, 0x40, 0x40, 0x40, 0x40},
	// 96: `
	{0x00, 0x01, 0x02, 0x04, 0x00},
	// 97: a
	{0x20, 0x54, 0x54, 0x54, 0x78},
	// 98: b
	{0x7f, 0x48, 0x44, 0x44, 0x38},
	// 99: c
	{0x38, 0x44, 0x44, 0x44, 0x20},
	// 100: d
	{0x38, 0x44, 0x44, 0x48, 0x7f},
	// 101: e
	{0x38, 0x54, 0x54, 0x54, 0x18},
	// 102: f
	{0x08, 0x7e, 0x09, 0x01, 0x02},
	// 103: g
	{0x08, 0x14, 0x54, 0x54, 0x3c},
	// 104: h
	{0x7f, 0x08, 0x04, 0x04, 0x78},
	// 105: i
	{0x00, 0x44, 0x7d, 0x40, 0x00},
	// 106: j
	{0x20, 0x40, 0x44, 0x3d, 0x00},
	// 107: k
	{0x7f, 0x10, 0x28, 0x44, 0x00},
	// 108: l
	{0x00, 0x41, 0x7f, 0x40, 0x00},
	// 109: m
	{0x7c, 0x04, 0x18, 0x04, 0x78},
	// 110: n
	{0x7c, 0x08, 0x04, 0x04, 0x78},
	// 111: o
	{0x38, 0x44, 0x44, 0x44, 0x38},
	// 112: p
	{0x7c, 0x14, 0x14, 0x14, 0x08},
	// 113: q
	{0x08, 0x14, 0x14, 0x18, 0x7c},
	// 114: r
	{0x7c, 0x08, 0x04, 0x04, 0x08},
	// 115: s
	{0x48, 0x54, 0x54, 0x54, 0x20},
	// 116: t
	{0x04, 0x3f, 0x44, 0x40, 0x20},
	// 117: u
	{0x3c, 0x40, 0x40, 0x20, 0x7c},
	// 118: v
	{0x1c, 0x20, 0x40, 0x20, 0x1c},
	// 119: w
	{0x3c, 0x40, 0x30, 0x40, 0x3c},
	// 120: x
	{0x44, 0x28, 0x10, 0x28, 0x44},
	// 121: y
	{0x0c, 0x50, 0x50, 0x50, 0x3c},
	// 122: z
	{0x44, 0x64, 0x54, 0x4c, 0x44},
	// 123: {
	{0x00, 0x08, 0x36, 0x41, 0x00},
	// 124: |
	{0x00, 0x00, 0x7f, 0x00, 0x00},
	// 125: }
	{0x00, 0x41, 0x36, 0x08, 0x00},
	// 126: ~
	{0x08, 0x04, 0x08, 0x10, 0x08},
	// 127: DEL
	{0x00, 0x00, 0x00, 0x00, 0x00},
}

// Effective font sizes are clamped to [minFontSize, maxFontSize] at both
// entry points (MeasureText for layout, DrawText for rasterization).
// Lower bound 1 keeps degenerate input (0/negative from app JSON) bounded
// and positive — callers already remap 0 to their default before we see it.
// Upper bound 512 caps the intScale^2 pixel loop in DrawText: at 512 the
// 5x7 bitmap scales by 51, so one glyph costs at most 35*51*51 ≈ 91k pixel
// writes and a full line still rasterizes in milliseconds. Larger sizes add
// no fidelity to a 5x7 bitmap and only widen the DoS surface (redteam R1
// P1-1: fontSize=1e6 stalled a frame past the 5s watchdog).
const (
	minFontSize = 1.0
	maxFontSize = 512.0
)

// clampFontSize bounds fontSize to [minFontSize, maxFontSize]; NaN and
// non-positive input map to the lower bound.
func clampFontSize(fontSize float64) float64 {
	if fontSize < minFontSize || math.IsNaN(fontSize) {
		return minFontSize
	}
	if fontSize > maxFontSize {
		return maxFontSize
	}
	return fontSize
}

// TextMeasurer is the seam for the phase-2 text stack: layout and drawing
// measure through the same implementation, so a TTF-backed measurer can be
// swapped in without touching call sites.
type TextMeasurer interface {
	Measure(text string, fontSize float64) float64
}

// bitmapMeasurer measures with the built-in 5x7 bitmap font metrics.
type bitmapMeasurer struct{}

func (bitmapMeasurer) Measure(text string, fontSize float64) float64 {
	fontSize = clampFontSize(fontSize)
	w := 0.0
	for _, r := range text {
		w += runeAdvance(r, fontSize)
	}
	return w
}

// DefaultTextMeasurer is the built-in TextMeasurer (bitmap font metrics).
// By default font_sfnt.go replaces it with an sfnt-backed measurer
// (bitmap fallback when the embedded font is unusable); the qorm_nocjk
// build tag opts out and keeps this bitmap measurer.
var DefaultTextMeasurer TextMeasurer = bitmapMeasurer{}

// ttfEngine is the phase-2 sfnt-backed text engine. font_sfnt.go installs a
// provider by default; the qorm_nocjk build tag disables it (ttfProvider
// stays nil) and everything keeps the phase-1 bitmap behaviour. Measure and drawText share
// one implementation so measured and drawn widths cannot drift apart.
type ttfEngine interface {
	TextMeasurer
	// drawText renders into img with the same contract as DrawText, but
	// takes the already-clamped font size (not the scale factor).
	drawText(img *image.RGBA, text string, pos image.Point, col color.RGBA, fontSize float64, clips []op.ClipOp)
}

// ttfProvider returns the shared engine, lazily parsing the embedded font on
// first use (a sync.Once inside the provider). A nil provider or a nil
// engine means "no usable TTF": callers fall back to the bitmap path.
var ttfProvider func() ttfEngine

func activeTTFEngine() ttfEngine {
	if ttfProvider == nil {
		return nil
	}
	return ttfProvider()
}

// MeasureText returns the layout width of text at fontSize in pixels — the
// single source of truth for text extents. When a TTF engine is active
// (the default) it answers with sfnt advances; otherwise widths are per
// rune: ASCII and other narrow runes are half-width (0.6*fontSize, matching
// the drawn advance), East Asian wide runes are full-width (1.0*fontSize).
// DrawText advances the pen by the same per-rune amounts, so measured and
// drawn width cannot drift apart.
func MeasureText(text string, fontSize float64) float64 {
	if e := activeTTFEngine(); e != nil {
		return e.Measure(text, fontSize)
	}
	return bitmapMeasurer{}.Measure(text, fontSize)
}

// runeAdvance is the per-rune pen advance shared by MeasureText and DrawText.
func runeAdvance(r rune, fontSize float64) float64 {
	if isWideRune(r) {
		return fontSize
	}
	return fontSize * 0.6
}

// isWideRune reports whether r is an East Asian wide (full-width) rune,
// following the usual wcwidth ranges. Kept table-driven so the engine stays
// dependency-free (no golang.org/x/text).
func isWideRune(r rune) bool {
	for _, wr := range wideRanges {
		if r >= wr[0] && r <= wr[1] {
			return true
		}
	}
	return false
}

var wideRanges = [][2]rune{
	{0x1100, 0x115F},   // Hangul Jamo
	{0x2E80, 0x303E},   // CJK Radicals Supplement .. CJK Symbols and Punctuation
	{0x3041, 0x33FF},   // Hiragana .. CJK Compatibility
	{0x3400, 0x4DBF},   // CJK Unified Ideographs Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA000, 0xA4CF},   // Yi Syllables, Yi Radicals
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE30, 0xFE4F},   // CJK Compatibility Forms
	{0xFF00, 0xFF60},   // Fullwidth Forms
	{0xFFE0, 0xFFE6},   // Fullwidth symbol signs
	{0x20000, 0x2FFFD}, // CJK Extension B and beyond
	{0x30000, 0x3FFFD}, // CJK Extension G
}

// DrawText draws text at pos with the given (already opacity-scaled) color
// into img, honouring the active clip stack (nested clips intersect). Pixels
// are alpha-composited so sub-full alpha text blends over whatever is below.
//
// scale is fontSize/10 (see graph.Text.Draw). When a TTF engine is active
// (the default) runes with real glyphs are rasterized from the embedded
// subset font and runes missing from it fall back to the bitmap '?' at a
// full-width advance. Without an engine, non-ASCII runes render as '?'
// (phase-1 fallback); either way the pen advances per rune by the same
// amounts MeasureText reports, so the layout width matches what is drawn.
func DrawText(img *image.RGBA, text string, pos image.Point, col color.RGBA, scale float64, clips []op.ClipOp) {
	fontSize := clampFontSize(scale * 10)
	if col.A == 0 {
		return
	}
	if e := activeTTFEngine(); e != nil {
		e.drawText(img, text, pos, col, fontSize, clips)
		return
	}
	drawTextBitmap(img, text, pos, col, fontSize, clips)
}

// DrawTextWeighted is DrawText with a CSS font weight: 600+ emboldens
// synthetically — the embedded font ships one weight, so a second pass at a
// small x offset thickens the strokes (classic faux-bold, same advance).
func DrawTextWeighted(img *image.RGBA, text string, pos image.Point, col color.RGBA, scale float64, weight int, clips []op.ClipOp) {
	DrawText(img, text, pos, col, scale, clips)
	if weight >= 600 {
		fontSize := clampFontSize(scale * 10)
		dx := int(fontSize)/24 + 1
		DrawText(img, text, image.Pt(pos.X+dx, pos.Y), col, scale, clips)
	}
}

// drawTextBitmap is the phase-1 rasterizer: one 5x7 bitmap glyph per rune
// ('?' for non-ASCII), advancing by the shared MeasureText metric. A sub-1
// font scale (a zoomed-out board) takes the box-filtering path instead of the
// old integer clamp, which drew full-size text that overflowed its box.
func drawTextBitmap(img *image.RGBA, text string, pos image.Point, col color.RGBA, fontSize float64, clips []op.ClipOp) {
	if scale := fontSize / 10; scale < 1 {
		drawTextBitmapFrac(img, text, pos, col, scale, clips)
		return
	}

	intScale := int(fontSize / 10)
	if intScale < 1 {
		intScale = 1
	}

	y := pos.Y
	penX := float64(pos.X)
	for _, r := range text {
		c := byte(r)
		if r < 32 || r > 127 {
			c = 63 // '?'
		}
		drawBitmapGlyph(img, c, int(penX), y, intScale, col, clips)
		// Advance per rune from the shared metric (MeasureText).
		penX += runeAdvance(r, fontSize)
	}
}

// drawTextBitmapFrac renders text whose 5x7 glyphs land at a fractional
// (sub-1) font scale: each destination pixel is box-filtered over the source
// bitmap cells it covers, so a glyph shrinks to its true screen size instead
// of being clamped to 1x. The pen advances by the same fractional metrics
// MeasureText reports, so drawn and measured widths stay in lockstep (the
// original text of a zoomed-out note keeps its box).
func drawTextBitmapFrac(img *image.RGBA, text string, pos image.Point, col color.RGBA, scale float64, clips []op.ClipOp) {
	gw, gh := 5*scale, 7*scale
	y0 := float64(pos.Y)
	penX := float64(pos.X)
	for _, r := range text {
		c := byte(r)
		if r < 32 || r > 127 {
			c = 63 // '?'
		}
		glyph := font5x7[c-32]

		x0 := int(math.Floor(penX))
		x1 := int(math.Ceil(penX + gw))
		yy0 := int(math.Floor(y0))
		yy1 := int(math.Ceil(y0 + gh))
		for y := yy0; y < yy1; y++ {
			// The destination pixel's source-cell row range, then columns.
			v0 := (float64(y) - y0) / scale
			v1 := (float64(y) + 1 - y0) / scale
			for x := x0; x < x1; x++ {
				u0 := (float64(x) - penX) / scale
				u1 := (float64(x) + 1 - penX) / scale
				cov := bitmapCellCoverage(glyph, u0, u1, v0, v1)
				if cov <= 0 {
					continue
				}
				clipCov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
				if clipCov <= 0 {
					continue
				}
				blendOver(img, x, y, withOpacity(col, cov*clipCov))
			}
		}
		// Advance per rune from the shared metric (MeasureText).
		penX += runeAdvance(r, scale*10)
	}
}

// bitmapCellCoverage returns how much of a destination pixel's source-cell
// range ([u0,u1)×[v0,v1)) overlaps the glyph's set bits, in [0,1] — a box
// filter, so a shrunk glyph keeps its shape with antialiased edges instead of
// dropping or doubling rows.
func bitmapCellCoverage(glyph [5]byte, u0, u1, v0, v1 float64) float64 {
	var cov float64
	for col := 0; col < 5; col++ {
		ow := math.Min(float64(col+1), u1) - math.Max(float64(col), u0)
		if ow <= 0 {
			continue
		}
		bits := glyph[col]
		for row := 0; row < 7; row++ {
			if (bits & (1 << row)) == 0 {
				continue
			}
			oh := math.Min(float64(row+1), v1) - math.Max(float64(row), v0)
			if oh > 0 {
				cov += ow * oh
			}
		}
	}
	if cov > 1 {
		return 1
	}
	return cov
}

// drawBitmapGlyph paints the 5x7 bitmap glyph for byte c (32..127) with its
// top-left corner at (x, y), scaled by intScale (>= 1), honouring clips.
func drawBitmapGlyph(img *image.RGBA, c byte, x, y, intScale int, col color.RGBA, clips []op.ClipOp) {
	glyph := font5x7[c-32]
	for colIdx := 0; colIdx < 5; colIdx++ {
		colBits := glyph[colIdx]
		for rowIdx := 0; rowIdx < 7; rowIdx++ {
			if (colBits & (1 << rowIdx)) != 0 {
				// Draw pixel at (x + colIdx*scale, y + rowIdx*scale)
				sx := x + colIdx*intScale
				sy := y + rowIdx*intScale
				for dx := 0; dx < intScale; dx++ {
					for dy := 0; dy < intScale; dy++ {
						px, py := sx+dx, sy+dy
						clipCov := clipCoverage(float64(px)+0.5, float64(py)+0.5, clips)
						if clipCov <= 0 {
							continue
						}
						blendOver(img, px, py, withOpacity(col, clipCov))
					}
				}
			}
		}
	}
}
