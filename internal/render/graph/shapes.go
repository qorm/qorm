package graph

import (
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/geom"
)

// Group represents a container node
type Group struct {
	BaseNode
}

// NewGroup creates a new Group node
func NewGroup() *Group {
	g := &Group{}
	g.Init(g)
	return g
}

func (g *Group) Base() *BaseNode { return &g.BaseNode }

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

// Draw recursively updates transform and draws children
func (g *Group) Draw(ctx *Context) {
	g.UpdateGlobalTransform()
	
	ctx.Save()
	ctx.Opacity(g.Opacity)
	ctx.Translate(int(g.X), int(g.Y)) // Naive translate for now, ideally apply matrix
	
	// Can add Group-level clipping here if needed
	g.DrawChildren(ctx)
	
	ctx.Restore()
}

// Rect represents a rectangular shape (with optional border radius)
type Rect struct {
	BaseNode
	Fill         color.RGBA
	Stroke       color.RGBA
	StrokeWidth  float64
	BorderRadius float64
	
	ShadowColor color.RGBA
	ShadowBlur  float64
	ShadowY     float64
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
	
	if r.Fill.A == 0 && r.Stroke.A == 0 {
		return // nothing to draw
	}

	ctx.Save()
	ctx.Translate(int(r.X), int(r.Y))
	
	rect := image.Rect(0, 0, int(r.Width), int(r.Height))
	
	// Draw Shadow (simulated with 3 concentric layers)
	if r.ShadowColor.A > 0 {
		steps := 3
		stepAlpha := float64(r.ShadowColor.A) / float64(steps) / 255.0
		for i := steps; i > 0; i-- {
			spread := float64(i) * (r.ShadowBlur / float64(steps))
			shadowRect := image.Rect(
				int(-spread),
				int(r.ShadowY-spread),
				int(r.Width+spread),
				int(r.Height+r.ShadowY+spread),
			)
			ctx.Save()
			ctx.Opacity(stepAlpha)
			if r.BorderRadius > 0 {
				ctx.ClipRRect(shadowRect, r.BorderRadius+spread)
			} else {
				ctx.ClipRect(shadowRect)
			}
			// Draw shadow color (ignoring its own alpha since we use Opacity)
			sc := r.ShadowColor
			sc.A = 255
			ctx.Fill(sc)
			ctx.Paint()
			ctx.Restore()
		}
	}
	
	// Draw main body
	if r.BorderRadius > 0 {
		ctx.ClipRRect(rect, r.BorderRadius)
	} else {
		ctx.ClipRect(rect)
	}
	
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
	Content  string
	FontSize float64
	Fill     color.RGBA
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
	ctx.Translate(int(t.X), int(t.Y))
	ctx.Fill(t.Fill)
	ctx.DrawText(t.Content, image.Point{0, 0}, t.FontSize/10.0)
	ctx.Restore()
}

// Circle represents a circular shape
type Circle struct {
	BaseNode
	Radius float64
	Fill   color.RGBA
	Stroke color.RGBA
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
	ctx.Translate(int(c.X), int(c.Y))
	
	// QORM op.Ops does not natively have an arc/circle op yet.
	// We'll emulate it by clipping a rect with a 100% border radius.
	rect := image.Rect(0, 0, int(c.Radius*2), int(c.Radius*2))
	ctx.ClipRRect(rect, c.Radius)
	
	if c.Fill.A > 0 {
		ctx.Fill(c.Fill)
		ctx.Paint()
	}
	
	if c.Stroke.A > 0 && c.StrokeWidth > 0 {
		ctx.SetStrokeWidth(c.StrokeWidth)
		ctx.Fill(c.Stroke)
		ctx.StrokePaint()
	}
	
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
	
	if dx*dx + dy*dy <= c.Radius*c.Radius {
		return c.Self
	}
	return nil
}

// Image represents a bitmap image shape
type Image struct {
	BaseNode
	Bitmap *image.RGBA
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

	ctx.Save()
	ctx.Translate(int(i.X), int(i.Y))
	
	// Need an ImageOp in op.Ops to draw bitmaps
	// For now we'll just clip
	
	ctx.Restore()
}
