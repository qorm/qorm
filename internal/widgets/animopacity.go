package widgets

import (
	"fmt"
	"image"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("animatedopacity", AnimOpacity{})
}

// AnimOpacity is Flutter's AnimatedOpacity (HTML render_animation.go
// animatedOpacity()): fades its children to the bound `opacity` (0..1,
// default 1) whenever the value changes, over the theme's motion tokens. The
// tween lifecycle runs through canvas.UpdateAndGetAnimatedStyle (the same
// tween engine animated_container uses); a per-node `duration` prop is NOT
// honoured — that helper takes its pace only from the theme motion tokens
// (250ms normal / standard easing with no theme loaded).
//
// v1 seam notes: the Widget contract is leaf-shaped and the engine applies
// style opacity to the node's OWN group only, so the widget wraps the
// children itself — Record mounts them (canvas.PerformLayout, the generic
// column flow) inside a draw.Group carrying the animated Opacity and drops
// ln.Children so the generic pass does not re-mount them opaque. Two honest
// consequences: the subtree is laid out with a nil Interaction (pressed/
// hovered/focus visuals and rings inside it do not light up; hit testing and
// onPress dispatch stay wired), and a mid-flight tween cannot set
// LayoutNode.NeedsRedraw (the Widget interface has no redraw channel) — so
// the widget is an AnimatedWidget instead, keeping the frame loop alive via
// sceneAnimating until the tween settles.
type AnimOpacity struct{}

// aopAnimated tracks which nodes have a mid-flight tween; Animating reports
// for the whole type because the AnimatedWidget seam carries no node.
var (
	aopMu       sync.Mutex
	aopAnimated = map[*model.Node]bool{}
)

// Animating reports true while any mounted node's opacity tween is running.
func (AnimOpacity) Animating() bool {
	aopMu.Lock()
	defer aopMu.Unlock()
	for _, running := range aopAnimated {
		if running {
			return true
		}
	}
	return false
}

// Measure reports the children's content size (column: widest child wide,
// heights summed) measured through canvas.Measure. The inter-child style gap
// is not counted — an author combining gap with no explicit size gets a box a
// few px short; style width/height resolve that through the generic override.
func (AnimOpacity) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height
		}
	}
	return w, h
}

// Record advances the tween for the current opacity target and mounts the
// children under a group carrying the interpolated opacity (group alpha
// multiplies through the subtree in the rasterizer — CSS opacity semantics).
func (a AnimOpacity) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return a.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the opacity group's
// children are laid out by the widget itself, so the frame's sinks must flow
// into that PerformLayout call (see Tabs for why).
func (a AnimOpacity) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return a.record(ln, rt, scale, sinks)
}

func (a AnimOpacity) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	target := 1.0
	if raw, ok := ln.Node.Prop("opacity"); ok {
		target = asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln)))
	}
	if target < 0 {
		target = 0
	}
	if target > 1 {
		target = 1
	}

	// First sight settles immediately at the target (no fade-in on mount),
	// matching animated_container's first-frame behaviour.
	style, running := canvas.UpdateAndGetAnimatedStyle(ln.Node.ID, canvas.NodeStyle{Opacity: target}, rt)
	aopMu.Lock()
	aopAnimated[ln.Node] = running
	aopMu.Unlock()

	g := draw.NewGroup()
	g.Opacity = style.Opacity

	// Mount the children with the generic column flow (padding + gap), then
	// drop them so the engine does not mount them again OUTSIDE the opacity
	// group. ln.Children is rebuilt by Measure every frame, so this is
	// frame-local.
	cy := ln.Style.Padding
	for _, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(ln.Style.Padding, cy, ln.Style.Padding+cw, cy+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), nil, rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		cy += ch + ln.Style.Gap
	}
	ln.Children = nil
	return g
}
