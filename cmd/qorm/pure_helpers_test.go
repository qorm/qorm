package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/model"
)

func TestSanitizeID(t *testing.T) {
	cases := map[string]string{
		"My App":    "my_app",
		"Counter-2": "counter_2",
		"already":   "already",
		"!!":        "qorm_app", // nothing survives -> fallback
		"":          "qorm_app",
		"Übung":     "bung", // non-ASCII letters are dropped
	}
	for in, want := range cases {
		if got := sanitizeID(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJSONStringEscapes(t *testing.T) {
	if got := jsonString(`a"b\c`); got != `a\"b\\c` {
		t.Errorf("jsonString = %q, want %q", got, `a\"b\\c`)
	}
}

func TestDnameEsc(t *testing.T) {
	in := `a,b+c"d<e>f;g\h`
	want := `a\,b\+c\"d\<e\>f\;g\\h`
	if got := dnameEsc(in); got != want {
		t.Errorf("dnameEsc(%q) = %q, want %q", in, got, want)
	}
	if got := dnameEsc("plain"); got != "plain" {
		t.Errorf("dnameEsc(plain) = %q", got)
	}
}

func TestAbsOr(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "x")
	if got := absOr(abs); got != abs {
		t.Errorf("absOr(abs) = %q, want identity", got)
	}
	rel := "relative/path"
	got := absOr(rel)
	if !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.FromSlash(rel)) {
		t.Errorf("absOr(rel) = %q, want absolute form of %q", got, rel)
	}
}

func TestPkgID(t *testing.T) {
	cases := map[string]string{
		"My App!": "myapp",
		"9lives":  "a9lives", // must not start with a digit
		"":        "app",
		"***":     "app",
		"counter": "counter",
	}
	for in, want := range cases {
		if got := pkgID(in); got != want {
			t.Errorf("pkgID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestXMLEsc(t *testing.T) {
	if got := xmlEsc(`a & b <c> "d"`); got != "a &amp; b &lt;c&gt; &quot;d&quot;" {
		t.Errorf("xmlEsc = %q", got)
	}
}

func TestAWProvider(t *testing.T) {
	ws := []model.Widget{{Name: "first"}, {Name: "second"}}
	if got := awProvider(ws, "fallback"); got.Name != "first" {
		t.Errorf("awProvider with widgets should return the first, got %q", got.Name)
	}
	if got := awProvider(nil, "fallback"); got.Name != "fallback" {
		t.Errorf("awProvider without widgets should return the placeholder, got %q", got.Name)
	}
}

func TestSpliceUser(t *testing.T) {
	appDir := t.TempDir()
	src := "header\n//USER_CODE\nfooter"

	// No native snippet: the fallback is injected at the marker.
	got := spliceUser(src, "//USER_CODE", appDir, "ops.swift", "/* default */")
	if got != "header\n/* default */\nfooter" {
		t.Errorf("spliceUser fallback = %q", got)
	}

	// With a snippet on disk it replaces the marker instead.
	if err := os.MkdirAll(filepath.Join(appDir, "native"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "native", "ops.swift"), []byte("USER_OPS"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = spliceUser(src, "//USER_CODE", appDir, "ops.swift", "/* default */")
	if got != "header\nUSER_OPS\nfooter" {
		t.Errorf("spliceUser snippet = %q", got)
	}
}

func TestCopyTreeAndDirExists(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "deep.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if string(mustReadFile(t, filepath.Join(dst, "top.txt"))) != "top" {
		t.Error("copyTree lost top.txt content")
	}
	if string(mustReadFile(t, filepath.Join(dst, "nested", "deep.txt"))) != "deep" {
		t.Error("copyTree lost nested content")
	}

	if !dirExists(src) {
		t.Error("dirExists(dir) = false, want true")
	}
	if dirExists(filepath.Join(src, "top.txt")) {
		t.Error("dirExists(file) = true, want false")
	}
	if dirExists(filepath.Join(src, "missing")) {
		t.Error("dirExists(missing) = true, want false")
	}
}

func TestAppIconSizes(t *testing.T) {
	pngMagic := []byte("\x89PNG\r\n\x1a\n")
	for _, n := range []int{16, 192, 512, 1024, 4096} {
		b := appIcon(n)
		if len(b) == 0 || !strings.HasPrefix(string(b), string(pngMagic)) {
			t.Errorf("appIcon(%d) should return a non-empty PNG, got %d bytes", n, len(b))
		}
	}
}

func TestNormPlatform(t *testing.T) {
	cases := map[string]string{
		"macos":   "mac",
		"desktop": "mac",
		"mac":     "mac",
		"ios":     "ios",
		"web":     "web",
	}
	for in, want := range cases {
		if got := normPlatform(in); got != want {
			t.Errorf("normPlatform(%q) = %q, want %q", in, got, want)
		}
	}
}

// nfcApp returns an app whose scene tree uses exactly one capability widget.
func nfcApp() *model.App {
	return &model.App{
		Scenes: map[string]*model.Node{
			"main": {
				Type: "column",
				Children: []*model.Node{
					{Type: "text"},
					{Type: "nfc", Children: []*model.Node{{Type: "text"}}},
				},
			},
			"other": {Type: "text"},
		},
	}
}

func TestUsedFeatures(t *testing.T) {
	feats := usedFeatures(nfcApp())
	if len(feats) != 1 || feats[0] != "nfc" {
		t.Fatalf("usedFeatures = %v, want [nfc]", feats)
	}
	// Portable-only app: no platform-specific features at all.
	portable := &model.App{Scenes: map[string]*model.Node{"m": {Type: "column", Children: []*model.Node{{Type: "text"}, {Type: "button"}}}}}
	if got := usedFeatures(portable); len(got) != 0 {
		t.Errorf("usedFeatures(portable) = %v, want empty", got)
	}
}

func TestSupportedOn(t *testing.T) {
	if got := supportedOn("nfc"); got != "ios, android" {
		t.Errorf("supportedOn(nfc) = %q, want %q", got, "ios, android")
	}
	if got := supportedOn("not-a-capability"); got != "(none)" {
		t.Errorf("supportedOn(unknown) = %q, want (none)", got)
	}
}

func TestWarnPlatformGaps(t *testing.T) {
	app := nfcApp()
	errOut := captureStderr(t, func() {
		if n := warnPlatformGaps(app, "mac"); n != 1 {
			t.Errorf("warnPlatformGaps(mac) = %d, want 1 (nfc unsupported)", n)
		}
	})
	if !strings.Contains(errOut, "nfc") || !strings.Contains(errOut, "works on: ios, android") {
		t.Errorf("gap warning should name the feature and where it works, got %q", errOut)
	}
	if !strings.Contains(errOut, "paid Apple Developer team") {
		t.Errorf("gap warning should carry the capability note, got %q", errOut)
	}

	// No gaps on a platform that supports the feature.
	errOut = captureStderr(t, func() {
		if n := warnPlatformGaps(app, "ios"); n != 0 {
			t.Errorf("warnPlatformGaps(ios) = %d, want 0", n)
		}
	})
	if errOut != "" {
		t.Errorf("no-gap platform should print nothing, got %q", errOut)
	}
}

func TestCheckPlatformMatrix(t *testing.T) {
	errOut := captureStderr(t, func() {
		checkPlatform(nfcApp(), "web")
	})
	if !strings.Contains(errOut, "platform capability matrix") {
		t.Errorf("matrix header missing, got %q", errOut)
	}
	if !strings.Contains(errOut, "nfc") {
		t.Errorf("matrix should list the nfc row, got %q", errOut)
	}

	// Fully portable app: nothing to print.
	portable := &model.App{Scenes: map[string]*model.Node{"m": {Type: "text"}}}
	if out := captureStderr(t, func() { checkPlatform(portable, "web") }); out != "" {
		t.Errorf("portable app should print no matrix, got %q", out)
	}
}

func TestWarnUserNativeGaps(t *testing.T) {
	appDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(appDir, "native"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "native", "ios.swift"), []byte("// ops"), 0o644); err != nil {
		t.Fatal(err)
	}

	errOut := captureStderr(t, func() { warnUserNativeGaps(appDir, "android") })
	if !strings.Contains(errOut, "android.java") || !strings.Contains(errOut, "ios") {
		t.Errorf("android target with ios-only ops should warn, got %q", errOut)
	}

	// Target platform that HAS its snippet: no warning.
	if out := captureStderr(t, func() { warnUserNativeGaps(appDir, "ios") }); out != "" {
		t.Errorf("ios target with ios snippet should not warn, got %q", out)
	}
	// No native dir at all: no warning.
	if out := captureStderr(t, func() { warnUserNativeGaps(t.TempDir(), "android") }); out != "" {
		t.Errorf("app without native/ should not warn, got %q", out)
	}
}

func TestUsedWidgetsCounter(t *testing.T) {
	used := usedWidgets(counterDir())
	for _, typ := range []string{"text", "button", "column", "row"} {
		if !used[typ] {
			t.Errorf("counter should use %q widgets, got %v", typ, used)
		}
	}
	// A missing app dir yields no widgets rather than an error.
	if got := usedWidgets(filepath.Join(t.TempDir(), "missing")); len(got) != 0 {
		t.Errorf("usedWidgets(missing) = %v, want empty", got)
	}
}

func TestAtoiOr(t *testing.T) {
	cases := []struct {
		in   string
		def  int
		want int
	}{
		{"12", 5, 12},
		{"0", 5, 5},  // not > 0
		{"-3", 5, 5}, // not > 0
		{"junk", 5, 5},
		{"", 7, 7},
	}
	for _, c := range cases {
		if got := atoiOr(c.in, c.def); got != c.want {
			t.Errorf("atoiOr(%q, %d) = %d, want %d", c.in, c.def, got, c.want)
		}
	}
}

func TestLatestModTime(t *testing.T) {
	dir := t.TempDir()
	early := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	writeAt := func(rel string, mt time.Time) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	writeAt("a.txt", early)
	writeAt(filepath.Join("sub", "b.txt"), mid)
	writeAt(".hidden.txt", late)                   // hidden file: skipped
	writeAt(filepath.Join(".git", "config"), late) // hidden dir: skipped entirely

	got := latestModTime(dir)
	if !got.Equal(mid) {
		t.Errorf("latestModTime = %v, want %v (hidden entries must not count)", got, mid)
	}
	if z := latestModTime(t.TempDir()); !z.IsZero() {
		t.Errorf("latestModTime(empty dir) = %v, want zero", z)
	}
}

func TestBuildSHA256Sums(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"b.bin": "bee", "a.bin": "ay", "extra.sig": "sig"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A stale manifest and *.sig files must be excluded from a fresh run.
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sums, err := buildSHA256Sums(dir)
	if err != nil {
		t.Fatalf("buildSHA256Sums: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 2 {
		t.Fatalf("manifest should cover exactly a.bin + b.bin, got %q", sums)
	}
	aSum := sha256.Sum256([]byte("ay"))
	if want := hex.EncodeToString(aSum[:]) + "  a.bin"; lines[0] != want {
		t.Errorf("line 0 = %q, want %q (sorted, correct hash)", lines[0], want)
	}
	if !strings.HasSuffix(lines[1], "  b.bin") {
		t.Errorf("line 1 = %q, want b.bin last", lines[1])
	}

	// Empty directory (or only-manifest directory) is an error.
	empty := t.TempDir()
	if _, err := buildSHA256Sums(empty); err == nil {
		t.Error("want error for a dir with no assets")
	}
}

func TestCmdReleaseSignFlow(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath := genKeyPair(t, dir)
	pub, err := keys.LoadPublic(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"qorm-a": "one", "qorm-b": "two"} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("QORM_RELEASE_KEY", privPath)
	out := captureStdout(t, func() {
		if code := cmdReleaseSign([]string{dist}); code != 0 {
			t.Fatalf("__release-sign exited %d", code)
		}
	})
	sumsBytes := mustReadFile(t, filepath.Join(dist, "SHA256SUMS"))
	if !strings.Contains(out, "2 assets") {
		t.Errorf("sign output = %q, want asset count", out)
	}
	// The signature must be a real ed25519 signature over the manifest.
	sigBytes := mustReadFile(t, filepath.Join(dist, "SHA256SUMS.sig"))
	rawSig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBytes)))
	if err != nil {
		t.Fatalf("SHA256SUMS.sig is not base64: %v", err)
	}
	if !ed25519.Verify(pub, sumsBytes, rawSig) {
		t.Error("SHA256SUMS.sig does not verify against the release public key")
	}

	// --verify passes on the pristine directory.
	out = captureStdout(t, func() {
		if code := cmdReleaseSign([]string{"--verify", dist}); code != 0 {
			t.Fatalf("__release-sign --verify exited %d", code)
		}
	})
	if !strings.Contains(out, "verified 2 assets") {
		t.Errorf("verify output = %q, want 2 assets", out)
	}

	// Tampering with one asset breaks --verify.
	if err := os.WriteFile(filepath.Join(dist, "qorm-a"), []byte("evil"), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut := captureStderr(t, func() {
		if code := cmdReleaseSign([]string{"--verify", dist}); code != 1 {
			t.Errorf("verify tampered dist: exit = %d, want 1", code)
		}
	})
	if !strings.Contains(errOut, "sha256 mismatch") {
		t.Errorf("verify tampered: stderr = %q, want sha256 mismatch", errOut)
	}

	// Missing signature file fails --verify.
	if err := os.WriteFile(filepath.Join(dist, "qorm-a"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dist, "SHA256SUMS.sig")); err != nil {
		t.Fatal(err)
	}
	if code := cmdReleaseSign([]string{"--verify", dist}); code != 1 {
		t.Errorf("verify without sig: exit = %d, want 1", code)
	}
}

func TestCmdReleaseSignUnsignedAndArgErrors(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "asset"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without QORM_RELEASE_KEY the manifest is written unsigned.
	t.Setenv("QORM_RELEASE_KEY", "")
	out := captureStdout(t, func() {
		if code := cmdReleaseSign([]string{dist}); code != 0 {
			t.Fatalf("__release-sign exited %d", code)
		}
	})
	if !strings.Contains(out, "unsigned") {
		t.Errorf("sign output = %q, should note the unsigned manifest", out)
	}
	if _, err := os.Stat(filepath.Join(dist, "SHA256SUMS.sig")); !os.IsNotExist(err) {
		t.Errorf("no .sig should be written without a key (stat err = %v)", err)
	}

	// --verify without the key cannot derive the public key.
	if code := cmdReleaseSign([]string{"--verify", dist}); code != 1 {
		t.Errorf("verify without key: exit = %d, want 1", code)
	}

	// An unreadable release key is an error, not a fallback to unsigned.
	t.Setenv("QORM_RELEASE_KEY", filepath.Join(dir, "missing.key"))
	if code := cmdReleaseSign([]string{dist}); code != 1 {
		t.Errorf("bad key path: exit = %d, want 1", code)
	}

	// Argument validation.
	if code := cmdReleaseSign([]string{"--bogus"}); code != 2 {
		t.Errorf("unknown flag: exit = %d, want 2", code)
	}
	if code := cmdReleaseSign([]string{"dir1", "dir2"}); code != 2 {
		t.Errorf("two dirs: exit = %d, want 2", code)
	}
	if code := cmdReleaseSign(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
}

func TestFetchSmallAsset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 4096))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	data, err := fetchSmallAsset(srv.Client(), srv.URL+"/ok", 1<<10)
	if err != nil || string(data) != "payload" {
		t.Fatalf("fetch ok = %q, %v", data, err)
	}
	if _, err := fetchSmallAsset(srv.Client(), srv.URL+"/missing", 1<<10); err == nil ||
		!strings.Contains(err.Error(), "404") {
		t.Errorf("fetch missing should report HTTP 404, got %v", err)
	}
	if _, err := fetchSmallAsset(srv.Client(), srv.URL+"/big", 100); err == nil ||
		!strings.Contains(err.Error(), "limit") {
		t.Errorf("fetch oversize should hit the byte limit, got %v", err)
	}
}

func TestManifestDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("binary"))
	lower := hex.EncodeToString(sum[:])
	upper := strings.ToUpper(lower)

	sums := []byte(fmt.Sprintf("deadbeef  other-asset\n%s  qorm-darwin-arm64\n", lower))
	if got, err := manifestDigest(sums, "qorm-darwin-arm64"); err != nil || got != lower {
		t.Fatalf("manifestDigest = %q, %v; want %q", got, err, lower)
	}
	// Uppercase digests are normalized to lowercase.
	up := []byte(upper + "  qorm-darwin-arm64\n")
	if got, err := manifestDigest(up, "qorm-darwin-arm64"); err != nil || got != lower {
		t.Fatalf("uppercase digest = %q, %v; want lowercase %q", got, err, lower)
	}
	// Binary-mode "*filename" entries match the plain asset name.
	star := []byte(lower + "  *qorm-darwin-arm64\n")
	if got, err := manifestDigest(star, "qorm-darwin-arm64"); err != nil || got != lower {
		t.Fatalf("star-prefixed digest = %q, %v", got, err)
	}
	// Malformed digests are rejected.
	badLen := []byte("abcd  asset\n")
	if _, err := manifestDigest(badLen, "asset"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("short digest should be malformed, got %v", err)
	}
	badHex := []byte(strings.Repeat("z", 64) + "  asset\n")
	if _, err := manifestDigest(badHex, "asset"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("non-hex digest should be malformed, got %v", err)
	}
	// An unlisted asset is an error.
	if _, err := manifestDigest(sums, "qorm-windows-amd64.exe"); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Errorf("unlisted asset should error, got %v", err)
	}
}

