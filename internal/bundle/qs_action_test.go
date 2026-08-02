package bundle

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
)

// Script-file actions (actions/<id>.qs) ride the SAME collect() walk as the
// JSON documents, so the packaged app is the reviewed app: the .qs spelling
// builds, verifies, and reconstructs the same runnable app the directory
// loader assembled.
func TestBuildQSScriptFileAction(t *testing.T) {
	const src = "state.count = state.count + 1\n"
	dir := appDir(t, map[string]string{
		"qorm.json": `{"type":"app","id":"t","entry":"main",
			"globalState":{"schema":{"count":"number"},"initial":{"count":0}}}`,
		"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root"}}`,
		"actions/bump.qs":  src,
	})
	b, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	doc := b.Content.Actions["bump"]
	if doc == nil {
		t.Fatal("bundle has no action bump")
	}
	if doc["script"] != src {
		t.Fatalf("bundled script = %q, want the .qs file's full text %q", doc["script"], src)
	}
	if doc["source"] != "actions/bump.qs" {
		t.Fatalf("bundled source = %q, want actions/bump.qs", doc["source"])
	}
	if err := Verify(b, nil); err != nil {
		t.Fatal(err)
	}
	// You sign what you tested: the app reconstructed from the bundle is the
	// app the directory loader (qorm run / CI) assembled.
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := appShape(t, b.ToApp()), appShape(t, app); got != want {
		t.Fatalf("bundle app != directory app:\nbundle: %s\ndir:    %s", got, want)
	}
	act := b.ToApp().Actions["bump"]
	if act == nil || act.Script != src {
		t.Fatalf("reconstructed action = %+v, want script %q", act, src)
	}
}

// A .qs file and a .json action defining one id is ambiguous — the directory
// loader keeps the first and packaging would keep the last, so the build
// refuses, exactly like two JSON action documents with one id.
func TestBuildQSJSONSameIDRefused(t *testing.T) {
	dir := appDir(t, map[string]string{
		"qorm.json":        `{"type":"app","id":"t","entry":"main"}`,
		"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root"}}`,
		"actions/dup.json": `{"type":"action","id":"dup","script":"state.count = 1"}`,
		"actions/dup.qs":   "state.count = 2",
	})
	_, err := Build(dir)
	if err == nil {
		t.Fatal("Build must refuse a json+qs same-id conflict")
	}
	if !strings.Contains(err.Error(), `"dup"`) {
		t.Fatalf("error %q must name the duplicated id", err)
	}
}
