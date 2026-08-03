package widgets

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("appbar", AppBar{})
}

// AppBar is the top title bar of a scaffold (HTML render_widgets.go:169): a
// 44px frosted bar with a hairline bottom separator, an optional leading slot
// (accent), a centred 17/600 title, and action children pinned right. The
// software rasterizer has no backdrop blur, so the bar paints the surface
// colour opaque — the same call the select panel made.
type AppBar struct{}

// appBarHeight mirrors the HTML bar's 44px (safe-area inset is a browser
// concern; the canvas window has no notch).
func appBarHeight(scale int) int { return 44 * scale }

// Measure spans the parent's cross axis (flex stretch) at 44px tall.
func (AppBar) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 0, appBarHeight(scale)
}

func (a AppBar) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return a.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: action children are
// laid out by the widget itself, so the frame's sinks must flow through.
func (a AppBar) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return a.record(ln, rt, scale, sinks)
}

func (a AppBar) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 {
		return nil
	}
	barH := appBarHeight(scale)

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	bg := draw.NewRect()
	bg.Width = float64(ln.Width)
	bg.Height = float64(barH)
	bg.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	bg.Fill.A = 255 // no backdrop blur in the software rasterizer
	g.AddChild(bg)

	sep := draw.NewRect()
	sep.Y = float64(barH - scale)
	sep.Width = float64(ln.Width)
	sep.Height = float64(scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)

	accent := themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	slotW := 44 * scale
	if lead := barPropStr(ln.Node, "leading", rt); lead != "" {
		fs := 17 * scale
		g.AddChild(formText(lead, float64(8*scale), float64((barH-lineHeight(fs))/2), fs, accent))
	}

	// Centred 17/600 title (HTML: white-space:nowrap;overflow:hidden —
	// canvas text is single-line, which is the same constraint).
	label := barPropStr(ln.Node, "label", rt)
	if label == "" {
		label = barPropStr(ln.Node, "text", rt)
	}
	if label != "" {
		fs := 17 * scale
		tw := int(canvas.MeasureText(label, float64(fs)))
		tx := (ln.Width - tw) / 2
		if tx < slotW {
			tx = slotW
		}
		g.AddChild(formText(label, float64(tx), float64((barH-lineHeight(fs))/2), fs, formInk(ln.Node, ln, rt)))
	}

	// Action children pin to the right slot (iOS blue), laid out with the
	// frame's sinks forwarded (see Tabs).
	if len(ln.Children) > 0 {
		ax := ln.Width - 8*scale
		for _, cln := range ln.Children {
			cw := cln.Width + cln.Style.MarginLeft + cln.Style.MarginRight
			ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
			ax -= cw
			y := (barH - ch) / 2
			if y < 0 {
				y = 0
			}
			if cn := canvas.PerformLayoutWithSinks(cln, image.Rect(ax, y, ax+cw, y+ch), image.Pt(ln.AbsX, ln.AbsY), sinks.Inter, rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
		}
		// The actions are mounted; the generic pass must not re-mount them
		// (ln.Children is rebuilt by Measure every frame, so this is
		// frame-local).
		ln.Children = nil
	}

	return g
}

// barPropStr evaluates a string prop (bindings included), following the
// widgets package's formCtx convention.
func barPropStr(n *model.Node, key string, rt *runtime.Runtime) string {
	raw, ok := n.Props[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil))))
}
