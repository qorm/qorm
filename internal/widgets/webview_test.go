package widgets

// The webview widget is host-independent: Measure/Record and the source
// resolution are pure canvas-registry behaviour, so they are tested here
// against the headless surface. The WKWebView overlay itself lives in the
// canvaswebview-tagged cmd/qorm host and is verified on a real machine.

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func webViewEngine(t *testing.T, n *model.Node) (*canvas.Engine, *canvas.HeadlessSurface) {
	t.Helper()
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(400, 300))
}

func TestWebViewRegistered(t *testing.T) {
	if _, ok := canvas.LookupWidget("webview"); !ok {
		t.Fatal("webview must be registered via this package's init")
	}
}

func TestWebViewMeasure(t *testing.T) {
	w, ok := canvas.LookupWidget("webview")
	if !ok {
		t.Fatal("webview not registered")
	}
	// Default: the classic 320×240 iframe box, scaled.
	mw, mh := w.Measure(&model.Node{Type: "webview"}, nil, nil, 1)
	if mw != 320 || mh != 240 {
		t.Errorf("default measure = %dx%d, want 320x240", mw, mh)
	}
	if mw, mh = w.Measure(&model.Node{Type: "webview"}, nil, nil, 2); mw != 640 || mh != 480 {
		t.Errorf("scale-2 measure = %dx%d, want 640x480", mw, mh)
	}
	// Explicit style size wins.
	n := &model.Node{Type: "webview", Style: map[string]any{"width": float64(200), "height": float64(100)}}
	if mw, mh = w.Measure(n, nil, nil, 1); mw != 200 || mh != 100 {
		t.Errorf("styled measure = %dx%d, want 200x100", mw, mh)
	}
}

func TestWebViewSource(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column"}}})
	rt.State["u"] = "https://example.com"

	// url wins over src; bindings evaluate.
	url, markup := WebViewSource(&model.Node{Type: "webview", Props: map[string]any{
		"url": "{{state.u}}", "src": "https://ignored.example", "html": "<p>ignored</p>",
	}}, rt)
	if url != "https://example.com" || markup != "" {
		t.Errorf("url precedence: got url=%q markup=%q", url, markup)
	}
	// src is the alias when url is absent.
	if url, _ = WebViewSource(&model.Node{Type: "webview", Props: map[string]any{"src": "https://src.example"}}, rt); url != "https://src.example" {
		t.Errorf("src alias: got %q", url)
	}
	// html only when no url/src.
	if url, markup = WebViewSource(&model.Node{Type: "webview", Props: map[string]any{"html": "<p>hi</p>"}}, rt); url != "" || markup != "<p>hi</p>" {
		t.Errorf("html fallback: got url=%q markup=%q", url, markup)
	}
	// nothing set → both empty (host targets about:blank).
	if url, markup = WebViewSource(&model.Node{Type: "webview"}, rt); url != "" || markup != "" {
		t.Errorf("empty source: got url=%q markup=%q", url, markup)
	}
}

func TestWebViewPlaceholderRenders(t *testing.T) {
	n := &model.Node{Type: "webview", ID: "wv", Props: map[string]any{"url": "https://example.com"}}
	e, surf := webViewEngine(t, n)
	e.DrawFrame(surf)
	img := surf.Frame()

	// The 320x240 placeholder sits at the origin: its 1px border strokes the
	// outermost pixel ring and the URL text inks the middle. Sample both.
	border := img.RGBAAt(0, 0)
	if border.A == 0 || (border.R == 255 && border.G == 255 && border.B == 255) {
		t.Errorf("border pixel at (0,0) looks empty: %+v", border)
	}
	var ink int
	for y := 100; y < 140; y++ {
		for x := 40; x < 280; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 0 && c.R < 200 && c.G < 200 && c.B < 200 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Error("no URL caption pixels in the placeholder centre")
	}
}

func TestWebViewFramesSurface(t *testing.T) {
	n := &model.Node{Type: "webview", ID: "wv", Props: map[string]any{"url": "https://example.com"}}
	e, surf := webViewEngine(t, n)
	e.DrawFrame(surf)

	frames := e.WidgetFrames("webview")
	if len(frames) != 1 {
		t.Fatalf("WidgetFrames(webview) = %d entries, want 1", len(frames))
	}
	got := frames[n]
	want := image.Rect(0, 0, 320, 240)
	if got != want {
		t.Errorf("webview frame = %v, want %v", got, want)
	}
	if len(e.WidgetFrames("badge")) != 0 {
		t.Error("WidgetFrames(badge) must be empty in a webview-only scene")
	}
}
