package main

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/keys"
)

// buildQORMBinary compiles the qorm CLI into a temp dir so dispatch can be
// exercised end-to-end (main calls os.Exit, so it cannot run in-process).
func buildQORMBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "qormcli")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/qorm: %v\n%s", err, out)
	}
	return bin
}

// runQORM runs the built binary with args (plus extra env) and returns its
// stdout, stderr, and exit code.
func runQORM(t *testing.T, bin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if err == nil {
		return so.String(), se.String(), 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("run qorm %v: %v", args, err)
	}
	return so.String(), se.String(), ee.ExitCode()
}

// TestCLIDispatch builds the real binary and drives main's subcommand dispatch
// end to end: version aliases, usage/unknown paths, and the keygen -> build ->
// sign -> verify chain over temp dirs plus examples/counter.
func TestCLIDispatch(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()
	counter := filepath.Join("..", "..", "examples", "counter")

	t.Run("version aliases", func(t *testing.T) {
		for _, arg := range []string{"version", "--version", "-v"} {
			out, _, code := runQORM(t, bin, nil, arg)
			if code != 0 {
				t.Errorf("%s: exit = %d, want 0", arg, code)
			}
			// The version is bumped by scripts/release.sh at release time —
			// pin the semver shape, never a specific version string.
			if !regexp.MustCompile(`^qorm \d+\.\d+\.\d+ `).MatchString(out) {
				t.Errorf("%s: stdout = %q, want prefix %q", arg, out, "qorm <semver> ")
			}
		}
	})

	t.Run("help aliases", func(t *testing.T) {
		for _, arg := range []string{"help", "--help", "-h"} {
			_, errOut, code := runQORM(t, bin, nil, arg)
			if code != 0 {
				t.Errorf("%s: exit = %d, want 0", arg, code)
			}
			if !strings.Contains(errOut, "qorm new <dir>") {
				t.Errorf("%s: stderr should print usage, got %q", arg, errOut)
			}
		}
	})

	t.Run("no args prints usage and exits 2", func(t *testing.T) {
		_, errOut, code := runQORM(t, bin, nil)
		if code != 2 {
			t.Fatalf("no args: exit = %d, want 2 (stderr %q)", code, errOut)
		}
		if !strings.Contains(errOut, "usage:") {
			t.Errorf("no args: stderr = %q, want usage text", errOut)
		}
	})

	t.Run("unknown command exits 2", func(t *testing.T) {
		_, errOut, code := runQORM(t, bin, nil, "frobnicate")
		if code != 2 {
			t.Fatalf("unknown: exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `unknown command "frobnicate"`) {
			t.Errorf("unknown: stderr = %q, want the command named", errOut)
		}
	})

	// keygen feeds the build/sign/verify subtests below.
	privPath := filepath.Join(work, "qorm_key")
	pubPath := filepath.Join(work, "qorm_key.pub")
	t.Run("keygen writes a loadable keypair", func(t *testing.T) {
		out, errOut, code := runQORM(t, bin, nil, "keygen", "--out-dir", work)
		if code != 0 {
			t.Fatalf("keygen: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "generated keypair") {
			t.Errorf("keygen stdout = %q, want confirmation", out)
		}
		priv, err := keys.LoadPrivate(privPath)
		if err != nil {
			t.Fatalf("private key on disk should load: %v", err)
		}
		pub, err := keys.LoadPublic(pubPath)
		if err != nil {
			t.Fatalf("public key on disk should load: %v", err)
		}
		if !priv.Public().(ed25519.PublicKey).Equal(pub) {
			t.Error("written public key does not match the private key")
		}
		if !strings.Contains(out, keys.KeyID(pub)) {
			t.Errorf("keygen stdout should name the key id %s, got %q", keys.KeyID(pub), out)
		}
	})

	appDir := filepath.Join(work, "cliapp")
	t.Run("new then render round-trip", func(t *testing.T) {
		if _, errOut, code := runQORM(t, bin, nil, "new", appDir); code != 0 {
			t.Fatalf("new: exit = %d, stderr %q", code, errOut)
		}
		if _, err := os.Stat(filepath.Join(appDir, "qorm.json")); err != nil {
			t.Fatalf("new should write qorm.json: %v", err)
		}
		htmlPath := filepath.Join(work, "cliapp.html")
		out, errOut, code := runQORM(t, bin, nil, "render", appDir, "-o", htmlPath)
		if code != 0 {
			t.Fatalf("render: exit = %d, stderr %q", code, errOut)
		}
		html, err := os.ReadFile(htmlPath)
		if err != nil {
			t.Fatalf("render output missing: %v", err)
		}
		if !strings.Contains(string(html), "cliapp") {
			t.Errorf("rendered page should contain the app name, got %d bytes", len(html))
		}
		if !strings.Contains(out, "rendered") {
			t.Errorf("render stdout = %q, want progress line", out)
		}
	})

	bundlePath := filepath.Join(work, "counter.qorm.bundle")
	t.Run("build signs the counter bundle", func(t *testing.T) {
		out, errOut, code := runQORM(t, bin, nil, "build", counter, "-o", bundlePath, "--key", privPath)
		if code != 0 {
			t.Fatalf("build: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "signed by key") {
			t.Errorf("build stdout = %q, want signing confirmation", out)
		}
		if _, err := os.Stat(bundlePath); err != nil {
			t.Fatalf("bundle missing: %v", err)
		}
	})

	t.Run("verify accepts the signed bundle", func(t *testing.T) {
		out, errOut, code := runQORM(t, bin, nil, "verify", bundlePath, "--trust", pubPath)
		if code != 0 {
			t.Fatalf("verify: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "OK:") || !strings.Contains(out, "integrity + signature") {
			t.Errorf("verify stdout = %q, want OK + signature scope", out)
		}
	})

	t.Run("verify rejects a tampered bundle", func(t *testing.T) {
		data, err := os.ReadFile(bundlePath)
		if err != nil {
			t.Fatal(err)
		}
		tampered := filepath.Join(work, "tampered.qorm.bundle")
		bad := append([]byte(nil), data...)
		bad[len(bad)/2] ^= 0x01
		if err := os.WriteFile(tampered, bad, 0o644); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "verify", tampered, "--trust", pubPath)
		if code != 1 {
			t.Fatalf("verify tampered: exit = %d, want 1", code)
		}
		if !strings.Contains(errOut, "VERIFY FAILED") {
			t.Errorf("verify tampered: stderr = %q, want VERIFY FAILED", errOut)
		}
	})

	t.Run("verify honours revocation", func(t *testing.T) {
		pub, err := keys.LoadPublic(pubPath)
		if err != nil {
			t.Fatal(err)
		}
		revoked := filepath.Join(work, "revoked.json")
		if err := os.WriteFile(revoked, []byte(`["`+keys.KeyID(pub)+`"]`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "verify", bundlePath, "--trust", pubPath, "--revoked", revoked)
		if code != 1 {
			t.Fatalf("verify revoked: exit = %d, want 1 (stderr %q)", code, errOut)
		}
		if !strings.Contains(errOut, "VERIFY FAILED") {
			t.Errorf("verify revoked: stderr = %q, want VERIFY FAILED", errOut)
		}
	})

	t.Run("sign re-signs into a separate output", func(t *testing.T) {
		signed := filepath.Join(work, "resigned.qorm.bundle")
		// An unsigned copy: rebuild without --key.
		unsigned := filepath.Join(work, "unsigned.qorm.bundle")
		if _, errOut, code := runQORM(t, bin, nil, "build", counter, "-o", unsigned); code != 0 {
			t.Fatalf("build unsigned: exit = %d, stderr %q", code, errOut)
		}
		if _, errOut, code := runQORM(t, bin, nil, "sign", unsigned, "--key", privPath, "-o", signed); code != 0 {
			t.Fatalf("sign: exit = %d, stderr %q", code, errOut)
		}
		if _, errOut, code := runQORM(t, bin, nil, "verify", signed, "--trust", pubPath); code != 0 {
			t.Fatalf("verify signed: exit = %d, stderr %q", code, errOut)
		}
	})

	t.Run("missing-argument usage errors exit 2", func(t *testing.T) {
		cases := [][]string{
			{"run"},
			{"render"},
			{"build"},
			{"verify"},
			{"sign"},
			{"sign", "bundle.bin"}, // key omitted
			{"mcp"},
			{"new"},
			{"audit"},
			{"audit", "a.jsonl", "b.jsonl"}, // exactly one arg required
			{"check"},
			{"check", "app"}, // neither --checks nor --audit
			{"preview"},
			{"measure"},
			{"__release-sign"},
			{"__release-sign", "--bogus"}, // unknown flag is a usage error
			{"update", "--bogus-flag"},    // unknown flag is a usage error
		}
		for _, args := range cases {
			if _, _, code := runQORM(t, bin, nil, args...); code != 2 {
				t.Errorf("qorm %v: exit = %d, want 2", args, code)
			}
		}
	})

	t.Run("missing inputs exit 1", func(t *testing.T) {
		cases := [][]string{
			{"run", filepath.Join(work, "no-such-app")},
			{"render", filepath.Join(work, "no-such-app")},
			{"build", filepath.Join(work, "no-such-app")},
			{"verify", filepath.Join(work, "no-such-bundle")},
			{"sign", filepath.Join(work, "no-such-bundle"), "--key", privPath},
			{"mcp", filepath.Join(work, "no-such-app")},
			{"audit", filepath.Join(work, "no-such-log.jsonl")},
			{"docs", "--docs", filepath.Join(work, "no-such-docs")},
			{"keygen", "--out-dir", filepath.Join(work, "no", "such", "dir")},
		}
		for _, args := range cases {
			if _, _, code := runQORM(t, bin, nil, args...); code != 1 {
				t.Errorf("qorm %v: exit = %d, want 1", args, code)
			}
		}
	})

	t.Run("shot refuses without the desktop tag", func(t *testing.T) {
		_, errOut, code := runQORM(t, bin, nil, "shot", counter)
		if code != 2 {
			t.Fatalf("shot: exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "desktop") {
			t.Errorf("shot stderr = %q, should mention the desktop tag", errOut)
		}
	})

	t.Run("pure-build measure/check/preview refuse", func(t *testing.T) {
		for _, args := range [][]string{
			{"measure", counter},
			{"check", counter, "--audit"},
			{"preview", counter},
		} {
			_, errOut, code := runQORM(t, bin, nil, args...)
			if code != 1 {
				t.Errorf("qorm %v: exit = %d, want 1", args, code)
			}
			if !strings.Contains(errOut, "-tags desktop") {
				t.Errorf("qorm %v: stderr = %q, should name the missing tag", args, errOut)
			}
		}
	})

	t.Run("docs renders a markdown tree", func(t *testing.T) {
		docsDir := filepath.Join(work, "docs")
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(docsDir, "guide.md"),
			[]byte("# Guide\n\nHello **world**.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		outDir := filepath.Join(work, "site")
		out, errOut, code := runQORM(t, bin, nil, "docs", "--docs", docsDir, "-o", outDir)
		if code != 0 {
			t.Fatalf("docs: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "rendered 1 pages") {
			t.Errorf("docs stdout = %q, want page count", out)
		}
		index, err := os.ReadFile(filepath.Join(outDir, "index.html"))
		if err != nil {
			t.Fatalf("index.html missing: %v", err)
		}
		if !strings.Contains(string(index), "Guide") {
			t.Error("index.html should render the heading")
		}
	})

	t.Run("updates rejects malformed rollout.json", func(t *testing.T) {
		dir := filepath.Join(work, "bundles")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rollout.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "updates", dir)
		if code != 1 {
			t.Fatalf("updates: exit = %d, want 1 (stderr %q)", code, errOut)
		}
	})

	t.Run("audit verifies a hash-chained log", func(t *testing.T) {
		log := filepath.Join(work, "audit.jsonl")
		if err := os.WriteFile(log, buildAuditChain(t, 3), 0o600); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := runQORM(t, bin, nil, "audit", log)
		if code != 0 {
			t.Fatalf("audit: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "AUDIT OK: 3 entries") {
			t.Errorf("audit stdout = %q, want 3 verified entries", out)
		}

		// Break the chain: edit the first entry's detail.
		lines := strings.Split(strings.TrimSpace(string(mustReadFile(t, log))), "\n")
		lines[0] = strings.Replace(lines[0], `"detail":"entry 1"`, `"detail":"forged"`, 1)
		broken := filepath.Join(work, "broken.jsonl")
		if err := os.WriteFile(broken, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, errOut, code = runQORM(t, bin, nil, "audit", broken)
		if code != 1 {
			t.Fatalf("audit broken: exit = %d, want 1", code)
		}
		if !strings.Contains(errOut, "AUDIT FAIL") {
			t.Errorf("audit broken: stderr = %q, want AUDIT FAIL", errOut)
		}
	})

	t.Run("release-sign round-trip", func(t *testing.T) {
		dist := filepath.Join(work, "dist")
		if err := os.MkdirAll(dist, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range map[string]string{
			"qorm-darwin-arm64": "binary one",
			"qorm-linux-amd64":  "binary two",
		} {
			if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		env := []string{"QORM_RELEASE_KEY=" + privPath}
		out, errOut, code := runQORM(t, bin, env, "__release-sign", dist)
		if code != 0 {
			t.Fatalf("__release-sign: exit = %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "SHA256SUMS") {
			t.Errorf("__release-sign stdout = %q, want manifest name", out)
		}
		if _, err := os.Stat(filepath.Join(dist, "SHA256SUMS.sig")); err != nil {
			t.Fatalf("signature file missing: %v", err)
		}
		if _, errOut, code := runQORM(t, bin, env, "__release-sign", "--verify", dist); code != 0 {
			t.Fatalf("__release-sign --verify: exit = %d, stderr %q", code, errOut)
		}
		// --verify without the key cannot derive the public key.
		if _, _, code := runQORM(t, bin, []string{"QORM_RELEASE_KEY="}, "__release-sign", "--verify", dist); code != 1 {
			t.Errorf("__release-sign --verify without key: exit = %d, want 1", code)
		}
	})
}
