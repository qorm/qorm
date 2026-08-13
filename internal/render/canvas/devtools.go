package canvas

import (
	"image"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
)

// appendInspectOverlay records the DevTool selection last, above application
// content. GetBBox is already in screen space, so the overlay is independent
// of the selected node's local transform and cannot change layout/hit testing.
func (e *Engine) appendInspectOverlay() {
	id := strings.TrimSpace(e.inspectNodeID)
	if id == "" || e.graphRoot == nil {
		return
	}
	var selected graph.Node
	var walk func(graph.Node) bool
	walk = func(n graph.Node) bool {
		if n == nil {
			return false
		}
		if m := n.Base().Model; m != nil && m.ID == id && !n.Base().Overlay {
			selected = n
			return true
		}
		for _, child := range n.Base().Children {
			if walk(child) {
				return true
			}
		}
		return false
	}
	if !walk(e.graphRoot) || selected == nil {
		return
	}
	b := selected.GetBBox()
	r := image.Rect(int(b.MinX), int(b.MinY), int(b.MaxX), int(b.MaxY))
	if r.Empty() {
		return
	}
	e.ops.Add(op.RRectOp{
		Rect:         r,
		Fill:         color.RGBA{10, 132, 255, 24},
		Stroke:       color.RGBA{10, 132, 255, 255},
		StrokeWidth:  2,
		Outline:      color.RGBA{255, 255, 255, 210},
		OutlineWidth: 1,
	})
}

// notifyHumanPresence mirrors the browser's privacy-safe /presence payload.
// It runs after each pointer/key event and only emits when the retained label
// changed, keeping mouse motion and key-up from flooding the collaboration
// channel. A secure input reports the field label plus "(hidden)", never its
// buffer contents.
func (e *Engine) notifyHumanPresence() {
	if e.HumanPresence == nil {
		return
	}
	el := e.humanPresenceElement()
	if el == e.lastPresence {
		return
	}
	e.lastPresence = el
	e.HumanPresence(el)
}

func (e *Engine) humanPresenceElement() string {
	n := e.Inter.Focused
	if n == nil {
		return ""
	}
	label := presenceNodeLabel(n)
	if s := e.Inter.Input; s != nil && s.Node == n && len(s.Runes) > 0 {
		if secureInput(n) {
			return label + " = (hidden)"
		}
		return label + " = " + string(s.Runes)
	}
	return label
}

func presenceNodeLabel(n *model.Node) string {
	if n == nil {
		return ""
	}
	for _, candidate := range []string{
		propText(n, "ariaLabel"), n.Label, propText(n, "label"),
		n.Placeholder, propText(n, "placeholder"), n.Text,
	} {
		if s := strings.TrimSpace(candidate); s != "" {
			return s
		}
	}
	if n.ID != "" {
		return "#" + n.ID
	}
	if n.Type != "" {
		return n.Type
	}
	return "canvas"
}

func propText(n *model.Node, key string) string {
	v, ok := n.Prop(key)
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
