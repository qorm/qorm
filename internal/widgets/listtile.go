package widgets

// The list tile (HTML: render_data.go:752) — a standard list row: optional
// leading glyph, a title with an optional subtitle, an optional trailing text
// and the iOS disclosure chevron when the tile has an onPress (the generic
// engine press path dispatches it — no widget-side input logic needed, and
// the declarative interaction effects apply).

import (
	"fmt"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("listtile", &ListTile{})
	canvas.RegisterWidget("listitem", &ListTile{})
}

// ListTile is the standard list row.
type ListTile struct{}

// Measure reports the row's height: padding + title line (+ subtitle line).
func (*ListTile) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	h = 20*scale + lineHeight(fs)
	if sub := formPropStr(n, "subtitle", rt); sub != "" {
		h += lineHeight(fs - 2*scale)
	}
	w = 300 * scale // rows stretch in a list; a sane minimum
	if n.Label != "" || n.Text != "" {
		w = int(canvas.MeasureText(formEvalStr(n.Label, rt), float64(fs))) + 80*scale
	}
	return
}

// Record draws the row: leading, title+subtitle, trailing / chevron.
func (*ListTile) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	sub := formPropStr(ln.Node, "subtitle", rt)
	trailing := formPropStr(ln.Node, "trailing", rt)

	g := draw.NewGroup()
	x := float64(14 * scale)
	title := formLabel(ln.Node, rt)
	if title == "" {
		title = "..."
	}
	if lead := formPropStr(ln.Node, "leading", rt); lead != "" {
		g.AddChild(formText(lead, x, float64(12*scale), fs+8*scale, ink))
		x += float64(30 * scale)
	}
	ty := float64(12 * scale)
	g.AddChild(formText(title, x, ty, fs, ink))
	ty += float64(lineHeight(fs))
	if sub != "" {
		g.AddChild(formText(sub, x, ty, fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}
	// trailing text or the disclosure chevron on pressable tiles.
	if trailing != "" {
		g.AddChild(formText(trailing, float64(ln.Width)-float64(14*scale)-float64(int(canvas.MeasureText(trailing, float64(fs-2*scale)))), float64(12*scale), fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	} else if ln.Node.OnPress != nil && formPropStr(ln.Node, "chevron", rt) != "false" {
		g.AddChild(formText("›", float64(ln.Width-20*scale), float64(10*scale), fs+6*scale, color.RGBA{196, 196, 198, 255}))
	}
	return g
}

// formPropStr reads a string prop, evaluating bindings ("" when absent).
func formPropStr(n *model.Node, key string, rt *runtime.Runtime) string {
	raw, ok := n.Prop(key)
	if !ok {
		return ""
	}
	return formEvalStr(fmt.Sprint(raw), rt)
}

// Inline keeps the tile's content size in flex containers.
func (ListTile) Inline() {}
