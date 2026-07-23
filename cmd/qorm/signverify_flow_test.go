package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
)

// captureStderr runs f with os.Stderr redirected to a pipe and returns what
// was printed (mirror of captureStdout in capability_bundle_test.go).
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	f()
	_ = w.Close()
	out := new(strings.Builder)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := r.Read(buf)
		out.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	return out.String()
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// auditEntry mirrors internal/server.LogEntry's wire format so tests can forge
// a self-consistent hash chain: hash = sha256(prev|seq|ts|source|detail).
type auditEntry struct {
	Seq    int    `json:"seq"`
	Time   string `json:"time"`
	TS     string `json:"ts,omitempty"`
	Source string `json:"source"`
	Detail string `json:"detail"`
	Hash   string `json:"hash,omitempty"`
}

// buildAuditChain writes n correctly chained audit-log entries. The hash
// mirrors server.auditHash: it covers EVERY persisted field — including the
// display time — so editing any part of a line breaks the chain.
func buildAuditChain(t *testing.T, n int) []byte {
	t.Helper()
	var b strings.Builder
	prev := ""
	for i := 1; i <= n; i++ {
		e := auditEntry{
			Seq:    i,
			Time:   "12:00:00",
			TS:     "2026-01-02T12:00:00Z",
			Source: "human",
			Detail: fmt.Sprintf("entry %d", i),
		}
		h := sha256.Sum256([]byte(prev + "|" + strconv.Itoa(e.Seq) + "|" + e.Time + "|" + e.TS + "|" + e.Source + "|" + e.Detail))
		e.Hash = hex.EncodeToString(h[:])
		prev = e.Hash
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal audit entry: %v", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// genKeyPair writes a fresh keypair under dir and returns the file paths.
func genKeyPair(t *testing.T, dir string) (privPath, pubPath string) {
	t.Helper()
	pub, priv, err := keys.Generate()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privPath = filepath.Join(dir, "sign_key")
	pubPath = filepath.Join(dir, "sign_key.pub")
	if err := keys.WritePrivate(privPath, priv); err != nil {
		t.Fatal(err)
	}
	if err := keys.WritePublic(pubPath, pub); err != nil {
		t.Fatal(err)
	}
	return privPath, pubPath
}

func counterDir() string {
	return filepath.Join("..", "..", "examples", "counter")
}

func buildCounterBundle(t *testing.T, out string, keyPath string) {
	t.Helper()
	args := []string{counterDir(), "-o", out}
	if keyPath != "" {
		args = append(args, "--key", keyPath)
	}
	captureStdout(t, func() {
		if code := cmdBuild(args); code != 0 {
			t.Fatalf("cmdBuild(%v) exited %d", args, code)
		}
	})
}

func TestCmdKeygenWritesKeypair(t *testing.T) {
	dir := t.TempDir()
	out := captureStdout(t, func() {
		if code := cmdKeygen([]string{"--out-dir", dir}); code != 0 {
			t.Fatalf("cmdKeygen exited %d", code)
		}
	})
	priv, err := keys.LoadPrivate(filepath.Join(dir, "qorm_key"))
	if err != nil {
		t.Fatalf("private key should load: %v", err)
	}
	pub, err := keys.LoadPublic(filepath.Join(dir, "qorm_key.pub"))
	if err != nil {
		t.Fatalf("public key should load: %v", err)
	}
	if !strings.Contains(out, keys.KeyID(pub)) {
		t.Errorf("keygen output should name key id %s, got %q", keys.KeyID(pub), out)
	}
	// The pair must actually sign/verify.
	msg := []byte("qorm")
	if sig := ed25519.Sign(priv, msg); !ed25519.Verify(pub, msg, sig) {
		t.Error("generated keypair does not round-trip a signature")
	}
}

func TestCmdKeygenDefaultsToCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	captureStdout(t, func() {
		if code := cmdKeygen(nil); code != 0 {
			t.Fatalf("cmdKeygen exited %d", code)
		}
	})
	for _, f := range []string{"qorm_key", "qorm_key.pub"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("keygen should write %s into the cwd: %v", f, err)
		}
	}
}

func TestCmdSignVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath := genKeyPair(t, dir)
	unsigned := filepath.Join(dir, "counter.qorm.bundle")
	buildCounterBundle(t, unsigned, "")

	// Integrity-only verification passes without a trust key.
	out := captureStdout(t, func() {
		if code := cmdVerify([]string{unsigned}); code != 0 {
			t.Fatalf("verify unsigned exited %d", code)
		}
	})
	if !strings.Contains(out, "(integrity)") {
		t.Errorf("unsigned verify should report integrity-only scope, got %q", out)
	}

	// With a trust key, an unsigned bundle must be rejected.
	if code := cmdVerify([]string{unsigned, "--trust", pubPath}); code != 1 {
		t.Errorf("verify unsigned with --trust: exit = %d, want 1", code)
	}

	// Sign into a separate output; verification then covers the signature.
	signed := filepath.Join(dir, "signed.qorm.bundle")
	signOut := captureStdout(t, func() {
		if code := cmdSign([]string{unsigned, "--key", privPath, "-o", signed}); code != 0 {
			t.Fatalf("cmdSign exited %d", code)
		}
	})
	pub, err := keys.LoadPublic(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signOut, keys.KeyID(pub)) {
		t.Errorf("sign output should name the key id, got %q", signOut)
	}
	verifyOut := captureStdout(t, func() {
		if code := cmdVerify([]string{signed, "--trust", pubPath}); code != 0 {
			t.Fatalf("verify signed exited %d", code)
		}
	})
	if !strings.Contains(verifyOut, "integrity + signature (key "+keys.KeyID(pub)+")") {
		t.Errorf("verify signed should report the signature scope, got %q", verifyOut)
	}

	// The signature must genuinely cover the content: flipping a payload byte
	// after signing breaks verification.
	raw := mustReadFile(t, signed)
	raw[len(raw)/2] ^= 0x01
	tampered := filepath.Join(dir, "tampered.qorm.bundle")
	if err := os.WriteFile(tampered, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		if code := cmdVerify([]string{tampered, "--trust", pubPath}); code != 1 {
			t.Errorf("verify tampered: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "VERIFY FAILED") {
		t.Errorf("verify tampered: stderr = %q, want VERIFY FAILED", errOut)
	}
}

func TestCmdSignInPlaceOverwritesInput(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath := genKeyPair(t, dir)
	path := filepath.Join(dir, "inplace.qorm.bundle")
	buildCounterBundle(t, path, "")

	// No -o: the input file itself becomes the signed bundle.
	captureStdout(t, func() {
		if code := cmdSign([]string{path, "--key", privPath}); code != 0 {
			t.Fatalf("cmdSign exited %d", code)
		}
	})
	b, err := bundle.Unmarshal(mustReadFile(t, path))
	if err != nil {
		t.Fatalf("re-read signed bundle: %v", err)
	}
	if b.Signature == nil {
		t.Fatal("in-place sign should leave a signature behind")
	}
	pub, err := keys.LoadPublic(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	if b.Signature.KeyID != keys.KeyID(pub) {
		t.Errorf("signature key id = %q, want %q", b.Signature.KeyID, keys.KeyID(pub))
	}
}

func TestCmdSignArgumentValidation(t *testing.T) {
	if code := cmdSign(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
	if code := cmdSign([]string{"bundle.bin"}); code != 2 {
		t.Errorf("missing --key: exit = %d, want 2", code)
	}
	if code := cmdSign([]string{"--key", "k"}); code != 2 {
		t.Errorf("missing bundle: exit = %d, want 2", code)
	}
	dir := t.TempDir()
	privPath, _ := genKeyPair(t, dir)
	if code := cmdSign([]string{filepath.Join(dir, "missing.bin"), "--key", privPath}); code != 1 {
		t.Errorf("missing bundle file: exit = %d, want 1", code)
	}
	notBundle := filepath.Join(dir, "not-a-bundle.bin")
	if err := os.WriteFile(notBundle, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdSign([]string{notBundle, "--key", privPath}); code != 1 {
		t.Errorf("non-bundle input: exit = %d, want 1", code)
	}
	unsigned := filepath.Join(dir, "u.qorm.bundle")
	buildCounterBundle(t, unsigned, "")
	if code := cmdSign([]string{unsigned, "--key", filepath.Join(dir, "missing.key")}); code != 1 {
		t.Errorf("missing key file: exit = %d, want 1", code)
	}
}

func TestCmdVerifyArgumentValidation(t *testing.T) {
	if code := cmdVerify(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
	dir := t.TempDir()
	if code := cmdVerify([]string{filepath.Join(dir, "missing.bin")}); code != 1 {
		t.Errorf("missing file: exit = %d, want 1", code)
	}
	junk := filepath.Join(dir, "junk.bin")
	if err := os.WriteFile(junk, []byte("definitely not a bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		if code := cmdVerify([]string{junk}); code != 1 {
			t.Errorf("junk file: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "not a valid QORM bundle") {
		t.Errorf("junk verify: stderr = %q, want decode failure", errOut)
	}

	// Trust / revocation inputs are validated before verification.
	unsigned := filepath.Join(dir, "u.qorm.bundle")
	buildCounterBundle(t, unsigned, "")
	if code := cmdVerify([]string{unsigned, "--trust", filepath.Join(dir, "missing.pub")}); code != 1 {
		t.Errorf("missing trust key: exit = %d, want 1", code)
	}
	if code := cmdVerify([]string{unsigned, "--revoked", filepath.Join(dir, "missing.json")}); code != 1 {
		t.Errorf("missing revocation list: exit = %d, want 1", code)
	}
	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdVerify([]string{unsigned, "--revoked", badJSON}); code != 1 {
		t.Errorf("malformed revocation list: exit = %d, want 1", code)
	}
	notAKey := filepath.Join(dir, "not-a-key.pub")
	if err := os.WriteFile(notAKey, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdVerify([]string{unsigned, "--trust", notAKey}); code != 1 {
		t.Errorf("invalid trust key: exit = %d, want 1", code)
	}
}

func TestCmdVerifyRevocationScopes(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath := genKeyPair(t, dir)
	signed := filepath.Join(dir, "s.qorm.bundle")
	buildCounterBundle(t, signed, privPath)
	pub, err := keys.LoadPublic(pubPath)
	if err != nil {
		t.Fatal(err)
	}

	// A list that does NOT contain our key verifies and extends the scope.
	okList := filepath.Join(dir, "others.json")
	if err := os.WriteFile(okList, []byte(`["someotherkeyid"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := cmdVerify([]string{signed, "--trust", pubPath, "--revoked", okList}); code != 0 {
			t.Fatalf("verify with clean revocation list exited %d", code)
		}
	})
	if !strings.Contains(out, "+ revocation") {
		t.Errorf("verify scope should include revocation, got %q", out)
	}

	// Revoking the signing key (both list formats) must fail closed.
	for i, body := range []string{
		`["` + keys.KeyID(pub) + `"]`,
		`{"revoked":["` + keys.KeyID(pub) + `"]}`,
	} {
		list := filepath.Join(dir, fmt.Sprintf("revoked%d.json", i))
		if err := os.WriteFile(list, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		errOut := captureStderr(t, func() {
			if code := cmdVerify([]string{signed, "--trust", pubPath, "--revoked", list}); code != 1 {
				t.Errorf("revoked verify: exit = %d, want 1", code)
			}
		})
		if !strings.Contains(errOut, "VERIFY FAILED") {
			t.Errorf("revoked verify: stderr = %q, want VERIFY FAILED", errOut)
		}
	}
}

// TestCmdVerifyRevokedRequiresTrust guards a fail-open trap: with --revoked but
// no --trust key, revocation cannot be checked (it is keyed on the verifying
// key), so the command must refuse rather than print a green
// "OK ... (integrity + revocation)" for a bundle whose revocation was never
// verified — even one signed by a revoked key.
func TestCmdVerifyRevokedRequiresTrust(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath := genKeyPair(t, dir)
	signed := filepath.Join(dir, "s.qorm.bundle")
	buildCounterBundle(t, signed, privPath)
	pub, err := keys.LoadPublic(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	// A revocation list naming the ACTUAL signing key: if this were honoured the
	// bundle would be rejected, so a green OK here proves revocation was skipped.
	revoked := filepath.Join(dir, "revoked.json")
	if err := os.WriteFile(revoked, []byte(`["`+keys.KeyID(pub)+`"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		if code := cmdVerify([]string{signed, "--revoked", revoked}); code != 2 {
			t.Errorf("verify --revoked without --trust: exit = %d, want 2", code)
		}
	})
	if !strings.Contains(errOut, "--revoked requires --trust") {
		t.Errorf("verify --revoked without --trust: stderr = %q, want the requires---trust error", errOut)
	}
	// Supplying the trust key makes the same invocation fail closed on revocation.
	errOut = captureStderr(t, func() {
		if code := cmdVerify([]string{signed, "--trust", pubPath, "--revoked", revoked}); code != 1 {
			t.Errorf("verify --trust --revoked (signer revoked): exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "VERIFY FAILED") {
		t.Errorf("verify with trust + revoked signer: stderr = %q, want VERIFY FAILED", errOut)
	}
}

func TestCmdBuildVersionAndDefaultOutput(t *testing.T) {
	// Resolve the counter path to absolute up front: the default-output
	// assertion chdirs away from the package dir, which would break the
	// relative counterDir() otherwise.
	counterAbs, err := filepath.Abs(counterDir())
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "v.qorm.bundle")
	captureStdout(t, func() {
		if code := cmdBuild([]string{counterAbs, "-o", out, "--version", "9.9.9"}); code != 0 {
			t.Fatalf("cmdBuild exited %d", code)
		}
	})
	b, err := bundle.Unmarshal(mustReadFile(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Version(); got != "9.9.9" {
		t.Errorf("bundle version = %q, want 9.9.9", got)
	}

	// Without -o the output lands in the cwd as <dir-base>.qorm.bundle.
	t.Chdir(dir)
	captureStdout(t, func() {
		if code := cmdBuild([]string{counterAbs}); code != 0 {
			t.Fatalf("cmdBuild default output exited %d", code)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "counter.qorm.bundle")); err != nil {
		t.Errorf("default bundle name should be counter.qorm.bundle: %v", err)
	}
}

func TestCmdBuildErrors(t *testing.T) {
	if code := cmdBuild(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
	if code := cmdBuild([]string{filepath.Join(t.TempDir(), "nope")}); code != 1 {
		t.Errorf("missing app dir: exit = %d, want 1", code)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "x.qorm.bundle")
	if code := cmdBuild([]string{counterDir(), "-o", out, "--key", filepath.Join(dir, "missing.key")}); code != 1 {
		t.Errorf("missing signing key: exit = %d, want 1", code)
	}
}

func TestCmdRender(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "renapp")
	if code := cmdNew([]string{app, "--name", "Render Target"}); code != 0 {
		t.Fatalf("cmdNew exited %d", code)
	}

	// Explicit -o.
	outPath := filepath.Join(dir, "explicit.html")
	msg := captureStdout(t, func() {
		if code := cmdRender([]string{app, "-o", outPath}); code != 0 {
			t.Fatalf("cmdRender exited %d", code)
		}
	})
	html := string(mustReadFile(t, outPath))
	if !strings.Contains(html, "Render Target") {
		t.Error("rendered page should contain the app name")
	}
	if !strings.Contains(msg, "rendered") {
		t.Errorf("render output = %q, want progress line", msg)
	}

	// Default output name derives from the input path, in the cwd.
	t.Chdir(dir)
	captureStdout(t, func() {
		if code := cmdRender([]string{app}); code != 0 {
			t.Fatalf("cmdRender default exited %d", code)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "renapp.html")); err != nil {
		t.Errorf("default render output should be renapp.html: %v", err)
	}

	// A bare scene file loads and renders too.
	sceneOut := filepath.Join(dir, "scene.html")
	captureStdout(t, func() {
		if code := cmdRender([]string{filepath.Join(app, "scenes", "main.json"), "-o", sceneOut}); code != 0 {
			t.Fatalf("cmdRender scene exited %d", code)
		}
	})
	if scene := string(mustReadFile(t, sceneOut)); !strings.Contains(scene, "Render Target") {
		t.Error("scene render should contain the scene's title text")
	}

	// Errors.
	if code := cmdRender(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
	if code := cmdRender([]string{filepath.Join(dir, "missing"), "-o", outPath}); code != 1 {
		t.Errorf("missing input: exit = %d, want 1", code)
	}
}

func TestCmdAudit(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.jsonl")
	if err := os.WriteFile(good, buildAuditChain(t, 4), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := cmdAudit([]string{good}); code != 0 {
			t.Fatalf("audit good log exited %d", code)
		}
	})
	if !strings.Contains(out, "AUDIT OK: 4 entries") {
		t.Errorf("audit output = %q, want 4 verified entries", out)
	}

	// Reordering entries breaks the chain.
	lines := strings.Split(strings.TrimSpace(string(mustReadFile(t, good))), "\n")
	reordered := filepath.Join(dir, "reordered.jsonl")
	if err := os.WriteFile(reordered, []byte(lines[1]+"\n"+lines[0]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		if code := cmdAudit([]string{reordered}); code != 1 {
			t.Errorf("audit reordered: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "AUDIT FAIL") {
		t.Errorf("audit reordered: stderr = %q, want AUDIT FAIL", errOut)
	}

	// A truncated chain (dropping the middle entry) also fails, at position 2.
	dropped := filepath.Join(dir, "dropped.jsonl")
	if err := os.WriteFile(dropped, []byte(lines[0]+"\n"+lines[2]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errOut = captureStderr(t, func() {
		if code := cmdAudit([]string{dropped}); code != 1 {
			t.Errorf("audit dropped: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "after 1 verified entries") {
		t.Errorf("audit dropped: stderr = %q, want failure after entry 1", errOut)
	}

	// Malformed JSON is reported, not a panic.
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte("{not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdAudit([]string{bad}); code != 1 {
		t.Errorf("audit malformed: exit = %d, want 1", code)
	}

	// An empty log is a valid (vacuous) chain.
	empty := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if code := cmdAudit([]string{empty}); code != 0 {
			t.Fatalf("audit empty log exited %d", code)
		}
	})
	if !strings.Contains(out, "AUDIT OK: 0 entries") {
		t.Errorf("audit empty: output = %q, want 0 entries", out)
	}

	// Argument handling.
	if code := cmdAudit(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
	if code := cmdAudit([]string{"a", "b"}); code != 2 {
		t.Errorf("two args: exit = %d, want 2", code)
	}
	if code := cmdAudit([]string{filepath.Join(dir, "missing.jsonl")}); code != 1 {
		t.Errorf("missing file: exit = %d, want 1", code)
	}
}

func TestCmdDocs(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	for rel, body := range map[string]string{
		"intro.md":       "# Intro\n\nWelcome.\n",
		"guide/start.md": "# Start\n\nGo.\n",
	} {
		p := filepath.Join(docs, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site := filepath.Join(dir, "site")
	out := captureStdout(t, func() {
		if code := cmdDocs([]string{"--docs", docs, "-o", site, "--name", "My Site"}); code != 0 {
			t.Fatalf("cmdDocs exited %d", code)
		}
	})
	if !strings.Contains(out, "rendered 2 pages") {
		t.Errorf("docs output = %q, want 2 pages", out)
	}
	for _, rel := range []string{"index.html", "intro.html", filepath.Join("guide", "start.html")} {
		if _, err := os.Stat(filepath.Join(site, rel)); err != nil {
			t.Errorf("docs should render %s: %v", rel, err)
		}
	}

	// Empty docs tree and missing tree both fail with exit 1.
	emptyDocs := filepath.Join(dir, "empty")
	if err := os.MkdirAll(emptyDocs, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := cmdDocs([]string{"--docs", emptyDocs, "-o", site}); code != 1 {
		t.Errorf("empty docs dir: exit = %d, want 1", code)
	}
	if code := cmdDocs([]string{"--docs", filepath.Join(dir, "missing"), "-o", site}); code != 1 {
		t.Errorf("missing docs dir: exit = %d, want 1", code)
	}
}

func TestCmdRunAndMCPFlagValidation(t *testing.T) {
	if code := cmdRun(nil); code != 2 {
		t.Errorf("run no args: exit = %d, want 2", code)
	}
	if code := cmdRun([]string{filepath.Join(t.TempDir(), "missing")}); code != 1 {
		t.Errorf("run missing app: exit = %d, want 1", code)
	}
	if code := cmdMCP(nil); code != 2 {
		t.Errorf("mcp no args: exit = %d, want 2", code)
	}
	if code := cmdMCP([]string{filepath.Join(t.TempDir(), "missing")}); code != 1 {
		t.Errorf("mcp missing app: exit = %d, want 1", code)
	}
}

func TestDesktopOnlyCommandsRefuseInPureBuild(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	if code := cmdNew([]string{app}); code != 0 {
		t.Fatalf("cmdNew exited %d", code)
	}

	if code := cmdMeasure(nil); code != 2 {
		t.Errorf("measure no args: exit = %d, want 2", code)
	}
	errOut := captureStderr(t, func() {
		if code := cmdMeasure([]string{app}); code != 1 {
			t.Errorf("measure pure build: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "-tags desktop") {
		t.Errorf("measure stderr = %q, should name the missing tag", errOut)
	}

	if code := cmdCheck(nil); code != 2 {
		t.Errorf("check no args: exit = %d, want 2", code)
	}
	if code := cmdCheck([]string{app}); code != 2 {
		t.Errorf("check without --checks/--audit: exit = %d, want 2", code)
	}
	if code := cmdCheck([]string{app, "--audit"}); code != 1 {
		t.Errorf("check pure build: exit = %d, want 1", code)
	}

	if code := cmdPreview(nil); code != 2 {
		t.Errorf("preview no args: exit = %d, want 2", code)
	}
	if code := cmdPreview([]string{app}); code != 1 {
		t.Errorf("preview pure build: exit = %d, want 1", code)
	}

	if code := cmdShot([]string{app}); code != 2 {
		t.Errorf("shot pure build: exit = %d, want 2", code)
	}
}

func TestCmdUpdatesRejectsMalformedRollout(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rollout.json"), []byte("{{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdUpdates([]string{dir}); code != 1 {
		t.Errorf("updates malformed rollout: exit = %d, want 1", code)
	}
}

func TestLoadRuntimeAndBuildServer(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	if code := cmdNew([]string{app, "--name", "Loader App"}); code != 0 {
		t.Fatalf("cmdNew exited %d", code)
	}

	// Directory input loads without trust.
	rt, err := loadRuntime(app, "", "")
	if err != nil {
		t.Fatalf("loadRuntime(dir): %v", err)
	}
	if rt.App.Name != "Loader App" {
		t.Errorf("app name = %q, want Loader App", rt.App.Name)
	}

	// Signed bundle input: passes with the right trust key.
	privPath, pubPath := genKeyPair(t, dir)
	bundlePath := filepath.Join(dir, "app.qorm.bundle")
	buildCounterBundle(t, bundlePath, privPath)
	captureStderr(t, func() {
		if _, err := loadRuntime(bundlePath, pubPath, ""); err != nil {
			t.Fatalf("loadRuntime(signed bundle): %v", err)
		}
	})

	// Without trust it still loads (integrity only) but warns on stderr.
	warn := captureStderr(t, func() {
		if _, err := loadRuntime(bundlePath, "", ""); err != nil {
			t.Fatalf("loadRuntime(bundle, no trust): %v", err)
		}
	})
	if !strings.Contains(warn, "authenticity NOT verified") {
		t.Errorf("no-trust bundle load should warn, stderr = %q", warn)
	}

	// A trust key that did not sign the bundle fails closed.
	otherDir := filepath.Join(dir, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, otherPub := genKeyPair(t, otherDir)
	if _, err := loadRuntime(bundlePath, otherPub, ""); err == nil {
		t.Fatal("loadRuntime with wrong trust key should fail")
	}
	// An unreadable trust path fails too.
	if _, err := loadRuntime(bundlePath, filepath.Join(dir, "missing.pub"), ""); err == nil {
		t.Fatal("loadRuntime with missing trust key should fail")
	}

	// loadApp is the no-signature facade over a directory.
	if _, err := loadApp(app); err != nil {
		t.Fatalf("loadApp(dir): %v", err)
	}
	if _, err := loadApp(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("loadApp(missing) should fail")
	}

	// loadRevocation: empty path -> nil list, no error.
	if rl, err := loadRevocation(""); err != nil || rl != nil {
		t.Fatalf("loadRevocation(\"\") = %v, %v; want nil, nil", rl, err)
	}
	if _, err := loadRevocation(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("loadRevocation(missing) should fail")
	}

	// buildServer names the app from the bundle content.
	captureStderr(t, func() {
		_, name, err := buildServer(bundlePath, pubPath, "")
		if err != nil {
			t.Fatalf("buildServer(bundle): %v", err)
		}
		if name != "QORM Premium Counter" {
			t.Errorf("bundle server name = %q, want QORM Premium Counter", name)
		}
	})
	// ... and from the runtime for a directory.
	_, name, err := buildServer(app, "", "")
	if err != nil {
		t.Fatalf("buildServer(dir): %v", err)
	}
	if name != "Loader App" {
		t.Errorf("dir server name = %q, want Loader App", name)
	}
	if _, _, err := buildServer(filepath.Join(dir, "missing"), "", ""); err == nil {
		t.Fatal("buildServer(missing) should fail")
	}
}
