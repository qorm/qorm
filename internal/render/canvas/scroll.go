package canvas

import (
	"image"
	"image/color"

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
//     the stable model pointer), applied as the content group's X/Y
//     translation in PerformLayout and clamped on each axis both on input
//     (HandleScroll) and on layout (scrollOffsetPos — a data shrink repairs a
//     held offset).
//
// Wave-2 scope, deliberately: wheel/trackpad only (no touch-drag scrolling),
// no scrollbar affordance (a later wave).

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

// ScrollPos is a scroll viewport's content offset in physical px, both axes.
type ScrollPos struct {
	X, Y float64
}

// addScrollbars paints a track-free thumb on each axis that overflows: a
// translucent rect at the viewport's right (vertical) / bottom (horizontal)
// edge, sized to the visible fraction and positioned by the offset. NoHit so
// it never intercepts pointer input. Painted after the content so it sits on
// top, inside the viewport clip.
func addScrollbars(ln *LayoutNode, group *graph.Group, pos ScrollPos, scale int) {
	if scale < 1 {
		scale = 1
	}
	thumb := color.RGBA{120, 120, 128, 150}
	barW, barH := 6*scale, 6*scale
	// Vertical: content taller than the box.
	if ln.ContentH > ln.Height {
		thumbH := ln.Height * ln.Height / ln.ContentH
		if thumbH < 8*scale {
			thumbH = 8 * scale
		}
		if thumbH > ln.Height {
			thumbH = ln.Height
		}
		thumbY := 0
		if max := ln.Height - thumbH; max > 0 {
			thumbY = int(float64(pos.Y) / float64(ln.ContentH-ln.Height) * float64(max))
		}
		bar := graph.NewRect()
		bar.NoHit = true
		bar.X = float64(ln.Width - barW)
		bar.Y = float64(thumbY)
		bar.Width = float64(barW)
		bar.Height = float64(thumbH)
		bar.Fill = thumb
		group.AddChild(bar)
	}
	// Horizontal: content wider than the box.
	if ln.ContentW > ln.Width {
		thumbW := ln.Width * ln.Width / ln.ContentW
		if thumbW < 8*scale {
			thumbW = 8 * scale
		}
		if thumbW > ln.Width {
			thumbW = ln.Width
		}
		thumbX := 0
		if max := ln.Width - thumbW; max > 0 {
			thumbX = int(float64(pos.X) / float64(ln.ContentW-ln.Width) * float64(max))
		}
		bar := graph.NewRect()
		bar.NoHit = true
		bar.X = float64(thumbX)
		bar.Y = float64(ln.Height - barH)
		bar.Width = float64(thumbW)
		bar.Height = float64(barH)
		bar.Fill = thumb
		group.AddChild(bar)
	}
}

// scrollOffsetPos returns the viewport's content offset, clamped on each axis
// to [0, contentSize-viewportSize]. The clamp doubles as a repair: when the
// content shrank under a held offset (items removed, window grown), the
// cross-frame state is pulled back into range here.
func scrollOffsetPos(ln *LayoutNode, inter *Interaction) ScrollPos {
	if inter == nil {
		return ScrollPos{}
	}
	pos := inter.ScrollOffsets[ln.Node]
	maxX := float64(ln.ContentW - ln.Width)
	maxY := float64(ln.ContentH - ln.Height)
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	clamped := pos
	if clamped.X < 0 {
		clamped.X = 0
	}
	if clamped.X > maxX {
		clamped.X = maxX
	}
	if clamped.Y < 0 {
		clamped.Y = 0
	}
	if clamped.Y > maxY {
		clamped.Y = maxY
	}
	if clamped != pos && inter.ScrollOffsets != nil {
		inter.ScrollOffsets[ln.Node] = clamped
	}
	return clamped
}

// scrollViewport applies dx/dy to one viewport's offsets, clamped to its
// scroll range on each axis, and returns the UNCONSUMED remainder per axis —
// (0,0) when the gesture was fully absorbed. HandleScroll walks the hit's
// ancestor chain feeding each viewport what the inner ones could not take,
// which is the web's scroll chaining: an inner list scrolled to its end passes
// the rest of the gesture outward. A viewport whose content fits on an axis
// consumes nothing on it (the whole delta bubbles out).
func (e *Engine) scrollViewport(vp *graph.Group, m *model.Node, dx, dy float64) (float64, float64) {
	if (dx != dx || dx > 1e308 || dx < -1e308) || (dy != dy || dy > 1e308 || dy < -1e308) {
		return 0, 0 // NaN/Inf deltas sail through both clamps and poison the
		// persisted offset (R6-C) — drop the gesture outright.
	}
	content := scrollContentOf(vp)
	if content == nil {
		return dx, dy
	}
	maxX := content.Base().Width - vp.Base().Width
	maxY := content.Base().Height - vp.Base().Height
	if maxX <= 0 && maxY <= 0 {
		return dx, dy // content fits both ways: bubble the whole gesture
	}
	pos := e.Inter.ScrollOffsets[m]
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
	}
	// X axis.
	var consumedX float64
	if maxX > 0 && dx != 0 {
		next := pos.X + dx
		if next < 0 {
			next = 0
		}
		if next > maxX {
			next = maxX
		}
		consumedX = next - pos.X
		pos.X = next
	}
	// Y axis.
	var consumedY float64
	if maxY > 0 && dy != 0 {
		next := pos.Y + dy
		if next < 0 {
			next = 0
		}
		if next > maxY {
			next = maxY
		}
		consumedY = next - pos.Y
		pos.Y = next
	}
	e.Inter.ScrollOffsets[m] = pos
	return dx - consumedX, dy - consumedY
}
