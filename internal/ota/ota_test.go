package ota

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/bundle"
	"github.com/qorm/platform/internal/keys"
	"github.com/qorm/platform/internal/model"
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

func TestFetchBlockPrivateRejectsPrivateTargets(t *testing.T) {
	// Direct fetches to private, link-local (incl. the cloud metadata
	// address), multicast and unspecified destinations must be refused before
	// any connection is made. IPv4 and IPv6 forms alike.
	sources := []string{
		"http://10.0.0.1/bundle.json",
		"http://172.16.0.1/bundle.json",
		"http://192.168.1.1/bundle.json",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata service
		"http://0.0.0.0:8080/bundle.json",
		"http://[fe80::1]/bundle.json",
		"http://[fd00::1]/bundle.json", // IPv6 ULA (private)
		"http://[::]/bundle.json",
		"http://[::ffff:10.0.0.1]/bundle.json", // IPv4-mapped IPv6
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			got, err := Fetch(src, BlockPrivate())
			if err == nil || !strings.Contains(err.Error(), "refusing") {
				t.Fatalf("BlockPrivate must refuse %s, got=%d bytes err=%v", src, len(got), err)
			}
		})
	}
}

func TestFetchBlockPrivateAllowsLoopback(t *testing.T) {
	// Loopback stays allowed under BlockPrivate: local bundle servers are the
	// normal dev workflow and loopback is reachable by any local process
	// anyway (see the BlockPrivate threat model).
	payload := []byte(`{"ok":true}`)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	got, err := Fetch(srv.URL, BlockPrivate())
	if err != nil {
		t.Fatalf("BlockPrivate must keep loopback fetches working: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("Fetch = %q, want %q", got, payload)
	}
	// Without BlockPrivate nothing changes either (historical behavior).
	if _, err := Fetch(srv.URL); err != nil {
		t.Fatalf("default Fetch must be unaffected: %v", err)
	}
}

func TestFetchBlockPrivateRejectsRedirectToPrivate(t *testing.T) {
	// A public-looking source that redirects into a private range must be
	// refused at the redirect hop — the redirect is the classic SSRF laundry.
	targets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/bundle.json",
		"ftp://internal.example/bundle.json", // non-http scheme hop
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			})
			got, err := Fetch(srv.URL, BlockPrivate())
			if err == nil || !strings.Contains(err.Error(), "refusing") {
				t.Fatalf("redirect to %s must be refused, got=%d bytes err=%v", target, len(got), err)
			}
		})
	}
	// A redirect that stays on loopback still works (dev servers redirect).
	dst := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	src := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dst.URL, http.StatusFound)
	})
	if got, err := Fetch(src.URL, BlockPrivate()); err != nil || string(got) != "ok" {
		t.Fatalf("loopback redirect should pass, got=%q err=%v", got, err)
	}
}

func TestFetchBlockPrivateRedirectCap(t *testing.T) {
	// The redirect chain is capped: an endless (or just long) chain errors
	// instead of following the default client's 10 hops.
	var srv *httptest.Server
	srv = serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+r.URL.Path+"r", http.StatusFound)
	})
	got, err := Fetch(srv.URL, BlockPrivate())
	if err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("redirect chain must hit the hop cap, got=%d bytes err=%v", len(got), err)
	}
	// A chain under the cap still resolves.
	hops := 0
	var short *httptest.Server
	short = serve(t, func(w http.ResponseWriter, r *http.Request) {
		if hops < 3 {
			hops++
			http.Redirect(w, r, short.URL, http.StatusFound)
			return
		}
		w.Write([]byte("ok"))
	})
	if got, err := Fetch(short.URL, BlockPrivate()); err != nil || string(got) != "ok" {
		t.Fatalf("a 3-hop chain should pass, got=%q err=%v", got, err)
	}
}

func TestFetchVerifiedBlockPrivatePassesOptionThrough(t *testing.T) {
	// FetchVerified forwards fetch options: a private source is refused
	// before any bundle bytes are read or verified.
	pub, _, _ := keys.Generate()
	got, err := FetchVerified("http://192.168.0.10/bundle.json", pub, nil, BlockPrivate())
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("FetchVerified must forward BlockPrivate, got=%v err=%v", got, err)
	}
	if got != nil {
		t.Error("rejected fetch must return no bundle")
	}
	// And a loopback source still verifies end to end.
	data, trusted, _, _ := signedBundle(t)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	b, err := FetchVerified(srv.URL, trusted, nil, BlockPrivate())
	if err != nil || b == nil {
		t.Fatalf("loopback FetchVerified under BlockPrivate: b=%v err=%v", b, err)
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
	// Payloads over the 32 MiB cap are a hard error naming the cap — never a
	// silent truncation (a truncated bundle must not reach the verifier).
	const cap32 = 32 << 20
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, cap32+1))
	})
	got, err := Fetch(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "32 MiB") {
		t.Fatalf("oversize payload should error naming the 32 MiB cap, got=%d bytes err=%v", len(got), err)
	}
	if got != nil {
		t.Error("an oversize fetch must return no bytes")
	}
	// Exactly at the cap is still accepted.
	srvOK := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, cap32))
	})
	got, err = Fetch(srvOK.URL)
	if err != nil || len(got) != cap32 {
		t.Errorf("a payload at the cap should pass, got=%d bytes err=%v", len(got), err)
	}
}

func TestFetchFileSizeCap(t *testing.T) {
	// The file source enforces the same 32 MiB cap as HTTP (it used to be
	// unbounded via os.ReadFile).
	const cap32 = 32 << 20
	path := filepath.Join(t.TempDir(), "big-bundle.json")
	if err := os.WriteFile(path, make([]byte, cap32+1), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Fetch(path)
	if err == nil || !strings.Contains(err.Error(), "32 MiB") {
		t.Fatalf("oversize file should error naming the 32 MiB cap, got=%d bytes err=%v", len(got), err)
	}
	if got != nil {
		t.Error("an oversize fetch must return no bytes")
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
