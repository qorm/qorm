package widgets

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("link", Link{})
}

// Link is the accent-colored, underlined text link (HTML
// render_widgets.go:208: an <a> with a safeURL-checked href and pressAttr).
// The canvas engine has no URL-navigation channel, so href is inert here —
// onPress still fires through the engine's generic type-agnostic dispatch
// (canvas/widget.go: canPress is type-agnostic). Color: an author style
// color wins (HTML textCSS), else the accent/primary theme token (the HTML
// link inherits var(--accent)).
type Link struct{}

// linkLabel resolves the link text like the HTML labelOf (render_style.go:587:
// label, then text), evaluating {{...}} bindings against state.
func linkLabel(n *model.Node, rt *runtime.Runtime) string {
	raw := n.Label
	if raw == "" {
		raw = n.Text
	}
	return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(raw, map[string]any{"state": rt.State})))
}

// accentColor resolves the link ink: palette "accent", then "primary", then
// the Apple-blue literal (matching defaultVars --accent, canvas/style.go:33).
func accentColor(rt *runtime.Runtime) color.RGBA {
	if rt != nil && rt.Theme != nil {
		if c, ok := rt.Theme.GetColor("accent"); ok {
			return c
		}
	}
	return themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
}

// linkFontSize mirrors the engine's text default (measure.go:154: 14 when
// the style sets no fontSize).
func linkFontSize(n *model.Node) int {
	if v, ok := styleNumber(n, "fontSize"); ok && v > 0 {
		return v
	}
	return 14
}

// Measure reports exactly the engine's text-node sizing for the label
// (measure.go:161: width = MeasureText, height = fs*1.2).
func (Link) Measure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := linkFontSize(n) * scale
	w = int(canvas.MeasureText(linkLabel(n, rt), float64(fs)))
	h = int(float64(fs) * 1.2)
	return
}

// Record builds the label text plus a 1px underline in the same ink.
func (Link) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := ln.Style.FontSize // already physical px (parseStyle scaleBy)
	if fs == 0 {
		fs = 14 * scale
	}
	label := linkLabel(ln.Node, rt)
	if label == "" {
		return nil
	}

	ink := ln.Style.Color
	if _, authorColor := ln.Node.Style["color"]; !authorColor || ink.A == 0 {
		ink = accentColor(rt)
	}

	txtW := int(canvas.MeasureText(label, float64(fs)))
	txtH := int(float64(fs) * 1.2)
	tx := 0
	if ln.Style.TextAlign == "center" {
		tx = (ln.Width - txtW) / 2
	}
	ty := (ln.Height - txtH) / 2

	text := draw.NewText()
	text.Content = label
	text.FontSize = float64(fs)
	text.Fill = ink
	text.X = float64(tx)
	text.Y = float64(ty)

	underline := draw.NewRect()
	underline.Fill = ink
	underline.X = float64(tx)
	underline.Y = float64(ty + txtH - scale)
	underline.Width = float64(txtW)
	underline.Height = float64(scale)

	g := draw.NewGroup()
	g.AddChild(text)
	g.AddChild(underline)
	return g
}
