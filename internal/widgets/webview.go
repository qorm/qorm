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
	canvas.RegisterWidget("webview", WebView{})
}

// WebView embeds live web content in a scene. What shows depends on the
// host:
//
//   - canvaswebview build (darwin, `-tags canvaswebview`): the native host
//     covers the widget's box with a real WKWebView subview (a platform-view
//     overlay, located via Engine.WidgetFrames) that loads the url/src/html
//     and answers the qormDesktop JS bridge. The placeholder below still
//     renders underneath — the overlay simply occludes it.
//   - every other canvas host (pure Go, no cgo): there is no web engine to
//     embed, so the widget degrades to this placeholder — a 1px bordered box
//     with the resolved URL centred — instead of pretending or crashing.
//   - HTML renderer: an <iframe> (internal/render render_media.go).
//
// Props: `url` (or alias `src`) loads that address; `html` loads inline
// markup via loadHTMLString (used when neither url nor src is set); with no
// props the target is about:blank. Values may carry {{state.*}} bindings.
type WebView struct{}

// WebViewSource resolves what a webview node should show: url wins over src,
// and inline `html` is used only when neither is set. The native overlay host
// (cmd/qorm, canvaswebview build) resolves the same way so the WKWebView and
// the placeholder never disagree. Bindings evaluate against state.
func WebViewSource(n *model.Node, rt *runtime.Runtime) (url, markup string) {
	eval := func(raw any) string {
		return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), map[string]any{"state": rt.State})))
	}
	if raw, ok := n.Prop("url"); ok {
		url = eval(raw)
	}
	if url == "" {
		if raw, ok := n.Prop("src"); ok {
			url = eval(raw)
		}
	}
	if url == "" {
		if raw, ok := n.Prop("html"); ok {
			markup = eval(raw)
		}
	}
	return url, markup
}

// Measure reports the style width/height when set, else a 320×240 default
// (the classic iframe default), scaled to physical px. The engine's generic
// sizing still applies explicit style width/height on top (measure.go).
func (WebView) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w, h = 320*scale, 240*scale
	if v, ok := styleNumber(n, "width"); ok && v > 0 {
		w = v * scale
	}
	if v, ok := styleNumber(n, "height"); ok && v > 0 {
		h = v * scale
	}
	return
}

// Record draws the placeholder: a 1px bordered box with the resolved URL
// (about:blank when unset, "[inline html]" for an html-prop node) centred in
// it. On the canvaswebview host this sits under the real WKWebView overlay.
func (WebView) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	url, markup := WebViewSource(ln.Node, rt)
	caption := url
	if caption == "" {
		caption = "about:blank"
		if markup != "" {
			caption = "[inline html]"
		}
	}

	box := draw.NewRect()
	box.Width = float64(ln.Width)
	box.Height = float64(ln.Height)
	box.Stroke = themeColor(rt, "separator", color.RGBA{180, 180, 185, 255})
	box.StrokeWidth = float64(scale)

	fs := 12 * scale
	txtW := int(canvas.MeasureText(caption, float64(fs)))
	txtH := int(float64(fs) * 1.2)
	text := draw.NewText()
	text.Content = caption
	text.FontSize = float64(fs)
	text.Fill = themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})
	text.X = float64((ln.Width - txtW) / 2)
	text.Y = (float64(ln.Height)-float64(txtH))/2 - float64(txtH)

	// Second line: how to get the live view — without the hint the pure-Go
	// build looks broken to anyone who ran the example plain.
	hint := "run with -tags canvaswebview for the live view"
	hintW := int(canvas.MeasureText(hint, float64(fs)))
	hintText := draw.NewText()
	hintText.Content = hint
	hintText.FontSize = float64(fs)
	hintText.Fill = themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})
	hintText.X = float64((ln.Width - hintW) / 2)
	hintText.Y = (float64(ln.Height)-float64(txtH))/2 + float64(txtH)

	g := draw.NewGroup()
	g.AddChild(box)
	g.AddChild(text)
	g.AddChild(hintText)
	return g
}
