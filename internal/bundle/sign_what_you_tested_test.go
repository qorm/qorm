package bundle

// "You sign what you tested."
//
// A QORM app is compiled twice by two different pieces of code: loader.LoadDir
// assembles the app that `qorm run`, the playground, CI renders and every
// static check see, and bundle.Build assembles the artifact that is hashed,
// ed25519-signed and shipped. The signature is worth exactly as much as the
// agreement between those two, and nothing in the type system enforces it —
// they are separate walks over the same map[string]any documents.
//
// They disagreed. On a duplicate component name the loader kept the FIRST
// definition and the bundle map assignment kept the LAST, so adding one
// components/z-panel.json redefining an existing component rendered the benign
// version in every place a human or a check could look, and signed the other
// one into the shipped bundle. Signing was never bypassed; the object that was
// authenticated simply was not the object that was reviewed.
//
// TestDirLoadEqualsBundleLoad below is that property, written down once for a
// batch of adversarial directory layouts.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// appDir writes a directory of source files and returns its path.
func appDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// appShape is an app reduced to the things that decide what a user sees: its
// serialised documents (sorted, so document ORDER — which the file walk picks —
// does not count as a difference) plus its message catalogs. Two apps with the
// same shape render the same UI and dispatch the same actions.
func appShape(t *testing.T, app *model.App) string {
	t.Helper()
	docs := loader.AppToDocs(app)
	sort.SliceStable(docs, func(i, j int) bool {
		key := func(d map[string]any) string { return loader.DocType(d) + "\x00" + loader.DocID(d) }
		return key(docs[i]) < key(docs[j])
	})
	blob, err := json.Marshal(map[string]any{"docs": docs, "locales": app.Locales})
	if err != nil {
		t.Fatalf("marshal app shape: %v", err)
	}
	return string(blob)
}

