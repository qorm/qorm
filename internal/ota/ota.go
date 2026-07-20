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

// maxBundle is the largest accepted bundle payload: 32 MiB. Anything bigger
// is a hard error (never a silent truncation) — the caller treats a failed
// fetch as "no update" and keeps running the current bundle.
const maxBundle = 32 << 20

// Fetch retrieves raw bundle bytes from an http(s) URL or a local file path.
// Both paths enforce the maxBundle size cap: an oversize payload is an error.
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
		return readCapped(resp.Body)
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readCapped(f)
}

// readCapped reads up to maxBundle bytes, erroring when the source holds more
// — so a truncated payload can never slip through to the verifier as if it
// were the whole bundle.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBundle+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBundle {
		return nil, fmt.Errorf("bundle exceeds the %d MiB size cap", maxBundle>>20)
	}
	return data, nil
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
