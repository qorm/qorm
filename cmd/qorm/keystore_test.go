package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureKeystoreReusesManagedStore(t *testing.T) {
	appDir := t.TempDir()
	q := filepath.Join(appDir, ".qorm")
	if err := os.MkdirAll(q, 0o700); err != nil {
		t.Fatal(err)
	}
	ks := filepath.Join(q, "release.keystore")
	if err := os.WriteFile(ks, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeProps(filepath.Join(q, "keystore.properties"), map[string]string{
		"storeFile":     ks,
		"storePassword": "sp123",
		"keyPassword":   "kp456",
		"keyAlias":      "myalias",
	}); err != nil {
		t.Fatal(err)
	}
	path, alias, storePass, keyPass, err := ensureKeystore(appDir, releaseOpts{Release: true})
	if err != nil {
		t.Fatalf("ensureKeystore: %v", err)
	}
	if path != ks || alias != "myalias" || storePass != "sp123" || keyPass != "kp456" {
		t.Fatalf("got %q %q %q %q", path, alias, storePass, keyPass)
	}
}

func TestEnsureKeystoreExternalFromEnv(t *testing.T) {
	t.Setenv("QORM_KEYSTORE_PASS", "envstore")
	t.Setenv("QORM_KEY_PASS", "envkey")
	dir := t.TempDir()
	ks := filepath.Join(dir, "user.keystore")
	if err := os.WriteFile(ks, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, alias, storePass, keyPass, err := ensureKeystore(t.TempDir(), releaseOpts{Release: true, Keystore: ks, KeyAlias: "upload"})
	if err != nil {
		t.Fatalf("ensureKeystore: %v", err)
	}
	if path != ks || alias != "upload" || storePass != "envstore" || keyPass != "envkey" {
		t.Fatalf("got %q %q %q %q", path, alias, storePass, keyPass)
	}
}

func TestEnsureKeystoreExternalKeyPassDefaultsToStorePass(t *testing.T) {
	t.Setenv("QORM_KEYSTORE_PASS", "onlystore")
	t.Setenv("QORM_KEY_PASS", "")
	dir := t.TempDir()
	ks := filepath.Join(dir, "user.keystore")
	if err := os.WriteFile(ks, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, alias, storePass, keyPass, err := ensureKeystore(t.TempDir(), releaseOpts{Release: true, Keystore: ks})
	if err != nil {
		t.Fatalf("ensureKeystore: %v", err)
	}
	if alias != "qorm" {
		t.Fatalf("default alias = %q, want qorm", alias)
	}
	if storePass != "onlystore" || keyPass != "onlystore" {
		t.Fatalf("got passwords %q %q", storePass, keyPass)
	}
}

func TestEnsureKeystoreExternalMissing(t *testing.T) {
	if _, _, _, _, err := ensureKeystore(t.TempDir(), releaseOpts{Release: true, Keystore: "/no/such/file.keystore"}); err == nil {
		t.Fatal("want error for missing keystore file")
	}
}

func TestEnsureKeystoreExternalNoPasswordNonTTY(t *testing.T) {
	// go test runs with stdin != TTY, so a missing env password must error
	t.Setenv("QORM_KEYSTORE_PASS", "")
	t.Setenv("QORM_KEY_PASS", "")
	dir := t.TempDir()
	ks := filepath.Join(dir, "user.keystore")
	if err := os.WriteFile(ks, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := ensureKeystore(t.TempDir(), releaseOpts{Release: true, Keystore: ks}); err == nil {
		t.Fatal("want error when no password source is available")
	}
}

func TestPropsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "keystore.properties")
	in := map[string]string{"storeFile": "/a/b.keystore", "storePassword": "x", "keyAlias": "qorm"}
	if err := writeProps(p, in); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("keystore.properties mode = %v, want 0600", st.Mode().Perm())
	}
	out := readProps(p)
	for k, v := range in {
		if out[k] != v {
			t.Fatalf("props[%q] = %q, want %q", k, out[k], v)
		}
	}
}

func TestRandPass(t *testing.T) {
	a, err := randPass()
	if err != nil {
		t.Fatal(err)
	}
	b, err := randPass()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 32 || a == b {
		t.Fatalf("randPass: len=%d a==b=%v", len(a), a == b)
	}
}
