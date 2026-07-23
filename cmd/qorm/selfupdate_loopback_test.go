package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// loopbackRelease spins up a local HTTP server that serves a SHA256SUMS manifest
// (covering bin under assetName) and its SHA256SUMS.sig, and returns a
// githubRelease whose asset URLs point at it — the same shape `qorm update`
// fetches from GitHub, but loopback so the test is deterministic and offline.
func loopbackRelease(t *testing.T, bin []byte, assetName string, signKey ed25519.PrivateKey, tamperSig bool) (githubRelease, *httptest.Server) {
	t.Helper()
	sum := sha256.Sum256(bin)
	sums := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName))
	sig := ed25519.Sign(signKey, sums)
	if tamperSig {
		sig[0] ^= 0x01 // signature no longer matches the manifest
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+sumsAssetName, func(w http.ResponseWriter, r *http.Request) { w.Write(sums) })
	mux.HandleFunc("/"+sigAssetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(base64.StdEncoding.EncodeToString(sig) + "\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	var rel githubRelease
	rel.TagName = "v9.9.9"
	assetsJSON := fmt.Sprintf(`[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]`,
		sumsAssetName, srv.URL+"/"+sumsAssetName, sigAssetName, srv.URL+"/"+sigAssetName)
	if err := json.Unmarshal([]byte(assetsJSON), &rel.Assets); err != nil {
		t.Fatalf("build release assets: %v", err)
	}
	return rel, srv
}

// withReleasePubKeys temporarily overrides the embedded release public keys for
// the duration of the test (the update path reads this package var).
func withReleasePubKeys(t *testing.T, pubs ...ed25519.PublicKey) {
	t.Helper()
	old := releasePubKeys
	encoded := make([]string, 0, len(pubs))
	for _, p := range pubs {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(p))
	}
	releasePubKeys = encoded
	t.Cleanup(func() { releasePubKeys = old })
}