// TestDirLoadEqualsBundleLoad: for every source layout, the app you RUN and the
// app you SHIP are the same app — or neither exists. A layout whose meaning is
// ambiguous must be refused by BOTH paths (the bundle with a hard error, the
// directory load with an error diagnostic), because "resolve it somehow" is how
// the two came to disagree in the first place.
func TestDirLoadEqualsBundleLoad(t *testing.T) {
	const manifest = `{"type":"app","id":"t","name":"T","entry":"main"}`
	const scene = `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
		{"type":"panel","id":"p1","children":[{"type":"text","id":"b","slot":"body","text":"BODY"}]}]}}`
	// The attack: a component document whose FILENAME sorts after the original.
	const good = `{"type":"component","id":"panel","slots":{"body":{"required":true}},
		"template":{"type":"card","id":"good","children":[{"type":"slot","name":"body"}]}}`
	const evil = `{"type":"component","id":"panel","slots":{"body":{"required":true}},
		"template":{"type":"card","id":"evil","children":[{"type":"slot","name":"body"}]}}`

	cases := []struct {
		name string
		// ambiguous layouts must be refused, not resolved
		ambiguous bool
		files     map[string]string
	}{
		{
			name: "component documents alongside scenes",
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene, "components/panel.json": good,
			},
		},
		{
			// A component document does not have to live in components/: the
			// walk is by document TYPE, not by path, so one dropped into
			// scenes/ must land in exactly the same place on both paths.
			name: "component document under scenes/",
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene, "scenes/panel.json": good,
			},
		},
		{
			name: "manifest-inline component",
			files: map[string]string{
				"qorm.json": `{"type":"app","id":"t","name":"T","entry":"main","components":{
					"panel":{"slots":{"body":{"required":true}},
					"template":{"type":"card","id":"good","children":[{"type":"slot","name":"body"}]}}}}`,
				"scenes/main.json": scene,
			},
		},
		{
			name:      "SAME component redefined by a later-sorting file",
			ambiguous: true,
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene,
				"components/a-panel.json": good,
				"components/z-panel.json": evil,
			},
		},
		{
			name:      "component document shadowing a manifest-inline component",
			ambiguous: true,
			files: map[string]string{
				"qorm.json": `{"type":"app","id":"t","name":"T","entry":"main","components":{
					"panel":{"template":{"type":"card","id":"good"}}}}`,
				"scenes/main.json":      scene,
				"components/panel.json": evil,
			},
		},
		{
			name:      "two manifests",
			ambiguous: true,
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene,
				"z-qorm.json": `{"type":"app","id":"t","name":"Evil","entry":"main"}`,
			},
		},
		{
			name:      "two scenes with one id",
			ambiguous: true,
			files: map[string]string{
				"qorm.json": manifest, "scenes/a.json": scene,
				"scenes/z.json": `{"type":"scene","id":"main","root":{"type":"text","id":"evil","text":"EVIL"}}`,
			},
		},
		{
			name:      "two actions with one id",
			ambiguous: true,
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene, "components/panel.json": good,
				"actions/a.json": `{"type":"action","id":"go","steps":[{"type":"navigate","to":"main"}]}`,
				"actions/z.json": `{"type":"action","id":"go","steps":[{"type":"navigate","to":"elsewhere"}]}`,
			},
		},
		{
			// A NON-STRING id: the loader coerced it to "1", the bundle's type
			// assertion produced "", and the component vanished from the
			// package while still rendering under `qorm run`.
			name: "non-string component id",
			files: map[string]string{
				"qorm.json": manifest,
				"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
					{"type":"1","id":"n1"}]}}`,
				"components/one.json": `{"type":"component","id":1,"template":{"type":"card","id":"numbered"}}`,
			},
		},
		{
			// Catalogs are read by LoadLocales on both paths and must reach
			// the app identically.
			name: "locales",
			files: map[string]string{
				"qorm.json": manifest, "scenes/main.json": scene, "components/panel.json": good,
				"locales/en.json": `{"hello":"Hello"}`,
				"locales/fr.json": `{"hello":"Bonjour"}`,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := appDir(t, c.files)

			app, loadErr := loader.LoadDir(dir)
			b, buildErr := Build(dir)

			if c.ambiguous {
				if buildErr == nil {
					t.Fatalf("an ambiguous layout must not be packaged (hash %s)", b.ContentHash)
				}
				if loadErr != nil {
					t.Fatalf("the directory load should still produce an app to diagnose: %v", loadErr)
				}
				found := false
				for _, d := range app.Diagnostics {
					if strings.HasPrefix(d, "error:") && strings.Contains(d, "被重复定义") {
						found = true
					}
				}
				if !found {
					t.Errorf("the directory path must diagnose the same ambiguity as an error: %v", app.Diagnostics)
				}
				return
			}

			if loadErr != nil || buildErr != nil {
				t.Fatalf("load=%v build=%v", loadErr, buildErr)
			}
			if got, want := appShape(t, b.ToApp()), appShape(t, app); got != want {
				t.Errorf("the packaged app differs from the loaded app\n run:  %s\n ship: %s", want, got)
			}
			if !reflect.DeepEqual(b.ToApp().Locales, app.Locales) {
				t.Errorf("locales differ: run %v ship %v", app.Locales, b.ToApp().Locales)
			}
		})
	}
}

// TestBuildAndFromAppAgreeOnHash: the two ways to compile the SAME app must
// content-address it identically. Build carries the raw source documents;
// FromApp carries the ones the serializer writes back for a live (possibly
// agent-patched) app. Any structural difference between them — components
// folded into the manifest by one route and kept as documents by the other —
// makes contentHash a value that disagrees with itself, so an MCP export and a
// CI package of one tree can never be deduplicated, pinned or audited against
// each other.
//
// The premise is a CANONICAL source tree: one that is already a fixed point of
// the serializer. That is the strongest statement available, because the model
// is deliberately lossy about things it does not interpret (a window icon path,
// an empty string value), so a non-canonical tree cannot round-trip byte for
// byte by construction. canonicalize() below produces the premise from any app,
// which is also how a `qorm build` of an agent-exported design behaves.
func TestBuildAndFromAppAgreeOnHash(t *testing.T) {
	sources := map[string]map[string]string{
		"cross-file component": {
			"qorm.json": `{"type":"app","id":"cf","name":"CF","entry":"main"}`,
			"components/panel.json": `{"type":"component","id":"panel",
				"props":{"title":{"type":"string","default":"DEF"}},
				"slots":{"body":{"required":true}},
				"template":{"type":"card","id":"pr","children":[
					{"type":"text","id":"pt","text":"T:{{ prop.title }}"},
					{"type":"slot","name":"body"}]}}`,
			"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
				{"type":"panel","id":"p1","children":[{"type":"text","id":"pb","slot":"body","text":"BODY"}]}]}}`,
		},
		"inline component": {
			"qorm.json": `{"type":"app","id":"in","name":"IN","entry":"main",
				"components":{"panel":{"type":"card","id":"pr"}}}`,
			"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"panel","id":"p1"}}`,
		},
		"no components": {
			"qorm.json": `{"type":"app","id":"pl","name":"PL","entry":"main",
				"globalState":{"schema":{"n":"number"},"initial":{"n":1}}}`,
			"scenes/main.json": `{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"{{ state.n }}"}}`,
			"actions/inc.json": `{"type":"action","id":"inc","steps":[{"type":"state.set","path":"n","value":"{{ n + 1 }}"}]}`,
			"locales/en.json":  `{"hello":"Hello"}`,
		},
	}
	for name, files := range sources {
		t.Run(name, func(t *testing.T) {
			dir := canonicalize(t, appDir(t, files))
			b, err := Build(dir)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			app, err := loader.LoadDir(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			fb, err := FromApp(app)
			if err != nil {
				t.Fatalf("FromApp: %v", err)
			}
			if b.ContentHash != fb.ContentHash {
				bj, _ := json.Marshal(b.Content)
				fj, _ := json.Marshal(fb.Content)
				t.Errorf("Build and FromApp content-address the same app differently\n  Build:   %s\n  FromApp: %s", bj, fj)
			}
		})
	}
}

