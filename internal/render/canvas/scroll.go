package canvas

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/geom"
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
// Wheel/trackpad scrolling with momentum inertia; touch-drag scrolling with
// rubber-band overscroll and spring settle; scrollbar thumbs on each
// overflowing axis. InteractiveWidget children still claim their own press
// stream (buttons/sliders); non-interactive content under a scroll viewport
// arms ScrollDrag (engine.go).

// isScrollType reports the viewport container spellings (the HTML names).
func isScrollType(t string) bool { return t == "scroll" || t == "scrollview" }

// clipNode is a zero-size, never-hit leaf whose Draw emits the viewport's
// clip rect in its parent's coordinate space. It exists because Group.Draw
// has no clip notion of its own: mounted first, its ClipOp covers every
// sibling drawn after it, and the parent group's Restore pops it.
// Radius > 0 emits a rounded clip (overflow:hidden + borderRadius).
// EllipseRX/RY > 0 emits an elliptical clip (clip-path: circle/ellipse).
// Poly (len>=3) emits a polygonal clip (clip-path: polygon()).
type clipNode struct {
	graph.BaseNode
	Radius    float64
	EllipseRX float64
	EllipseRY float64
	Poly      []geom.Point
	EvenOdd   bool
}

func newClipNode(w, h float64) *clipNode {
	return newClipNodeR(w, h, 0)
}

func newClipNodeR(w, h, radius float64) *clipNode {
	c := &clipNode{}
	c.Init(c)
	c.NoHit = true
	c.Width, c.Height = w, h
	c.Radius = radius
	return c
}

func newClipEllipse(w, h, rx, ry float64) *clipNode {
	c := newClipNodeR(w, h, 0)
	c.EllipseRX, c.EllipseRY = rx, ry
	return c
}

func newClipPolygon(w, h float64, pts [][2]float64, evenOdd bool) *clipNode {
	c := newClipNodeR(w, h, 0)
	c.Poly = make([]geom.Point, len(pts))
	for i, p := range pts {
		c.Poly[i] = geom.Point{X: p[0], Y: p[1]}
	}
	c.EvenOdd = evenOdd
	return c
}

func (c *clipNode) Base() *graph.BaseNode { return &c.BaseNode }

