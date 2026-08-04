package widgets

import (
	"image"
	"math"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("draggable", &Draggable{states: map[*model.Node]*dragState{}})
	canvas.RegisterWidget("longpressdraggable", &Draggable{states: map[*model.Node]*dragState{}})
}

// Draggable wraps children (the drag handle) and publishes a cross-panel drag
// when the pointer drags it: a press followed by movement past the slop sets
// Interaction.Drag with the evaluated `data` payload, which a DragTarget
// consumes on drop. It takes pointer capture on press (the slop-crossing move
// may leave its narrow bounds) and releases it the moment the drag publishes —
// from then on the pointer's moves and release route to whatever is under it
// (the drop target), not back to the source.
type Draggable struct {
	mu     sync.Mutex
	states map[*model.Node]*dragState
}

type dragState struct {
	pressPt   geom.Point
	pressAt   time.Time
	down      bool
	published bool
}

const dragStartSlop = 6.0 // px of movement before a press becomes a drag

func (d *Draggable) state(n *model.Node) *dragState {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.states[n]
	if st == nil {
		st = &dragState{}
		d.states[n] = st
	}
	return st
}

// Measure reports the wrapped child's size (children measure through).
func (*Draggable) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return contentMeasure(n, rt, scale)
}

func (d *Draggable) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return d.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget.
func (d *Draggable) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return d.record(ln, rt, scale, sinks)
}

func (d *Draggable) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
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

// HandlePointer publishes the drag once the press moves past the slop (or,
// for longpressdraggable, once the hold passes the long-press duration). The
// `data` prop is evaluated at drag start so a bound payload carries the live
// value.
//
// Capture is taken on press (the slop-crossing move may leave the draggable's
// narrow bounds) and RELEASED the moment the drag publishes — from then on the
// pointer's moves and release route to whatever is under it, which is exactly
// what lets a DragTarget see the drop.
func (d *Draggable) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	st := d.state(n)
	switch p.Type {
	case canvas.PointerPress:
		inter.Pressed = n
		st.down = true
		st.published = false
		st.pressPt = geom.Point{X: p.X, Y: p.Y}
		st.pressAt = time.Now()
	case canvas.PointerMove:
		if st.down && p.Buttons > 0 && !st.published {
			drift := math.Hypot(p.X-st.pressPt.X, p.Y-st.pressPt.Y)
			hold := n.Type == "longpressdraggable" && time.Since(st.pressAt) >= gestureLongPressMs
			if drift > dragStartSlop || hold {
				st.published = true
				inter.Pressed = nil // hand the stream back to the drop targets
				inter.Drag = canvas.DragState{Active: true, Data: dragData(n, rt)}
			}
		}
	case canvas.PointerRelease:
		st.down = false
	}
	return false
}

// dragData evaluates the draggable's `data` prop (a binding is resolved
// against state) — the payload a dragtarget's onDrop receives.
func dragData(n *model.Node, rt *runtime.Runtime) string {
	raw, ok := n.Prop("data")
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		if rt != nil {
			if v := runtime.EvalBinding(s, map[string]any{"state": rt.State}); v != nil {
				if str, ok := v.(string); ok {
					return str
				}
				return runtime.Stringify(v)
			}
		}
		return s
	}
	return runtime.Stringify(raw)
}
