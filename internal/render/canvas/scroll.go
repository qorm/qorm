package canvas

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
)

// This file is the canvas counterpart of the HTML path's scroll containers
// (render_style.go:122: `scroll`/`scrollview` get overflow:auto). A `scroll`
// node is a VIEWPORT: children taller than the box are clipped and shifted by
// a content offset, driven by wheel/trackpad input.
//
// The three pieces:
//
//   - paint clipping: a clipNode mounted as the viewport group's FIRST child
//     emits a ClipOp for the viewport box, so every following sibling (the
//     background, the offset content, a focus ring) is cut to the box and the
//     group's own Save/Restore pops the clip (software.go's clip stack).
//   - hit clipping: the group is marked Clip, and graph's HitTest refuses
//     points outside the box for every descendant (graph/node.go) — the two
//     sides of the same rule, since pixels that were cut must not be
//     clickable either.
//   - the offset: cross-frame state in Interaction.ScrollOffsets (keyed by
//     the stable model pointer), applied as the content group's Y translation
//     in PerformLayout and clamped to [0, contentHeight-viewportHeight] both
//     on input (HandleScroll) and on layout (scrollOffset — a data shrink
//     repairs a held offset).
//
// Wave-2 scope, deliberately: vertical only (DX is ignored; the HTML path's
// `horizontal` is a later wave), no touch-drag scrolling, no scrollbar
// affordance.

// isScrollType reports the viewport container spellings (the HTML names).
func isScrollType(t string) bool { return t == "scroll" || t == "scrollview" }

// clipNode is a zero-size, never-hit leaf whose Draw emits the viewport's
// clip rect in its parent's coordinate space. It exists because Group.Draw
// has no clip notion of its own: mounted first, its ClipOp covers every
// sibling drawn after it, and the parent group's Restore pops it.
type clipNode struct {
	graph.BaseNode
}

func newClipNode(w, h float64) *clipNode {
	c := &clipNode{}
	c.Init(c)
	c.NoHit = true
	c.Width, c.Height = w, h
	return c
}

func (c *clipNode) Base() *graph.BaseNode { return &c.BaseNode }

// Draw emits the clip rect. No Save/Translate of its own: the parent group
// already established the local coordinate space, and the clip must outlive
// the siblings' own Save/Restore pairs.
func (c *clipNode) Draw(ctx *graph.Context) {
	ctx.ClipRect(image.Rect(0, 0, int(c.Width), int(c.Height)))
}

// scrollContentOf returns the viewport's content group — the single
// *graph.Group child PerformLayout wraps the scrolled children in. Its
// siblings are the clip leaf and decorative rects (background, focus ring),
// never groups; keep that invariant or this lookup lies.
func scrollContentOf(vp *graph.Group) *graph.Group {
	for _, c := range vp.Children {
		if g, ok := c.(*graph.Group); ok {
			return g
		}
	}
	return nil
}

// scrollOffset returns the viewport's content offset in physical px, clamped
// to [0, contentHeight-viewportHeight]. The clamp doubles as a repair: when
// the content shrank under a held offset (items removed, window grown), the
// cross-frame state is pulled back into range here.
func scrollOffset(ln *LayoutNode, inter *Interaction) float64 {
	if inter == nil {
		return 0
	}
	off := inter.ScrollOffsets[ln.Node]
	max := float64(ln.ContentH - ln.Height)
	if max < 0 {
		max = 0
	}
	clamped := off
	if clamped < 0 {
		clamped = 0
	}
	if clamped > max {
		clamped = max
	}
	if clamped != off && inter.ScrollOffsets != nil {
		inter.ScrollOffsets[ln.Node] = clamped
	}
	return clamped
}

// scrollViewport applies dy to one viewport's offset, clamped to its scroll
// range, and returns the UNCONSUMED remainder — 0 when the gesture was fully
// absorbed. HandleScroll walks the hit's ancestor chain feeding each viewport
// what the inner ones could not take, which is the web's scroll chaining: an
// inner list scrolled to its end passes the rest of the gesture outward.
func (e *Engine) scrollViewport(vp *graph.Group, m *model.Node, dy float64) float64 {
	if dy != dy || dy > 1e308 || dy < -1e308 {
		return 0 // NaN/Inf deltas sail through both clamps and poison the
		// persisted offset (R6-C) — drop the gesture outright.
	}
	content := scrollContentOf(vp)
	if content == nil {
		return dy
	}
	max := content.Base().Height - vp.Base().Height
	if max <= 0 {
		return dy // content fits: nothing to scroll, bubble the whole gesture
	}
	off := e.Inter.ScrollOffsets[m]
	next := off + dy
	if next < 0 {
		next = 0
	}
	if next > max {
		next = max
	}
	consumed := next - off
	if consumed == 0 {
		return dy // already at the edge in this direction
	}
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]float64{}
	}
	e.Inter.ScrollOffsets[m] = next
	return dy - consumed
}
