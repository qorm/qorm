// Package keys handles ed25519 key generation and on-disk storage for bundle
// signing. Keys are stored as a small text format: a header line plus the
// base64-encoded key bytes, so they are easy to inspect and move around.
package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

const (
	privHeader = "QORM-ED25519-PRIVATE-KEY"
	pubHeader  = "QORM-ED25519-PUBLIC-KEY"
)

// Generate creates a new ed25519 keypair.
func Generate() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// KeyID returns a short, stable identifier derived from a public key.
func KeyID(pub ed25519.PublicKey) string {
	return base64.RawStdEncoding.EncodeToString(pub)[:12]
}

// WritePrivate writes a private key file.
func WritePrivate(path string, priv ed25519.PrivateKey) error {
	return writeKey(path, privHeader, priv)
}

// WritePublic writes a public key file.
func WritePublic(path string, pub ed25519.PublicKey) error {
	return writeKey(path, pubHeader, pub)
}

func writeKey(path, header string, key []byte) error {
	body := fmt.Sprintf("%s\n%s\n", header, base64.StdEncoding.EncodeToString(key))
	return os.WriteFile(path, []byte(body), 0o600)
}

// LoadPrivate reads a private key file.
func LoadPrivate(path string) (ed25519.PrivateKey, error) {
	raw, err := readKey(path, privHeader)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length %d", len(raw))
	}
	return ed25519.PrivateKey(raw), nil
}

// LoadPublic reads a public key file.
func LoadPublic(path string) (ed25519.PublicKey, error) {
	raw, err := readKey(path, pubHeader)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func readKey(path, header string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 || strings.TrimSpace(lines[0]) != header {
		return nil, fmt.Errorf("%s: not a %s file", path, header)
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
}
