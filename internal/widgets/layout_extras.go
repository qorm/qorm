package widgets

// Canvas ports for high-traffic HTML layout / feedback types that previously
// lived only on the HTML path (see htmlOnlyCoreAllowlist).

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("activityindicator", Spinner{})
	canvas.RegisterWidget("tag", &Chip{})

	canvas.RegisterWidget("aspectratio", AspectRatio{})
	canvas.RegisterWidget("ignorepointer", IgnorePointer{})
	canvas.RegisterWidget("skeleton", Skeleton{})
	canvas.RegisterWidget("circularprogress", CircularProgress{})
	canvas.RegisterWidget("circularprogressindicator", CircularProgress{})
}

// ---- aspectratio ------------------------------------------------------------

// AspectRatio constrains its box to width/height = ratio (default 1) and
// mounts children in a column flow inside that box.
type AspectRatio struct{}

func aspectRatioOf(n *model.Node, rt *runtime.Runtime) float64 {
	if n == nil {
		return 1
	}
	if raw, ok := n.Prop("ratio"); ok {
		v := asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt)))
		if v > 0 {
			return v
		}
	}
	return 1
}

func (AspectRatio) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	ratio := aspectRatioOf(n, rt)
	w = 100 * scale
	if n != nil && n.Style != nil {
		if f, ok := n.Style["width"].(float64); ok && f > 0 {
			w = int(f) * scale
		} else if i, ok := n.Style["width"].(int); ok && i > 0 {
			w = i * scale
		}
	}
	h = int(math.Round(float64(w) / ratio))
	if h < 1 {
		h = 1
	}
	return w, h
}

func (a AspectRatio) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return a.record(ln, rt, scale, nil)
}

func (a AspectRatio) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return a.record(ln, rt, scale, sinks)
}

func (AspectRatio) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Clip = true
	cy := ln.Style.Padding
	for _, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(ln.Style.Padding, cy, ln.Style.Padding+cw, cy+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		cy += ch + ln.Style.Gap
	}
	ln.Children = nil
	return g
}

// ---- ignorepointer ----------------------------------------------------------

// IgnorePointer mounts children in a column flow and marks the subtree NoHit
// (HTML pointer-events:none).
type IgnorePointer struct{}

func (IgnorePointer) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height
		}
	}
	if w < 1 {
		w = scale
	}
	if h < 1 {
		h = scale
	}
	return w, h
}

func (ip IgnorePointer) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return ip.record(ln, rt, scale, nil)
}

func (ip IgnorePointer) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return ip.record(ln, rt, scale, sinks)
}

func (IgnorePointer) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.NoHit = true
	cy := ln.Style.Padding
	for _, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(ln.Style.Padding, cy, ln.Style.Padding+cw, cy+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			markNoHitTree(cn)
			g.AddChild(cn)
		}
		cy += ch + ln.Style.Gap
	}
	ln.Children = nil
	return g
}

func markNoHitTree(n draw.Node) {
	if n == nil {
		return
	}
	n.Base().NoHit = true
	for _, c := range n.Base().Children {
		markNoHitTree(c)
	}
}

// ---- skeleton ---------------------------------------------------------------

// Skeleton is a pulsing placeholder block for loading states.
type Skeleton struct{}

var skelEpoch = time.Now()

func (Skeleton) Animating() bool { return true }

func (Skeleton) Measure(n *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w, h = 120*scale, 16*scale
	if n != nil && n.Style != nil {
		if f, ok := n.Style["width"].(float64); ok && f > 0 {
			w = int(f) * scale
		}
		if f, ok := n.Style["height"].(float64); ok && f > 0 {
			h = int(f) * scale
		}
	}
	return w, h
}

func (Skeleton) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	phase := time.Since(skelEpoch).Seconds() * (2 * math.Pi / 1.2)
	op := 0.6 + 0.25*math.Sin(phase)
	if op < 0.35 {
		op = 0.35
	}
	r := draw.NewRect()
	r.Width = float64(ln.Width)
	r.Height = float64(ln.Height)
	r.BorderRadius = float64(4 * scale)
	r.Fill = themeColor(rt, "inputBg", color.RGBA{229, 229, 234, 255})
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Opacity = op
	g.NoHit = true
	g.AddChild(r)
	return g
}

// ---- circularprogress -------------------------------------------------------

// CircularProgress paints a track ring plus a progress arc as bead circles.
type CircularProgress struct{}

func (CircularProgress) Animating() bool { return true }

func (CircularProgress) Measure(_ *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	const d = 40
	return d * scale, d * scale
}

func (CircularProgress) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	cx := float64(ln.Width) / 2
	cy := float64(ln.Height) / 2
	radius := math.Min(cx, cy) - float64(3*scale)
	if radius < 4 {
		radius = 4
	}
	stroke := float64(3 * scale)
	if stroke < 2 {
		stroke = 2
	}
	trackCol := themeColor(rt, "inputBg", color.RGBA{229, 229, 234, 255})
	accent := themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	pct := circularPct(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.NoHit = true
	addArcBeads(g, cx, cy, radius, stroke, 0, 360, trackCol)
	if pct < 0 {
		start := math.Mod(time.Since(skelEpoch).Seconds()*180, 360)
		addArcBeads(g, cx, cy, radius, stroke, start, start+90, accent)
		return g
	}
	span := pct / 100 * 360
	if span > 0 {
		addArcBeads(g, cx, cy, radius, stroke, -90, -90+span, accent)
	}
	return g
}

func addArcBeads(g *draw.Group, cx, cy, r, stroke, fromDeg, toDeg float64, col color.RGBA) {
	if g == nil || toDeg <= fromDeg || r <= 0 || stroke <= 0 {
		return
	}
	steps := int((toDeg-fromDeg)/5) + 1
	if steps < 2 {
		steps = 2
	}
	beadR := stroke / 2
	if beadR < 1 {
		beadR = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		deg := fromDeg + (toDeg-fromDeg)*t
		rad := deg * math.Pi / 180
		x := cx + r*math.Cos(rad)
		y := cy + r*math.Sin(rad)
		c := draw.NewCircle()
		c.Radius = beadR
		c.Fill = col
		cg := draw.NewGroup()
		cg.X, cg.Y = x-beadR, y-beadR
		cg.NoHit = true
		cg.AddChild(c)
		g.AddChild(cg)
	}
}

func circularPct(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) float64 {
	if n == nil {
		return -1
	}
	raw, ok := n.Prop("value")
	if !ok || raw == nil || fmt.Sprint(raw) == "" {
		if n.Value == "" {
			return -1
		}
		raw = n.Value
	}
	v := asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln)))
	if v >= 0 && v <= 1 {
		return v * 100
	}
	return clampPct(v)
}
