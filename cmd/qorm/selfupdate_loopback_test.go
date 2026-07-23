package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// stubBinary returns the test binary's own bytes. Exec'd with
// QORM_TEST_STUB_VERSION=v in the environment (TestMain intercepts before any
// test runs), those bytes are a real, EXECUTABLE binary that prints
// `qorm v (go... ...)` and exits 0 — a deterministic, toolchain-free stand-in
// for a downloaded release binary that reports a pinned version.
func stubBinary(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatalf("read the test binary for the stub payload: %v", err)
	}
	return b
}

// withStubVersionEnv pins the version a stubBinary payload reports when the
// update flow execs it (the exec'd child inherits this environment).
func withStubVersionEnv(t *testing.T, v string) {
	t.Helper()
	t.Setenv("QORM_TEST_STUB_VERSION", v)
}

// writeVersionScript writes an executable POSIX shell script that prints one
// `qorm <v> (...)` line — a pinned-version stub for the GOBIN binary in tests
// where the GOBIN binary and selfExe must report DIFFERENT versions (the
// TestMain interceptor stub reports whatever QORM_TEST_STUB_VERSION pins for
// every copy of the test binary in the same run).
func writeVersionScript(t *testing.T, path, v string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the POSIX shell version stub is not available on Windows")
	}
	script := fmt.Sprintf("#!/bin/sh\necho 'qorm %s (go1.99.9 %s/%s)'\n", v, runtime.GOOS, runtime.GOARCH)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

func withoutGoInstallPhase(t *testing.T) {
	t.Helper()
	old := goInstallPhase
	goInstallPhase = func() error { return errGoInstallUnavailable }
	t.Cleanup(func() { goInstallPhase = old })
}

// withNoopGoInstallPhase makes the 'go install' fast path report success
// without touching the network or a Go toolchain, so only the post-install
// version check (driven by the selfExe stub) decides the update's outcome.
func withNoopGoInstallPhase(t *testing.T) {
	t.Helper()
	old := goInstallPhase
	goInstallPhase = func() error { return nil }
	t.Cleanup(func() { goInstallPhase = old })
}

// withStubSelfExe points selfExe at the test binary itself acting as a fake
// installed qorm: TestMain intercepts QORM_TEST_STUB_VERSION before any test
// runs and prints `qorm <reported> (go... ...)` instead of testing, so the
// post-install check execs a deterministic stub and never the real binary.
func withStubSelfExe(t *testing.T, reported string) {
	t.Helper()
	t.Setenv("QORM_TEST_STUB_VERSION", reported)
	withSelfExe(t, os.Args[0])
}

