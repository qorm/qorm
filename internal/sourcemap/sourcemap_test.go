package sourcemap

import (
	"os"
	"path/filepath"
	"testing"
)

func writeApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("qorm.json", "{ \"type\": \"app\", \"id\": \"demo\", \"entry\": \"main\" }\n")
	must("scenes/main.json", "{ \"type\": \"scene\", \"id\": \"main\", \"root\": {\n"+
		"  \"type\": \"column\", \"id\": \"col\", \"children\": [\n"+
		"    { \"type\": \"text\", \"id\": \"hello\", \"text\": \"hi\" },\n"+
		"    { \"type\":\"button\",\"id\":\"save\", \"label\": \"Save\" }\n"+
		"  ] } }\n")
	must(".hidden/ignore.json", "{ \"id\": \"save\" }\n") // hidden dir must be skipped
	return dir
}

func TestLocate(t *testing.T) {
	dir := writeApp(t)

	loc, ok := Locate(dir, "hello")
	if !ok {
		t.Fatal("hello not located")
	}
	if loc.File != filepath.Join("scenes", "main.json") || loc.Line != 3 {
		t.Errorf("hello at %s:%d, want scenes/main.json:3", loc.File, loc.Line)
	}

	// tight spacing "id":"save" must match too, and the hidden dir must be skipped
	loc, ok = Locate(dir, "save")
	if !ok || loc.Line != 4 || loc.File != filepath.Join("scenes", "main.json") {
		t.Errorf("save at %s:%d ok=%v, want scenes/main.json:4", loc.File, loc.Line, ok)
	}

	if _, ok := Locate(dir, "nope"); ok {
		t.Error("unknown id should not be located")
	}
	if _, ok := Locate("", "hello"); ok {
		t.Error("empty baseDir (bundle) must return not-found")
	}
}

func TestLocateAll(t *testing.T) {
	all := LocateAll(writeApp(t))
	for _, id := range []string{"demo", "main", "col", "hello", "save"} {
		if _, ok := all[id]; !ok {
			t.Errorf("LocateAll missing id %q", id)
		}
	}
	if all["save"].Line != 4 { // first (non-hidden) declaration wins
		t.Errorf("save line = %d, want 4", all["save"].Line)
	}
}
