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
	appVersion, buildNum := rel.AppVersion, rel.BuildNum
	if appVersion == "" {
		appVersion = "1.0"
	}
	if buildNum == "" {
		buildNum = "1"
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>` + appName + `</string>
  <key>CFBundleDisplayName</key><string>` + appName + `</string>
  <key>CFBundleIdentifier</key><string>com.qorm.` + id + `</string>
  <key>CFBundleExecutable</key><string>` + id + `</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>` + appVersion + `</string>
  <key>CFBundleVersion</key><string>` + buildNum + `</string>
  <key>LSMinimumSystemVersion</key><string>10.13</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSQuitAlwaysKeepsWindows</key><false/>
` + macPermKeys(appName, srcDir) + `</dict></plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	if !rel.Release {
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
	// --release: Developer ID signature (hardened runtime), then DMG, then
	// optional notarization. Never falls back to ad-hoc — a release artifact
	// that Gatekeeper rejects on every other Mac is worse than a hard error.
	identity := rel.Identity
	if identity == "" {
		var err error
		if identity, err = findDeveloperID(); err != nil {
			return err
		}
	}
	if err := signMacRelease(bundle, identity); err != nil {
		return err
	}
	artifact := bundle // what gets notarized (and shipped)
	if !rel.NoDMG {
		dmg := filepath.Join(out, appName+".dmg")
		if err := makeDMG(bundle, dmg, appName); err != nil {
			return err
		}
		// sign the DMG too, so the container itself carries a Developer ID seal
		sign := exec.Command("codesign", "--force", "--timestamp", "--sign", identity, dmg)
		sign.Stderr = os.Stderr
		if err := sign.Run(); err != nil {
			return fmt.Errorf("codesign of %s failed: %w", dmg, err)
		}
		artifact = dmg
	}
	if rel.Notarize {
		if err := notarizeMac(artifact, rel); err != nil {
			return err
		}
	}
	fmt.Printf("packaged %s -> %s (signed \"%s\")\n", appName, artifact, identity)
	return nil
}

// findDeveloperID auto-discovers the single "Developer ID Application" signing
// identity in the keychain. Zero or several candidates is a hard error (with
// the list, so --identity can pick one) — a --release build must never fall
// back to an ad-hoc signature silently.
func findDeveloperID() (string, error) {
	out, err := exec.Command("security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return "", fmt.Errorf("security find-identity failed: %w", err)
	}
	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "Developer ID Application") {
			continue
		}
		// `  1) <SHA1> "Developer ID Application: Some Name (TEAMID)"`
		if i := strings.Index(line, `"`); i >= 0 {
			ids = append(ids, strings.Trim(strings.TrimSpace(line[i:]), `"`))
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", fmt.Errorf("--release needs a \"Developer ID Application\" certificate, but `security find-identity -v -p codesigning` lists none; enroll in the Apple Developer Program and install one (Xcode > Settings > Accounts > Manage Certificates), or pass --identity")
	default:
		return "", fmt.Errorf("multiple Developer ID Application identities found — pick one with --identity:\n  %s", strings.Join(ids, "\n  "))
	}
}

// signMacRelease signs the .app with a Developer ID identity and the hardened
// runtime (both required by the notary service). No --deep: Apple no longer
// recommends it, and this bundle has exactly one nested binary, so signing the
// .app once (codesign covers the main executable inside-out) is sufficient.
func signMacRelease(bundlePath, identity string) error {
	// Intentionally minimal entitlements (empty dict). If WKWebView ever
	// crashes under the hardened runtime, add here:
	//   <key>com.apple.security.cs.allow-unsigned-executable-memory</key><true/>
	ent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<!-- Minimal on purpose. If WKWebView crashes under the hardened runtime, add:
     <key>com.apple.security.cs.allow-unsigned-executable-memory</key><true/> -->
<dict/>
</plist>
`
	entPath := filepath.Join(filepath.Dir(bundlePath), "release.entitlements")
	if err := os.WriteFile(entPath, []byte(ent), 0o644); err != nil {
		return err
	}
	sign := exec.Command("codesign", "--force", "--options", "runtime", "--timestamp",
		"--entitlements", entPath, "--sign", identity, bundlePath)
	sign.Stderr = os.Stderr
	if err := sign.Run(); err != nil {
		return fmt.Errorf("release codesign of %s failed: %w", bundlePath, err)
	}
	return nil
}

// makeDMG wraps the signed .app in a compressed (UDZO) disk image — the
// canonical "drag to Applications" macOS distribution artifact.
func makeDMG(appBundle, dmgPath, volName string) error {
	cmd := exec.Command("hdiutil", "create", "-volname", volName, "-srcfolder", appBundle, "-ov", "-format", "UDZO", dmgPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hdiutil create %s failed: %w", dmgPath, err)
	}
	return nil
}

// notarizeMac submits the artifact (the .dmg, or with --no-dmg the .app, which
// gets ditto-zipped first — notarytool won't take a bare bundle) to Apple's
// notary service, waits, and staples the ticket to the artifact and to the
// sibling .app so Gatekeeper passes even offline.
func notarizeMac(path string, rel releaseOpts) error {
	profile := rel.KeychainProfile
	if profile == "" {
		profile = "qorm"
	}
	target := path
	if strings.HasSuffix(path, ".app") {
		zip := strings.TrimSuffix(path, ".app") + ".zip"
		z := exec.Command("ditto", "-c", "-k", "--keepParent", path, zip)
		z.Stderr = os.Stderr
		if err := z.Run(); err != nil {
			return fmt.Errorf("ditto zip for notarization failed: %w", err)
		}
		defer os.Remove(zip)
		target = zip
	}
	fmt.Fprintf(os.Stderr, "notarizing %s with keychain profile %q (waits on Apple, typically minutes)…\n", filepath.Base(target), profile)
	sub := exec.Command("xcrun", "notarytool", "submit", target, "--keychain-profile", profile, "--wait")
	out, err := sub.CombinedOutput()
	os.Stderr.Write(out)
	if err != nil || !strings.Contains(string(out), "status: Accepted") {
		logID := "<submission-id>"
		if id := notarySubmissionID(string(out)); id != "" {
			logID = id
		}
		return fmt.Errorf(`notarization failed; per-file reasons:
  xcrun notarytool log %s --keychain-profile %s
first-time setup (password = app-specific password from appleid.apple.com):
  xcrun notarytool store-credentials %s --apple-id <you@example.com> --team-id <TEAMID> --password <app-specific-password>`,
			logID, profile, profile)
	}
	// staple the ticket: the artifact itself, plus the .app beside a .dmg (the
	// DMG's ticket covers the bundle it contains).
	staple := []string{path}
	if strings.HasSuffix(path, ".dmg") {
		if app := strings.TrimSuffix(path, ".dmg") + ".app"; dirExists(app) {
			staple = append(staple, app)
		}
	}
	for _, p := range staple {
		s := exec.Command("xcrun", "stapler", "staple", p)
		s.Stderr = os.Stderr
		if err := s.Run(); err != nil {
			return fmt.Errorf("stapler staple %s failed: %w", p, err)
		}
	}
	return nil
}

// notarySubmissionID pulls the first "id: <uuid>" out of notarytool output so
// a failure can point at the exact notary log to fetch.
func notarySubmissionID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "id:" {
			return f[1]
		}
	}
	return ""
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