// TestSelfSignedTLS exercises cert.go's dev-TLS config: the certificate must
// generate (its validity window is encodable as ASN.1 GeneralizedTime — the
// old year-36812 NotAfter overflowed and made `qorm run --tls` always fail),
// parse, carry the expected localhost names, and complete a real handshake.
func TestSelfSignedTLS(t *testing.T) {
	cfg, err := selfSignedTLS()
	if err != nil {
		t.Fatalf("selfSignedTLS: %v", err)
	}

	if len(cfg.Certificates) != 1 {
		t.Fatalf("config carries %d certificates, want 1", len(cfg.Certificates))
	}
	cert, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	foundLocalhost := false
	for _, d := range cert.DNSNames {
		if d == "localhost" {
			foundLocalhost = true
		}
	}
	if !foundLocalhost {
		t.Errorf("certificate DNS names %v should include localhost", cert.DNSNames)
	}
	foundLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundLoopback = true
		}
	}
	if !foundLoopback {
		t.Errorf("certificate IPs %v should include 127.0.0.1", cert.IPAddresses)
	}
	if !cert.NotAfter.After(time.Now().Add(24 * time.Hour)) {
		t.Errorf("certificate expired or about to: NotAfter = %v", cert.NotAfter)
	}
	// The validity window is a real decade around now: NotBefore slightly
	// backdated (clock-skew tolerance), NotAfter ~10 years out. This pins the
	// fix — the old values (1970 -> year 36812) either predated everything or
	// overflowed ASN.1 GeneralizedTime and never encoded at all.
	now := time.Now()
	if cert.NotBefore.Before(now.Add(-2*time.Hour)) || cert.NotBefore.After(now) {
		t.Errorf("NotBefore = %v, want slightly backdated from now (%v)", cert.NotBefore, now)
	}
	if cert.NotAfter.Before(now.AddDate(9, 0, 0)) || cert.NotAfter.After(now.AddDate(11, 0, 0)) {
		t.Errorf("NotAfter = %v, want about a decade from now (%v)", cert.NotAfter, now)
	}

	// A real handshake: a client trusting only this cert connects by name.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverErr := make(chan error, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			serverErr <- aerr
			return
		}
		sc := tls.Server(c, cfg)
		herr := sc.Handshake()
		sc.Close()
		serverErr <- herr
	}()
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: roots, ServerName: "localhost"})
	if err != nil {
		t.Fatalf("client handshake against the self-signed cert: %v", err)
	}
	conn.Close()
	if err := <-serverErr; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
}

