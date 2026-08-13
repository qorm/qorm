package loader

import (
	"path/filepath"
	"strings"
	"testing"
)

// configApp writes a minimal manifest + one scene so LoadDir has something to
// load, then overlays whatever qorm.config.json / manifest window fields the
// test supplies. It returns the dir.
func configApp(t *testing.T, manifest string, config string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), manifest)
	writeFile(t, filepath.Join(dir, "scenes", "home.json"), `{
		"type": "scene", "id": "home",
		"root": {"type": "column", "id": "root", "children": []}
	}`)
	if config != "" {
		writeFile(t, filepath.Join(dir, "qorm.config.json"), config)
	}
	return dir
}

const plainManifest = `{
	"type": "app", "id": "cfg", "name": "Cfg", "entry": "home",
	"globalState": {"schema": {}, "initial": {}}
}`

func TestLoadConfigWindowBlock(t *testing.T) {
	dir := configApp(t, plainManifest, `{
		"window": {
			"width": 1024, "height": 480, "title": "Raiden",
			"resizable": false,
			"chromeless": true, "transparent": true,
			"hideLog": true, "hideTray": true
		}
	}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	w := app.Window
	if w.Width != 1024 || w.Height != 480 {
		t.Errorf("size = %dx%d, want 1024x480", w.Width, w.Height)
	}
	if w.Title != "Raiden" {
		t.Errorf("title = %q, want Raiden", w.Title)
	}
	if !w.Fixed || w.Resizable {
		t.Errorf("resizable:false must set Fixed (Fixed=%v Resizable=%v)", w.Fixed, w.Resizable)
	}
	if !w.Chromeless || !w.Transparent || !w.HideLog || !w.HideTray {
		t.Errorf("chrome flags not all applied: %+v", w)
	}
}

func TestLoadConfigDisplayLegacy(t *testing.T) {
	dir := configApp(t, plainManifest, `{
		"display": {"width": 500, "height": 700, "title": "Legacy", "resizable": true}
	}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	w := app.Window
	if w.Width != 500 || w.Height != 700 || w.Title != "Legacy" {
		t.Errorf("legacy display not applied: %+v", w)
	}
	if w.Fixed || !w.Resizable {
		t.Errorf("resizable:true must not set Fixed (Fixed=%v Resizable=%v)", w.Fixed, w.Resizable)
	}
}

func TestLoadConfigWindowWinsOverDisplay(t *testing.T) {
	// Within qorm.config.json a `window` block present beside `display` wins.
	dir := configApp(t, plainManifest, `{
		"display": {"width": 100, "height": 100, "title": "Display"},
		"window":  {"width": 200, "height": 200, "title": "Window"}
	}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if app.Window.Title != "Window" || app.Window.Width != 200 {
		t.Errorf("window block must win over display: %+v", app.Window)
	}
}

func TestLoadConfigWinsOverManifest(t *testing.T) {
	// qorm.config.json beats both the manifest's top-level display and
	// platforms.desktop.window.
	manifest := `{
		"type": "app", "id": "cfg", "name": "Cfg", "entry": "home",
		"globalState": {"schema": {}, "initial": {}},
		"display": {"width": 111, "height": 111, "title": "TopLevel"},
		"platforms": {"desktop": {"window": {"width": 222, "height": 222, "title": "Desktop", "chromeless": true}}}
	}`
	dir := configApp(t, manifest, `{"window": {"width": 333, "height": 333, "title": "Config"}}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if app.Window.Width != 333 || app.Window.Title != "Config" {
		t.Errorf("qorm.config.json must win over manifest: %+v", app.Window)
	}
	// A chrome flag the manifest set but the config did not override survives.
	if !app.Window.Chromeless {
		t.Errorf("manifest chromeless should survive when config doesn't override it: %+v", app.Window)
	}
}

func TestManifestWindowPrecedence(t *testing.T) {
	// No config file: top-level display seeds size/title; desktop.window fills
	// only what display left zero and owns the chrome flags.
	manifest := `{
		"type": "app", "id": "cfg", "name": "Cfg", "entry": "home",
		"globalState": {"schema": {}, "initial": {}},
		"display": {"width": 111, "height": 111, "title": "TopLevel"},
		"platforms": {"desktop": {"window": {"width": 222, "height": 222, "title": "Desktop", "transparent": true}}}
	}`
	dir := configApp(t, manifest, "")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if app.Window.Width != 111 || app.Window.Title != "TopLevel" {
		t.Errorf("top-level display must win for size/title: %+v", app.Window)
	}
	if !app.Window.Transparent {
		t.Errorf("desktop.window chrome flag must apply: %+v", app.Window)
	}
}

func TestLoadConfigMalformedJSON(t *testing.T) {
	dir := configApp(t, plainManifest, `{ not valid json`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("a broken qorm.config.json must not fail the load: %v", err)
	}
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "qorm.config.json") && strings.Contains(d, "not valid JSON") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a diagnostic about the broken config, got %v", app.Diagnostics)
	}
}

func TestLoadConfigUnknownKeys(t *testing.T) {
	dir := configApp(t, plainManifest, `{
		"window": {"width": 100, "heigh": 200},
		"bogus": {"x": 1}
	}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	var diags string
	for _, d := range app.Diagnostics {
		diags += d + "\n"
	}
	if !strings.Contains(diags, `"heigh"`) {
		t.Errorf("unknown window key should be diagnosed, got %q", diags)
	}
	if !strings.Contains(diags, `"bogus"`) {
		t.Errorf("unknown top-level block should be diagnosed, got %q", diags)
	}
}

func TestWindowDefaultResizable(t *testing.T) {
	// No resizable declared anywhere → window stays resizable (Fixed=false).
	dir := configApp(t, plainManifest, `{"window": {"width": 100, "height": 100}}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if app.Window.Fixed {
		t.Errorf("absent resizable must not set Fixed: %+v", app.Window)
	}
}
