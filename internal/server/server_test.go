package server

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/render"
)

func counterDir() string { return filepath.Join("..", "..", "examples", "counter") }

func signedBundle(t *testing.T, version string, priv ed25519.PrivateKey, pub ed25519.PublicKey) *bundle.Bundle {
	t.Helper()
	b, err := bundle.Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := b.SetVersion(version); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return b
}

func writeBundle(t *testing.T, b *bundle.Bundle) string {
	t.Helper()
	data, err := bundle.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	p := filepath.Join(t.TempDir(), "b.bundle")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestOTAUpdateRejectAndRollback(t *testing.T) {
	pub, priv, _ := keys.Generate()
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}

	if !strings.Contains(renderCurrent(s), ">COUNTER<") {
		t.Fatal("v1 should render the counter")
	}

	// Trusted update to v2 succeeds.
	if _, err := s.Update(writeBundle(t, signedBundle(t, "2.0.0", priv, pub))); err != nil {
		t.Fatalf("trusted update should succeed: %v", err)
	}
	if s.current.Version() != "2.0.0" {
		t.Fatalf("want active version 2.0.0, got %s", s.current.Version())
	}

	// Update signed by an untrusted key is rejected; the app stays on v2.
	otherPub, otherPriv, _ := keys.Generate()
	evil := signedBundle(t, "9.9.9", otherPriv, otherPub)
	if _, err := s.Update(writeBundle(t, evil)); err == nil {
		t.Fatal("untrusted update must be rejected")
	}
	if s.current.Version() != "2.0.0" {
		t.Fatalf("rejected update must not change the live app; got %s", s.current.Version())
	}

	// Rollback returns to v1.
	if _, err := s.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if s.current.Version() != "1.0.0" {
		t.Fatalf("want rolled-back version 1.0.0, got %s", s.current.Version())
	}
}

// renderCurrent renders the server's current runtime to HTML for assertions.
func renderCurrent(s *Server) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return render.Render(s.rt).HTML
}

func TestOTARejectsRevokedKey(t *testing.T) {
	pub, priv, _ := keys.Generate()
	kid := keys.KeyID(pub)
	// Server running v1, but the signing key is on the revocation list.
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, bundle.RevocationList{kid: true})
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	// An OTA update signed by that revoked key must be refused; app stays on v1.
	p := writeBundle(t, signedBundle(t, "2.0.0", priv, pub))
	if _, err := s.Update(p); err == nil {
		t.Fatal("OTA update signed by a revoked key must be refused")
	}
	if s.current.Version() != "1.0.0" {
		t.Errorf("live app must stay on 1.0.0 after refused update, got %s", s.current.Version())
	}
}

// TestRequiredCapabilitiesGateStartup verifies the runtime refuses to start a
// bundle whose declared capability requirements the current platform cannot
// meet — a hard error, not a warning.
func TestRequiredCapabilitiesGateStartup(t *testing.T) {
	pub, priv, _ := keys.Generate()

	// Supported everywhere on desktop: starts fine.
	ok := signedCapBundle(t, []string{"camera", "location"}, priv, pub)
	if _, err := NewBundle(ok, pub, nil); err != nil {
		t.Fatalf("bundle requiring camera+location should start on desktop: %v", err)
	}

	// nfc is iOS/Android-only — no desktop platform supports it.
	bad := signedCapBundle(t, []string{"nfc"}, priv, pub)
	if _, err := NewBundle(bad, pub, nil); err == nil {
		t.Fatal("bundle requiring nfc must refuse to start on desktop")
	} else if !strings.Contains(err.Error(), "nfc") {
		t.Fatalf("error should name the missing capability, got: %v", err)
	}

	// An unknown capability name is also a hard error.
	unknown := signedCapBundle(t, []string{"teleport"}, priv, pub)
	if _, err := NewBundle(unknown, pub, nil); err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Fatalf("unknown capability must be refused, got: %v", err)
	}

	// The stem/widget split is honoured: "badge" (widget "dockbadge") resolves.
	if err := CheckRequiredCapabilities(signedCapBundle(t, []string{"badge"}, priv, pub)); err != nil {
		if hostPlatform() == "mac" {
			t.Fatalf("badge (widget dockbadge) should resolve via its stem on mac: %v", err)
		}
	}
}

// TestOTAUpdateRejectsUnsupportedCapabilities verifies a hot update to a bundle
// with unmeetable requirements is refused and the live app is untouched.
func TestOTAUpdateRejectsUnsupportedCapabilities(t *testing.T) {
	pub, priv, _ := keys.Generate()
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	next := signedCapBundle(t, []string{"nfc"}, priv, pub)
	if _, err := s.Update(writeBundle(t, next)); err == nil {
		t.Fatal("OTA update requiring nfc must be refused on desktop")
	}
	if s.current.Version() != "1.0.0" {
		t.Errorf("refused update must not change the live app, got %s", s.current.Version())
	}
}

// signedCapBundle builds a signed counter bundle declaring the given
// capability requirements.
func signedCapBundle(t *testing.T, caps []string, priv ed25519.PrivateKey, pub ed25519.PublicKey) *bundle.Bundle {
	t.Helper()
	b, err := bundle.Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := b.SetVersion("2.0.0"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := b.SetRequiredCapabilities(caps); err != nil {
		t.Fatalf("caps: %v", err)
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return b
}