// TestUpdateLoopback verifies the `qorm update` verification path end-to-end
// against a loopback release server: a downloaded binary whose SHA256SUMS
// manifest is ed25519-signed by an embedded release key is accepted, and the
// same flow with a bad signature is rejected fail-closed.
func TestUpdateLoopback(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release key: %v", err)
	}
	withReleasePubKeys(t, pub)

	bin := []byte("pretend this is the downloaded qorm binary for this platform")
	const assetName = "qorm-linux-amd64"
	tmpPath := filepath.Join(t.TempDir(), assetName)
	if err := os.WriteFile(tmpPath, bin, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("signed manifest verifies", func(t *testing.T) {
		rel, _ := loopbackRelease(t, bin, assetName, priv, false)
		out := captureStdout(t, func() {
			if err := verifyDownloadedBinary(&http.Client{}, rel, tmpPath, assetName); err != nil {
				t.Fatalf("expected loopback verification to pass, got: %v", err)
			}
		})
		if !strings.Contains(out, "ed25519 ok") {
			t.Errorf("success path should confirm ed25519, stdout = %q", out)
		}
	})

	t.Run("bad signature is fail-closed", func(t *testing.T) {
		rel, _ := loopbackRelease(t, bin, assetName, priv, true)
		err := verifyDownloadedBinary(&http.Client{}, rel, tmpPath, assetName)
		if err == nil {
			t.Fatal("expected verification to fail for a bad manifest signature, got nil")
		}
		if !strings.Contains(err.Error(), "signature does not verify") {
			t.Errorf("expected a signature failure, got: %v", err)
		}
	})

	t.Run("manifest signed by an untrusted key is fail-closed", func(t *testing.T) {
		_, stranger, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		rel, _ := loopbackRelease(t, bin, assetName, stranger, false)
		if err := verifyDownloadedBinary(&http.Client{}, rel, tmpPath, assetName); err == nil {
			t.Fatal("expected verification to fail when the manifest is signed by a key this build does not embed, got nil")
		}
	})

	t.Run("binary not matching the manifest is fail-closed", func(t *testing.T) {
		rel, _ := loopbackRelease(t, bin, assetName, priv, false)
		other := filepath.Join(t.TempDir(), assetName)
		if err := os.WriteFile(other, []byte("a different, tampered binary"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := verifyDownloadedBinary(&http.Client{}, rel, other, assetName)
		if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("expected sha256 mismatch for a tampered binary, got: %v", err)
		}
	})
}

// installedBinaryBody is what the loopback update tests place at the fake
// install path; a self-update that runs must replace exactly these bytes.
const installedBinaryBody = "the currently installed qorm binary"

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

func withoutGoInstallPhase(t *testing.T) {
	t.Helper()
	old := goInstallPhase
	goInstallPhase = false
	t.Cleanup(func() { goInstallPhase = old })
}

func withSelfExe(t *testing.T, path string) {
	t.Helper()
	old := selfExe
	selfExe = func() (string, error) { return path, nil }
	t.Cleanup(func() { selfExe = old })
}

func withReleaseAPIURL(t *testing.T, url string) {
	t.Helper()
	old := releaseAPIURL
	releaseAPIURL = url
	t.Cleanup(func() { releaseAPIURL = old })
}

// updateFlow wires `qorm update` end-to-end against a loopback release server:
// release metadata JSON (the GitHub API shape) at /releases/latest advertising
// tag as the latest release, this platform's binary asset (body newBin) at
// /bin, and the ed25519-signed SHA256SUMS sidecar. The "installed" binary is a
// temp file (selfExe is overridden), never the running test binary; the running
// version is pinned and the 'go install' phase is disabled so the flow stays
// deterministic and offline. The returned counter tracks /bin downloads.
func updateFlow(t *testing.T, trustedPub ed25519.PublicKey, currentVersion, tag string, newBin []byte, signKey ed25519.PrivateKey) (exePath string, binHits *atomic.Int32) {
	t.Helper()
	binHits = &atomic.Int32{}

	assetName := fmt.Sprintf("qorm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	sum := sha256.Sum256(newBin)
	sums := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName))
	sig := ed25519.Sign(signKey, sums)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		var rel githubRelease
		rel.TagName = tag
		assetsJSON := fmt.Sprintf(`[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]`,
			assetName, srv.URL+"/bin", sumsAssetName, srv.URL+"/"+sumsAssetName, sigAssetName, srv.URL+"/"+sigAssetName)
		if err := json.Unmarshal([]byte(assetsJSON), &rel.Assets); err != nil {
			t.Errorf("build release assets: %v", err)
		}
		if err := json.NewEncoder(w).Encode(&rel); err != nil {
			t.Errorf("encode release JSON: %v", err)
		}
	})
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) {
		binHits.Add(1)
		w.Write(newBin)
	})
	mux.HandleFunc("/"+sumsAssetName, func(w http.ResponseWriter, r *http.Request) { w.Write(sums) })
	mux.HandleFunc("/"+sigAssetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(base64.StdEncoding.EncodeToString(sig) + "\n"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	withReleasePubKeys(t, trustedPub)
	withVersion(t, currentVersion)
	withoutGoInstallPhase(t)

	dir := t.TempDir()
	exePath = filepath.Join(dir, "qorm")
	if err := os.WriteFile(exePath, []byte(installedBinaryBody), 0o755); err != nil {
		t.Fatal(err)
	}
	withSelfExe(t, exePath)
	withReleaseAPIURL(t, srv.URL+"/releases/latest")
	return exePath, binHits
}

// runUpdate runs cmdUpdate with no flags, capturing both output streams.
func runUpdate(t *testing.T) (code int, out, errOut string) {
	t.Helper()
	out = captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			code = cmdUpdate(nil)
		})
	})
	return code, out, errOut
}

