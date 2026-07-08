package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/miniapp"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

//go:embed icons/appicon-1024.png icons/appicon-512.png icons/appicon-192.png icons/tray.png icons/macicon-1024.png
var iconFS embed.FS

// appIcon returns the embedded QORM logo icon nearest to size n.
func appIcon(n int) []byte {
	name := "icons/appicon-1024.png"
	switch {
	case n <= 192:
		name = "icons/appicon-192.png"
	case n <= 512:
		name = "icons/appicon-512.png"
	}
	b, _ := iconFS.ReadFile(name)
	return b
}

// releaseOpts carries the release-build parameters through cmdPackage to the
// per-platform scaffolds. Zero value = debug/dev packaging (today's default).
type releaseOpts struct {
	Release    bool   // --release: produce a distributable, signed artifact
	AppVersion string // MARKETING_VERSION / versionName / CFBundleShortVersionString
	BuildNum   string // CURRENT_PROJECT_VERSION / versionCode / CFBundleVersion

	// iOS
	ExportMethod string // exportOptions method (default app-store-connect)
	Upload       bool   // exportOptions destination=upload (TestFlight)
	APIKey       string // App Store Connect API key .p8 path (unattended)
	APIKeyID     string
	APIIssuer    string

	// Android
	Keystore string // path to an existing keystore (else auto-managed)
	KeyAlias string
	APK      bool // additionally produce a signed APK next to the AAB

	// macOS
	Identity        string // "Developer ID Application: …" (else auto-discover)
	Notarize        bool
	KeychainProfile string // notarytool store-credentials profile name
	NoDMG           bool
}

