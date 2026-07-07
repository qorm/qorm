package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
)

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
