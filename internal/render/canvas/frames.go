package canvas

import (
	"image"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
)

// WidgetFrames returns the rendered bounding boxes (physical px, global
// scene coordinates) of every graph node built from a model node of type
// typ, keyed by that model node. It is the seam a native host uses to overlay
// platform views on the canvas window — e.g. the canvaswebview build covers
// each "webview" widget's box with a real WKWebView subview, created, moved
// and destroyed to track this map frame by frame.
//
// It must be called from the render thread (graphRoot is render-thread
// state, rebuilt by every RenderInto). A repeat template's instances share
// one model pointer, so they collapse to the FIRST instance in layout order,
// matching findGroupByModel; a hidden (if/visible-gated) node never reaches
// the graph and is simply absent from the map.
func (e *Engine) WidgetFrames(typ string) map[*model.Node]image.Rectangle {
	frames := map[*model.Node]image.Rectangle{}
	if e.graphRoot == nil || typ == "" {
		return frames
	}
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		if m := n.Base().Model; m != nil && m.Type == typ {
			if _, ok := frames[m]; !ok {
				bb := n.GetBBox()
				frames[m] = image.Rect(int(bb.MinX), int(bb.MinY), int(bb.MaxX), int(bb.MaxY))
			}
		}
		if g, ok := n.(*graph.Group); ok {
			for _, c := range g.Children {
				walk(c)
			}
		}
	}
	walk(e.graphRoot)
	return frames
}
