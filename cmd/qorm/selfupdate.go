package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/keys"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// cmdUpdate checks for QORM CLI updates on GitHub and performs a self-update.
// Downloaded binaries are verified against the release's ed25519-signed
// SHA256SUMS manifest before they replace the current executable, unless
// --insecure-skip-verify is given.
func cmdUpdate(args []string) int {
	skipVerify := false
	for _, a := range args {
		switch a {
		case "--insecure-skip-verify":
			skipVerify = true
		default:
			fmt.Fprintf(os.Stderr, "error: unknown flag %q\nusage: qorm update [--insecure-skip-verify]\n", a)
			return 2
		}
	}

	fmt.Println("Checking for updates...")

	req, err := http.NewRequest("GET", "https://api.github.com/repos/qorm/qorm/releases/latest", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create request: %v\n", err)
		return 1
	}
	req.Header.Set("User-Agent", "qorm-cli-updater")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to check updates: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: failed to check updates, GitHub returned HTTP %d\n", resp.StatusCode)
		return 1
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse update response: %v\n", err)
		return 1
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if latestVersion == currentVersion {
		fmt.Printf("QORM is already up to date (version %s)\n", version)
		return 0
	}

	// Simple version comparison (stable string match or newer check)
	fmt.Printf("A new version of QORM is available: v%s (current: v%s)\n", latestVersion, currentVersion)

	// Phase 1: Try using 'go install' if Go toolchain is locally available
	if _, err := exec.LookPath("go"); err == nil {
		fmt.Println("Updating via 'go install'...")
		cmd := exec.Command("go", "install", "github.com/qorm/qorm/cmd/qorm@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			fmt.Println("QORM updated successfully via go install!")
			return 0
		}
		fmt.Println("warn: go install failed, falling back to pre-compiled binary update...")
	}

	// Phase 2: Self-update by downloading the pre-compiled binary asset
	expectedAsset := fmt.Sprintf("qorm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expectedAsset += ".exe"
	}

	targetAsset := ""
	targetURL := ""
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, expectedAsset) {
			targetAsset = asset.Name
			targetURL = asset.DownloadURL
			break
		}
	}

	if targetURL == "" {
		fmt.Fprintf(os.Stderr, "error: no pre-compiled binary asset named %q found for %s/%s. Please update manually.\n", expectedAsset, runtime.GOOS, runtime.GOARCH)
		return 1
	}

	fmt.Printf("Downloading pre-compiled binary: %s...\n", targetAsset)

	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to resolve executable path: %v\n", err)
		return 1
	}

	respDL, err := client.Get(targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: download failed: %v\n", err)
		return 1
	}
	defer respDL.Body.Close()

	if respDL.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: download failed, GitHub returned HTTP %d\n", respDL.StatusCode)
		return 1
	}

	tmpDir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(tmpDir, "qorm_download_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create temporary file: %v\n", err)
		return 1
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, respDL.Body); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to save download contents: %v\n", err)
		return 1
	}
	tmpFile.Close()

	if skipVerify {
		fmt.Fprintln(os.Stderr, "WARNING: --insecure-skip-verify given — installing the downloaded binary WITHOUT sha256/ed25519 verification. Only do this if you trust the network path to GitHub.")
	} else if err := verifyDownloadedBinary(client, release, tmpPath, targetAsset); err != nil {
		fmt.Fprintf(os.Stderr, "error: release verification failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: re-run with --insecure-skip-verify to update without verification (NOT recommended).")
		return 1
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to set executable permissions: %v\n", err)
		return 1
	}

	// Rename current executable to backup
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to back up current binary: %v\n", err)
		return 1
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to replace binary: %v (restoring backup...)\n", err)
		_ = os.Rename(oldPath, exePath)
		return 1
	}

	_ = os.Remove(oldPath)

	fmt.Printf("QORM updated successfully to version v%s!\n", latestVersion)
	return 0
}

// maxManifestSize caps SHA256SUMS / SHA256SUMS.sig downloads (1 MiB).
const maxManifestSize = 1 << 20