// TestUpdateLoopbackVersionGate pins the self-update version gate end-to-end.
// The install decision must not be "the remote tag differs from the running
// version": a compromised or misconfigured release endpoint serving an OLDER
// signed release (valid signature, valid checksums) must not roll the CLI back
// to a stale, potentially vulnerable build. Only a strictly newer signed
// release is installed; equal, older, prerelease-of-current, and unparseable
// tags are all refused without replacing the binary (and exit 0: a refused
// downgrade is not an error condition).
func TestUpdateLoopbackVersionGate(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release key: %v", err)
	}
	newBin := []byte("the newer signed release binary")

	t.Run("older signed release is refused, binary untouched", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v1.2.3", newBin, priv)
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("refusing a downgrade is not an error; exit = %d", code)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("an older signed release must not replace the running binary; it now contains %q", got)
		}
		if n := binHits.Load(); n != 0 {
			t.Errorf("the older release binary was downloaded %d time(s); the gate must refuse before any download", n)
		}
		if !strings.Contains(out, "refusing to downgrade") {
			t.Errorf("stdout should say the downgrade is refused, got %q", out)
		}
	})

	t.Run("equal release reports already up to date", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v9.9.9", newBin, priv)
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("an equal-version release must not replace the binary; it now contains %q", got)
		}
		if binHits.Load() != 0 {
			t.Error("nothing should be downloaded for an equal version")
		}
		if !strings.Contains(out, "already up to date") {
			t.Errorf("stdout = %q, want an up-to-date notice", out)
		}
	})

	t.Run("prerelease of the running version is refused", func(t *testing.T) {
		exePath, _ := updateFlow(t, pub, "9.9.9", "v9.9.9-rc1", newBin, priv)
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("a prerelease of the same X.Y.Z sorts older and must not be installed; binary now %q", got)
		}
		if !strings.Contains(out, "refusing to downgrade") {
			t.Errorf("stdout = %q, want a downgrade refusal", out)
		}
	})

	t.Run("malformed remote tag is refused", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "banana", newBin, priv)
		code, _, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("safe refusal should exit 0, got %d", code)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("an unparseable remote tag must not lead to an install; binary now %q", got)
		}
		if binHits.Load() != 0 {
			t.Error("nothing should be downloaded for an unparseable tag")
		}
		if !strings.Contains(errOut, "refusing to self-update") {
			t.Errorf("stderr = %q, want a refusal notice", errOut)
		}
	})

	t.Run("newer signed release is installed", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("a strictly newer signed release must install; exit = %d, out = %q", code, out)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, newBin) {
			t.Errorf("installed binary = %q, want the served release binary", got)
		}
		if n := binHits.Load(); n != 1 {
			t.Errorf("binary downloads = %d, want exactly 1", n)
		}
		if !strings.Contains(out, "ed25519 ok") {
			t.Errorf("the install must be signature-verified, stdout = %q", out)
		}
		if !strings.Contains(out, "v10.0.0") {
			t.Errorf("success message should name the new version, stdout = %q", out)
		}
		if _, err := os.Stat(exePath + ".old"); !os.IsNotExist(err) {
			t.Errorf("backup should be cleaned up after a successful update (stat err = %v)", err)
		}
	})

	t.Run("final release of a prerelease build is installed", func(t *testing.T) {
		exePath, _ := updateFlow(t, pub, "9.9.9-rc1", "v9.9.9", newBin, priv)
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("the final release after a prerelease must install; exit = %d, out = %q", code, out)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, newBin) {
			t.Errorf("installed binary = %q, want the served release binary", got)
		}
	})

	t.Run("newer release signed by an untrusted key is not installed", func(t *testing.T) {
		_, stranger, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		exePath, _ := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, stranger)
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("a release signed by an untrusted key must not install")
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("an untrusted-signature release must not replace the binary; it now contains %q", got)
		}
		if !strings.Contains(errOut, "verification failed") {
			t.Errorf("stderr = %q, want a verification failure", errOut)
		}
	})
}

// TestUpdateLoopbackNoEmbeddedKeys guards the fail-closed default: a build that
// embeds no release public keys cannot verify any download.
func TestUpdateLoopbackNoEmbeddedKeys(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	withReleasePubKeys(t) // empty: no trusted release keys in this build

	bin := []byte("binary")
	tmpPath := filepath.Join(t.TempDir(), "qorm-linux-amd64")
	if err := os.WriteFile(tmpPath, bin, 0o644); err != nil {
		t.Fatal(err)
	}
	rel, _ := loopbackRelease(t, bin, "qorm-linux-amd64", priv, false)
	err = verifyDownloadedBinary(&http.Client{}, rel, tmpPath, "qorm-linux-amd64")
	if err == nil || !strings.Contains(err.Error(), "no release public keys") {
		t.Fatalf("expected a no-embedded-keys failure, got: %v", err)
	}
}
