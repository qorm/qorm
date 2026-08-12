package widgets

// CheckboxListTile and RadioListTile — list rows with a leading control
// (HTML controlTile). SwitchListTile lives in fab_controltile.go.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("checkboxlisttile", &CheckboxListTile{local: map[*model.Node]bool{}})
	canvas.RegisterWidget("radiolisttile", &RadioListTile{local: map[*model.Node]string{}})
}

// ---- checkboxlisttile -------------------------------------------------------

// CheckboxListTile is a full-width row: leading checkbox + title/subtitle.
type CheckboxListTile struct {
	mu    sync.Mutex
	local map[*model.Node]bool
}

func (c *CheckboxListTile) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	h = 16*scale + lineHeight(fs)
	if formPropStr(n, "subtitle", rt) != "" {
		h += lineHeight(fs - 2*scale)
	}
	box := 18 * scale
	if h < box+16*scale {
		h = box + 16*scale
	}
	return 300 * scale, h
}

func (c *CheckboxListTile) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	title := formLabel(ln.Node, rt)
	if title == "" {
		title = "..."
	}
	sub := formPropStr(ln.Node, "subtitle", rt)
	on := c.checked(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)

	boxS := float64(18 * scale)
	boxY := (float64(ln.Height) - boxS) / 2
	boxX := float64(14 * scale)
	box := draw.NewRect()
	box.X, box.Y = boxX, boxY
	box.Width, box.Height = boxS, boxS
	box.BorderRadius = 4 * float64(scale)
	if on {
		box.Fill = formAccent(rt)
		checkMark(g, boxX, boxY, boxS, color.RGBA{255, 255, 255, 255})
	} else {
		box.Fill = themeColor(rt, "cardBg", color.RGBA{255, 255, 255, 255})
		box.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
		box.StrokeWidth = float64(scale)
	}
	g.AddChild(box)

	tx := boxX + boxS + float64(12*scale)
	ty := float64(10 * scale)
	g.AddChild(formText(title, tx, ty, fs, ink))
	if sub != "" {
		g.AddChild(formText(sub, tx, ty+float64(lineHeight(fs)), fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}
	return g
}

func (c *CheckboxListTile) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	next := !c.checked(n, nil, rt)
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, next)
	} else {
		c.mu.Lock()
		c.local[n] = next
		c.mu.Unlock()
	}
	commitFormChange(n, rt, next)
	return true
}

func (c *CheckboxListTile) checked(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) bool {
	if formBoundPath(n.Value) != "" {
		return formTruthy(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	}
	c.mu.Lock()
	lv, ok := c.local[n]
	c.mu.Unlock()
	if ok {
		return lv
	}
	if v, ok := n.Prop("checked"); ok {
		return formTruthy(runtime.EvalBinding(fmt.Sprint(v), formCtxLn(rt, ln)))
	}
	return false
}

// ---- radiolisttile ----------------------------------------------------------

// RadioListTile is one radio option row: leading circle + title/subtitle.
// The bound Value is the group selection; the option's identity is prop "value".
type RadioListTile struct {
	mu    sync.Mutex
	local map[*model.Node]string
}

func (r *RadioListTile) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	h = 16*scale + lineHeight(fs)
	if formPropStr(n, "subtitle", rt) != "" {
		h += lineHeight(fs - 2*scale)
	}
	if h < radioCircleD*scale+16*scale {
		h = radioCircleD*scale + 16*scale
	}
	return 300 * scale, h
}

func (r *RadioListTile) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	title := formLabel(ln.Node, rt)
	if title == "" {
		title = "..."
	}
	sub := formPropStr(ln.Node, "subtitle", rt)
	opt := formPropStr(ln.Node, "value", rt)
	if opt == "" {
		opt = title
	}
	sel := r.selected(ln.Node, ln, rt)
	on := sel == opt

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)

	d := float64(radioCircleD * scale)
	cx := float64(14*scale) + d/2
	cy := float64(ln.Height) / 2
	outer := draw.NewCircle()
	outer.Radius = d / 2
	outer.Fill = themeColor(rt, "cardBg", color.RGBA{255, 255, 255, 255})
	outer.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	outer.StrokeWidth = float64(scale)
	if on {
		outer.Stroke = formAccent(rt)
		outer.StrokeWidth = float64(2 * scale)
	}
	og := draw.NewGroup()
	og.X, og.Y = cx-d/2, cy-d/2
	og.AddChild(outer)
	g.AddChild(og)
	if on {
		inner := draw.NewCircle()
		inner.Radius = d / 4
		inner.Fill = formAccent(rt)
		ig := draw.NewGroup()
		ig.X, ig.Y = cx-inner.Radius, cy-inner.Radius
		ig.AddChild(inner)
		g.AddChild(ig)
	}

	tx := float64(14*scale) + d + float64(12*scale)
	ty := float64(10 * scale)
	g.AddChild(formText(title, tx, ty, fs, ink))
	if sub != "" {
		g.AddChild(formText(sub, tx, ty+float64(lineHeight(fs)), fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}
	return g
}

func (r *RadioListTile) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	opt := formPropStr(n, "value", rt)
	if opt == "" {
		opt = formLabel(n, rt)
	}
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, opt)
	} else {
		r.mu.Lock()
		r.local[n] = opt
		r.mu.Unlock()
	}
	commitFormChange(n, rt, opt)
	return true
}

func (r *RadioListTile) selected(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	if formBoundPath(n.Value) != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.local[n]
}

// silence unused image import helpers used by HandlePointer signatures
var _ = image.Point{}
