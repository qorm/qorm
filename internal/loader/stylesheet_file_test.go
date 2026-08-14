package loader

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// Stylesheets: styles/<id>.qss collects as a type:"stylesheet" document whose
// id is the filename and whose qss is the file's full text — the style leg of
// the DOM+CSS+JS separation, living in a file of its own.
func qssAppDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
  "type": "app", "id": "t", "entry": "main",
  "globalState": {"schema": {"count": "number"}, "initial": {"count": 0}}
}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
  "type": "scene", "id": "main",
  "root": {"type": "column", "id": "root", "class": "page", "children": [
    {"type": "button", "id": "go", "label": "Go", "class": "accent big"}
  ]}
}`)
	return dir
}

func TestLoadDirStylesheetFile(t *testing.T) {
	dir := qssAppDir(t)
	const src = "# shared styles\nbutton { borderRadius: 12 }\n.accent { background: #007AFF }\n#go { height: 44 }\n"
	writeFile(t, filepath.Join(dir, "styles", "app.qss"), src)

	// The raw document collect() synthesises.
	docs, err := CollectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, d := range docs {
		if DocType(d) == "stylesheet" {
			found = d
		}
	}
	if found == nil {
		t.Fatal("CollectDocs returned no stylesheet document for styles/app.qss")
	}
	if DocID(found) != "app" {
		t.Fatalf("doc id = %q, want %q (filename minus .qss)", DocID(found), "app")
	}
	if found["qss"] != src {
		t.Fatalf("doc qss = %q, want the file's full text %q", found["qss"], src)
	}
	if found["source"] != "styles/app.qss" {
		t.Fatalf("doc source = %q, want styles/app.qss (slashed, for the content hash)", found["source"])
	}

	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Fatalf("unexpected diagnostic: %s", d)
	}
	if len(app.Stylesheets) != 1 || app.Stylesheets[0].ID != "app" || app.Stylesheets[0].QSS != src {
		t.Fatalf("Stylesheets = %+v, want the one authored sheet", app.Stylesheets)
	}
	want := []struct {
		kind, name string
	}{
		{model.StyleRuleType, "button"},
		{model.StyleRuleClass, "accent"},
		{model.StyleRuleID, "go"},
	}
	if len(app.Styles) != len(want) {
		t.Fatalf("Styles = %v, want %d rules", app.Styles, len(want))
	}
	for i, w := range want {
		if app.Styles[i].Kind != w.kind || app.Styles[i].Name != w.name {
			t.Errorf("Styles[%d] = {%q %q}, want {%q %q}", i, app.Styles[i].Kind, app.Styles[i].Name, w.kind, w.name)
		}
	}
	if got := app.Styles[0].Style["borderRadius"]; got != float64(12) {
		t.Errorf("borderRadius = %v (%T), want float64(12)", got, got)
	}
}

// Rule order is the collect walk's file order, then declaration order within
// each file — the cascade's declaration order.
func TestStylesheetRuleOrderAcrossFiles(t *testing.T) {
	dir := qssAppDir(t)
	writeFile(t, filepath.Join(dir, "styles", "a.qss"), ".x { fontSize: 10 }\n.x { fontWeight: 700 }\n")
	writeFile(t, filepath.Join(dir, "styles", "b.qss"), ".x { fontSize: 20 }\n")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var sizes []any
	for _, r := range app.Styles {
		if r.Kind == model.StyleRuleClass && r.Name == "x" {
			if v, ok := r.Style["fontSize"]; ok {
				sizes = append(sizes, v)
			}
		}
	}
	if len(sizes) != 2 || sizes[0] != float64(10) || sizes[1] != float64(20) {
		t.Fatalf("class .x fontSize rules in order = %v, want [10 20] (a.qss before b.qss)", sizes)
	}
}

// A .qss file outside a styles/ directory is not a stylesheet and is ignored.
func TestQSSOutsideStylesDirIgnored(t *testing.T) {
	dir := qssAppDir(t)
	writeFile(t, filepath.Join(dir, "scenes", "stray.qss"), "button { fontSize: 1 }")
	writeFile(t, filepath.Join(dir, "toplevel.qss"), "button { fontSize: 1 }")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Styles) != 0 || len(app.Stylesheets) != 0 {
		t.Fatalf(".qss files outside styles/ must not load as stylesheets, got %v", app.Styles)
	}
}

// A .qss file that does not parse is a load-time error diagnostic naming BOTH
// the file and the line — the parse-time surface of the .qss spelling. The
// rules that do parse still load.
func TestQSSParseErrorDiagnostic(t *testing.T) {
	dir := qssAppDir(t)
	writeFile(t, filepath.Join(dir, "styles", "broken.qss"), "button { fontSize: 3 }\n.accent {\nfontSize 3\n}\n.tail { height: 1 }\n")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "broken") && strings.Contains(d, "Stylesheet") {
			found = d
		}
	}
	if found == "" {
		t.Fatalf("no stylesheet diagnostic for the broken .qss; diagnostics = %v", app.Diagnostics)
	}
	if !strings.Contains(found, "styles/broken.qss") {
		t.Fatalf("diagnostic %q must name the source file styles/broken.qss", found)
	}
	if !strings.Contains(found, ":3:") {
		t.Fatalf("diagnostic %q must name the offending line (3)", found)
	}
	// The rules around the error still loaded (button, .tail).
	if len(app.Styles) != 2 {
		t.Fatalf("Styles = %v, want the two well-formed rules to survive", app.Styles)
	}
}

// Two stylesheet documents with one id are a duplicate definition: an error
// diagnostic, first wins — exactly like scenes and actions. (A directory can
// only produce one file per name, so the collision is exercised at the
// document layer — the same layer bundle.ToApp reconstructs.)
func TestStylesheetDuplicateID(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "t", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "root"}},
		{"type": "stylesheet", "id": "app", "qss": ".a { fontSize: 10 }"},
		{"type": "stylesheet", "id": "app", "qss": ".a { fontSize: 99 }"},
	})
	var found string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "样式表") {
			found = d
		}
	}
	if found == "" {
		t.Fatalf("no duplicate-stylesheet diagnostic; diagnostics = %v", app.Diagnostics)
	}
	if len(app.Styles) != 1 || app.Styles[0].Style["fontSize"] != float64(10) {
		t.Fatalf("Styles = %v, want only the first sheet's rule", app.Styles)
	}
}

// An unknown style key in a rule body is a warning (same contract as a node's
// inline style), not a silent drop.
func TestStylesheetUnknownKeyWarning(t *testing.T) {
	dir := qssAppDir(t)
	writeFile(t, filepath.Join(dir, "styles", "app.qss"), "button { colr: red; fontSize: 12 }\n")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "colr") {
			found = d
		}
	}
	if found == "" {
		t.Fatalf("no unknown-key warning; diagnostics = %v", app.Diagnostics)
	}
	if !strings.HasPrefix(found, "warning:") {
		t.Fatalf("unknown key must be a warning, got %q", found)
	}
}

// The serializer writes a sheet back to its own document, so FromDocs ∘
// AppToDocs is a fixed point for styles too (bundle.FromApp parity).
func TestStylesheetSerializeRoundTrip(t *testing.T) {
	dir := qssAppDir(t)
	writeFile(t, filepath.Join(dir, "styles", "app.qss"), "button { borderRadius: 12 }\n.accent { background: #007AFF }\n")
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	docs := AppToDocs(app)
	var sheetDoc map[string]any
	for _, d := range docs {
		if DocType(d) == "stylesheet" {
			sheetDoc = d
		}
	}
	if sheetDoc == nil {
		t.Fatal("AppToDocs emitted no stylesheet document")
	}
	app2 := FromDocs(docs)
	if len(app2.Stylesheets) != 1 || app2.Stylesheets[0] != app.Stylesheets[0] {
		t.Fatalf("Stylesheets after round trip = %+v, want %+v", app2.Stylesheets, app.Stylesheets)
	}
	if len(app2.Styles) != len(app.Styles) {
		t.Fatalf("Styles after round trip = %v, want %v", app2.Styles, app.Styles)
	}
	for i := range app.Styles {
		if app2.Styles[i].Kind != app.Styles[i].Kind || app2.Styles[i].Name != app.Styles[i].Name {
			t.Fatalf("Styles[%d] changed through the round trip: %+v -> %+v", i, app.Styles[i], app.Styles[i])
		}
	}
}
