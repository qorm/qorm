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
	canvas.RegisterWidget("gesturedetector", &GestureDetector{states: map[*model.Node]*gestureState{}})
	canvas.RegisterWidget("gesture", &GestureDetector{states: map[*model.Node]*gestureState{}})
	canvas.RegisterWidget("inkwell", &GestureDetector{states: map[*model.Node]*gestureState{}})
}

// GestureDetector is Flutter's GestureDetector / InkWell (HTML render_gesture.go
// gestureDetector): wraps children and wires tap (onPress), double-tap
// (onDoubleTap) and long-press (onLongPress) to action dispatches. The
// detector owns its subtree's input: a drag past the tap slop cancels the
// gesture; a hold past the long-press duration fires onLongPress on release;
// otherwise a quick release is a tap — onPress always fires, and a second tap
// within the double-tap window ALSO fires onDoubleTap (the browser's onclick +
// ondblclick both fire). The pointer cursor over it is the interactive-widget
// hint.
type GestureDetector struct {
	mu     sync.Mutex
	states map[*model.Node]*gestureState
}

type gestureState struct {
	pressPt geom.Point
	pressAt time.Time
	down    bool
	moved   bool // drifted past the slop → a drag, cancels the gesture
	lastTap time.Time
}

const (
	gestureTapSlop     = 10.0 // px of drift before a press is a drag
	gestureLongPressMs = 500 * time.Millisecond
	gestureDoubleTapMs = 350 * time.Millisecond
)

func (g *GestureDetector) state(n *model.Node) *gestureState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.states[n]
	if st == nil {
		st = &gestureState{}
		g.states[n] = st
	}
	return st
}

// Measure reports the wrapped child's size (children measure through).
func (*GestureDetector) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0
}

func (g *GestureDetector) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return g.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the wrapped content
// flows through the generic pass with the frame's sinks.
func (g *GestureDetector) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return g.record(ln, rt, scale, sinks)
}

func (g *GestureDetector) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g2 := draw.NewGroup()
	g2.Width = float64(ln.Width)
	g2.Height = float64(ln.Height)
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g2.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g2
}

// HandlePointer implements canvas.InteractiveWidget: classifies the press
// stream into tap / double-tap / long-press and dispatches the matching
// handlers, reporting whether anything fired (the dispatch may change state,
// so the engine should repaint).
func (g *GestureDetector) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	st := g.state(n)
	fired := false
	switch p.Type {
	case canvas.PointerPress:
		st.down = true
		st.moved = false
		st.pressPt = geom.Point{X: p.X, Y: p.Y}
		st.pressAt = time.Now()
	case canvas.PointerMove:
		if st.down && p.Buttons > 0 && !st.moved &&
			math.Hypot(p.X-st.pressPt.X, p.Y-st.pressPt.Y) > gestureTapSlop {
			st.moved = true // a drag cancels the tap/long-press
		}
	case canvas.PointerRelease:
		if !st.down {
			break
		}
		st.down = false
		if st.moved {
			break
		}
		if time.Since(st.pressAt) >= gestureLongPressMs {
			fired = dispatchGesture(n, rt, "onLongPress") || fired
			break
		}
		// A tap: onPress always fires; a second tap within the window also
		// fires onDoubleTap (the browser's onclick + ondblclick both fire).
		if n.OnPress != nil {
			dispatchInvoke(n.OnPress, rt)
			fired = true
		}
		if time.Since(st.lastTap) < gestureDoubleTapMs {
			fired = dispatchGesture(n, rt, "onDoubleTap") || fired
		}
		st.lastTap = time.Now()
	}
	return fired
}

// dispatchGesture resolves a {name, args} handler prop (onDoubleTap,
// onLongPress) and dispatches it, reporting whether it fired.
func dispatchGesture(n *model.Node, rt *runtime.Runtime, prop string) bool {
	raw, ok := n.Prop(prop)
	if !ok {
		return false
	}
	if inv := propInvokeWidget(raw); inv != nil {
		dispatchInvoke(inv, rt)
		return true
	}
	return false
}

// dispatchInvoke dispatches an Invoke's action with its args.
func dispatchInvoke(inv *model.Invoke, rt *runtime.Runtime) {
	argAny := make(map[string]any, len(inv.Args))
	for k, v := range inv.Args {
		argAny[k] = v
	}
	rt.Dispatch(inv.Name, argAny)
}
