package main

// Run-time middle-layer injection: the stock `qorm run` binary has no user
// Go, so an app's native/desktop.go (custom native ops, custom canvas
// widgets) only took effect via `qorm package`. This closes the dev loop:
// when the app has a middle layer, `qorm run` builds a cached user binary on
// demand and hands the run to it — same command line, same window, but with
// the app's Go compiled in.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// maybeRunUserBinary hands the run to the per-app user binary when the app
// has native/desktop.go. Returns (true, code) when it handled the run
// (success or failure); (false, 0) means continue with the stock binary.
func maybeRunUserBinary(dir string, args []string) (bool, int) {
	if os.Getenv("QORM_USER_BINARY") == "1" {
		return false, 0 // we ARE the injected binary — never recurse
	}
	src := filepath.Join(dir, "native", "desktop.go")
	data, err := os.ReadFile(src)
	if err != nil {
		return false, 0 // no middle layer: the stock binary is fine
	}
	bin, err := userBinaryPath(dir, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: middle layer found but could not be compiled (%v); running WITHOUT it\n", err)
		return false, 0
	}
	cmd := exec.Command(bin, append([]string{"run"}, args...)...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = append(os.Environ(), "QORM_USER_BINARY=1")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return true, ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: user binary failed to start: %v\n", err)
		return true, 1
	}
	return true, 0
}

// userBinaryPath returns the cached user binary for this middle-layer
// content (hash of desktop.go + the qorm version), building it on first use.
// The cache key is content-based, so editing the app's Go rebuilds exactly
// once, and unrelated apps share nothing.
func userBinaryPath(dir string, data []byte) (string, error) {
	h := sha256.Sum256(append(data, []byte(version)...))
	cacheDir := filepath.Join(os.TempDir(), "qorm-userbin")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	bin := filepath.Join(cacheDir, fmt.Sprintf("qorm-user-%x", h[:8]))
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	cleanup := injectUserGo(dir, "github.com/qorm/platform/cmd/qorm")
	defer cleanup()
	tmp := bin + ".tmp"
	cmd := exec.Command("go", "build", "-o", tmp, "github.com/qorm/platform/cmd/qorm")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	if err := os.Rename(tmp, bin); err != nil {
		return "", err
	}
	return bin, nil
}
