package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// usedWidgets returns the set of widget types an app actually uses across all its
// scenes — the basis for deriving the permissions the package must declare.
func usedWidgets(appDir string) map[string]bool {
	used := map[string]bool{}
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return used
	}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		used[n.Type] = true
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.Template)
	}
	if r := app.EntryRoot(); r != nil {
		walk(r)
	}
	for _, sc := range app.Scenes {
		walk(sc)
	}
	return used
}

// pkgID turns an app name into a safe reverse-DNS-ish identifier segment.
func pkgID(name string) string {
	id := regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(strings.ToLower(name), "")
	if id == "" {
		id = "app"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "a" + id
	}
	return id
}

// scaffoldAndroid generates a complete WebView Android project around the
// offline web payload (out/www) and builds an APK if the toolchain is present.
// The payload is served to the WebView over https via WebViewAssetLoader so the
// WASM runtime's fetch() works from packaged assets.
func scaffoldAndroid(out, name, appName, dev, appDir string) error {
	id := pkgID(name)
	ns := "com.qorm." + id
	awidgets := appWidgets(appDir)
	hasWidget := len(awidgets) > 0 && dev == ""
	wName := appName
	if hasWidget && awidgets[0].Name != "" {
		wName = awidgets[0].Name
	}
	nsPath := filepath.Join(out, "app", "src", "main", "java", "com", "qorm", id)
	assets := filepath.Join(out, "app", "src", "main", "assets", "www")
	res := filepath.Join(out, "app", "src", "main", "res")
	for _, d := range []string{nsPath, assets, filepath.Join(res, "mipmap-mdpi"), filepath.Join(res, "values"), filepath.Join(res, "layout"), filepath.Join(res, "xml")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// offline mode bundles the web payload; dev mode connects to a server.
	if dev == "" {
		if err := copyTree(filepath.Join(out, "www"), assets); err != nil {
			return err
		}
	}
	writeIconFor(appDir, filepath.Join(res, "mipmap-mdpi", "ic_launcher.png"), 192)

	files := map[string]string{
		"settings.gradle": `pluginManagement {
    repositories { google(); mavenCentral(); gradlePluginPortal() }
}
dependencyResolutionManagement {
    repositories { google(); mavenCentral() }
}
rootProject.name = "` + id + `"
include(":app")
`,
		"build.gradle": `plugins {
    id 'com.android.application' version '8.7.2' apply false
}
`,
		"gradle.properties": "android.useAndroidX=true\norg.gradle.jvmargs=-Xmx2048m\n",
		"app/build.gradle": `plugins { id 'com.android.application' }
android {
    namespace '` + ns + `'
    compileSdk 34
    defaultConfig {
        applicationId "` + ns + `"
        minSdk 24
        targetSdk 34
        versionCode 1
        versionName "1.0"
    }
    buildTypes { release { minifyEnabled false } }
    compileOptions {
        sourceCompatibility JavaVersion.VERSION_17
        targetCompatibility JavaVersion.VERSION_17
    }
}
dependencies {
    implementation 'androidx.webkit:webkit:1.12.1'
    implementation 'androidx.biometric:biometric:1.1.0'
    implementation 'androidx.security:security-crypto:1.1.0-alpha06'
}
`,
		"app/src/main/AndroidManifest.xml": `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">` + androidPerms(dev, appDir) + `
    <application
        android:label="` + xmlEsc(appName) + `"
        android:icon="@mipmap/ic_launcher"
        android:hardwareAccelerated="true"` + androidCleartext(dev) + `
        android:theme="@android:style/Theme.Material.Light.NoActionBar">
        <activity android:name=".MainActivity" android:exported="true"
            android:configChanges="orientation|screenSize|keyboardHidden">
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
` + androidWidgetReceiver(hasWidget) + `    </application>
</manifest>
`,
		"app/src/main/java/com/qorm/" + id + "/MainActivity.java": spliceUser(androidMainActivity(ns, dev, androidScFor(dev, appDir)), "//__QORM_USER_ANDROID__", appDir, "android.java", ""),
	}
	// always ship the provider + resources so the updateWidget op compiles; the
	// manifest <receiver> (added only when the app declares a widget) surfaces it.
	files["app/src/main/java/com/qorm/"+id+"/QormWidget.java"] = androidWidgetProvider(ns, awProvider(awidgets, wName))
	files["app/src/main/res/layout/qorm_widget.xml"] = androidWidgetLayout()
	files["app/src/main/res/xml/qorm_widget_info.xml"] = androidWidgetInfo()
	for rel, content := range files {
		p := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("generated Android project -> %s\n", out)
	return buildAndroid(out)
}

// buildAndroid runs a Gradle debug build if a toolchain is available, else
// prints how to finish the build.
func buildAndroid(dir string) error {
	gradle, err := exec.LookPath("gradle")
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		sdk = os.Getenv("ANDROID_SDK_ROOT")
	}
	if err != nil || sdk == "" {
		fmt.Printf("  Android SDK/Gradle not both present — project is ready to build:\n    cd %s && gradle assembleDebug\n", dir)
		return nil
	}
	// Pin a Gradle wrapper to a version the Android plugin supports (a newer
	// system Gradle can be incompatible with AGP), then build via the wrapper.
	fmt.Fprintf(os.Stderr, "preparing Gradle wrapper (8.9)…\n")
	wrap := exec.Command(gradle, "wrapper", "--gradle-version", "8.9", "--no-daemon", "--console=plain")
	wrap.Dir = dir
	wrap.Stdout, wrap.Stderr = os.Stderr, os.Stderr
	builder := "./gradlew"
	if err := wrap.Run(); err != nil {
		builder = gradle // fall back to system gradle
	}
	fmt.Fprintf(os.Stderr, "building APK (first run downloads Gradle + the Android plugin)…\n")
	cmd := exec.Command(builder, "assembleDebug", "--no-daemon", "--console=plain")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Gradle build did not complete. Project is ready:\n    cd %s && ./gradlew assembleDebug\n", dir)
		return nil
	}
	apk := filepath.Join(dir, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if _, e := os.Stat(apk); e == nil {
		fmt.Printf("  [ok] APK: %s\n", apk)
	}
	return nil
}

// iosScFor returns an app's shortcuts, or none for the app-agnostic dev client.
func iosScFor(dev, appDir string) []model.Shortcut {
	if dev != "" {
		return nil
	}
	return appShortcuts(appDir)
}

// iosWidgetSwift generates the WidgetKit extension: a SwiftUI widget that renders
// a title + label/value lines read from the App Group the main app writes to.
func iosWidgetSwift(appGroup string, w model.Widget) string {
	title := w.Title
	if title == "" {
		title = w.Name
	}
	var lines strings.Builder
	lines.WriteString("[")
	for i, ln := range w.Lines {
		if i > 0 {
			lines.WriteString(", ")
		}
		lines.WriteString("[" + strconv.Quote(ln.Label) + ", " + strconv.Quote(ln.Value) + "]")
	}
	lines.WriteString("]")
	return `import WidgetKit
import SwiftUI

struct QormEntry: TimelineEntry {
    let date: Date
    let title: String
    let lines: [[String]]
}
struct QormProvider: TimelineProvider {
    func placeholder(in c: Context) -> QormEntry { QormProvider.load() }
    func getSnapshot(in c: Context, completion: @escaping (QormEntry) -> Void) { completion(QormProvider.load()) }
    func getTimeline(in c: Context, completion: @escaping (Timeline<QormEntry>) -> Void) {
        completion(Timeline(entries: [QormProvider.load()], policy: .atEnd))
    }
    static func load() -> QormEntry {
        let d = UserDefaults(suiteName: "` + appGroup + `")
        let title = d?.string(forKey: "widget_title") ?? "` + title + `"
        var lines: [[String]] = ` + lines.String() + `
        if let s = d?.string(forKey: "widget_lines"), let data = s.data(using: .utf8),
           let arr = try? JSONSerialization.jsonObject(with: data) as? [[String: String]] {
            lines = []
            for it in arr { lines.append([it["label"] ?? "", it["value"] ?? ""]) }
        }
        return QormEntry(date: Date(), title: title, lines: lines)
    }
}
struct QormWidgetEntryView: View {
    var entry: QormEntry
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(entry.title).font(.headline)
            ForEach(entry.lines.indices, id: \.self) { i in
                HStack {
                    Text(entry.lines[i][0]).foregroundColor(.secondary)
                    Spacer()
                    Text(entry.lines[i][1]).fontWeight(.semibold)
                }.font(.caption)
            }
            Spacer()
        }.padding()
    }
}
@main
struct QormWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "QormWidget", provider: QormProvider()) { entry in
            QormWidgetEntryView(entry: entry)
        }
        .configurationDisplayName("` + w.Name + `")
        .description("` + w.Name + `")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}
`
}

// iosWidgetTarget returns the XcodeGen target for the widget extension, or "".
func iosWidgetTarget(hasWidget bool, id, name, team string) string {
	if !hasWidget {
		return ""
	}
	return `
  ` + id + `Widget:
    type: app-extension
    platform: iOS
    sources: [WidgetSources]
    info:
      path: WidgetSources/Info.plist
      properties:
        CFBundleDisplayName: ` + name + `
        NSExtension:
          NSExtensionPointIdentifier: com.apple.widgetkit-extension
    settings:
      base:
        PRODUCT_BUNDLE_IDENTIFIER: com.qorm.` + id + `.widget
        MARKETING_VERSION: "1.0"
        CURRENT_PROJECT_VERSION: "1"
        CODE_SIGN_ENTITLEMENTS: ` + id + `Widget.entitlements` + signingYML(team)
}

// iosWidgetDep returns the app target's embed-dependency on the widget, or "".
func iosWidgetDep(hasWidget bool, id string) string {
	if !hasWidget {
		return ""
	}
	return `
    dependencies:
      - target: ` + id + `Widget
        embed: true`
}

// iosAppGroupXML returns the App Group entitlement entry, or "".
func iosAppGroupXML(hasWidget bool, appGroup string) string {
	if !hasWidget {
		return ""
	}
	return `
  <key>com.apple.security.application-groups</key>
  <array><string>` + appGroup + `</string></array>`
}

// appWidgets loads an app's home-screen widgets.
func appWidgets(appDir string) []model.Widget {
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return nil
	}
	return app.Widgets
}

// appShortcuts loads an app's icon quick actions.
func appShortcuts(appDir string) []model.Shortcut {
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return nil
	}
	return app.Shortcuts
}

// iosAppDelegate builds the AppDelegate, registering the app's quick actions and
// routing a selection to fireShortcut (which emits 'shortcut' on the event bus).
func iosAppDelegate() string {
	return `import UIKit

var qormPendingShortcut: String?

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = ViewController()
        window?.makeKeyAndVisible()
        if let sc = launchOptions?[.shortcutItem] as? UIApplicationShortcutItem {
            qormPendingShortcut = sc.type  // delivered when the page pulls it on load
        }
        return true
    }
    func application(_ application: UIApplication, performActionFor shortcutItem: UIApplicationShortcutItem, completionHandler: @escaping (Bool) -> Void) {
        qormPendingShortcut = shortcutItem.type
        (window?.rootViewController as? ViewController)?.qormDeliverPending()
        completionHandler(true)
    }
}
`
}

