package main

import (
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/keys"
)

// TestPackageUpdateFlagPairing pins the OTA packaging contract: --update-url
// and --trust must be given together (an OTA client without a trusted key
// cannot authenticate updates — same fail-closed model as the server's
// /update), and the URL must be http(s).
func TestPackageUpdateFlagPairing(t *testing.T) {
	if got := cmdPackage([]string{"someapp", "--update-url", "https://updates.example.com"}); got != 2 {
		t.Errorf("--update-url without --trust: exit = %d, want 2", got)
	}
	if got := cmdPackage([]string{"someapp", "--trust", "key.pub"}); got != 2 {
		t.Errorf("--trust without --update-url: exit = %d, want 2", got)
	}
	if got := cmdPackage([]string{"someapp", "--update-url", "ftp://updates.example.com", "--trust", "key.pub"}); got != 2 {
		t.Errorf("non-http(s) --update-url: exit = %d, want 2", got)
	}
}

// TestPackageUpdateFlagsAccepted: a well-formed pair passes flag validation and
// loads the trust key — the run then fails on the (nonexistent) app dir with
// exit 1, not the flag-usage exit 2.
func TestPackageUpdateFlagsAccepted(t *testing.T) {
	dir := t.TempDir()
	pub, _, err := keys.Generate()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(dir, "key.pub")
	if err := keys.WritePublic(pubPath, pub); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(dir, "no-such-app")
	if got := cmdPackage([]string{appDir, "--update-url", "https://updates.example.com", "--trust", pubPath}); got != 1 {
		t.Errorf("valid pair + missing app dir: exit = %d, want 1 (load error, not flag error)", got)
	}
	// A trust file that is not a public key must fail cleanly before packaging.
	if got := cmdPackage([]string{appDir, "--update-url", "https://updates.example.com", "--trust", filepath.Join(dir, "missing.pub")}); got != 1 {
		t.Errorf("unreadable --trust: exit = %d, want 1", got)
	}
}
