// Command qorm is the QORM Go runtime: load a QORM app and run it live in the
// browser, or render a static HTML snapshot. Pure Go — cross-compiles to every
// platform with `GOOS`/`GOARCH` and no C toolchain.
package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/loader"

	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// version is the QORM release version. It defaults to a dev value and is stamped
// at build time via -ldflags "-X main.version=<tag>" (see the build scripts / CI).
var version = "0.1.3"

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
	case "audit":
		os.Exit(cmdAudit(os.Args[2:]))
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
	case "update":
		os.Exit(cmdUpdate(os.Args[2:]))
	case "__release-sign": // internal: checksum + ed25519-sign release assets (CI)
		os.Exit(cmdReleaseSign(os.Args[2:]))
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
  qorm run <app-dir|bundle> [--trust pub.key] [--revoked list.json] [--app] [--port N=10383] [--no-open] [--mcp-read-only]
                                                  run app live (verifies signed bundles; --app = standalone window;
                                                  --mcp-read-only = agents may inspect but not mutate)
  qorm render <app-dir|scene.json> [-o out.html]  write a static HTML snapshot
  qorm shot <app-dir> -o out.png                 render an app to a PNG (macOS, -tags desktop)
  qorm measure <app-dir> [-o report.json]          render + self-measure layout & styles (needs -tags desktop)
  qorm check <app-dir> (--checks c.json | --audit) [-o r.json]
                                                  verify layout/styles/behaviour vs expectations (or generic audit)
  qorm build <app-dir> -o app.qorm.bundle [--key priv.key] [--require-capability camera,location]
                                                  compile (+optionally sign) a bundle; declared capabilities are
                                                  enforced at startup on the running platform
  qorm keygen [--out-dir .]                        generate an ed25519 signing keypair
  qorm sign <bundle> --key priv.key [-o out]       sign an existing (e.g. agent-exported) bundle
  qorm verify <bundle> [--trust pub.key] [--revoked list.json]
                                                  verify integrity (+ signature, + revocation)
  qorm mcp <app-dir|bundle> [--trust pub.key]      serve the app to agents over MCP (stdio)
  qorm package <app-dir> -p web|ios|android|mac [-o out] [--dev URL] [--team ID] [--no-branding] [--subscribed]
                                                  package as an installable app for a platform
       --release [--app-version V --build N]      distributable build — iOS .ipa (--export-method / --upload /
                                                  --api-key*), Android signed .aab (--keystore / --key-alias /
                                                  --apk), macOS Developer ID + DMG (--identity / --notarize)
       --update-url URL --trust pub.key           wire the package to an OTA update server (flags are paired)
  qorm preview <package-dir> [--width N] [-o report.json]
                                                  render a packaged app and report its layout
  qorm docs [--docs docs] [-o site]                render the markdown docs to a static HTML site
  qorm audit <audit-log.jsonl>                     verify a hash-chained activity audit log
                                                  (written by qorm run --audit-log <file>)
  qorm updates <bundles-dir> [--port N]            OTA publish server (staged rollout via rollout.json)
  qorm update [--insecure-skip-verify]             update the CLI to the latest version (verifies signed checksums)
  qorm version                                     print the version
`)
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
				srv, serr := server.NewBundle(b, trust, revoked)
				if serr != nil {
					return nil, "", serr
				}
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

func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
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