// scaffoldIOS generates a WKWebView iOS app around the offline web payload
// (out/www, kept as a folder reference so the WASM asset structure is
// preserved), builds the .xcodeproj with XcodeGen, and does a simulator build
// (no signing) to verify it compiles. A device IPA needs the user's signing.
func scaffoldIOS(out, name, appName, team, dev, appDir string) error {
	id := pkgID(name)
	widgets := appWidgets(appDir)
	hasWidget := len(widgets) > 0 && dev == ""
	appGroup := "group.com.qorm." + id
	widgetName := appName
	if hasWidget && widgets[0].Name != "" {
		widgetName = widgets[0].Name
	}
	src := filepath.Join(out, "Sources")
	if err := os.MkdirAll(src, 0o755); err != nil {
		return err
	}
	// AppIcon asset catalog (the QORM logo) so the installed app has our icon.
	iconSet := filepath.Join(out, "Assets.xcassets", "AppIcon.appiconset")
	os.MkdirAll(iconSet, 0o755)
	os.WriteFile(filepath.Join(out, "Assets.xcassets", "Contents.json"), []byte(`{"info":{"author":"xcode","version":1}}`), 0o644)
	os.WriteFile(filepath.Join(iconSet, "icon-1024.png"), appIconFor(appDir, 1024), 0o644)
	os.WriteFile(filepath.Join(iconSet, "Contents.json"), []byte(`{"images":[{"filename":"icon-1024.png","idiom":"universal","platform":"ios","size":"1024x1024"}],"info":{"author":"xcode","version":1}}`), 0o644)
	// NFC entitlement (Core NFC). Needs a PAID team + the capability enabled on
	// the App ID; a free personal team can't sign it.
	os.WriteFile(filepath.Join(out, id+".entitlements"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>com.apple.developer.nfc.readersession.formats</key>
  <array><string>NDEF</string></array>`+iosAppGroupXML(hasWidget, appGroup)+`
</dict></plist>`), 0o644)
	files := map[string]string{
		"Sources/AppDelegate.swift":    iosAppDelegate(),
		"Sources/ViewController.swift": spliceUser(iosViewController(dev), "//__QORM_USER_IOS__", appDir, "ios.swift", "func qormUserOp(_ op: String, _ body: [String: Any]) {}"),
		"project.yml": `name: ` + id + `
options:
  bundleIdPrefix: com.qorm
  deploymentTarget:
    iOS: "14.0"
targets:
  ` + id + `:
    type: application
    platform: iOS
    sources:` + iosSources(dev) + `` + iosWidgetDep(hasWidget, id) + `
` + iosInfo(dev, appName, appDir) + `    settings:
      base:
        PRODUCT_BUNDLE_IDENTIFIER: com.qorm.` + id + `
        MARKETING_VERSION: "1.0"
        CURRENT_PROJECT_VERSION: "1"
        TARGETED_DEVICE_FAMILY: "1,2"
        ASSETCATALOG_COMPILER_APPICON_NAME: AppIcon
        CODE_SIGN_ENTITLEMENTS: ` + id + `.entitlements` + iosGenInfo(dev, appName, appDir) + signingYML(team) + `
` + iosWidgetTarget(hasWidget, id, widgetName, team) + `
`,
	}
	if hasWidget {
		files["Sources/WidgetSupport.swift"] = "import WidgetKit\nfunc qormReloadWidgets() { if #available(iOS 14.0, *) { WidgetCenter.shared.reloadAllTimelines() } }\n"
	} else {
		files["Sources/WidgetSupport.swift"] = "func qormReloadWidgets() {}\n"
	}
	if hasWidget {
		files["WidgetSources/QormWidget.swift"] = iosWidgetSwift(appGroup, widgets[0])
		files[id+"Widget.entitlements"] = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>com.apple.security.application-groups</key>
  <array><string>` + appGroup + `</string></array>
</dict></plist>`
	}
	for rel, content := range files {
		p := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("generated iOS project -> %s\n", out)
	return buildIOS(out, id, team)
}

// signingYML adds automatic-signing settings when a development team is given.
func signingYML(team string) string {
	if team == "" {
		return ""
	}
	return "\n        DEVELOPMENT_TEAM: " + team + "\n        CODE_SIGN_STYLE: Automatic"
}

// buildIOS runs XcodeGen then builds: for the device (signed, installable) when
// a team is given, else an unsigned simulator build.
func buildIOS(dir, id, team string) error {
	xg, xgErr := exec.LookPath("xcodegen")
	xb, xbErr := exec.LookPath("xcodebuild")
	if xgErr != nil || xbErr != nil {
		fmt.Printf("  XcodeGen/Xcode not both present — sources are ready:\n    cd %s && xcodegen && open %s.xcodeproj\n", dir, id)
		return nil
	}
	gen := exec.Command(xg, "generate")
	gen.Dir = dir
	gen.Stdout, gen.Stderr = os.Stderr, os.Stderr
	if err := gen.Run(); err != nil {
		fmt.Printf("  xcodegen failed; sources ready at %s\n", dir)
		return nil
	}
	if team != "" {
		// Signed device build: automatic signing provisions against the team,
		// producing an installable .app for a physical iPhone.
		fmt.Fprintf(os.Stderr, "building for a physical device (signed, team %s)…\n", team)
		build := exec.Command(xb, "-project", id+".xcodeproj", "-scheme", id,
			"-destination", "generic/platform=iOS", "-configuration", "Debug",
			"-derivedDataPath", "build", "-allowProvisioningUpdates",
			"DEVELOPMENT_TEAM="+team, "CODE_SIGN_STYLE=Automatic", "build")
		build.Dir = dir
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		if err := build.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "signing failed — retrying without the NFC entitlement (needs a paid team)…\n")
			dropNFCEntitlement(dir, id, xg)
			b2 := exec.Command(xb, "-project", id+".xcodeproj", "-scheme", id,
				"-destination", "generic/platform=iOS", "-configuration", "Debug",
				"-derivedDataPath", "build", "-allowProvisioningUpdates",
				"DEVELOPMENT_TEAM="+team, "CODE_SIGN_STYLE=Automatic", "build")
			b2.Dir = dir
			b2.Stdout, b2.Stderr = os.Stderr, os.Stderr
			if err := b2.Run(); err != nil {
				fmt.Printf("  device build did not complete. Open %s/%s.xcodeproj in Xcode.\n", dir, id)
				return nil
			}
			fmt.Printf("  built WITHOUT NFC (this team can't sign it); other hardware works, paid team needed for NFC.\n")
		}
		appPath := filepath.Join(dir, "build", "Build", "Products", "Debug-iphoneos", id+".app")
		fmt.Printf("  [ok] Signed device .app: %s\n", appPath)
		return installIOS(appPath)
	}
	fmt.Fprintf(os.Stderr, "building for iOS Simulator (unsigned)…\n")
	build := exec.Command(xb, "-project", id+".xcodeproj", "-scheme", id,
		"-sdk", "iphonesimulator", "-configuration", "Debug",
		"-derivedDataPath", "build", "CODE_SIGNING_ALLOWED=NO", "build")
	build.Dir = dir
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Printf("  simulator build did not complete; open %s/%s.xcodeproj in Xcode to build+sign.\n", dir, id)
		return nil
	}
	appPath := filepath.Join(dir, "build", "Build", "Products", "Debug-iphonesimulator", id+".app")
	if _, e := os.Stat(appPath); e == nil {
		fmt.Printf("  [ok] Simulator .app: %s\n  For a device build: pass --team <id>.\n", appPath)
	}
	return nil
}

// installIOS installs a signed .app onto the first connected physical device.
func installIOS(appPath string) error {
	if _, err := os.Stat(appPath); err != nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "installing to the connected device…\n")
	args := []string{"devicectl", "device", "install", "app"}
	if dev := firstDevice(); dev != "" {
		args = append(args, "--device", dev)
	}
	args = append(args, appPath)
	inst := exec.Command("xcrun", args...)
	inst.Stdout, inst.Stderr = os.Stderr, os.Stderr
	if err := inst.Run(); err != nil {
		fmt.Printf("  install via devicectl failed — install by hand:\n    xcrun devicectl device install app %q\n", appPath)
		return nil
	}
	fmt.Printf("  [ok] installed on device. Launch it from the home screen.\n")
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// firstDevice returns the identifier of a connected, available physical device.
func firstDevice() string {
	out, err := exec.Command("xcrun", "devicectl", "list", "devices").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "available") && !strings.Contains(line, "unavailable") &&
			(strings.Contains(strings.ToLower(line), "iphone") || strings.Contains(strings.ToLower(line), "ipad")) {
			f := strings.Fields(line)
			for _, tok := range f {
				if len(tok) == 36 && strings.Count(tok, "-") == 4 {
					return tok
				}
			}
		}
	}
	return ""
}

