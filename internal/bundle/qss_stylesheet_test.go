package bundle

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
)

// Stylesheets (styles/<id>.qss) ride the SAME collect() walk as the JSON
// documents, so the packaged app is the reviewed app: the .qss spelling
// builds, verifies, and reconstructs the same runnable app the directory
// loader assembled — and the two compile paths content-address it identically.
func TestBuildQSSStylesheet(t *testing.T) {
	const src = "button { borderRadius: 12 }\n.accent { background: #007AFF }\n"
	dir := appDir(t, map[string]string{
		"qorm.json": `{"type":"app","id":"t","entry":"main",
			"globalState":{"schema":{"count":"number"},"initial":{"count":0}}}`,
		"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root"}}`,
		"styles/app.qss":   src,
	})
	b, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	doc := b.Content.Stylesheets["app"]
	if doc == nil {
		t.Fatal("bundle has no stylesheet app")
	}
	if doc["qss"] != src {
		t.Fatalf("bundled qss = %q, want the .qss file's full text %q", doc["qss"], src)
	}
	if doc["source"] != "styles/app.qss" {
		t.Fatalf("bundled source = %q, want styles/app.qss", doc["source"])
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
	got := b.ToApp().Styles
	if len(got) != 2 || got[0].Name != "button" || got[1].Name != "accent" {
		t.Fatalf("reconstructed styles = %+v, want the button and .accent rules", got)
	}

	// The two compile paths of the SAME app must content-address it
	// identically (Build carries the collected documents, FromApp the
	// serialised ones) — and rebuilding is a fixed point.
	fb, err := FromApp(app)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	if fb.ContentHash != b.ContentHash {
		t.Fatalf("Build and FromApp content-address the same app differently\n  Build:   %s\n  FromApp: %s", b.ContentHash, fb.ContentHash)
	}
	b2, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b2.ContentHash != b.ContentHash {
		t.Fatalf("rebuild changed the content hash: %s -> %s", b.ContentHash, b2.ContentHash)
	}
}

// Two stylesheet documents defining one id is ambiguous — the directory
// loader keeps the first and packaging would keep the last, so the build
// refuses, exactly like duplicate scenes and actions.
func TestBuildStylesheetSameIDRefused(t *testing.T) {
	_, err := fromDocs([]map[string]any{
		{"type": "app", "id": "t", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "root"}},
		{"type": "stylesheet", "id": "app", "qss": ".a { fontSize: 10 }"},
		{"type": "stylesheet", "id": "app", "qss": ".a { fontSize: 99 }"},
	}, nil)
	if err == nil {
		t.Fatal("Build must refuse two stylesheet documents with one id")
	}
	if !strings.Contains(err.Error(), `"app"`) {
		t.Fatalf("error %q must name the duplicated id", err)
	}
}

// A bundle with NO stylesheets encodes exactly as before — omitempty keeps the
// content hash of every pre-stylesheet app unchanged.
func TestBuildWithoutStylesheetsHasNoStylesheetContent(t *testing.T) {
	dir := appDir(t, map[string]string{
		"qorm.json":        `{"type":"app","id":"t","entry":"main"}`,
		"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root"}}`,
	})
	b, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Content.Stylesheets != nil {
		t.Fatalf("Stylesheets = %v, want nil (omitempty) for a sheet-less app", b.Content.Stylesheets)
	}
}
