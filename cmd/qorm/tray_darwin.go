//go:build darwin && desktop

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ServiceManagement -framework WebKit -framework LocalAuthentication -framework CoreWLAN -framework CoreBluetooth -framework CoreAudio -framework AudioToolbox -framework Security -framework IOKit
#include <stdlib.h>
void qormRunTray(const unsigned char* png, int pngLen, const char** items, int n, const char* tip);
void qormRunTrayJSON(const unsigned char* png, int pngLen, const char* menuJSON, const char* tip);
void qormSetDockIcon(const unsigned char* png, int len);
void qormSetAppMenu(const char* appName, const char* menuJSON);
void qormSetBadge(const char* label);
int qormSetLoginItem(int enabled);
int qormLoginItemEnabled(void);
void qormNotify(const char* title, const char* body, const char* ident);
void qormCenterWindow(void);
const char* qormScreenInfo(void);
const char* qormWindowFrame(void);
void qormSetWindowFrame(int x, int y, int w, int h);
void qormGrantMedia(void* window);
void qormDisableRestore(void);
void qormFixWindow(void);
void qormMoveWindow(int x, int y, int w, int h);
void qormBiometric(void);
const char* qormWifiInfo(void);
void qormBluetoothScan(void);
void qormBluetoothState(void);
double qormGetBrightness(void);
int qormSetBrightness(double v);
void qormShareText(const char* text);
void qormWatchVolume(void);
int qormReadMute(void);
const char* qormSystemModes(void);
void qormWatchBrightness(void);
void qormClipboardSet(const char* text);
const char* qormClipboardGet(void);
void qormOpenURL(const char* url);
const char* qormOSVersion(void);
void qormKeepAwake(int on);
void qormSpeak(const char* text);
void qormSpeakStop(void);
const char* qormScreenshot(void);
float qormGetSystemVolume(void);
int qormSetSystemVolume(float volume);
int qormSecureSet(const char* key, const char* val);
const char* qormSecureGet(const char* key);
*/
import "C"

import (
	"strconv"
	"unsafe"
)

var trayClick func(int)

//export qormTrayClicked
func qormTrayClicked(idx C.int) {
	if trayClick != nil {
		trayClick(int(idx))
	}
}

// nativeTray shows a menu-bar status item with the given icon (PNG) + menu
// items, invoking onClick(index) on selection. Runs the Cocoa loop (blocks).
func nativeTray(png []byte, items []string, tip string, onClick func(int)) {
	trayClick = onClick
	cItems := make([]*C.char, len(items))
	for i, s := range items {
		cItems[i] = C.CString(s)
	}
	ctip := C.CString(tip)
	var pngPtr *C.uchar
	if len(png) > 0 {
		pngPtr = (*C.uchar)(unsafe.Pointer(&png[0]))
	}
	C.qormRunTray(pngPtr, C.int(len(png)), (**C.char)(unsafe.Pointer(&cItems[0])), C.int(len(items)), ctip)
}

// nativeTrayJSON builds the tray from a JSON menu (icons + submenus) and runs the
// Cocoa loop; selections route through qormTraySelected -> traySelected.
func nativeTrayJSON(png []byte, menuJSON, tip string) {
	m := C.CString(menuJSON)
	defer C.free(unsafe.Pointer(m))
	t := C.CString(tip)
	defer C.free(unsafe.Pointer(t))
	var pngPtr *C.uchar
	if len(png) > 0 {
		pngPtr = (*C.uchar)(unsafe.Pointer(&png[0]))
	}
	C.qormRunTrayJSON(pngPtr, C.int(len(png)), m, t)
}

//export qormTraySelected
func qormTraySelected(cid *C.char) {
	traySelected(C.GoString(cid))
}

// setDockIcon sets the app's Dock icon (a raw binary otherwise shows the
// generic executable icon).
func setDockIcon(png []byte) {
	if len(png) == 0 {
		return
	}
	C.qormSetDockIcon((*C.uchar)(unsafe.Pointer(&png[0])), C.int(len(png)))
}

// setAppMenu installs a standard macOS app menu (App / Edit / Window) so the
// app has a proper menu bar and Cut/Copy/Paste work in WebView inputs.
func setAppMenu(appName, menuJSON string) {
	c := C.CString(appName)
	defer C.free(unsafe.Pointer(c))
	m := C.CString(menuJSON)
	defer C.free(unsafe.Pointer(m))
	C.qormSetAppMenu(c, m)
}

//export qormMenuClicked
func qormMenuClicked(id *C.char) {
	mid := C.GoString(id)
	nativeEval("main", "qormEmit('menu', {id:"+strconv.Quote(mid)+"})")
}

// setDockBadge sets (or clears, when empty) the Dock icon badge label.
func setDockBadge(label string) {
	c := C.CString(label)
	C.qormSetBadge(c)
}

// setLoginItem enables/disables launch-at-login (macOS 13+ SMAppService).
// Returns true on success (needs a proper .app bundle).
func setLoginItem(enabled bool) bool {
	e := C.int(0)
	if enabled {
		e = 1
	}
	return C.qormSetLoginItem(e) == 1
}

// loginItemEnabled reports whether launch-at-login is currently on.
func loginItemEnabled() bool { return C.qormLoginItemEnabled() == 1 }

//export goNotifyClicked
func goNotifyClicked(id *C.char) {
	if notifyClickHandler != nil {
		notifyClickHandler(C.GoString(id))
	}
}

// nativeNotify posts a notification whose click routes to notifyClickHandler.
func nativeNotify(title, body, ident string) {
	C.qormNotify(C.CString(title), C.CString(body), C.CString(ident))
}

