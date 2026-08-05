package widgets

// The chip (HTML: render_input.go:456) — a pill label: fill/ink switch to
// accent when `selected` is truthy, a leading avatar glyph, an optional check
// mark on selected filterchips, and a delete "×" that dispatches onChange.
// Pressing the chip body dispatches onPress (the generic engine path).

import (
	"fmt"
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("chip", &Chip{})
	canvas.RegisterWidget("inputchip", &Chip{})
	canvas.RegisterWidget("choicechip", &Chip{})
	canvas.RegisterWidget("filterchip", &Chip{})
}

// Chip is the pill label.
type Chip struct{}

// chipLabel resolves the label: the `label` prop first, then n.Label/n.Text.
func chipLabel(n *model.Node, rt *runtime.Runtime) string {
	if v := formPropStr(n, "label", rt); v != "" {
		return v
	}
	return formLabel(n, rt)
}

// chipSelected evaluates the `selected` prop.
func chipSelected(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("selected")
	if !ok {
		return false
	}
	return formTruthy(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil)))
}

// Measure reports the pill size: label (+ avatar + check) with pill padding.
func (*Chip) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	w = int(canvas.MeasureText(chipLabel(n, rt), float64(fs))) + 24*scale
	if av := formPropStr(n, "avatar", rt); av != "" {
		w += 20 * scale
	}
	if chipSelected(n, rt) && (n.Type == "filterchip" || formPropStr(n, "showCheck", rt) == "true") {
		w += 14 * scale
	}
	h = lineHeight(fs) + 10*scale
	return
}

// Record draws the pill: check (filterchips), avatar, label, delete ×.
func (*Chip) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	selected := chipSelected(ln.Node, rt)
	fs := formFontSizeLN(ln, scale)
	ink := color.RGBA{55, 48, 163, 255}
	accent := formAccent(rt)
	if selected {
		ink = color.RGBA{255, 255, 255, 255}
	}

	g := draw.NewGroup()
	pill := draw.NewRect()
	pill.Width = float64(ln.Width)
	pill.Height = float64(ln.Height)
	pill.BorderRadius = float64(ln.Height) / 2
	if selected {
		pill.Fill = accent
	} else {
		pill.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	}
	g.AddChild(pill)

	x := float64(10 * scale)
	if selected && (ln.Node.Type == "filterchip" || formPropStr(ln.Node, "showCheck", rt) == "true") {
		g.AddChild(formText("✓", x, float64(3*scale), fs, ink))
		x += float64(14 * scale)
	}
	if av := formPropStr(ln.Node, "avatar", rt); av != "" {
		g.AddChild(formText(av, x, float64(3*scale), fs+4*scale, ink))
		x += float64(20 * scale)
	}
	g.AddChild(formText(chipLabel(ln.Node, rt), x, float64((ln.Height-lineHeight(fs))/2), fs, ink))
	if ln.Node.OnChange != nil || ln.Node.Type == "inputchip" {
		g.AddChild(formText("×", float64(ln.Width-14*scale), float64((ln.Height-lineHeight(fs))/2), fs, color.RGBA{0, 0, 0, 140}))
	}
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on the delete
// "×" dispatches onChange; a press elsewhere falls through to the generic
// onPress dispatch (the widget returns false and the engine bubbles).
func (*Chip) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || n.OnChange == nil {
		return false
	}
	// The delete affordance occupies the right ~24px of the pill.
	if p.X < float64(frame.Max.X-24) {
		return false
	}
	argAny := make(map[string]any, len(n.OnChange.Args))
	for k, v := range n.OnChange.Args {
		argAny[k] = v
	}
	rt.Dispatch(n.OnChange.Name, argAny)
	return true
}

// Inline keeps the pill at its content size in flex containers.
func (Chip) Inline() {}
