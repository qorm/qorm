package canvas

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// Layout computes the bounding boxes and generates drawing operations for a tree of nodes.
// inter carries cross-frame interaction state (pressed/hovered/focused); it may be nil.
// scale is the device-pixel ratio (1 = logical == physical; 2 = Retina).
func Layout(ops *op.Ops, root *model.Node, size image.Point, rt *runtime.Runtime, inter *Interaction, scale int) (graph.Node, bool) {
	if root == nil {
		return nil, false
	}
	if scale < 1 {
		scale = 1
	}

	bounds := image.Rect(0, 0, size.X, size.Y)

	// Feed the live surface size to expressions (viewport.width / height /
	// orientation) so responsive `when` nodes and {{ viewport.* }} bindings
	// resolve against the real window — the canvas counterpart of the browser
	// pushing its size over POST /viewport (runtime.Viewport). Logical pixels,
	// matching the browser's CSS px.
	if rt != nil {
		rt.Viewport = runtime.Viewport{W: size.X, H: size.Y}
	}

	// 1. Measure pass (bottom-up)
	rootNode := Measure(root, rt, inter, scale)
	if rootNode == nil {
		return nil, false // the whole scene is conditionally hidden
	}

	// 2. Layout pass (top-down) builds the scene graph
	rootGraphNode := PerformLayout(rootNode, bounds, inter, rt, scale)
	
	// 3. Render pass (retained mode graph -> display list)
	if rootGraphNode != nil {
		ctx := graph.NewContext(ops)
		rootGraphNode.Draw(ctx)
	}
	
	return rootGraphNode, rootNode.NeedsRedraw
}
