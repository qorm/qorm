package render

import (
	"fmt"
	"html"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func (r *renderer) image(n *model.Node) {
	src := propStr(n, "src")
	// fit is an author prop interpolated into a quoted style attribute.
	style := r.boxCSS(n) + "object-fit:" + styleAttr(propStrOr(n, "fit", "cover")) + ";"
	fmt.Fprintf(&r.sb, `<img id=%q src=%q style=%q alt=%q%s>`,
		attrID(n.ID), html.EscapeString(src), style, html.EscapeString(propStr(n, "alt")), a11y(n))
}

func (r *renderer) avatar(n *model.Node) {
	size := propNum(n, "size", 40)
	base := fmt.Sprintf("width:%gpx;height:%gpx;border-radius:50%%;overflow:hidden;flex-shrink:0;", size, size)
	if src := propStr(n, "src"); src != "" {
		fmt.Fprintf(&r.sb, `<img id=%q src=%q style=%q alt="">`, attrID(n.ID), html.EscapeString(src), r.boxCSS(n)+base+"object-fit:cover;")
		return
	}
	initials := r.interp(propStrOr(n, "initials", propStr(n, "name")))
	if rs := []rune(initials); len(rs) > 2 {
		initials = string(rs[:2]) // rune-safe: don't split a multibyte glyph
	}
	style := r.boxCSS(n) + base + r.textCSS(n) +
		"display:inline-flex;align-items:center;justify-content:center;background:#6366f1;color:#fff;font-weight:600;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>%s</div>`, attrID(n.ID), style, html.EscapeString(strings.ToUpper(initials)))
}

func (r *renderer) icon(n *model.Node) {
	name := r.interp(propStrOr(n, "icon", propStrOr(n, "glyph", n.Text)))
	style := r.boxCSS(n) + r.textCSS(n) + "display:inline-flex;align-items:center;justify-content:center;line-height:1;"
	// Prefer a built-in SVG icon (the framework's alternative to emoji); fall
	// back to the raw text/glyph for names we don't ship.
	if svg := iconSVG(name, propNum(n, "size", 22)); svg != "" {
		fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, attrID(n.ID), style, a11y(n), svg)
		return
	}
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, attrID(n.ID), style, a11y(n), html.EscapeString(name))
}

// chart renders a bar / line / area / sparkline as inline SVG. Data is a bound
// array ("{{state.series}}") or a literal number array in the `data` prop.
func (r *renderer) chart(n *model.Node) {
	vals := r.chartData(n)
	w := numOrDefault(n.Style, "width", 240)
	h := numOrDefault(n.Style, "height", 80)
	color := propStrOr(n, "color", "var(--accent)")
	var inner string
	switch propStrOr(n, "chartType", "bar") {
	case "line", "sparkline", "area":
		inner = chartLine(vals, w, h, color, propStrOr(n, "chartType", "line"))
	default:
		inner = chartBars(vals, w, h, color)
	}
	// width:100% so the chart scales to its container (a fixed px width would
	// overflow narrow cards); the viewBox keeps the path coordinates crisp.
	extra := ""
	if m := colorStr(n.Style, "margin"); m != "" {
		_ = m
	}
	// width attribute (natural size) + max-width:100% — fills up to its natural
	// width, caps at the container (no overflow), and keeps a non-zero
	// max-content so it never collapses a shrink-to-fit parent to 0.
	fmt.Fprintf(&r.sb, `<svg id=%q width="%g" height="%g" viewBox="0 0 %g %g" preserveAspectRatio="none" style="display:block;max-width:100%%;height:%gpx;%s">%s</svg>`,
		attrID(n.ID), w, h, w, h, h, extra, inner)
}

func (r *renderer) chartData(n *model.Node) []float64 {
	raw, _ := n.Prop("data")
	switch d := raw.(type) {
	case string:
		if arr, ok := runtime.EvalBinding(d, r.ctx()).([]any); ok {
			return toFloats(arr)
		}
	case []any:
		return toFloats(d)
	}
	return nil
}

func (r *renderer) video(n *model.Node) {
	fmt.Fprintf(&r.sb, `<video id=%q src=%q controls style=%q></video>`,
		attrID(n.ID), html.EscapeString(propStr(n, "src")), r.boxCSS(n))
}
