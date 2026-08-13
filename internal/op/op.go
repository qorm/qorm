package op

import (
	"encoding/binary"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"unsafe"

	"github.com/qorm/qorm/internal/geom"
)

// Ops is a list of rendering and state operations.
type Ops struct {
	ops []Op
}

// Op represents a single drawing or state operation.
type Op interface {
	isOp()
}

// Reset clears the operation list.
func (o *Ops) Reset() {
	o.ops = o.ops[:0]
}

// Add appends an operation to the list.
func (o *Ops) Add(op Op) {
	o.ops = append(o.ops, op)
}

// Operations returns the underlying slice of operations.
func (o *Ops) Operations() []Op {
	return o.ops
}

// Fingerprint returns a stable 64-bit content hash of the display list.
// Used by the canvas engine to skip software rasterization when a dirty
// frame produces an identical op stream (static UI with spurious redraws).
func (o *Ops) Fingerprint() uint64 {
	h := fnv.New64a()
	var buf [16]byte
	writeU8 := func(v uint8) {
		buf[0] = v
		_, _ = h.Write(buf[:1])
	}
	writeU32 := func(v uint32) {
		binary.LittleEndian.PutUint32(buf[:4], v)
		_, _ = h.Write(buf[:4])
	}
	writeU64 := func(v uint64) {
		binary.LittleEndian.PutUint64(buf[:8], v)
		_, _ = h.Write(buf[:8])
	}
	writeF64 := func(v float64) {
		writeU64(math.Float64bits(v))
	}
	writeColor := func(c color.RGBA) {
		buf[0], buf[1], buf[2], buf[3] = c.R, c.G, c.B, c.A
		_, _ = h.Write(buf[:4])
	}
	writeRect := func(r image.Rectangle) {
		writeU32(uint32(r.Min.X))
		writeU32(uint32(r.Min.Y))
		writeU32(uint32(r.Max.X))
		writeU32(uint32(r.Max.Y))
	}
	writePt := func(p image.Point) {
		writeU32(uint32(p.X))
		writeU32(uint32(p.Y))
	}
	writeStr := func(s string) {
		writeU32(uint32(len(s)))
		_, _ = h.Write([]byte(s))
	}
	for _, operation := range o.ops {
		switch t := operation.(type) {
		case ColorOp:
			writeU8(1)
			writeColor(t.Color)
		case OpacityOp:
			writeU8(2)
			writeF64(t.Alpha)
		case StrokeOp:
			writeU8(3)
			writeF64(t.Width)
		case TransformOp:
			writeU8(4)
			writeF64(t.M.A)
			writeF64(t.M.B)
			writeF64(t.M.C)
			writeF64(t.M.D)
			writeF64(t.M.E)
			writeF64(t.M.F)
		case ClipOp:
			writeU8(5)
			writeRect(t.Rect)
			writeF64(t.Radius)
			writeF64(t.EllipseRX)
			writeF64(t.EllipseRY)
			writeU32(uint32(len(t.Poly)))
			for _, p := range t.Poly {
				writeF64(p.X)
				writeF64(p.Y)
			}
			if t.EvenOdd {
				writeU8(1)
			} else {
				writeU8(0)
			}
		case SaveOp:
			writeU8(6)
		case RestoreOp:
			writeU8(7)
		case PaintOp:
			writeU8(8)
		case StrokePaintOp:
			writeU8(9)
		case TextOp:
			writeU8(10)
			writeStr(t.Text)
			writePt(t.Pos)
			writeF64(t.Scale)
			writeU32(uint32(t.Weight))
			writeF64(t.LetterSpacing)
			if t.Italic {
				writeU8(1)
			} else {
				writeU8(0)
			}
			writeColor(t.StrokeColor)
			writeF64(t.StrokeWidth)
			writeColor(t.ShadowColor)
			writeF64(t.ShadowBlur)
			writeF64(t.ShadowX)
			writeF64(t.ShadowY)
			if t.Underline {
				writeU8(1)
			} else {
				writeU8(0)
			}
			if t.LineThrough {
				writeU8(1)
			} else {
				writeU8(0)
			}
			if t.Overline {
				writeU8(1)
			} else {
				writeU8(0)
			}
		case ImageOp:
			writeU8(11)
			// Pointer identity + dest; pixel content is assumed stable for the
			// life of a cached *image.RGBA in the graph.
			writeU64(uint64(uintptr(unsafe.Pointer(t.Src))))
			writeRect(t.Dest)
			if t.Pixelated {
				writeU8(1)
			} else {
				writeU8(0)
			}
		case RRectOp:
			writeU8(12)
			writeRect(t.Rect)
			writeF64(t.Radius)
			writeColor(t.Fill)
			writeU32(uint32(len(t.GradientStops)))
			for _, c := range t.GradientStops {
				writeColor(c)
			}
			writeU32(uint32(len(t.GradientStopPos)))
			for _, p := range t.GradientStopPos {
				writeF64(p)
			}
			writeF64(t.GradientAngle)
			if t.GradientRadial {
				writeU8(1)
			} else {
				writeU8(0)
			}
			if t.GradientConic {
				writeU8(1)
			} else {
				writeU8(0)
			}
			writeF64(t.BackdropBlur)
			writeColor(t.BackdropTint)
			writeColor(t.Stroke)
			writeF64(t.StrokeWidth)
			writeColor(t.Shadow)
			writeF64(t.ShadowBlur)
			writeF64(t.ShadowX)
			writeF64(t.ShadowY)
			if t.ShadowInset {
				writeU8(1)
			} else {
				writeU8(0)
			}
			writeColor(t.Outline)
			writeF64(t.OutlineWidth)
			writeF64(t.OutlineOffset)
		case RectOp:
			writeU8(13)
			writeRect(t.Rect)
			writeF64(t.Radius)
		case LayerOp:
			writeU8(14)
			writeF64(t.Blur)
			writeF64(t.Brightness)
			writeF64(t.Contrast)
			writeF64(t.Saturate)
			writeF64(t.Grayscale)
			writeF64(t.HueRotate)
			writeF64(t.Invert)
			writeF64(t.Sepia)
			writeColor(t.Tint)
			writeF64(t.Opacity)
			writeF64(t.DropShadowX)
			writeF64(t.DropShadowY)
			writeF64(t.DropShadowBlur)
			writeColor(t.DropShadowColor)
			writeStr(t.BlendMode)
			writeStr(t.MaskFade)
			writeF64(t.MaskFadeSize)
			writeStr(t.CacheKey)
			writeU64(t.CacheFP)
		case EndLayerOp:
			writeU8(15)
		default:
			writeU8(255)
		}
	}
	return h.Sum64()
}

