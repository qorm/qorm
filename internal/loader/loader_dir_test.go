package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a test helper that writes content to path, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadDirMissingDirectory(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("loading a nonexistent directory must fail")
	}
}

func TestLoadDirEmptyDirectory(t *testing.T) {
	_, err := LoadDir(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no QORM source documents") {
		t.Fatalf("empty dir should report no documents, got %v", err)
	}
}

// TestLoadDirOnlyUnusableDocs verifies malformed JSON, non-object JSON and
// test fixtures are all filtered out, leaving "no documents" (not a crash).
func TestLoadDirOnlyUnusableDocs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.json"), "{not json at all")
	writeFile(t, filepath.Join(dir, "array.json"), `[1, 2, 3]`)
	writeFile(t, filepath.Join(dir, "fixture.json"), `{"type": "test", "id": "t1"}`)
	writeFile(t, filepath.Join(dir, "notes.txt"), `{"type": "scene"}`) // wrong extension
	_, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "no QORM source documents") {
		t.Fatalf("dir with only unusable docs should report no documents, got %v", err)
	}
}

// TestLoadDirFullApp builds a small on-disk app (manifest + scene + action +
// locales) and checks the assembled model, including BaseDir.
func TestLoadDirFullApp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{
		"type": "app", "id": "io", "name": "IO", "entry": "home",
		"globalState": {"schema": {"n": "number"}, "initial": {"n": 1}}
	}`)
	writeFile(t, filepath.Join(dir, "scenes", "home.json"), `{
		"type": "scene", "id": "home",
		"root": {"type": "column", "id": "root",
			"children": [{"type": "text", "id": "t", "text": "{{ state.n }}"}]}
	}`)
	writeFile(t, filepath.Join(dir, "actions", "inc.json"), `{
		"type": "action", "id": "inc",
		"steps": [{"type": "state.set", "path": "n", "value": "{{ n + 1 }}"}]
	}`)
	writeFile(t, filepath.Join(dir, "locales", "en.json"), `{"hello": "Hi"}`)

	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if app.BaseDir != dir {
		t.Errorf("BaseDir = %q, want %q", app.BaseDir, dir)
	}
	if app.Entry != "home" {
		t.Errorf("Entry = %q, want home", app.Entry)
	}
	if app.Scenes["home"] == nil || app.Actions["inc"] == nil {
		t.Fatalf("scene/action not loaded: scenes=%v actions=%v", app.Scenes, app.Actions)
	}
	if app.Locales["en"]["hello"] != "Hi" {
		t.Errorf("locales not loaded: %v", app.Locales)
	}
	if app.GlobalState.Schema["n"] != "number" {
		t.Errorf("schema not loaded: %v", app.GlobalState.Schema)
	}
	// A correct schema yields no type-error diagnostics.
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "type mismatch") {
			t.Errorf("unexpected type diagnostic: %s", d)
		}
	}
}

// TestCollectDocsSkipsFixturesAndNestedProjects checks every skip rule: the
// reserved directory names, type:"test" fixtures, wrong extensions and
// malformed / non-object JSON — while nested ordinary dirs ARE walked.
func TestCollectDocsSkipsFixturesAndNestedProjects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{"type": "app", "id": "c"}`)
	writeFile(t, filepath.Join(dir, "scenes", "deep", "s.json"), `{"type": "scene", "id": "s"}`)
	// Reserved directory names are never walked.
	for _, skip := range []string{"node_modules", "src", "target", "assets", ".git", "qorm_standalone"} {
		writeFile(t, filepath.Join(dir, skip, "hidden.json"), `{"type": "scene", "id": "hidden"}`)
	}
	// Fixtures and junk are filtered file-by-file.
	writeFile(t, filepath.Join(dir, "tests", "f.json"), `{"type": "test", "id": "f"}`)
	writeFile(t, filepath.Join(dir, "junk", "x.txt"), `{"type": "scene", "id": "x"}`)
	writeFile(t, filepath.Join(dir, "junk", "bad.json"), `{"unterminated": `)
	writeFile(t, filepath.Join(dir, "junk", "num.json"), `42`)

	docs, err := CollectDocs(dir)
	if err != nil {
		t.Fatalf("CollectDocs: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("want exactly app + scene docs, got %d: %v", len(docs), docs)
	}
	got := map[string]bool{}
	for _, d := range docs {
		got[asString(d["id"])] = true
	}
	if !got["c"] || !got["s"] {
		t.Fatalf("missing expected docs, got %v", got)
	}
}