// TestMain doubles as the stub "installed binary" for the post-install version
// check tests: when QORM_TEST_STUB_VERSION is set the test binary was exec'd as
// `qorm version`, so print the pinned version line (main.go's exact format)
// and exit 0 instead of running tests.
func TestMain(m *testing.M) {
	if v, ok := os.LookupEnv("QORM_TEST_STUB_VERSION"); ok {
		fmt.Printf("qorm %s (%s %s/%s)\n", v, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}
	os.Exit(m.Run())
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

// withGoInstallBinDir points the GOBIN/GOPATH-bin resolution seam at dir so
// the go-install post-check runs against a temp directory — never a real
// `go env`, keeping the test offline and toolchain-free.
func withGoInstallBinDir(t *testing.T, dir string) {
	t.Helper()
	old := goInstallBinDir
	goInstallBinDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { goInstallBinDir = old })
}

// withUnresolvableGoInstallBinDir makes the GOBIN resolution seam fail (as
// `go env` would when no toolchain is on PATH), exercising the warn-and-fall-
// back-to-selfExe path.
func withUnresolvableGoInstallBinDir(t *testing.T) {
	t.Helper()
	old := goInstallBinDir
	goInstallBinDir = func() (string, error) { return "", errors.New("go toolchain not available (test seam)") }
	t.Cleanup(func() { goInstallBinDir = old })
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
	// Keep the go-install post-check off the real toolchain: the GOBIN seam
	// resolves to an existing-but-empty temp dir by default (the "no qorm
	// binary at the resolved destination" fallback to selfExe). GOBIN-specific
	// subtests override this.
	withGoInstallBinDir(t, filepath.Join(t.TempDir(), "empty-gobin"))

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
		// The downloaded payload is an EXECUTABLE stub reporting the approved
		// version: the pre-swap check execs it before the swap (an inert
		// payload would be refused as unrunnable).
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v10.0.0", stubBinary(t), priv)
		withStubVersionEnv(t, "10.0.0")
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("a strictly newer signed release must install; exit = %d, out = %q", code, out)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, stubBinary(t)) {
			t.Errorf("installed binary is not the served release binary")
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
		exePath, _ := updateFlow(t, pub, "9.9.9-rc1", "v9.9.9", stubBinary(t), priv)
		withStubVersionEnv(t, "9.9.9") // the stub reports the approved target exactly
		code, out, _ := runUpdate(t)
		if code != 0 {
			t.Fatalf("the final release after a prerelease must install; exit = %d, out = %q", code, out)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, stubBinary(t)) {
			t.Errorf("installed binary is not the served release binary")
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

// TestUpdateLoopbackGoInstallPostCheck pins the post-install version check on
// the 'go install' fast path. That phase trusts the Go module proxy — a
// compromised proxy could resolve @latest to a module OLDER than the
// strictly-newer release the version gate approved — so after the phase
// "succeeds" the freshly installed binary must report a version >= the
// approved target, or the update is refused with a non-zero exit. The phase is
// stubbed as a no-op success and selfExe points at a deterministic stub binary
// (the test binary via TestMain), so nothing touches the network, the Go
// toolchain, or the real binary.
func TestUpdateLoopbackGoInstallPostCheck(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release key: %v", err)
	}
	newBin := []byte("the newer signed release binary")

	t.Run("stub reporting the approved version succeeds", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withStubSelfExe(t, "10.0.0")
		code, out, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("a stub reporting the approved version must succeed; exit = %d, out = %q, err = %q", code, out, errOut)
		}
		if !strings.Contains(out, "successfully via go install") {
			t.Errorf("stdout should confirm the go-install update, got %q", out)
		}
		if n := binHits.Load(); n != 0 {
			t.Errorf("binary downloads = %d; the go-install branch must not download the signed asset", n)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("the go-install branch must not replace the binary itself (that is 'go install's job); it now contains %q", got)
		}
	})

	t.Run("stub reporting a NEWER version succeeds", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withStubSelfExe(t, "10.0.1") // proxy served something even newer than the gate's target
		code, out, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("a version newer than the approved target is still >= it; exit = %d, out = %q, err = %q", code, out, errOut)
		}
	})

	t.Run("stub reporting an OLDER version fails closed", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withStubSelfExe(t, "9.9.8") // a stale module the compromised proxy served as @latest
		code, out, errOut := runUpdate(t)
		if code == 0 {
			t.Fatalf("a stub reporting a version older than the approved target must fail; out = %q, err = %q", out, errOut)
		}
		if !strings.Contains(errOut, "post-install version check failed") {
			t.Errorf("stderr = %q, want the post-install check failure", errOut)
		}
		if !strings.Contains(errOut, "WARNING") {
			t.Errorf("stderr = %q, want a loud warning that success is NOT being reported", errOut)
		}
		if strings.Contains(out, "successfully") {
			t.Errorf("stdout must not claim a successful update, got %q", out)
		}
		if binHits.Load() != 0 {
			t.Error("a failed post-install check must exit, not fall through to the binary download")
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("the go-install branch must not replace the binary itself; it now contains %q", got)
		}
	})

	t.Run("stub reporting an unparseable version fails closed", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withStubSelfExe(t, "banana")
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("a stub reporting an unparseable version must fail")
		}
		if !strings.Contains(errOut, "unparseable version") {
			t.Errorf("stderr = %q, want an unparseable-version failure", errOut)
		}
	})

	t.Run("version command that cannot run fails closed", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withSelfExe(t, filepath.Join(t.TempDir(), "no-such-qorm"))
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("an unrunnable installed binary must fail the post-install check")
		}
		if !strings.Contains(errOut, "cannot verify the installed version") {
			t.Errorf("stderr = %q, want a verification failure", errOut)
		}
	})
}

