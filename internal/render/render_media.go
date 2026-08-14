package render

import (
	"fmt"
	"html"
	"strings"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func (r *renderer) image(n *model.Node) {
	// src/alt are interpolated so a data-driven row ({{item.src}}) resolves to
	// the element's own URL; a polymorphic feed renders one image node per item.
	src := r.interp(propStr(n, "src"))
	// fit is an author prop landing mid-declaration ("object-fit:%s;"): the
	// CSS-value allowlist (cssValueOr) stops a `;` from ending that declaration
	// and starting its own; styleAttr guards the attribute on top.
	style := r.boxCSS(n) + "object-fit:" + styleAttr(cssValueOr(propStr(n, "fit"), "cover")) + ";"
	// placeholder paints the element's background while (or if) the real src
	// loads: a low-res/blur image URL renders as a covering background image,
	// anything else is treated as a CSS color (e.g. "#e5e7eb", "var(--fill)").
	// It follows boxCSS so an explicit placeholder wins over style.background.
	//
	// Each branch is validated for the syntax it is written into, because
	// styleAttr is not enough for either: entity encoding leaves `;` and `)`
	// alone, so the colour branch would accept
	// `#eee;position:fixed;…;width:100vw;height:100vh` (a full-screen overlay)
	// and the URL branch would accept `a.png);position:fixed;…;background:url(b`
	// — a value looksLikeImageURL happily calls a URL. cssStyleValue and
	// cssURLToken reject those; an unusable placeholder is simply not painted,
	// which is what the element looked like before the prop existed.
	if ph := propStr(n, "placeholder"); ph != "" {
		if looksLikeImageURL(ph) {
			if u := cssURLToken(ph); u != "" {
				style += "background:url(" + styleAttr(u) + ") center/cover no-repeat;"
			}
		} else if c := cssStyleValue(ph); c != "" {
			style += "background:" + styleAttr(c) + ";"
		}
	}
	// Native lazy loading is on by default (below-the-fold images defer until
	// scroll); lazy:false opts a hero image back into eager loading.
	lazyAttr := ` loading="lazy"`
	if v, ok := n.Prop("lazy"); ok && !asBool(v) {
		lazyAttr = ""
	}
	// fallback swaps in an alternate src (or data: URI) when the image fails to
	// load. The handler rides an inline onerror attribute, built with the same
	// two-layer construction as the gesture wiring scripts: jsStringID makes the
	// fallback a safe JS string literal (quotes/backslashes escaped, "<"
	// neutralised), then html.EscapeString entity-encodes the WHOLE handler so
	// nothing can terminate the quoted attribute — the browser decodes entities
	// before the JS parser runs, so the code executes exactly as written.
	// Clearing this.onerror first makes a failing fallback stop, not loop.
	fallbackAttr := ""
	if fb := propStr(n, "fallback"); fb != "" {
		fallbackAttr = ` onerror="` + html.EscapeString("this.onerror=null;this.src="+jsStringID(fb)) + `"`
	}
	fmt.Fprintf(&r.sb, `<img id=%q src=%q style=%q alt=%q%s%s%s>`,
		attrID(n.ID), html.EscapeString(src), style, html.EscapeString(r.interp(propStr(n, "alt"))),
		lazyAttr, fallbackAttr, r.a11y(n))
}

// looksLikeImageURL decides which form an image `placeholder` takes: an image
// URL (path/absolute/data URI, or a bare file name with an image extension)
// paints as a background image, anything else as a background color. The color
// grammar can contain "." and "(" (rgba(0,0,0,.5), var(--x)) but never "/",
// and legitimate URLs without a "/" still end in an image extension — so the
// two rules together disambiguate without a full URL parse.
func looksLikeImageURL(s string) bool {
	if strings.Contains(s, "/") || strings.HasPrefix(s, "data:") {
		return true
	}
	dot := strings.LastIndexByte(s, '.')
	if dot < 0 {
		return false
	}
	switch strings.ToLower(s[dot+1:]) {
	case "png", "jpg", "jpeg", "gif", "webp", "avif", "svg", "ico", "bmp":
		return true
	}
	return false
}

func (r *renderer) avatar(n *model.Node) {
	size := propNum(n, "size", 40)
	base := fmt.Sprintf("width:%gpx;height:%gpx;border-radius:50%%;overflow:hidden;flex-shrink:0;", size, size)
	if src := r.interp(propStr(n, "src")); src != "" {
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
		fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, attrID(n.ID), style, r.a11y(n), svg)
		return
	}
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, attrID(n.ID), style, r.a11y(n), html.EscapeString(name))
}

// chart renders a bar / line / area / sparkline as inline SVG. Data is a bound
// array ("{{state.series}}") or a literal number array in the `data` prop.
func (r *renderer) chart(n *model.Node) {
	vals := r.chartData(n)
	s := r.resolveStyle(r.effectiveStyle(n))
	w := numOrDefault(s, "width", 240)
	h := numOrDefault(s, "height", 80)
	// The series colour becomes an SVG fill/stroke (chartBars/chartLine, which
	// entity-encode it into the quoted attribute). A presentation attribute
	// cannot be broken out of with a `;`, but it is a CSS paint value all the
	// same, so it goes through the same allowlist as every other colour —
	// keeping `url(//attacker/x.svg#p)` out of a fill.
	color := cssValueOr(propStr(n, "color"), "var(--accent)")
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
	if m := colorStr(s, "margin"); m != "" {
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
		attrID(n.ID), html.EscapeString(r.interp(propStr(n, "src"))), r.boxCSS(n))
}

// webview embeds live web content: an <iframe> on the HTML renderer (the
// canvaswebview native build covers the same node with a real WKWebView
// overlay; the pure-Go canvas draws a placeholder). url wins over src, and
// inline `html` (srcdoc) is used only when neither is set. An author URL goes
// through safeURL's scheme allowlist, like a link href — a "javascript:" src
// would run in the iframe's origin-less context but still degrades to "#".
func (r *renderer) webview(n *model.Node) {
	src := r.interp(propStr(n, "url"))
	if src == "" {
		src = r.interp(propStr(n, "src"))
	}
	if src == "" {
		if markup := r.interp(propStr(n, "html")); markup != "" {
			fmt.Fprintf(&r.sb, `<iframe id=%q srcdoc=%q style=%q></iframe>`,
				attrID(n.ID), html.EscapeString(markup), r.boxCSS(n))
			return
		}
		// Our own constant, not author data — it must NOT go through safeURL
		// (whose scheme allowlist would rewrite "about:" to "#", an iframe
		// that reloads the parent page recursively).
		src = "about:blank"
	} else {
		src = safeURL(src)
	}
	fmt.Fprintf(&r.sb, `<iframe id=%q src=%q style=%q></iframe>`,
		attrID(n.ID), html.EscapeString(src), r.boxCSS(n))
}
