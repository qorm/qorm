package widgets

// The bottom sheet (HTML: render_feedback.go:167) — a panel sliding from the
// bottom over a dimmed backdrop, holding the children. The `open` prop (a
// binding) controls visibility; tapping the backdrop dispatches the node's
// onPress / `onDismiss` handler. Same pattern as the modal, bottom-anchored.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("sheet", &Sheet{geoms: map[*model.Node]*sheetGeo{}})
	canvas.RegisterWidget("bottomsheet", &Sheet{geoms: map[*model.Node]*sheetGeo{}})
}

// Sheet is the bottom-anchored panel overlay.
type Sheet struct {
	mu    sync.Mutex
	geoms map[*model.Node]*sheetGeo
}

type sheetGeo struct {
	panel image.Rectangle
}

func (s *Sheet) geo(n *model.Node) *sheetGeo {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.geoms[n]
	if g == nil {
		g = &sheetGeo{}
		s.geoms[n] = g
	}
	return g
}

// sheetOpen evaluates the `open` prop (a binding or literal).
func sheetOpen(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("open")
	if !ok {
		return false
	}
	return formTruthy(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil)))
}

// sheetDismiss resolves the close handler: the node's onPress, else the
// `onDismiss` prop.
func sheetDismiss(n *model.Node) *model.Invoke {
	if n.OnPress != nil {
		return n.OnPress
	}
	if raw, ok := n.Prop("onDismiss"); ok {
		if inv := propInvokeWidget(raw); inv != nil {
			return inv
		}
	}
	return nil
}

// Measure reports the children's content size (the panel sizes to it).
func (*Sheet) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
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
	return w + 40*scale, h + 40*scale
}

func (s *Sheet) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return nil // the overlay IS the sheet
}

// OverlayOpen reports whether the sheet is mounted (the `open` prop).
func (s *Sheet) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return sheetOpen(n, rt)
}

// OverlayRecord draws the dimmed backdrop and the bottom-anchored panel.
func (s *Sheet) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !s.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	panelW := ln.Width
	if panelW > stageW {
		panelW = stageW
	}
	panelH := ln.Height
	if panelH > stageH-20*scale {
		panelH = stageH - 20 * scale
	}
	panelX := (stageW - panelW) / 2
	panelY := stageH - panelH
	s.mu.Lock()
	s.geoms[ln.Node] = &sheetGeo{panel: image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)}
	s.mu.Unlock()

	g := draw.NewGroup()
	g.Width = float64(stageW)
	g.Height = float64(stageH)
	g.Model = ln.Node
	g.Overlay = true

	backdrop := draw.NewRect()
	backdrop.Width = float64(stageW)
	backdrop.Height = float64(stageH)
	backdrop.Fill = color.RGBA{0, 0, 0, 115}
	g.AddChild(backdrop)

	panel := draw.NewRect()
	panel.X = float64(panelX)
	panel.Y = float64(panelY)
	panel.Width = float64(panelW)
	panel.Height = float64(panelH)
	panel.BorderRadius = 16 * float64(scale)
	panel.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panel.ShadowColor = color.RGBA{0, 0, 0, 80}
	panel.ShadowBlur = 20 * float64(scale)
	panel.ShadowY = -4 * float64(scale)
	g.AddChild(panel)

	// The children, stacked with padding from the panel top.
	inner := image.Rect(panelX+20*scale, panelY+20*scale, panelX+panelW-20*scale, panelY+panelH-20*scale)
	cy := inner.Min.Y
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(inner.Min.X, cy, inner.Max.X, cy+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, cy), canvas.SinksInter(nil), rt, scale, nil); cn != nil {
			g.AddChild(cn)
		}
		cy += ch + cln.Style.MarginTop + cln.Style.MarginBot
	}
	ln.Children = nil
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on the backdrop
// (outside the panel) dispatches the dismiss handler.
func (s *Sheet) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	g := s.geo(n)
	outside := p.X < float64(g.panel.Min.X) || p.X >= float64(g.panel.Max.X) ||
		p.Y < float64(g.panel.Min.Y) || p.Y >= float64(g.panel.Max.Y)
	if !outside {
		return false // taps inside the panel fall through to the children
	}
	if inv := sheetDismiss(n); inv != nil {
		argAny := make(map[string]any, len(inv.Args))
		for k, v := range inv.Args {
			argAny[k] = v
		}
		rt.Dispatch(inv.Name, argAny)
	}
	return true
}
