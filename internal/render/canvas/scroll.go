package canvas

import (
	"image"
	"image/color"
	"math"
	"time"

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
// Wheel/trackpad scrolling with momentum inertia; scrollbar thumbs on each
// overflowing axis. Touch-drag scrolling on the viewport itself is not yet
// implemented (the pointer stream is claimed by the content's interactive
// children — the HTML path's overflow-scroll touch model).

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

// ensureFocusVisible scrolls every scroll viewport on the focused node's
// ancestor chain until the node's box is inside the viewport: keyboard focus
// that lands on a clipped node (its ring invisible until scrolled in) brings
// it into view, like the browser's focus scrolling. It adjusts each viewport's
// offset by the node's overshoot relative to the viewport box, clamped to the
// scroll range, and marks the engine dirty for the re-layout.
//
// The ancestors are walked innermost-first, and scrolling an inner viewport
// SHIFTS the node's global position (its content moves by -offset), so each
// outer viewport must compute its overshoot against the node's position AFTER
// the inner scrolls — shiftX/shiftY accumulate those deltas, or the outer
// viewport would over-scroll by the whole inner adjustment and push the node
// back out of view on the other side.
func (e *Engine) ensureFocusVisible(m *model.Node) {
	g := e.findGroupByModelIndex(m, e.Inter.FocusedItem)
	if g == nil {
		return
	}
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
	}
	// Collect the scroll ancestors innermost-first.
	var vps []*graph.Group
	for p := g.Base().Parent; p != nil; p = p.Base().Parent {
		if mm := p.Base().Model; mm != nil && p.Clip && isScrollType(mm.Type) {
			vps = append(vps, p)
		}
	}
	nb := g.GetBBox()
	shiftX, shiftY := 0.0, 0.0
	for _, vp := range vps {
		mm := vp.Base().Model
		vb := vp.GetBBox()
		old := e.Inter.ScrollOffsets[mm]
		pos := old
		if nb.MinY+shiftY < vb.MinY {
			pos.Y += nb.MinY + shiftY - vb.MinY // node above → scroll up
		} else if nb.MaxY+shiftY > vb.MaxY {
			pos.Y += nb.MaxY + shiftY - vb.MaxY // node below → scroll down
		}
		if nb.MinX+shiftX < vb.MinX {
			pos.X += nb.MinX + shiftX - vb.MinX
		} else if nb.MaxX+shiftX > vb.MaxX {
			pos.X += nb.MaxX + shiftX - vb.MaxX
		}
		if content := scrollContentOf(vp); content != nil {
			if pos.X < 0 {
				pos.X = 0
			}
			if pos.Y < 0 {
				pos.Y = 0
			}
			if pos.X > content.Base().Width-vp.Base().Width {
				pos.X = content.Base().Width - vp.Base().Width
			}
			if pos.Y > content.Base().Height-vp.Base().Height {
				pos.Y = content.Base().Height - vp.Base().Height
			}
		}
		e.Inter.ScrollOffsets[mm] = pos
		e.dirty.Store(true)
		// The offset delta shifts the node the opposite way in global space,
		// so outer viewports compute against the post-inner-scroll position.
		shiftX -= pos.X - old.X
		shiftY -= pos.Y - old.Y
	}
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

	// Track momentum velocity: smooth the instantaneous velocity (consumed
	// delta per event) into a per-viewport moving average so the deceleration
	// phase starts from a reasonable initial speed.
	if consumedX != 0 || consumedY != 0 {
		if e.Inter.ScrollMomentum == nil {
			e.Inter.ScrollMomentum = map[*model.Node]ScrollMomentum{}
		}
		mom := e.Inter.ScrollMomentum[m]
		// Exponential moving average: 15% new delta, 85% old — the low weight
		// keeps a single discrete wheel event from producing a large velocity
		// overshoot, while continuous trackpad input still accumulates smooth speed.
		const alpha = 0.15
		mom.VX = alpha*consumedX + (1-alpha)*mom.VX
		mom.VY = alpha*consumedY + (1-alpha)*mom.VY
		mom.Active = true
		mom.LastTime = time.Now()
		e.Inter.ScrollMomentum[m] = mom
	}

	return dx - consumedX, dy - consumedY
}

// ScrollMomentum is the per-viewport scroll inertia state: velocity in physical
// px per ideal frame (~16.7ms at 60fps) and the timestamp of the last scroll
// event (so the deceleration phase knows the elapsed time for frame-rate
// independent friction).
type ScrollMomentum struct {
	VX, VY   float64   // velocity in physical px per ideal frame (~16.7ms)
	Active   bool      // momentum phase is in flight
	LastTime time.Time // last scroll-event timestamp for this viewport
}