// iosViewController returns the WKWebView controller: a thin dev client that
// connects to a live server (dynamic debugging) when dev is set, else the
// offline payload server.
func iosBridgeBody() string {
	return `    let loc = CLLocationManager()
    let motion = CMMotionManager()
    let pedometer = CMPedometer()
    let altimeter = CMAltimeter()
    var recorder: AVAudioRecorder?
    var recURL: URL?
    var central: CBCentralManager?
    var bt: [String: [String: Any]] = [:]
    var btScanning = false
    var nfcSession: NFCNDEFReaderSession?
    let volumeView = MPVolumeView(frame: .zero)
    var sysVol: Double = -1
    let speechSynth = AVSpeechSynthesizer()
    var speechRecognizer: SFSpeechRecognizer? = SFSpeechRecognizer()
    var sttRequest: SFSpeechAudioBufferRecognitionRequest?
    var sttTask: SFSpeechRecognitionTask?
    let sttEngine = AVAudioEngine()
    var qrSession: AVCaptureSession?

    // ---- native hardware bridge: JS calls window.webkit.messageHandlers.qorm ----
    func userContentController(_ ucc: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let op = body["op"] as? String else { return }
        switch op {
        case "location":
            loc.requestWhenInUseAuthorization()
            loc.requestLocation()
        case "motionStart":
            if motion.isDeviceMotionAvailable {
                motion.deviceMotionUpdateInterval = 0.2
                motion.startDeviceMotionUpdates(to: .main) { [weak self] m, _ in
                    guard let a = m?.attitude else { return }
                    let d = 180.0 / Double.pi
                    self?.js("qormOnMotion(\(a.yaw*d),\(a.pitch*d),\(a.roll*d))")
                }
            }
        case "motionStop":
            motion.stopDeviceMotionUpdates()
        case "recordStart":
            let sess = AVAudioSession.sharedInstance()
            try? sess.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker])
            try? sess.setActive(true)
            sess.requestRecordPermission { [weak self] granted in
                DispatchQueue.main.async {
                    guard let self = self else { return }
                    guard granted else { self.js("qormOnAudioError(\"microphone permission\")"); return }
                    let u = FileManager.default.temporaryDirectory.appendingPathComponent("qorm-rec.m4a")
                    self.recURL = u
                    let settings: [String: Any] = [AVFormatIDKey: Int(kAudioFormatMPEG4AAC), AVSampleRateKey: 44100, AVNumberOfChannelsKey: 1]
                    self.recorder = try? AVAudioRecorder(url: u, settings: settings)
                    self.recorder?.record()
                }
            }
        case "recordStop":
            recorder?.stop()
            recorder = nil
            if let u = recURL, let data = try? Data(contentsOf: u) {
                js("qormOnAudio(" + jsString("data:audio/mp4;base64," + data.base64EncodedString()) + ")")
            }
        case "biometric":
            let lac = LAContext()
            var lerr: NSError?
            if lac.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &lerr) {
                lac.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: "Authenticate to continue") { [weak self] ok, e in
                    DispatchQueue.main.async {
                        let msg = ok ? "authenticated" : (e?.localizedDescription ?? "failed")
                        self?.js("qormOnBiometric(" + (ok ? "true" : "false") + "," + (self?.jsString(msg) ?? "\"\"") + ")")
                    }
                }
            } else {
                js("qormOnBiometric(false," + jsString(lerr?.localizedDescription ?? "biometrics unavailable") + ")")
            }
        case "bluetoothScan":
            bt.removeAll()
            btScanning = true
            if central == nil { central = CBCentralManager(delegate: self, queue: nil) }
            else if central?.state == .poweredOn { central?.scanForPeripherals(withServices: nil, options: nil) }
            DispatchQueue.main.asyncAfter(deadline: .now() + 5) { [weak self] in
                self?.central?.stopScan(); self?.btScanning = false; self?.reportBT()
            }
        case "bluetoothState":
            if central == nil { central = CBCentralManager(delegate: self, queue: nil) }
            else { js("qormOnBluetoothState(" + (central?.state == .poweredOn ? "true" : "false") + ")") }
        case "nfcRead":
            if NFCNDEFReaderSession.readingAvailable {
                nfcSession = NFCNDEFReaderSession(delegate: self, queue: nil, invalidateAfterFirstRead: true)
                nfcSession?.begin()
            } else {
                js("qormOnNfc(" + jsString("{\"error\":\"NFC not available on this device\"}") + ")")
            }
        case "platform":
            js("qormOnPlatform('ios')")
        case "volumeGet":
            sysVol = Double(AVAudioSession.sharedInstance().outputVolume)
            js("qormOnVolume(\(sysVol))")
        case "brightnessGet":
            js("qormOnBrightness(\(Double(UIScreen.main.brightness)))")
        case "torchGet":
            if let dev = AVCaptureDevice.default(for: .video), dev.hasTorch {
                js("qormOnTorch(" + (dev.torchMode == .on ? "true" : "false") + ")")
            } else {
                js("qormOnTorch(false)")
            }
        case "volumeSet":
            sysVol = max(0.0, min(1.0, body["value"] as? Double ?? sysVol))
            if let slider = volumeView.subviews.compactMap({ $0 as? UISlider }).first { slider.setValue(Float(sysVol), animated: false) }
            js("qormOnVolume(\(sysVol))")
        case "volumeUp", "volumeDown":
            if sysVol < 0 { sysVol = Double(AVAudioSession.sharedInstance().outputVolume) }
            sysVol = max(0.0, min(1.0, sysVol + (op == "volumeUp" ? 0.0625 : -0.0625)))
            if let slider = volumeView.subviews.compactMap({ $0 as? UISlider }).first {
                slider.setValue(Float(sysVol), animated: false)
            }
            js("qormOnVolume(\(sysVol))")
        case "brightnessSet":
            let bset = max(0.0, min(1.0, body["value"] as? Double ?? Double(UIScreen.main.brightness)))
            UIScreen.main.brightness = CGFloat(bset)
            js("qormOnBrightness(\(bset))")
        case "brightnessUp", "brightnessDown":
            let target = max(0.0, min(1.0, Double(UIScreen.main.brightness) + (op == "brightnessUp" ? 0.1 : -0.1)))
            UIScreen.main.brightness = CGFloat(target)
            js("qormOnBrightness(\(target))")
        case "vibrate":
            AudioServicesPlaySystemSound(kSystemSoundID_Vibrate)
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        case "torchToggle":
            if let dev = AVCaptureDevice.default(for: .video), dev.hasTorch {
                try? dev.lockForConfiguration()
                let on = dev.torchMode != .on
                dev.torchMode = on ? .on : .off
                dev.unlockForConfiguration()
                js("qormOnTorch(" + (on ? "true" : "false") + ")")
            } else {
                js("qormOnTorch(false)")
            }
        case "battery":
            UIDevice.current.isBatteryMonitoringEnabled = true
            let lvl = max(0.0, Double(UIDevice.current.batteryLevel))
            let charging = UIDevice.current.batteryState == .charging || UIDevice.current.batteryState == .full
            js("qormOnBattery(\(lvl),\(charging ? "true" : "false"))")
        case "wifiInfo":
            NEHotspotNetwork.fetchCurrent { [weak self] net in
                DispatchQueue.main.async {
                    if let n = net {
                        self?.js("qormOnWifi(" + (self?.jsString("{\"ssid\":\"" + n.ssid + "\"}") ?? "{}") + ")")
                    } else {
                        self?.js("qormOnWifi(" + (self?.jsString("{\"error\":\"iOS blocks Wi-Fi scanning; SSID needs the Access-WiFi-Information entitlement.\"}") ?? "{}") + ")")
                    }
                }
            }
        case "screenshot":
            webView.takeSnapshot(with: nil) { [weak self] image, _ in
                if let img = image, let data = img.jpegData(compressionQuality: 0.8) {
                    self?.js("qormOnScreenshot(" + (self?.jsString("data:image/jpeg;base64," + data.base64EncodedString()) ?? "\"\"") + ")")
                } else { self?.js("qormOnScreenshot(\"\")") }
            }
        case "screenRecordStart":
            RPScreenRecorder.shared().startRecording { [weak self] err in
                DispatchQueue.main.async { self?.js("qormOnScreenRecord(" + (self?.jsString(err == nil ? "● recording…" : "failed to start") ?? "\"\"") + ")") }
            }
        case "screenRecordStop":
            RPScreenRecorder.shared().stopRecording { [weak self] preview, _ in
                DispatchQueue.main.async {
                    if let preview = preview {
                        preview.previewControllerDelegate = self
                        self?.present(preview, animated: true)
                    }
                    self?.js("qormOnScreenRecord(" + (self?.jsString("stopped — preview to save/share") ?? "\"\"") + ")")
                }
            }
        case "share":
            var items: [Any] = []
            if let t = body["text"] as? String, !t.isEmpty { items.append(t) }
            if let u = body["url"] as? String, let url = URL(string: u) { items.append(url) }
            let av = UIActivityViewController(activityItems: items, applicationActivities: nil)
            av.popoverPresentationController?.sourceView = self.view
            av.popoverPresentationController?.sourceRect = CGRect(x: self.view.bounds.midX, y: self.view.bounds.midY, width: 0, height: 0)
            self.present(av, animated: true)
            js("qormOnShare(true)")
        case "secureSet":
            let sk = body["key"] as? String ?? ""
            let sv = (body["value"] as? String ?? "").data(using: .utf8) ?? Data()
            let sq: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: sk]
            SecItemDelete(sq as CFDictionary)
            var sadd = sq; sadd[kSecValueData as String] = sv
            SecItemAdd(sadd as CFDictionary, nil)
            js("qormOnSecure(" + jsString(sk) + ", " + jsString("saved") + ")")
        case "secureGet":
            let gk = body["key"] as? String ?? ""
            let gq: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: gk, kSecReturnData as String: true]
            var gout: AnyObject?
            SecItemCopyMatching(gq as CFDictionary, &gout)
            let gval = (gout as? Data).flatMap { String(data: $0, encoding: .utf8) } ?? ""
            js("qormOnSecure(" + jsString(gk) + ", " + jsString(gval) + ")")
        case "pickFile":
            let dp = UIDocumentPickerViewController(forOpeningContentTypes: [.item])
            dp.delegate = self
            self.present(dp, animated: true)
        case "pickContact":
            let cp = CNContactPickerViewController()
            cp.delegate = self
            self.present(cp, animated: true)
        case "addEvent":
            let store = EKEventStore()
            store.requestAccess(to: .event) { [weak self] granted, _ in
                DispatchQueue.main.async {
                    guard granted else { self?.js("qormOnCalendar(" + (self?.jsString("denied") ?? "\"\"") + ")"); return }
                    let ev = EKEvent(eventStore: store)
                    ev.title = body["title"] as? String ?? "QORM Event"
                    ev.startDate = Date().addingTimeInterval(3600)
                    ev.endDate = Date().addingTimeInterval(7200)
                    ev.calendar = store.defaultCalendarForNewEvents
                    do { try store.save(ev, span: .thisEvent); self?.js("qormOnCalendar(" + (self?.jsString("added") ?? "\"\"") + ")") }
                    catch { self?.js("qormOnCalendar(" + (self?.jsString("error") ?? "\"\"") + ")") }
                }
            }
        case "pickPhoto":
            var cfg = PHPickerConfiguration(); cfg.filter = .images; cfg.selectionLimit = 1
            let picker = PHPickerViewController(configuration: cfg); picker.delegate = self
            self.present(picker, animated: true)
        case "recordVideo":
            let vp = UIImagePickerController()
            vp.sourceType = .camera
            vp.mediaTypes = ["public.movie"]
            vp.videoQuality = .typeLow
            vp.videoMaximumDuration = 20
            vp.delegate = self
            self.present(vp, animated: true)
        case "scanQR":
            AVCaptureDevice.requestAccess(for: .video) { [weak self] granted in
                DispatchQueue.main.async {
                    guard let self = self else { return }
                    guard granted else { self.js("qormOnScan(" + self.jsString("camera denied") + ")"); return }
                    self.startQRScanner()
                }
            }

        case "listenStart":
            let sttLang = body["lang"] as? String ?? Locale.preferredLanguages.first ?? "en-US"
            speechRecognizer = SFSpeechRecognizer(locale: Locale(identifier: sttLang)) ?? SFSpeechRecognizer()
            SFSpeechRecognizer.requestAuthorization { st in
                DispatchQueue.main.async {
                    if st == .authorized { self.startSTT() } else { self.js("qormOnSpeech(" + self.jsString("") + ")") }
                }
            }
        case "listenStop":
            sttEngine.stop()
            sttRequest?.endAudio()
            sttTask?.cancel()
        case "headingStart":
            loc.startUpdatingHeading()
        case "headingStop":
            loc.stopUpdatingHeading()
        case "proximityStart":
            UIDevice.current.isProximityMonitoringEnabled = true
            NotificationCenter.default.addObserver(self, selector: #selector(self.qormProximityChanged), name: UIDevice.proximityStateDidChangeNotification, object: nil)
        case "proximityStop":
            UIDevice.current.isProximityMonitoringEnabled = false
        case "pedometerStart":
            guard CMPedometer.isStepCountingAvailable() else { self.js("qormOnSteps(0)"); return }
            // from the start of today, so it shows today's cumulative steps
            // immediately (not just steps taken after the tap) + live updates.
            pedometer.startUpdates(from: Calendar.current.startOfDay(for: Date())) { [weak self] data, err in
                DispatchQueue.main.async {
                    if let d = data { self?.js("qormOnSteps(" + String(d.numberOfSteps.intValue) + ")") }
                    else if err != nil { self?.js("qormOnSteps(0)") }
                }
            }
        case "pedometerStop":
            pedometer.stopUpdates()
        case "barometerStart":
            if CMAltimeter.isRelativeAltitudeAvailable() {
                altimeter.startRelativeAltitudeUpdates(to: .main) { [weak self] data, _ in
                    if let d = data { self?.js("qormOnPressure(" + String(d.pressure.doubleValue) + ")") }
                }
            }
        case "barometerStop":
            altimeter.stopRelativeAltitudeUpdates()
        case "getModes":
            let low = ProcessInfo.processInfo.isLowPowerModeEnabled
            let dark = self.traitCollection.userInterfaceStyle == .dark
            let modes: [String: Any] = ["lowPower": low, "darkMode": dark, "airplane": NSNull(), "dnd": NSNull()]
            if let jd = try? JSONSerialization.data(withJSONObject: modes), let mj = String(data: jd, encoding: .utf8) {
                self.js("qormOnModes(" + self.jsString(mj) + ")")
            }
        case "updateWidget":
            let g = "group." + (Bundle.main.bundleIdentifier ?? "")
            if let d = UserDefaults(suiteName: g) {
                if let title = body["title"] as? String { d.set(title, forKey: "widget_title") }
                if let lines = body["lines"], let ld = try? JSONSerialization.data(withJSONObject: lines), let ls = String(data: ld, encoding: .utf8) { d.set(ls, forKey: "widget_lines") }
                d.synchronize()
                qormReloadWidgets()
                self.js("qormOnWidget(" + self.jsString("updated") + ")")
            } else {
                self.js("qormOnWidget(" + self.jsString("NO APP GROUP: " + g) + ")")
            }
        case "pendingShortcut":
            if let sc = qormPendingShortcut { qormPendingShortcut = nil; self.fireShortcut(sc) }
        case "getInsets":
            let ins = self.view.safeAreaInsets
            let im: [String: Any] = ["top": Int(ins.top), "bottom": Int(ins.bottom), "left": Int(ins.left), "right": Int(ins.right)]
            if let jd = try? JSONSerialization.data(withJSONObject: im), let ij = String(data: jd, encoding: .utf8) {
                self.js("qormOnInsets(" + self.jsString(ij) + ")")
            }
        case "speak":
            let u = AVSpeechUtterance(string: body["text"] as? String ?? "")
            if let lang = body["lang"] as? String, let v = AVSpeechSynthesisVoice(language: lang) { u.voice = v }
            try? AVAudioSession.sharedInstance().setCategory(.playback, options: [.duckOthers, .mixWithOthers])
            try? AVAudioSession.sharedInstance().setActive(true)
            speechSynth.speak(u)
        case "speakStop":
            speechSynth.stopSpeaking(at: .immediate)
        case "openURL":
            if let u = body["url"] as? String, let url = URL(string: u) { UIApplication.shared.open(url) }
        case "clipboardSet":
            UIPasteboard.general.string = body["text"] as? String ?? ""
            js("qormOnClipboard(" + jsString(UIPasteboard.general.string ?? "") + ")")
        case "clipboardGet":
            js("qormOnClipboard(" + jsString(UIPasteboard.general.string ?? "") + ")")
        case "deviceInfo":
            let d = UIDevice.current
            let info = "{\"model\":\"" + d.model + "\",\"name\":\"" + d.name + "\",\"os\":\"" + d.systemName + " " + d.systemVersion + "\",\"screen\":\"" + String(Int(UIScreen.main.bounds.width)) + "x" + String(Int(UIScreen.main.bounds.height)) + "\"}"
            js("qormOnDeviceInfo(" + jsString(info) + ")")
        case "networkStatus":
            let mon = NWPathMonitor()
            mon.pathUpdateHandler = { [weak self] path in
                let online = path.status == .satisfied
                let type = path.usesInterfaceType(.wifi) ? "wifi" : (path.usesInterfaceType(.cellular) ? "cellular" : "other")
                DispatchQueue.main.async { self?.js("qormOnNetwork(" + (self?.jsString("{\"online\":" + (online ? "true" : "false") + ",\"type\":\"" + type + "\"}") ?? "\"\"") + ")") }
                mon.cancel()
            }
            mon.start(queue: DispatchQueue.global())
        case "keepAwake":
            UIApplication.shared.isIdleTimerDisabled = body["on"] as? Bool ?? false
        case "haptic":
            switch body["type"] as? String ?? "success" {
            case "warning": UINotificationFeedbackGenerator().notificationOccurred(.warning)
            case "error": UINotificationFeedbackGenerator().notificationOccurred(.error)
            case "selection": UISelectionFeedbackGenerator().selectionChanged()
            case "light": UIImpactFeedbackGenerator(style: .light).impactOccurred()
            case "heavy": UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
            case "medium": UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            default: UINotificationFeedbackGenerator().notificationOccurred(.success)
            }
        case "storageSet":
            UserDefaults.standard.set(body["value"] as? String ?? "", forKey: body["key"] as? String ?? "")
        case "storageGet":
            let k = body["key"] as? String ?? ""
            js("qormOnStorage(" + jsString(k) + ", " + jsString(UserDefaults.standard.string(forKey: k) ?? "") + ")")
        case "notify":
            let ntitle = body["title"] as? String ?? "QORM"
            let nbody = body["body"] as? String ?? ""
            let center = UNUserNotificationCenter.current()
            center.delegate = self
            center.requestAuthorization(options: [.alert, .sound, .badge]) { [weak self] granted, _ in
                DispatchQueue.main.async {
                    guard granted else { self?.js("window.qormOnNotify&&qormOnNotify(false)"); return }
                    let content = UNMutableNotificationContent()
                    content.title = ntitle
                    content.body = nbody
                    content.sound = .default
                    let req = UNNotificationRequest(identifier: UUID().uuidString, content: content,
                        trigger: UNTimeIntervalNotificationTrigger(timeInterval: 0.3, repeats: false))
                    center.add(req) { err in
                        DispatchQueue.main.async { self?.js("window.qormOnNotify&&qormOnNotify(" + (err == nil ? "true" : "false") + ")") }
                    }
                }
            }
        case "badge":
            let n = body["count"] as? Int ?? 0
            let center = UNUserNotificationCenter.current()
            center.requestAuthorization(options: [.badge]) { granted, _ in
                DispatchQueue.main.async {
                    guard granted else { return }
                    if #available(iOS 16.0, *) { center.setBadgeCount(n) }
                    else { UIApplication.shared.applicationIconBadgeNumber = n }
                }
            }
        default:
            self.qormUserOp(op, body)
        }
    }

    // iOS shows a notification while the app is in the FOREGROUND only if the
    // delegate opts in — without this, notify silently does nothing during use.
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list, .sound, .badge])
    }
    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput metadataObjects: [AVMetadataObject], from connection: AVCaptureConnection) {
        guard let obj = metadataObjects.first as? AVMetadataMachineReadableCodeObject, let val = obj.stringValue else { return }
        qrSession?.stopRunning()
        self.presentedViewController?.dismiss(animated: true)
        self.js("qormOnScan(" + self.jsString(val) + ")")
    }
    func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
        picker.dismiss(animated: true)
        // WKWebView can't load a file:// temp path in a <video src>, so hand back a
        // base64 data URL the video element can actually play.
        guard let url = info[.mediaURL] as? URL, let data = try? Data(contentsOf: url) else {
            self.js("qormOnVideo(" + self.jsString("") + ")"); return
        }
        self.js("qormOnVideo(" + self.jsString("data:video/quicktime;base64," + data.base64EncodedString()) + ")")
    }
    func imagePickerControllerDidCancel(_ picker: UIImagePickerController) { picker.dismiss(animated: true) }
    func startQRScanner() {
            guard let device = AVCaptureDevice.default(for: .video), let input = try? AVCaptureDeviceInput(device: device) else { self.js("qormOnScan(" + self.jsString("") + ")"); return }
            let session = AVCaptureSession()
            if session.canAddInput(input) { session.addInput(input) }
            let output = AVCaptureMetadataOutput()
            if session.canAddOutput(output) {
                session.addOutput(output)
                output.setMetadataObjectsDelegate(self, queue: .main)
                output.metadataObjectTypes = [.qr, .ean13, .code128, .code39, .pdf417]
            }
            self.qrSession = session
            let scanVC = UIViewController()
            scanVC.view.backgroundColor = .black
            let preview = AVCaptureVideoPreviewLayer(session: session)
            preview.frame = UIScreen.main.bounds
            preview.videoGravity = .resizeAspectFill
            scanVC.view.layer.addSublayer(preview)
            let closeBtn = UIButton(type: .system)
            closeBtn.setTitle("Close", for: .normal)
            closeBtn.setTitleColor(.white, for: .normal)
            closeBtn.frame = CGRect(x: 20, y: 52, width: 90, height: 40)
            closeBtn.addTarget(self, action: #selector(self.closeQRScanner), for: .touchUpInside)
            scanVC.view.addSubview(closeBtn)
            self.present(scanVC, animated: true) { DispatchQueue.global(qos: .userInitiated).async { session.startRunning() } }
    }
    @objc func closeQRScanner() {
        qrSession?.stopRunning()
        self.presentedViewController?.dismiss(animated: true)
    }
    func startSTT() {
        sttTask?.cancel(); sttTask = nil
        let session = AVAudioSession.sharedInstance()
        try? session.setCategory(.record, mode: .measurement, options: .duckOthers)
        try? session.setActive(true, options: .notifyOthersOnDeactivation)
        sttRequest = SFSpeechAudioBufferRecognitionRequest()
        sttRequest?.shouldReportPartialResults = true
        let inputNode = sttEngine.inputNode
        sttTask = speechRecognizer?.recognitionTask(with: sttRequest!) { [weak self] result, _ in
            if let result = result {
                self?.js("qormOnSpeech(" + (self?.jsString(result.bestTranscription.formattedString) ?? "\"\"") + ")")
            }
        }
        let fmt = inputNode.outputFormat(forBus: 0)
        inputNode.installTap(onBus: 0, bufferSize: 1024, format: fmt) { [weak self] buf, _ in self?.sttRequest?.append(buf) }
        sttEngine.prepare()
        try? sttEngine.start()
    }
    func previewControllerDidFinish(_ previewController: RPPreviewViewController) {
        previewController.dismiss(animated: true)
    }
    func contactPicker(_ picker: CNContactPickerViewController, didSelect contact: CNContact) {
        let nm = contact.givenName + " " + contact.familyName
        let ph = contact.phoneNumbers.first?.value.stringValue ?? ""
        self.js("qormOnContact(" + self.jsString("{\"name\":\"" + nm + "\",\"phone\":\"" + ph + "\"}") + ")")
    }
    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard let u = urls.first else { self.js("qormOnFile(" + self.jsString("{}") + ")"); return }
        let acc = u.startAccessingSecurityScopedResource()
        defer { if acc { u.stopAccessingSecurityScopedResource() } }
        let data = (try? Data(contentsOf: u)) ?? Data()
        let json = "{\"name\":\"" + u.lastPathComponent + "\",\"size\":" + String(data.count) + ",\"dataURL\":\"data:application/octet-stream;base64," + data.base64EncodedString() + "\"}"
        self.js("qormOnFile(" + self.jsString(json) + ")")
    }
    func picker(_ picker: PHPickerViewController, didFinishPicking results: [PHPickerResult]) {
        picker.dismiss(animated: true)
        guard let r = results.first else { self.js("qormOnPhoto(\"\")"); return }
        r.itemProvider.loadObject(ofClass: UIImage.self) { [weak self] obj, _ in
            if let img = obj as? UIImage, let data = img.jpegData(compressionQuality: 0.8) {
                DispatchQueue.main.async { self?.js("qormOnPhoto(" + (self?.jsString("data:image/jpeg;base64," + data.base64EncodedString()) ?? "\"\"") + ")") }
            }
        }
    }
    //__QORM_USER_IOS__

    func locationManager(_ m: CLLocationManager, didUpdateHeading newHeading: CLHeading) {
        js("qormOnHeading(" + String(newHeading.magneticHeading) + ")")
    }
    @objc func qormProximityChanged() {
        js("qormOnProximity(" + (UIDevice.current.proximityState ? "true" : "false") + ")")
    }
    func locationManager(_ m: CLLocationManager, didUpdateLocations locs: [CLLocation]) {
        guard let c = locs.last else { return }
        js("qormOnLocation(\(c.coordinate.latitude),\(c.coordinate.longitude),\(c.horizontalAccuracy))")
    }
    func locationManager(_ m: CLLocationManager, didFailWithError error: Error) {
        js("qormOnLocationError(\(jsString(error.localizedDescription)))")
    }

    func centralManagerDidUpdateState(_ c: CBCentralManager) {
        js("qormOnBluetoothState(" + (c.state == .poweredOn ? "true" : "false") + ")")
        if btScanning {
            if c.state == .poweredOn { c.scanForPeripherals(withServices: nil, options: nil) }
            else { js("qormOnBluetooth(" + jsString("[]") + ")") }
        }
    }
    func centralManager(_ c: CBCentralManager, didDiscover p: CBPeripheral, advertisementData: [String: Any], rssi RSSI: NSNumber) {
        bt[p.identifier.uuidString] = ["name": p.name ?? (advertisementData[CBAdvertisementDataLocalNameKey] as? String ?? "(unknown)"), "rssi": RSSI.intValue]
    }
    func reportBT() {
        var items: [String] = []
        for (_, d) in bt {
            let name = (d["name"] as? String ?? "(unknown)").replacingOccurrences(of: "\"", with: "")
            let rssi = d["rssi"] as? Int ?? 0
            items.append("{\"name\":\"" + name + "\",\"rssi\":" + String(rssi) + "}")
        }
        js("qormOnBluetooth(" + jsString("[" + items.joined(separator: ",") + "]") + ")")
    }

    func readerSession(_ s: NFCNDEFReaderSession, didDetectNDEFs messages: [NFCNDEFMessage]) {
        var text = ""
        for m in messages { for r in m.records { if let p = String(data: r.payload, encoding: .utf8) { text += p } } }
        let clean = text.replacingOccurrences(of: "\"", with: "").replacingOccurrences(of: "\n", with: " ")
        DispatchQueue.main.async { self.js("qormOnNfc(" + self.jsString("{\"text\":\"" + clean + "\"}") + ")") }
    }
    func readerSession(_ s: NFCNDEFReaderSession, didInvalidateWithError error: Error) {}

    func js(_ s: String) { webView.evaluateJavaScript(s, completionHandler: nil) }
    func fireShortcut(_ id: String) { let p = self.jsString(id); self.js("(function(){var f=function(){if(window.qormEmit){qormEmit('shortcut'," + p + ");}else{setTimeout(f,300);}};f();})()") }
    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let a = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        a.addAction(UIAlertAction(title: "OK", style: .default) { _ in completionHandler() })
        self.present(a, animated: true)
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let a = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        a.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in completionHandler(false) })
        a.addAction(UIAlertAction(title: "OK", style: .default) { _ in completionHandler(true) })
        self.present(a, animated: true)
    }
    func qormDeliverPending() { if let s = qormPendingShortcut { qormPendingShortcut = nil; self.fireShortcut(s) } }
    func jsString(_ s: String) -> String {
        let esc = s.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
        return "\"" + esc + "\""
    }
`
}

