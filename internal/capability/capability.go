// Package capability is the single source of truth for QORM's built-in hardware
// / native capabilities. One entry per capability defines its canonical stem and
// every derived name (widget type, qormToNative ops, qormOn<Stem> callback) plus
// which platforms implement it. This one list drives: the package-time platform
// check (cmd/qorm/platform.go), agent discovery (the qorm_capabilities MCP tool),
// and the docs — so a capability is defined once and stays consistent across
// every layer, and both a human and an AI can discover what exists and how to
// call it without reading source.
//
// Naming contract (unambiguous, mechanically derivable):
//
//	widget type = Stem                     e.g. "volume"
//	trigger JS  = qorm<Stem>               qormVolume(...)
//	read op     = <stem>Get                volumeGet
//	write op    = <stem>Set                volumeSet
//	nudge ops   = <stem>Up / <stem>Down    volumeUp / volumeDown
//	lifecycle   = <stem>Start / <stem>Stop recordStart / recordStop
//	callback    = qormOn<Stem>             qormOnVolume
package capability

import (
	"sort"
	"strings"
)

// Platform keys used across the framework.
const (
	IOS     = "ios"
	Android = "android"
	Mac     = "mac"
	Linux   = "linux"
	Windows = "windows"
	Web     = "web"
)

// Cap describes one built-in capability and all its derived names.
type Cap struct {
	Stem      string   // canonical name; the widget type is this
	Widget    string   // widget type string (usually == Stem)
	Ops       []string // qormToNative op strings this capability accepts
	Callback  string   // qormOn<Stem> JS callback ("" if fire-and-forget)
	Platforms []string // platforms that implement it natively or via Web API
	Desc      string
	Notes     string // caveat (paid tier, partial impl, etc.)
}

