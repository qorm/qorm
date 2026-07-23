package main

import (
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/mcp"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/server"
)

func cmdMCP(args []string) int {
	var in, trust, revoked string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--trust":
			if i+1 < len(args) {
				i++
				trust = args[i]
			}
		case "--revoked":
			if i+1 < len(args) {
				i++
				revoked = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "error: missing <app-dir|bundle>")
		return 2
	}
	rt, err := loadRuntime(in, trust, revoked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := mcp.New(rt, os.Stdin, os.Stdout).Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func cmdRun(args []string) int {
	port := 10383
	open := true
	appMode := false
	consoleMode := false
	lan := false
	tlsOn := false
	mcpReadOnly := false
	noWatch := false
	var dir, trust, revoked, auditLog string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-watch":
			noWatch = true
		case "--mcp-read-only":
			mcpReadOnly = true
		case "--audit-log":
			if i+1 < len(args) {
				i++
				auditLog = args[i]
			}
		case "--lan":
			lan = true
			open = false
		case "--tls":
			tlsOn = true
			lan = true
			open = false
		case "--port":
			if i+1 < len(args) {
				i++
				port, _ = strconv.Atoi(args[i])
			}
		case "--trust":
			if i+1 < len(args) {
				i++
				trust = args[i]
			}
		case "--revoked":
			if i+1 < len(args) {
				i++
				revoked = args[i]
			}
		case "--app":
			appMode = true
		case "--console":
			consoleMode = true
		case "--no-open":
			open = false
		default:
			dir = args[i]
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "error: missing <app-dir>")
		return 2
	}
	srv, name, err := buildServer(dir, trust, revoked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	// Surface the loader's static diagnostics (deprecated attributes,
	// expression type mismatches, ...) without blocking the run: the app
	// still starts, exactly as before.
	printDiagnostics(dir)
	if mcpReadOnly {
		srv.SetMCPReadOnly(true)
	}
	if auditLog != "" {
		if err := srv.SetAuditLog(auditLog); err != nil {
			fmt.Fprintf(os.Stderr, "error: --audit-log: %v\n", err)
			return 1
		}
		fmt.Printf("audit log -> %s (hash-chained; verify with: qorm audit %s)\n", auditLog, auditLog)
	}
	// Dev hot-reload: watch the app's source directory and live-reload every
	// connected client when a file changes (disabled by --no-watch, or when the
	// app is a signed bundle rather than a directory).
	if !noWatch {
		if info, serr := os.Stat(dir); serr == nil && info.IsDir() {
			go watchAndReload(dir, trust, revoked, srv)
			fmt.Println("  hot-reload: watching source — edit a scene/action and it updates live (--no-watch to disable)")
		}
	}
	host := "127.0.0.1"
	if lan {
		host = "0.0.0.0" // reachable by physical devices on the LAN
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		// Preferred port taken (a second QORM app, or a dev server on the same
		// port) — fall back to an ephemeral port so a double-clicked .app doesn't
		// silently fail to launch. The window loads whatever we actually bound.
		ln, err = net.Listen("tcp", fmt.Sprintf("%s:0", host))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	port = ln.Addr().(*net.TCPAddr).Port
	scheme := "http"
	var tlsCfg *tls.Config
	if tlsOn {
		var terr error
		if tlsCfg, terr = selfSignedTLS(); terr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", terr)
			return 1
		}
		scheme = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/", scheme, port)
	fmt.Printf("QORM %q running at %s  (Ctrl-C to stop)\n", name, url)
	fmt.Printf("  agent (MCP over HTTP): %smcp   — AI shares this live session\n", url)
	if mcpReadOnly {
		fmt.Println("  MCP is read-only: mutating agent tools (dispatch/set_state/apply_patch) are disabled")
	}
	if lan {
		printDeviceConnect(port, scheme)
	}

	// In a `-tags desktop` build, host the app in a native WebView window
	// (Wails-style). launchWindow serves and blocks; a false return means this
	// is the pure-Go build with no native WebView, so fall back to a browser.
	if appMode && launchWindow(srv, ln, url, name) {
		return 0
	}
	openURL := url
	if consoleMode {
		openURL = url + "console"
		fmt.Printf("  collaboration console: %sconsole\n", url)
	}
	if open {
		if appMode {
			openAppWindow(openURL)
		} else {
			openBrowser(openURL)
		}
	}
	if tlsCfg != nil {
		srvHTTP := &http.Server{Handler: srv.Handler(), TLSConfig: tlsCfg}
		if err := srvHTTP.ServeTLS(ln, "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func cmdRender(args []string) int {
	var in, out string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "error: missing input")
		return 2
	}
	rt, err := loadApp(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	res := render.Render(rt)
	page := server.Page(rt, res.HTML, 0)
	if out == "" {
		out = filepath.Base(filepath.Clean(in)) + ".html"
	}
	if err := os.WriteFile(out, []byte(page), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("rendered %s -> %s\n", in, out)
	return 0
}

func cmdBuild(args []string) int {
	var dir, out, keyPath, version, requireCaps string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		case "--key":
			if i+1 < len(args) {
				i++
				keyPath = args[i]
			}
		case "--version":
			if i+1 < len(args) {
				i++
				version = args[i]
			}
		case "--require-capability":
			if i+1 < len(args) {
				i++
				requireCaps = args[i]
			}
		default:
			dir = args[i]
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "error: missing <app-dir>")
		return 2
	}
	b, err := bundle.Build(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	// Print static diagnostics to stderr at build time. NOTE(v0.2.1 C3): the
	// release plan also called for writing diagnostics into the bundle
	// metadata; that is deliberately deferred — bundle content just changed
	// for requiredCapabilities, and adding diagnostics would touch the
	// hash/signature surface again. Printing keeps the signal without a
	// format change.
	printDiagnostics(dir)
	if version != "" {
		if err := b.SetVersion(version); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	if caps := splitCapabilityList(requireCaps); len(caps) > 0 {
		if err := b.SetRequiredCapabilities(caps); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	signed := "unsigned"
	if keyPath != "" {
		priv, err := keys.LoadPrivate(keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := b.Sign(priv, keys.KeyID(priv.Public().(ed25519.PublicKey))); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		signed = "signed by key " + b.Signature.KeyID
	}
	data, err := bundle.Marshal(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if out == "" {
		out = filepath.Base(filepath.Clean(dir)) + ".qorm.bundle"
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if caps := b.RequiredCapabilities(); len(caps) > 0 {
		fmt.Printf("built %s -> %s (%s, %s, requires: %s)\n", dir, out, b.ContentHash, signed, strings.Join(caps, ", "))
	} else {
		fmt.Printf("built %s -> %s (%s, %s)\n", dir, out, b.ContentHash, signed)
	}
	return 0
}

// printDiagnostics writes the loader's static diagnostics for an app
// directory to stderr, one per line. Non-directory inputs (scene files,
// compiled bundles) are skipped: diagnostics belong to authoring time.
// Diagnostics never fail the command — `error:`-prefixed entries mark type
// errors, the rest are warnings.
func printDiagnostics(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	docs, err := loader.CollectDocs(path)
	if err != nil {
		return
	}
	for _, d := range loader.FromDocs(docs).Diagnostics {
		fmt.Fprintln(os.Stderr, d)
	}
}

// splitCapabilityList parses a comma-separated --require-capability value
// ("camera,location") into a clean slice (trimmed, empties dropped).
func splitCapabilityList(s string) []string {
	var caps []string
	for _, c := range strings.Split(s, ",") {
		if c = strings.TrimSpace(c); c != "" {
			caps = append(caps, c)
		}
	}
	return caps
}

func cmdKeygen(args []string) int {
	outDir := "."
	for i := 0; i < len(args); i++ {
		if args[i] == "--out-dir" && i+1 < len(args) {
			i++
			outDir = args[i]
		}
	}
	pub, priv, err := keys.Generate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	privPath := filepath.Join(outDir, "qorm_key")
	pubPath := filepath.Join(outDir, "qorm_key.pub")
	if err := keys.WritePrivate(privPath, priv); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := keys.WritePublic(pubPath, pub); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("generated keypair (id %s)\n  private: %s\n  public:  %s\n",
		keys.KeyID(pub), privPath, pubPath)
	return 0
}

func cmdSign(args []string) int {
	var in, out, keyPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		case "--key":
			if i+1 < len(args) {
				i++
				keyPath = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" || keyPath == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm sign <bundle> --key priv.key [-o out]")
		return 2
	}
	data, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	b, err := bundle.Unmarshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	priv, err := keys.LoadPrivate(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := b.Sign(priv, keys.KeyID(priv.Public().(ed25519.PublicKey))); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	signed, err := bundle.Marshal(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if out == "" {
		out = in
	}
	if err := os.WriteFile(out, signed, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("signed %s -> %s (key %s)\n", in, out, b.Signature.KeyID)
	return 0
}

func cmdVerify(args []string) int {
	var in, trustPath, revokedPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--trust":
			if i+1 < len(args) {
				i++
				trustPath = args[i]
			}
		case "--revoked":
			if i+1 < len(args) {
				i++
				revokedPath = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "error: missing <bundle>")
		return 2
	}
	data, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	b, err := bundle.Unmarshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "VERIFY FAILED: not a valid QORM bundle (corrupt/tampered or not a bundle file): %v\n", err)
		return 1
	}
	var trust ed25519.PublicKey
	if trustPath != "" {
		trust, err = keys.LoadPublic(trustPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	var revoked bundle.RevocationList
	if revokedPath != "" {
		rdata, rerr := os.ReadFile(revokedPath)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", rerr)
			return 1
		}
		if revoked, err = bundle.LoadRevocation(rdata); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	// Revocation is checked against the ACTUAL verifying key (see
	// bundle.VerifyWithRevocation), so it is meaningless — and a fail-open trap —
	// without a trust key: with trust==nil the library does integrity only and
	// silently ignores the revocation list. Refuse rather than report a green
	// "integrity + revocation" for a bundle whose revocation was never checked.
	if revoked != nil && trust == nil {
		fmt.Fprintln(os.Stderr, "error: --revoked requires --trust <key.pub>: revocation is checked against the trusted signing key, so a trust key is mandatory")
		return 2
	}
	if err := bundle.VerifyWithRevocation(b, trust, revoked); err != nil {
		fmt.Fprintf(os.Stderr, "VERIFY FAILED: %v\n", err)
		return 1
	}
	scope := "integrity"
	if trust != nil {
		scope = "integrity + signature (key " + b.Signature.KeyID + ")"
	}
	if revoked != nil {
		scope += " + revocation"
	}
	fmt.Printf("OK: %s verified (%s)\n", in, scope)
	if caps := b.RequiredCapabilities(); len(caps) > 0 {
		fmt.Printf("requires capabilities: %s\n", strings.Join(caps, ", "))
	}
	return 0
}

// cmdMeasure renders the app in a native WebView, lets it self-measure its own
// layout + computed styles, and prints a complete report that joins the user's
// intent (each node's type/text/binding) with the rendered result (rect +
// styles). This is how an AI gets complete, precise, verifiable results.
func cmdMeasure(args []string) int {
	var in, out string
	width := 400
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--width":
			if i+1 < len(args) {
				i++
				width = atoiOr(args[i], 400)
			}
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "error: missing <app-dir>")
		return 2
	}
	if err := runMeasure(in, out, width); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func cmdCheck(args []string) int {
	var in, checks, out string
	audit := false
	width := 400
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--width":
			if i+1 < len(args) {
				i++
				width = atoiOr(args[i], 400)
			}
		case "--audit":
			audit = true
		case "--checks":
			if i+1 < len(args) {
				i++
				checks = args[i]
			}
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		default:
			in = args[i]
		}
	}
	if in == "" || (checks == "" && !audit) {
		fmt.Fprintln(os.Stderr, "usage: qorm check <app-dir> (--checks checks.json | --audit) [--width N] [-o report.json]")
		return 2
	}
	if err := runCheck(in, checks, out, audit, width); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func cmdPreview(args []string) int {
	dir, out, eval := "", "", ""
	width := 400
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--width":
			if i+1 < len(args) {
				i++
				width = atoiOr(args[i], 400)
			}
		case "--eval":
			if i+1 < len(args) {
				i++
				eval = args[i]
			}
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		default:
			dir = args[i]
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm preview <package-dir> [--width N] [--eval JS] [-o report.json]")
		return 2
	}
	if err := runPreview(dir, width, eval, out); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// cmdAudit verifies a hash-chained activity audit log written by
// `qorm run --audit-log <file>`: any edited, dropped, reordered or
// re-attributed entry breaks the chain and is reported with its position.
func cmdAudit(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: qorm audit <audit-log.jsonl>")
		return 2
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()
	n, err := server.VerifyAuditChain(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUDIT FAIL after %d verified entries: %v\n", n, err)
		return 1
	}
	fmt.Printf("AUDIT OK: %d entries, hash chain intact\n", n)
	return 0
}

// watchAndReload polls the app's source directory and hot-reloads the running
// server whenever a file changes. It re-parses the app and swaps it into the
// live session (preserving state/scene/viewport). A parse error mid-edit (e.g. a
// half-written JSON file) is reported and the current app kept, so a bad save
// never takes the app down — the next good save recovers. Dependency-free: a
// coarse mtime poll, which is plenty for the small handful of files in an app.
func watchAndReload(dir, trust, revoked string, srv *server.Server) {
	last := latestModTime(dir)
	for {
		time.Sleep(400 * time.Millisecond)
		m := latestModTime(dir)
		if !m.After(last) {
			continue
		}
		last = m
		rt, err := loadRuntime(dir, trust, revoked)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  hot-reload: %v — keeping the current app\n", err)
			continue
		}
		srv.Reload(rt)
		fmt.Printf("  ↻ hot-reloaded (%s)\n", time.Now().Format("15:04:05"))
	}
}

// latestModTime returns the newest mod time among the app's source files. Hidden
// entries (.git, editor temp/swap files) are skipped so they don't trigger
// spurious reloads.
func latestModTime(dir string) time.Time {
	var t time.Time
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if name != "." && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
		return nil
	})
	return t
}
