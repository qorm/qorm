package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qorm/qorm/internal/capability"
)

// dropNFCEntitlement removes the NFC entitlement (a free personal team can't
// sign it) and regenerates the project, so the app still installs without NFC.
func dropNFCEntitlement(dir, id, xg string) {
	os.Remove(filepath.Join(dir, id+".entitlements"))
	p := filepath.Join(dir, "project.yml")
	if data, err := os.ReadFile(p); err == nil {
		var out []string
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "CODE_SIGN_ENTITLEMENTS") {
				out = append(out, line)
			}
		}
		os.WriteFile(p, []byte(strings.Join(out, "\n")), 0o644)
	}
	g := exec.Command(xg, "generate")
	g.Dir = dir
	g.Run()
}

// scaffoldMac builds a macOS .app bundle: the desktop QORM binary + the app
// data + a proper icon and Info.plist, so it double-clicks open like any app.
// macPermKeys renders the Info.plist usage keys a mac app needs, derived from the
// capabilities it actually uses.
func macPermKeys(appName, srcDir string) string {
	var b strings.Builder
	for _, k := range capability.PermsFor(usedWidgets(srcDir), capability.Mac) {
		b.WriteString("  <key>" + k + "</key><string>" + appName + " " + capability.IOSPermReason(k) + ".</string>\n")
	}
	return b.String()
}

func scaffoldMac(out, name, appName, srcDir string, rel releaseOpts) error {
	_ = rel // release signing/notarization/DMG lands with v0.2.1 A3
	id := pkgID(name)
	bundle := filepath.Join(out, appName+".app")
	macos := filepath.Join(bundle, "Contents", "MacOS")
	res := filepath.Join(bundle, "Contents", "Resources")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(res, "app"), 0o755); err != nil {
		return err
	}
	// The app payload = the SAME compiled+signed bundle.json the web/mobile package
	// ships (not the raw source), plus native/ so the server can inject web.js.
	// bundledApp() detects bundle.json and runs it in --app mode.
	if err := writeAppBundle(srcDir, filepath.Join(res, "app")); err != nil {
		return err
	}
	// build the desktop binary (webview + tray; needs cgo, macOS only)
	// Compile the app's own Go middle-layer (native/desktop.go) INTO this one
	// binary — the user writes Go, it ships in the single executable.
	defer injectUserGo(srcDir, "github.com/qorm/qorm/cmd/qorm")()
	fmt.Fprintf(os.Stderr, "building the desktop binary (webview + tray)…\n")
	build := exec.Command("go", "build", "-tags", "desktop", "-o", filepath.Join(macos, id), "github.com/qorm/qorm/cmd/qorm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("desktop build failed (need macOS + cgo): %w", err)
	}
	// icon (.icns from the QORM logo) + Info.plist
	makeICNS(filepath.Join(res, "AppIcon.icns"), srcDir)
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>` + appName + `</string>
  <key>CFBundleDisplayName</key><string>` + appName + `</string>
  <key>CFBundleIdentifier</key><string>com.qorm.` + id + `</string>
  <key>CFBundleExecutable</key><string>` + id + `</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>LSMinimumSystemVersion</key><string>10.13</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSQuitAlwaysKeepsWindows</key><false/>
` + macPermKeys(appName, srcDir) + `</dict></plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	// Ad-hoc code-sign the finished bundle so macOS can PROMPT for TCC-protected
	// APIs (Bluetooth, camera, mic, location) on first use instead of killing an
	// unsigned .app the moment it touches them.
	sign := exec.Command("codesign", "--force", "--deep", "--sign", "-", bundle)
	sign.Stderr = os.Stderr
	if err := sign.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: ad-hoc codesign failed (%v) — TCC-protected APIs may still crash\n", err)
	}
	fmt.Printf("packaged %s -> %s (double-click to run)\n", appName, bundle)
	return nil
}

// makeICNS builds an .icns from the embedded QORM logo via iconutil.
func makeICNS(dst, srcDir string) {
	iconset, err := os.MkdirTemp("", "qorm-icns")
	if err != nil {
		return
	}
	defer os.RemoveAll(iconset)
	set := filepath.Join(iconset, "icon.iconset")
	os.MkdirAll(set, 0o755)
	base := filepath.Join(iconset, "1024.png")
	// the author's icon.png wins; otherwise use the mac-styled QORM logo.
	macPNG := appIconFor(srcDir, 1024)
	if _, statErr := os.Stat(filepath.Join(srcDir, "icon.png")); statErr != nil {
		if b, e := iconFS.ReadFile("icons/macicon-1024.png"); e == nil {
			macPNG = b
		}
	}
	if len(macPNG) == 0 {
		macPNG = appIcon(1024)
	}
	os.WriteFile(base, macPNG, 0o644)
	sizes := []struct {
		name string
		px   string
	}{{"icon_16x16.png", "16"}, {"icon_16x16@2x.png", "32"}, {"icon_32x32.png", "32"}, {"icon_32x32@2x.png", "64"},
		{"icon_128x128.png", "128"}, {"icon_128x128@2x.png", "256"}, {"icon_256x256.png", "256"}, {"icon_256x256@2x.png", "512"},
		{"icon_512x512.png", "512"}, {"icon_512x512@2x.png", "1024"}}
	for _, s := range sizes {
		exec.Command("sips", "-z", s.px, s.px, base, "--out", filepath.Join(set, s.name)).Run()
	}
	exec.Command("iconutil", "-c", "icns", set, "-o", dst).Run()
}
