package canvas

import (
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// timerMinEveryMS floors a timer node's `every` interval for the canvas engine.
//
// The canvas engine owns its frame loop (frame.go / engine.go RenderInto) and
// drives timers from it, so a 16ms floor (60fps) is safe — that is the
// physical frame budget the host already runs at. The HTML path keeps
// render.TimerMinEveryMS = 250 because client-side setTimeout at finer
// intervals wastes CPU on polling; the two engines stay decoupled and only
// the canvas floor is this small.
//
// Game authors who want 60fps physics (mario, raiden) write every: 16 here.
const timerMinEveryMS = 16 * time.Millisecond

// sceneTimer is the engine's live record for one mounted timer node. Entries
// key by the node's model pointer — the same identity the Interaction uses —
// and are dropped the frame the node unmounts or hides (if/visible/show
// turning falsy), the canvas mirror of the HTML marker-removal semantics.
type sceneTimer struct {
	invoke   *model.Invoke
	every    time.Duration // repeating schedule; 0 for a one-shot `after`
	nextFire time.Time
	fired    bool // a one-shot already fired; kept (for `seen`) but never refires
}

// tickTimers scans the mounted scene for timer nodes and dispatches the
// onTick of every one whose deadline has passed. It runs at the top of
// RenderInto — after the external-mutation drain, before layout — so a fired
// timer's state writes land in the frame about to be recorded, the same
// ordering an input event gets.
//
// The HTML path schedules timers in client JS; the native engine has no
// client, so the frame loop itself is the scheduler: while any timer is
// pending the engine reports Animating (timersPending) and the host's poll
// loop keeps calling RenderInto. A scene that wants the loop to settle hides
// its timer with an `if` prop — a game pauses its gravity exactly like an
// AnimatedWidget settling.
func (e *Engine) tickTimers(root *model.Node) {
	now := time.Now()
	if e.timers == nil {
		e.timers = map[*model.Node]*sceneTimer{}
	}
	seen := map[*model.Node]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || !nodeVisible(n, e.RT) {
			return
		}
		if n.Type == "when" {
			walk(whenBranch(n, e.RT))
			return
		}
		if n.Type == "timer" {
			e.tickTimer(n, now, seen)
			return // a timer renders no children (HTML emits a marker span)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	for n := range e.timers {
		if !seen[n] {
			delete(e.timers, n) // unmounted or hidden: cancel the schedule
		}
	}
}

// tickTimer registers or fires one timer node. The schedule is re-resolved
// every frame (the `every`/`after` props may bind state — a game's gravity
// speeds up with its level), and an interval change re-arms the countdown.
func (e *Engine) tickTimer(n *model.Node, now time.Time, seen map[*model.Node]bool) {
	if n.ID == "" {
		return // no idempotency key — the loader warns; nothing to schedule
	}
	inv := timerOnTick(n)
	every := timerDurProp(n, "every", e.RT)
	after := timerDurProp(n, "after", e.RT)
	if every > 0 && every < timerMinEveryMS {
		every = timerMinEveryMS
	}
	if inv == nil || (every <= 0 && after <= 0) {
		return // no schedule — mirrors the HTML no-op (the loader warns)
	}
	seen[n] = true
	t := e.timers[n]
	if t == nil {
		t = &sceneTimer{invoke: inv, every: every}
		if every > 0 {
			t.nextFire = now.Add(every)
		} else {
			t.nextFire = now.Add(after)
		}
		e.timers[n] = t
		return
	}
	t.invoke = inv
	if every > 0 && t.every != every { // interval re-bound mid-run: re-arm
		t.every = every
		t.fired = false
		t.nextFire = now.Add(every)
	}
	if t.fired || now.Before(t.nextFire) {
		return
	}
	if t.every > 0 {
		t.nextFire = now.Add(t.every)
	} else {
		t.fired = true
	}
	e.dispatch(t.invoke, nil)
	e.dirty.Store(true)
}

// timersPending reports whether any mounted timer is still live (a repeating
// one always is; a one-shot until it fires). It joins Animating, so the
// host's poll loop stays alive for the next deadline and settles when only
// spent one-shots (or nothing) remain.
func (e *Engine) timersPending() bool {
	for _, t := range e.timers {
		if !t.fired {
			return true
		}
	}
	return false
}

// timerDurProp resolves a millisecond prop that may be a number literal or a
// {{...}} binding (evaluated against state + viewport) — the canvas mirror of
// render_timer.go's timerMSProp.
func timerDurProp(n *model.Node, key string, rt *runtime.Runtime) time.Duration {
	raw, ok := n.Prop(key)
	if !ok {
		return 0
	}
	if s, isStr := raw.(string); isStr {
		f, _ := runtime.EvalBinding(s, evalCtx(rt)).(float64)
		return time.Duration(f) * time.Millisecond
	}
	if f, isNum := raw.(float64); isNum {
		return time.Duration(f) * time.Millisecond
	}
	return 0
}

// timerOnTick reads the onTick handler: the string shorthand ("onTick":
// "refresh") or the {name, args} invoke form — mirroring timerInvoke in
// render_timer.go so one JSON drives both engines.
func timerOnTick(n *model.Node) *model.Invoke {
	raw, ok := n.Prop("onTick")
	if !ok {
		return nil
	}
	if name, isStr := raw.(string); isStr {
		if name == "" {
			return nil
		}
		return &model.Invoke{Name: name, Args: map[string]string{}}
	}
	if m, isMap := raw.(map[string]any); isMap {
		name, _ := m["name"].(string)
		if name == "" {
			return nil
		}
		inv := &model.Invoke{Name: name, Args: map[string]string{}}
		if am, ok := m["args"].(map[string]any); ok {
			for k, v := range am {
				if s, ok := v.(string); ok {
					inv.Args[k] = s
				}
			}
		}
		return inv
	}
	return nil
}
