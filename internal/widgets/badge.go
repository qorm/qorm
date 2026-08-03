// Package widgets hosts the built-in widget library that lives OUTSIDE the
// canvas engine, and is the home of app-defined custom components. Widgets
// compose the low-level draw layer (internal/render/draw) and register
// themselves through canvas.RegisterWidget — the engine looks scene types up
// in that registry instead of hardcoding them. Importing this package (e.g.
// from the app host's main) registers all built-ins via init.
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
	canvas.RegisterWidget("badge", Badge{})
}

// Badge is the pill-shaped status label (HTML: inline-flex, padding 2px 8px,
// border-radius 999px, 12px semibold, background var(--fill) with secondary
// label color — render_feedback.go badge()).
type Badge struct{}

// Measure reports the pill's content size: label width plus 8px horizontal
// padding per side, one line tall plus 2px vertical padding (× scale).
func (Badge) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := 12 * scale
	w = int(canvas.MeasureText(badgeLabel(n, nil, rt), float64(fs))) + 16*scale
	h = int(float64(fs)*1.2) + 4*scale
	return
}

// Record builds the pill (fully rounded rect) with its label centered.
func (Badge) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	fs := 12 * scale

	pill := draw.NewRect()
	pill.Width = float64(ln.Width)
	pill.Height = float64(ln.Height)
	pill.BorderRadius = float64(ln.Height) / 2 // a full pill, like CSS 999px
	// HTML spells the standalone badge background var(--fill) (a grey) —
	// canvas resolves that alias to inputBg; surface would be translucent
	// white and invisible on the white scene (R7-A).
	pill.Fill = themeColor(rt, "inputBg", color.RGBA{238, 238, 240, 255})

	label := badgeLabel(ln.Node, ln, rt)
	txtW := int(canvas.MeasureText(label, float64(fs)))
	txtH := int(float64(fs) * 1.2)
	text := draw.NewText()
	text.Content = label
	text.FontSize = float64(fs)
	text.Fill = themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})
	text.X = float64((ln.Width - txtW) / 2)
	text.Y = float64((ln.Height - txtH) / 2)

	g := draw.NewGroup()
	g.AddChild(pill)
	g.AddChild(text)
	return g
}

// badgeLabel resolves the pill text from label/text/value props (HTML
// labelOf order), evaluating {{...}} bindings against state.
func badgeLabel(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	for _, k := range []string{"label", "text", "value"} {
		if raw, ok := n.Prop(k); ok {
			return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln))))
		}
	}
	return ""
}

// themeColor resolves a theme color token with a hard fallback.
func themeColor(rt *runtime.Runtime, token string, fallback color.RGBA) color.RGBA {
	if rt != nil && rt.Theme != nil {
		if c, ok := rt.Theme.GetColor(token); ok {
			return c
		}
	}
	return fallback
}
