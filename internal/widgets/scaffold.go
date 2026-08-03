package widgets

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("scaffold", Scaffold{})
}

// Scaffold is Flutter's Scaffold (HTML render_widgets.go:74): an appbar
// child pins to the top, bottomnav children pin to the bottom, and the rest
// is the body filling the space between. The widget lays its children out
// itself (ChildLayoutWidget) so the frame's sinks flow through — a select
// inside a scaffold's body keeps its overlay.
type Scaffold struct{}

// Measure reports the viewport height when known (the bar pins to the
// viewport bottom like a phone screen); width comes from the parent.
func (Scaffold) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if rt != nil && rt.Viewport.H > 0 {
		h = rt.Viewport.H
	}
	return 0, h
}

func (s Scaffold) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return s.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget.
func (s Scaffold) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return s.record(ln, rt, scale, sinks)
}

func (s Scaffold) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	abs := image.Pt(ln.AbsX, ln.AbsY)
	// align-items:stretch semantics for the scaffold column (same rule
	// tabs.record applies): an auto-width block child grows to the scaffold
	// width — the delegated PerformLayout has no flex kernel to do it, and
	// span-type widgets (appbar/bottomnav measure width 0) would otherwise
	// record nothing at all.
	stretchW := func(cln *canvas.LayoutNode) {
		if cln.Style.Width > 0 || cln.Style.WidthRaw != "" {
			return
		}
		if w, ok := canvas.LookupWidget(cln.Node.Type); ok {
			if _, inline := w.(canvas.InlineWidget); inline {
				return
			}
		}
		if cln.Width < ln.Width {
			cln.Width = ln.Width
		}
	}
	top := 0
	// App bars first (document order kept): they stack from the top.
	var bottoms, bodies []*canvas.LayoutNode
	for _, cln := range ln.Children {
		switch cln.Node.Type {
		case "appbar":
			stretchW(cln)
			bounds := image.Rect(0, top, ln.Width, top+cln.Height)
			if cn := canvas.PerformLayoutWithSinks(cln, bounds, abs, sinks.Inter, rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
			top += cln.Height
		case "bottomnav", "bottomnavigationbar", "navigationbar":
			bottoms = append(bottoms, cln)
		default:
			bodies = append(bodies, cln)
		}
	}
	bottomH := 0
	for _, cln := range bottoms {
		bottomH += cln.Height
	}
	// Body region: between the app bars and the bottom bars. Each body child
	// flows from the top of that region; the region itself gets the leftover
	// height so a fill-height body (the common scroll) reaches the bar.
	bodyY := top
	bodyH := ln.Height - top - bottomH
	if bodyH < 0 {
		bodyH = 0
	}
	for _, cln := range bodies {
		stretchW(cln)
		ch := cln.Height
		// Only an explicit fill-height body child takes the region height
		// (HTML puts flex:1 on the body WRAPPER, not on each child — filling
		// every auto-height child made the first one, a largetitle, consume
		// the entire body and push the rest off-window).
		if cln.Style.HeightRaw == "fill" && ch < bodyH {
			ch = bodyH
		}
		bounds := image.Rect(0, bodyY, ln.Width, bodyY+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, abs, sinks.Inter, rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		bodyY += ch
	}
	// Bottom bars stack upward from the scaffold's bottom edge.
	by := ln.Height
	for _, cln := range bottoms {
		stretchW(cln)
		by -= cln.Height
		bounds := image.Rect(0, by, ln.Width, by+cln.Height)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, abs, sinks.Inter, rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
	}
	// Children are mounted; the generic pass must not re-mount them
	// (ln.Children is rebuilt by Measure every frame, so this is frame-local).
	ln.Children = nil
	return g
}
