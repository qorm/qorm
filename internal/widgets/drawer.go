package widgets

import (
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("drawer", &Drawer{opens: map[*model.Node]bool{}, geom: map[*model.Node]image.Rectangle{}})
	canvas.RegisterWidget("navigationdrawer", &Drawer{opens: map[*model.Node]bool{}, geom: map[*model.Node]image.Rectangle{}})
}

// Drawer is a side panel that overlays the scene when `open` is truthy (the
// author toggles state, HTML render_widgets.go drawer): a semi-transparent
// backdrop plus a panel anchored left or right (default right, width
// min(80%, 320px)) holding the children. A press on the backdrop closes it —
// writing the bound `open` state back to false when the prop is a binding —
// and a press on the panel passes through to the panel's own content.
type Drawer struct {
	mu    sync.Mutex
	opens map[*model.Node]bool
	geom  map[*model.Node]image.Rectangle // last-drawn panel bounds (screen px)
}

// Measure reports nothing (the panel is the overlay).
func (*Drawer) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0
}

func (d *Drawer) isOpen(n *model.Node) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.opens[n]
}

// Record renders nothing in flow: the drawer is entirely an overlay.
func (*Drawer) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return nil
}

// OverlayOpen reports whether the bound `open` prop is truthy (the drawer is
// mounted as an overlay above the scene). A string prop is evaluated (a
// binding like {{state.open}}); a bool prop is used directly.
func (d *Drawer) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("open")
	if !ok {
		return d.isOpen(n)
	}
	if s, ok := raw.(string); ok {
		return formTruthy(runtime.EvalBinding(s, formCtxLn(rt, nil)))
	}
	return formTruthy(raw)
}

// OverlayRecord draws the backdrop + side panel with the children flowing
// vertically inside it.
func (d *Drawer) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !d.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	side := "right"
	if v, ok := ln.Node.Prop("side"); ok {
		if s, ok := v.(string); ok && s == "left" {
			side = "left"
		}
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	panelW := stageW * 80 / 100
	if panelW > 320*scale {
		panelW = 320 * scale
	}

	root := draw.NewGroup()
	root.Width = float64(stageW)
	root.Height = float64(stageH)
	root.Model = ln.Node
	root.Overlay = true

	// Backdrop: a press anywhere outside the panel closes the drawer.
	backdrop := draw.NewRect()
	backdrop.Width = float64(stageW)
	backdrop.Height = float64(stageH)
	backdrop.Fill = color.RGBA{0, 0, 0, 100}
	root.AddChild(backdrop)

	panel := draw.NewGroup()
	panelX := stageW - panelW
	if side == "left" {
		panelX = 0
	}
	panelRect := draw.NewRect()
	panelRect.X = float64(panelX)
	panelRect.Width = float64(panelW)
	panelRect.Height = float64(stageH)
	panelRect.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panel.AddChild(panelRect)

	// The children flow vertically inside the panel (padding 20).
	y := 20 * scale
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(panelX+20*scale, y, panelX+panelW-20*scale, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX+panelX+20*scale, ln.AbsY+y), canvas.SinksInter(nil), rt, scale, nil); cn != nil {
			panel.AddChild(cn)
		}
		y += ch + 12*scale
	}
	ln.Children = nil
	root.AddChild(panel)

	// Record the panel bounds for backdrop/panel press disambiguation.
	d.mu.Lock()
	d.geom[ln.Node] = image.Rect(panelX, 0, panelX+panelW, stageH)
	d.mu.Unlock()
	return root
}

// HandlePointer implements canvas.InteractiveWidget: a press on the backdrop
// (outside the panel) closes the drawer, writing the bound `open` prop back to
// false when it is a binding; a press on the panel is left to the panel's own
// content (the widget does not consume it).
func (d *Drawer) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	d.mu.Lock()
	panel := d.geom[n]
	d.mu.Unlock()
	if p.X >= float64(panel.Min.X) && p.X <= float64(panel.Max.X) {
		return false // a press on the panel passes through to its content
	}
	// Backdrop: close (write the bound state back to false).
	if raw, ok := n.Prop("open"); ok {
		if s, ok := raw.(string); ok {
			if path := formBoundPath(s); path != "" {
				rt.SetStatePath(path, false)
			}
		}
	}
	d.mu.Lock()
	d.opens[n] = false
	d.mu.Unlock()
	return true
}
