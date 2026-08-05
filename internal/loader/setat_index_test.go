package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// The JSON "index" key of a state.setAt step must reach model.Step.Index —
// the runtime no-ops the step without it (board-cell writes in games).
func TestLoadDirStateSetAtIndex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {"board": "array"}, "initial": {"board": []}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root"}
}`)
	writeFile(t, filepath.Join(dir, "actions", "mark.json"), `{
  "type": "action", "id": "mark",
  "steps": [
    {"type": "state.setAt", "path": "board", "index": "{{ state.y * 10 + state.x }}", "value": "{{ 1 }}"}
  ]
}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	steps := app.Actions["mark"].Steps
	if len(steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(steps))
	}
	if steps[0].Index != "{{ state.y * 10 + state.x }}" {
		t.Fatalf("Step.Index = %q, want the JSON index expression", steps[0].Index)
	}
}

// writeFile mirrors the helper used across this package's tests (declared in
// loader_dir_test.go); redeclared nowhere else — keep the name in sync.
var _ = os.WriteFile // (docs only: os is used by the JSON literals above)
