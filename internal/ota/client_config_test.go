package ota

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
)

// clientConfigDoc builds the JSON document the packager injects as
// window.__QORM_UPDATE__ (the server.UpdateConfig shape) around a generated
// trust key, with an optional revocation array.
func clientConfigDoc(t *testing.T, revoked any) string {
	t.Helper()
	pub, _, _ := keys.Generate()
	doc := `{"url":"https://updates.example.test","app":"myapp","trust":"` + base64.StdEncoding.EncodeToString(pub) + `"`
	switch r := revoked.(type) {
	case nil:
	case []string:
		if len(r) == 0 {
			doc += `,"revoked":[]`
		} else {
			var ids []string
			for _, id := range r {
				ids = append(ids, `"`+id+`"`)
			}
			doc += `,"revoked":[` + strings.Join(ids, ",") + `]`
		}
	default:
		t.Fatalf("unexpected revoked spec %T", revoked)
	}
	return doc + `}`
}

func TestParseClientConfig(t *testing.T) {
	pub, _, _ := keys.Generate()
	good := base64.StdEncoding.EncodeToString(pub)
	tests := []struct {
		name string
		doc  string
		want string // error substring; "" means success
	}{
		{"plain config", `{"url":"https://u.test/","app":"a","trust":"` + good + `"}`, ""},
		{"trailing slash trimmed", `{"url":"https://u.test///","app":"a","trust":"` + good + `"}`, ""},
		{"empty revocation array", `{"url":"https://u.test","app":"a","trust":"` + good + `","revoked":[]}`, ""},
		{"revocation entries accepted", `{"url":"https://u.test","app":"a","trust":"` + good + `","revoked":["rev1","rev2"]}`, ""},
		{"missing url", `{"app":"a","trust":"` + good + `"}`, "needs url and app"},
		{"missing app", `{"url":"https://u.test","trust":"` + good + `"}`, "needs url and app"},
		{"missing trust key", `{"url":"https://u.test","app":"a"}`, "no valid trust key"},
		{"trust not base64", `{"url":"https://u.test","app":"a","trust":"!!!not-base64!!!"}`, "no valid trust key"},
		{"trust wrong length", `{"url":"https://u.test","app":"a","trust":"` + base64.StdEncoding.EncodeToString([]byte("short")) + `"}`, "no valid trust key"},
		{"malformed revocation", `{"url":"https://u.test","app":"a","trust":"` + good + `","revoked":"nope"}`, "revocation"},
		{"revocation non-string entry", `{"url":"https://u.test","app":"a","trust":"` + good + `","revoked":[42]}`, "revocation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseClientConfig([]byte(tt.doc))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ParseClientConfig = %v, want success", err)
				}
				if cfg.URL == "" || cfg.App == "" || len(cfg.Trust) != ed25519.PublicKeySize {
					t.Fatalf("config fields not populated: %+v", cfg)
				}
				if strings.HasSuffix(cfg.URL, "/") {
					t.Fatalf("URL should be trimmed of trailing slashes, got %q", cfg.URL)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseClientConfig error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestParseClientConfigRevocationForms(t *testing.T) {
	pub, _, _ := keys.Generate()
	good := base64.StdEncoding.EncodeToString(pub)
	// The object form bundle.LoadRevocation accepts is allowed too.
	cfg, err := ParseClientConfig([]byte(`{"url":"https://u.test","app":"a","trust":"` + good + `","revoked":{"revoked":["rev1"]}}`))
	if err != nil {
		t.Fatalf("object-form revocation must parse: %v", err)
	}
	if !cfg.Revoked["rev1"] {
		t.Fatalf("revoked list should contain rev1, got %v", cfg.Revoked)
	}
	// Absent revocation stays a nil list — no one is revoked.
	cfg, err = ParseClientConfig([]byte(`{"url":"https://u.test","app":"a","trust":"` + good + `"}`))
	if err != nil {
		t.Fatalf("plain config must parse: %v", err)
	}
	if len(cfg.Revoked) != 0 {
		t.Fatalf("no revocation declared means an empty list, got %v", cfg.Revoked)
	}
}

// TestClientConfigVerifyBundle covers the wasm OTA verify call sites: an
// OTA-origin bundle (qormInit's "ota"/"prev" levels) or a fetched one must be
// verified against the config's trust key AND its shipped revocation snapshot.
func TestClientConfigVerifyBundle(t *testing.T) {
	data, pub, _, keyID := signedBundle(t)
	cfg, err := ParseClientConfig([]byte(`{"url":"https://u.test","app":"a","trust":"` + base64.StdEncoding.EncodeToString(pub) + `"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, err := bundle.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// A key that is not revoked verifies cleanly.
	if err := cfg.VerifyBundle(b); err != nil {
		t.Fatalf("non-revoked signer must verify: %v", err)
	}
	// The same config carrying the signer's key id in its revocation snapshot
	// must refuse the bundle — the exact defense the previous nil revoked
	// list never provided.
	cfg.Revoked = bundle.RevocationList{keyID: true}
	err = cfg.VerifyBundle(b)
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked signer must be refused, got %v", err)
	}
	// A config without a trust key fails closed, as does a nil config
	// (window.__QORM_UPDATE__ missing on an OTA boot level).
	keyless := &ClientConfig{}
	if keyless.VerifyBundle(b) == nil {
		t.Fatal("keyless config must not verify a bundle")
	}
	var nilCfg *ClientConfig
	if nilCfg.VerifyBundle(b) == nil {
		t.Fatal("nil config must not verify a bundle")
	}
}
