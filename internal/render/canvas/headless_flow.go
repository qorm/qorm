package canvas

// Deterministic headless-flow seams. These methods are intentionally small:
// production hosts still use wall-clock frames and real pointer coordinates,
// while `qorm check` can reveal a target, drive the same input handlers and
// advance every canvas-owned clock without sleeping.

import (
	"image"
	"time"

	"github.com/qorm/qorm/internal/render/graph"
)

// AdvanceTime moves all canvas-owned deadlines and animation anchors back by
// d, which is equivalent to advancing the engine's clock by d before its next
// frame. It never sleeps and does not alter process wall time. The caller
// should render once before advancing so newly-triggered effects have mounted.
func (e *Engine) AdvanceTime(d time.Duration) {
	if e == nil || d <= 0 {
		return
	}
	for _, timer := range e.timers {
		if timer != nil && !timer.nextFire.IsZero() {
			timer.nextFire = timer.nextFire.Add(-d)
		}
	}
	for _, st := range e.Inter.Entrance {
		if st != nil {
			st.start = st.start.Add(-d)
		}
	}
	for _, st := range e.Inter.FX {
		if st != nil {
			st.start = st.start.Add(-d)
		}
	}
	for _, st := range e.Inter.Timeline {
		if st != nil && st.seq != nil {
			st.seq.StartTime = st.seq.StartTime.Add(-d)
		}
	}
	if s := e.Inter.Input; s != nil && !s.BlinkStart.IsZero() {
		s.BlinkStart = s.BlinkStart.Add(-d)
	}
	if c := e.Inter.Click; c != nil && !c.lastTime.IsZero() {
		c.lastTime = c.lastTime.Add(-d)
	}
	for n, mom := range e.Inter.ScrollMomentum {
		if !mom.LastTime.IsZero() {
			mom.LastTime = mom.LastTime.Add(-d)
			e.Inter.ScrollMomentum[n] = mom
		}
	}
	if !e.Inter.ScrollDrag.MomLast.IsZero() {
		e.Inter.ScrollDrag.MomLast = e.Inter.ScrollDrag.MomLast.Add(-d)
	}
	if !e.Inter.Board.PanMomLast.IsZero() {
		e.Inter.Board.PanMomLast = e.Inter.Board.PanMomLast.Add(-d)
	}
	// Property tweens and FLIP currently live in package-level retained stores;
	// advance their controller anchors too so transition waits settle without
	// sleeping. The canvas engine is single-threaded by contract.
	for _, st := range globalAnimStates {
		if st != nil && st.Controller != nil {
			st.Controller.StartTime = st.Controller.StartTime.Add(-d)
		}
	}
	for _, st := range globalFLIP {
		if st != nil && st.ctrl != nil {
			st.ctrl.StartTime = st.ctrl.StartTime.Add(-d)
		}
	}
	e.MarkDirty()
}

// BoundsByID returns the first rendered node with id, including widget overlay
// panels such as modals and menus. Coordinates are physical pixels, matching
// HandlePointer/HandleScroll.
func (e *Engine) BoundsByID(id string) (image.Rectangle, bool) {
	n := e.graphByID(id)
	if n == nil {
		return image.Rectangle{}, false
	}
	b := n.GetBBox()
	return image.Rect(int(b.MinX), int(b.MinY), int(b.MaxX), int(b.MaxY)), true
}

// RevealID scrolls every ancestor viewport enough to expose the first node
// with id. It uses the keyboard focus scrolling primitive but does not change
// focus, so a following pointer event is still the thing that activates it.
func (e *Engine) RevealID(id string) bool {
	n := e.graphByID(id)
	if n == nil || n.Base().Model == nil {
		return false
	}
	e.ensureFocusVisible(n.Base().Model)
	return true
}

// FocusID establishes focus on a rendered id for a headless key/type step.
// keyboard controls focus-visible; ordinary flow typing passes false.
func (e *Engine) FocusID(id string, keyboard bool) bool {
	n := e.graphByID(id)
	if n == nil || n.Base().Model == nil || nodeDisabled(n.Base().Model, e.RT) {
		return false
	}
	m := n.Base().Model
	e.Inter.Focused = m
	e.Inter.FocusedItem = 0
	e.Inter.FocusVisible = keyboard
	e.ensureFocusVisible(m)
	e.notifyWidgetFocused(m)
	e.syncEditSession()
	e.MarkDirty()
	return true
}

func (e *Engine) graphByID(id string) graph.Node {
	if e == nil || id == "" || e.graphRoot == nil {
		return nil
	}
	var found graph.Node
	var walk func(graph.Node) bool
	walk = func(n graph.Node) bool {
		if n == nil {
			return false
		}
		if m := n.Base().Model; m != nil && m.ID == id {
			found = n
			return true
		}
		for _, c := range n.Base().Children {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(e.graphRoot)
	return found
}