// canonicalize rewrites an app directory into the serializer's own spelling of
// it (the fixed point of AppToDocs∘FromDocs), preserving locales. Document file
// names are irrelevant to both compile paths — they key by type and id — so the
// documents are simply numbered.
func canonicalize(t *testing.T, dir string) string {
	t.Helper()
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	files := map[string]string{}
	for i, doc := range loader.AppToDocs(app) {
		blob, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		name := "qorm.json"
		if loader.DocType(doc) != "app" {
			name = filepath.Join("docs", loader.DocType(doc)+"-"+string(rune('a'+i))+".json")
		}
		files[name] = string(blob)
	}
	for lang, cat := range app.Locales {
		blob, err := json.Marshal(cat)
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		files["locales/"+lang+".json"] = string(blob)
	}
	return appDir(t, files)
}

// TestNonStringDocIDsSurvivePackaging is the narrow unit behind the
// "non-string component id" case above: the bundle keys its content with the
// loader's own id coercion, so a document the loader registers can never be
// silently dropped from the package.
func TestNonStringDocIDsSurvivePackaging(t *testing.T) {
	b, err := fromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": float64(7), "root": map[string]any{"type": "text", "id": "t", "text": "x"}},
		{"type": "component", "id": float64(1), "template": map[string]any{"type": "card", "id": "c"}},
	}, nil)
	if err != nil {
		t.Fatalf("fromDocs: %v", err)
	}
	if b.Content.Components["1"] == nil {
		t.Errorf("a numeric component id must key the content as %q, got %v", "1", b.Content.Components)
	}
	if b.Content.Scenes["7"] == nil {
		t.Errorf("a numeric scene id must key the content as %q, got %v", "7", b.Content.Scenes)
	}
	app := b.ToApp()
	if app.Components["1"] == nil || app.Scenes["7"] == nil {
		t.Errorf("documents lost through ToApp: %v %v", app.Components, app.Scenes)
	}
}

// TestDuplicateDocumentsRefuseToPackage pins the hard failure itself, including
// the message: whoever hits it needs to know it is not their JSON that is
// malformed but their directory that is ambiguous.
func TestDuplicateDocumentsRefuseToPackage(t *testing.T) {
	base := map[string]any{"type": "app", "id": "x", "entry": "main"}
	cases := []struct {
		name string
		docs []map[string]any
		want string
	}{
		{"manifest", []map[string]any{base, {"type": "app", "id": "y"}}, "more than one"},
		{"scene", []map[string]any{base,
			{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "a"}},
			{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "b"}}}, `two scene documents define the id "main"`},
		{"action", []map[string]any{base,
			{"type": "action", "id": "go"}, {"type": "action", "id": "go"}}, `two action documents define the id "go"`},
		{"component", []map[string]any{base,
			{"type": "component", "id": "panel", "template": map[string]any{"type": "card", "id": "a"}},
			{"type": "component", "id": "panel", "template": map[string]any{"type": "card", "id": "b"}}}, `two component documents define the id "panel"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := fromDocs(c.docs, nil)
			if err == nil {
				t.Fatal("a duplicate definition must refuse to package")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error must name the conflict %q, got %v", c.want, err)
			}
		})
	}
}