// TestUpdateLoopbackPreSwapVersionCheck pins the pre-swap version check on the
// signed-binary download path. That path writes the verified binary to a temp
// file and THEN swaps it over the running binary — once swapped it cannot be
// undone, and a signature only proves the publisher signed THIS file, not that
// the file is the release its tag claims (the release pipeline itself could
// publish a stale build under a newer tag). So BEFORE the swap the update flow
// execs the downloaded binary's `version` command and refuses unless it
// reports >= the approved target — while the current binary is still in place.
// The downloaded payload is the executable TestMain stub (stubBinary), so the
// check runs a real binary without a network or toolchain.
func TestUpdateLoopbackPreSwapVersionCheck(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release key: %v", err)
	}

	t.Run("stub reporting an OLDER version is refused before the swap", func(t *testing.T) {
		exePath, binHits := updateFlow(t, pub, "9.9.9", "v10.0.0", stubBinary(t), priv)
		withStubVersionEnv(t, "9.9.8") // the release pipeline published a stale build under v10.0.0
		code, out, errOut := runUpdate(t)
		if code == 0 {
			t.Fatalf("a downloaded binary OLDER than the approved target must not be swapped in; out = %q, err = %q", out, errOut)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("a refused swap must leave the current binary byte-identical; it now contains %q", got)
		}
		if n := binHits.Load(); n != 1 {
			t.Errorf("downloads = %d, want 1: the check runs after download+signature verification, before the swap", n)
		}
		if !strings.Contains(errOut, "refusing to install the downloaded binary") {
			t.Errorf("stderr = %q, want a loud refusal notice", errOut)
		}
		if !strings.Contains(errOut, "OLDER than the approved target") {
			t.Errorf("stderr = %q, want the pre-swap version failure", errOut)
		}
		if strings.Contains(out, "successfully") {
			t.Errorf("stdout must not claim a successful update, got %q", out)
		}
		// The temp download must be cleaned up and no backup left behind
		// (refusal happens before the running binary is ever renamed).
		entries, err := os.ReadDir(filepath.Dir(exePath))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "qorm_download_") {
				t.Errorf("temp download file %s should be removed after a refused swap", e.Name())
			}
		}
		if _, err := os.Stat(exePath + ".old"); !os.IsNotExist(err) {
			t.Errorf("no backup should exist after a refusal that never swapped (stat err = %v)", err)
		}
	})

	t.Run("stub reporting an unparseable version is refused", func(t *testing.T) {
		exePath, _ := updateFlow(t, pub, "9.9.9", "v10.0.0", stubBinary(t), priv)
		withStubVersionEnv(t, "banana")
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("a downloaded binary reporting an unparseable version must not be swapped in")
		}
		if !strings.Contains(errOut, "unparseable version") {
			t.Errorf("stderr = %q, want an unparseable-version failure", errOut)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("a refused swap must leave the current binary untouched; it now contains %q", got)
		}
	})

	t.Run("unrunnable downloaded binary is refused", func(t *testing.T) {
		inert := []byte("valid signature, valid checksums, but not an executable")
		exePath, _ := updateFlow(t, pub, "9.9.9", "v10.0.0", inert, priv)
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("a downloaded binary whose version command cannot run must not be swapped in")
		}
		if !strings.Contains(errOut, "cannot verify the downloaded binary") {
			t.Errorf("stderr = %q, want a cannot-verify failure", errOut)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("a refused swap must leave the current binary untouched; it now contains %q", got)
		}
	})
}

