package canvas

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// findBoard walks the tree depth-first and returns the first node of type
// "board", or nil. Document order wins — siblings before nested children —
// which matches the rule "the first board in the tree drives the camera".
// Used to let a board live anywhere in the tree (typical: a `row` root with
// a `board` game world + a `view` HUD overlay next to it), not just at the
// root, so apps can have multiple top-level panels without giving up the
// pan/zoom board affordance on the canvas region.
func findBoard(n *model.Node) *model.Node {
	if n == nil {
		return nil
	}
	if n.Type == "board" {
		return n
	}
	for _, c := range n.Children {
		if b := findBoard(c); b != nil {
			return b
		}
	}
	return nil
}

// Layout computes the bounding boxes and generates drawing operations for a tree of nodes.
// inter carries cross-frame interaction state (pressed/hovered/focused); it may be nil.
// scale is the device-pixel ratio (1 = logical == physical; 2 = Retina).
func Layout(ops *op.Ops, root *model.Node, size image.Point, rt *runtime.Runtime, inter *Interaction, scale int) (graph.Node, bool) {
	g, needsRedraw, _, _ := layout(ops, root, size, rt, inter, scale)
	return g, needsRedraw
}

// layout is Layout plus the repeat-instance sidecar (list.go) the engine
// keeps for event dispatch: which item scope a hit belongs to. The public
// wrapper keeps the two-result form for layout-only callers. The LayoutNode
// root is returned so the engine can feed CollectMeasure style fields.
func layout(ops *op.Ops, root *model.Node, size image.Point, rt *runtime.Runtime, inter *Interaction, scale int) (graph.Node, bool, map[graph.Node]itemInstance, *LayoutNode) {
	if root == nil {
		return nil, false, nil, nil
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
		return nil, false, nil, nil // the whole scene is conditionally hidden
	}

	// The scene root is the page: it spans the viewport width (CSS's initial
	// containing block). A bare column/scroll with no width:fill otherwise
	// shrinks to its content and the whole page hugs the left edge — and
	// flex-stretch children then have nothing to stretch into (uikit's
	// panels rendered at content width).
	if rootNode.Style.WidthRaw == "" && rootNode.Style.Width <= 0 && rootNode.Width < bounds.Dx() {
		rootNode.Width = bounds.Dx()
	}

	// An infinite-canvas board is a window-sized plane: it spans the viewport
	// in BOTH axes (its children are absolutely positioned and out of flow, so
	// they contribute nothing to its size), and its interaction sidecar carries
	// the live pan/zoom. The board flag is set here, per frame, so a scene
	// switch to a non-board root clears it via the Interaction reset.
	// The board may sit anywhere in the tree — typically the root for a
	// single-canvas app, but nested inside a `row` / `view` / `column` when
	// the app has siblings (e.g. a HUD overlay next to the game world) that
	// should NOT inherit the pan/zoom. Only the FIRST board in document
	// order drives the camera; a second board in the same tree would be a
	// typo.
	boardNode := findBoard(root)
	if boardNode != nil {
		if inter != nil {
			inter.Board.Active = true
			if inter.Board.Zoom == 0 {
				inter.Board.Zoom = 1
			}
			// A follow-cam board (mario / metroid / sonic) sets
			// `disablePan: true` so the engine never starts a manual pan on
			// a blank-space drag — the camera follow is the only pan, and
			// user drags would fight it. The whiteboard example leaves it
			// off, preserving the existing drag-to-pan behaviour.
			if raw, ok := boardNode.Prop("disablePan"); ok {
				if b, ok := raw.(bool); ok && b {
					inter.Board.PanDisabled = true
				}
			}
		}
	}

	// 1b. Fold text that overflows its column (wrap.go). This must run after
	// measure (sizes known) and before layout (origins assigned); it repairs
	// ancestor heights so pass 2 sees consistent boxes.
	wrapTree(rootNode, bounds.Dx())

	// 2. Layout pass (top-down) builds the scene graph
	items := map[graph.Node]itemInstance{}
	overlays := []graph.Node{}
	rootGraphNode := performLayout(rootNode, bounds, image.Point{}, inter, rt, scale, items, &overlays)

	if rootGraphNode != nil {
		if rootGroup, ok := rootGraphNode.(*graph.Group); ok {
			for _, overlay := range overlays {
				if overlay != nil {
					rootGroup.AddChild(overlay)
				}
			}
		}
	}

	// 3. Render pass (retained mode graph -> display list)
	if rootGraphNode != nil {
		ctx := graph.NewContext(ops)
		rootGraphNode.Draw(ctx)
	}

	return rootGraphNode, rootNode.NeedsRedraw, items, rootNode
}
