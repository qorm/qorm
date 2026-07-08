package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/keys"
)

// Names of the checksum manifest assets attached to every GitHub release.
const (
	sumsAssetName = "SHA256SUMS"
	sigAssetName  = "SHA256SUMS.sig"
)

// cmdReleaseSign implements the hidden `qorm __release-sign [--verify] <dist-dir>`
// command used by scripts/build-all.sh and the release workflow.
//
// Default mode: write <dist-dir>/SHA256SUMS covering every regular file in the
// directory (excluding SHA256SUMS itself and *.sig). When the QORM_RELEASE_KEY
// environment variable names an ed25519 private key file (internal/keys
// format, see `qorm keygen`), the manifest is additionally signed and the
// base64 signature written to <dist-dir>/SHA256SUMS.sig. Without the variable
// only the unsigned manifest is produced.
//
// --verify mode: derive the public key from QORM_RELEASE_KEY and re-run the
// exact client-side check (verifyReleaseAsset) for every asset listed in the
// on-disk SHA256SUMS, so CI proves the artifacts it is about to publish would
// pass `qorm update` verification.
func cmdReleaseSign(args []string) int {
	verify := false
	dir := ""
	for _, a := range args {
		switch {
		case a == "--verify":
			verify = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "error: unknown flag %q\nusage: qorm __release-sign [--verify] <dist-dir>\n", a)
			return 2
		case dir != "":
			fmt.Fprintln(os.Stderr, "usage: qorm __release-sign [--verify] <dist-dir>")
			return 2
		default:
			dir = a
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm __release-sign [--verify] <dist-dir>")
		return 2
	}
	if verify {
		return releaseVerify(dir)
	}
	return releaseSign(dir)
}

func releaseSign(dir string) int {
	sums, err := buildSHA256Sums(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	sumsPath := filepath.Join(dir, sumsAssetName)
	if err := os.WriteFile(sumsPath, sums, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s (%d assets)\n", sumsPath, strings.Count(string(sums), "\n"))

	keyPath := os.Getenv("QORM_RELEASE_KEY")
	if keyPath == "" {
		fmt.Println("note: QORM_RELEASE_KEY not set — manifest left unsigned (no SHA256SUMS.sig)")
		return 0
	}
	priv, err := keys.LoadPrivate(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: QORM_RELEASE_KEY: %v\n", err)
		return 1
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums)) + "\n"
	sigPath := filepath.Join(dir, sigAssetName)
	if err := os.WriteFile(sigPath, []byte(sig), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s (key %s)\n", sigPath, keys.KeyID(priv.Public().(ed25519.PublicKey)))
	return 0
}

func releaseVerify(dir string) int {
	keyPath := os.Getenv("QORM_RELEASE_KEY")
	if keyPath == "" {
		fmt.Fprintln(os.Stderr, "error: --verify requires QORM_RELEASE_KEY to derive the public key")
		return 1
	}
	priv, err := keys.LoadPrivate(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: QORM_RELEASE_KEY: %v\n", err)
		return 1
	}
	pub := priv.Public().(ed25519.PublicKey)
	sums, err := os.ReadFile(filepath.Join(dir, sumsAssetName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	sig, err := os.ReadFile(filepath.Join(dir, sigAssetName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	n := 0
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := fields[1]
		bin, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := verifyReleaseAsset(bin, sums, sig, name, []ed25519.PublicKey{pub}); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", name, err)
			return 1
		}
		n++
	}
	if n == 0 {
		fmt.Fprintf(os.Stderr, "error: %s lists no assets\n", sumsAssetName)
		return 1
	}
	fmt.Printf("verified %d assets: sha256 ok, ed25519 ok (key %s)\n", n, keys.KeyID(pub))
	return 0
}

// buildSHA256Sums hashes every regular file in dir — excluding the manifest
// itself and *.sig files — and renders the classic "sha256hex  filename"
// manifest, sorted by filename for reproducibility.
func buildSHA256Sums(dir string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		n := e.Name()
		if n == sumsAssetName || strings.HasSuffix(n, ".sig") {
			continue
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no assets to checksum", dir)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), n)
	}
	return []byte(b.String()), nil
}
