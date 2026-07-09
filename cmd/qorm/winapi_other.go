//go:build desktop && !darwin && !windows && !linux

package main

import "unsafe"

// setWindowPos: webview_go has no window-move API and no GTK path is wired yet.
func setWindowPos(hwnd unsafe.Pointer, x, y, w, h int) {}

// startWindowDrag: no GTK drag path wired yet.
func startWindowDrag(hwnd unsafe.Pointer) {}

// nativeSecureSet/Get: no OS-backed secure storage wired on BSD yet; refuse
// rather than store plaintext. Linux has a real DBus Secret Service path in
// secret_linux.go.
func nativeSecureSet(key, val string) bool { return false }
func nativeSecureGet(key string) string    { return "" }

// nativeVolumeGet/Set: no native master-volume API wired on BSD; the desktop
// bridge uses pactl there (see desktopHardwareLinux).
func nativeVolumeGet() (float64, bool) { return 0, false }
func nativeVolumeSet(v float64) bool   { return false }