// ColorOp sets the current fill color.
type ColorOp struct {
	Color color.RGBA
}

func (ColorOp) isOp() {}

// RectOp draws a rectangle.
type RectOp struct {
	Rect   image.Rectangle
	Radius float64
}

func (RectOp) isOp() {}

// TextOp draws text.
type TextOp struct {
	Text  string
	Pos   image.Point
	Scale float64
	// Weight is the CSS-style font weight (400 normal, 700 bold); 0 means
	// normal. The sfnt engine emboldens synthetically (the embedded font
	// has one weight).
	Weight int
	// LetterSpacing is extra px between runes (CSS letter-spacing); 0 is default.
	LetterSpacing float64
	// Italic requests a light faux-italic second pass in the rasterizer.
	Italic bool
	// Optional CSS-like text decorations (drawn under the fill).
	StrokeColor color.RGBA
	StrokeWidth float64
	ShadowColor color.RGBA
	ShadowBlur  float64
	ShadowX     float64
	ShadowY     float64
	// CSS text-decoration lines (underline / line-through / overline).
	Underline   bool
	LineThrough bool
	Overline    bool
}

func (TextOp) isOp() {}

// TransformOp applies an affine transformation matrix: every subsequent op's
// geometry (clip rects, text positions, image dests, rounded-rect corners,
// stroke widths) is transformed by it before rasterization, and text font
// scale and stroke widths multiply by the matrix's uniform scale. The graph
// layer emits the same local matrix it bakes into GlobalTransform, so hit
// testing (which inverts GlobalTransform) and pixels stay consistent even
// under scale (an infinite-canvas board's zoom).
type TransformOp struct {
	M geom.Matrix
}

func (TransformOp) isOp() {}

// ClipOp sets a clipping boundary. Rect is always the AABB (for bounds
// intersection). When Poly has 3+ points, the clip is that polygon (local
// coords at emit; the rasterizer transforms them to screen space).
// When EllipseRX and EllipseRY are both > 0, the clip is an
// axis-aligned ellipse centered in Rect (CSS clip-path: circle/ellipse).
// Otherwise Radius > 0 is a rounded rect; Radius == 0 is a hard rect.
type ClipOp struct {
	Rect      image.Rectangle
	Radius    float64
	EllipseRX float64 // screen px; with EllipseRY > 0 enables ellipse clip
	EllipseRY float64
	Poly      []geom.Point // local coords; screen-space after ClipOp transform
	EvenOdd   bool         // CSS fill-rule evenodd; default nonzero
}

func (ClipOp) isOp() {}

// PaintOp fills the current clipping region with the current color.
type PaintOp struct{}

func (PaintOp) isOp() {}

// StrokeOp sets the current stroke parameters (width)
// Color is set via ColorOp.
type StrokeOp struct {
	Width float64
}

func (StrokeOp) isOp() {}

// StrokePaintOp strokes the current clipping region
type StrokePaintOp struct{}

