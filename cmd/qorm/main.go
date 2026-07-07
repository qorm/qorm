// Command qorm is the QORM Go runtime: load a QORM app and run it live in the
// browser, or render a static HTML snapshot. Pure Go — cross-compiles to every
// platform with `GOOS`/`GOARCH` and no C toolchain.
package main

import (
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/loader"

	"github.com/qorm/qorm/internal/mcp"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// version is the QORM release version. It defaults to a dev value and is stamped
// at build time via -ldflags "-X main.version=<tag>" (see the build scripts / CI).
var version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		if app := bundledApp(); app != "" {
			os.Exit(cmdRun([]string{app, "--app"}))
		}
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "new":
		os.Exit(cmdNew(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "render":
		os.Exit(cmdRender(os.Args[2:]))
	case "build":
		os.Exit(cmdBuild(os.Args[2:]))
	case "keygen":
		os.Exit(cmdKeygen(os.Args[2:]))
	case "sign":
		os.Exit(cmdSign(os.Args[2:]))
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "mcp":
		os.Exit(cmdMCP(os.Args[2:]))
	case "preview":
		os.Exit(cmdPreview(os.Args[2:]))
	case "package":
		os.Exit(cmdPackage(os.Args[2:]))
	case "measure":
		os.Exit(cmdMeasure(os.Args[2:]))
	case "shot":
		os.Exit(cmdShot(os.Args[2:]))
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "docs":
		os.Exit(cmdDocs(os.Args[2:]))
	case "updates":
		os.Exit(cmdUpdates(os.Args[2:]))
	case "__logwin": // internal: open the separate log window (desktop)
		if len(os.Args) >= 4 {
			runLogWindow(os.Args[2], os.Args[3])
		}
	case "__tray": // internal: system-tray process (desktop)
		if len(os.Args) >= 4 {
			tj := ""
			if len(os.Args) > 4 {
				tj = os.Args[4]
			}
			runTray(os.Args[2], os.Args[3], tj)
		}
	case "version", "--version", "-v":
		fmt.Printf("qorm %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `qorm — QORM Go runtime

usage:
  qorm new <dir> [--name "App Name"]              scaffold a new runnable app
  qorm run <app-dir|bundle> [--trust pub.key] [--revoked list.json] [--app] [--port N=10383] [--no-open]
                                                  run app live (verifies signed bundles; --app = standalone window)
  qorm render <app-dir|scene.json> [-o out.html]  write a static HTML snapshot
  qorm shot <app-dir> -o out.png                 render an app to a PNG (macOS, -tags desktop)
  qorm measure <app-dir> [-o report.json]          render + self-measure layout & styles (needs -tags desktop)
  qorm check <app-dir> (--checks c.json | --audit) [-o r.json]
                                                  verify layout/styles/behaviour vs expectations (or generic audit)
  qorm build <app-dir> -o app.qorm.bundle [--key priv.key]
                                                  compile (+optionally sign) a bundle
  qorm keygen [--out-dir .]                        generate an ed25519 signing keypair
  qorm sign <bundle> --key priv.key [-o out]       sign an existing (e.g. agent-exported) bundle
  qorm verify <bundle> [--trust pub.key] [--revoked list.json]
                                                  verify integrity (+ signature, + revocation)
  qorm mcp <app-dir|bundle> [--trust pub.key]      serve the app to agents over MCP (stdio)
  qorm package <app-dir> -p web|ios|android|mac [-o out] [--dev URL] [--team ID] [--no-branding] [--subscribed]
                                                  package as an installable app for a platform
  qorm preview <package-dir> [--width N] [-o report.json]
                                                  render a packaged app and report its layout
  qorm docs [--docs docs] [-o site]                render the markdown docs to a static HTML site
  qorm updates <bundles-dir> [--port N]            OTA publish server (staged rollout via rollout.json)
  qorm version                                     print the version
`)
}

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

// loadApp loads an app for rendering (no signature enforcement).
func loadApp(path string) (*qrt.Runtime, error) {
	return loadRuntime(path, "", "")
}

// loadRevocation loads a revocation list from a path (nil when path is empty).
func loadRevocation(path string) (bundle.RevocationList, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bundle.LoadRevocation(data)
}

// loadRuntime loads a directory, a scene file, or a compiled bundle. When the
// input is a bundle it is verified before use: integrity always, authenticity
// when a trusted key is supplied, and revocation when a list is supplied.
func loadRuntime(path, trustPath, revokedPath string) (*qrt.Runtime, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		a, err := loader.LoadDir(path)
		if err != nil {
			return nil, err
		}
		return qrt.New(a), nil
	}
	// A file: try to decode it as a bundle first.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if b, err := bundle.Unmarshal(data); err == nil {
		var trust ed25519.PublicKey
		if trustPath != "" {
			if trust, err = keys.LoadPublic(trustPath); err != nil {
				return nil, err
			}
		}
		revoked, err := loadRevocation(revokedPath)
		if err != nil {
			return nil, err
		}
		if err := bundle.VerifyWithRevocation(b, trust, revoked); err != nil {
			return nil, err
		}
		if trust == nil {
			fmt.Fprintf(os.Stderr, "warn: %s passed integrity only — authenticity NOT verified (no --trust key); a crafted bundle would still load. Pass --trust <key.pub> to require a signature.\n", filepath.Base(path))
		}
		app := b.ToApp()
		// so native/web.js (a sibling of the bundle) still loads on desktop
		app.BaseDir = filepath.Dir(path)
		return qrt.New(app), nil
	}
	a, err := loader.LoadFile(path)
	if err != nil {
		return nil, err
	}
	return qrt.New(a), nil
}

func cmdRun(args []string) int {
	port := 10383
	open := true
	appMode := false
	consoleMode := false
	lan := false
	tlsOn := false
	var dir, trust, revoked string
	for i := 0; i < len(args); i++ {
		switch args[i] {
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

// buildServer returns a server for the input. A bundle input yields an
// OTA-capable server (with /update and /rollback, honouring trust + revocation);
// a dir/scene yields a plain live server.
func buildServer(path, trustPath, revokedPath string) (*server.Server, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		if data, rerr := os.ReadFile(path); rerr == nil {
			if b, berr := bundle.Unmarshal(data); berr == nil {
				var trust ed25519.PublicKey
				if trustPath != "" {
					if trust, err = keys.LoadPublic(trustPath); err != nil {
						return nil, "", err
					}
				}
				revoked, rerr := loadRevocation(revokedPath)
				if rerr != nil {
					return nil, "", rerr
				}
				if verr := bundle.VerifyWithRevocation(b, trust, revoked); verr != nil {
					return nil, "", verr
				}
				name := "app"
				if b.Content.App != nil {
					if n, ok := b.Content.App["name"].(string); ok {
						name = n
					}
				}
				srv := server.NewBundle(b, trust, revoked)
				// so the server injects native/web.js sitting beside the bundle
				srv.SetAppBaseDir(filepath.Dir(path))
				return srv, name, nil
			}
		}
	}
	rt, err := loadRuntime(path, trustPath, revokedPath)
	if err != nil {
		return nil, "", err
	}
	return server.New(rt), rt.App.Name, nil
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
	var dir, out, keyPath, version string
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
	if version != "" {
		if err := b.SetVersion(version); err != nil {
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
	fmt.Printf("built %s -> %s (%s, %s)\n", dir, out, b.ContentHash, signed)
	return 0
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
	return 0
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
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
		fmt.Fprintln(os.Stderr, "usage: qorm check <app-dir> (--checks checks.json | --audit) [-o report.json]")
		return 2
	}
	if err := runCheck(in, checks, out, audit, width); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
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

// bundledApp returns the app dir bundled inside a .app (Contents/Resources/app),
// so double-clicking a packaged desktop app runs it. Empty when not bundled.
func bundledApp() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Join(filepath.Dir(exe), "..", "Resources", "app")
	// Prefer the compiled+signed bundle (consistent with the web/mobile package);
	// fall back to raw source for older bundles.
	if _, err := os.Stat(filepath.Join(dir, "bundle.json")); err == nil {
		return filepath.Join(dir, "bundle.json")
	}
	if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err == nil {
		return dir
	}
	return ""
}
