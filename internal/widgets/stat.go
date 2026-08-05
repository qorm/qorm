package widgets

import (
	"fmt"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("stat", Stat{})
	canvas.RegisterWidget("metric", Stat{})
}

// Stat is the big-number metric block (HTML render_data.go:1463): an
// uppercase secondary label, a 28/800 value, and an optional delta line
// coloured by deltaType (up/positive/success → green, down/negative/error →
// red, else gray). `metric` is the alias.
type Stat struct{}

// Measure stacks label(12) + value(28) + optional delta(13) with the HTML
// 2px gap, padded by nothing (the card around it owns the padding).
func (Stat) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	h = (12 + 28 + 2*2) * scale
	if statProp(n, "delta", rt) != "" {
		h += (13 + 2) * scale
	}
	w = int(canvas.MeasureText(statValue(n, rt), float64(28*scale)))
	if lw := int(canvas.MeasureText(statProp(n, "label", rt), float64(12*scale))); lw > w {
		w = lw
	}
	return w, h
}

// Record draws the three text lines (all NoHit — the card around the stat
// takes the taps).
func (Stat) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	y := 0
	label := statProp(n0(ln), "label", rt)
	if label != "" {
		fs := 12 * scale
		g.AddChild(formText(label, 0, float64(y), fs, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		y += fs + 2*scale
	}
	vfs := 28 * scale
	g.AddChild(formText(statValue(n0(ln), rt), 0, float64(y), vfs, formInk(ln.Node, ln, rt)))
	y += vfs + 2*scale
	if d := statProp(n0(ln), "delta", rt); d != "" {
		col := color.RGBA{107, 114, 128, 255}
		if raw, ok := n0(ln).Prop("deltaType"); ok {
			switch fmt.Sprint(raw) {
			case "up", "positive", "success":
				col = color.RGBA{22, 163, 74, 255}
			case "down", "negative", "error":
				col = color.RGBA{220, 38, 38, 255}
			}
		}
		g.AddChild(formText(d, 0, float64(y), 13*scale, col))
	}
	return g
}

func n0(ln *canvas.LayoutNode) *model.Node { return ln.Node }

// statValue resolves the big number: the `value` prop, else the node text
// (HTML order).
func statValue(n *model.Node, rt *runtime.Runtime) string {
	if raw, ok := n.Prop("value"); ok {
		return fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), map[string]any{"state": rt.State}))
	}
	return fmt.Sprint(runtime.EvalBinding(n.Text, map[string]any{"state": rt.State}))
}

// statProp evaluates one string prop against state (label/delta).
func statProp(n *model.Node, key string, rt *runtime.Runtime) string {
	raw, ok := n.Prop(key)
	if !ok {
		return ""
	}
	return fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), map[string]any{"state": rt.State}))
}

// Inline marks Stat as inline-level (canvas.InlineWidget): flex containers keep its content size.
func (Stat) Inline() {}
