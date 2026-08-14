package widgets

import (
	"fmt"
	"image"
	"image/color"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("bottomnav", &BottomNav{local: map[*model.Node]string{}})
	canvas.RegisterWidget("bottomnavigationbar", &BottomNav{local: map[*model.Node]string{}})
	canvas.RegisterWidget("navigationbar", &BottomNav{local: map[*model.Node]string{}})
}

// BottomNav is Flutter's BottomNavigationBar/NavigationBar (HTML
// render_widgets.go:120): a full-width bar with a top hairline, icon+label
// destinations spread evenly, the current one accent-coloured. A tap writes
// the bound value back (state.x) and dispatches onChange with {value} — the
// form family's controlled/uncontrolled duality (the manifest's onChange may
// own the write instead; a bound path still takes the direct write, like
// every other picker).
type BottomNav struct {
	local map[*model.Node]string // uncontrolled current value per node
}

type navItem struct {
	value string
	icon  string
	label string
}

func navItems(n *model.Node, rt *runtime.Runtime) []navItem {
	raw, ok := n.Props["items"]
	if !ok {
		return nil
	}
	var arr []any
	switch v := raw.(type) {
	case []any:
		arr = v
	case string:
		arr, _ = runtime.EvalBinding(v, formCtx(rt)).([]any)
	}
	out := make([]navItem, 0, len(arr))
	for _, it := range arr {
		obj, _ := it.(map[string]any)
		if obj == nil {
			continue
		}
		out = append(out, navItem{
			value: fmt.Sprint(obj["value"]),
			icon:  fmt.Sprint(obj["icon"]),
			label: fmt.Sprint(obj["label"]),
		})
	}
	return out
}

// navBarH mirrors the HTML bar: 8 + 20 + 2 + 12 + 8 ≈ 50 logical px.
func navBarH(scale int) int { return 50 * scale }

// Measure spans the cross axis at bar height.
func (b *BottomNav) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 0, navBarH(scale)
}

// current resolves the shown value: the binding, then the uncontrolled pick,
// then the first item (HTML shows the bound/initial current).
func (b *BottomNav) current(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime, items []navItem) string {
	if formBoundPath(n.Value) != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	}
	if v, ok := b.local[n]; ok {
		return v
	}
	if n.Value != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	}
	if len(items) > 0 {
		return items[0].value
	}
	return ""
}

// Record paints the bar (surface, top hairline) and the destinations.
func (b *BottomNav) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	items := navItems(ln.Node, rt)
	if ln.Width <= 0 || len(items) == 0 {
		return nil
	}
	barH := navBarH(scale)
	cur := b.current(ln.Node, ln, rt, items)
	accent := themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	idle := color.RGBA{107, 114, 128, 255}

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(barH)

	bg := draw.NewRect()
	bg.Width = float64(ln.Width)
	bg.Height = float64(barH)
	bg.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	bg.Fill.A = 255
	g.AddChild(bg)
	sep := draw.NewRect()
	sep.Width = float64(ln.Width)
	sep.Height = float64(scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)

	itemW := float64(ln.Width) / float64(len(items))
	iconS := 20 * scale
	labelFs := 12 * scale
	for i, it := range items {
		ink := idle
		if it.value == cur {
			ink = accent
		}
		x0 := float64(i) * itemW
		if body, ok := iconSet[it.icon]; ok {
			ic := draw.NewImage()
			ic.NoHit = true
			ic.Bitmap = rasterIcon(body, iconS, iconS, ink)
			ic.Width = float64(iconS)
			ic.Height = float64(iconS)
			ic.X = x0 + (itemW-float64(iconS))/2
			ic.Y = float64(8 * scale)
			ic.Fit = "fill"
			g.AddChild(ic)
		}
		if it.label != "" {
			tw := int(canvas.MeasureText(it.label, float64(labelFs)))
			g.AddChild(formText(it.label, x0+(itemW-float64(tw))/2, float64(8*scale+iconS+2*scale), labelFs, ink))
		}
	}
	return g
}

// HandlePointer picks the tapped destination: bound write-back + onChange
// with {value} (the select pattern).
func (b *BottomNav) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || formDisabled(n, rt) {
		return false
	}
	items := navItems(n, rt)
	if len(items) == 0 || frame.Empty() {
		return false
	}
	idx := int((p.X - float64(frame.Min.X)) / (float64(frame.Dx()) / float64(len(items))))
	if idx < 0 || idx >= len(items) {
		return false
	}
	next := items[idx].value
	inter.Focused = n
	inter.FocusVisible = false
	if next != b.current(n, &canvas.LayoutNode{Node: n}, rt, items) {
		if path := formBoundPath(n.Value); path != "" {
			rt.SetStatePath(path, next)
		} else {
			b.local[n] = next
		}
		commitFormChange(n, rt, next)
	}
	return true
}
