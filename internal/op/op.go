package op

import (
	"image"
	"image/color"

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

// ClipOp sets a clipping boundary.
type ClipOp struct {
	Rect   image.Rectangle
	Radius float64
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
	GradientAngle  float64
	GradientRadial bool
	// BackdropBlur px: frost pixels already in the buffer under the rect.
	BackdropBlur float64
	BackdropTint color.RGBA
	Stroke       color.RGBA
	StrokeWidth  float64
	Shadow       color.RGBA
	ShadowBlur   float64
	ShadowX      float64
	ShadowY      float64
}

func (RRectOp) isOp() {}

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
