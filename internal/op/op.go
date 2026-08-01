package op

import (
	"image"
	"image/color"
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
}

func (TextOp) isOp() {}

// TransformOp applies an affine transformation matrix.
type TransformOp struct {
	// A 2D affine transform matrix [a, b, tx; c, d, ty]
	// Using a simple Offset for now, but ready for matrix.
	Offset image.Point
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
