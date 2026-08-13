package canvas

import (
	"image"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// path renders an SVG-subset vector path ('d' node prop) into the display
// list: the fill comes from the style `background` (a 'transparent'
// background means no fill), the outline from `strokeColor`/`strokeWidth`.
// 'd' is a plain node prop re-evaluated from bindings at every render, so a
// state-driven swap snaps the shape — the decided MVP scope, no morph
// interpolation (the HTML backend updates the <path d=…> the same way).
//
// The path's coordinates are authoritative in the node's own pixel space
// (1 unit = 1 px, origin at the node's top-left), mirroring the HTML side's
// viewBox: coordinates are authored against the layout box the style
// declares. Without explicit width/height the box defaults to a 0,0-anchored
// box sized bb.Max.X x bb.Max.Y (Measure reports it — see the semantics
// choice above Measure; the generic explicit-size override still wins when
// set).
func init() {
	RegisterWidget("path", pathWidget{})
}

type pathWidget struct{}

// svgPathD evaluates the node's `d` prop, resolving bindings against the live
// runtime (state swaps re-shape the path on the next frame — no morphing).
func svgPathD(n *model.Node, rt *runtime.Runtime) string {
	if n == nil {
		return ""
	}
	raw, ok := n.Prop("d")
	if !ok {
		return ""
	}
	s, _ := evalStyleProp(raw, rt).(string)
	return strings.TrimSpace(s)
}

// pathBBox returns the union of all subpath bounds in path coordinates
// (image.Rectangle{} for an empty/malformed d).
func pathBBox(d string) image.Rectangle {
	sub := parsePathToSubpaths(d)
	if len(sub) == 0 {
		return image.Rectangle{}
	}
	bb := computeBBox(sub[0])
	for _, p := range sub[1:] {
		bb = bb.Union(computeBBox(p))
	}
	return bb
}

func (pathWidget) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	d := svgPathD(n, rt)
	if d == "" {
		return 0, 0
	}
	bb := pathBBox(d)
	// Chosen semantics: a box anchored at the node's origin (0,0) sized
	// bb.Max.X x bb.Max.Y — the canvas mirror of the HTML side's
	// viewBox="0 0 w h", so both backends keep the geometry in author
	// coordinates and render shape-for-shape identical. The previous size
	// (bb.Dx x bb.Dy) cropped off-path shapes: Record never translates the
	// geometry by -bb.Min, so a path with bbox.Min > 0 and no explicit
	// width/height (e.g. "M 50 50 L 150 150 L 50 150 Z") painted only a
	// corner sliver inside its own raster. Negative coordinates clip,
	// exactly as viewBox="0 0 w h" clips on the HTML side — authors anchor
	// paths at 0,0 or set explicit width/height.
	return bb.Max.X * scale, bb.Max.Y * scale
}

func (pathWidget) Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	d := svgPathD(ln.Node, rt)
	if d == "" {
		return nil
	}
	// A gradient background paints no solid fill (only solid `background`
	// values fill a path in v1); unset/transparent backgrounds leave alpha 0
	// and skip fill entirely.
	fill := color.RGBA{}
	if len(ln.Style.GradientStops) < 2 {
		fill = ln.Style.Background
	}
	sw := ln.Style.StrokeWidth
	if scale < 1 {
		scale = 1
	}
	if sw > 0 {
		sw *= float64(scale)
	}
	ops := pathOps(d, fill, ln.Style.StrokeColor, sw)
	if len(ops) == 0 {
		return nil
	}
	// The authored coordinates are CSS px; ln.Width/ln.Height are physical px,
	// so a high-DPI frame scales the whole path (stroke included) uniformly —
	// exactly what the HTML side's browser scaling does.
	if scale > 1 {
		ops = scalePathOps(ops, scale)
	}
	img := Rasterize(listOps(ops), image.Pt(ln.Width, ln.Height))
	node := graph.NewImage()
	node.Width = float64(ln.Width)
	node.Height = float64(ln.Height)
	node.Bitmap = img
	node.Fit = "fill"
	return node
}

// scalePathOps wraps a display-list batch (with its Save/Restore groups) in a
// uniform scale transform so one authored path renders crisply at any device
// scale. The previous state is restored on Exit… the RestoreOp already inside
// each group pops to the Pre-ColorOp color under the SCRALED matrix; to keep
// the group isolation intact the transform wraps the whole list.
func scalePathOps(ops []op.Op, scale int) []op.Op {
	m := geom.Identity().Scale(float64(scale), float64(scale))
	out := make([]op.Op, 0, len(ops)+2)
	out = append(out, op.SaveOp{}, op.TransformOp{M: m})
	out = append(out, ops...)
	out = append(out, op.RestoreOp{})
	return out
}

// listOps assembles a display list from a batch of ops (the batch carries its
// own Save/Restore pairs, so Rasterize's interpreter starts balanced).
func listOps(ops []op.Op) *op.Ops {
	o := &op.Ops{}
	for _, p := range ops {
		o.Add(p)
	}
	return o
}
