package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
)

// runQORMStdin runs the built binary with args, feeding stdin, and returns
// stdout, stderr and exit code. Used to drive the stdio transports end-to-end.
func runQORMStdin(t *testing.T, bin, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
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

// newAppDir scaffolds a fresh app under work via `qorm new` and returns its dir.
func newAppDir(t *testing.T, bin, work, name string) string {
	t.Helper()
	dir := filepath.Join(work, name)
	if _, errOut, code := runQORM(t, bin, nil, "new", dir); code != 0 {
		t.Fatalf("qorm new %s: exit %d, stderr %q", name, code, errOut)
	}
	return dir
}

// TestProbeVerifyUntrustedKey drives `qorm verify` end-to-end: a bundle signed
// by one key must be rejected when verified against a DIFFERENT trusted key
// (authenticity, not just integrity).
func TestProbeVerifyUntrustedKey(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()
	app := newAppDir(t, bin, work, "app")

	signer := filepath.Join(work, "signer")
	stranger := filepath.Join(work, "stranger")
	if err := os.MkdirAll(signer, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runQORM(t, bin, nil, "keygen", "--out-dir", signer); code != 0 {
		t.Fatalf("keygen signer: exit %d, stderr %q", code, errOut)
	}
	if _, errOut, code := runQORM(t, bin, nil, "keygen", "--out-dir", stranger); code != 0 {
		t.Fatalf("keygen stranger: exit %d, stderr %q", code, errOut)
	}

	bundlePath := filepath.Join(work, "app.qorm.bundle")
	if _, errOut, code := runQORM(t, bin, nil, "build", app, "-o", bundlePath, "--key", filepath.Join(signer, "qorm_key")); code != 0 {
		t.Fatalf("build: exit %d, stderr %q", code, errOut)
	}

	// Against the signer's own public key: OK.
	out, errOut, code := runQORM(t, bin, nil, "verify", bundlePath, "--trust", filepath.Join(signer, "qorm_key.pub"))
	if code != 0 {
		t.Fatalf("verify with signer key: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "integrity + signature") {
		t.Errorf("verify with signer key: stdout = %q, want signature scope", out)
	}

	// Against an unrelated (untrusted) public key: rejected.
	_, errOut, code = runQORM(t, bin, nil, "verify", bundlePath, "--trust", filepath.Join(stranger, "qorm_key.pub"))
	if code != 1 {
		t.Fatalf("verify with untrusted key: exit %d, want 1 (stderr %q)", code, errOut)
	}
	if !strings.Contains(errOut, "VERIFY FAILED") || !strings.Contains(errOut, "signature does not match") {
		t.Errorf("verify with untrusted key: stderr = %q, want signature mismatch", errOut)
	}
}

// TestProbeAuditReorderAndTruncate drives `qorm audit` end-to-end on tampered
// chains: reordering two entries and truncating a line mid-JSON must both be
// rejected with a precise message (the valid + mutated cases are covered in
// cli_dispatch_test.go).
func TestProbeAuditReorderAndTruncate(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()

	chain := string(buildAuditChain(t, 4))
	lines := strings.Split(strings.TrimSpace(chain), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 chain lines, got %d", len(lines))
	}

	t.Run("reordered entries are rejected", func(t *testing.T) {
		reordered := []string{lines[0], lines[2], lines[1], lines[3]} // swap 2 & 3
		log := filepath.Join(work, "reordered.jsonl")
		if err := os.WriteFile(log, []byte(strings.Join(reordered, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "audit", log)
		if code != 1 {
			t.Fatalf("audit reordered: exit %d, want 1 (stderr %q)", code, errOut)
		}
		if !strings.Contains(errOut, "AUDIT FAIL") || !strings.Contains(errOut, "hash mismatch") {
			t.Errorf("audit reordered: stderr = %q, want a hash-mismatch failure", errOut)
		}
	})

	t.Run("a truncated line is rejected as invalid JSON", func(t *testing.T) {
		// Cut the file partway through the second line: the first entry still
		// verifies, the half-written second is not valid JSON.
		log := filepath.Join(work, "truncated.jsonl")
		partial := lines[0] + "\n" + lines[1][:len(lines[1])/2]
		if err := os.WriteFile(log, []byte(partial), 0o600); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "audit", log)
		if code != 1 {
			t.Fatalf("audit truncated: exit %d, want 1 (stderr %q)", code, errOut)
		}
		if !strings.Contains(errOut, "AUDIT FAIL") || !strings.Contains(errOut, "not valid JSON") {
			t.Errorf("audit truncated: stderr = %q, want a not-valid-JSON failure", errOut)
		}
	})

	t.Run("the whole valid chain still verifies", func(t *testing.T) {
		log := filepath.Join(work, "valid.jsonl")
		if err := os.WriteFile(log, []byte(chain), 0o600); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := runQORM(t, bin, nil, "audit", log)
		if code != 0 {
			t.Fatalf("audit valid: exit %d, stderr %q", code, errOut)
		}
		if !strings.Contains(out, "AUDIT OK: 4 entries") {
			t.Errorf("audit valid: stdout = %q, want 4 verified entries", out)
		}
	})
}

// TestProbeMCPStdioFraming drives `qorm mcp` over its stdio transport: a
// well-formed initialize/tools-list sequence answers each request, a
// notification produces NO output, and a garbage line yields the JSON-RPC
// -32700 parse-error line on stdout (round-4 behaviour).
func TestProbeMCPStdioFraming(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()
	app := newAppDir(t, bin, work, "mcpapp")

	t.Run("initialize and tools/list answer; notification is silent", func(t *testing.T) {
		stdin := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		}, "\n") + "\n"
		out, errOut, code := runQORMStdin(t, bin, stdin, "mcp", app)
		if code != 0 {
			t.Fatalf("mcp: exit %d, stderr %q", code, errOut)
		}
		respLines := strings.Split(strings.TrimSpace(out), "\n")
		if len(respLines) != 2 {
			t.Fatalf("mcp: expected exactly 2 responses (notification is silent), got %d:\n%s", len(respLines), out)
		}
		if !strings.Contains(respLines[0], `"id":1`) || !strings.Contains(respLines[0], `"protocolVersion"`) {
			t.Errorf("first response should be the initialize result, got %q", respLines[0])
		}
		if !strings.Contains(respLines[1], `"id":2`) || !strings.Contains(respLines[1], `"tools"`) {
			t.Errorf("second response should be the tools/list result, got %q", respLines[1])
		}
	})

	t.Run("a garbage line yields the -32700 parse-error line on stdout", func(t *testing.T) {
		out, _, code := runQORMStdin(t, bin, "this is not json {{{\n", "mcp", app)
		if code != 0 {
			t.Fatalf("mcp should survive a garbage line and exit 0 on EOF, got %d", code)
		}
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			t.Fatal("garbage line should produce a parse-error response on stdout, got nothing")
		}
		if !strings.Contains(trimmed, `-32700`) {
			t.Errorf("parse-error response should carry code -32700, got %q", trimmed)
		}
		if !strings.Contains(trimmed, `"id":null`) {
			t.Errorf("parse-error response should have a null id, got %q", trimmed)
		}
	})

	t.Run("a lone notification yields no output", func(t *testing.T) {
		out, _, code := runQORMStdin(t, bin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n", "mcp", app)
		if code != 0 {
			t.Fatalf("mcp notification: exit %d, want 0", code)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("a notification must produce no response, got %q", out)
		}
	})
}

// TestProbeRenderBundle drives `qorm render` on a COMPILED BUNDLE (not just a
// source dir): a valid bundle renders, and a tampered bundle is refused on
// integrity.
func TestProbeRenderBundle(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()
	app := newAppDir(t, bin, work, "rendapp")

	bundlePath := filepath.Join(work, "rend.qorm.bundle")
	if _, errOut, code := runQORM(t, bin, nil, "build", app, "-o", bundlePath); code != 0 {
		t.Fatalf("build: exit %d, stderr %q", code, errOut)
	}

	t.Run("a compiled bundle renders to HTML", func(t *testing.T) {
		outPath := filepath.Join(work, "from_bundle.html")
		_, errOut, code := runQORM(t, bin, nil, "render", bundlePath, "-o", outPath)
		if code != 0 {
			t.Fatalf("render bundle: exit %d, stderr %q", code, errOut)
		}
		html, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("render output missing: %v", err)
		}
		if !strings.Contains(string(html), "rendapp") {
			t.Errorf("rendered bundle HTML should contain the app name, got %d bytes", len(html))
		}
	})

	t.Run("a tampered bundle is refused on integrity", func(t *testing.T) {
		// Corrupt the stored content hash while keeping the JSON well-formed, so
		// the bundle decodes but fails the integrity check deterministically
		// (a random byte-flip could land on JSON structure and fail the decode
		// instead — still refused, but a different message).
		data, err := os.ReadFile(bundlePath)
		if err != nil {
			t.Fatal(err)
		}
		b, err := bundle.Unmarshal(data)
		if err != nil {
			t.Fatalf("bundle should decode: %v", err)
		}
		b.ContentHash = "sha256:tampered-content-hash"
		bad, err := bundle.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		tampered := filepath.Join(work, "tampered.qorm.bundle")
		if err := os.WriteFile(tampered, bad, 0o644); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := runQORM(t, bin, nil, "render", tampered, "-o", filepath.Join(work, "nope.html"))
		if code != 1 {
			t.Fatalf("render tampered bundle: exit %d, want 1 (stderr %q)", code, errOut)
		}
		if !strings.Contains(errOut, "content hash mismatch") {
			t.Errorf("render tampered bundle: stderr = %q, want integrity failure", errOut)
		}
	})
}

// TestProbeDocsNameFlag drives `qorm docs --name`: the supplied site name is
// stamped into the rendered HTML header (and -o chooses the output dir).
func TestProbeDocsNameFlag(t *testing.T) {
	bin := buildQORMBinary(t)
	work := t.TempDir()
	docsDir := filepath.Join(work, "src-docs")
	if err := os.MkdirAll(filepath.Join(docsDir, "tutorials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\n\nHello **world**.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "tutorials", "first.md"), []byte("# First\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(work, "out-site")
	out, errOut, code := runQORM(t, bin, nil, "docs", "--docs", docsDir, "-o", outDir, "--name", "MyCustomSite")
	if code != 0 {
		t.Fatalf("docs: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "rendered 2 pages") {
		t.Errorf("docs stdout = %q, want 2 pages", out)
	}
	page, err := os.ReadFile(filepath.Join(outDir, "guide.html"))
	if err != nil {
		t.Fatalf("guide.html missing: %v", err)
	}
	if !strings.Contains(string(page), "MyCustomSite") {
		t.Error("rendered page should carry the --name label in the header")
	}
	if !strings.Contains(string(page), "<strong>world</strong>") {
		t.Error("rendered page should render inline markdown (bold)")
	}
	// The nested page is written under its directory, honouring -o.
	if _, err := os.Stat(filepath.Join(outDir, "tutorials", "first.html")); err != nil {
		t.Errorf("nested page missing under -o dir: %v", err)
	}
}
