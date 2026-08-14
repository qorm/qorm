package widgets

// The tooltip (HTML: render_feedback.go:278) — a wrapper showing a small
// bubble with the `tooltip` prop's hint text when the pointer hovers the
// wrapped child. The bubble mounts as an overlay below the box.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("tooltip", &Tooltip{inters: map[*model.Node]*canvas.Interaction{}})
}

// Tooltip wraps one child and shows its hint bubble on hover.
type Tooltip struct {
	mu     sync.Mutex
	inters map[*model.Node]*canvas.Interaction
}

// tooltipHovered reports whether n (or any descendant) is the hovered node —
// the model tree is children-only, so the walk descends from the tooltip.
func tooltipHovered(n, hovered *model.Node) bool {
	if hovered == nil || n == nil {
		return false
	}
	if n == hovered {
		return true
	}
	for _, c := range n.Children {
		if tooltipHovered(c, hovered) {
			return true
		}
	}
	return false
}

// Measure reports the wrapped child's size (children measure through).
func (*Tooltip) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0
}

func (t *Tooltip) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return t.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the wrapped child flows
// with the frame's sinks, and the interaction is cached for the hover check.
func (t *Tooltip) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	t.mu.Lock()
	t.inters[ln.Node] = canvas.SinksInter(sinks)
	t.mu.Unlock()
	return t.record(ln, rt, scale, sinks)
}

func (t *Tooltip) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g
}

// OverlayOpen reports whether the pointer hovers the wrapped child.
func (t *Tooltip) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("tooltip")
	if !ok || fmt.Sprint(raw) == "" {
		return false
	}
	t.mu.Lock()
	inter := t.inters[n]
	t.mu.Unlock()
	return inter != nil && tooltipHovered(n, inter.Hovered)
}

// OverlayRecord draws the hint bubble just below the box.
func (t *Tooltip) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !t.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	tip := formEvalStr(fmt.Sprint(propAny(ln.Node, "tooltip")), rt)
	fs := 12 * scale
	tw := int(canvas.MeasureText(tip, float64(fs)))
	bubbleW := tw + 16*scale
	bubbleH := 28 * scale
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	bx := ln.AbsX + ln.Width/2 - bubbleW/2
	by := ln.AbsY + ln.Height + 6*scale
	if bx < 4*scale {
		bx = 4 * scale
	}
	if bx+bubbleW > stageW-4*scale {
		bx = stageW - bubbleW - 4*scale
	}

	g := draw.NewGroup()
	g.Width = float64(stageW)
	g.Height = float64(stageH)
	g.Model = ln.Node
	g.Overlay = true

	bubble := draw.NewRect()
	bubble.X = float64(bx)
	bubble.Y = float64(by)
	bubble.Width = float64(bubbleW)
	bubble.Height = float64(bubbleH)
	bubble.BorderRadius = 6 * float64(scale)
	bubble.Fill = color.RGBA{40, 40, 46, 235}
	g.AddChild(bubble)
	g.AddChild(formText(tip, float64(bx+8*scale), float64(by)+(float64(bubbleH)-float64(lineHeight(fs)))/2, fs, color.RGBA{255, 255, 255, 255}))
	return g
}

// propAny reads a raw prop value ("" when absent).
func propAny(n *model.Node, key string) any {
	v, _ := n.Prop(key)
	return v
}
