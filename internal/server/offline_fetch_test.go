package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// TestOfflineHTMLFetchesBundle guards the offline boot path: the HTML must fetch
// "bundle.json" — the exact artifact the packager writes next to it. A mismatch
// (it once fetched app.json) 404s and shows "app load failed" on every offline
// package (web/iOS/Android).
func TestOfflineHTMLFetchesBundle(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "text", ID: "t", Text: "hi"}}},
	}}
	html, err := OfflineHTML(runtime.New(app), `{"entry":"main"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "fetch('bundle.json')") {
		t.Error("offline HTML must fetch bundle.json (the packaged artifact)")
	}
	if strings.Contains(html, "fetch('app.json')") {
		t.Error("offline HTML fetches app.json but the packager writes bundle.json")
	}
}
