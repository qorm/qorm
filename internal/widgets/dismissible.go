package widgets

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
	canvas.RegisterWidget("dismissible", &Dismissible{states: map[*model.Node]*dismissState{}})
}

// Dismissible is the swipe-to-delete row (HTML render_gesture.go dismissible):
// the row content sits on a red danger background; swiping it left past half
// the row width dismisses it — the content slides fully off, the row stays
// collapsed until the parent re-renders, and the onPress (or `onDismissed`
// prop) handler dispatches so an owning list can drop the item from its data.
type Dismissible struct {
	mu     sync.Mutex
	states map[*model.Node]*dismissState
}

type dismissState struct {
	offset     float64 // content shift, [-(rowWidth), 0]
	rowWidth   float64
	dragStartX float64
	dragStartOf float64
	dragging   bool
	dismissed  bool
}

func (d *Dismissible) state(n *model.Node) *dismissState {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.states[n]
	if st == nil {
		st = &dismissState{}
		d.states[n] = st
	}
	return st
}

// Measure reports the wrapped row's own size (children measure through).
func (d *Dismissible) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0
}

func (d *Dismissible) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return d.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget (row content flows).
func (d *Dismissible) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return d.record(ln, rt, scale, sinks)
}

func (d *Dismissible) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	st := d.state(ln.Node)
	st.rowWidth = float64(ln.Width)
	if st.dismissed {
		st.offset = -st.rowWidth // keep the row slid out until the parent removes it
	}

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)
	g.AddChild(newClipLeaf(ln.Width, ln.Height))

	// The red danger background, revealed as the content slides left.
	danger := draw.NewRect()
	danger.Width = float64(ln.Width)
	danger.Height = float64(ln.Height)
	danger.Fill = themeColor(rt, "danger", color.RGBA{255, 59, 48, 255})
	g.AddChild(danger)

	// The row content on top, its surface background travelling with it so a
	// closed row hides the red and a slid row reveals exactly the swipe.
	cg := draw.NewGroup()
	cg.X = st.offset
	contentBG := draw.NewRect()
	contentBG.Width = float64(ln.Width)
	contentBG.Height = float64(ln.Height)
	contentBG.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	cg.AddChild(contentBG)
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX+int(st.offset), ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			cg.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	g.AddChild(cg)
	return g
}

// HandlePointer implements canvas.InteractiveWidget: the drag slides the
// content left; releasing past half the row width dismisses (dispatch onPress
// or `onDismissed`), otherwise it snaps back.
func (d *Dismissible) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	st := d.state(n)
	switch p.Type {
	case canvas.PointerPress:
		// Take pointer capture so the whole drag stream stays with this widget
		// even when the finger leaves the row; a press on a dismissed row also
		// clears the dismissal so a drag-back can recover it (a parent that
		// did NOT remove the node leaves the row recoverable instead of stuck).
		inter.Pressed = n
		st.dismissed = false
		st.dragging = true
		st.dragStartX = p.X
		st.dragStartOf = st.offset
	case canvas.PointerMove:
		if st.dragging && p.Buttons > 0 {
			st.offset = st.dragStartOf + (p.X - st.dragStartX)
			if st.offset > 0 {
				st.offset = 0
			}
			if st.offset < -st.rowWidth {
				st.offset = -st.rowWidth
			}
			return true
		}
	case canvas.PointerRelease:
		if st.dragging {
			st.dragging = false
			if st.offset < -st.rowWidth/2 {
				st.dismissed = true
				if inv := dismissInvoke(n); inv != nil {
					argAny := make(map[string]any, len(inv.Args))
					for k, v := range inv.Args {
						argAny[k] = v
					}
					rt.Dispatch(inv.Name, argAny)
				}
				return true
			}
			st.offset = 0 // snapped back
			return true
		}
	}
	return false
}

// dismissInvoke resolves the dismissal handler: the node's onPress, else the
// `onDismissed` prop ({name, args}).
func dismissInvoke(n *model.Node) *model.Invoke {
	if n.OnPress != nil {
		return n.OnPress
	}
	if raw, ok := n.Prop("onDismissed"); ok {
		if m, ok := raw.(map[string]any); ok {
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
			if inv.Name != "" {
				return inv
			}
		}
	}
	return nil
}