// TestUpdateLoopbackGoInstallGOBIN pins the GOBIN/GOPATH-bin reconciliation on
// the go-install post-install check. `go install` writes to GOBIN (or
// GOPATH/bin), which may differ from the running binary's directory (selfExe);
// reading only selfExe would then see the STALE binary and fail closed on a
// LEGITIMATE install. So the check runs against the resolved install location
// when it exists and differs from selfExe, succeeds when EITHER location
// reports the expected version, fails closed only when neither does, and —
// when the resolved location cannot be checked (go env unavailable / no binary
// there) — warns and falls back to selfExe instead of a hard failure. The seam
// keeps every subtest off the real toolchain.
func TestUpdateLoopbackGoInstallGOBIN(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate release key: %v", err)
	}
	newBin := []byte("the newer signed release binary")

	t.Run("GOBIN binary reporting the approved version succeeds with a stale selfExe", func(t *testing.T) {
		exePath, _ := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		gobin := t.TempDir()
		withGoInstallBinDir(t, gobin)
		// `go install` wrote the fresh binary into GOBIN; selfExe's directory
		// (updateFlow's inert installedBinaryBody file) still holds the OLD one.
		if err := os.WriteFile(filepath.Join(gobin, qormBinaryName()), stubBinary(t), 0o755); err != nil {
			t.Fatal(err)
		}
		withStubVersionEnv(t, "10.0.0")
		code, out, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("a GOBIN binary reporting the approved version must succeed even when selfExe is stale; exit = %d, out = %q, err = %q", code, out, errOut)
		}
		if !strings.Contains(out, "successfully via go install") {
			t.Errorf("stdout should confirm the go-install update, got %q", out)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("the go-install branch must not touch selfExe's file; it now contains %q", got)
		}
	})

	t.Run("older GOBIN binary still succeeds when selfExe reports the approved version", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		gobin := t.TempDir()
		withGoInstallBinDir(t, gobin)
		// The two locations must report DIFFERENT versions in one run: the
		// GOBIN binary is a shell script pinned to an OLD version, while
		// selfExe is the TestMain stub pinned (via env) to the approved one.
		writeVersionScript(t, filepath.Join(gobin, qormBinaryName()), "9.9.8")
		withStubSelfExe(t, "10.0.0")
		code, out, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("the check must pass when EITHER location reports the approved version; exit = %d, out = %q, err = %q", code, out, errOut)
		}
		if !strings.Contains(out, "successfully via go install") {
			t.Errorf("stdout should confirm the go-install update, got %q", out)
		}
	})

	t.Run("unresolvable GOBIN falls back to selfExe and succeeds when selfExe is correct", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withUnresolvableGoInstallBinDir(t) // `go env` unavailable
		withStubSelfExe(t, "10.0.0")
		code, out, errOut := runUpdate(t)
		if code != 0 {
			t.Fatalf("an unresolvable GOBIN must fall back to selfExe, not hard-fail; exit = %d, out = %q, err = %q", code, out, errOut)
		}
		if !strings.Contains(errOut, "warn: cannot resolve the 'go install' destination") {
			t.Errorf("stderr = %q, want a warning that the GOBIN location could not be resolved", errOut)
		}
		if !strings.Contains(out, "successfully via go install") {
			t.Errorf("stdout should confirm the go-install update, got %q", out)
		}
	})

	t.Run("GOBIN dir without a qorm binary falls back to selfExe and fails when selfExe is stale", func(t *testing.T) {
		exePath, _ := updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		withGoInstallBinDir(t, t.TempDir()) // resolved dir exists but holds no qorm binary
		// selfExe is updateFlow's inert installed binary: its version command
		// cannot run, so the fallback check fails closed.
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("a fallback to a stale selfExe must fail closed")
		}
		if !strings.Contains(errOut, "no qorm binary at the resolved 'go install' destination") {
			t.Errorf("stderr = %q, want the missing-binary warning", errOut)
		}
		if !strings.Contains(errOut, "cannot verify the installed version") {
			t.Errorf("stderr = %q, want the selfExe verification failure", errOut)
		}
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != installedBinaryBody {
			t.Errorf("the go-install branch must not touch selfExe's file; it now contains %q", got)
		}
	})

	t.Run("older GOBIN binary AND stale selfExe fail closed", func(t *testing.T) {
		updateFlow(t, pub, "9.9.9", "v10.0.0", newBin, priv)
		withNoopGoInstallPhase(t)
		gobin := t.TempDir()
		withGoInstallBinDir(t, gobin)
		if err := os.WriteFile(filepath.Join(gobin, qormBinaryName()), stubBinary(t), 0o755); err != nil {
			t.Fatal(err)
		}
		withStubVersionEnv(t, "9.9.8") // GOBIN reports older; selfExe is inert
		code, _, errOut := runUpdate(t)
		if code == 0 {
			t.Fatal("neither location reporting the approved version must fail closed")
		}
		if !strings.Contains(errOut, "neither") {
			t.Errorf("stderr = %q, want a failure naming both checked locations", errOut)
		}
		if !strings.Contains(errOut, "post-install version check failed") {
			t.Errorf("stderr = %q, want the post-install check failure", errOut)
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
