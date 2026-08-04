package widgets

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("dragtarget", &DragTarget{})
	canvas.RegisterWidget("droptarget", &DragTarget{})
}

// DragTarget is the drop zone for a Draggable: when a drag is in flight and
// the pointer RELEASES inside its bounds, it dispatches the `onDrop` handler
// (or the node's onPress) with the draggable's payload as {data}. It is an
// InteractiveWidget so the release over it routes here; the drop clears the
// engine's in-flight drag.
type DragTarget struct{}

// Measure reports the wrapped child's size (children measure through).
func (*DragTarget) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return contentMeasure(n, rt, scale)
}

func (t *DragTarget) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return t.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget.
func (t *DragTarget) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return t.record(ln, rt, scale, sinks)
}

func (t *DragTarget) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g
}

// HandlePointer consumes a drop: a release inside the target's bounds while a
// drag is in flight dispatches onDrop (or onPress) with {data} and clears the
// drag.
func (t *DragTarget) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerRelease || !inter.Drag.Active {
		return false
	}
	inside := p.X >= float64(frame.Min.X) && p.X < float64(frame.Max.X) &&
		p.Y >= float64(frame.Min.Y) && p.Y < float64(frame.Max.Y)
	data := inter.Drag.Data
	inter.Drag = canvas.DragState{} // consume the drag regardless of the point
	if !inside {
		return false
	}
	if raw, ok := n.Prop("onDrop"); ok {
		if inv := propInvokeWidget(raw); inv != nil {
			argAny := map[string]any{"data": data}
			for k, v := range inv.Args {
				argAny[k] = v
			}
			rt.Dispatch(inv.Name, argAny)
			return true
		}
	}
	if n.OnPress != nil {
		argAny := map[string]any{"data": data}
		for k, v := range n.OnPress.Args {
			argAny[k] = v
		}
		rt.Dispatch(n.OnPress.Name, argAny)
		return true
	}
	return false
}
