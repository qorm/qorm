package canvas

import (
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// Interaction carries cross-frame interaction state for the canvas backend.
//
// The scene graph is rebuilt on every Layout call, so pressed/hovered/focused
// identities cannot live on graph nodes. They live here instead, keyed by
// *model.Node pointers, which are stable between frames (the model tree is
// only rebuilt on reload/OTA — the event loop resets Interaction when it
// detects a scene-root change). PerformLayout re-stamps the flags onto the
// freshly built graph nodes each frame.
type Interaction struct {
	Pressed *model.Node
	Hovered *model.Node
	Focused *model.Node
	// PressedItem/HoveredItem/FocusedItem disambiguate WHICH repeat instance
	// the companion identity names: a list item's model node is the shared
	// template pointer, so the pointer alone would light every instance at
	// once. 0 means "outside a list" and "the first instance" alike —
	// unambiguous because the companion is only consulted together with an
	// identity that names a template-subtree node, and those exist only
	// inside instances. Nested repeats collapse to the innermost index
	// (list.go documents this).
	PressedItem int
	HoveredItem int
	FocusedItem int
	// FocusVisible is true when focus was established by the keyboard;
	// the focus ring is only drawn in that case (:focus-visible semantics).
	FocusVisible bool
	// Entrance tracks per-node entrance animation clocks (the `animation`
	// prop), keyed by (node, list index). Reset with the rest of Interaction
	// on scene switch — exactly when entrances replay (entrance.go).
	Entrance map[entranceKey]*entranceState
	// Input is the live edit session of the focused input node, nil when no
	// input is being edited. Same cross-frame home as the identities above:
	// the buffer and cursor survive the per-frame graph rebuild here
	// (input.go).
	Input *InputState
	// ScrollOffsets holds each scroll viewport's content offset (physical
	// px, vertical), keyed by the stable model pointer — the same
	// cross-frame home as the identities above: the graph is rebuilt every
	// frame, so offsets survive here. Lazily allocated by HandleScroll;
	// reset with the rest of Interaction on a scene switch.
	ScrollOffsets map[*model.Node]float64
}

// interForInstance scopes the interaction identities to one repeat instance:
// Pressed/Hovered/Focused name a TEMPLATE node every instance shares, so an
// identity only applies to the instance whose index was captured at hit time.
// Outside a repeat (nil scope) the identities pass through untouched. Measure
// uses this to keep the theme's interactive overlay (pressed/hovered
// background) on the one instance that earned it.
func interForInstance(inter *Interaction, sc *listScope) *Interaction {
	if inter == nil || sc == nil {
		return inter
	}
	cp := *inter
	if cp.PressedItem != sc.index {
		cp.Pressed = nil
	}
	if cp.HoveredItem != sc.index {
		cp.Hovered = nil
	}
	if cp.FocusedItem != sc.index {
		cp.Focused = nil
	}
	return &cp
}

// ModelOf returns the model node backing the hit node, walking up to the
// first ancestor whose group carries a Model back-reference.
func ModelOf(hit graph.Node) *model.Node {
	for hit != nil {
		if m := hit.Base().Model; m != nil {
			return m
		}
		// Parent is a typed *Group: at the root it is nil, and a typed nil in
		// the graph.Node interface is non-nil — assigning it back to hit would
		// loop once more and dereference nil. Guard like the hover walks do.
		p := hit.Base().Parent
		if p == nil {
			return nil
		}
		hit = p
	}
	return nil
}

// canPress reports whether m warrants pressed visual feedback. Inputs are
// included for the focus side of the gesture, not the feedback: a pointer
// press is also how a text field gains focus (VisualTarget picks the focus
// target), and the input theme styles define no pressed overlay, so the
// Pressed flag itself paints nothing on them.
func canPress(m *model.Node, rt *runtime.Runtime) bool {
	if m == nil || nodeDisabled(m, rt) {
		return false
	}
	if m.OnPress != nil || m.Type == "button" || m.Type == "input" {
		return true
	}
	if v, ok := m.Prop("focusable"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// nodeDisabled reads the `disabled` style key, mirroring the web renderer's
// styleDisabled (same truthiness): it is a state, not just a look — a
// disabled node is transparent to pointer activation and its handlers never
// dispatch. A binding string ("{{state.x}}") is evaluated against rt, matching
// the web path where style bindings resolve before styleDisabled reads them;
// a nil rt leaves bindings unevaluated (static keys still apply).
func nodeDisabled(m *model.Node, rt *runtime.Runtime) bool {
	if m == nil {
		return false
	}
	switch t := evalStyleProp(m.Style["disabled"], rt).(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "true" || t == "1"
	}
	return false
}

// VisualTarget walks up from hit and returns the first model node that
// warrants pressed feedback (has OnPress, is a button or input, or
// focusable:true).
func VisualTarget(hit graph.Node, rt *runtime.Runtime) *model.Node {
	for hit != nil {
		if m := hit.Base().Model; m != nil && canPress(m, rt) {
			return m
		}
		// Same typed-nil guard as ModelOf: the root's Parent is a nil *Group,
		// which is non-nil once stored in the graph.Node interface.
		p := hit.Base().Parent
		if p == nil {
			return nil
		}
		hit = p
	}
	return nil
}

// interactiveWidgetAt walks up from hit for the nearest model node whose
// type is a registered InteractiveWidget (the widget that claims its own
// pointer stream). Same typed-nil guard as the other parent walks.
func interactiveWidgetAt(hit graph.Node) (InteractiveWidget, *model.Node) {
	for hit != nil {
		if m := hit.Base().Model; m != nil {
			if w, ok := LookupWidget(m.Type); ok {
				if iw, yes := w.(InteractiveWidget); yes {
					return iw, m
				}
			}
		}
		p := hit.Base().Parent
		if p == nil {
			return nil, nil
		}
		hit = p
	}
	return nil, nil
}
