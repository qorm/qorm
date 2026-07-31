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
func Layout(ops *op.Ops, root *model.Node, size image.Point, rt *runtime.Runtime, inter *Interaction) (graph.Node, bool) {
	if root == nil {
		return nil, false
	}

	bounds := image.Rect(0, 0, size.X, size.Y)

	// 1. Measure pass (bottom-up)
	rootNode := Measure(root, rt, inter)

	// 2. Layout pass (top-down) builds the scene graph
	rootGraphNode := PerformLayout(rootNode, bounds, inter, rt)
	
	// 3. Render pass (retained mode graph -> display list)
	if rootGraphNode != nil {
		ctx := graph.NewContext(ops)
		rootGraphNode.Draw(ctx)
	}
	
	return rootGraphNode, rootNode.NeedsRedraw
}
