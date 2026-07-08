package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// signedFixture builds a two-asset SHA256SUMS manifest covering bin (as
// assetName) plus a dummy sibling asset, signs it, and returns everything the
// verifier needs.
func signedFixture(t *testing.T, bin []byte, assetName string) (sums, sig []byte, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	binSum := sha256.Sum256(bin)
	otherSum := sha256.Sum256([]byte("some other platform binary"))
	sums = []byte(fmt.Sprintf("%s  %s\n%s  qorm-linux-arm64\n",
		hex.EncodeToString(binSum[:]), assetName, hex.EncodeToString(otherSum[:])))
	sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums)) + "\n")
	return sums, sig, pub
}

func TestVerifyReleaseAssetOK(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, pub := signedFixture(t, bin, "qorm-linux-amd64")
	if err := verifyReleaseAsset(bin, sums, sig, "qorm-linux-amd64", []ed25519.PublicKey{pub}); err != nil {
		t.Fatalf("expected verification to pass, got: %v", err)
	}
}

func TestVerifyReleaseAssetSecondKeyOK(t *testing.T) {
	// Rotation scenario: the matching key is not the first embedded key.
	bin := []byte("binary payload")
	sums, sig, pub := signedFixture(t, bin, "qorm-darwin-arm64")
	stranger, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := verifyReleaseAsset(bin, sums, sig, "qorm-darwin-arm64", []ed25519.PublicKey{stranger, pub}); err != nil {
		t.Fatalf("expected verification with second key to pass, got: %v", err)
	}
}

func TestVerifyReleaseAssetTamperedBinary(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, pub := signedFixture(t, bin, "qorm-linux-amd64")
	tampered := append([]byte(nil), bin...)
	tampered[0] ^= 0x01 // flip one bit
	err := verifyReleaseAsset(tampered, sums, sig, "qorm-linux-amd64", []ed25519.PublicKey{pub})
	if err == nil {
		t.Fatal("expected sha256 mismatch for tampered binary, got nil")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch error, got: %v", err)
	}
}

func TestVerifyReleaseAssetTamperedSums(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, pub := signedFixture(t, bin, "qorm-linux-amd64")
	tampered := append([]byte(nil), sums...)
	tampered[0] ^= 0x01 // breaks the signature
	err := verifyReleaseAsset(bin, tampered, sig, "qorm-linux-amd64", []ed25519.PublicKey{pub})
	if err == nil {
		t.Fatal("expected signature failure for tampered SHA256SUMS, got nil")
	}
	if !strings.Contains(err.Error(), "signature does not verify") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}

func TestVerifyReleaseAssetUnknownAsset(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, pub := signedFixture(t, bin, "qorm-linux-amd64")
	err := verifyReleaseAsset(bin, sums, sig, "qorm-windows-amd64.exe", []ed25519.PublicKey{pub})
	if err == nil {
		t.Fatal("expected error for asset missing from manifest, got nil")
	}
	if !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("expected 'not listed' error, got: %v", err)
	}
}

func TestVerifyReleaseAssetNoPubKeys(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, _ := signedFixture(t, bin, "qorm-linux-amd64")
	if err := verifyReleaseAsset(bin, sums, sig, "qorm-linux-amd64", nil); err == nil {
		t.Fatal("expected error when no public keys are embedded, got nil")
	}
}

func TestVerifyReleaseAssetWrongKey(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, sig, _ := signedFixture(t, bin, "qorm-linux-amd64")
	stranger, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := verifyReleaseAsset(bin, sums, sig, "qorm-linux-amd64", []ed25519.PublicKey{stranger}); err == nil {
		t.Fatal("expected error when signature was made by an untrusted key, got nil")
	}
}

func TestVerifyReleaseAssetGarbageSig(t *testing.T) {
	bin := []byte("pretend this is a compiled qorm binary")
	sums, _, pub := signedFixture(t, bin, "qorm-linux-amd64")
	if err := verifyReleaseAsset(bin, sums, []byte("!!not-base64!!"), "qorm-linux-amd64", []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("expected error for non-base64 signature, got nil")
	}
}

func TestParseReleasePubKeys(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubs, err := parseReleasePubKeys([]string{base64.StdEncoding.EncodeToString(pub)})
	if err != nil {
		t.Fatalf("expected valid key to parse, got: %v", err)
	}
	if len(pubs) != 1 || !pub.Equal(pubs[0]) {
		t.Fatal("parsed key does not round-trip")
	}
	if _, err := parseReleasePubKeys([]string{"short"}); err == nil {
		t.Fatal("expected error for invalid embedded key, got nil")
	}
	if pubs, err := parseReleasePubKeys(nil); err != nil || len(pubs) != 0 {
		t.Fatalf("empty list should yield no keys and no error, got %d keys, err %v", len(pubs), err)
	}
}
