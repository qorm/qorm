// Package draw is QORM's low-level structured-drawing layer — the public seam
// widget code (built-in library and app-defined custom components) composes
// from. It knows NOTHING about widgets: a caller records drawing structure
// (shapes, clips, transforms, text, images) which the canvas engine's
// rasterizer executes, and gets hit-testing, z-order, clip and opacity
// semantics for free.
//
// Two ways to draw, both structured (no pixel access):
//
//   - Compose retained nodes (Rect/Text/Image/Group) — the common case.
//     Fields map to CSS-like concepts (Fill, BorderRadius, ShadowBlur, …).
//   - Implement the Node interface's Draw(*Context) for full control: the
//     Context is a stateful Canvas-like API (Save/Restore, Translate,
//     ClipRect/ClipRRect, Fill/Paint, Opacity, DrawText, DrawImage, RRect)
//     recording onto the display list.
//
// The types are aliases of internal/render/graph so engine internals and
// widget code share one vocabulary; this package exists so widget authors
// import a single, stable, drawing-only surface.
package draw

import (
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
)

type (
	// Context is the stateful Canvas-like recording API (see package doc).
	Context = graph.Context
	// Node is one retained drawing node; implement Draw for custom structure.
	Node = graph.Node
	// Group is a container node (children, opacity, transforms, clip).
	Group = graph.Group
	// Rect is a (rounded) filled/stroked rectangle with optional SDF shadow.
	Rect = graph.Rect
	// Text is a text run (content, size, fill).
	Text = graph.Text
	// Image is a bitmap with fit modes and corner clipping.
	Image = graph.Image
	// Circle is a filled circle.
	Circle = graph.Circle
)

// NewContext starts recording onto the given display list.
func NewContext(ops *op.Ops) *Context { return graph.NewContext(ops) }

// Ops is the display list a Context records onto.
type Ops = op.Ops

// Node constructors (see the type docs for drawable fields).
func NewGroup() *Group   { return graph.NewGroup() }
func NewRect() *Rect     { return graph.NewRect() }
func NewText() *Text     { return graph.NewText() }
func NewImage() *Image   { return graph.NewImage() }
func NewCircle() *Circle { return graph.NewCircle() }
