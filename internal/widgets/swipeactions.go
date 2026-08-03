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
	canvas.RegisterWidget("swipeactions", &SwipeActions{states: map[*model.Node]*swipeState{}})
	canvas.RegisterWidget("swipeaction", &SwipeActions{states: map[*model.Node]*swipeState{}})
}

// SwipeActions wraps one list row whose trailing actions reveal on a swipe
// left (HTML render_widgets.go swipeActions, driven by the `actions` prop:
// [{name, args, color, label}]). The row content sits on top of a fixed-width
// action strip; dragging the row left shifts it by the drag delta, and on
// release it snaps open (past half the strip) or closed. Tapping an open
// action dispatches its handler; tapping the open content closes the row.
// The row's iOS list hairline (surface background + bottom separator) is
// kept so rows read as list rows.
type SwipeActions struct {
	mu     sync.Mutex
	states map[*model.Node]*swipeState
}

// swipeState is the drag/reveal state of one row: the content's X shift in
// [-maxReveal, 0] (negative = shifted left, revealing the right-side actions),
// plus the drag anchors. Cross-frame (Record re-runs every frame), keyed by the
// stable model pointer like the engine's own interaction state.
type swipeState struct {
	offset      float64
	maxReveal   float64
	dragStartX  float64
	dragStartOf float64
	dragging    bool
}

// swipeActionW is each action's fixed width in physical px (the HTML path uses
// min-width:76px).
const swipeActionW = 76

func (s *SwipeActions) state(n *model.Node) *swipeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.states[n]
	if st == nil {
		st = &swipeState{}
		s.states[n] = st
	}
	return st
}

// swipeAction is one trailing action parsed from the `actions` prop.
type swipeAction struct {
	name  string
	args  map[string]string
	color string
	label string
}

func parseSwipeActions(n *model.Node) []swipeAction {
	var out []swipeAction
	raw, ok := n.Prop("actions")
	if !ok {
		return out
	}
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, a := range arr {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		act := swipeAction{}
		if v, ok := m["name"].(string); ok {
			act.name = v
		}
		if v, ok := m["label"].(string); ok {
			act.label = v
		}
		if v, ok := m["color"].(string); ok {
			act.color = v
		}
		if args, ok := m["args"].(map[string]any); ok {
			act.args = make(map[string]string, len(args))
			for k, v := range args {
				act.args[k] = fmt.Sprint(v)
			}
		}
		out = append(out, act)
	}
	return out
}

// Measure reports the wrapped row's own size (the children measure through).
func (s *SwipeActions) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0 // content-sized from children (the generic pass measures them)
}

func (s *SwipeActions) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return s.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the row content is
// laid out by the widget itself so the frame's sinks flow through.
func (s *SwipeActions) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return s.record(ln, rt, scale, sinks)
}

func (s *SwipeActions) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	actions := parseSwipeActions(ln.Node)
	st := s.state(ln.Node)
	// Re-clamp the persisted reveal against the current action strip (the prop
	// or the strip width may have changed between frames).
	st.maxReveal = float64(len(actions) * swipeActionW * scale)
	if st.offset > 0 {
		st.offset = 0
	}
	if st.offset < -st.maxReveal {
		st.offset = -st.maxReveal
	}

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)
	// The whole row is clipped to its box: a revealed row shows the actions at
	// the right but never spills its shifted content off the left edge.
	g.AddChild(newClipLeaf(ln.Width, ln.Height))

	// The trailing action strip sits behind the content at the right.
	if len(actions) > 0 {
		ag := draw.NewGroup()
		ag.X = float64(ln.Width) - st.maxReveal
		for i, a := range actions {
			rect := draw.NewRect()
			rect.X = float64(i * swipeActionW * scale)
			rect.Width = float64(swipeActionW * scale)
			rect.Height = float64(ln.Height)
			rect.Fill = canvas.ResolveColor(a.color, rt)
			if rect.Fill.A == 0 {
				rect.Fill = color.RGBA{255, 59, 48, 255} // the HTML path's --danger default
			}
			ag.AddChild(rect)
			if a.label != "" {
				fs := 13 * scale
				tw := int(canvas.MeasureText(a.label, float64(fs)))
				tx := float64(i*swipeActionW*scale) + (float64(swipeActionW*scale)-float64(tw))/2
				ag.AddChild(formText(a.label, tx, float64(ln.Height)/2-float64(lineHeight(fs))/2, fs, color.RGBA{255, 255, 255, 255}))
			}
		}
		g.AddChild(ag)
	}

	// The row content, on top, shifted left by the reveal offset. Its surface
	// background travels WITH the content, so a closed row's bg covers the
	// actions and an open row's shifted bg reveals exactly the strip.
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
	ln.Children = nil // mounted (frame-local; Measure rebuilds every frame)
	g.AddChild(cg)

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

// HandlePointer implements canvas.InteractiveWidget: it owns the row's input
// so the reveal drag and the action taps work. frame is the row's rendered
// box in screen px. Tapping an open action dispatches its handler; a press on
// the open content closes the row; a horizontal drag shifts the reveal.
func (s *SwipeActions) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	st := s.state(n)
	switch p.Type {
	case canvas.PointerPress:
		// Take pointer capture so the whole drag stream (moves and the release,
		// even off the row) stays with this widget — without it the engine only
		// routes while the pointer is over the row's rendered bounds.
		inter.Pressed = n
		// While open, a press in the revealed strip taps an action; elsewhere
		// it closes the row and arms a fresh drag.
		if st.offset < 0 && p.X >= float64(frame.Max.X)-st.maxReveal {
			if acts := parseSwipeActions(n); len(acts) > 0 {
				// Each action is maxReveal/len(acts) physical px wide (the
				// recorded strip already accounts for the device scale); the
				// strip's true left edge is Max.X-maxReveal regardless of the
				// current (partial) reveal.
				perW := st.maxReveal / float64(len(acts))
				idx := int(p.X-(float64(frame.Max.X)-st.maxReveal)) / int(perW)
				if idx >= 0 && idx < len(acts) && acts[idx].name != "" {
					argAny := make(map[string]any, len(acts[idx].args))
					for k, v := range acts[idx].args {
						argAny[k] = v
					}
					rt.Dispatch(acts[idx].name, argAny)
				}
			}
			st.offset = 0
			return true
		}
		st.offset = 0
		st.dragging = true
		st.dragStartX = p.X
		st.dragStartOf = 0
	case canvas.PointerMove:
		if st.dragging && p.Buttons > 0 {
			st.offset = st.dragStartOf + (p.X - st.dragStartX)
			if st.offset > 0 {
				st.offset = 0
			}
			if st.offset < -st.maxReveal {
				st.offset = -st.maxReveal
			}
			return true
		}
	case canvas.PointerRelease:
		if st.dragging {
			st.dragging = false
			// Snap open past half the strip, else closed.
			if st.offset < -st.maxReveal/2 {
				st.offset = -st.maxReveal
			} else {
				st.offset = 0
			}
			return true
		}
	}
	return false
}
