package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestAllExamplesRenderDeterministically walks every example app and asserts it
// loads, renders non-empty, and renders identically twice — guarding against
// non-deterministic output (e.g. map-iteration order) across the whole widget
// set.
func TestAllExamplesRenderDeterministically(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err != nil {
			continue // not an app dir
		}
		t.Run(e.Name(), func(t *testing.T) {
			app, err := loader.LoadDir(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			h1 := render.Render(qrt.New(app)).HTML
			h2 := render.Render(qrt.New(app)).HTML
			if len(h1) == 0 {
				t.Fatal("rendered empty")
			}
			if h1 != h2 {
				t.Error("render is non-deterministic")
			}
			if !strings.Contains(h1, "id=") {
				t.Error("rendered output has no elements")
			}
		})
	}
}
