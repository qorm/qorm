package loader

// The collect() walk is a TRUST BOUNDARY, not a convenience: every document it
// returns is hashed into a bundle and covered by that bundle's ed25519
// signature. Whatever the walk picks up is what gets signed, shipped and run on
// a device — so what it must NOT pick up is a security property, and these
// tests are that property written down.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// symlinkOrSkip creates a symlink, skipping the test where the platform or the
// sandbox does not allow it (Windows without developer mode).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
}

// TestCollectDocsNoSymlinkEscape is the regression guard for the reported
// escape: `ln -s ~/.docker app/locales` put the developer's registry
// credentials, verbatim, into `content.locales` of a signed bundle that was
// about to be distributed. The same trick with a single .json file symlink
// pulled any readable JSON on the machine into the document set.
//
// Both routes must now come back empty, and the file route must come back as an
// ERROR: collect() has an error channel, and a build that silently drops a file
// the author thinks they included is its own kind of surprise.
func TestCollectDocsNoSymlinkEscape(t *testing.T) {
	// The "outside world" — a sibling of the app directory, standing in for
	// $HOME. It is deliberately NOT under the app root.
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "config.json"), `{"auths":{"registry":{"auth":"c3VwZXItc2VjcmV0"}}}`)
	writeFile(t, filepath.Join(outside, "secrets", "en.json"), `{"privateKey":"leaked"}`)

	t.Run("json file symlink", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
		writeFile(t, filepath.Join(dir, "scenes", "main.json"),
			`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"hi"}}`)
		symlinkOrSkip(t, filepath.Join(outside, "config.json"), filepath.Join(dir, "scenes", "stolen.json"))

		docs, err := CollectDocs(dir)
		if err == nil {
			t.Fatalf("a .json symlink out of the app tree must fail the collection, got %d docs", len(docs))
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Errorf("the error must name the cause, got %v", err)
		}
		for _, d := range docs {
			if _, leaked := d["auths"]; leaked {
				t.Fatal("the outside document was collected anyway")
			}
		}
		// And the whole app load fails closed rather than shipping a partial app.
		if _, err := LoadDir(dir); err == nil {
			t.Error("LoadDir must fail closed on an escaping symlink")
		}
	})

	t.Run("locales directory symlink", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
		writeFile(t, filepath.Join(dir, "scenes", "main.json"),
			`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"hi"}}`)
		symlinkOrSkip(t, filepath.Join(outside, "secrets"), filepath.Join(dir, "locales"))

		if loc := LoadLocales(dir); loc != nil {
			t.Fatalf("a locales directory symlinked out of the tree must load nothing, got %v", loc)
		}
		app, err := LoadDir(dir)
		if err != nil {
			t.Fatalf("the app itself must still load: %v", err)
		}
		if app.Locales != nil {
			t.Errorf("escaped catalogs must not reach the app: %v", app.Locales)
		}
	})

	t.Run("single catalog symlink", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
		writeFile(t, filepath.Join(dir, "locales", "en.json"), `{"hello":"Hello"}`)
		symlinkOrSkip(t, filepath.Join(outside, "config.json"), filepath.Join(dir, "locales", "de.json"))

		loc := LoadLocales(dir)
		if loc["en"]["hello"] != "Hello" {
			t.Fatalf("the real catalog must still load: %v", loc)
		}
		if _, escaped := loc["de"]; escaped {
			t.Errorf("a catalog symlinked out of the tree must be skipped: %v", loc["de"])
		}
	})
}

