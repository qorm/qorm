package graph

import (
	"testing"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

// drawForTransforms runs a Draw pass so every node's GlobalTransform is
// computed (HitTest works in global space via the inverse transforms).
func drawForTransforms(n Node) {
	ops := &op.Ops{}
	n.Draw(NewContext(ops))
}

// A clipped container (a scroll viewport) must not resolve hits on the
// children its clip cuts away: those pixels are never painted, so they must
// not be clickable either (canvas-support report P1-11).
func TestHitTestRespectsClip(t *testing.T) {
	root := NewGroup()
	root.Width, root.Height = 200, 100
	root.Clip = true

	inside := NewRect() // fully inside the box
	inside.X, inside.Y = 0, 0
	inside.Width, inside.Height = 50, 50
	root.AddChild(inside)

	spanning := NewRect() // half in, half out: y 75..125 vs the 0..100 box
	spanning.X, spanning.Y = 60, 75
	spanning.Width, spanning.Height = 50, 50
	root.AddChild(spanning)

	outside := NewRect() // fully below the box
	outside.X, outside.Y = 0, 150
	outside.Width, outside.Height = 50, 50
	root.AddChild(outside)

	drawForTransforms(root)

	if hit := root.HitTest(geom.Point{X: 25, Y: 175}); hit != nil {
		t.Errorf("fully clipped child hit = %v, want nil", hit)
	}
	if hit := root.HitTest(geom.Point{X: 85, Y: 110}); hit != nil {
		t.Errorf("clipped half of a spanning child hit = %v, want nil", hit)
	}
	if hit := root.HitTest(geom.Point{X: 85, Y: 90}); hit != spanning {
		t.Errorf("visible half of a spanning child hit = %v, want the child", hit)
	}
	if hit := root.HitTest(geom.Point{X: 25, Y: 25}); hit != inside {
		t.Errorf("inside child hit = %v, want the child", hit)
	}
	if hit := root.HitTest(geom.Point{X: 150, Y: 50}); hit != root {
		t.Errorf("empty point inside the box hit = %v, want the viewport itself", hit)
	}

	// Without the flag the very same geometry hits the out-of-box children
	// (the historical, pre-clip behaviour containers still get).
	root.Clip = false
	if hit := root.HitTest(geom.Point{X: 25, Y: 175}); hit != outside {
		t.Errorf("unclipped hit = %v, want the out-of-box child", hit)
	}
}

// The clip must reach DESCENDANTS at any depth: a grandchild outside the
// viewport box is cut even though its own parent is not the clipping node.
func TestHitTestClipReachesGrandchildren(t *testing.T) {
	root := NewGroup()
	root.Width, root.Height = 200, 100
	root.Clip = true

	inner := NewGroup()
	inner.X, inner.Y = 0, 150 // entirely below the box
	inner.Width, inner.Height = 50, 50
	root.AddChild(inner)

	leaf := NewRect()
	leaf.Width, leaf.Height = 50, 50
	inner.AddChild(leaf)

	drawForTransforms(root)

	if hit := root.HitTest(geom.Point{X: 25, Y: 175}); hit != nil {
		t.Errorf("clipped grandchild hit = %v, want nil", hit)
	}

	// A scrolled-down transform puts the grandchild back inside the box.
	inner.Y = 50
	drawForTransforms(root)
	if hit := root.HitTest(geom.Point{X: 25, Y: 75}); hit != leaf {
		t.Errorf("grandchild inside the box hit = %v, want the leaf", hit)
	}
}