// verifyDownloadedBinary fetches the release's SHA256SUMS + SHA256SUMS.sig and
// verifies the binary saved at tmpPath against them using the public keys
// embedded in this build (releasePubKeys). It errors when this build embeds no
// keys or the release carries no signed manifest.
func verifyDownloadedBinary(client *http.Client, release githubRelease, tmpPath, assetName string) error {
	pubs, err := parseReleasePubKeys(releasePubKeys)
	if err != nil {
		return err
	}
	if len(pubs) == 0 {
		return errors.New("this qorm build embeds no release public keys, so downloads cannot be verified")
	}

	sumsURL, sigURL := "", ""
	for _, asset := range release.Assets {
		switch asset.Name {
		case sumsAssetName:
			sumsURL = asset.DownloadURL
		case sigAssetName:
			sigURL = asset.DownloadURL
		}
	}
	if sumsURL == "" || sigURL == "" {
		return fmt.Errorf("release %s does not provide signed checksums (%s + %s)", release.TagName, sumsAssetName, sigAssetName)
	}

	sums, err := fetchSmallAsset(client, sumsURL, maxManifestSize)
	if err != nil {
		return fmt.Errorf("download %s: %w", sumsAssetName, err)
	}
	sig, err := fetchSmallAsset(client, sigURL, maxManifestSize)
	if err != nil {
		return fmt.Errorf("download %s: %w", sigAssetName, err)
	}
	bin, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	if err := verifyReleaseAsset(bin, sums, sig, assetName, pubs); err != nil {
		return err
	}
	pub, err := matchReleaseKey(sums, sig, pubs)
	if err != nil {
		return err // unreachable after verifyReleaseAsset, kept for safety
	}
	fmt.Printf("verified: sha256 ok, ed25519 ok (key %s)\n", keys.KeyID(pub))
	return nil
}

// fetchSmallAsset downloads url, refusing bodies larger than limit bytes.
func fetchSmallAsset(client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("asset exceeds %d byte limit", limit)
	}
	return data, nil
}

// parseReleasePubKeys decodes base64-encoded embedded release public keys.
func parseReleasePubKeys(encoded []string) ([]ed25519.PublicKey, error) {
	pubs := make([]ed25519.PublicKey, 0, len(encoded))
	for _, s := range encoded {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("embedded release public key %q is invalid", s)
		}
		pubs = append(pubs, ed25519.PublicKey(raw))
	}
	return pubs, nil
}

// matchReleaseKey returns the first key in pubs whose ed25519 signature over
// sums verifies. sig is the base64 signature (surrounding whitespace ignored).
func matchReleaseKey(sums, sig []byte, pubs []ed25519.PublicKey) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %v", sigAssetName, err)
	}
	for _, pub := range pubs {
		if ed25519.Verify(pub, sums, raw) {
			return pub, nil
		}
	}
	return nil, fmt.Errorf("%s signature does not verify against any embedded release public key", sumsAssetName)
}

// verifyReleaseAsset checks a downloaded binary against a signed SHA256SUMS
// manifest: the manifest's ed25519 signature must verify against one of pubs,
// the manifest must list assetName, and bin's sha256 must match the listed
// digest. Pure function — unit-tested in selfupdate_test.go.
func verifyReleaseAsset(bin, sums, sig []byte, assetName string, pubs []ed25519.PublicKey) error {
	if len(pubs) == 0 {
		return errors.New("no release public keys embedded in this build")
	}
	if _, err := matchReleaseKey(sums, sig, pubs); err != nil {
		return err
	}
	want, err := manifestDigest(sums, assetName)
	if err != nil {
		return err
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("sha256 mismatch for %s: manifest has %s, downloaded binary is %s", assetName, want, hex.EncodeToString(got[:]))
	}
	return nil
}

// manifestDigest extracts the lowercase hex sha256 recorded for assetName in a
// "sha256hex  filename" manifest.
func manifestDigest(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != sha256.Size*2 {
			return "", fmt.Errorf("malformed sha256 digest for %s in %s", assetName, sumsAssetName)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", fmt.Errorf("malformed sha256 digest for %s in %s", assetName, sumsAssetName)
		}
		return digest, nil
	}
	return "", fmt.Errorf("asset %q is not listed in %s", assetName, sumsAssetName)
}
