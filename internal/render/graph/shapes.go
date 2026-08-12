package graph

import (
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

// Group represents a container node
type Group struct {
	BaseNode
	// CSS filter on the whole subtree (offscreen LayerOp).
	FilterBlur       float64
	FilterBrightness float64
	FilterContrast   float64
	FilterSaturate   float64
	FilterGrayscale  float64 // 0..1
	FilterHueRotate  float64 // degrees
	FilterOpacity    float64 // 1 = identity; 0 = unset when all other zero
	// Drop-shadow filter (not box-shadow).
	DropShadowX, DropShadowY, DropShadowBlur float64
	DropShadowColor                          color.RGBA
	// MixBlendMode is CSS mix-blend-mode when compositing the layer.
	MixBlendMode string
}

// NewGroup creates a new Group node
func NewGroup() *Group {
	g := &Group{}
	g.Init(g)
	return g
}

func (g *Group) Base() *BaseNode { return &g.BaseNode }

// localTransform returns a node's local affine matrix — scale, then skew,
// then rotate, then translate — the same matrix UpdateGlobalTransform
// multiplies into the global one, so the rasterizer applies exactly what hit
// testing inverts. Emitting it (instead of the old integer translate) is what
// makes a node's ScaleX/ScaleY/Rotation/Skew reach the pixels: the board
// viewport sets zoom on its root group and pan + zoom follow for free.
func localTransform(b *BaseNode) geom.Matrix {
	m := geom.Identity().Translate(b.X, b.Y).Rotate(b.Rotation)
	if b.SkewX != 0 || b.SkewY != 0 {
		m = m.Skew(b.SkewX, b.SkewY)
	}
	return m.Scale(b.ScaleX, b.ScaleY)
}

// HitTest for Group checks children in reverse order (top-most first)
func (g *Group) HitTest(p geom.Point) Node {
	if g.NoHit {
		return nil
	}
	// First check children (they might be out of bounds if clipping is false,
	// but for now we assume they are within or we test them anyway)
	for i := len(g.Children) - 1; i >= 0; i-- {
		child := g.Children[i]
		if child.Base().NoHit {
			continue
		}
		if hit := child.HitTest(p); hit != nil {
			return hit
		}
	}

	// If no child is hit, check if the point hits the Group itself
	return g.BaseNode.HitTest(p)
}

// hasFilter reports whether any CSS filter / blend needs an offscreen layer.
func (g *Group) hasFilter() bool {
	if g.FilterBlur > 0 || g.FilterGrayscale > 0 || g.FilterHueRotate != 0 {
		return true
	}
	if g.DropShadowColor.A > 0 && (g.DropShadowBlur > 0 || g.DropShadowX != 0 || g.DropShadowY != 0) {
		return true
	}
	if g.MixBlendMode != "" && g.MixBlendMode != "normal" {
		return true
	}
	b, c, s, o := g.FilterBrightness, g.FilterContrast, g.FilterSaturate, g.FilterOpacity
	if b == 0 && c == 0 && s == 0 && o == 0 && g.FilterBlur == 0 && g.FilterGrayscale == 0 {
		return false // unset zero-value group
	}
	if o > 0 && o != 1 {
		return true
	}
	if b == 0 && c == 0 && s == 0 {
		return false
	}
	return b != 1 || c != 1 || s != 1
}

// Draw recursively updates transform and draws children
func (g *Group) Draw(ctx *Context) {
	g.UpdateGlobalTransform()

	useLayer := g.hasFilter()
	if useLayer {
		b, c, s, o := g.FilterBrightness, g.FilterContrast, g.FilterSaturate, g.FilterOpacity
		if b == 0 && c == 0 && s == 0 {
			b, c, s = 1, 1, 1
		}
		if o <= 0 {
			o = 1
		}
		ctx.BeginLayerEx(op.LayerOp{
			Blur: g.FilterBlur, Brightness: b, Contrast: c, Saturate: s,
			Grayscale: g.FilterGrayscale, HueRotate: g.FilterHueRotate, Opacity: o,
			DropShadowX: g.DropShadowX, DropShadowY: g.DropShadowY, DropShadowBlur: g.DropShadowBlur,
			DropShadowColor: g.DropShadowColor, BlendMode: g.MixBlendMode,
		})
	}

	ctx.Save()
	ctx.Opacity(g.Opacity)
	ctx.Transform(localTransform(&g.BaseNode))

	// Can add Group-level clipping here if needed
	g.DrawChildren(ctx)

	ctx.Restore()

	if useLayer {
		ctx.EndLayer()
	}
}

// Rect represents a rectangular shape (with optional border radius)
type Rect struct {
	BaseNode
	Fill          color.RGBA
	GradientStops []color.RGBA
	GradientAngle float64
	BackdropBlur  float64
	BackdropTint  color.RGBA
	Stroke        color.RGBA
	StrokeWidth   float64
	BorderRadius  float64

	ShadowColor color.RGBA
	ShadowBlur  float64
	ShadowX     float64
	ShadowY     float64
	// ShadowInset is CSS box-shadow: inset (inner shadow).
	ShadowInset bool
	// GradientStopPos optional 0..1 positions; empty = even spacing.
	GradientStopPos []float64
	GradientRadial  bool
	GradientConic   bool
	// CSS outline (outside the border box).
	OutlineColor  color.RGBA
	OutlineWidth  float64
	OutlineOffset float64
}

func NewRect() *Rect {
	r := &Rect{}
	r.Init(r)
	return r
}

func (r *Rect) Base() *BaseNode { return &r.BaseNode }

// Draw renders the rectangle
func (r *Rect) Draw(ctx *Context) {
	r.UpdateGlobalTransform()

	hasGrad := len(r.GradientStops) >= 2
	hasFrost := r.BackdropBlur > 0
	if r.Fill.A == 0 && r.Stroke.A == 0 && !hasGrad && !hasFrost {
		return // nothing to draw
	}

	ctx.Save()
	ctx.Transform(localTransform(&r.BaseNode))

	rect := image.Rect(0, 0, int(r.Width), int(r.Height))

	// Rounded, shadowed, gradient, outlined, or frosted: per-pixel SDF path.
	hasOutline := r.OutlineColor.A > 0 && r.OutlineWidth > 0
	if r.BorderRadius > 0 || r.ShadowColor.A > 0 || hasGrad || hasFrost || hasOutline || r.GradientConic {
		ctx.AddRRect(op.RRectOp{
			Rect: rect, Radius: r.BorderRadius,
			Fill: r.Fill, Stroke: r.Stroke, StrokeWidth: r.StrokeWidth,
			Shadow: r.ShadowColor, ShadowBlur: r.ShadowBlur, ShadowX: r.ShadowX, ShadowY: r.ShadowY,
			ShadowInset: r.ShadowInset,
			GradientStops: r.GradientStops, GradientStopPos: r.GradientStopPos,
			GradientAngle: r.GradientAngle, GradientRadial: r.GradientRadial, GradientConic: r.GradientConic,
			BackdropBlur: r.BackdropBlur, BackdropTint: r.BackdropTint,
			Outline: r.OutlineColor, OutlineWidth: r.OutlineWidth, OutlineOffset: r.OutlineOffset,
		})
		ctx.Restore()
		return
	}

	ctx.ClipRect(rect)

	if r.Fill.A > 0 {
		ctx.Fill(r.Fill)
		ctx.Paint()
	}

	if r.Stroke.A > 0 && r.StrokeWidth > 0 {
		ctx.SetStrokeWidth(r.StrokeWidth)
		ctx.Fill(r.Stroke)
		ctx.StrokePaint()
	}

	ctx.Restore()
}

// Text represents a text shape
type Text struct {
	BaseNode
	Content       string
	FontSize      float64
	FontWeight    int     // CSS weight (0/400 normal, 600+ emboldened)
	LetterSpacing float64 // CSS letter-spacing in px (0 = default)
	Italic        bool    // faux-italic second pass when true
	Fill          color.RGBA
	// Optional CSS-like decorations (software draws shadow → stroke → fill).
	StrokeColor color.RGBA
	StrokeWidth float64
	ShadowColor color.RGBA
	ShadowBlur  float64
	ShadowX     float64
	ShadowY     float64
	// CSS text-decoration lines.
	Underline   bool
	LineThrough bool
	Overline    bool
}

func NewText() *Text {
	t := &Text{}
	t.Init(t)
	return t
}

func (t *Text) Base() *BaseNode { return &t.BaseNode }

func (t *Text) Draw(ctx *Context) {
	t.UpdateGlobalTransform()

	if t.Content == "" {
		return
	}

	ctx.Save()
	ctx.Transform(localTransform(&t.BaseNode))
	ctx.Fill(t.Fill)
	ctx.AddText(op.TextOp{
		Text: t.Content, Pos: image.Point{0, 0}, Scale: t.FontSize / 10.0,
		Weight: t.FontWeight, LetterSpacing: t.LetterSpacing, Italic: t.Italic,
		StrokeColor: t.StrokeColor, StrokeWidth: t.StrokeWidth,
		ShadowColor: t.ShadowColor, ShadowBlur: t.ShadowBlur, ShadowX: t.ShadowX, ShadowY: t.ShadowY,
		Underline: t.Underline, LineThrough: t.LineThrough, Overline: t.Overline,
	})
	ctx.Restore()
}

// Circle represents a circular shape
type Circle struct {
	BaseNode
	Radius      float64
	Fill        color.RGBA
	Stroke      color.RGBA
	StrokeWidth float64
}

func NewCircle() *Circle {
	c := &Circle{}
	c.Init(c)
	return c
}

func (c *Circle) Base() *BaseNode { return &c.BaseNode }

func (c *Circle) Draw(ctx *Context) {
	c.UpdateGlobalTransform()

	if c.Fill.A == 0 && c.Stroke.A == 0 {
		return
	}

	ctx.Save()
	ctx.Opacity(c.Opacity)
	ctx.Transform(localTransform(&c.BaseNode))

	// Use the SDF-backed rounded-rectangle path instead of a binary rounded
	// clip.  A circle is simply a square whose corner radius is half its side;
	// RRectOp gives its edge a real coverage band, which is especially visible
	// on small controls and on 2x/3x displays.
	rect := image.Rect(0, 0, int(c.Radius*2), int(c.Radius*2))
	ctx.RRect(rect, c.Radius, c.Fill, c.Stroke, c.StrokeWidth,
		color.RGBA{}, 0, 0)

	ctx.Restore()
}

// HitTest for Circle does a precise radius check
func (c *Circle) HitTest(p geom.Point) Node {
	if c.NoHit {
		return nil
	}
	inv, ok := c.GlobalTransform.Invert()
	if !ok {
		return nil
	}
	localP := inv.TransformPoint(p)

	// Distance squared from center
	cx := c.Radius
	cy := c.Radius
	dx := localP.X - cx
	dy := localP.Y - cy

	if dx*dx+dy*dy <= c.Radius*c.Radius {
		return c.Self
	}
	return nil
}

// Image represents a bitmap image shape. Bitmap holds STRAIGHT
// (non-premultiplied) pixels (the renderer's buffer convention).
type Image struct {
	BaseNode
	Bitmap *image.RGBA
	// Fit is the object-fit mode: "fill", "contain", "cover" or "none"
	// (empty/unknown behaves as "cover", the HTML default,
	// render_media.go:19). The canvas widget layer validates author values.
	Fit string
	// BorderRadius clips the image to a rounded rect (style borderRadius).
	BorderRadius float64
}

func NewImage() *Image {
	i := &Image{}
	i.Init(i)
	return i
}

func (i *Image) Base() *BaseNode { return &i.BaseNode }

func (i *Image) Draw(ctx *Context) {
	i.UpdateGlobalTransform()

	if i.Bitmap == nil {
		return
	}
	w, h := int(i.Width), int(i.Height)
	if w <= 0 || h <= 0 {
		return
	}

	ctx.Save()
	ctx.Transform(localTransform(&i.BaseNode))

	dest := imageFitRect(i.Bitmap.Bounds().Dx(), i.Bitmap.Bounds().Dy(), w, h, i.Fit)
	if dest.Empty() {
		ctx.Restore()
		return
	}

	// Clip to the node's own rect so "cover" overflow and the border radius
	// are cut exactly like a Rect body is.
	nodeRect := image.Rect(0, 0, w, h)
	if i.BorderRadius > 0 {
		ctx.ClipRRect(nodeRect, i.BorderRadius)
	} else {
		ctx.ClipRect(nodeRect)
	}
	ctx.DrawImage(i.Bitmap, dest)

	ctx.Restore()
}

// imageFitRect returns the destination rect (local coords, box w×h) the
// source image (iw×ih) is scaled into for the given object-fit mode.
// contain/cover/none centre the image inside the box like CSS does.
func imageFitRect(iw, ih, w, h int, fit string) image.Rectangle {
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	switch fit {
	case "fill":
		return image.Rect(0, 0, w, h)
	case "contain", "cover":
		num, den := w*ih, h*iw // compare w/iw vs h/ih without floats
		scaleW := true         // scale so width fits exactly
		if fit == "contain" {
			scaleW = num <= den // w/iw <= h/ih → width is the limiting axis
		} else {
			scaleW = num >= den
		}
		var dw, dh int
		if scaleW {
			dw = w
			dh = ih * w / iw
		} else {
			dh = h
			dw = iw * h / ih
		}
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
		return image.Rect((w-dw)/2, (h-dh)/2, (w-dw)/2+dw, (h-dh)/2+dh)
	case "none":
		return image.Rect((w-iw)/2, (h-ih)/2, (w-iw)/2+iw, (h-ih)/2+ih)
	default:
		// Unknown/empty: the canvas widget layer warns; here we degrade to
		// the HTML default (cover).
		return imageFitRect(iw, ih, w, h, "cover")
	}
}
