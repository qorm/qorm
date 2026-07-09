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
	"strconv"
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
	{Stem: "volume", Widget: "volume", Ops: []string{"volumeGet", "volumeSet", "volumeUp", "volumeDown"}, Callback: "qormOnVolume", Platforms: []string{IOS, Android, Mac, Linux, Windows}, Desc: "System output volume.", Notes: "Android volumeSet pending (get/up/down only)"},
	{Stem: "brightness", Widget: "brightness", Ops: []string{"brightnessGet", "brightnessSet", "brightnessUp", "brightnessDown"}, Callback: "qormOnBrightness", Platforms: []string{IOS, Android, Mac, Linux}, Desc: "Screen brightness.", Notes: "Linux needs brightnessctl + a backlight device; Android brightnessSet pending; Windows pending"},
	{Stem: "vibrate", Widget: "vibrate", Ops: []string{"vibrate"}, Callback: "", Platforms: []string{IOS, Android, Web}, Desc: "Basic vibration."},
	{Stem: "torch", Widget: "torch", Ops: []string{"torchGet", "torchToggle"}, Callback: "qormOnTorch", Platforms: []string{IOS, Android}, Desc: "Flashlight / torch."},
	{Stem: "battery", Widget: "battery", Ops: []string{"battery"}, Callback: "qormOnBattery", Platforms: []string{IOS, Android, Mac, Linux, Web}, Desc: "Battery level + charging state."},
	{Stem: "notify", Widget: "notify", Ops: []string{"notify"}, Callback: "qormOnNotify", Platforms: []string{IOS, Mac, Linux, Windows, Web}, Desc: "Local notification.", Notes: "Android falls back to the Web Notification API; Windows shows a WinRT toast (balloon fallback)"},
	{Stem: "badge", Widget: "dockbadge", Ops: []string{"badge"}, Callback: "", Platforms: []string{IOS, Mac}, Desc: "App icon / Dock badge count."},
	{Stem: "screenshot", Widget: "screenshot", Ops: []string{"screenshot"}, Callback: "qormOnScreenshot", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Capture the screen / app view.", Notes: "Linux needs grim, scrot or ImageMagick"},
	{Stem: "screenrecord", Widget: "screenrecord", Ops: []string{"screenRecordStart", "screenRecordStop"}, Callback: "qormOnScreenRecord", Platforms: []string{IOS, Mac, Web}, Desc: "Record the screen.", Notes: "Android needs MediaProjection (pending)"},
	{Stem: "share", Widget: "share", Ops: []string{"share"}, Callback: "qormOnShare", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Open the system share sheet.", Notes: "Linux/Windows fall back to copying the text to the clipboard"},
	{Stem: "clipboard", Widget: "clipboard", Ops: []string{"clipboardSet", "clipboardGet"}, Callback: "qormOnClipboard", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Read/write the clipboard."},
	{Stem: "deviceinfo", Widget: "deviceinfo", Ops: []string{"deviceInfo"}, Callback: "qormOnDeviceInfo", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Device model / OS / name."},
	{Stem: "network", Widget: "network", Ops: []string{"networkStatus"}, Callback: "qormOnNetwork", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Online state + connection type."},
	{Stem: "keepawake", Widget: "keepawake", Ops: []string{"keepAwake"}, Callback: "", Platforms: []string{IOS, Android, Mac, Linux, Web}, Desc: "Prevent the screen from sleeping."},
	{Stem: "haptics", Widget: "haptics", Ops: []string{"haptic"}, Callback: "", Platforms: []string{IOS, Android, Web}, Desc: "Fine haptic feedback (success/warning/error/selection)."},
	{Stem: "storage", Widget: "storage", Ops: []string{"storageSet", "storageGet"}, Callback: "qormOnStorage", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Key/value local storage."},
	{Stem: "loginitem", Widget: "loginitem", Ops: []string{"loginItem", "loginItemGet"}, Callback: "qormOnLoginItem", Platforms: []string{Mac}, Desc: "Launch at login.", Notes: "macOS-only (needs the installed .app)"},
	{Stem: "stt", Widget: "stt", Ops: []string{"listenStart", "listenStop"}, Callback: "qormOnSpeech", Platforms: []string{IOS, Android, Web}, Desc: "Speech to text (voice input / dictation)."},
	{Stem: "securestorage", Widget: "securestorage", Ops: []string{"secureSet", "secureGet"}, Callback: "qormOnSecure", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Secure key/value storage (iOS Keychain / Android Keystore / Windows DPAPI).", Notes: "web falls back to localStorage (not hardware-encrypted); Linux uses the DBus Secret Service (GNOME Keyring / KWallet)"},
	{Stem: "filepicker", Widget: "filepicker", Ops: []string{"pickFile"}, Callback: "qormOnFile", Platforms: []string{IOS, Android, Web}, Desc: "Pick a file from storage (returns name/size/data URL)."},
	{Stem: "photopicker", Widget: "photopicker", Ops: []string{"pickPhoto"}, Callback: "qormOnPhoto", Platforms: []string{IOS, Android, Web}, Desc: "Pick an existing photo from the library (returns a data URL)."},
	{Stem: "orientation", Widget: "orientation", Ops: []string{"lockOrientation"}, Callback: "", Platforms: []string{Android, Web}, Desc: "Lock screen orientation (portrait/landscape).", Notes: "iOS orientation lock needs AppDelegate support (pending)"},
	{Stem: "videocapture", Widget: "videocapture", Ops: []string{"recordVideo"}, Callback: "qormOnVideo", Platforms: []string{IOS, Web}, Desc: "Record a video with the camera.", Notes: "Android via MediaRecorder pending"},
	{Stem: "qrscan", Widget: "qrscan", Ops: []string{"scanQR"}, Callback: "qormOnScan", Platforms: []string{IOS, Web}, Desc: "Scan a QR code / barcode with the camera.", Notes: "Android needs CameraX+MLKit (pending)"},
	{Stem: "tts", Widget: "tts", Ops: []string{"speak", "speakStop"}, Callback: "", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Text to speech (speak a string aloud)."},
	{Stem: "compass", Widget: "compass", Ops: []string{"headingStart", "headingStop"}, Callback: "qormOnHeading", Platforms: []string{IOS, Android, Web}, Desc: "Compass heading (degrees from magnetic north)."},
	{Stem: "proximity", Widget: "proximity", Ops: []string{"proximityStart", "proximityStop"}, Callback: "qormOnProximity", Platforms: []string{IOS, Android}, Desc: "Proximity sensor (near/far)."},
	{Stem: "pedometer", Widget: "pedometer", Ops: []string{"pedometerStart", "pedometerStop"}, Callback: "qormOnSteps", Platforms: []string{IOS, Android}, Desc: "Step counter / pedometer."},
	{Stem: "barometer", Widget: "barometer", Ops: []string{"barometerStart", "barometerStop"}, Callback: "qormOnPressure", Platforms: []string{IOS, Android}, Desc: "Barometric pressure / relative altitude."},
	{Stem: "contacts", Widget: "contacts", Ops: []string{"pickContact"}, Callback: "qormOnContact", Platforms: []string{IOS, Android, Web}, Desc: "Pick a contact (name + phone)."},
	{Stem: "calendar", Widget: "calendar", Ops: []string{"addEvent"}, Callback: "qormOnCalendar", Platforms: []string{IOS, Android}, Desc: "Add a calendar event."},
	{Stem: "systemmodes", Widget: "systemmodes", Ops: []string{"getModes"}, Callback: "qormOnModes", Platforms: []string{IOS, Android, Mac, Web}, Desc: "Read system modes: low-power, dark/appearance, airplane (Android), do-not-disturb (Android). Null where a platform has no public API."},
	{Stem: "insets", Widget: "insets", Ops: []string{"getInsets"}, Callback: "qormOnInsets", Platforms: []string{IOS, Android, Web}, Desc: "Safe-area insets in points/dp (status bar, notch, home indicator, nav bar)."},
	{Stem: "openurl", Widget: "openurl", Ops: []string{"openURL"}, Callback: "qormOnOpenUrl", Platforms: []string{IOS, Android, Mac, Linux, Windows, Web}, Desc: "Open a URL / deep link (http, mailto, tel, sms, maps)."},
	{Stem: "screens", Widget: "screens", Ops: []string{"screens"}, Callback: "qormOnScreens", Platforms: []string{Mac}, Desc: "Enumerate displays.", Notes: "Linux/Windows enumeration pending (returns an empty list)"},
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

// Markdown renders the registry as a human-readable capability reference table.
// The docs are generated from this ONE source (see TestCapabilityDocInSync), so
// a human reads exactly what the code implements — no drift.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# Capabilities\n\n")
	b.WriteString("> Auto-generated from the capability registry — do not edit by hand.\n\n")
	b.WriteString("QORM has " + strconv.Itoa(len(All)) + " built-in hardware/native capabilities. For each: the widget type is the capability name, `qormToNative(op)` triggers it, and the result comes back via `qormOn<X>`. Agents discover them all with the `qorm_capabilities` MCP tool.\n\n")
	b.WriteString("| Capability | Widget | Ops | Callback | Platforms | Description |\n")
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

	// Per-platform view: each target's full hardware-interface list.
	b.WriteString("\n## Hardware interfaces by platform\n\n")
	b.WriteString("Every capability each target implements natively or via a Web API.\n\n")
	for _, p := range []struct{ key, label string }{
		{IOS, "iOS"}, {Android, "Android"}, {Mac, "macOS"}, {Linux, "Linux"}, {Windows, "Windows"}, {Web, "Web"},
	} {
		var caps []string
		for _, c := range All {
			for _, cp := range c.Platforms {
				if cp == p.key {
					caps = append(caps, "`"+c.Stem+"`")
					break
				}
			}
		}
		b.WriteString("- **" + p.label + "** (" + strconv.Itoa(len(caps)) + ") — " + strings.Join(caps, ", ") + "\n")
	}
	return b.String()
}

var descZH = map[string]string{
	"camera":        "拍照（通过 getUserMedia 实时捕获或文件采集）；直接绑定到状态。",
	"location":      "当前 GPS 定位。",
	"recorder":      "录制麦克风音频。",
	"sensors":       "设备运动传感器（加速度计/陀螺仪）。",
	"biometric":     "Face ID / 指纹身份验证。",
	"bluetooth":     "扫描 BLE 设备 + 适配器状态。",
	"wifi":          "当前 Wi-Fi 网络信息。",
	"nfc":           "读取 NFC/NDEF 标签。",
	"volume":        "系统输出音量。",
	"brightness":    "屏幕亮度。",
	"vibrate":       "基本振动。",
	"torch":         "手电筒 / 闪光灯。",
	"battery":       "电量水平 + 充电状态。",
	"notify":        "本地通知。",
	"badge":         "应用图标 / Dock 徽标计数。",
	"screenshot":    "捕获屏幕 / 应用视图。",
	"screenrecord":  "录制屏幕。",
	"share":         "打开系统分享面板。",
	"clipboard":     "读/写剪贴板。",
	"deviceinfo":    "设备型号 / 操作系统 / 名称。",
	"network":       "在线状态 + 连接类型。",
	"keepawake":     "防止屏幕休眠。",
	"haptics":       "精细触觉反馈（成功/警告/错误/选择）。",
	"storage":       "键值对本地存储。",
	"loginitem":     "开机自启。",
	"stt":           "语音转文字（语音输入 / 听写）。",
	"securestorage": "安全键值存储（iOS Keychain / Android Keystore）。",
	"filepicker":    "从存储中选择文件（返回名称、大小、数据 URL）。",
	"photopicker":   "从相册选择已有照片（返回数据 URL）。",
	"orientation":   "锁定屏幕方向（竖屏/横屏）。",
	"videocapture":  "用摄像头录制视频。",
	"qrscan":        "用摄像头扫描二维码 / 条形码。",
	"tts":           "文字转语音（大声朗读字符串）。",
	"compass":       "罗盘朝向（相对于磁北的角度）。",
	"proximity":     "距离传感器（近/远）。",
	"pedometer":     "计步器 / 步数计数。",
	"barometer":     "气压 / 相对高度。",
	"contacts":      "选择联系人（姓名 + 电话）。",
	"calendar":      "添加日历事件。",
	"systemmodes":   "读取系统模式：低电量、深色/外观样式、飞行模式 (Android)、免打扰 (Android)。在平台没有公开 API 时返回空值。",
	"insets":        "安全区域内边距（以 point/dp 为单位，含状态栏、刘海屏、Home 指示条、导航栏）。",
	"openurl":       "打开 URL / 深度链接（http, mailto, tel, sms, maps）。",
	"screens":       "枚举显示器。",
}

var notesZH = map[string]string{
	"biometric":     "桌面端目前仅支持 macOS Touch ID",
	"wifi":          "iOS 限制仅能获取当前连接网络的信息",
	"nfc":           "iOS 需要付费的 Apple Developer 团队账号",
	"volume":        "Android 的 volumeSet 待完成（仅支持读取/加减）",
	"brightness":    "Linux 需要 brightnessctl 与背光设备；Android brightnessSet 待完成；Windows 待完成",
	"notify":        "Android 会回退到 Web Notification API；Windows 为 WinRT Toast 通知（气泡回退）",
	"screenshot":    "Linux 需要 grim、scrot 或 ImageMagick",
	"screenrecord":  "Android 需要 MediaProjection 权限（待完成）",
	"share":         "Linux/Windows 回退为复制文本到剪贴板",
	"loginitem":     "仅限 macOS（需要安装后的 .app 包）",
	"screens":       "Linux/Windows 枚举待完成（返回空列表）",
	"securestorage": "Web 端会回退到 localStorage（无硬件加密）；Linux 走 DBus Secret Service（GNOME Keyring / KWallet）",
	"orientation":   "iOS 屏幕方向锁定需要 AppDelegate 支持（待完成）",
	"videocapture":  "Android 端的 MediaRecorder 适配待完成",
	"qrscan":        "Android 端需要 CameraX+MLKit 支持（待完成）",
}

// MarkdownZH renders the registry as a Chinese human-readable capability reference table.
func MarkdownZH() string {
	var b strings.Builder
	b.WriteString("# 能力清单\n\n")
	b.WriteString("> 由能力注册表自动生成 —— 请勿手动修改。\n\n")
	b.WriteString("QORM 内置了 " + strconv.Itoa(len(All)) + " 项硬件/原生能力。对应地：组件类型即能力名称，由 `qormToNative(op)` 触发，结果通过 `qormOn<X>` 回传。智能体可通过 `qorm_capabilities` MCP 工具发现所有能力。\n\n")
	b.WriteString("| 能力 | 组件 | 操作 | 回调 | 平台 | 描述 |\n")
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
		desc := descZH[c.Stem]
		if desc == "" {
			desc = c.Desc // fallback
		}
		note := notesZH[c.Stem]
		if note == "" && c.Notes != "" {
			note = c.Notes
		}
		if note != "" {
			desc += " (" + note + ")"
		}
		b.WriteString("| `" + c.Stem + "` | `" + c.Widget + "` | " + ops + " | `" + cb + "` | " + strings.Join(c.Platforms, ", ") + " | " + desc + " |\n")
	}

	// Per-platform view: each target's full hardware-interface list.
	b.WriteString("\n## 各平台硬件接口支持\n\n")
	b.WriteString("每个运行目标原生支持或通过 Web API 实现的全部能力清单。\n\n")
	for _, p := range []struct{ key, label string }{
		{IOS, "iOS"}, {Android, "Android"}, {Mac, "macOS"}, {Linux, "Linux"}, {Windows, "Windows"}, {Web, "Web"},
	} {
		var caps []string
		for _, c := range All {
			for _, cp := range c.Platforms {
				if cp == p.key {
					caps = append(caps, "`"+c.Stem+"`")
					break
				}
			}
		}
		b.WriteString("- **" + p.label + "** (" + strconv.Itoa(len(caps)) + ") — " + strings.Join(caps, ", ") + "\n")
	}
	return b.String()
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
