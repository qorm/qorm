package widgets

// The alert (HTML: render_feedback.go:334) — an INLINE status banner: a
// tinted rounded box with the title and message, colored by the `variant`
// prop (info/success/warning/danger). Unlike the modal it is in-flow, not an
// overlay — it takes its slot in the layout.

import (
	"fmt"
	"image/color"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("alert", &Alert{})
}

// Alert is the inline status banner.
type Alert struct{}

// alertTint returns the variant's tint + ink colors (the HTML alertColors:
// info blue, success green, warning orange, danger red). The tint alpha is
// strong enough to read as a band on a white surface, not just a ghost.
func alertTint(variant string) (bg, fg color.RGBA) {
	switch variant {
	case "success":
		return color.RGBA{52, 199, 89, 60}, color.RGBA{24, 90, 41, 255}
	case "warning":
		return color.RGBA{255, 149, 0, 60}, color.RGBA{120, 66, 0, 255}
	case "danger":
		return color.RGBA{255, 59, 48, 60}, color.RGBA{139, 24, 18, 255}
	default: // info
		return color.RGBA{0, 122, 255, 60}, color.RGBA{0, 62, 130, 255}
	}
}

// Measure reports the title + message content size.
func (*Alert) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	txt := formLabel(n, rt)
	if txt == "" {
		txt = "..."
	}
	w = int(canvas.MeasureText(txt, float64(fs))) + 28*scale
	h = lineHeight(fs) + 24*scale
	if title := formTitle(n, rt); title != "" {
		if tw := int(canvas.MeasureText(title, float64(fs))) + 28*scale; tw > w {
			w = tw
		}
		h += lineHeight(fs) + 2*scale
	}
	return
}

// Record draws the tinted banner with the title and the message.
func (*Alert) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	variant := "info"
	if raw, ok := ln.Node.Prop("variant"); ok {
		if v := formEvalStr(stringify(raw), rt); v != "" {
			variant = v
		}
	}
	bg, fg := alertTint(variant)
	fs := formFontSizeLN(ln, scale)

	g := draw.NewGroup()
	banner := draw.NewRect()
	banner.Width = float64(ln.Width)
	banner.Height = float64(ln.Height)
	banner.BorderRadius = 12 * float64(scale)
	banner.Fill = bg
	g.AddChild(banner)

	y := float64(12 * scale)
	if title := formTitle(ln.Node, rt); title != "" {
		g.AddChild(formText(title, float64(14*scale), y, fs+2*scale, fg))
		y += float64(lineHeight(fs+2*scale) + 2*scale)
	}
	txt := formLabel(ln.Node, rt)
	if txt == "" {
		txt = "..."
	}
	g.AddChild(formText(txt, float64(14*scale), y, fs, fg))
	return g
}

// formTitle reads the title prop (formLabel reads label/text).
func formTitle(n *model.Node, rt *runtime.Runtime) string {
	raw, ok := n.Prop("title")
	if !ok {
		return ""
	}
	return formEvalStr(stringify(raw), rt)
}

// stringify renders any prop value as its display string.
func stringify(v any) string {
	return fmt.Sprint(v)
}