// iosViewController builds the iOS WKWebView host. Both the dev client and the
// offline (WASM) package embed the SAME native hardware bridge (iosBridgeBody)
// so the app reaches native hardware via qormToNative on either — the offline
// package differs only in serving the bundled WASM over a custom scheme.
func iosViewController(dev string) string {
	imports := `import UIKit
import WebKit
import CoreLocation
import CoreMotion
import AVFoundation
import LocalAuthentication
import CoreBluetooth
import NetworkExtension
import CoreNFC
import MediaPlayer
import AudioToolbox
import UserNotifications
import ReplayKit
import Network
import PhotosUI
import UniformTypeIdentifiers
import Security
import Speech
import Contacts
import ContactsUI
import EventKit`
	conf := ", WKScriptMessageHandler, WKUIDelegate, CLLocationManagerDelegate, CBCentralManagerDelegate, NFCNDEFReaderSessionDelegate, UNUserNotificationCenterDelegate, RPPreviewViewControllerDelegate, PHPickerViewControllerDelegate, UIDocumentPickerDelegate, AVCaptureMetadataOutputObjectsDelegate, UIImagePickerControllerDelegate, UINavigationControllerDelegate, CNContactPickerDelegate"
	if dev != "" {
		return imports + `

// DEV client: connects to the live QORM dev server AND bridges native hardware.
class ViewController: UIViewController, WKNavigationDelegate` + conf + ` {
    var webView: WKWebView!
    let status = UILabel()
    let url = URL(string: "` + dev + `")!
` + iosBridgeBody() + `
    override func loadView() {
        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "qorm")
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []
        webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.scrollView.minimumZoomScale = 1
        webView.scrollView.maximumZoomScale = 1
        webView.scrollView.bouncesZoom = false
        webView.allowsLinkPreview = false
        webView.navigationDelegate = self
        webView.uiDelegate = self
        view = webView
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        loc.delegate = self
        status.text = "Connecting to\n` + dev + `…"
        status.numberOfLines = 0
        status.textAlignment = .center
        status.textColor = .gray
        status.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(status)
        volumeView.alpha = 0.01
        view.addSubview(volumeView)
        NSLayoutConstraint.activate([
            status.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            status.centerYAnchor.constraint(equalTo: view.centerYAnchor),
            status.leadingAnchor.constraint(greaterThanOrEqualTo: view.leadingAnchor, constant: 24),
        ])
        load()
    }

    func load() { webView.load(URLRequest(url: url)) }
    func retry() { DispatchQueue.main.asyncAfter(deadline: .now() + 1.2) { [weak self] in self?.load() } }
    // trust the dev server's self-signed cert (if https is used)
    func webView(_ w: WKWebView, didReceive challenge: URLAuthenticationChallenge,
                 completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        if let trust = challenge.protectionSpace.serverTrust {
            completionHandler(.useCredential, URLCredential(trust: trust))
        } else {
            completionHandler(.performDefaultHandling, nil)
        }
    }

    func webView(_ w: WKWebView, didFinish nav: WKNavigation!) { status.isHidden = true; qormDeliverPending() }
    func webView(_ w: WKWebView, didFailProvisionalNavigation nav: WKNavigation!, withError error: Error) {
        status.text = "Connecting to\n` + dev + `…\n(retrying)"
        retry()
    }
    func webView(_ w: WKWebView, didFail nav: WKNavigation!, withError error: Error) { retry() }
}
`
	}
	return imports + `

// Offline: serves the bundled WASM payload over a custom scheme AND carries the
// full native hardware bridge, so the shipped app's Go/WASM middle-layer reaches
// hardware (incl. iOS bluetooth/NFC) via qormToNative, not just Web APIs.
class ViewController: UIViewController, WKURLSchemeHandler, WKNavigationDelegate` + conf + ` {
    var webView: WKWebView!

    override func loadView() {
        let config = WKWebViewConfiguration()
        config.setURLSchemeHandler(self, forURLScheme: "qormapp")
        config.userContentController.add(self, name: "qorm")
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []
        webView = WKWebView(frame: .zero, configuration: config)
        webView.scrollView.minimumZoomScale = 1
        webView.scrollView.maximumZoomScale = 1
        webView.scrollView.bouncesZoom = false
        webView.allowsLinkPreview = false
        webView.navigationDelegate = self
        webView.uiDelegate = self
        view = webView
    }
    func webView(_ w: WKWebView, didFinish nav: WKNavigation!) { qormDeliverPending() }

    override func viewDidLoad() {
        super.viewDidLoad()
        loc.delegate = self
        volumeView.alpha = 0.01
        view.addSubview(volumeView)
        webView.load(URLRequest(url: URL(string: "qormapp://app/index.html")!))
    }
` + iosBridgeBody() + `
    func webView(_ webView: WKWebView, start task: WKURLSchemeTask) {
        guard let url = task.request.url else { return }
        var path = url.path
        if path.isEmpty || path == "/" { path = "/index.html" }
        let base = Bundle.main.resourceURL!.appendingPathComponent("www")
        let fileURL = base.appendingPathComponent(path)
        if let data = try? Data(contentsOf: fileURL) {
            let resp = HTTPURLResponse(url: url, statusCode: 200, httpVersion: "HTTP/1.1",
                                       headerFields: ["Content-Type": mimeType(for: path)])!
            task.didReceive(resp); task.didReceive(data); task.didFinish()
        } else {
            task.didFailWithError(NSError(domain: "qorm", code: 404))
        }
    }
    func webView(_ webView: WKWebView, stop task: WKURLSchemeTask) {}

    func mimeType(for p: String) -> String {
        if p.hasSuffix(".wasm") { return "application/wasm" }
        if p.hasSuffix(".js") { return "text/javascript" }
        if p.hasSuffix(".html") { return "text/html" }
        if p.hasSuffix(".json") || p.hasSuffix(".webmanifest") { return "application/json" }
        if p.hasSuffix(".png") { return "image/png" }
        return "application/octet-stream"
    }
}
`
}

