package miniapp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func TestBuildProjectWeChat(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files := BuildProject(qrt.New(app))

	for _, want := range []string{
		"app.json", "app.js", "app.wxss", "project.config.json", "sitemap.json",
		"pages/index/index.wxml", "pages/index/index.wxss", "pages/index/index.js", "pages/index/index.json",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("project is missing %s", want)
		}
	}

	wxml := files["pages/index/index.wxml"]
	if strings.Contains(wxml, "<div") || strings.Contains(wxml, "<img ") || strings.Contains(wxml, "onclick=") {
		t.Error("WXML should not contain HTML div/img/onclick")
	}
	if !strings.Contains(wxml, "<view") {
		t.Error("WXML should render boxes as <view>")
	}
	if !strings.Contains(wxml, `bindtap="onTap"`) || !strings.Contains(wxml, `data-h="`) {
		t.Error("WXML should wire tap bindings with the handler index")
	}
	// every <image> is closed
	if strings.Count(wxml, "<image") != strings.Count(wxml, "</image>") {
		t.Error("every <image> must be closed for the WXML parser")
	}

	// the page JS must tell developers this is a static export with no runtime
	if js := files["pages/index/index.js"]; !strings.Contains(js, "static export — actions do not run in mini-programs") {
		t.Error("pages/index/index.js should carry the static-export developer notice")
	}
}

// TestWXMLValidForRichApp checks a widget-heavy app (icons, charts, varied
// widgets) produces WXML with no tokens the WXML parser rejects. The `<`-prefixed
// tokens can't appear inside a base64 data-URI (base64 has no '<'), so a literal
// hit means real leftover markup.
func TestWXMLValidForRichApp(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "gallery"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	wxml := BuildProject(qrt.New(app))["pages/index/index.wxml"]
	for _, bad := range []string{"<svg", "<rect", "<path", "<polyline", "<polygon", "<div", "<img ", "role=", "aria-", "onclick="} {
		if strings.Contains(wxml, bad) {
			t.Errorf("WXML still contains invalid token %q", bad)
		}
	}
	// inline SVG should have become data-URI images
	if !strings.Contains(wxml, "data:image/svg+xml;base64,") {
		t.Error("expected inline SVG (icons/charts) to convert to data-URI <image>")
	}
}