// Draw emits the clip rect. No Save/Translate of its own: the parent group
// already established the local coordinate space, and the clip must outlive
// the siblings' own Save/Restore pairs.
func (c *clipNode) Draw(ctx *graph.Context) {
	if len(c.Poly) >= 3 {
		ctx.ClipPolygon(c.Poly, c.EvenOdd)
		return
	}
	r := image.Rect(0, 0, int(c.Width), int(c.Height))
	if c.EllipseRX > 0 && c.EllipseRY > 0 {
		ctx.ClipEllipse(r, c.EllipseRX, c.EllipseRY)
		return
	}
	if c.Radius > 0 {
		ctx.ClipRRect(r, c.Radius)
		return
	}
	ctx.ClipRect(r)
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
//
// Soft exceptions: an active finger drag or an overscroll spring on this
// viewport keeps the raw offset so rubber-band paint can show past the edge.
func scrollOffsetPos(ln *LayoutNode, inter *Interaction) ScrollPos {
	if inter == nil {
		return ScrollPos{}
	}
	pos := inter.ScrollOffsets[ln.Node]
	if scrollAllowsOverscroll(inter, ln.Node) {
		return pos
	}
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

// scrollAllowsOverscroll reports whether this viewport may paint past its
// hard clamp (active drag or spring settle).
func scrollAllowsOverscroll(inter *Interaction, n *model.Node) bool {
	if inter == nil || n == nil {
		return false
	}
	if inter.ScrollDrag.Active && inter.ScrollDrag.Node == n {
		return true
	}
	if mom, ok := inter.ScrollMomentum[n]; ok && mom.Spring {
		return true
	}
	return false
}

// scrollAncestor returns the innermost scroll viewport group under hit
// (or hit itself), plus its model node.
func scrollAncestor(hit graph.Node) (*graph.Group, *model.Node) {
	chain := scrollAncestorChain(hit)
	if len(chain) == 0 {
		return nil, nil
	}
	return chain[0].vp, chain[0].m
}

type scrollLink struct {
	vp *graph.Group
	m  *model.Node
}

// scrollAncestorChain returns scroll viewports from innermost to outermost.
func scrollAncestorChain(hit graph.Node) []scrollLink {
	var chain []scrollLink
	for n := hit; n != nil; {
		if g, ok := n.(*graph.Group); ok {
			if m := g.Base().Model; m != nil && isScrollType(m.Type) {
				chain = append(chain, scrollLink{vp: g, m: m})
			}
		}
		p := n.Base().Parent
		if p == nil {
			break
		}
		n = p
	}
	return chain
}

// scrollRange returns the max content offset (0 when content fits).
func scrollRange(vp *graph.Group) (maxX, maxY float64) {
	content := scrollContentOf(vp)
	if content == nil {
		return 0, 0
	}
	maxX = content.Base().Width - vp.Base().Width
	maxY = content.Base().Height - vp.Base().Height
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	return maxX, maxY
}

// scrollCanDrag reports whether the viewport has overflow on either axis.
func scrollCanDrag(vp *graph.Group) bool {
	maxX, maxY := scrollRange(vp)
	return maxX > 0 || maxY > 0
}

// rubberDelta damps a scroll delta when the offset is already past the hard
// edge (or about to cross it). dim is the viewport size on that axis.
func rubberDelta(offset, max, dim, delta float64) float64 {
	if dim < 1 {
		dim = 1
	}
	// Inside the hard range: full delta until the step would leave it.
	next := offset + delta
	if next >= 0 && next <= max {
		return delta
	}
	// Crossing or already outside: depth-based resistance.
	over := 0.0
	if next < 0 {
		if offset > 0 {
			// Consume the in-range portion fully, rubber the rest.
			inRange := -offset
			if delta >= inRange {
				return inRange + (delta-inRange)*overscrollRubber
			}
		}
		over = math.Abs(math.Min(offset, 0))
	} else if next > max {
		if offset < max {
			inRange := max - offset
			if delta <= inRange {
				return delta
			}
			return inRange + (delta-inRange)*overscrollRubber
		}
		over = offset - max
		if over < 0 {
			over = 0
		}
	}
	return delta * overscrollRubber / (1 + over/(dim*0.55))
}

// scrollViewportRubber is scrollViewport with soft overscroll (rubber-band).
// Used by touch-drag; wheel path keeps the hard clamp.
func (e *Engine) scrollViewportRubber(vp *graph.Group, m *model.Node, dx, dy float64) {
	if (dx != dx || dx > 1e308 || dx < -1e308) || (dy != dy || dy > 1e308 || dy < -1e308) {
		return
	}
	maxX, maxY := scrollRange(vp)
	if maxX <= 0 && maxY <= 0 {
		// Still allow vertical pull for pull-to-refresh when content fits.
		if dy == 0 {
			return
		}
		maxY = 0
	}
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
	}
	pos := e.Inter.ScrollOffsets[m]
	if maxX > 0 && dx != 0 {
		pos.X += rubberDelta(pos.X, maxX, vp.Base().Width, dx)
	} else if maxX <= 0 {
		pos.X = 0
	}
	if dy != 0 {
		// Always rubber on Y so a short list can still pull-to-refresh.
		pos.Y += rubberDelta(pos.Y, maxY, vp.Base().Height, dy)
	}
	e.Inter.ScrollOffsets[m] = pos
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
// independent friction). Spring is true while rubber-band overscroll is
// easing back into the hard clamp range after a drag release. Snap is true
// while easing to a scroll-snap target after coast/release (CSS scroll-snap).
type ScrollMomentum struct {
	VX, VY   float64 // velocity in physical px per ideal frame (~16.7ms)
	Active   bool    // momentum phase is in flight
	Spring   bool    // overscroll spring-back (ignores velocity, lerps to clamp)
	Snap     bool    // scroll-snap settle toward SnapTo*
	SnapToX  float64
	SnapToY  float64
	HasSnapX bool
	HasSnapY bool
	LastTime time.Time // last scroll-event timestamp for this viewport
}

// scrollDragSlop is how far a press must travel before touch-drag scroll
// activates (taps and small nudges stay free for onPress / swipe).
const scrollDragSlop = 6.0

// overscrollRubber is the base resistance factor for rubber-band drag past
// the hard clamp (further overscroll is damped by depth).
const overscrollRubber = 0.45

// pullRefreshThreshold is how far (physical px) past the top a drag must
// overscroll before release fires onRefresh / a wrapping refreshindicator.
const pullRefreshThreshold = 56.0

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
// animating until it all settles). Spring mode lerps overscrolled offsets
// back into range without free coasting past the edge.
func (e *Engine) applyScrollMomentum(now time.Time) bool {
	if e.Inter.ScrollMomentum == nil {
		return false
	}
	any := false
	for m, mom := range e.Inter.ScrollMomentum {
		if !mom.Active {
			continue
		}
		elapsed := now.Sub(mom.LastTime).Seconds()
		frames := elapsed * 60 // normalize to 60fps
		if frames < 0.12 {
			// Spring must not stall when the host draws faster than the
			// momentum clock (tests, high-refresh): advance a minimum step.
			if mom.Spring {
				frames = 0.5
			} else {
				any = true
				continue
			}
		}
		if e.Inter.ScrollOffsets == nil {
			e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
		}
		pos := e.Inter.ScrollOffsets[m]

		// Spring-back from rubber-band overscroll (after touch-drag release).
		if mom.Spring {
			maxX, maxY := 0.0, 0.0
			if g := e.findGroupByModel(m); g != nil {
				if gp, ok := g.(*graph.Group); ok {
					maxX, maxY = scrollRange(gp)
				}
			}
			const spring = 0.35 // per ideal frame toward the clamp edge
			t := 1 - math.Pow(1-spring, frames)
			if t > 1 {
				t = 1
			}
			tx, ty := pos.X, pos.Y
			if tx < 0 {
				tx = 0
			} else if tx > maxX {
				tx = maxX
			}
			if ty < 0 {
				ty = 0
			} else if ty > maxY {
				ty = maxY
			}
			pos.X += (tx - pos.X) * t
			pos.Y += (ty - pos.Y) * t
			mom.LastTime = now
			if math.Abs(pos.X-tx) < 0.5 && math.Abs(pos.Y-ty) < 0.5 {
				pos.X, pos.Y = tx, ty
				mom.Spring = false
				mom.Active = false
				mom.VX, mom.VY = 0, 0
				// After spring-back, still may need scroll-snap (rare).
				if e.tryArmScrollSnap(m, &pos, &mom) {
					any = true
				}
			} else {
				any = true
			}
			e.Inter.ScrollOffsets[m] = pos
			e.Inter.ScrollMomentum[m] = mom
			continue
		}

		// CSS scroll-snap settle: lerp toward the nearest snap target.
		if mom.Snap {
			const snap = 0.4
			t := 1 - math.Pow(1-snap, frames)
			if t > 1 {
				t = 1
			}
			if mom.HasSnapX {
				pos.X += (mom.SnapToX - pos.X) * t
			}
			if mom.HasSnapY {
				pos.Y += (mom.SnapToY - pos.Y) * t
			}
			mom.LastTime = now
			doneX := !mom.HasSnapX || math.Abs(pos.X-mom.SnapToX) < 0.5
			doneY := !mom.HasSnapY || math.Abs(pos.Y-mom.SnapToY) < 0.5
			if doneX && doneY {
				if mom.HasSnapX {
					pos.X = mom.SnapToX
				}
				if mom.HasSnapY {
					pos.Y = mom.SnapToY
				}
				mom.Snap = false
				mom.Active = false
				mom.VX, mom.VY = 0, 0
				mom.HasSnapX, mom.HasSnapY = false, false
			} else {
				any = true
			}
			e.Inter.ScrollOffsets[m] = pos
			e.Inter.ScrollMomentum[m] = mom
			continue
		}

		// Decay velocity: friction adjusted for the elapsed time since the
		// last event (or last momentum frame). A 16ms frame → friction^1,
		// a 32ms frame → friction^2, keeping the feel frame-rate independent.
		// When a scroll event just arrived (elapsed < 2ms), skip momentum
		// this frame — the event handler already applied its own delta and
		// double-counting would overshoot the offset.
		mom.VX *= math.Pow(momentumFriction, frames)
		mom.VY *= math.Pow(momentumFriction, frames)
		mom.LastTime = now

		// Stop when velocity is imperceptible.
		if math.Abs(mom.VX) < momentumStopThreshold && math.Abs(mom.VY) < momentumStopThreshold {
			mom.VX, mom.VY = 0, 0
			// If we stopped while overscrolled (rare), snap into spring.
			if pos.X < 0 || pos.Y < 0 {
				mom.Spring = true
				mom.Active = true
				e.Inter.ScrollMomentum[m] = mom
				e.Inter.ScrollOffsets[m] = pos
				any = true
				continue
			}
			// CSS scroll-snap: ease to nearest snap point when configured.
			if e.tryArmScrollSnap(m, &pos, &mom) {
				e.Inter.ScrollOffsets[m] = pos
				e.Inter.ScrollMomentum[m] = mom
				any = true
				continue
			}
			mom.Active = false
			e.Inter.ScrollMomentum[m] = mom
			continue
		}

		// Apply velocity to the scroll offset, scaled by elapsed frames so
		// a longer frame (e.g. 32ms at 30fps) advances twice as far — the
		// velocity is in physical px per ideal frame (~16.7ms at 60fps).
		pos.X += mom.VX * frames
		pos.Y += mom.VY * frames

		// Soft floor: free coast does not overscroll; hit the edge and stop
		// that axis. (Rubber-band is drag-only; release overshoot uses Spring.)
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

// handleScrollDrag processes one pointer event against an armed or active
// ScrollDrag. Returns true when the event was consumed (caller must return).
//
// When Pending under an InteractiveWidget capture (list tile, button inside
// a scroll), a drag past slop on the scroll's dominant axis STEALS the
// stream so list rows scroll instead of only responding to taps.
func (e *Engine) handleScrollDrag(p PointerInput) bool {
	d := &e.Inter.ScrollDrag
	switch {
	case p.Type == PointerRelease || (p.Type == PointerMove && p.Buttons == 0):
		wasActive := d.Active
		if wasActive {
			e.endScrollDrag()
			e.dirty.Store(true)
			return true
		}
		// Pending only (tap): clear without consuming so the widget/generic
		// release can still fire onPress / swipe.
		e.Inter.ScrollDrag = ScrollDragState{}
		return false
	case p.Type == PointerMove && p.Buttons > 0:
		if d.Node == nil {
			e.Inter.ScrollDrag = ScrollDragState{}
			return false
		}
		if d.Pending {
			adx := math.Abs(p.X - d.StartX)
			ady := math.Abs(p.Y - d.StartY)
			if math.Hypot(adx, ady) < scrollDragSlop {
				d.LastX, d.LastY = p.X, p.Y
				return false // still a tap / widget candidate
			}
			// Axis lock: only steal when travel matches the viewport's
			// overflow axis (vertical list + vertical drag, etc.). A
			// horizontal swipe on a vertical scroller leaves the child
			// (swipeactions, slider) alone.
			maxX, maxY := 0.0, 0.0
			if g := e.findGroupByModel(d.Node); g != nil {
				if vp, ok := g.(*graph.Group); ok {
					maxX, maxY = scrollRange(vp)
				}
			}
			const dominance = 1.15
			if maxY > 0 && maxX <= 0 && ady < adx*dominance {
				e.Inter.ScrollDrag = ScrollDragState{} // horizontal → child wins
				return false
			}
			if maxX > 0 && maxY <= 0 && adx < ady*dominance {
				e.Inter.ScrollDrag = ScrollDragState{} // vertical → child wins
				return false
			}
			// Both axes scrollable, or matching axis: steal.
			d.Pending = false
			d.Active = true
			// Steal InteractiveWidget / generic capture so pressables don't
			// fire on release after a scroll gesture.
			e.Inter.Pressed = nil
			e.Inter.PressedItem = 0
			e.Inter.PressedScope = nil
			e.Inter.Swipe.Armed = false
		}
		if !d.Active {
			return false
		}
		// Finger motion → content follows → scroll offset is opposite.
		dx := -(p.X - d.LastX)
		dy := -(p.Y - d.LastY)
		d.LastX, d.LastY = p.X, p.Y
		now := time.Now()
		dt := now.Sub(d.MomLast).Seconds()
		if dt > 0 && dt < 0.5 {
			frames := dt * 60
			if frames > 0 {
				// MomV is in scroll-space (same sign as offset change).
				d.MomVX = 0.25*(dx/frames) + 0.75*d.MomVX
				d.MomVY = 0.25*(dy/frames) + 0.75*d.MomVY
			}
		}
		d.MomLast = now
		e.scrollDragApplyNested(d.Node, dx, dy)
		e.dirty.Store(true)
		return true
	default:
		// PointerPress is handled by armScrollDragFromHit; do not clear here.
		return false
	}
}

// scrollDragApplyNested routes a touch-drag delta through nested scroll
// viewports: the innermost absorbs while it has room (or is allowed to
// rubber-band); remainder bubbles to outer viewports with a hard clamp —
// matching wheel scroll chaining + iOS nested scroll feel.
func (e *Engine) scrollDragApplyNested(inner *model.Node, dx, dy float64) {
	if inner == nil || (dx == 0 && dy == 0) {
		return
	}
	g := e.findGroupByModel(inner)
	if g == nil {
		return
	}
	chain := scrollAncestorChain(g)
	if len(chain) == 0 {
		if vp, ok := g.(*graph.Group); ok {
			e.scrollViewportRubber(vp, inner, dx, dy)
			e.Inter.ScrollDrag.Node = inner
		}
		return
	}
	remainX, remainY := dx, dy
	for i, link := range chain {
		if remainX == 0 && remainY == 0 {
			break
		}
		if e.Inter.ScrollOffsets == nil {
			e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
		}
		pos := e.Inter.ScrollOffsets[link.m]
		maxX, maxY := scrollRange(link.vp)
		// How much of remainY can this viewport take before the hard edge?
		takeX, takeY := remainX, remainY
		if i > 0 {
			// Outer: hard clamp — only absorb the in-range portion.
			if takeY < 0 && pos.Y+takeY < 0 {
				takeY = -pos.Y
			}
			if takeY > 0 && pos.Y+takeY > maxY {
				takeY = maxY - pos.Y
			}
			if takeX < 0 && pos.X+takeX < 0 {
				takeX = -pos.X
			}
			if takeX > 0 && pos.X+takeX > maxX {
				takeX = maxX - pos.X
			}
			if takeX == 0 && takeY == 0 {
				continue
			}
			before := pos
			e.scrollViewport(link.vp, link.m, takeX, takeY)
			after := e.Inter.ScrollOffsets[link.m]
			remainX -= after.X - before.X
			remainY -= after.Y - before.Y
			e.Inter.ScrollDrag.Node = link.m
			continue
		}
		// Innermost: if already at the hard edge in the drag direction and an
		// outer viewport exists, bubble the delta (no rubber fight). Otherwise
		// rubber-band on this viewport (pull-to-refresh / edge bounce).
		atTop, atBot := pos.Y <= 0.5, maxY <= 0 || pos.Y >= maxY-0.5
		atLeft, atRight := pos.X <= 0.5, maxX <= 0 || pos.X >= maxX-0.5
		bubbleY := len(chain) > 1 && ((takeY > 0 && atBot) || (takeY < 0 && atTop))
		bubbleX := len(chain) > 1 && ((takeX > 0 && atRight) || (takeX < 0 && atLeft))
		applyX, applyY := takeX, takeY
		if bubbleY {
			applyY = 0
			remainY = takeY
		}
		if bubbleX {
			applyX = 0
			remainX = takeX
		}
		if applyX != 0 || applyY != 0 {
			e.scrollViewportRubber(link.vp, link.m, applyX, applyY)
			e.Inter.ScrollDrag.Node = link.m
		}
		if !bubbleY {
			remainY = 0
		}
		if !bubbleX {
			remainX = 0
		}
	}
}

// armScrollDragFromHit arms a pending touch-drag when the press is over a
// scroll viewport (including presses that land on InteractiveWidget children).
func (e *Engine) armScrollDragFromHit(hit graph.Node) {
	if _, m := scrollAncestor(hit); m != nil {
		if e.Inter.ScrollMomentum != nil {
			if mom, ok := e.Inter.ScrollMomentum[m]; ok {
				mom.Active, mom.Spring = false, false
				mom.VX, mom.VY = 0, 0
				e.Inter.ScrollMomentum[m] = mom
			}
		}
		// LastX/Y filled by the caller with the press coordinates.
		e.Inter.ScrollDrag.Node = m
		e.Inter.ScrollDrag.Pending = true
		e.Inter.ScrollDrag.Active = false
		e.Inter.ScrollDrag.MomVX, e.Inter.ScrollDrag.MomVY = 0, 0
		e.Inter.ScrollDrag.MomLast = time.Now()
		return
	}
	e.Inter.ScrollDrag = ScrollDragState{}
}

// endScrollDrag finishes a touch-drag: seeds coast velocity and/or a spring
// when rubber-banded past the edge; fires pull-to-refresh when pulled past
// the top threshold.
func (e *Engine) endScrollDrag() {
	d := e.Inter.ScrollDrag
	e.Inter.ScrollDrag = ScrollDragState{}
	if d.Node == nil {
		return
	}
	m := d.Node
	if e.Inter.ScrollOffsets == nil {
		e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{}
	}
	pos := e.Inter.ScrollOffsets[m]
	maxX, maxY := 0.0, 0.0
	var vp *graph.Group
	if g := e.findGroupByModel(m); g != nil {
		if gp, ok := g.(*graph.Group); ok {
			vp = gp
			maxX, maxY = scrollRange(gp)
		}
	}
	// Pull-to-refresh: overscrolled past the top by the threshold.
	if pos.Y < -pullRefreshThreshold {
		e.firePullRefresh(m, vp)
	}
	overX := pos.X < 0 || pos.X > maxX
	overY := pos.Y < 0 || pos.Y > maxY
	if e.Inter.ScrollMomentum == nil {
		e.Inter.ScrollMomentum = map[*model.Node]ScrollMomentum{}
	}
	mom := e.Inter.ScrollMomentum[m]
	if overX || overY {
		mom.Spring = true
		mom.Active = true
		mom.Snap = false
		mom.VX, mom.VY = 0, 0
		mom.LastTime = time.Now()
		e.Inter.ScrollMomentum[m] = mom
		return
	}
	// Coast with the drag's smoothed velocity (finger direction already
	// converted to scroll deltas in the drag path — MomV is scroll-space).
	if math.Abs(d.MomVX) >= momentumStopThreshold || math.Abs(d.MomVY) >= momentumStopThreshold {
		mom.VX, mom.VY = d.MomVX, d.MomVY
		mom.Active = true
		mom.Spring = false
		mom.Snap = false
		mom.LastTime = time.Now()
		e.Inter.ScrollMomentum[m] = mom
		return
	}
	// No coast: still may need scroll-snap settle.
	if e.tryArmScrollSnap(m, &pos, &mom) {
		e.Inter.ScrollOffsets[m] = pos
		e.Inter.ScrollMomentum[m] = mom
	}
}

// --- CSS scroll-snap (scrollSnapType / scrollSnapAlign) ---

// scrollSnapConfig reads the viewport's scroll-snap policy from style or props.
// Returns axis ("x","y","both","") and whether snapping is mandatory.
func scrollSnapConfig(m *model.Node) (axis string, mandatory bool) {
	if m == nil {
		return "", false
	}
	raw := ""
	if m.Style != nil {
		if s, ok := m.Style["scrollSnapType"].(string); ok {
			raw = s
		}
	}
	if raw == "" && m.Props != nil {
		if s, ok := m.Props["scrollSnapType"].(string); ok {
			raw = s
		}
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "none" {
		return "", false
	}
	// Tokenize so "proximity" does not match axis "y" via substring search.
	hasX, hasY, hasBoth := false, false, false
	sawMode := false
	mandatory = true // CSS default when only axis is given is often mandatory in apps
	for _, tok := range strings.Fields(raw) {
		switch tok {
		case "x":
			hasX = true
		case "y":
			hasY = true
		case "both", "block", "inline":
			// block≈y, inline≈x in horizontal-tb; treat both as both for simplicity.
			if tok == "both" {
				hasBoth = true
			} else if tok == "block" {
				hasY = true
			} else {
				hasX = true
			}
		case "mandatory":
			mandatory = true
			sawMode = true
		case "proximity":
			mandatory = false
			sawMode = true
		}
	}
	if !sawMode && strings.Contains(raw, "proximity") {
		mandatory = false
	}
	switch {
	case hasBoth || (hasX && hasY):
		axis = "both"
	case hasX:
		axis = "x"
	case hasY:
		axis = "y"
	default:
		// Axis omitted ("mandatory" alone) → vertical lists.
		axis = "y"
	}
	return axis, mandatory
}

// scrollSnapAlignOf returns a child's align (start/center/end/none).
// Empty means "start" when the parent snaps (pragmatic JSON default).
func scrollSnapAlignOf(n *model.Node) string {
	if n == nil {
		return "start"
	}
	raw := ""
	if n.Style != nil {
		if s, ok := n.Style["scrollSnapAlign"].(string); ok {
			raw = s
		}
	}
	if raw == "" && n.Props != nil {
		if s, ok := n.Props["scrollSnapAlign"].(string); ok {
			raw = s
		}
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "none":
		return "none"
	case "center", "end", "start":
		return raw
	default:
		return "start"
	}
}

// collectSnapOffsets returns sorted unique scroll offsets for the axis
// ('x' or 'y') from content children of the viewport.
func collectSnapOffsets(vp *graph.Group, axis byte) []float64 {
	content := scrollContentOf(vp)
	if content == nil {
		return nil
	}
	maxX, maxY := scrollRange(vp)
	vpW, vpH := vp.Base().Width, vp.Base().Height
	var pts []float64
	for _, ch := range content.Children {
		b := ch.Base()
		if b.NoHit || b.Width <= 0 || b.Height <= 0 {
			continue
		}
		align := "start"
		if b.Model != nil {
			align = scrollSnapAlignOf(b.Model)
			if align == "none" {
				continue
			}
		}
		var off float64
		if axis == 'y' {
			switch align {
			case "center":
				off = b.Y + b.Height/2 - vpH/2
			case "end":
				off = b.Y + b.Height - vpH
			default:
				off = b.Y
			}
			if off < 0 {
				off = 0
			}
			if off > maxY {
				off = maxY
			}
		} else {
			switch align {
			case "center":
				off = b.X + b.Width/2 - vpW/2
			case "end":
				off = b.X + b.Width - vpW
			default:
				off = b.X
			}
			if off < 0 {
				off = 0
			}
			if off > maxX {
				off = maxX
			}
		}
		pts = append(pts, off)
	}
	if len(pts) == 0 {
		return nil
	}
	sort.Float64s(pts)
	// Dedupe nearby points.
	out := pts[:1]
	for _, p := range pts[1:] {
		if p-out[len(out)-1] > 0.5 {
			out = append(out, p)
		}
	}
	return out
}

// nearestSnap picks the closest snap offset; for proximity, returns ok=false
// when the nearest is farther than thresh.
func nearestSnap(cur float64, pts []float64, mandatory bool, thresh float64) (float64, bool) {
	if len(pts) == 0 {
		return cur, false
	}
	best, bestD := pts[0], math.Abs(pts[0]-cur)
	for _, p := range pts[1:] {
		d := math.Abs(p - cur)
		if d < bestD {
			best, bestD = p, d
		}
	}
	if !mandatory && bestD > thresh {
		return cur, false
	}
	if bestD < 0.5 {
		return best, false // already there
	}
	return best, true
}

// tryArmScrollSnap configures mom for a snap settle when the viewport
// declares scrollSnapType. Returns true when snap was armed.
func (e *Engine) tryArmScrollSnap(m *model.Node, pos *ScrollPos, mom *ScrollMomentum) bool {
	axis, mandatory := scrollSnapConfig(m)
	if axis == "" {
		return false
	}
	g := e.findGroupByModel(m)
	if g == nil {
		return false
	}
	vp, ok := g.(*graph.Group)
	if !ok {
		return false
	}
	threshX := vp.Base().Width * 0.35
	threshY := vp.Base().Height * 0.35
	armed := false
	if axis == "x" || axis == "both" {
		if pts := collectSnapOffsets(vp, 'x'); len(pts) > 0 {
			if t, ok := nearestSnap(pos.X, pts, mandatory, threshX); ok {
				mom.SnapToX, mom.HasSnapX = t, true
				armed = true
			}
		}
	}
	if axis == "y" || axis == "both" {
		if pts := collectSnapOffsets(vp, 'y'); len(pts) > 0 {
			if t, ok := nearestSnap(pos.Y, pts, mandatory, threshY); ok {
				mom.SnapToY, mom.HasSnapY = t, true
				armed = true
			}
		}
	}
	if !armed {
		return false
	}
	mom.Snap = true
	mom.Active = true
	mom.Spring = false
	mom.VX, mom.VY = 0, 0
	mom.LastTime = time.Now()
	return true
}

// firePullRefresh dispatches onRefresh on the scroll node, a wrapping
// refreshindicator, or the nearest ancestor declaring the prop.
func (e *Engine) firePullRefresh(scroll *model.Node, vp *graph.Group) {
	// Walk model via graph parents from the viewport group.
	var chain []*model.Node
	if scroll != nil {
		chain = append(chain, scroll)
	}
	if vp != nil {
		for p := vp.Base().Parent; p != nil; p = p.Base().Parent {
			if m := p.Base().Model; m != nil {
				chain = append(chain, m)
			}
		}
	}
	for _, m := range chain {
		if m.Type == "refreshindicator" {
			if inv := parseInvokeFromNode(m, "onRefresh"); inv != nil {
				e.dispatch(inv, nil)
				return
			}
			if m.OnPress != nil {
				e.dispatch(m.OnPress, nil)
				return
			}
		}
		if inv := parseInvokeFromNode(m, "onRefresh"); inv != nil {
			e.dispatch(inv, nil)
			return
		}
	}
}

// parseInvokeFromNode reads a {name,args} or string action prop.
func parseInvokeFromNode(n *model.Node, key string) *model.Invoke {
	if n == nil {
		return nil
	}
	raw, ok := n.Prop(key)
	if !ok || raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case string:
		if t == "" {
			return nil
		}
		return &model.Invoke{Name: t}
	case map[string]any:
		return propInvoke(t)
	default:
		return nil
	}
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
