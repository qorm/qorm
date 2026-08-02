package loader

import (
	"path/filepath"
	"strings"
	"testing"
)

// Script actions: the JSON "script" key compiles at load time into
// model.Action.Script; parse errors and the script+steps combination are
// load-time diagnostics, so an agent fixes the app before it ever runs.
func TestLoadDirScriptAction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {"count": "number"}, "initial": {"count": 0}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root"}
}`)
	writeFile(t, filepath.Join(dir, "actions", "bump.json"), `{
  "type": "action", "id": "bump",
  "script": "state.count = state.count + 1"
}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Fatalf("unexpected diagnostic: %s", d)
	}
	act := app.Actions["bump"]
	if act == nil {
		t.Fatal("action bump missing")
	}
	if act.Script != "state.count = state.count + 1" {
		t.Fatalf("Action.Script = %q, want the source stored", act.Script)
	}
	if len(act.Steps) != 0 {
		t.Fatalf("a pure script action must have no steps, got %d", len(act.Steps))
	}
}

// A script that does not parse is a load-time error diagnostic naming the
// script line — the compile-time error surface for script actions.
func TestLoadDirScriptParseErrorNamesLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {}, "initial": {}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root"}
}`)
	writeFile(t, filepath.Join(dir, "actions", "broken.json"), `{
  "type": "action", "id": "broken",
  "script": "let a = 1\nlet b = 2\nfor x in 42 {"
}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "broken") && strings.Contains(d, "script") {
			found = d
		}
	}
	if found == "" {
		t.Fatalf("no script diagnostic for the broken action; diagnostics = %v", app.Diagnostics)
	}
	if !strings.Contains(found, "line 3") {
		t.Fatalf("diagnostic %q must name the offending script line (3)", found)
	}
}

// Declaring both script and steps is a warning (the runtime runs the script
// and ignores the steps) — not silently one or the other.
func TestLoadDirScriptAndStepsWarns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {"count": "number"}, "initial": {"count": 0}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root"}
}`)
	writeFile(t, filepath.Join(dir, "actions", "both.json"), `{
  "type": "action", "id": "both",
  "script": "state.count = 1",
  "steps": [{"type": "state.set", "path": "count", "value": "{{ 2 }}"}]
}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "both") && strings.Contains(d, "warning") && strings.Contains(d, "script") {
			found = true
		}
	}
	if !found {
		t.Fatalf("script+steps must warn; diagnostics = %v", app.Diagnostics)
	}
}