// TestCollectDocsAllowsIntraTreeSymlink is the other half of the boundary: the
// rule is CONTAINMENT, not "no symlinks". A project that organises its own
// files with a link inside its own tree keeps working — the bytes are part of
// what the author reviewed either way, so there is nothing to refuse.
func TestCollectDocsAllowsIntraTreeSymlink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
	writeFile(t, filepath.Join(dir, "shared", "panel.json"),
		`{"type":"component","id":"panel","template":{"type":"card","id":"pc"}}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"),
		`{"type":"scene","id":"main","root":{"type":"panel","id":"p"}}`)
	if err := os.MkdirAll(filepath.Join(dir, "components"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join(dir, "shared", "panel.json"), filepath.Join(dir, "components", "panel.json"))

	// The link and its target are the same document, so the app now defines
	// "panel" twice — which is a duplicate-definition error, NOT a symlink
	// error. That distinction is the point: containment let the file through.
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("an intra-tree symlink must not fail the load: %v", err)
	}
	if !compDiag(app, "error:", `组件 "panel" 被重复定义`) {
		t.Errorf("want the duplicate diagnostic, got %v", app.Diagnostics)
	}
	if app.Components["panel"] == nil {
		t.Error("the component behind the symlink was not collected")
	}
}

// TestCollectDocsSkipsLocalesAndNestedProjects: neither belongs in the parent
// app's document set. Message catalogs are typeless by design and are read by
// LoadLocales into their own bundle section — walking them here reported every
// catalog an i18n app owns as an "unknown or missing type" error, which as of
// the build gate would refuse to package the app. A nested project's documents
// belong to that app; merging them in produced duplicate ids out of nowhere and
// signed a stranger's scenes into this app's bundle.
func TestCollectDocsSkipsLocalesAndNestedProjects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"outer","entry":"main"}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"),
		`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"outer"}}`)
	writeFile(t, filepath.Join(dir, "locales", "en.json"), `{"hello":"Hello"}`)
	// A nested app: same scene id, so a merge would be caught by the duplicate
	// rule, and its own manifest would fight the outer one for `entry`.
	writeFile(t, filepath.Join(dir, "vendor-app", "qorm.json"), `{"type":"app","id":"inner","entry":"inner"}`)
	writeFile(t, filepath.Join(dir, "vendor-app", "scenes", "main.json"),
		`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"inner"}}`)

	docs, err := CollectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("want just the outer manifest + scene, got %d: %v", len(docs), docs)
	}
	for _, d := range docs {
		if DocID(d) == "inner" {
			t.Error("a nested project's manifest was collected into the parent")
		}
		if _, isCatalog := d["hello"]; isCatalog {
			t.Error("a message catalog was collected as a source document")
		}
	}
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("an i18n app with a vendored sub-app must load clean: %v", app.Diagnostics)
	}
	if app.Entry != "main" {
		t.Errorf("the nested manifest must not steal the entry: %q", app.Entry)
	}
	if app.Locales["en"]["hello"] != "Hello" {
		t.Errorf("skipping locales/ on the doc walk must not stop LoadLocales: %v", app.Locales)
	}
}

// chmodUnreadableOrSkip makes path unreadable, skipping the test when that has
// no effect (running as root, or a filesystem without permission bits).
func chmodUnreadableOrSkip(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Skipf("cannot drop permissions on %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		if _, err := os.ReadDir(path); err == nil {
			t.Skip("permissions have no effect here (running as root?)")
		}
		return
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("permissions have no effect here (running as root?)")
	}
}

// TestCollectDocsReportsFilesystemErrors: a file the walk cannot read is a
// filesystem problem, not a malformed document, and must abort the collection
// rather than quietly produce a smaller app than the author wrote. (Malformed
// JSON is the opposite case and is skipped — see TestLoadDirOnlyUnusableDocs.)
func TestCollectDocsReportsFilesystemErrors(t *testing.T) {
	t.Run("unreadable document", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
		secret := filepath.Join(dir, "scenes", "main.json")
		writeFile(t, secret, `{"type":"scene","id":"main","root":{"type":"text","id":"t"}}`)
		chmodUnreadableOrSkip(t, secret)
		if _, err := CollectDocs(dir); err == nil {
			t.Error("an unreadable .json document must abort the collection")
		}
	})
	t.Run("unreadable directory", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main"}`)
		sub := filepath.Join(dir, "scenes")
		writeFile(t, filepath.Join(sub, "main.json"), `{"type":"scene","id":"main","root":{"type":"text","id":"t"}}`)
		chmodUnreadableOrSkip(t, sub)
		if _, err := CollectDocs(dir); err == nil {
			t.Error("a directory the walk cannot enter must abort the collection")
		}
	})
	t.Run("unreadable catalog is skipped", func(t *testing.T) {
		// LoadLocales has no error channel, so here the only safe move is to
		// leave the catalog out: an app missing a translation renders keys, an
		// app that refuses to start renders nothing.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "locales", "en.json"), `{"hello":"Hello"}`)
		bad := filepath.Join(dir, "locales", "de.json")
		writeFile(t, bad, `{"hello":"Hallo"}`)
		chmodUnreadableOrSkip(t, bad)
		loc := LoadLocales(dir)
		if loc["en"]["hello"] != "Hello" {
			t.Errorf("the readable catalog must still load: %v", loc)
		}
		if _, ok := loc["de"]; ok {
			t.Errorf("an unreadable catalog must be skipped: %v", loc)
		}
	})
}

