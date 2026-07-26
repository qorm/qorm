package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func counterDir() string { return filepath.Join("..", "..", "examples", "counter") }

func TestBuildSignVerifyAndRun(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.HasPrefix(b.ContentHash, "sha256:") {
		t.Fatalf("expected content hash, got %q", b.ContentHash)
	}

	pub, priv, err := keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Round-trips through marshal/unmarshal and verifies against the trusted key.
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := Verify(got, pub); err != nil {
		t.Fatalf("verify(trusted) should pass: %v", err)
	}

	// A different key must be rejected.
	otherPub, _, _ := keys.Generate()
	if err := Verify(got, otherPub); err == nil {
		t.Errorf("verify with wrong key should fail")
	}

	// The reconstructed app renders the real UI.
	html := render.Render(qrt.New(got.ToApp())).HTML
	if !strings.Contains(html, ">COUNTER<") {
		t.Errorf("bundle should reconstruct the counter UI")
	}
}

func TestTamperIsDetected(t *testing.T) {
	b, _ := Build(counterDir())
	pub, priv, _ := keys.Generate()
	_ = b.Sign(priv, keys.KeyID(pub))

	// Mutate content after signing: the recomputed hash no longer matches.
	b.Content.App["name"] = "Malicious Rename"
	if err := Verify(b, pub); err == nil {
		t.Fatal("tampered content must fail verification")
	}
	// Integrity-only verification also catches it.
	if err := Verify(b, nil); err == nil {
		t.Fatal("tampered content must fail integrity check")
	}
}

func TestUnsignedRequiresNoTrust(t *testing.T) {
	b, _ := Build(counterDir())
	if err := Verify(b, nil); err != nil {
		t.Errorf("unsigned bundle should pass integrity-only verify: %v", err)
	}
	pub, _, _ := keys.Generate()
	if err := Verify(b, pub); err == nil {
		t.Error("unsigned bundle must fail when a trusted key is required")
	}
}

func TestRevokedKeyIsRejected(t *testing.T) {
	b, _ := Build(counterDir())
	pub, priv, _ := keys.Generate()
	keyID := keys.KeyID(pub)
	_ = b.Sign(priv, keyID)

	// Valid signature, but the key is on the revocation list -> rejected.
	revoked := RevocationList{keyID: true}
	if err := VerifyWithRevocation(b, pub, revoked); err == nil {
		t.Fatal("bundle signed by a revoked key must be rejected")
	}
	// A different (non-revoked) key list lets it through.
	if err := VerifyWithRevocation(b, pub, RevocationList{"someoneelse": true}); err != nil {
		t.Errorf("non-revoked key should verify: %v", err)
	}
	// Revocation list parsing: both array and object forms.
	for _, form := range []string{`["` + keyID + `"]`, `{"revoked":["` + keyID + `"]}`} {
		rl, err := LoadRevocation([]byte(form))
		if err != nil || !rl[keyID] {
			t.Errorf("LoadRevocation(%s) failed: %v", form, err)
		}
	}
}

func TestFromAppRoundTrips(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	b, err := FromApp(app)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	if !strings.HasPrefix(b.ContentHash, "sha256:") {
		t.Error("expected content hash")
	}
	// The bundle reconstructs a runnable app with the same scenes + actions.
	rebuilt := b.ToApp()
	if rebuilt.EntryRoot() == nil {
		t.Fatal("rebuilt app has no entry root")
	}
	if _, ok := rebuilt.Actions["increment"]; !ok {
		t.Error("action 'increment' lost through FromApp -> ToApp")
	}
}

func TestLocalesSurviveBundle(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "i18n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.Locales) < 2 {
		t.Fatal("i18n example should load >=2 locales")
	}
	b, _ := FromApp(app)
	if b.Content.Locales["zh"]["title"] != "你好，世界" {
		t.Error("bundle content should carry zh translations")
	}
	// hash covers translations (tamper-evident)
	rebuilt := b.ToApp()
	if rebuilt.Locales["zh"]["title"] != "你好，世界" {
		t.Error("ToApp should restore locales")
	}
	if rebuilt.DefaultLocale != "en" {
		t.Errorf("defaultLocale lost through bundle, got %q", rebuilt.DefaultLocale)
	}
}

