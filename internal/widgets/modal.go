package widgets

// The modal (HTML: render_feedback.go:99) — a blocking dialog: a dimmed
// backdrop with a centered surface panel holding the title and the children.
// The `open` prop (a binding) controls visibility; tapping the backdrop
// dispatches the node's onPress / `onDismiss` handler so the author can flip
// `open` off.

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
	canvas.RegisterWidget("modal", &Modal{geoms: map[*model.Node]*modalGeo{}})
}

// propInvokeWidget converts a {name, args} prop object (the modal's onDismiss
// spelling) into a dispatchable Invoke, or nil when absent or nameless.
func propInvokeWidget(raw any) *model.Invoke {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	inv := &model.Invoke{}
	if s, ok := m["name"].(string); ok {
		inv.Name = s
	}
	if args, ok := m["args"].(map[string]any); ok {
		inv.Args = make(map[string]string, len(args))
		for k, v := range args {
			inv.Args[k] = fmt.Sprint(v)
		}
	}
	if inv.Name == "" {
		return nil
	}
	return inv
}

// Modal is the blocking dialog overlay.
type Modal struct {
	mu    sync.Mutex
	geoms map[*model.Node]*modalGeo
}

type modalGeo struct {
	panel image.Rectangle // the panel rect in stage px (for backdrop taps)
}

func (m *Modal) geo(n *model.Node) *modalGeo {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.geoms[n]
	if g == nil {
		g = &modalGeo{}
		m.geoms[n] = g
	}
	return g
}

// modalOpen evaluates the `open` prop (a binding or literal).
func modalOpen(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("open")
	if !ok {
		return false
	}
	return formTruthy(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil)))
}

// modalDismiss resolves the close handler: the node's onPress, else the
// `onDismiss` prop ({name, args}).
func modalDismiss(n *model.Node) *model.Invoke {
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

// Measure reports the children's content size (the panel sizes to it), plus
// a title line when a title prop is set.
func (*Modal) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	if formLabel(n, rt) != "" {
		h += lineHeight(fs+4*scale) + 12*scale
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

func (m *Modal) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return nil // nothing in-flow; the overlay IS the modal
}

// OverlayOpen reports whether the modal is mounted (the `open` prop).
func (m *Modal) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return modalOpen(n, rt)
}

// OverlayRecord draws the backdrop and the centered panel with the children.
func (m *Modal) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !m.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	panelW := ln.Width
	panelH := ln.Height
	if panelW < 40*scale {
		panelW = 40 * scale
	}
	if panelW > stageW-40*scale {
		panelW = stageW - 40*scale
	}
	panelX := (stageW - panelW) / 2
	panelY := (stageH - panelH) / 2
	if panelY < 10*scale {
		panelY = 10 * scale
	}
	m.mu.Lock()
	m.geoms[ln.Node] = &modalGeo{panel: image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)}
	m.mu.Unlock()

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
	panel.BorderRadius = 14 * float64(scale)
	panel.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panel.ShadowColor = color.RGBA{0, 0, 0, 80}
	panel.ShadowBlur = 20 * float64(scale)
	panel.ShadowY = 8 * float64(scale)
	g.AddChild(panel)

	// Title, then the children stacked under it with padding.
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	cy := panelY + 20*scale
	if title := formLabel(ln.Node, rt); title != "" {
		g.AddChild(formText(title, float64(panelX+20*scale), float64(cy), fs+4*scale, ink))
		cy += lineHeight(fs+4*scale) + 12*scale
	}
	inner := image.Rect(panelX+20*scale, cy, panelX+panelW-20*scale, panelY+panelH-20*scale)
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, inner.Min.Y), canvas.SinksInter(nil), rt, scale, nil); cn != nil {
			g.AddChild(cn)
		}
		cy += ch + cln.Style.MarginTop + cln.Style.MarginBot
	}
	ln.Children = nil
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on the backdrop
// (outside the panel) or Escape dispatches the dismiss handler.
func (m *Modal) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type == canvas.PointerPress {
		g := m.geo(n)
		outside := p.X < float64(g.panel.Min.X) || p.X >= float64(g.panel.Max.X) ||
			p.Y < float64(g.panel.Min.Y) || p.Y >= float64(g.panel.Max.Y)
		if outside {
			if inv := modalDismiss(n); inv != nil {
				argAny := make(map[string]any, len(inv.Args))
				for k, v := range inv.Args {
					argAny[k] = v
				}
				rt.Dispatch(inv.Name, argAny)
			}
			return true
		}
	}
	return false
}