// iosSources lists the project sources: dev has no bundled www payload.
func iosSources(dev string) string {
	if dev != "" {
		return "\n      - path: Sources\n      - path: Assets.xcassets"
	}
	return "\n      - path: Sources\n      - path: Assets.xcassets\n      - path: www\n        type: folder"
}

// iosInfo emits an explicit Info.plist (via XcodeGen) for the dev client so it
// may reach a plain-HTTP dev server on the local network.
func iosInfo(dev, appName, appDir string) string {
	if dev != "" {
		return `    info:
      path: Sources/Info.plist
      properties:
        CFBundleDisplayName: "` + appName + `"
        UILaunchScreen: {}
        NSLocalNetworkUsageDescription: "QORM Dev connects to your dev server."
        NSCameraUsageDescription: "QORM Dev can use the camera so any app you test can."
        NSPhotoLibraryUsageDescription: "QORM Dev can access photos so any app you test can."
        NSPhotoLibraryAddUsageDescription: "QORM Dev can save photos so any app you test can."
        NSMicrophoneUsageDescription: "QORM Dev can use the microphone so any app you test can."
        NSMotionUsageDescription: "QORM Dev can read motion/step/altitude so any app you test can."
        NSCalendarsUsageDescription: "QORM Dev can add calendar events so any app you test can."
        NSSpeechRecognitionUsageDescription: "QORM Dev can transcribe speech so any app you test can."
        NSLocationWhenInUseUsageDescription: "QORM Dev can use location so any app you test can."
        NSLocationAlwaysAndWhenInUseUsageDescription: "QORM Dev can use location so any app you test can."
        NSBluetoothAlwaysUsageDescription: "QORM Dev can use Bluetooth so any app you test can."
        NSBluetoothPeripheralUsageDescription: "QORM Dev can use Bluetooth so any app you test can."
        NSFaceIDUsageDescription: "QORM Dev can use Face ID so any app you test can."
        NSNfcReaderUsageDescription: "QORM Dev can read NFC so any app you test can."
        NSAppTransportSecurity:
          NSAllowsArbitraryLoads: true
          NSAllowsLocalNetworking: true
`
	}
	// packaged: explicit Info.plist with derived perms + STATIC shortcuts (these
	// appear immediately after install and carry SF Symbol icons, unlike
	// runtime-registered dynamic shortcuts which vanish until the first launch).
	var b strings.Builder
	b.WriteString("    info:\n      path: Sources/Info.plist\n      properties:\n")
	b.WriteString("        CFBundleDisplayName: \"" + appName + "\"\n")
	b.WriteString("        UILaunchScreen: {}\n")
	for _, k := range capability.PermsFor(usedWidgets(appDir), capability.IOS) {
		b.WriteString("        " + k + ": \"" + appName + " " + capability.IOSPermReason(k) + "\"\n")
	}
	scs := iosScFor(dev, appDir)
	if len(scs) > 0 {
		b.WriteString("        UIApplicationShortcutItems:\n")
		for _, sc := range scs {
			b.WriteString("          - UIApplicationShortcutItemType: " + sc.ID + "\n")
			b.WriteString("            UIApplicationShortcutItemTitle: \"" + sc.Title + "\"\n")
			if sc.Subtitle != "" {
				b.WriteString("            UIApplicationShortcutItemSubtitle: \"" + sc.Subtitle + "\"\n")
			}
			if sc.Icon != "" {
				b.WriteString("            UIApplicationShortcutItemIconSymbolName: " + sc.Icon + "\n")
			}
		}
	}
	return b.String()
}

// iosGenInfo emits the generated-Info.plist settings for the offline build
// (the dev build supplies its own Info.plist above).
func iosGenInfo(dev, appName, appDir string) string {
	// perms + shortcuts now live in the explicit Info.plist from iosInfo (both dev
	// and packaged), so no generated-plist settings are needed.
	return ""
}

// awProvider returns the first declared widget (or a name-only placeholder).
func awProvider(ws []model.Widget, name string) model.Widget {
	if len(ws) > 0 {
		return ws[0]
	}
	return model.Widget{Name: name}
}

// androidWidgetReceiver returns the manifest <receiver> for the home-screen
// widget, or "" when the app declares none.
func androidWidgetReceiver(hasWidget bool) string {
	if !hasWidget {
		return ""
	}
	return `        <receiver android:name=".QormWidget" android:exported="false">
            <intent-filter>
                <action android:name="android.appwidget.action.APPWIDGET_UPDATE"/>
            </intent-filter>
            <meta-data android:name="android.appwidget.provider" android:resource="@xml/qorm_widget_info"/>
        </receiver>
`
}

// androidWidgetProvider generates the AppWidgetProvider that renders a title +
// label/value lines from SharedPreferences the app keeps updated.
func androidWidgetProvider(ns string, w model.Widget) string {
	title := w.Title
	if title == "" {
		title = w.Name
	}
	baked := "[]"
	if len(w.Lines) > 0 {
		var b strings.Builder
		b.WriteString("[")
		for i, ln := range w.Lines {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{\"label\":\"` + ln.Label + `\",\"value\":\"` + ln.Value + `\"}`)
		}
		b.WriteString("]")
		baked = b.String()
	}
	return `package ` + ns + `;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.Context;
import android.widget.RemoteViews;
public class QormWidget extends AppWidgetProvider {
    @Override public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        android.content.SharedPreferences p = ctx.getSharedPreferences("qorm_widget", Context.MODE_PRIVATE);
        String title = p.getString("widget_title", "` + title + `");
        StringBuilder sb = new StringBuilder();
        try {
            org.json.JSONArray arr = new org.json.JSONArray(p.getString("widget_lines", "` + baked + `"));
            for (int i = 0; i < arr.length(); i++) {
                org.json.JSONObject o = arr.getJSONObject(i);
                if (i > 0) sb.append("\n");
                sb.append(o.optString("label")).append(": ").append(o.optString("value"));
            }
        } catch (Exception e) {}
        for (int id : ids) {
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.qorm_widget);
            rv.setTextViewText(R.id.qorm_widget_title, title);
            rv.setTextViewText(R.id.qorm_widget_body, sb.toString());
            mgr.updateAppWidget(id, rv);
        }
    }
}
`
}

// androidWidgetLayout is the RemoteViews layout for the widget.
func androidWidgetLayout() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<LinearLayout xmlns:android="http://schemas.android.com/apk/res/android"
    android:layout_width="match_parent" android:layout_height="match_parent"
    android:orientation="vertical" android:padding="12dp" android:background="#FFFFFF">
    <TextView android:id="@+id/qorm_widget_title" android:layout_width="wrap_content"
        android:layout_height="wrap_content" android:textStyle="bold" android:textSize="16sp" android:textColor="#000000"/>
    <TextView android:id="@+id/qorm_widget_body" android:layout_width="wrap_content"
        android:layout_height="wrap_content" android:textSize="13sp" android:textColor="#333333" android:layout_marginTop="6dp"/>
</LinearLayout>
`
}

// androidWidgetInfo is the appwidget-provider metadata.
func androidWidgetInfo() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<appwidget-provider xmlns:android="http://schemas.android.com/apk/res/android"
    android:minWidth="110dp" android:minHeight="40dp" android:updatePeriodMillis="0"
    android:initialLayout="@layout/qorm_widget" android:resizeMode="horizontal|vertical"
    android:widgetCategory="home_screen"/>
`
}

// androidPerms adds INTERNET permission for the dev client (it reaches a server).
func androidPerms(dev, appDir string) string {
	var perms []string
	if dev == "" {
		// a packaged app declares only what its capabilities use
		perms = append([]string{"android.permission.INTERNET"}, capability.PermsFor(usedWidgets(appDir), capability.Android)...)
	} else {
		perms = []string{
			"android.permission.INTERNET",
			"android.permission.CAMERA",
			"android.permission.READ_CONTACTS",
			"android.permission.RECORD_AUDIO",
			"android.permission.ACCESS_FINE_LOCATION",
			"android.permission.ACCESS_COARSE_LOCATION",
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
			"android.permission.READ_EXTERNAL_STORAGE",
			"android.permission.BLUETOOTH_CONNECT",
			"android.permission.BLUETOOTH_SCAN",
			"android.permission.NFC",
			"android.permission.VIBRATE",
			"android.permission.POST_NOTIFICATIONS",
		}
	}
	var b strings.Builder
	for _, p := range perms {
		b.WriteString("\n    <uses-permission android:name=\"" + p + "\"/>")
	}
	// camera/mic hardware not required (so it installs on devices without them)
	b.WriteString("\n    <uses-feature android:name=\"android.hardware.camera\" android:required=\"false\"/>")
	return b.String()
}