// momentumFriction is the per-frame velocity multiplier — 0.88 at ~16.7ms/frame
// decays to ~5% after ~40 frames (~667ms), which feels like a natural trackpad
// coast without overshooting a discrete wheel event.
const momentumFriction = 0.88

// momentumStopThreshold is the velocity magnitude below which momentum stops
// (physical px per frame) — below this the deceleration is imperceptible.
const momentumStopThreshold = 0.3

// applyScrollMomentum advances every active viewport's momentum by one frame:
// apply the velocity to the offset (clamped), then decay the velocity. Returns
// whether any viewport still has active momentum (the engine must keep
// animating until it all settles).
func (e *Engine) applyScrollMomentum(now time.Time) bool {
	if e.Inter.ScrollMomentum == nil {
		return false
	}
	any := false
	for m, mom := range e.Inter.ScrollMomentum {
		if !mom.Active {
			continue
		}
		// Decay velocity: friction adjusted for the elapsed time since the
		// last event (or last momentum frame). A 16ms frame → friction^1,
		// a 32ms frame → friction^2, keeping the feel frame-rate independent.
		// When a scroll event just arrived (elapsed < 2ms), skip momentum
		// this frame — the event handler already applied its own delta and
		// double-counting would overshoot the offset.
		elapsed := now.Sub(mom.LastTime).Seconds()
		frames := elapsed * 60 // normalize to 60fps
		if frames < 0.12 {
			continue
		}
		mom.VX *= math.Pow(momentumFriction, frames)
		mom.VY *= math.Pow(momentumFriction, frames)
		mom.LastTime = now

		// Stop when velocity is imperceptible.
		if math.Abs(mom.VX) < momentumStopThreshold && math.Abs(mom.VY) < momentumStopThreshold {
			mom.Active = false
			mom.VX, mom.VY = 0, 0
			e.Inter.ScrollMomentum[m] = mom
			continue
		}

		// Apply velocity to the scroll offset, scaled by elapsed frames so
		// a longer frame (e.g. 32ms at 30fps) advances twice as far — the
		// velocity is in physical px per ideal frame (~16.7ms at 60fps).
		if e.Inter.ScrollOffsets == nil {
			e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
		}
		pos := e.Inter.ScrollOffsets[m]
		pos.X += mom.VX * frames
		pos.Y += mom.VY * frames

		// Clamp — the content size is only known at layout time, so the full
		// clamp (scrollOffsetPos) runs in performLayout. A soft floor of 0
		// here keeps the offset from drifting negative when the content has not
		// yet been measured this frame; the layout-time clamp repairs the rest.
		if pos.X < 0 {
			pos.X = 0
			mom.VX = 0
		}
		if pos.Y < 0 {
			pos.Y = 0
			mom.VY = 0
		}
		e.Inter.ScrollOffsets[m] = pos
		e.Inter.ScrollMomentum[m] = mom
		any = true
	}
	return any
}

// hasScrollMomentum reports whether any viewport still has active scroll
// momentum — the engine's animating gate keeps ticking until this settles.
func (e *Engine) hasScrollMomentum() bool {
	if e.Inter.ScrollMomentum == nil {
		return false
	}
	for _, mom := range e.Inter.ScrollMomentum {
		if mom.Active {
			return true
		}
	}
	return false
}

// boardMomentumFriction is the per-frame velocity multiplier for board pan
// coast — 0.92 gives a longer coast than scroll (≈50 frames / 833ms to 5%)
// because panning an infinite canvas should feel floaty.
const boardMomentumFriction = 0.92

// applyBoardMomentum advances the board's pan momentum by one frame: apply the
// velocity to PanX/PanY, then decay. Returns whether the coast is still active.
func (e *Engine) applyBoardMomentum(now time.Time) bool {
	b := &e.Inter.Board
	if !b.PanMomActive {
		return false
	}
	elapsed := now.Sub(b.PanMomLast).Seconds()
	frames := elapsed * 60 // normalize to 60fps
	if frames < 0.12 {
		return true // still active but not yet time to advance
	}
	b.PanMomVX *= math.Pow(boardMomentumFriction, frames)
	b.PanMomVY *= math.Pow(boardMomentumFriction, frames)
	b.PanMomLast = now

	// Stop when velocity is imperceptible.
	if math.Abs(b.PanMomVX) < momentumStopThreshold && math.Abs(b.PanMomVY) < momentumStopThreshold {
		b.PanMomActive = false
		b.PanMomVX, b.PanMomVY = 0, 0
		return false
	}

	b.PanX += b.PanMomVX * frames
	b.PanY += b.PanMomVY * frames
	return true
}

// hasBoardMomentum reports whether the board pan coast is still in flight.
func (e *Engine) hasBoardMomentum() bool {
	return e.Inter.Board.PanMomActive
}
