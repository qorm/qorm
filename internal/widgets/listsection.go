package widgets

import (
	"image"
	"image/color"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("listsection", &ListSection{})
	canvas.RegisterWidget("cupertinolistsection", &ListSection{})
}

// ListSection is a labelled list group (HTML render_data.go listSection): an
// optional uppercase `header` label, a rounded surface box holding the
// children (with hairline separators between them), and an optional `footer`
// label. Purely structural — the children keep their own interactions.
type ListSection struct{}

func (s *ListSection) Measure(n *model.Node, rt *runtime.Runtime, vars map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w, h = contentMeasure(n, rt, vars, scale)
	if v, ok := n.Prop("header"); ok {
		if str, ok := v.(string); ok && str != "" {
			h += 24 * scale
			w += 32 * scale
		}
	}
	if v, ok := n.Prop("footer"); ok {
		if str, ok := v.(string); ok && str != "" {
			h += 20 * scale
		}
	}
	return w, h
}

func (s *ListSection) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return s.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget (the children flow with
// the frame's sinks).
func (s *ListSection) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return s.record(ln, rt, scale, sinks)
}

func (s *ListSection) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	fs := formFontSizeLN(ln, scale)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	sep := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	y := 0
	if v, ok := ln.Node.Prop("header"); ok {
		if h, ok := v.(string); ok && h != "" {
			g.AddChild(formText(h, float64(16*scale), float64(y), fs-2*scale, ink2))
			y += 22 * scale
		}
	}

	// The rounded surface box holding the children, with hairlines between.
	boxTop := y
	boxH := 0
	for _, cln := range ln.Children {
		boxH += cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
	}
	box := draw.NewRect()
	box.X = float64(16 * scale)
	box.Y = float64(boxTop)
	box.Width = float64(ln.Width - 32*scale)
	box.Height = float64(boxH)
	box.BorderRadius = 10 * float64(scale)
	box.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	g.AddChild(box)

	cy := boxTop
	for i, cln := range ln.Children {
		if i > 0 {
			line := draw.NewRect()
			line.X = float64(16 * scale)
			line.Y = float64(cy)
			line.Width = float64(ln.Width - 32*scale)
			line.Height = float64(scale)
			line.Fill = sep
			g.AddChild(line)
		}
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(16*scale, cy, ln.Width-16*scale, cy+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX+16*scale, ln.AbsY+cy), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		cy += ch
	}

	if v, ok := ln.Node.Prop("footer"); ok {
		if f, ok := v.(string); ok && f != "" {
			g.AddChild(formText(f, float64(16*scale), float64(cy+6*scale), fs-2*scale, ink2))
		}
	}
	ln.Children = nil
	return g
}
