package bundle

import (
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