// TestCollectDocsRootNamedSkipDir verifies a project whose own directory
// happens to be named like a reserved dir (e.g. .../src) still loads: only
// SUBdirectories with reserved names are skipped.
func TestCollectDocsRootNamedSkipDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	writeFile(t, filepath.Join(root, "qorm.json"), `{"type": "app", "id": "r"}`)
	docs, err := CollectDocs(root)
	if err != nil {
		t.Fatalf("CollectDocs: %v", err)
	}
	if len(docs) != 1 || asString(docs[0]["id"]) != "r" {
		t.Fatalf("root named 'src' must not be skipped, got %v", docs)
	}
}

func TestLoadLocalesVariants(t *testing.T) {
	dir := t.TempDir()
	// Valid catalog with non-string values: coerced via asString.
	writeFile(t, filepath.Join(dir, "locales", "en.json"), `{"hi": "Hello", "n": 5, "flag": true}`)
	// Ignored entries: subdirectory, wrong extension, malformed JSON, non-object.
	if err := os.MkdirAll(filepath.Join(dir, "locales", "subdir.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "locales", "readme.txt"), `{"x": "y"}`)
	writeFile(t, filepath.Join(dir, "locales", "broken.json"), `{oops`)
	writeFile(t, filepath.Join(dir, "locales", "arr.json"), `["a"]`)

	out := LoadLocales(dir)
	if out == nil {
		t.Fatal("expected a locales map")
	}
	if len(out) != 1 {
		t.Fatalf("only en.json should parse, got %v", out)
	}
	en := out["en"]
	if en["hi"] != "Hello" || en["n"] != "5" || en["flag"] != "true" {
		t.Errorf("coercion wrong: %#v", en)
	}
}

func TestLoadLocalesAbsentAndEmpty(t *testing.T) {
	if got := LoadLocales(t.TempDir()); got != nil {
		t.Errorf("no locales dir should give nil, got %v", got)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "locales", "bad.json"), `[1]`) // non-object -> skipped
	if got := LoadLocales(dir); got != nil {
		t.Errorf("only-unusable locales should give nil, got %v", got)
	}
}

func TestLoadFileScene(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.json")
	writeFile(t, path, `{
		"type": "scene", "id": "solo",
		"root": {"type": "text", "id": "t", "text": "hi"}
	}`)
	app, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if app.Entry != "solo" || app.Scenes["solo"] == nil {
		t.Fatalf("entry should become the loaded scene id: entry=%q scenes=%v", app.Entry, app.Scenes)
	}
	if app.Scenes["solo"].Text != "hi" {
		t.Errorf("root not built: %+v", app.Scenes["solo"])
	}
}

func TestLoadFileMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, path, `{"root": `)
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("malformed JSON should error with the path, got %v", err)
	}
}

func TestLoadFileMissing(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("missing file must error")
	}
}

// TestLoadFileNonSceneDoc verifies a non-scene document loads without error
// but yields no scenes (single-file mode only understands scenes).
func TestLoadFileNonSceneDoc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "act.json")
	writeFile(t, path, `{"type": "action", "id": "a"}`)
	app, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(app.Scenes) != 0 || app.Entry != "main" {
		t.Fatalf("non-scene doc: scenes=%v entry=%q", app.Scenes, app.Entry)
	}
}
