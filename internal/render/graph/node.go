package graph

import (
	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
)

// Node is the core interface for all Scene Graph elements
type Node interface {
	Base() *BaseNode
	Draw(ctx *Context)
	HitTest(p geom.Point) Node
	GetBBox() geom.BBox
}

// BaseNode contains all standard transform and hierarchy data.
// By embedding this, every Shape/Group gets transform capabilities.
type BaseNode struct {
	ID string

	// Local Transform Properties
	X, Y     float64
	ScaleX   float64
	ScaleY   float64
	Rotation float64
	Opacity  float64
	Width    float64
	Height   float64

	// Global Transform Matrix (Computed during rendering)
	GlobalTransform geom.Matrix

	// Hierarchy
	Parent   *Group
	Children []Node

	// Ref to the actual implementing Node (for virtual dispatch)
	Self Node

	// Custom interaction handler
	OnPress      *model.Invoke
	OnCollide    *model.Invoke
	OnChange     *model.Invoke
	OnKeyDown    *model.Invoke
	OnKeyUp      *model.Invoke
	OnHoverIn    *model.Invoke
	OnHoverOut   *model.Invoke
	OnTouchStart *model.Invoke
	OnTouchMove  *model.Invoke
	OnTouchEnd   *model.Invoke

	// Interactive state (set by the event loop, read by the renderer)
	Pressed bool
	Hovered bool
	Focused bool

	// NoHit marks purely decorative nodes (e.g. the focus ring) that must
	// never participate in hit testing.
	NoHit bool

	// Clip marks a container whose children are clipped to its own box when
	// rendered (scroll viewports). HitTest honours it: a point outside the
	// box can never resolve to a clipped descendant, matching the pixels the
	// renderer actually cuts.
	Clip bool

	// Model is a back-reference to the model node this graph node was built
	// from (canvas backend). Nil for leaf shapes. Used to map hit results
	// back to a stable cross-frame identity.
	Model *model.Node
}

// Init initialize the base node defaults
func (b *BaseNode) Init(self Node) {
	b.ScaleX = 1
	b.ScaleY = 1
	b.Opacity = 1
	b.Self = self
}

// AddChild adds a child node
func (b *BaseNode) AddChild(child Node) {
	b.Children = append(b.Children, child)
	child.Base().Parent = b.Self.(*Group) // Assumes b.Self is a Group
}

// GetBBox calculates the absolute bounding box in global space
func (b *BaseNode) GetBBox() geom.BBox {
	localBBox := geom.NewBBox(0, 0, b.Width, b.Height)
	return b.GlobalTransform.TransformBBox(localBBox)
}

// UpdateGlobalTransform recalculates the absolute matrix based on parent
func (b *BaseNode) UpdateGlobalTransform() {
	m := geom.Identity().Translate(b.X, b.Y).Rotate(b.Rotation).Scale(b.ScaleX, b.ScaleY)
	if b.Parent != nil {
		b.GlobalTransform = b.Parent.Base().GlobalTransform.Multiply(m)
	} else {
		b.GlobalTransform = m
	}
}

// HitTest Base implementation checks children recursively (reverse order for z-index)
func (b *BaseNode) HitTest(p geom.Point) Node {
	if b.NoHit {
		return nil
	}
	// Clipping ancestors (scroll viewports) cut off everything outside their
	// own box: those pixels are never painted, so they must not be hittable
	// either. The check walks the PARENT chain — the clip is a property of
	// the container, not the child — which keeps this interface's signature
	// unchanged for every shape.
	for a := b.Parent; a != nil; a = a.Parent {
		if !a.Clip {
			continue
		}
		ainv, ok := a.GlobalTransform.Invert()
		if !ok {
			return nil
		}
		lp := ainv.TransformPoint(p)
		if lp.X < 0 || lp.Y < 0 || lp.X > a.Width || lp.Y > a.Height {
			return nil
		}
	}
	// First convert p to local space
	inv, ok := b.GlobalTransform.Invert()
	if !ok {
		return nil
	}
	localP := inv.TransformPoint(p)

	// Check children from top to bottom (last drawn to first drawn)
	for i := len(b.Children) - 1; i >= 0; i-- {
		child := b.Children[i]
		if child.Base().NoHit {
			continue
		}
		if hit := child.HitTest(p); hit != nil {
			return hit
		}
	}

	// Check self bounding box
	if localP.X >= 0 && localP.Y >= 0 && localP.X <= b.Width && localP.Y <= b.Height {
		return b.Self
	}
	return nil
}

// Draw children
func (b *BaseNode) DrawChildren(ctx *Context) {
	for _, child := range b.Children {
		child.Draw(ctx)
	}
}