// androidCleartext allows plain-HTTP to the dev server on the LAN.
func androidCleartext(dev string) string {
	if dev == "" {
		return ""
	}
	return "\n        android:usesCleartextTraffic=\"true\""
}

// androidMainActivity returns the Activity: a thin dev client that loads the
// live server URL, or the offline asset-loader client.
// androidScFor returns an app's shortcuts, or none for the dev client.
func androidScFor(dev, appDir string) []model.Shortcut {
	if dev != "" {
		return nil
	}
	return appShortcuts(appDir)
}

// androidShortcutRegister renders the onCreate Java that registers the app's
// icon quick actions with ShortcutManager (each launches MainActivity carrying a
// qorm_shortcut extra).
func androidShortcutRegister(scs []model.Shortcut) string {
	if len(scs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("        if (android.os.Build.VERSION.SDK_INT >= 25) {\n")
	b.WriteString("            android.content.pm.ShortcutManager scMgr = getSystemService(android.content.pm.ShortcutManager.class);\n")
	b.WriteString("            if (scMgr != null) {\n")
	b.WriteString("                java.util.List<android.content.pm.ShortcutInfo> qscs = new java.util.ArrayList<>();\n")
	for _, sc := range scs {
		b.WriteString("                qscs.add(new android.content.pm.ShortcutInfo.Builder(this, " + strconv.Quote(sc.ID) + ").setShortLabel(" + strconv.Quote(sc.Title) + ")")
		if sc.Subtitle != "" {
			b.WriteString(".setLongLabel(" + strconv.Quote(sc.Subtitle) + ")")
		}
		b.WriteString(".setIntent(new android.content.Intent(android.content.Intent.ACTION_VIEW, null, this, MainActivity.class).putExtra(\"qorm_shortcut\", " + strconv.Quote(sc.ID) + ")).build());\n")
	}
	b.WriteString("                try { scMgr.setDynamicShortcuts(qscs); } catch (Exception e) {}\n")
	b.WriteString("            }\n")
	b.WriteString("        }\n")
	b.WriteString("        String qsc0 = getIntent().getStringExtra(\"qorm_shortcut\");\n")
	b.WriteString("        if (qsc0 != null) qormPendingShortcut = qsc0;\n")
	return b.String()
}

func androidMainActivity(ns, dev string, scs []model.Shortcut) string {
	// Both the dev client and the offline (WASM) package embed the SAME native
	// hardware Bridge (window.qormAndroid @JavascriptInterface) so qormToNative
	// reaches native hardware on either. They differ only in how the WebView is
	// fed content: the dev client loads the live server URL over HTTP(S); the
	// offline build serves the bundled WASM/assets over an https virtual host via
	// WebViewAssetLoader so fetch()/WebAssembly.instantiate work from app assets.
	clientSetup := `        wv.setWebViewClient(new android.webkit.WebViewClient() {
            @Override public void onReceivedSslError(WebView view, android.webkit.SslErrorHandler handler, android.net.http.SslError error) {
                handler.proceed();
            }
        });
        wv.addJavascriptInterface(new Bridge(), "qormAndroid");
        setContentView(wv);
        wv.loadUrl("` + dev + `");`
	if dev == "" {
		clientSetup = `        final androidx.webkit.WebViewAssetLoader loader = new androidx.webkit.WebViewAssetLoader.Builder()
            .addPathHandler("/assets/", new androidx.webkit.WebViewAssetLoader.AssetsPathHandler(this))
            .build();
        wv.setWebViewClient(new android.webkit.WebViewClient() {
            @Override public android.webkit.WebResourceResponse shouldInterceptRequest(WebView view, android.webkit.WebResourceRequest request) {
                return loader.shouldInterceptRequest(request.getUrl());
            }
        });
        wv.addJavascriptInterface(new Bridge(), "qormAndroid");
        setContentView(wv);
        wv.loadUrl("https://appassets.androidplatform.net/assets/www/index.html");`
	}
	return `package ` + ns + `;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;
import android.webkit.JavascriptInterface;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.content.Context;
import android.media.MediaRecorder;
import androidx.biometric.BiometricPrompt;
import androidx.core.content.ContextCompat;
import java.util.concurrent.Executor;
import android.util.Base64;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.le.BluetoothLeScanner;
import android.bluetooth.le.ScanCallback;
import android.bluetooth.le.ScanResult;
import android.net.wifi.WifiManager;
import android.net.wifi.WifiInfo;
import android.nfc.NfcAdapter;
import android.nfc.Tag;
import android.media.AudioManager;
import android.os.Vibrator;
import android.os.VibrationEffect;
import android.os.BatteryManager;
import android.hardware.camera2.CameraManager;
import android.content.IntentFilter;
import android.content.Intent;
import android.view.WindowManager;

// QORM client + native hardware bridge (dev AND offline): window.qormAndroid.<op>()
// calls native SensorManager/LocationManager etc.; results are pushed back with
// evaluateJavascript (qormOnLocation / qormOnMotion). Full native access.
public class MainActivity extends androidx.fragment.app.FragmentActivity implements SensorEventListener, LocationListener {
    WebView wv;
    android.speech.tts.TextToSpeech qormTts;
    android.speech.SpeechRecognizer qormSpeechRec;
    android.hardware.SensorEventListener qormCompassL, qormProxL, qormStepL, qormPressL;
    androidx.activity.result.ActivityResultLauncher<String> qormPhotoPick;
    androidx.activity.result.ActivityResultLauncher<String> qormFilePick;
    androidx.activity.result.ActivityResultLauncher<Void> qormContactPick;
    String qormPendingShortcut;
    SensorManager sm;
    Sensor rot;
    LocationManager lm;
    MediaRecorder mrec;
    String recPath;

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        qormContactPick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.PickContact(), uri -> {
            if (uri == null) { js("qormOnContact('{}')"); return; }
            try {
                android.database.Cursor c = getContentResolver().query(uri, null, null, null, null);
                String name = "";
                if (c != null && c.moveToFirst()) {
                    int idx = c.getColumnIndex(android.provider.ContactsContract.Contacts.DISPLAY_NAME);
                    if (idx >= 0) name = c.getString(idx);
                }
                if (c != null) c.close();
                js("qormOnContact('{\"name\":\"" + name + "\",\"phone\":\"\"}')");
            } catch (Exception e) { js("qormOnContact('{}')"); }
        });
        qormFilePick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.GetContent(), uri -> {
            if (uri == null) { js("qormOnFile('{}')"); return; }
            try {
                java.io.InputStream ins = getContentResolver().openInputStream(uri);
                java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                byte[] buf = new byte[8192]; int n; while ((n = ins.read(buf)) > 0) baos.write(buf, 0, n);
                String name = uri.getLastPathSegment();
                js("qormOnFile('{\"name\":\"" + name + "\",\"size\":" + baos.size() + ",\"dataURL\":\"data:application/octet-stream;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "\"}')");
            } catch (Exception e) { js("qormOnFile('{}')"); }
        });
        qormPhotoPick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.GetContent(), uri -> {
            if (uri == null) { js("qormOnPhoto('')"); return; }
            try {
                java.io.InputStream ins = getContentResolver().openInputStream(uri);
                android.graphics.Bitmap bmp = android.graphics.BitmapFactory.decodeStream(ins);
                java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                bmp.compress(android.graphics.Bitmap.CompressFormat.JPEG, 80, baos);
                js("qormOnPhoto('data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "')");
            } catch (Exception e) { js("qormOnPhoto('')"); }
        });
        try {
            requestPermissions(new String[]{
                "android.permission.CAMERA", "android.permission.RECORD_AUDIO",
                "android.permission.ACCESS_FINE_LOCATION", "android.permission.ACCESS_COARSE_LOCATION"}, 1);
        } catch (Exception e) {}
        wv = new WebView(this);
        wv.getSettings().setJavaScriptEnabled(true);
        wv.getSettings().setDomStorageEnabled(true);
` + clientSetup + `
        sm = (SensorManager) getSystemService(Context.SENSOR_SERVICE);
        rot = sm.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR);
        lm = (LocationManager) getSystemService(Context.LOCATION_SERVICE);
` + androidShortcutRegister(scs) + `    }

    class Bridge {
        @JavascriptInterface public void location(String a) { runOnUiThread(() -> getLoc()); }
        @JavascriptInterface public void motionStart(String a) { runOnUiThread(() -> sm.registerListener(MainActivity.this, rot, SensorManager.SENSOR_DELAY_UI)); }
        @JavascriptInterface public void motionStop(String a) { runOnUiThread(() -> sm.unregisterListener(MainActivity.this)); }
        @JavascriptInterface public void recordStart(String a) { runOnUiThread(() -> startRec()); }
        @JavascriptInterface public void recordStop(String a) { runOnUiThread(() -> stopRec()); }
        @JavascriptInterface public void biometric(String a) { runOnUiThread(() -> doBio()); }
        @JavascriptInterface public void bluetoothScan(String a) { runOnUiThread(() -> btScan()); }
        @JavascriptInterface public void wifiInfo(String a) { runOnUiThread(() -> wifiImpl()); }
        @JavascriptInterface public void nfcRead(String a) { runOnUiThread(() -> nfcEnable()); }
        @JavascriptInterface public void screenshot(String a) {
            runOnUiThread(() -> {
                try {
                    android.graphics.Bitmap bmp = android.graphics.Bitmap.createBitmap(Math.max(1, wv.getWidth()), Math.max(1, wv.getHeight()), android.graphics.Bitmap.Config.ARGB_8888);
                    wv.draw(new android.graphics.Canvas(bmp));
                    java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                    bmp.compress(android.graphics.Bitmap.CompressFormat.JPEG, 80, baos);
                    js("qormOnScreenshot('data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "')");
                } catch (Exception e) { js("qormOnScreenshot('')"); }
            });
        }
        @JavascriptInterface public void screenRecordStart(String a) { runOnUiThread(() -> js("qormOnScreenRecord('Android screen recording needs MediaProjection (not yet wired)')")); }
        @JavascriptInterface public void screenRecordStop(String a) { runOnUiThread(() -> js("qormOnScreenRecord('')")); }
        @JavascriptInterface public void share(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                String text = o.optString("text",""), url = o.optString("url","");
                android.content.Intent i = new android.content.Intent(android.content.Intent.ACTION_SEND);
                i.setType("text/plain");
                i.putExtra(android.content.Intent.EXTRA_TEXT, text + (url.isEmpty()?"":" "+url));
                startActivity(android.content.Intent.createChooser(i, "Share"));
                js("qormOnShare(true)");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void clipboardSet(String a) {
            runOnUiThread(() -> { try {
                String text = new org.json.JSONObject(a).optString("text","");
                android.content.ClipboardManager cb = (android.content.ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                cb.setPrimaryClip(android.content.ClipData.newPlainText("qorm", text));
                js("qormOnClipboard(" + jsStr(text) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void clipboardGet(String a) {
            runOnUiThread(() -> {
                android.content.ClipboardManager cb = (android.content.ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                String text = "";
                if (cb.hasPrimaryClip() && cb.getPrimaryClip().getItemCount() > 0) text = String.valueOf(cb.getPrimaryClip().getItemAt(0).getText());
                js("qormOnClipboard(" + jsStr(text) + ")");
            });
        }
        @JavascriptInterface public void deviceInfo(String a) {
            runOnUiThread(() -> {
                String info = "{\"model\":\"" + android.os.Build.MODEL + "\",\"name\":\"" + android.os.Build.MANUFACTURER + "\",\"os\":\"Android " + android.os.Build.VERSION.RELEASE + "\",\"screen\":\"" + getResources().getDisplayMetrics().widthPixels + "x" + getResources().getDisplayMetrics().heightPixels + "\"}";
                js("qormOnDeviceInfo(" + jsStr(info) + ")");
            });
        }
        @JavascriptInterface public void networkStatus(String a) {
            runOnUiThread(() -> {
                android.net.ConnectivityManager cm = (android.net.ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
                android.net.NetworkCapabilities nc = cm.getNetworkCapabilities(cm.getActiveNetwork());
                boolean online = nc != null;
                String type = nc == null ? "none" : (nc.hasTransport(android.net.NetworkCapabilities.TRANSPORT_WIFI) ? "wifi" : nc.hasTransport(android.net.NetworkCapabilities.TRANSPORT_CELLULAR) ? "cellular" : "other");
                js("qormOnNetwork(" + jsStr("{\"online\":" + online + ",\"type\":\"" + type + "\"}") + ")");
            });
        }
        @JavascriptInterface public void keepAwake(String a) {
            runOnUiThread(() -> { try {
                boolean on = new org.json.JSONObject(a).optBoolean("on", false);
                if (on) getWindow().addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                else getWindow().clearFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void haptic(String a) {
            runOnUiThread(() -> { try {
                String type = new org.json.JSONObject(a).optString("type", "success");
                android.os.Vibrator v = (android.os.Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
                long ms = type.equals("error") ? 100 : type.equals("heavy") ? 80 : type.equals("light") ? 15 : 40;
                if (android.os.Build.VERSION.SDK_INT >= 26) v.vibrate(android.os.VibrationEffect.createOneShot(ms, android.os.VibrationEffect.DEFAULT_AMPLITUDE));
                else v.vibrate(ms);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void storageSet(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                getSharedPreferences("qorm", Context.MODE_PRIVATE).edit().putString(o.optString("key",""), o.optString("value","")).apply();
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void storageGet(String a) {
            runOnUiThread(() -> { try {
                String key = new org.json.JSONObject(a).optString("key","");
                String val = getSharedPreferences("qorm", Context.MODE_PRIVATE).getString(key, "");
                js("qormOnStorage(" + jsStr(key) + ", " + jsStr(val) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void listenStart(String a) {
            runOnUiThread(() -> {
                if (qormSpeechRec == null) qormSpeechRec = android.speech.SpeechRecognizer.createSpeechRecognizer(MainActivity.this);
                qormSpeechRec.setRecognitionListener(new android.speech.RecognitionListener() {
                    public void onResults(android.os.Bundle b) {
                        java.util.ArrayList<String> r = b.getStringArrayList(android.speech.SpeechRecognizer.RESULTS_RECOGNITION);
                        if (r != null && !r.isEmpty()) js("qormOnSpeech(" + jsStr(r.get(0)) + ")");
                    }
                    public void onReadyForSpeech(android.os.Bundle b) {}
                    public void onBeginningOfSpeech() {}
                    public void onRmsChanged(float v) {}
                    public void onBufferReceived(byte[] b) {}
                    public void onEndOfSpeech() {}
                    public void onError(int e) {}
                    public void onPartialResults(android.os.Bundle b) {}
                    public void onEvent(int e, android.os.Bundle b) {}
                });
                android.content.Intent i = new android.content.Intent(android.speech.RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
                i.putExtra(android.speech.RecognizerIntent.EXTRA_LANGUAGE_MODEL, android.speech.RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
                try { String lang = new org.json.JSONObject(a).optString("lang", ""); if (!lang.isEmpty()) i.putExtra(android.speech.RecognizerIntent.EXTRA_LANGUAGE, lang); } catch (Exception e) {}
                qormSpeechRec.startListening(i);
            });
        }
        @JavascriptInterface public void listenStop(String a) { runOnUiThread(() -> { if (qormSpeechRec != null) qormSpeechRec.stopListening(); }); }
        @JavascriptInterface public void lockOrientation(String a) {
            runOnUiThread(() -> { try {
                String mode = new org.json.JSONObject(a).optString("mode", "portrait");
                setRequestedOrientation(mode.equals("landscape") ? android.content.pm.ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE : android.content.pm.ActivityInfo.SCREEN_ORIENTATION_PORTRAIT);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void speak(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                String text = o.optString("text", "");
                String lang = o.optString("lang", "");
                if (qormTts == null) qormTts = new android.speech.tts.TextToSpeech(MainActivity.this, st -> {});
                if (!lang.isEmpty()) qormTts.setLanguage(java.util.Locale.forLanguageTag(lang));
                qormTts.speak(text, android.speech.tts.TextToSpeech.QUEUE_FLUSH, null, "qorm");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void speakStop(String a) {
            runOnUiThread(() -> { if (qormTts != null) qormTts.stop(); });
        }
        @JavascriptInterface public void secureSet(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                qormSecurePrefs().edit().putString(o.optString("key",""), o.optString("value","")).apply();
                js("qormOnSecure(" + jsStr(o.optString("key","")) + ", " + jsStr("saved") + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void secureGet(String a) {
            runOnUiThread(() -> { try {
                String key = new org.json.JSONObject(a).optString("key", "");
                String val = qormSecurePrefs().getString(key, "");
                js("qormOnSecure(" + jsStr(key) + ", " + jsStr(val) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pickContact(String a) { runOnUiThread(() -> { if (qormContactPick != null) qormContactPick.launch(null); }); }
        @JavascriptInterface public void addEvent(String a) {
            runOnUiThread(() -> { try {
                String title = new org.json.JSONObject(a).optString("title", "QORM Event");
                android.content.Intent i = new android.content.Intent(android.content.Intent.ACTION_INSERT)
                    .setData(android.provider.CalendarContract.Events.CONTENT_URI)
                    .putExtra(android.provider.CalendarContract.Events.TITLE, title);
                startActivity(i);
                js("qormOnCalendar('opened')");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pickFile(String a) { runOnUiThread(() -> { if (qormFilePick != null) qormFilePick.launch("*/*"); }); }
        @JavascriptInterface public void pickPhoto(String a) { runOnUiThread(() -> { if (qormPhotoPick != null) qormPhotoPick.launch("image/*"); }); }
        @JavascriptInterface public void openURL(String a) {
            runOnUiThread(() -> { try {
                String url = new org.json.JSONObject(a).optString("url", "");
                startActivity(new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url)));
                js("qormOnOpenUrl(true)");
            } catch (Exception e) { js("qormOnOpenUrl(false)"); } });
        }
        android.hardware.SensorManager qormSM() { return (android.hardware.SensorManager) getSystemService(Context.SENSOR_SERVICE); }
        void qormListen(int type, android.hardware.SensorEventListener[] slot, java.util.function.Function<android.hardware.SensorEvent, String> fn) {
            android.hardware.Sensor sensor = qormSM().getDefaultSensor(type);
            if (sensor == null) return;
            slot[0] = new android.hardware.SensorEventListener() {
                public void onSensorChanged(android.hardware.SensorEvent e) { String out = fn.apply(e); if (out != null) js(out); }
                public void onAccuracyChanged(android.hardware.Sensor sn, int ac) {}
            };
            qormSM().registerListener(slot[0], sensor, android.hardware.SensorManager.SENSOR_DELAY_UI);
        }
        @JavascriptInterface public void headingStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_ROTATION_VECTOR, slot, e -> {
                    float[] R = new float[9]; android.hardware.SensorManager.getRotationMatrixFromVector(R, e.values);
                    float[] o = new float[3]; android.hardware.SensorManager.getOrientation(R, o);
                    double az = Math.toDegrees(o[0]); if (az < 0) az += 360; return "qormOnHeading(" + az + ")";
                }); qormCompassL = slot[0]; });
        }
        @JavascriptInterface public void headingStop(String a) { runOnUiThread(() -> { if (qormCompassL != null) qormSM().unregisterListener(qormCompassL); }); }
        @JavascriptInterface public void proximityStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_PROXIMITY, slot, e -> "qormOnProximity(" + (e.values[0] < 5 ? "true" : "false") + ")"); qormProxL = slot[0]; });
        }
        @JavascriptInterface public void proximityStop(String a) { runOnUiThread(() -> { if (qormProxL != null) qormSM().unregisterListener(qormProxL); }); }
        @JavascriptInterface public void pedometerStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_STEP_COUNTER, slot, e -> "qormOnSteps(" + (int) e.values[0] + ")"); qormStepL = slot[0]; });
        }
        @JavascriptInterface public void pedometerStop(String a) { runOnUiThread(() -> { if (qormStepL != null) qormSM().unregisterListener(qormStepL); }); }
        @JavascriptInterface public void barometerStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_PRESSURE, slot, e -> "qormOnPressure(" + (e.values[0] / 10.0) + ")"); qormPressL = slot[0]; });
        }
        @JavascriptInterface public void barometerStop(String a) { runOnUiThread(() -> { if (qormPressL != null) qormSM().unregisterListener(qormPressL); }); }
        @JavascriptInterface public void getModes(String a) {
            runOnUiThread(() -> { try {
                boolean airplane = android.provider.Settings.Global.getInt(getContentResolver(), android.provider.Settings.Global.AIRPLANE_MODE_ON, 0) != 0;
                android.os.PowerManager pm = (android.os.PowerManager) getSystemService(Context.POWER_SERVICE);
                boolean low = pm != null && pm.isPowerSaveMode();
                boolean dark = (getResources().getConfiguration().uiMode & android.content.res.Configuration.UI_MODE_NIGHT_MASK) == android.content.res.Configuration.UI_MODE_NIGHT_YES;
                android.app.NotificationManager nm = (android.app.NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
                boolean dnd = nm != null && nm.getCurrentInterruptionFilter() != android.app.NotificationManager.INTERRUPTION_FILTER_ALL;
                String json = "{"lowPower":" + low + ","darkMode":" + dark + ","airplane":" + airplane + ","dnd":" + dnd + "}";
                js("qormOnModes(" + org.json.JSONObject.quote(json) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pendingShortcut(String a) { runOnUiThread(() -> { if (qormPendingShortcut != null) { String s = qormPendingShortcut; qormPendingShortcut = null; qormFireShortcut(s); } }); }
        @JavascriptInterface public void updateWidget(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                android.content.SharedPreferences.Editor e = getSharedPreferences("qorm_widget", Context.MODE_PRIVATE).edit();
                e.putString("widget_title", o.optString("title", ""));
                e.putString("widget_lines", o.optJSONArray("lines") != null ? o.optJSONArray("lines").toString() : "[]");
                e.apply();
                android.content.Intent i = new android.content.Intent(this, QormWidget.class);
                i.setAction(android.appwidget.AppWidgetManager.ACTION_APPWIDGET_UPDATE);
                int[] ids = android.appwidget.AppWidgetManager.getInstance(this).getAppWidgetIds(new android.content.ComponentName(this, QormWidget.class));
                i.putExtra(android.appwidget.AppWidgetManager.EXTRA_APPWIDGET_IDS, ids);
                sendBroadcast(i);
                js("qormOnWidget('updated')");
            } catch (Exception ex) {} });
        }
        @JavascriptInterface public void getInsets(String a) {
            runOnUiThread(() -> {
                int t=0,b=0,l=0,r=0;
                android.view.WindowInsets wi = wv.getRootWindowInsets();
                if (wi != null) {
                    float dn = getResources().getDisplayMetrics().density;
                    if (android.os.Build.VERSION.SDK_INT >= 30) {
                        android.graphics.Insets si = wi.getInsets(android.view.WindowInsets.Type.systemBars() | android.view.WindowInsets.Type.displayCutout());
                        t=(int)(si.top/dn); b=(int)(si.bottom/dn); l=(int)(si.left/dn); r=(int)(si.right/dn);
                    } else {
                        t=(int)(wi.getSystemWindowInsetTop()/dn); b=(int)(wi.getSystemWindowInsetBottom()/dn); l=(int)(wi.getSystemWindowInsetLeft()/dn); r=(int)(wi.getSystemWindowInsetRight()/dn);
                    }
                }
                js("qormOnInsets('{"top":"+t+","bottom":"+b+","left":"+l+","right":"+r+"}')");
            });
        }
        //__QORM_USER_ANDROID__
        @JavascriptInterface public void platform(String a) { runOnUiThread(() -> js("qormOnPlatform('android')")); }
        @JavascriptInterface public void volumeGet(String a) { runOnUiThread(() -> volGet()); }
        @JavascriptInterface public void brightnessGet(String a) { runOnUiThread(() -> js("qormOnBrightness(" + curBright() + ")")); }
        @JavascriptInterface public void torchGet(String a) { runOnUiThread(() -> js("qormOnTorch(" + torchOn + ")")); }
        @JavascriptInterface public void bluetoothState(String a) { runOnUiThread(() -> {
            BluetoothManager bm = (BluetoothManager) getSystemService(Context.BLUETOOTH_SERVICE);
            boolean on = bm.getAdapter() != null && bm.getAdapter().isEnabled();
            js("qormOnBluetoothState(" + on + ")");
        }); }
        @JavascriptInterface public void volumeUp(String a) { runOnUiThread(() -> vol(1)); }
        @JavascriptInterface public void volumeDown(String a) { runOnUiThread(() -> vol(-1)); }
        @JavascriptInterface public void brightnessUp(String a) { runOnUiThread(() -> bright(0.1f)); }
        @JavascriptInterface public void brightnessDown(String a) { runOnUiThread(() -> bright(-0.1f)); }
        @JavascriptInterface public void vibrate(String a) { runOnUiThread(() -> doVibrate()); }
        @JavascriptInterface public void torchToggle(String a) { runOnUiThread(() -> torch()); }
        @JavascriptInterface public void battery(String a) { runOnUiThread(() -> batteryImpl()); }
    }

    java.util.HashMap<String, Object[]> btMap = new java.util.HashMap<>();
    BluetoothLeScanner scanner;
    ScanCallback scanCb;

    void btScan() {
        try {
            BluetoothManager bm = (BluetoothManager) getSystemService(Context.BLUETOOTH_SERVICE);
            scanner = bm.getAdapter().getBluetoothLeScanner();
            btMap.clear();
            scanCb = new ScanCallback() {
                @Override public void onScanResult(int t, ScanResult r) {
                    String name = r.getDevice().getName(); if (name == null) name = "(unknown)";
                    btMap.put(r.getDevice().getAddress(), new Object[]{name, r.getRssi()});
                }
            };
            scanner.startScan(scanCb);
            wv.postDelayed(() -> { try { scanner.stopScan(scanCb); } catch (Exception e) {} reportBt(); }, 5000);
        } catch (Exception e) { js("qormOnBluetooth('[]')"); }
    }
    void reportBt() {
        StringBuilder sb = new StringBuilder("[");
        boolean first = true;
        for (Object[] v : btMap.values()) {
            if (!first) sb.append(","); first = false;
            sb.append("{\"name\":\"").append(((String) v[0]).replace("\"", "")).append("\",\"rssi\":").append(v[1]).append("}");
        }
        sb.append("]");
        js("qormOnBluetooth('" + sb.toString() + "')");
    }
    void wifiImpl() {
        try {
            WifiManager wm = (WifiManager) getApplicationContext().getSystemService(Context.WIFI_SERVICE);
            WifiInfo info = wm.getConnectionInfo();
            String ssid = info.getSSID().replace("\"", "");
            int n = 0;
            try { n = wm.getScanResults().size(); } catch (Exception e) {}
            js("qormOnWifi('{\"ssid\":\"" + ssid + "\",\"networks\":" + n + "}')");
        } catch (Exception e) { js("qormOnWifi('{\"error\":\"wifi unavailable\"}')"); }
    }
    NfcAdapter nfc;
    void nfcEnable() {
        nfc = NfcAdapter.getDefaultAdapter(this);
        if (nfc == null) { js("qormOnNfc('{\"error\":\"no NFC on this device\"}')"); return; }
        nfc.enableReaderMode(this, (Tag tag) -> {
            String id = bytesToHex(tag.getId());
            js("qormOnNfc('{\"id\":\"" + id + "\"}')");
            runOnUiThread(() -> nfc.disableReaderMode(this));
        }, NfcAdapter.FLAG_READER_NFC_A | NfcAdapter.FLAG_READER_NFC_B | NfcAdapter.FLAG_READER_NFC_F | NfcAdapter.FLAG_READER_NFC_V, null);
    }
    static String bytesToHex(byte[] b) {
        StringBuilder sb = new StringBuilder();
        for (byte x : b) sb.append(String.format("%02X", x));
        return sb.toString();
    }
    void vol(int dir) {
        AudioManager am = (AudioManager) getSystemService(Context.AUDIO_SERVICE);
        am.adjustStreamVolume(AudioManager.STREAM_MUSIC, dir > 0 ? AudioManager.ADJUST_RAISE : AudioManager.ADJUST_LOWER, 0);
        int cur = am.getStreamVolume(AudioManager.STREAM_MUSIC), max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC);
        js("qormOnVolume(" + ((float) cur / max) + ")");
    }
    void volGet() {
        AudioManager am = (AudioManager) getSystemService(Context.AUDIO_SERVICE);
        int cur = am.getStreamVolume(AudioManager.STREAM_MUSIC), max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC);
        js("qormOnVolume(" + ((float) cur / max) + ")");
    }
    float curBright() {
        try {
            int b = android.provider.Settings.System.getInt(getContentResolver(), android.provider.Settings.System.SCREEN_BRIGHTNESS);
            brightness = b / 255f;
        } catch (Exception e) {}
        return brightness;
    }
    float brightness = 0.5f;
    void bright(float d) {
        brightness = Math.max(0.05f, Math.min(1f, brightness + d));
        WindowManager.LayoutParams lp = getWindow().getAttributes();
        lp.screenBrightness = brightness;
        getWindow().setAttributes(lp);
        js("qormOnBrightness(" + brightness + ")");
    }
    void doVibrate() {
        Vibrator v = (Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
        try { v.vibrate(VibrationEffect.createOneShot(200, VibrationEffect.DEFAULT_AMPLITUDE)); } catch (Exception e) {}
    }
    boolean torchOn = false;
    void torch() {
        try {
            CameraManager cm = (CameraManager) getSystemService(Context.CAMERA_SERVICE);
            String id = cm.getCameraIdList()[0];
            torchOn = !torchOn;
            cm.setTorchMode(id, torchOn);
            js("qormOnTorch(" + torchOn + ")");
        } catch (Exception e) { js("qormOnTorch(false)"); }
    }
    void batteryImpl() {
        Intent i = registerReceiver(null, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));
        int lvl = i.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
        int scale = i.getIntExtra(BatteryManager.EXTRA_SCALE, 100);
        int st = i.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
        boolean charging = st == BatteryManager.BATTERY_STATUS_CHARGING || st == BatteryManager.BATTERY_STATUS_FULL;
        js("qormOnBattery(" + ((float) lvl / scale) + "," + charging + ")");
    }

    void doBio() {
        try {
            Executor ex = ContextCompat.getMainExecutor(this);
            BiometricPrompt bp = new BiometricPrompt(this, ex, new BiometricPrompt.AuthenticationCallback() {
                @Override public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) { js("qormOnBiometric(true,'authenticated')"); }
                @Override public void onAuthenticationError(int code, CharSequence msg) { js("qormOnBiometric(false,'" + msg + "')"); }
            });
            BiometricPrompt.PromptInfo info = new BiometricPrompt.PromptInfo.Builder()
                .setTitle("Authenticate").setNegativeButtonText("Cancel").build();
            bp.authenticate(info);
        } catch (Exception e) { js("qormOnBiometric(false,'" + e + "')"); }
    }

    void startRec() {
        try {
            recPath = getCacheDir().getAbsolutePath() + "/qorm-rec.m4a";
            mrec = new MediaRecorder();
            mrec.setAudioSource(MediaRecorder.AudioSource.MIC);
            mrec.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);
            mrec.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);
            mrec.setOutputFile(recPath);
            mrec.prepare();
            mrec.start();
        } catch (Exception e) { js("qormOnAudioError('rec: " + e + "')"); }
    }

    void stopRec() {
        try {
            mrec.stop(); mrec.release(); mrec = null;
            java.io.File f = new java.io.File(recPath);
            byte[] bytes = new byte[(int) f.length()];
            java.io.FileInputStream in = new java.io.FileInputStream(f);
            in.read(bytes); in.close();
            String b64 = Base64.encodeToString(bytes, Base64.NO_WRAP);
            js("qormOnAudio('data:audio/mp4;base64," + b64 + "')");
        } catch (Exception e) { js("qormOnAudioError('stop: " + e + "')"); }
    }

    void getLoc() {
        try {
            Location l = lm.getLastKnownLocation(LocationManager.GPS_PROVIDER);
            if (l == null) l = lm.getLastKnownLocation(LocationManager.NETWORK_PROVIDER);
            if (l != null) js("qormOnLocation(" + l.getLatitude() + "," + l.getLongitude() + "," + l.getAccuracy() + ")");
            else lm.requestSingleUpdate(LocationManager.NETWORK_PROVIDER, this, null);
        } catch (SecurityException e) { js("qormOnLocationError('permission needed')"); }
    }

    @Override public void onSensorChanged(SensorEvent e) {
        float[] R = new float[9]; float[] o = new float[3];
        SensorManager.getRotationMatrixFromVector(R, e.values);
        SensorManager.getOrientation(R, o);
        double d = 180.0 / Math.PI;
        js("qormOnMotion(" + (o[0]*d) + "," + (o[1]*d) + "," + (o[2]*d) + ")");
    }
    @Override public void onAccuracyChanged(Sensor s, int a) {}
    @Override public void onLocationChanged(Location l) { js("qormOnLocation(" + l.getLatitude() + "," + l.getLongitude() + "," + l.getAccuracy() + ")"); }
    @Override public void onProviderEnabled(String p) {}
    @Override public void onProviderDisabled(String p) {}
    @Override public void onStatusChanged(String p, int s, Bundle b) {}

    android.content.SharedPreferences qormSecurePrefs() throws Exception {
        androidx.security.crypto.MasterKey mk = new androidx.security.crypto.MasterKey.Builder(this).setKeyScheme(androidx.security.crypto.MasterKey.KeyScheme.AES256_GCM).build();
        return androidx.security.crypto.EncryptedSharedPreferences.create(this, "qorm_secure", mk,
            androidx.security.crypto.EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            androidx.security.crypto.EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM);
    }
    void qormFireShortcut(String id) { String p = org.json.JSONObject.quote(id); js("(function(){var f=function(){if(window.qormEmit){qormEmit('shortcut'," + p + ");}else{setTimeout(f,300);}};f();})()"); }
    @Override protected void onNewIntent(android.content.Intent intent) { super.onNewIntent(intent); setIntent(intent); String q = intent.getStringExtra("qorm_shortcut"); if (q != null) qormFireShortcut(q); }
    void js(String s) { runOnUiThread(() -> wv.evaluateJavascript(s, null)); }
    String jsStr(String s) { return "\"" + s.replace("\\","\\\\").replace("\"","\\\"").replace("\n","\\n") + "\""; }
}
`
}

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

func scaffoldMac(out, name, appName, srcDir string) error {
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

// spliceUser injects the app's native/<file> snippet at a marker in a generated
// bridge, so an app can register its OWN native ops (the same qormToNative /
// qormOn<X> contract) without forking the framework. Empty file → fallback.
func spliceUser(src, marker, appDir, file, fallback string) string {
	code := fallback
	if b, err := os.ReadFile(filepath.Join(appDir, "native", file)); err == nil {
		code = string(b) // the app's snippet replaces the fallback
	}
	return strings.Replace(src, marker, code, 1)
}

// injectUserGo copies the app's native/desktop.go into the cmd/qorm package as
// userops_gen.go so `go build` compiles the user's Go middle-layer INTO the one
// binary; the returned func removes it afterward. No file → no-op.
func injectUserGo(appDir, pkg string) func() {
	src := filepath.Join(appDir, "native", "desktop.go")
	data, err := os.ReadFile(src)
	if err != nil {
		return func() {}
	}
	out, err := exec.Command("go", "list", "-e", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: can't locate cmd/qorm to compile native/desktop.go: %v\n", err)
		return func() {}
	}
	// strip any //go:build / // +build lines (the app keeps them so go build ./...
	// skips the file; injected into cmd/qorm it must compile unconditionally).
	var kept []string
	for _, ln := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "//go:build") || strings.HasPrefix(t, "// +build") {
			continue
		}
		kept = append(kept, ln)
	}
	dst := filepath.Join(strings.TrimSpace(string(out)), "userops_gen.go")
	if err := os.WriteFile(dst, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		// Don't fail silently: a go-installed qorm points at the read-only module
		// cache, so the write fails and the app's native ops would be dropped
		// with no warning. Tell the user how to get them included.
		fmt.Fprintf(os.Stderr, "warn: could NOT compile native/desktop.go into this package (%v)\n"+
			"      your custom native ops will be MISSING. Build qorm from a writable\n"+
			"      source checkout (git clone + go build ./cmd/qorm) instead of `go install`.\n", err)
		return func() {}
	}
	fmt.Fprintf(os.Stderr, "compiling your Go middle-layer (native/desktop.go) into the binary…\n")
	return func() { os.Remove(dst) }
}

// writeAppBundle compiles the app source into one signed bundle.json in destDir
// (the same artifact web/mobile ships) and copies native/ alongside so the
// desktop server can still inject native/web.js. Keeps all platforms consistent:
// one bundle drives the app everywhere.
func writeAppBundle(srcDir, destDir string) error {
	b, err := bundle.Build(srcDir)
	if err != nil {
		return err
	}
	bj, err := bundle.Marshal(b)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, "bundle.json"), bj, 0o644); err != nil {
		return err
	}
	if nd := filepath.Join(srcDir, "native"); dirExists(nd) {
		return copyTree(nd, filepath.Join(destDir, "native"))
	}
	return nil
}

func dirExists(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