// All is the registry — the single source of truth.
var All = []Cap{
	{Stem: "camera", Widget: "camera", Ops: nil, Callback: "", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Capture a photo (getUserMedia live path / file capture); binds to state directly."},
	{Stem: "location", Widget: "location", Ops: []string{"location"}, Callback: "qormOnLocation", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Current GPS location."},
	{Stem: "recorder", Widget: "recorder", Ops: []string{"recordStart", "recordStop"}, Callback: "qormOnAudio", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Record microphone audio."},
	{Stem: "sensors", Widget: "sensors", Ops: []string{"motionStart", "motionStop"}, Callback: "qormOnMotion", Platforms: []string{IOS, Android}, Desc: "Device motion sensors (accelerometer/gyroscope)."},
	{Stem: "biometric", Widget: "biometric", Ops: []string{"biometric"}, Callback: "qormOnBiometric", Platforms: []string{IOS, Android, Mac}, Desc: "Face ID / fingerprint authentication.", Notes: "desktop is macOS Touch ID only"},
	{Stem: "bluetooth", Widget: "bluetooth", Ops: []string{"bluetoothScan", "bluetoothState"}, Callback: "qormOnBluetooth", Platforms: []string{IOS, Android, Mac}, Desc: "Scan for BLE devices + adapter state."},
	{Stem: "wifi", Widget: "wifi", Ops: []string{"wifiInfo"}, Callback: "qormOnWifi", Platforms: []string{IOS, Android, Mac}, Desc: "Current Wi-Fi network info.", Notes: "iOS restricts Wi-Fi info to the current network"},
	{Stem: "nfc", Widget: "nfc", Ops: []string{"nfcRead"}, Callback: "qormOnNfc", Platforms: []string{IOS, Android}, Desc: "Read an NFC/NDEF tag.", Notes: "iOS requires a paid Apple Developer team"},
	{Stem: "volume", Widget: "volume", Ops: []string{"volumeGet", "volumeSet", "volumeUp", "volumeDown"}, Callback: "qormOnVolume", Platforms: []string{IOS, Android, Mac, Linux}, Desc: "System output volume."},
	{Stem: "brightness", Widget: "brightness", Ops: []string{"brightnessGet", "brightnessSet", "brightnessUp", "brightnessDown"}, Callback: "qormOnBrightness", Platforms: []string{IOS, Android, Mac}, Desc: "Screen brightness.", Notes: "desktop is macOS-only for now"},
	{Stem: "vibrate", Widget: "vibrate", Ops: []string{"vibrate"}, Callback: "", Platforms: []string{IOS, Android, Web}, Desc: "Basic vibration."},
	{Stem: "torch", Widget: "torch", Ops: []string{"torchGet", "torchToggle"}, Callback: "qormOnTorch", Platforms: []string{IOS, Android}, Desc: "Flashlight / torch."},
	{Stem: "battery", Widget: "battery", Ops: []string{"battery"}, Callback: "qormOnBattery", Platforms: []string{IOS, Android, Mac, Linux, Web}, Desc: "Battery level + charging state."},
	{Stem: "notify", Widget: "notify", Ops: []string{"notify"}, Callback: "qormOnNotify", Platforms: []string{IOS, Mac, Linux, Web}, Desc: "Local notification.", Notes: "Android falls back to the Web Notification API"},
	{Stem: "badge", Widget: "dockbadge", Ops: []string{"badge"}, Callback: "", Platforms: []string{IOS, Mac}, Desc: "App icon / Dock badge count."},
	{Stem: "screenshot", Widget: "screenshot", Ops: []string{"screenshot"}, Callback: "qormOnScreenshot", Platforms: []string{IOS, Android, Mac, Web}, Desc: "Capture the screen / app view."},
	{Stem: "screenrecord", Widget: "screenrecord", Ops: []string{"screenRecordStart", "screenRecordStop"}, Callback: "qormOnScreenRecord", Platforms: []string{IOS, Mac, Web}, Desc: "Record the screen.", Notes: "Android needs MediaProjection (pending)"},
	{Stem: "share", Widget: "share", Ops: []string{"share"}, Callback: "qormOnShare", Platforms: []string{IOS, Android, Mac, Web}, Desc: "Open the system share sheet."},
	{Stem: "clipboard", Widget: "clipboard", Ops: []string{"clipboardSet", "clipboardGet"}, Callback: "qormOnClipboard", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Read/write the clipboard."},
	{Stem: "deviceinfo", Widget: "deviceinfo", Ops: []string{"deviceInfo"}, Callback: "qormOnDeviceInfo", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Device model / OS / name."},
	{Stem: "network", Widget: "network", Ops: []string{"networkStatus"}, Callback: "qormOnNetwork", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Online state + connection type."},
	{Stem: "keepawake", Widget: "keepawake", Ops: []string{"keepAwake"}, Callback: "", Platforms: []string{IOS, Android, Mac, Linux, Web}, Desc: "Prevent the screen from sleeping."},
	{Stem: "haptics", Widget: "haptics", Ops: []string{"haptic"}, Callback: "", Platforms: []string{IOS, Android, Web}, Desc: "Fine haptic feedback (success/warning/error/selection)."},
	{Stem: "storage", Widget: "storage", Ops: []string{"storageSet", "storageGet"}, Callback: "qormOnStorage", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Key/value local storage."},
	{Stem: "loginitem", Widget: "loginitem", Ops: []string{"loginItem", "loginItemGet"}, Callback: "qormOnLoginItem", Platforms: []string{Mac}, Desc: "Launch at login.", Notes: "macOS-only (needs the installed .app)"},
	{Stem: "stt", Widget: "stt", Ops: []string{"listenStart", "listenStop"}, Callback: "qormOnSpeech", Platforms: []string{IOS, Android, Web}, Desc: "Speech to text (voice input / dictation)."},
	{Stem: "securestorage", Widget: "securestorage", Ops: []string{"secureSet", "secureGet"}, Callback: "qormOnSecure", Platforms: []string{IOS, Android, Mac, Web}, Desc: "Secure key/value storage (iOS Keychain / Android Keystore).", Notes: "web falls back to localStorage (not hardware-encrypted)"},
	{Stem: "filepicker", Widget: "filepicker", Ops: []string{"pickFile"}, Callback: "qormOnFile", Platforms: []string{IOS, Android, Web}, Desc: "Pick a file from storage (returns name/size/data URL)."},
	{Stem: "photopicker", Widget: "photopicker", Ops: []string{"pickPhoto"}, Callback: "qormOnPhoto", Platforms: []string{IOS, Android, Web}, Desc: "Pick an existing photo from the library (returns a data URL)."},
	{Stem: "orientation", Widget: "orientation", Ops: []string{"lockOrientation"}, Callback: "", Platforms: []string{Android, Web}, Desc: "Lock screen orientation (portrait/landscape).", Notes: "iOS orientation lock needs AppDelegate support (pending)"},
	{Stem: "videocapture", Widget: "videocapture", Ops: []string{"recordVideo"}, Callback: "qormOnVideo", Platforms: []string{IOS, Web}, Desc: "Record a video with the camera.", Notes: "Android via MediaRecorder pending"},
	{Stem: "qrscan", Widget: "qrscan", Ops: []string{"scanQR"}, Callback: "qormOnScan", Platforms: []string{IOS, Web}, Desc: "Scan a QR code / barcode with the camera.", Notes: "Android needs CameraX+MLKit (pending)"},
	{Stem: "tts", Widget: "tts", Ops: []string{"speak", "speakStop"}, Callback: "", Platforms: []string{IOS, Android, Mac, Linux, Web}, Desc: "Text to speech (speak a string aloud)."},
	{Stem: "compass", Widget: "compass", Ops: []string{"headingStart", "headingStop"}, Callback: "qormOnHeading", Platforms: []string{IOS, Android, Web}, Desc: "Compass heading (degrees from magnetic north)."},
	{Stem: "proximity", Widget: "proximity", Ops: []string{"proximityStart", "proximityStop"}, Callback: "qormOnProximity", Platforms: []string{IOS, Android}, Desc: "Proximity sensor (near/far)."},
	{Stem: "pedometer", Widget: "pedometer", Ops: []string{"pedometerStart", "pedometerStop"}, Callback: "qormOnSteps", Platforms: []string{IOS, Android}, Desc: "Step counter / pedometer."},
	{Stem: "barometer", Widget: "barometer", Ops: []string{"barometerStart", "barometerStop"}, Callback: "qormOnPressure", Platforms: []string{IOS, Android}, Desc: "Barometric pressure / relative altitude."},
	{Stem: "contacts", Widget: "contacts", Ops: []string{"pickContact"}, Callback: "qormOnContact", Platforms: []string{IOS, Android, Web}, Desc: "Pick a contact (name + phone)."},
	{Stem: "calendar", Widget: "calendar", Ops: []string{"addEvent"}, Callback: "qormOnCalendar", Platforms: []string{IOS, Android}, Desc: "Add a calendar event."},
	{Stem: "systemmodes", Widget: "systemmodes", Ops: []string{"getModes"}, Callback: "qormOnModes", Platforms: []string{IOS, Android, Mac, Web}, Desc: "Read system modes: low-power, dark/appearance, airplane (Android), do-not-disturb (Android). Null where a platform has no public API."},
	{Stem: "insets", Widget: "insets", Ops: []string{"getInsets"}, Callback: "qormOnInsets", Platforms: []string{IOS, Android, Web}, Desc: "Safe-area insets in points/dp (status bar, notch, home indicator, nav bar)."},
	{Stem: "openurl", Widget: "openurl", Ops: []string{"openURL"}, Callback: "qormOnOpenUrl", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Open a URL / deep link (http, mailto, tel, sms, maps)."},
	{Stem: "screens", Widget: "screens", Ops: []string{"screens"}, Callback: "qormOnScreens", Platforms: []string{Mac, Linux, Windows}, Desc: "Enumerate displays."},
}

// byWidget indexes All by widget type for O(1) lookup.
var byWidget = func() map[string]*Cap {
	m := make(map[string]*Cap, len(All))
	for i := range All {
		m[All[i].Widget] = &All[i]
	}
	return m
}()

// ForWidget returns the capability for a widget type, or nil if the type is not
// a capability (a plain layout/content widget).
func ForWidget(widget string) *Cap { return byWidget[widget] }

// Supported reports whether a capability (by widget type) works on a platform.
func Supported(widget, platform string) bool {
	c := byWidget[widget]
	if c == nil {
		return false
	}
	for _, p := range c.Platforms {
		if p == platform {
			return true
		}
	}
	return false
}

// PlatformsFor returns the platform support set for a widget type.
func PlatformsFor(widget string) map[string]bool {
	c := byWidget[widget]
	if c == nil {
		return nil
	}
	m := make(map[string]bool, len(c.Platforms))
	for _, p := range c.Platforms {
		m[p] = true
	}
	return m
}

// Widgets returns the sorted list of capability widget types (for the render
// switch / validation).
func Widgets() []string {
	out := make([]string, 0, len(All))
	for _, c := range All {
		out = append(out, c.Widget)
	}
	sort.Strings(out)
	return out
}

// Markdown renders the registry as a human-readable capability reference table.
// The docs are generated from this ONE source (see TestCapabilityDocInSync), so
// a human reads exactly what the code implements — no drift.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# 能力清单 · Capabilities\n\n")
	b.WriteString("> 本文件由 `internal/capability` 注册表自动生成(`TestCapabilityDocInSync`),请勿手改。\n")
	b.WriteString("> Auto-generated from the capability registry — do not edit by hand.\n\n")
	b.WriteString("QORM 内置 " + itoa(len(All)) + " 个硬件/原生能力。每个能力:组件类型 = 能力名,触发 `qormToNative(op)`,结果回 `qormOn<X>`。AI 可用 `qorm_capabilities` MCP 工具发现全部。\n\n")
	b.WriteString("| 能力 Capability | Widget | Ops | 回调 Callback | 平台 Platforms | 说明 |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, c := range All {
		ops := strings.Join(c.Ops, "<br>")
		if ops == "" {
			ops = "—"
		}
		cb := c.Callback
		if cb == "" {
			cb = "—"
		}
		desc := c.Desc
		if c.Notes != "" {
			desc += " (" + c.Notes + ")"
		}
		b.WriteString("| `" + c.Stem + "` | `" + c.Widget + "` | " + ops + " | `" + cb + "` | " + strings.Join(c.Platforms, ", ") + " | " + desc + " |\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		return "-" + string(d)
	}
	return string(d)
}

// perms lists the permission identifiers each capability needs per platform, so
// a packaged app ships ONLY the permissions its capabilities actually use (the
// dev client, being app-agnostic, still bakes them all). iOS/Mac values are
// Info.plist keys; Android values are AndroidManifest <uses-permission> names.
// Capabilities not listed here need no runtime permission (or use a system
// picker that grants access out-of-process, e.g. PHPicker / CNContactPicker).
var perms = map[string]map[string][]string{
	"camera":       {IOS: {"NSCameraUsageDescription"}, Mac: {"NSCameraUsageDescription"}, Android: {"android.permission.CAMERA"}},
	"recorder":     {IOS: {"NSMicrophoneUsageDescription"}, Mac: {"NSMicrophoneUsageDescription"}, Android: {"android.permission.RECORD_AUDIO"}},
	"location":     {IOS: {"NSLocationWhenInUseUsageDescription"}, Mac: {"NSLocationWhenInUseUsageDescription"}, Android: {"android.permission.ACCESS_FINE_LOCATION", "android.permission.ACCESS_COARSE_LOCATION"}},
	"bluetooth":    {IOS: {"NSBluetoothAlwaysUsageDescription"}, Mac: {"NSBluetoothAlwaysUsageDescription", "NSBluetoothPeripheralUsageDescription"}, Android: {"android.permission.BLUETOOTH_CONNECT", "android.permission.BLUETOOTH_SCAN"}},
	"biometric":    {IOS: {"NSFaceIDUsageDescription"}},
	"notify":       {Android: {"android.permission.POST_NOTIFICATIONS"}},
	"sensors":      {IOS: {"NSMotionUsageDescription"}},
	"pedometer":    {IOS: {"NSMotionUsageDescription"}},
	"barometer":    {IOS: {"NSMotionUsageDescription"}},
	"compass":      {IOS: {"NSLocationWhenInUseUsageDescription"}},
	"calendar":     {IOS: {"NSCalendarsUsageDescription"}},
	"contacts":     {Android: {"android.permission.READ_CONTACTS"}},
	"photopicker":  {Android: {"android.permission.READ_MEDIA_IMAGES"}},
	"videocapture": {IOS: {"NSCameraUsageDescription", "NSMicrophoneUsageDescription"}, Android: {"android.permission.CAMERA", "android.permission.RECORD_AUDIO"}},
	"qrscan":       {IOS: {"NSCameraUsageDescription"}, Android: {"android.permission.CAMERA"}},
	"stt":          {IOS: {"NSMicrophoneUsageDescription", "NSSpeechRecognitionUsageDescription"}, Android: {"android.permission.RECORD_AUDIO"}},
	"haptics":      {Android: {"android.permission.VIBRATE"}},
	"vibrate":      {Android: {"android.permission.VIBRATE"}},
	"nfc":          {Android: {"android.permission.NFC"}},
	"wifi":         {Android: {"android.permission.ACCESS_WIFI_STATE"}},
}

// iosPermReason maps an Info.plist usage key to a human reason (filled with the
// app name by the packager). Keeps the generated plist honest and App-Store-safe.
var iosPermReason = map[string]string{
	"NSCameraUsageDescription":            "uses the camera",
	"NSMicrophoneUsageDescription":        "uses the microphone",
	"NSLocationWhenInUseUsageDescription": "uses your location",
	"NSBluetoothAlwaysUsageDescription":   "uses Bluetooth",
	"NSFaceIDUsageDescription":            "uses Face ID",
	"NSMotionUsageDescription":            "reads motion & fitness data",
	"NSCalendarsUsageDescription":         "adds calendar events",
	"NSSpeechRecognitionUsageDescription": "transcribes your speech",
}

// PermsFor returns the deduped, stably-ordered union of permission identifiers
// the given used widget types require on a platform — the exact set a packaged
// app should declare, derived from what it actually uses.
func PermsFor(widgets map[string]bool, platform string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range All { // registry order = stable output order
		if !widgets[c.Widget] {
			continue
		}
		for _, p := range perms[c.Stem][platform] {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// IOSPermReason returns the human usage reason for an Info.plist key.
func IOSPermReason(key string) string {
	if r, ok := iosPermReason[key]; ok {
		return r
	}
	return "needs this capability"
}
