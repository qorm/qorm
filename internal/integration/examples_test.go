package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/playcore"
	"github.com/qorm/platform/internal/render"
	qrt "github.com/qorm/platform/internal/runtime"
)

// TestExamplesRenderCleanly loads every bundled example app and renders each of
// its scenes, asserting none produces an unrecognised (data-qorm-unknown) widget
// — a regression gate so an example using a widget type the renderer doesn't
// handle (like the earlier "body" gap) is caught before it ships as a visual bug.
func TestExamplesRenderCleanly(t *testing.T) {
	root := examplesDir(t, "")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err != nil {
			continue
		}
		app, err := loader.LoadDir(dir)
		if err != nil {
			t.Errorf("example %s: load: %v", e.Name(), err)
			continue
		}
		for id := range app.Scenes {
			if u := render.RenderScene(qrt.New(app), id).Unknown; len(u) > 0 {
				t.Errorf("example %s scene %q renders unrecognised widget types: %v", e.Name(), id, u)
			}
		}
	}
}

// exampleDirErrorExemptions lists examples whose *directory-path* load
// (loader.LoadDir, the `qorm run` path) is allowed to carry error-level
// diagnostics, with the reason. Warning-level diagnostics are NEVER exempt —
// the whole examples/ tree loads warning-free and this test locks that in.
// The map is EMPTY, and that is the point: `qorm build` now refuses to package
// an app carrying error-level diagnostics, so an exemption here would be an
// example that cannot be shipped. (It last held "i18n", whose locales/*.json
// catalogs the CollectDocs walk reported as typeless documents; collect() skips
// locales/ now, since LoadLocales reads those into their own bundle section.)
var exampleDirErrorExemptions = map[string]string{}

// isErrorDiag reports whether a loader diagnostic is error-level. The loader
// prefixes hard errors with "error:"; everything else (including the
// unprefixed misconfigured-'value' / deprecated-'on' messages) renders as a
// warning in the playground diagnostics strip (see bridge.js renderDiagnostics).
func isErrorDiag(d string) bool {
	return strings.HasPrefix(strings.ToLower(d), "error:")
}

// playgroundDocs assembles an example's documents exactly the way the live
// playground's generated templates do (web_server/gen-templates.mjs):
// qorm.json first with platforms.*.window.icon stripped, then scenes/*.json,
// then actions/*.json plus actions/*.qs (a script-file action becomes the
// same {type:"action", id, script} document the loader's collect() walk
// synthesises), each dir in name order. locales/ is not included —
// the playground has no docs-array representation for message catalogs.
func playgroundDocs(t *testing.T, dir string) []map[string]any {
	t.Helper()
	readDoc := func(path string) map[string]any {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return m
	}
	man := readDoc(filepath.Join(dir, "qorm.json"))
	if plats, ok := man["platforms"].(map[string]any); ok {
		for _, p := range plats {
			if pm, ok := p.(map[string]any); ok {
				if w, ok := pm["window"].(map[string]any); ok {
					delete(w, "icon")
				}
			}
		}
	}
	docs := []map[string]any{man}
	for _, sub := range []string{"scenes", "actions"} {
		ents, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		var names []string
		for _, f := range ents {
			if f.IsDir() {
				continue
			}
			if strings.HasSuffix(f.Name(), ".json") || (sub == "actions" && strings.HasSuffix(f.Name(), ".qs")) {
				names = append(names, f.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if strings.HasSuffix(name, ".qs") {
				b, err := os.ReadFile(filepath.Join(dir, sub, name))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				docs = append(docs, map[string]any{
					"type": "action", "id": strings.TrimSuffix(name, ".qs"), "script": string(b),
				})
				continue
			}
			docs = append(docs, readDoc(filepath.Join(dir, sub, name)))
		}
	}
	return docs
}

// TestExamplesLoadWithZeroWarnings is the diagnostics gate for examples/: the
// canonical apps must compile with ZERO warning-level loader diagnostics on
// both load paths, and with zero diagnostics of any level on the playground
// docs path (what the live playground's template picker compiles). Error-level
// dir-path diagnostics need an entry in exampleDirErrorExemptions to pass.
//
// This locks in the 2026-07 cleanup (misconfigured-'value' false positives,
// floating's typeless actions/toggle.json). If this test fails after an edit,
// fix the example (examples are the canonical format reference — AGENTS.md),
// or, for a genuinely value-bearing new widget, extend loader.valueWidgets.
func TestExamplesLoadWithZeroWarnings(t *testing.T) {
	root := examplesDir(t, "")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err != nil {
			continue
		}

		// Directory path: qorm run / LoadDir.
		app, err := loader.LoadDir(dir)
		if err != nil {
			t.Errorf("example %s: LoadDir: %v", e.Name(), err)
			continue
		}
		for _, d := range app.Diagnostics {
			if isErrorDiag(d) {
				if reason, ok := exampleDirErrorExemptions[e.Name()]; ok {
					t.Logf("example %s: exempt error diagnostic (%s): %s", e.Name(), reason, d)
					continue
				}
				t.Errorf("example %s: LoadDir error diagnostic: %s", e.Name(), d)
				continue
			}
			t.Errorf("example %s: LoadDir warning diagnostic (must be zero): %s", e.Name(), d)
		}

		// Playground docs path: what the generated templates compile to.
		res := playcore.CompileDocs(playgroundDocs(t, dir))
		for _, d := range res.Diagnostics {
			t.Errorf("example %s: playground compile diagnostic (must be zero): %s", e.Name(), d)
		}
		for _, u := range res.Unknown {
			t.Errorf("example %s: playground compile unknown widget: %s", e.Name(), u)
		}
	}
}