// centerWindow centers the app window on the screen under the cursor.
func centerWindow() { C.qormCenterWindow() }

// screenInfo returns the displays as a JSON array [{w,h,scale,main}].
func screenInfo() string {
	c := C.qormScreenInfo()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

// windowFrame returns the app window frame as "x,y,w,h".
func windowFrame() string {
	c := C.qormWindowFrame()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

// setWindowFrame restores a saved frame (centers if it's off all screens).
func setWindowFrame(x, y, w, h int) {
	C.qormSetWindowFrame(C.int(x), C.int(y), C.int(w), C.int(h))
}

// grantMedia installs a WKUIDelegate on the app's WebView that grants
// camera/microphone capture requests, so getUserMedia works on macOS (WKWebView
// otherwise denies media capture without a delegate decision).
func grantMedia(window unsafe.Pointer) { C.qormGrantMedia(window) }

//export goBiometricResult
func goBiometricResult(ok C.int, msg *C.char) {
	if biometricHandler != nil {
		biometricHandler(ok == 1, C.GoString(msg))
	}
}

// nativeBiometric runs Touch ID (LocalAuthentication) and calls biometricHandler.
func nativeBiometric() { C.qormBiometric() }

// wifiDesktopInfo returns the current Wi-Fi as JSON {ssid,rssi} or {error}.
func wifiDesktopInfo() string {
	c := C.qormWifiInfo()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

//export goBluetoothState
func goBluetoothState(on C.int) {
	if btStateHandler != nil {
		btStateHandler(on == 1)
	}
}

//export goBluetoothScan
func goBluetoothScan(json *C.char) {
	if btScanHandler != nil {
		btScanHandler(C.GoString(json))
	}
}

func nativeBluetoothScan()  { C.qormBluetoothScan() }
func nativeBluetoothState() { C.qormBluetoothState() }

// disableRestore turns off macOS window state restoration, so a crash doesn't
// leave the app relaunching/reopening (which re-triggers the frame-change bug).
func disableRestore() { C.qormDisableRestore() }

// fixWindow pins the window (non-movable, non-resizable) so its frame never
// changes — a workaround for the webview_go+macOS WebKit abort on frame-change
// IPC (WebPageProxy::windowAndViewFramesChanged). Setting these flags does NOT
// change the frame, so it's safe.
func fixWindow() { C.qormFixWindow() }

// moveMainWindow repositions/resizes the app window (control-engine driven).
func moveMainWindow(x, y, w, h int) { C.qormMoveWindow(C.int(x), C.int(y), C.int(w), C.int(h)) }

// nativeBrightnessGet reads the main display brightness (0..1); ok=false if the
// private DisplayServices API is unavailable.
func nativeBrightnessGet() (float64, bool) {
	v := float64(C.qormGetBrightness())
	if v < 0 {
		return 0, false
	}
	return v, true
}

// nativeBrightnessSet sets the main display brightness (0..1).
func nativeBrightnessSet(v float64) int { return int(C.qormSetBrightness(C.double(v))) }

// nativeShare opens the native macOS share sheet with the given text.
func nativeShare(text string) {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	C.qormShareText(c)
}

var volumeWatchHandler func(float64)

//export goVolumeChanged
func goVolumeChanged(v C.double) {
	if volumeWatchHandler != nil {
		volumeWatchHandler(float64(v))
	}
}

var brightnessWatchHandler func(float64)

//export goBrightnessChanged
func goBrightnessChanged(v C.double) {
	if brightnessWatchHandler != nil {
		brightnessWatchHandler(float64(v))
	}
}

// nativeWatchBrightness registers a real-time display-brightness listener.
func nativeWatchBrightness() { C.qormWatchBrightness() }

var muteWatchHandler func(bool)

//export goMuteChanged
func goMuteChanged(m C.int) {
	if muteWatchHandler != nil {
		muteWatchHandler(m == 1)
	}
}

// nativeReadMute returns 1 muted, 0 unmuted, -1 unknown.
func nativeReadMute() int { return int(C.qormReadMute()) }

// nativeWatchVolume registers a real-time system-volume listener.
func nativeWatchVolume() { C.qormWatchVolume() }

// nativeSystemModes returns the readable system modes as JSON.
func nativeSystemModes() string {
	c := C.qormSystemModes()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

func nativeClipboardSet(text string) {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	C.qormClipboardSet(c)
}

func nativeClipboardGet() string {
	c := C.qormClipboardGet()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

func nativeOpenURL(url string) {
	c := C.CString(url)
	defer C.free(unsafe.Pointer(c))
	C.qormOpenURL(c)
}

func nativeOSVersion() string {
	c := C.qormOSVersion()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

func nativeSetKeepAwake(on bool) {
	val := 0
	if on {
		val = 1
	}
	C.qormKeepAwake(C.int(val))
}

func nativeSpeak(text string) {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	C.qormSpeak(c)
}

func nativeSpeakStop() {
	C.qormSpeakStop()
}

func nativeScreenshot() string {
	c := C.qormScreenshot()
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}

func nativeVolumeGet() (float64, bool) {
	v := float64(C.qormGetSystemVolume())
	if v < 0 {
		return 0, false
	}
	return v, true
}

func nativeVolumeSet(v float64) bool {
	return C.qormSetSystemVolume(C.float(v)) == 1
}

func nativeSecureSet(key, val string) bool {
	ck := C.CString(key)
	cv := C.CString(val)
	defer C.free(unsafe.Pointer(ck))
	defer C.free(unsafe.Pointer(cv))
	return C.qormSecureSet(ck, cv) == 1
}

func nativeSecureGet(key string) string {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	c := C.qormSecureGet(ck)
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}
