package canvas

import (
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
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
		hit = hit.Base().Parent
	}
	return nil
}

// canPress reports whether m warrants pressed visual feedback.
func canPress(m *model.Node) bool {
	if m == nil {
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

// VisualTarget walks up from hit and returns the first model node that
// warrants pressed feedback (has OnPress, is a button, or focusable:true).
func VisualTarget(hit graph.Node) *model.Node {
	for hit != nil {
		if m := hit.Base().Model; m != nil && canPress(m) {
			return m
		}
		hit = hit.Base().Parent
	}
	return nil
}
