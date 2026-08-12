package graph

import (
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

// Context provides a stateful Canvas-like API for drawing.
type Context struct {
	ops *op.Ops
}

// NewContext creates a new drawing context that writes to the given ops list.
func NewContext(ops *op.Ops) *Context {
	return &Context{
		ops: ops,
	}
}

// Save pushes the current state (transform, clip, color) onto the stack.
func (c *Context) Save() {
	c.ops.Add(op.SaveOp{})
}

// Restore pops the state stack.
func (c *Context) Restore() {
	c.ops.Add(op.RestoreOp{})
}

// Translate translates the coordinate system. Kept as a thin helper over
// Transform: the op layer carries a full matrix, so a node's Draw can hand the
// same local matrix (position × scale) it baked into its GlobalTransform —
// the rasterizer applies it, keeping hit testing and pixels in lockstep.
func (c *Context) Translate(dx, dy int) {
	c.ops.Add(op.TransformOp{M: geom.Identity().Translate(float64(dx), float64(dy))})
}

// Transform post-multiplies the current transform by the affine matrix m.
func (c *Context) Transform(m geom.Matrix) {
	c.ops.Add(op.TransformOp{M: m})
}

// ClipRect sets the clipping region to a rectangle.
func (c *Context) ClipRect(r image.Rectangle) {
	c.ops.Add(op.ClipOp{Rect: r})
}

// ClipRRect sets the clipping region to a rounded rectangle.
func (c *Context) ClipRRect(r image.Rectangle, radius float64) {
	c.ops.Add(op.ClipOp{Rect: r, Radius: radius})
}

// Fill sets the current fill color.
func (c *Context) Fill(color color.RGBA) {
	c.ops.Add(op.ColorOp{Color: color})
}

// Paint fills the current clipping region with the current fill color.
func (c *Context) Paint() {
	c.ops.Add(op.PaintOp{})
}

// Opacity sets the current global alpha
func (c *Context) Opacity(alpha float64) {
	c.ops.Add(op.OpacityOp{Alpha: alpha})
}

// SetStrokeWidth sets the current stroke width
func (c *Context) SetStrokeWidth(w float64) {
	c.ops.Add(op.StrokeOp{Width: w})
}

// StrokePaint strokes the current clipping region
func (c *Context) StrokePaint() {
	c.ops.Add(op.StrokePaintOp{})
}

// DrawText draws text at the specified coordinates.
func (c *Context) DrawText(text string, pos image.Point, scale float64) {
	c.ops.Add(op.TextOp{
		Text:  text,
		Pos:   pos,
		Scale: scale,
	})
}

// DrawTextWeighted is DrawText with a font weight (0/400 normal; 600+ is
// emboldened synthetically by the rasterizer).
func (c *Context) DrawTextWeighted(text string, pos image.Point, scale float64, weight int) {
	c.DrawTextTracking(text, pos, scale, weight, 0, false)
}

// DrawTextTracking is DrawTextWeighted with letter-spacing and optional italic.
func (c *Context) DrawTextTracking(text string, pos image.Point, scale float64, weight int, letterSpacing float64, italic bool) {
	c.DrawTextDecorated(text, pos, scale, weight, letterSpacing, italic,
		color.RGBA{}, 0, color.RGBA{}, 0, 0, 0)
}

// DrawTextDecorated is DrawTextTracking plus optional stroke outline and drop
// shadow (CSS text-stroke / text-shadow analogues). Zero alpha skips a layer.
func (c *Context) DrawTextDecorated(text string, pos image.Point, scale float64, weight int, letterSpacing float64, italic bool,
	stroke color.RGBA, strokeWidth float64, shadow color.RGBA, shadowBlur, shadowX, shadowY float64) {
	c.ops.Add(op.TextOp{
		Text:          text,
		Pos:           pos,
		Scale:         scale,
		Weight:        weight,
		LetterSpacing: letterSpacing,
		Italic:        italic,
		StrokeColor:   stroke,
		StrokeWidth:   strokeWidth,
		ShadowColor:   shadow,
		ShadowBlur:    shadowBlur,
		ShadowX:       shadowX,
		ShadowY:       shadowY,
	})
}

// DrawImage draws src scaled into dest (current coordinate space).
func (c *Context) DrawImage(src *image.RGBA, dest image.Rectangle) {
	c.ops.Add(op.ImageOp{Src: src, Dest: dest})
}

// RRect records an antialiased rounded rectangle with optional inner stroke
// and drop shadow (see op.RRectOp) — per-pixel SDF coverage instead of the
// binary clip+paint path, so corners and shadow falloff render smoothly.
func (c *Context) RRect(r image.Rectangle, radius float64, fill, stroke color.RGBA, strokeWidth float64, shadow color.RGBA, shadowBlur, shadowY float64) {
	c.RRectEx(r, radius, fill, stroke, strokeWidth, shadow, shadowBlur, 0, shadowY, nil, nil, 0, false, 0, color.RGBA{}, false)
}

// RRectEx is RRect plus gradient stops (linear or radial), optional frost, and
// inset (inner) shadow.
func (c *Context) RRectEx(r image.Rectangle, radius float64, fill, stroke color.RGBA, strokeWidth float64, shadow color.RGBA, shadowBlur, shadowX, shadowY float64, grad []color.RGBA, gradPos []float64, gradAngle float64, gradRadial bool, backdropBlur float64, backdropTint color.RGBA, shadowInset bool) {
	c.ops.Add(op.RRectOp{
		Rect: r, Radius: radius,
		Fill: fill, Stroke: stroke, StrokeWidth: strokeWidth,
		Shadow: shadow, ShadowBlur: shadowBlur, ShadowX: shadowX, ShadowY: shadowY,
		ShadowInset:   shadowInset,
		GradientStops: grad, GradientStopPos: gradPos, GradientAngle: gradAngle, GradientRadial: gradRadial,
		BackdropBlur: backdropBlur, BackdropTint: backdropTint,
	})
}

// BeginLayer starts an offscreen layer with blur only. Must pair with EndLayer.
func (c *Context) BeginLayer(blur float64) {
	c.BeginLayerEx(op.LayerOp{Blur: blur, Brightness: 1, Contrast: 1, Saturate: 1, Opacity: 1})
}

// BeginLayerFilter starts an offscreen layer with classic color filters.
func (c *Context) BeginLayerFilter(blur, brightness, contrast, saturate float64) {
	c.BeginLayerEx(op.LayerOp{
		Blur: blur, Brightness: brightness, Contrast: contrast, Saturate: saturate, Opacity: 1,
	})
}

// BeginLayerEx records a full LayerOp (filter stack + drop-shadow + blend).
func (c *Context) BeginLayerEx(layer op.LayerOp) {
	c.ops.Add(layer)
}

// EndLayer closes the current offscreen layer and composites it.
func (c *Context) EndLayer() {
	c.ops.Add(op.EndLayerOp{})
}