func TestLanIPv4sProperties(t *testing.T) {
	for _, ip := range lanIPv4s() {
		p := net.ParseIP(ip)
		if p == nil {
			t.Errorf("lanIPv4s returned unparsable address %q", ip)
			continue
		}
		if p.To4() == nil {
			t.Errorf("lanIPv4s returned non-IPv4 address %q", ip)
		}
		if p.IsLoopback() || p.IsLinkLocalUnicast() {
			t.Errorf("lanIPv4s should drop loopback/link-local, got %q", ip)
		}
	}
}

func TestEnsureKeystoreNeedsKeytoolToGenerate(t *testing.T) {
	// An empty PATH guarantees keytool is not found, so first-release
	// generation must fail with an actionable error (never a silent no-op).
	t.Setenv("PATH", t.TempDir())
	_, _, _, _, err := ensureKeystore(t.TempDir(), releaseOpts{Release: true})
	if err == nil {
		t.Fatal("want an error when keytool is unavailable")
	}
	if !strings.Contains(err.Error(), "keytool") {
		t.Errorf("error should name keytool, got: %v", err)
	}
}

func TestBundledAppOutsideAppPackage(t *testing.T) {
	// The test binary is not inside a .app bundle, so nothing is bundled.
	if got := bundledApp(); got != "" {
		t.Errorf("bundledApp() = %q, want empty outside a packaged .app", got)
	}
}