// cmdPackage compiles a QORM app into an installable, fully offline package: a
// self-contained web app (the runtime runs client-side via Go-WASM, no server),
// optionally wrapped as an Android (APK) or iOS (IPA) project.
func cmdPackage(args []string) int {
	in, out, platform, team, dev := "", "", "web", "", ""
	noBranding, subscribed := false, false
	var rel releaseOpts
	strArg := func(i *int, dst *string) {
		if *i+1 < len(args) {
			*i++
			*dst = args[*i]
		}
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-branding":
			noBranding = true
		case "--subscribed":
			subscribed = true
		case "--dev":
			strArg(&i, &dev)
		case "--team":
			strArg(&i, &team)
		case "-p", "--platform":
			strArg(&i, &platform)
		case "-o", "--out":
			strArg(&i, &out)
		case "--release":
			rel.Release = true
		case "--app-version":
			strArg(&i, &rel.AppVersion)
		case "--build":
			strArg(&i, &rel.BuildNum)
		case "--export-method":
			strArg(&i, &rel.ExportMethod)
		case "--upload":
			rel.Upload = true
		case "--api-key":
			strArg(&i, &rel.APIKey)
		case "--api-key-id":
			strArg(&i, &rel.APIKeyID)
		case "--api-issuer":
			strArg(&i, &rel.APIIssuer)
		case "--keystore":
			strArg(&i, &rel.Keystore)
		case "--key-alias":
			strArg(&i, &rel.KeyAlias)
		case "--apk":
			rel.APK = true
		case "--identity":
			strArg(&i, &rel.Identity)
		case "--notarize":
			rel.Notarize = true
		case "--keychain-profile":
			strArg(&i, &rel.KeychainProfile)
		case "--no-dmg":
			rel.NoDMG = true
		default:
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm package <app-dir> [-p web|android|ios|mac|miniapp] [-o out-dir] [--release]")
		return 2
	}
	if rel.Release && dev != "" {
		fmt.Fprintln(os.Stderr, "error: --release and --dev are mutually exclusive (the dev client is always a debug build)")
		return 2
	}
	if rel.AppVersion == "" {
		rel.AppVersion = "1.0"
	}
	if rel.BuildNum == "" {
		rel.BuildNum = "1"
	}
	name := filepath.Base(strings.TrimRight(in, "/"))
	if out == "" {
		out = name + "-" + platform
	}

	app, err := loader.LoadDir(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if noBranding {
		app.Branding = false
	}
	// Honour-system commercial gate: using a custom icon or removing the
	// "Made with QORM" note is commercial white-labeling — confirm membership.
	if dev == "" && !confirmCommercial(in, app.Branding, subscribed) {
		return 1
	}
	b, err := bundle.Build(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	bj, _ := bundle.Marshal(b)

	// --dev builds a thin native client that connects to a live dev server for
	// dynamic debugging (hot reload, agent inspect/measure) — no offline payload.
	// It is app-agnostic (it shows whatever the server runs), so it uses a fixed
	// "QORM Dev" identity: install it ONCE and reuse it for every app, with all
	// the common hardware permissions already baked in.
	if dev != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		devName := "QORM Dev"
		fmt.Printf("packaging the QORM Dev client → %s (install once; reuse for every app; changes hot-reload)\n", dev)
		switch platform {
		case "ios":
			if err := scaffoldIOS(out, devName, devName, team, dev, in, rel); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		case "android":
			if err := scaffoldAndroid(out, devName, devName, dev, in, rel); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		default:
			fmt.Fprintf(os.Stderr, "error: --dev needs -p ios or -p android\n")
			return 2
		}
		return 0
	}

	// Surface platform capability gaps (e.g. a feature iOS has but Android
	// doesn't, or a mobile-only sensor targeted for desktop) before building.
	checkPlatform(app, platform)
	warnUserNativeGaps(in, platform)

	// A macOS .app bundles the app + the desktop binary (no WASM payload needed).
	if platform == "mac" || platform == "macos" || platform == "desktop" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := scaffoldMac(out, name, app.Name, in, rel); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0
	}

	// A mini-program (小程序) is a WXML/WXSS project, not HTML/WASM — QORM remaps
	// its rendered UI and emits a WeChat-style project (open it in WeChat DevTools).
	if platform == "miniapp" || platform == "miniprogram" || platform == "weapp" {
		rt := qrt.New(app)
		for path, content := range miniapp.BuildProject(rt) {
			full := filepath.Join(out, path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		}
		fmt.Printf("packaged %s -> %s (WeChat mini-program project; open it in WeChat DevTools)\n", name, out)
		return 0
	}

	// The web assets are the payload for every platform; native shells wrap them.
	webDir := out
	if platform != "web" {
		webDir = filepath.Join(out, "www")
	}
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	rt := qrt.New(app)
	html, err := server.OfflineHTML(rt, string(bj))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if platform == "web" {
		reg := `<script>if('serviceWorker' in navigator && location.protocol.indexOf('http')===0){addEventListener('load',function(){navigator.serviceWorker.register('sw.js').catch(function(){})})}</script>`
		if strings.Contains(html, "</body>") {
			html = strings.Replace(html, "</body>", reg+"</body>", 1)
		} else {
			html += reg
		}
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte(html), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	// The whole app — the source's many JSON files (qorm.json + scenes/* +
	// actions) compiled into ONE signed bundle — ships as its own artifact next
	// to the HTML. The runtime fetches it, it stays inspectable/cacheable, and
	// it is exactly what an OTA update swaps and what the signature covers.
	if err := os.WriteFile(filepath.Join(webDir, "bundle.json"), bj, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "compiling client-side runtime (Go->WASM)…\n")
	if err := buildWASM(filepath.Join(webDir, "qorm.wasm"), in); err != nil {
		fmt.Fprintf(os.Stderr, "error: WASM build failed: %v\n", err)
		return 1
	}
	if err := copyWasmExec(filepath.Join(webDir, "wasm_exec.js")); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	writeManifest(webDir, app.Name)
	writeIconFor(in, filepath.Join(webDir, "icon-192.png"), 192)
	writeIconFor(in, filepath.Join(webDir, "icon-512.png"), 512)

	switch platform {
	case "web":
		writeServiceWorker(webDir)
		fmt.Printf("packaged %s -> %s (installable, offline-capable PWA; serve it and Add to Home Screen)\n", name, out)
	case "android":
		if err := scaffoldAndroid(out, name, app.Name, "", in, rel); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	case "ios":
		if err := scaffoldIOS(out, name, app.Name, team, "", in, rel); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown platform %q (web|android|ios|mac|miniapp)\n", platform)
		return 2
	}
	return 0
}

// buildWASM compiles the client-side runtime for the browser/WebView.
func buildWASM(dst, appDir string) error {
	defer injectUserGo(appDir, "github.com/qorm/qorm/cmd/qorm-wasm")()
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-trimpath", "-o", dst, "github.com/qorm/qorm/cmd/qorm-wasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w (packaging builds the client WASM via 'go build github.com/qorm/qorm/cmd/qorm-wasm'; run 'qorm package' from the QORM repo, or a dir whose go.mod requires github.com/qorm/qorm)", err)
	}
	return nil
}

// copyWasmExec copies Go's WASM support shim from GOROOT.
func copyWasmExec(dst string) error {
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	goroot := strings.TrimSpace(string(root))
	for _, p := range []string{"lib/wasm/wasm_exec.js", "misc/wasm/wasm_exec.js"} {
		if data, err := os.ReadFile(filepath.Join(goroot, p)); err == nil {
			return os.WriteFile(dst, data, 0o644)
		}
	}
	return fmt.Errorf("wasm_exec.js not found under %s", goroot)
}

// writeManifest writes a PWA web app manifest so the app is installable.
func writeManifest(dir, name string) {
	esc := strings.ReplaceAll(name, `"`, `\"`)
	m := `{
  "name": "` + esc + `",
  "short_name": "` + esc + `",
  "start_url": ".",
  "display": "standalone",
  "background_color": "#000000",
  "theme_color": "#000000",
  "icons": [
    { "src": "icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable" }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.webmanifest"), []byte(m), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warn: could not write manifest: %v\n", err)
	}
}

// confirmCommercial is the honour-system gate for commercial white-labeling: a
// custom app icon or removing the "Made with QORM" note is commercial use. It
// asks the packager to confirm a QORM Patreon membership ($1/mo individual,
// $7/mo company) — no verification, just a "yes". Personal / educational /
// open-source use (default icon + branding on) never triggers it.
func confirmCommercial(appDir string, branding, subscribed bool) bool {
	customIcon := false
	if b, err := os.ReadFile(filepath.Join(appDir, "icon.png")); err == nil && len(b) > 0 {
		customIcon = true
	}
	if !customIcon && branding {
		return true // no commercial feature in use
	}
	var feats []string
	if customIcon {
		feats = append(feats, "a custom app icon")
	}
	if !branding {
		feats = append(feats, `no "Made with QORM" note`)
	}
	fmt.Fprintf(os.Stderr, "note: this package uses a commercial feature (%s).\n"+
		"      Commercial use asks a QORM Patreon membership — Indie $1/mo or Studio\n"+
		"      $7/mo. See ops/TERMS.md · https://www.patreon.com/qorm\n",
		strings.Join(feats, ", "))
	if subscribed {
		fmt.Fprintln(os.Stderr, "      confirmed via --subscribed.")
		return true
	}
	if st, _ := os.Stdin.Stat(); st != nil && st.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprint(os.Stderr, "      Are you a subscriber? type yes to confirm: ")
		var ans string
		fmt.Fscanln(os.Stdin, &ans)
		if s := strings.ToLower(strings.TrimSpace(ans)); s == "yes" || s == "y" {
			return true
		}
		fmt.Fprintln(os.Stderr, "      not confirmed — subscribe at https://www.patreon.com/qorm, then re-run (or pass --subscribed).")
		return false
	}
	fmt.Fprintln(os.Stderr, "      non-interactive: pass --subscribed to confirm your membership.")
	return false
}

// appIconFor returns the app's OWN launcher icon: <appDir>/icon.png if the author
// shipped one (recommended 1024x1024 PNG), else the built-in QORM logo at the
// nearest size. Lets every packaged app carry its own identity.
func appIconFor(appDir string, n int) []byte {
	if appDir != "" {
		if b, err := os.ReadFile(filepath.Join(appDir, "icon.png")); err == nil && len(b) > 0 {
			return b
		}
	}
	return appIcon(n)
}

// writeServiceWorker writes a minimal cache-first service worker so the packaged
// web app installs (Chrome PWA prompt) and runs offline after the first load.
func writeServiceWorker(dir string) {
	const sw = `const CACHE='qorm-shell-v1';
const ASSETS=['./','index.html','qorm.wasm','wasm_exec.js','bundle.json','manifest.webmanifest','icon-192.png','icon-512.png'];
self.addEventListener('install',e=>{e.waitUntil(caches.open(CACHE).then(c=>c.addAll(ASSETS)).then(()=>self.skipWaiting()))});
self.addEventListener('activate',e=>{e.waitUntil(caches.keys().then(ks=>Promise.all(ks.filter(k=>k!==CACHE).map(k=>caches.delete(k)))).then(()=>self.clients.claim()))});
self.addEventListener('fetch',e=>{if(e.request.method!=='GET')return;e.respondWith(caches.match(e.request).then(r=>r||fetch(e.request)).catch(()=>caches.match('index.html')))});
`
	os.WriteFile(filepath.Join(dir, "sw.js"), []byte(sw), 0o644)
}

// writeIconFor writes the app's own icon (or the QORM logo) at path.
func writeIconFor(appDir, path string, n int) {
	if b := appIconFor(appDir, n); b != nil {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not write icon %s: %v\n", path, err)
		}
	}
}