func (StrokePaintOp) isOp() {}

// ImageOp draws the source image scaled into Dest (in the current
// transformed coordinate space). The source holds STRAIGHT (non-premultiplied)
// pixels, matching the renderer's buffer convention. Only destination pixels
// inside the active clip stack are written.
type ImageOp struct {
	Src  *image.RGBA
	Dest image.Rectangle
	// Pixelated forces nearest-neighbour sampling even when Dest != Src size
	// (CSS image-rendering: pixelated).
	Pixelated bool
}

func (ImageOp) isOp() {}

// RRectOp draws a rounded rectangle — fill, inner stroke, and an optional
// drop shadow — with PER-PIXEL coverage (signed distance field): the rounded
// corners get a ~1px antialiasing band and the shadow falls off smoothly,
// unlike the ClipOp+PaintOp path whose clip edges are binary. Rect is in the
// current transformed coordinate space; the shadow is the same shape offset
// by (ShadowX, ShadowY) with a smoothstep falloff over ShadowBlur pixels.
type RRectOp struct {
	Rect   image.Rectangle
	Radius float64
	Fill   color.RGBA
	// GradientStops when len>=2 paints a gradient fill (Fill is first-stop fallback).
	GradientStops []color.RGBA
	// GradientStopPos optional 0..1 positions aligned with GradientStops; empty
	// means evenly spaced stops.
	GradientStopPos []float64
	// GradientAngle CSS degrees for linear gradients (0 = to top, 90 = to right).
	// When GradientRadial is true, angle is ignored and fill is radial from center.
	// When GradientConic is true, fill is a conic/sweep gradient from center;
	// GradientAngle is the starting angle (CSS: 0 = from top, clockwise).
	GradientAngle  float64
	GradientRadial bool
	GradientConic  bool
	// BackdropBlur px: frost pixels already in the buffer under the rect.
	BackdropBlur float64
	BackdropTint color.RGBA
	Stroke       color.RGBA
	StrokeWidth  float64
	Shadow       color.RGBA
	ShadowBlur   float64
	ShadowX      float64
	ShadowY      float64
	// ShadowInset draws the shadow inside the shape (CSS box-shadow: inset).
	ShadowInset bool
	// Outline is CSS outline (outside the border box, does not affect layout).
	Outline       color.RGBA
	OutlineWidth  float64
	OutlineOffset float64 // gap between border edge and outline
}

func (RRectOp) isOp() {}

// LayerOp begins an offscreen layer. Subsequent ops draw into a transparent
// buffer until EndLayerOp; the layer is then filtered and composited onto the
// parent target (CSS filter + mix-blend-mode on a group).
type LayerOp struct {
	// Blur is the Gaussian-approx radius in screen pixels (3 stacked box blurs).
	Blur float64
	// Brightness multiplies RGB (1 = identity; 0 = black).
	Brightness float64
	// Contrast scales about mid-gray (1 = identity).
	Contrast float64
	// Saturate scales chroma (1 = identity; 0 = grayscale).
	Saturate float64
	// Grayscale 0..1 (0 = identity, 1 = full gray). CSS grayscale().
	Grayscale float64
	// HueRotate degrees. CSS hue-rotate().
	HueRotate float64
	// Invert 0..1. CSS invert().
	Invert float64
	// Sepia 0..1. CSS sepia().
	Sepia float64
	// Tint RGB-modulates the layer (Godot modulate / Phaser tint). A==0 = unset.
	Tint color.RGBA
	// Opacity multiplies layer alpha (1 = identity). CSS filter: opacity().
	Opacity float64
	// Drop-shadow (CSS filter: drop-shadow): offset + blur + color of alpha.
	DropShadowX, DropShadowY, DropShadowBlur float64
	DropShadowColor                          color.RGBA
	// BlendMode is CSS mix-blend-mode when compositing onto the parent
	// ("normal", "multiply", "screen", "overlay", "darken", "lighten").
	BlendMode string
	// MaskFade soft-fades one edge of the layer ("top"|"bottom"|"left"|"right").
	MaskFade     string
	MaskFadeSize float64 // screen px of the fade band
	// CacheKey when non-empty enables static layer reuse: identical content
	// (CacheFP) skips re-drawing children and re-composites the cached buffer.
	CacheKey string
	CacheFP  uint64
}

func (LayerOp) isOp() {}

// EndLayerOp closes the most recent LayerOp: blur + composite onto parent.
type EndLayerOp struct{}

func (EndLayerOp) isOp() {}

// OpacityOp sets the current opacity.
type OpacityOp struct {
	Alpha float64
}

func (OpacityOp) isOp() {}

// SaveOp saves the current state (color, offset, clip).
type SaveOp struct{}

func (SaveOp) isOp() {}

// RestoreOp restores the previously saved state.
type RestoreOp struct{}

func (RestoreOp) isOp() {}
