//go:build !qorm_nocjk

package canvas

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/qorm/qorm/fonts"
	"github.com/qorm/qorm/internal/op"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Phase-2 text stack (plan B): the embedded OFL subset font (GB2312 hanzi +
// fullwidth symbols + ASCII) is parsed lazily once and serves both measuring
// and rasterization, so layout and drawing share one metric source exactly
// like the phase-1 bitmap path. Missing glyphs fall back to the phase-1
// bitmap '?' at a full-width advance; a missing/unparsable font falls the
// whole engine back to the bitmap path.

// parseFont loads and parses the embedded subset font. A var so tests can
// inject failures and count parse attempts.
var parseFont = func() (*sfnt.Font, error) {
	data, err := fonts.FS.ReadFile(fonts.FontFile)
	if err != nil {
		return nil, err
	}
	return sfnt.Parse(data)
}

var (
	engineOnce sync.Once
	engineInst *sfntEngine
)

// sharedEngine parses the embedded font exactly once on first use. A parse
// failure is negatively cached — engineInst stays nil and every later call
// costs one branch, not one parse — so the bitmap fallback is cheap.
func sharedEngine() *sfntEngine {
	engineOnce.Do(func() {
		if f, err := parseFont(); err == nil {
			engineInst = &sfntEngine{font: f}
		}
	})
	return engineInst
}

func init() {
	ttfProvider = func() ttfEngine {
		// NB: convert the typed nil — returning sharedEngine() directly
		// would box a nil *sfntEngine into a non-nil ttfEngine interface.
		if e := sharedEngine(); e != nil {
			return e
		}
		return nil
	}
	// TextMeasurer seam injection: callers of DefaultTextMeasurer
	// transparently get sfnt metrics when the font parses, bitmap metrics
	// otherwise.
	DefaultTextMeasurer = textMeasurerFunc(func(text string, fontSize float64) float64 {
		if e := sharedEngine(); e != nil {
			return e.Measure(text, fontSize)
		}
		return bitmapMeasurer{}.Measure(text, fontSize)
	})
}

type textMeasurerFunc func(text string, fontSize float64) float64

func (f textMeasurerFunc) Measure(text string, fontSize float64) float64 {
	return f(text, fontSize)
}

// sfntEngine implements ttfEngine over the parsed embedded font. *sfnt.Font
// is documented safe for concurrent use, and each call builds its own
// short-lived face, so the engine itself is stateless.
type sfntEngine struct {
	font *sfnt.Font
}

func (e *sfntEngine) newFace(fontSize float64) (font.Face, error) {
	// Full hinting snaps advances to whole pixels; measure and draw use the
	// same face settings so hinted advances cannot drift between them.
	return opentype.NewFace(e.font, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// fullAdvance is the advance a missing glyph occupies: one full width, the
// same the phase-1 bitmap path reports for a wide rune.
func fullAdvance(fontSize float64) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(fontSize * 64))
}

func (e *sfntEngine) Measure(text string, fontSize float64) float64 {
	fontSize = clampFontSize(fontSize)
	face, err := e.newFace(fontSize)
	if err != nil {
		return bitmapMeasurer{}.Measure(text, fontSize)
	}
	defer face.Close()

	prev := rune(-1)
	w := fixed.Int26_6(0)
	for _, r := range text {
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			// Missing glyph: drawn as the bitmap '?' (see drawText), so it
			// measures a full-width advance.
			prev = -1
			w += fullAdvance(fontSize)
			continue
		}
		if prev >= 0 {
			w += face.Kern(prev, r)
		}
		w += adv
		prev = r
	}
	return float64(w) / 64
}

func (e *sfntEngine) drawText(img *image.RGBA, text string, pos image.Point, col color.RGBA, fontSize float64, clips []op.ClipOp) {
	face, err := e.newFace(fontSize)
	if err != nil {
		// Unreachable in practice (Measure guards the same construction);
		// stay on the bitmap path rather than dropping text.
		drawTextBitmap(img, text, pos, col, fontSize, clips)
		return
	}
	defer face.Close()

	// intScale for the '?' fallback glyph, mirroring the bitmap path.
	intScale := int(fontSize / 10)
	if intScale < 1 {
		intScale = 1
	}
	full := fullAdvance(fontSize)
	// Baseline such that the cap-height top sits at pos.Y: the bitmap font
	// inks from pos.Y downward with no descender, so aligning on cap height
	// keeps the sfnt glyph ink in the same band instead of letting deep
	// CJK ascender metrics push glyphs below the text box.
	capH := face.Metrics().CapHeight
	if capH <= 0 {
		capH = face.Metrics().Ascent
	}
	baseline := fixed.I(pos.Y) + capH
	penX := fixed.I(pos.X)
	prev := rune(-1)
	for _, r := range text {
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			drawBitmapGlyph(img, '?', penX.Floor(), pos.Y, intScale, col, clips)
			penX += full
			prev = -1
			continue
		}
		if prev >= 0 {
			penX += face.Kern(prev, r)
		}
		dr, mask, maskp, _, gok := face.Glyph(fixed.Point26_6{X: penX, Y: baseline}, r)
		if gok {
			compositeMask(img, dr, mask, maskp, col, clips)
		}
		penX += adv
		prev = r
	}
}

// compositeMask blends a glyph coverage mask into img over dr using col
// (straight-alpha, already opacity-scaled). The mask modulates only the
// alpha, and the compositing itself is blendOver — the same "over" semantics
// as the bitmap path — honouring the clip stack.
func compositeMask(img *image.RGBA, dr image.Rectangle, mask image.Image, maskp image.Point, col color.RGBA, clips []op.ClipOp) {
	// Loop over the clipped rect but index the mask with the ORIGINAL dr:
	// maskp corresponds to dr.Min, and Intersect may move it.
	clipped := dr.Intersect(img.Bounds())
	if clipped.Empty() {
		return
	}
	alpha, isAlpha := mask.(*image.Alpha)
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		for x := clipped.Min.X; x < clipped.Max.X; x++ {
			mx, my := maskp.X+x-dr.Min.X, maskp.Y+y-dr.Min.Y
			var ma uint32
			if isAlpha {
				ma = uint32(alpha.AlphaAt(mx, my).A)
			} else {
				_, _, _, m := mask.At(mx, my).RGBA()
				ma = m >> 8
			}
			if ma == 0 {
				continue
			}
			clipCov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
			if clipCov <= 0 {
				continue
			}
			c := col
			if ma < 255 {
				c.A = uint8(uint32(col.A) * ma / 255)
			}
			if clipCov < 1 {
				c = withOpacity(c, clipCov)
			}
			blendOver(img, x, y, c)
		}
	}
}