// TestDocIDAndDocTypeMatchTheLoader pins the shared coercion the bundle builder
// keys its content by: it must be the loader's own, or a document the loader
// registers can be dropped from the package (or filed under a different name).
func TestDocIDAndDocTypeMatchTheLoader(t *testing.T) {
	for _, c := range []struct {
		doc            map[string]any
		wantType, want string
	}{
		{map[string]any{"type": "scene", "id": "main"}, "scene", "main"},
		{map[string]any{"type": "component", "id": float64(1)}, "component", "1"},
		{map[string]any{"type": "action", "id": true}, "action", "true"},
		{map[string]any{}, "", ""},
	} {
		if got := DocID(c.doc); got != c.want {
			t.Errorf("DocID(%v) = %q, want %q", c.doc, got, c.want)
		}
		if got := DocType(c.doc); got != c.wantType {
			t.Errorf("DocType(%v) = %q, want %q", c.doc, got, c.wantType)
		}
		if got := asString(c.doc["id"]); got != DocID(c.doc) {
			t.Errorf("DocID drifted from the loader's own coercion: %q vs %q", DocID(c.doc), got)
		}
	}
}

// TestBoundGuardRedirectIsAnError: the loader used to wave a {{binding}}
// redirect through as "resolved at run time". The runtime never resolves it —
// it uses the string as a scene id — so the guard silently drops the navigation
// instead of sending the user to a login screen. A broken guard must be loud.
func TestBoundGuardRedirectIsAnError(t *testing.T) {
	guarded := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "home", "globalState": map[string]any{"schema": map[string]any{"user": "string"}}},
		{"type": "scene", "id": "home", "root": map[string]any{"type": "text", "id": "h", "text": "h"}},
		{"type": "scene", "id": "secret", "root": map[string]any{"type": "text", "id": "s", "text": "s"},
			"guard": map[string]any{"condition": "{{ state.user != '' }}", "redirect": "{{ state.loginScene }}"}},
	})
	if !compDiag(guarded, "error:", "redirect", "运行时不会对 redirect 求值") {
		t.Errorf("a bound redirect must be an error: %v", guarded.Diagnostics)
	}
	// A literal redirect to a real scene stays clean.
	ok := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "home", "globalState": map[string]any{"schema": map[string]any{"user": "string"}}},
		{"type": "scene", "id": "home", "root": map[string]any{"type": "text", "id": "h", "text": "h"}},
		{"type": "scene", "id": "secret", "root": map[string]any{"type": "text", "id": "s", "text": "s"},
			"guard": map[string]any{"condition": "{{ state.user != '' }}", "redirect": "home"}},
	})
	if len(ok.Diagnostics) != 0 {
		t.Errorf("a well-formed guard must still load clean: %v", ok.Diagnostics)
	}
}
