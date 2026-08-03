package widgets

import (
	"fmt"
	"image/color"
	"image"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("largetitle", LargeTitle{})
	canvas.RegisterWidget("cupertinolargetitle", LargeTitle{})
	canvas.RegisterWidget("sliverappbar", LargeTitle{})
}

// LargeTitle is the iOS large-title header (HTML render_widgets.go:1152):
// a 36px compact bar (accent action children on the right, 17/600 mini
// title) above the big title block (34/800, optional subtitle). The HTML
// collapses the big title on scroll; the canvas renders the expanded static
// form (the collapse is a scroll-driven browser effect).
type LargeTitle struct{}

// Measure: 36px compact bar + big-title block (34 line + optional 15px
// subtitle + paddings).
func (LargeTitle) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	h = 36 * scale
	h += (34 + 12) * scale // big title + its block padding
	if ltProp(n, "subtitle", rt) != "" {
		h += (15 + 2) * scale
	}
	return 0, h
}

func (l LargeTitle) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return l.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget (action children are
// laid out by the widget).
func (l LargeTitle) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return l.record(ln, rt, scale, sinks)
}

func (l LargeTitle) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 {
		return nil
	}
	barH := 36 * scale
	bigFs := 34 * scale
	subFs := 15 * scale

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	bg := draw.NewRect()
	bg.Width = float64(ln.Width)
	bg.Height = float64(ln.Height)
	bg.Fill = themeColor(rt, "background", color.RGBA{242, 242, 247, 255})
	g.AddChild(bg)

	// Compact bar: mini title centred, action children right slot.
	if title := ltProp(n0(ln), "label", rt); title != "" {
		fs := 17 * scale
		tw := int(canvas.MeasureText(title, float64(fs)))
		tx := (ln.Width - tw) / 2
		if tx < 44*scale {
			tx = 44 * scale
		}
		g.AddChild(formText(title, float64(tx), float64((barH-lineHeight(fs))/2), fs, formInk(ln.Node, ln, rt)))
	}
	if len(ln.Children) > 0 {
		ax := ln.Width - 12 * scale
		for _, cln := range ln.Children {
			cw := cln.Width + cln.Style.MarginLeft + cln.Style.MarginRight
			ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
			ax -= cw
			y := (barH - ch) / 2
			if y < 0 {
				y = 0
			}
			if cn := canvas.PerformLayoutWithSinks(cln, image.Rect(ax, y, ax+cw, y+ch), image.Pt(ln.AbsX, ln.AbsY), nil, rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
		}
		ln.Children = nil
	}

	// Big title block.
	y := barH + 6*scale
	if title := ltProp(n0(ln), "label", rt); title != "" {
		g.AddChild(formText(title, float64(16*scale), float64(y), bigFs, formInk(ln.Node, ln, rt)))
		y += int(float64(bigFs) * 1.2)
	}
	if sub := ltProp(n0(ln), "subtitle", rt); sub != "" {
		g.AddChild(formText(sub, float64(16*scale), float64(y+2*scale), subFs, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}

	// The block's own hairline under everything (HTML: the wrapper's rule
	// scrolls with the big title).
	sep := draw.NewRect()
	sep.Y = float64(ln.Height - scale)
	sep.Width = float64(ln.Width)
	sep.Height = float64(scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)
	return g
}

// ltProp evaluates one string prop (label/subtitle) against state.
func ltProp(n *model.Node, key string, rt *runtime.Runtime) string {
	raw, ok := n.Prop(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), map[string]any{"state": rt.State})))
}

