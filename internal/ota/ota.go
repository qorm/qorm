// Package ota fetches a QORM bundle from a remote or local source and returns
// it only if it verifies. It is the transport half of over-the-air UI updates;
// the trust half lives in package bundle. A failed fetch or a failed
// verification returns an error and never yields a bundle — so the caller can
// safely keep running the previous one (rollback by inaction).
package ota

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/bundle"
)

// Fetch retrieves raw bundle bytes from an http(s) URL or a local file path.
func Fetch(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %d", source, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
	}
	return os.ReadFile(source)
}

// FetchVerified fetches, decodes and verifies a bundle in one step, rejecting
// revoked signing keys. The returned bundle is safe to activate. Any error
// means the update should be rejected and the current bundle kept.
func FetchVerified(source string, trust ed25519.PublicKey, revoked bundle.RevocationList) (*bundle.Bundle, error) {
	data, err := Fetch(source)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	b, err := bundle.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := bundle.VerifyWithRevocation(b, trust, revoked); err != nil {
		return nil, err
	}
	return b, nil
}
