package loader

import (
	"path/filepath"
	"strings"
	"testing"
)

// Script-file actions: actions/<id>.qs collects as a type:"action" document
// whose id is the filename and whose script is the file's full text — the
// DOM+CSS+JS separation's third layer, living in a file of its own.
func qsAppDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {"count": "number"}, "initial": {"count": 0}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root"}
}`)
	return dir
}

func TestLoadDirQSScriptFileAction(t *testing.T) {
	dir := qsAppDir(t)
	const src = "# bump the counter\nstate.count = state.count + 1\n"
	writeFile(t, filepath.Join(dir, "actions", "bump.qs"), src)

	// The raw document collect() synthesises: same shape as the JSON spelling.
	docs, err := CollectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, d := range docs {
		if DocType(d) == "action" {
			found = d
		}
	}
	if found == nil {
		t.Fatal("CollectDocs returned no action document for actions/bump.qs")
	}
	if DocID(found) != "bump" {
		t.Fatalf("doc id = %q, want %q (filename minus .qs)", DocID(found), "bump")
	}
	if found["script"] != src {
		t.Fatalf("doc script = %q, want the file's full text %q", found["script"], src)
	}
	if found["source"] != "actions/bump.qs" {
		t.Fatalf("doc source = %q, want actions/bump.qs (slashed, for the content hash)", found["source"])
	}

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
	if act.Script != src {
		t.Fatalf("Action.Script = %q, want %q", act.Script, src)
	}
}

// A .qs file outside an actions/ directory is not an action and is ignored —
// the extension alone must not pull stray files into a signed bundle.
func TestLoadDirQSOutsideActionsIgnored(t *testing.T) {
	dir := qsAppDir(t)
	writeFile(t, filepath.Join(dir, "scenes", "stray.qs"), "state.count = 1")
	writeFile(t, filepath.Join(dir, "toplevel.qs"), "state.count = 1")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Actions) != 0 {
		t.Fatalf(".qs files outside actions/ must not load as actions, got %v", app.Actions)
	}
	for _, d := range app.Diagnostics {
		t.Fatalf("unexpected diagnostic: %s", d)
	}
}

// A .qs file that does not parse is a load-time error diagnostic naming BOTH
// the file and the script line — the compile-time surface of the .qs spelling.
func TestLoadDirQSParseErrorNamesFileAndLine(t *testing.T) {
	dir := qsAppDir(t)
	writeFile(t, filepath.Join(dir, "actions", "broken.qs"), "let a = 1\nlet b = 2\nfor x in 42 {")
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
		t.Fatalf("no script diagnostic for the broken .qs action; diagnostics = %v", app.Diagnostics)
	}
	if !strings.Contains(found, "actions/broken.qs") {
		t.Fatalf("diagnostic %q must name the source file actions/broken.qs", found)
	}
	if !strings.Contains(found, "line 3") {
		t.Fatalf("diagnostic %q must name the offending script line (3)", found)
	}
}

// A .qs file and a .json action with the same id are one duplicate definition:
// the walk is lexicographic (<id>.json sorts before <id>.qs), so the JSON
// spelling wins on directory load and the conflict is an error diagnostic —
// the same rule two JSON documents follow (qorm build refuses it outright).
func TestLoadDirQSJSONSameIDConflict(t *testing.T) {
	dir := qsAppDir(t)
	writeFile(t, filepath.Join(dir, "actions", "dup.json"), `{
  "type": "action", "id": "dup",
  "script": "state.count = 1"
}`)
	writeFile(t, filepath.Join(dir, "actions", "dup.qs"), "state.count = 2")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "error:") && strings.Contains(d, `"dup"`) && strings.Contains(d, "重复定义") {
			found = true
		}
	}
	if !found {
		t.Fatalf("json+qs same-id conflict must be an error diagnostic; diagnostics = %v", app.Diagnostics)
	}
	act := app.Actions["dup"]
	if act == nil {
		t.Fatal("action dup missing")
	}
	if act.Script != "state.count = 1" {
		t.Fatalf("first definition (the .json) must win; Script = %q", act.Script)
	}
}
