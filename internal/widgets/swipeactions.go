package widgets

import (
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("swipeactions", SwipeActions{})
	canvas.RegisterWidget("swipeaction", SwipeActions{})
}

// SwipeActions wraps one list row whose trailing actions reveal on a swipe
// (HTML render_widgets.go swipeActions). The canvas v1 renders the row with
// the iOS list hairline under it (surface background + bottom separator) so
// rows read as list rows; the drag-to-reveal gesture itself is a later
// milestone (the actions prop is parsed but not yet interactive).
type SwipeActions struct{}

// Measure reports the wrapped row's own size (the children measure through).
func (SwipeActions) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0 // content-sized from children (the generic pass measures them)
}

func (s SwipeActions) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return s.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the row content is
// laid out by the widget itself so the frame's sinks flow through.
func (s SwipeActions) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return s.record(ln, rt, scale, sinks)
}

func (s SwipeActions) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	bg := draw.NewRect()
	bg.Width = float64(ln.Width)
	bg.Height = float64(ln.Height)
	bg.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	g.AddChild(bg)

	// Row content flows from the top with the child's own height.
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), sinks.Inter, rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil // mounted (frame-local; Measure rebuilds every frame)

	// iOS list-row hairline under the row (indented like the system list).
	sep := draw.NewRect()
	sep.X = float64(16 * scale)
	sep.Y = float64(ln.Height - scale)
	sep.Width = float64(ln.Width - 16*scale)
	sep.Height = float64(scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)
	return g
}
