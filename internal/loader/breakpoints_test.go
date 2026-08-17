package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBreakpoints(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "scenes"), 0o755)
	os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(`{
  "type": "app",
  "id": "bp_test",
  "name": "Breakpoints",
  "entry": "main",
  "breakpoints": { "md": 800, "lg": 1200 },
  "globalState": { "schema": {}, "initial": {} }
}`), 0o644)
	os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(`{
  "type": "scene", "id": "main",
  "root": { "type": "text", "id": "t", "text": "hi" }
}`), 0o644)

	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if app.BreakpointWidths["md"] != 800 || app.BreakpointWidths["lg"] != 1200 {
		t.Fatalf("breakpoints: %#v", app.BreakpointWidths)
	}
}
