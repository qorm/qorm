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
}
