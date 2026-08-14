package render

import (
	"fmt"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// TimerMinEveryMS is the floor for a timer's `every` interval in milliseconds.
// A lower value is clamped here at render time (the loader also warns), so a
// mis-typed interval (every: 1) cannot hammer the /event endpoint into a
// self-inflicted denial of service.
//
// The floor is 16ms (60fps frame budget) rather than 250ms: modern browsers
// can sustain requestAnimationFrame / setTimeout at 16ms without
// performance impact, and game / animation apps need 60fps physics to
// feel responsive. The HTML path used to enforce 250ms for the historical
// reason that 60fps polling "felt heavy"; that turned out to be true only
// for trivial timers with no state to recompute, and our /event handler
// can absorb the rate. The canvas path enforces the same 16ms.
const TimerMinEveryMS = 16

// timer renders the declarative time primitive as an INVISIBLE marker element;
// the client-side scheduler (qormTimersSync in app.js, re-run after every
// morph) reconciles the live DOM markers against its timer registry:
//
//   - `every` (ms, >= TimerMinEveryMS) fires onTick repeatedly — polling;
//   - `after` (ms) fires onTick once — a delayed one-shot (`every` wins when
//     both are set);
//   - the tick dispatches through qorm(h), the exact same /event (server) or
//     qormEvent (WASM) chain a button press uses;
//   - the marker's presence in the RENDERED tree is the timer's lifecycle: an
//     `if`/`visible` prop that turns falsy removes the marker and the client
//     stops the schedule (a countdown reaching zero stops itself), and the
//     same id after a re-render/morph is recognised — never scheduled twice;
//   - data-h is re-read from the live DOM at fire time, so a morph that
//     renumbered the handler table can never dispatch a stale index.
//
// The id is the idempotency key: a timer without one is not scheduled (the
// loader warns at load time).
func (r *renderer) timer(n *model.Node) {
	if n.ID == "" {
		return
	}
	inv := timerInvoke(n)
	h := -1
	if inv != nil {
		h = r.register(inv)
	}
	every := int(r.timerMSProp(n, "every"))
	after := int(r.timerMSProp(n, "after"))
	if every > 0 && every < TimerMinEveryMS {
		every = TimerMinEveryMS
	}
	if every <= 0 && after <= 0 {
		return // no schedule — nothing to emit (loader warns)
	}
	fmt.Fprintf(&r.sb, `<span id=%q data-qorm-timer=%q data-h="%d" hidden aria-hidden="true"`,
		attrID(r.nid(n)), attrID(r.nid(n)), h)
	if every > 0 {
		fmt.Fprintf(&r.sb, ` data-every="%d"`, every)
	} else {
		fmt.Fprintf(&r.sb, ` data-after="%d"`, after)
	}
	r.sb.WriteString(`></span>`)
}

// timerMSProp resolves a millisecond prop that may be a number literal or a
// {{...}} binding (evaluated against the current scope).
func (r *renderer) timerMSProp(n *model.Node, key string) float64 {
	raw, ok := n.Prop(key)
	if !ok {
		return 0
	}
	if s, isStr := raw.(string); isStr {
		return asFloat(runtime.EvalBinding(s, r.ctx()))
	}
	return asFloat(raw)
}

// timerInvoke reads the onTick handler: the {name,args} invoke form or the
// string shorthand ("onTick": "refresh"), mirroring parseInvoke in the loader.
func timerInvoke(n *model.Node) *model.Invoke {
	if s, ok := n.Prop("onTick"); ok {
		if name, isStr := s.(string); isStr && name != "" {
			return &model.Invoke{Name: name, Args: map[string]string{}}
		}
	}
	return parseInvokeProp(n, "onTick")
}
