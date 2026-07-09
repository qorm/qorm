package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func offlineTestApp() *model.App {
	return &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "text", ID: "t", Text: "hi"}}},
	}}
}

// TestOfflineHTMLFetchesBundle guards the offline boot path: the HTML must fetch
// "bundle.json" — the exact artifact the packager writes next to it. A mismatch
// (it once fetched app.json) 404s and shows "app load failed" on every offline
// package (web/iOS/Android).
func TestOfflineHTMLFetchesBundle(t *testing.T) {
	html, err := OfflineHTML(runtime.New(offlineTestApp()), `{"entry":"main"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "fetch('bundle.json')") {
		t.Error("offline HTML must fetch bundle.json (the packaged artifact)")
	}
	if strings.Contains(html, "fetch('app.json')") {
		t.Error("offline HTML fetches app.json but the packager writes bundle.json")
	}
	// No update config -> no OTA machinery: boot stays identical to pre-OTA packages.
	for _, banned := range []string{"__QORM_UPDATE__", "qorm.ota.bundle", "qorm.ota.prev", "qormCheckUpdate"} {
		if strings.Contains(html, banned) {
			t.Errorf("offline HTML without an update config must not contain %q", banned)
		}
	}
}

// TestOfflineHTMLWithUpdateConfig guards the OTA boot path: with --update-url
// the HTML must inject window.__QORM_UPDATE__ and boot through the three-level
// bundle chain (localStorage qorm.ota.bundle -> qorm.ota.prev -> the packaged
// bundle.json as the final fallback), then schedule a silent qormCheckUpdate.
func TestOfflineHTMLWithUpdateConfig(t *testing.T) {
	upd := &UpdateConfig{URL: "https://updates.example.com", App: "counter", Trust: "cHVia2V5"}
	html, err := OfflineHTML(runtime.New(offlineTestApp()), `{"entry":"main"}`, upd)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`window.__QORM_UPDATE__={"url":"https://updates.example.com","app":"counter","trust":"cHVia2V5"};`,
		`'qorm.ota.bundle'`,    // level 1: latest OTA bundle
		`'qorm.ota.prev'`,      // level 2: rollback copy
		`fetch('bundle.json')`, // level 3: the packaged artifact, still the last resort
		`qormCheckUpdate`,      // silent post-boot update check
	} {
		if !strings.Contains(html, want) {
			t.Errorf("offline HTML with update config must contain %q", want)
		}
	}
	// The OTA levels must be tried BEFORE the packaged bundle.json.
	if strings.Index(html, "qorm.ota.bundle") > strings.Index(html, "fetch('bundle.json')") {
		t.Error("boot must try the OTA localStorage levels before falling back to bundle.json")
	}
}
