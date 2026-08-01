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
	// FocusVisible is true when focus was established by the keyboard;
	// the focus ring is only drawn in that case (:focus-visible semantics).
	FocusVisible bool
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

// canPress reports whether m warrants pressed visual feedback.
func canPress(m *model.Node, rt *runtime.Runtime) bool {
	if m == nil || nodeDisabled(m, rt) {
		return false
	}
	if m.OnPress != nil || m.Type == "button" {
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
// warrants pressed feedback (has OnPress, is a button, or focusable:true).
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
