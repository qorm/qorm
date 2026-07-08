package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs f with os.Stdout redirected to a pipe and returns what
// was printed.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	f()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// TestBuildRequireCapabilityFlowsThroughVerifyAndRun is the P0.3 integration
// test: `qorm build --require-capability` stamps the requirements into the
// bundle, `qorm verify` reports them, and starting a bundle that requires a
// capability the current (desktop) platform lacks is refused with a clear
// error.
func TestBuildRequireCapabilityFlowsThroughVerifyAndRun(t *testing.T) {
	app := filepath.Join("..", "..", "examples", "counter")
	dir := t.TempDir()

	// Build with declared requirements the desktop platform supports.
	out := filepath.Join(dir, "counter.qorm.bundle")
	buildOut := captureStdout(t, func() {
		if code := cmdBuild([]string{app, "-o", out, "--require-capability", "camera,location"}); code != 0 {
			t.Errorf("qorm build exited %d", code)
		}
	})
	if !strings.Contains(buildOut, "requires: camera, location") {
		t.Errorf("build output should list requirements, got %q", buildOut)
	}

	// Verify reports the capability requirements.
	verifyOut := captureStdout(t, func() {
		if code := cmdVerify([]string{out}); code != 0 {
			t.Errorf("qorm verify exited %d", code)
		}
	})
	if !strings.Contains(verifyOut, "requires capabilities: camera, location") {
		t.Errorf("verify output should list requirements, got %q", verifyOut)
	}

	// A supported requirement starts fine from the bundle.
	if _, _, err := buildServer(out, "", ""); err != nil {
		t.Fatalf("bundle requiring camera+location should start on desktop: %v", err)
	}

	// nfc is iOS/Android-only: starting the bundle on desktop must be refused.
	nfcOut := filepath.Join(dir, "nfc.qorm.bundle")
	captureStdout(t, func() {
		if code := cmdBuild([]string{app, "-o", nfcOut, "--require-capability", "nfc"}); code != 0 {
			t.Errorf("qorm build exited %d", code)
		}
	})
	if _, _, err := buildServer(nfcOut, "", ""); err == nil {
		t.Fatal("starting a bundle that requires nfc must fail on desktop")
	} else if !strings.Contains(err.Error(), "nfc") {
		t.Fatalf("startup error should name the missing capability, got: %v", err)
	}
}
