package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
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
