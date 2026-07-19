package ota

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/model"
)

// testApp is the smallest app that compiles into a bundle.
func testApp() *model.App {
	return &model.App{
		ID:    "otatest",
		Name:  "OTA Test",
		Entry: "home",
		Scenes: map[string]*model.Node{
			"home": {Type: "column", Children: []*model.Node{{Type: "text", Text: "hello"}}},
		},
		Actions: map[string]*model.Action{
			"noop": {ID: "noop", Steps: []model.Step{{Type: "state.set", Path: "x", Value: "1"}}},
		},
	}
}

// signedBundle returns marshaled bundle bytes plus the signing keypair and id.
func signedBundle(t *testing.T) (data []byte, pub ed25519.PublicKey, priv ed25519.PrivateKey, keyID string) {
	t.Helper()
	b, err := bundle.FromApp(testApp())
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	pub, priv, err = keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	keyID = keys.KeyID(pub)
	if err := b.Sign(priv, keyID); err != nil {
		t.Fatalf("sign: %v", err)
	}
	data, err = bundle.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data, pub, priv, keyID
}

func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchHTTP(t *testing.T) {
	payload := []byte(`{"ok":true}`)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	got, err := Fetch(srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("Fetch = %q, want %q", got, payload)
	}
}

func TestFetchHTTPStatusError(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	if _, err := Fetch(srv.URL); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Fetch should surface the HTTP status, got %v", err)
	}
}

func TestFetchHTTPUnreachable(t *testing.T) {
	// Port 1 is never listening, so the dial fails fast (this does not
	// exercise the hardcoded 30s client timeout).
	if _, err := Fetch("http://127.0.0.1:1/bundle.json"); err == nil {
		t.Error("Fetch to an unreachable host should error")
	}
}

func TestFetchFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.json")
	want := []byte(`{"local":true}`)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Fetch(path)
	if err != nil {
		t.Fatalf("Fetch(file): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Fetch(file) = %q, want %q", got, want)
	}
	if _, err := Fetch(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("Fetch of a missing file should error")
	}
}

func TestFetchSizeCap(t *testing.T) {
	// HTTP responses are capped at 32 MiB; the excess is silently truncated
	// (Fetch itself does not error — integrity checking happens in Verify).
	const cap32 = 32 << 20
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, cap32+1))
	})
	got, err := Fetch(srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != cap32 {
		t.Errorf("Fetch returned %d bytes, want the %d-byte cap", len(got), cap32)
	}
}

func TestFetchVerifiedActivatesValidBundle(t *testing.T) {
	data, pub, _, _ := signedBundle(t)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	b, err := FetchVerified(srv.URL, pub, nil)
	if err != nil {
		t.Fatalf("FetchVerified: %v", err)
	}
	if b == nil || !strings.HasPrefix(b.ContentHash, "sha256:") {
		t.Fatalf("activated bundle has no content hash: %+v", b)
	}
	// The activated bundle reconstructs a runnable app.
	if b.ToApp().EntryRoot() == nil {
		t.Error("activated bundle must reconstruct an entry scene")
	}
}

func TestFetchVerifiedRejectsTampered(t *testing.T) {
	data, pub, _, _ := signedBundle(t)

	// Semantic tamper: valid JSON, stale hash/signature.
	b, err := bundle.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b.Content.App["name"] = "Evil Rename"
	tampered, err := bundle.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Byte-flip tamper: corrupt the raw payload itself.
	flipped := append([]byte(nil), data...)
	flipped[len(flipped)/2] ^= 0xff

	for name, payload := range map[string][]byte{"semantic": tampered, "byte-flip": flipped} {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
			got, err := FetchVerified(srv.URL, pub, nil)
			if err == nil {
				t.Fatal("tampered bundle must be rejected")
			}
			// Rollback by inaction: a rejected fetch must yield no bundle.
			if got != nil {
				t.Error("rejected fetch must return no bundle")
			}
		})
	}
}

func TestFetchVerifiedUnsignedVsTrust(t *testing.T) {
	b, err := bundle.FromApp(testApp())
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	data, err := bundle.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })

	// With no trusted key, an unsigned bundle passes the integrity check...
	got, err := FetchVerified(srv.URL, nil, nil)
	if err != nil || got == nil {
		t.Fatalf("unsigned bundle with nil trust should activate: got=%v err=%v", got, err)
	}
	// ...but requiring a trusted key must reject it, returning nothing.
	pub, _, _ := keys.Generate()
	got, err = FetchVerified(srv.URL, pub, nil)
	if err == nil {
		t.Fatal("unsigned bundle must be rejected when a trusted key is required")
	}
	if got != nil {
		t.Error("rejected fetch must return no bundle")
	}
}

func TestFetchVerifiedRejectsWrongKey(t *testing.T) {
	data, _, _, _ := signedBundle(t)
	otherPub, _, _ := keys.Generate()
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	got, err := FetchVerified(srv.URL, otherPub, nil)
	if err == nil {
		t.Fatal("bundle signed by a different key must be rejected")
	}
	if got != nil {
		t.Error("rejected fetch must return no bundle")
	}
}

func TestFetchVerifiedRejectsRevokedKey(t *testing.T) {
	data, pub, _, keyID := signedBundle(t)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	got, err := FetchVerified(srv.URL, pub, bundle.RevocationList{keyID: true})
	if err == nil {
		t.Fatal("bundle signed by a revoked key must be rejected")
	}
	if got != nil {
		t.Error("rejected fetch must return no bundle")
	}
}

func TestFetchVerifiedRejectsUndecodable(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("this is not a bundle")) })
	pub, _, _ := keys.Generate()
	for _, trust := range []ed25519.PublicKey{nil, pub} {
		got, err := FetchVerified(srv.URL, trust, nil)
		if err == nil {
			t.Error("undecodable payload must be rejected")
		}
		if got != nil {
			t.Error("rejected fetch must return no bundle")
		}
	}
	// Valid JSON with the wrong format id is a decode rejection too.
	srv2 := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"format":"other/1"}`)) })
	if _, err := FetchVerified(srv2.URL, nil, nil); err == nil {
		t.Error("unsupported format must be rejected")
	}
}

func TestFetchVerifiedFileSource(t *testing.T) {
	data, pub, _, _ := signedBundle(t)
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	b, err := FetchVerified(path, pub, nil)
	if err != nil {
		t.Fatalf("FetchVerified(file): %v", err)
	}
	if b == nil || b.ToApp().EntryRoot() == nil {
		t.Error("file-sourced bundle should activate and reconstruct")
	}

	// A tampered file on disk is rejected the same way.
	bad, err := bundle.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bad.Content.App["name"] = "Tampered On Disk"
	badData, err := bundle.Marshal(bad)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, badData, 0o600); err != nil {
		t.Fatalf("write tampered fixture: %v", err)
	}
	if got, err := FetchVerified(path, pub, nil); err == nil || got != nil {
		t.Errorf("tampered file bundle must be rejected with no bundle, got=%v err=%v", got, err)
	}
}