func TestRequiredCapabilitiesRoundTrip(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := b.SetRequiredCapabilities([]string{"camera", "location"}); err != nil {
		t.Fatalf("SetRequiredCapabilities: %v", err)
	}
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Requirements survive marshal/unmarshal and stay covered by the signature.
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	caps := got.RequiredCapabilities()
	if len(caps) != 2 || caps[0] != "camera" || caps[1] != "location" {
		t.Fatalf("requiredCapabilities lost in round-trip: %v", caps)
	}
	if err := Verify(got, pub); err != nil {
		t.Fatalf("signed bundle with requirements should verify: %v", err)
	}

	// Tampering with the declared requirements breaks the hash/signature —
	// an attacker cannot silently drop a requirement.
	got.Content.RequiredCapabilities = nil
	if err := Verify(got, pub); err == nil {
		t.Fatal("stripping requiredCapabilities must fail verification")
	}
}

func TestLegacyBundleWithoutRequiredCapabilities(t *testing.T) {
	// A bundle built with no requirements encodes exactly as pre-field bundles
	// did (omitempty), so old bundles keep verifying and report nil.
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "requiredCapabilities") {
		t.Fatal("bundles without requirements must not encode the field (hash compat with old bundles)")
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RequiredCapabilities() != nil {
		t.Fatalf("legacy bundle should report nil requirements, got %v", got.RequiredCapabilities())
	}
	if err := Verify(got, nil); err != nil {
		t.Fatalf("legacy bundle must still verify: %v", err)
	}
}

// componentAppDir writes an app whose component lives in its own
// components/*.json document (the cross-file definition form).
func componentAppDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("qorm.json", `{"type":"app","id":"cf","name":"CF","entry":"main"}`)
	write("components/panel.json", `{"type":"component","id":"panel",
		"props":{"title":{"type":"string","default":"DEF"}},
		"slots":{"body":{"required":true}},
		"template":{"type":"card","id":"pr","children":[
			{"type":"text","id":"pt","text":"T:{{ prop.title }}"},
			{"type":"slot","name":"body"}]}}`)
	write("scenes/main.json", `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
		{"type":"panel","id":"p1","children":[{"type":"text","id":"pb","slot":"body","text":"BODY"}]}]}}`)
	return dir
}

// TestCrossFileComponentsSurviveBundle: a standalone type:"component" document
// is bundle content in its own right — Build must carry it (it is not part of
// the manifest), ToApp must rehydrate it, and the hash must cover it. Without
// this, packaging an app whose components live in components/*.json would
// silently ship an app that renders them as unknown widgets.
func TestCrossFileComponentsSurviveBundle(t *testing.T) {
	b, err := Build(componentAppDir(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.Content.Components["panel"] == nil {
		t.Fatalf("component document dropped from bundle content: %+v", b.Content)
	}
	app := b.ToApp()
	if app.Components["panel"] == nil {
		t.Fatalf("component lost through ToApp: %+v", app.Components)
	}
	if sc := app.ComponentSchemas["panel"]; sc == nil || !sc.Slots["body"].Required {
		t.Errorf("component declaration lost through ToApp: %+v", sc)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, w := range []string{"T:DEF", "BODY"} {
		if !strings.Contains(html, w) {
			t.Errorf("bundled cross-file component did not render %q:\n%s", w, html)
		}
	}

	// FromApp takes the other route (the app is serialised back to documents
	// first); it must put the component in the SAME place, or the two routes
	// content-address the same app differently. See TestBuildAndFromAppAgreeOnHash.
	loaded, err := loader.LoadDir(componentAppDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fb, err := FromApp(loaded)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	if fb.Content.Components["panel"] == nil {
		t.Errorf("FromApp must keep a component document in the components section, got %v", fb.Content.Components)
	}
	if fb.ToApp().Components["panel"] == nil {
		t.Error("component lost through FromApp -> ToApp")
	}

	// The hash covers the component section: tampering must fail verification.
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	data, _ := Marshal(b)
	c, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	c.Content.Components["panel"]["injected"] = true
	if err := Verify(c, pub); err == nil {
		t.Error("a tampered component document must fail verification")
	}
}

// TestBundleWithoutComponentsKeepsLegacyEncoding: the components section is
// omitempty, so every bundle without cross-file components encodes — and
// therefore hashes — exactly as it did before the section existed.
func TestBundleWithoutComponentsKeepsLegacyEncoding(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.Content.Components != nil {
		t.Errorf("an app with no component documents must leave the section nil: %v", b.Content.Components)
	}
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"components"`) {
		t.Error("the empty components section must be omitted from the encoding")
	}
}
