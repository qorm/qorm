package keys

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	pub, priv, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("unexpected key sizes: pub=%d priv=%d", len(pub), len(priv))
	}
	// The returned public key must correspond to the private key.
	if !pub.Equal(priv.Public().(ed25519.PublicKey)) {
		t.Fatal("public key does not match private key")
	}
	sig := ed25519.Sign(priv, []byte("qorm"))
	if !ed25519.Verify(pub, []byte("qorm"), sig) {
		t.Fatal("signature made with the generated key does not verify")
	}
}

func TestKeyID(t *testing.T) {
	pub, _, _ := Generate()
	id := KeyID(pub)
	if len(id) != 12 {
		t.Fatalf("KeyID length = %d, want 12", len(id))
	}
	// Derivation: first 12 chars of the raw-base64 encoding of the public key.
	if want := base64.RawStdEncoding.EncodeToString(pub)[:12]; id != want {
		t.Errorf("KeyID = %q, want %q", id, want)
	}
	if id != KeyID(pub) {
		t.Error("KeyID must be stable for the same key")
	}
	other, _, _ := Generate()
	if KeyID(other) == id {
		t.Error("different keys produced the same KeyID")
	}
}

func TestPrivateKeyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.key")
	pub, priv, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := WritePrivate(path, priv); err != nil {
		t.Fatalf("WritePrivate: %v", err)
	}
	// Key material must be written owner-only.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file perm = %o, want 600", perm)
	}
	got, err := LoadPrivate(path)
	if err != nil {
		t.Fatalf("LoadPrivate: %v", err)
	}
	if !got.Equal(priv) {
		t.Error("loaded private key differs from written key")
	}
	if !got.Public().(ed25519.PublicKey).Equal(pub) {
		t.Error("loaded private key no longer matches the generated public key")
	}
}

func TestPublicKeyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.pub")
	pub, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := WritePublic(path, pub); err != nil {
		t.Fatalf("WritePublic: %v", err)
	}
	got, err := LoadPublic(path)
	if err != nil {
		t.Fatalf("LoadPublic: %v", err)
	}
	if !got.Equal(pub) {
		t.Error("loaded public key differs from written key")
	}
	// The id must survive the round trip unchanged.
	if KeyID(got) != KeyID(pub) {
		t.Error("KeyID changed across write/load")
	}
}

func TestLoadMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.key")
	if _, err := LoadPrivate(missing); err == nil {
		t.Error("LoadPrivate on a missing file should error")
	}
	if _, err := LoadPublic(missing); err == nil {
		t.Error("LoadPublic on a missing file should error")
	}
}

func TestLoadCorrupt(t *testing.T) {
	pub, priv, _ := Generate()
	loadPriv := func(p string) error { _, err := LoadPrivate(p); return err }
	loadPub := func(p string) error { _, err := LoadPublic(p); return err }

	cases := []struct {
		name    string
		content string
		load    func(string) error
	}{
		{"empty file", "", loadPriv},
		{"header only", privHeader + "\n", loadPriv},
		{"wrong header", "SOMETHING-ELSE\n" + base64.StdEncoding.EncodeToString(priv) + "\n", loadPriv},
		{"bad base64", privHeader + "\n!!!not-base64!!!\n", loadPriv},
		{"truncated private key", privHeader + "\n" + base64.StdEncoding.EncodeToString(priv[:32]) + "\n", loadPriv},
		{"truncated public key", pubHeader + "\n" + base64.StdEncoding.EncodeToString(pub[:16]) + "\n", loadPub},
		// A public-key file is not a private key, even though its bytes decode.
		{"public file as private", pubHeader + "\n" + base64.StdEncoding.EncodeToString(pub) + "\n", loadPriv},
		{"private file as public", privHeader + "\n" + base64.StdEncoding.EncodeToString(priv) + "\n", loadPub},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.key")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if err := tc.load(path); err == nil {
				t.Errorf("load should reject %s", tc.name)
			}
		})
	}
}

func TestWriteErrors(t *testing.T) {
	pub, priv, _ := Generate()
	badDir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := WritePrivate(filepath.Join(badDir, "id.key"), priv); err == nil {
		t.Error("WritePrivate into a missing directory should error")
	}
	if err := WritePublic(filepath.Join(badDir, "id.pub"), pub); err == nil {
		t.Error("WritePublic into a missing directory should error")
	}
}
