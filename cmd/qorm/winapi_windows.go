//go:build desktop && windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// setWindowPos moves/resizes a native window (user32 SetWindowPos).
func setWindowPos(hwnd unsafe.Pointer, x, y, w, h int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SetWindowPos")
	// SWP_NOZORDER = 0x0004 | SWP_NOACTIVATE = 0x0010
	proc.Call(
		uintptr(hwnd),
		0,
		uintptr(x),
		uintptr(y),
		uintptr(w),
		uintptr(h),
		0x0004|0x0010,
	)
}

// startWindowDrag begins a native window drag from a chromeless window.
func startWindowDrag(hwnd unsafe.Pointer) {
	user32 := syscall.NewLazyDLL("user32.dll")
	releaseCapture := user32.NewProc("ReleaseCapture")
	sendMessage := user32.NewProc("SendMessageW")
	// WM_NCLBUTTONDOWN = 0x00A1, HTCAPTION = 2
	releaseCapture.Call()
	sendMessage.Call(uintptr(hwnd), 0x00A1, 2, 0)
}

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func encryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	dll := syscall.NewLazyDLL("crypt32.dll")
	proc := dll.NewProc("CryptProtectData")

	in := dataBlob{
		cbData: uint32(len(data)),
		pbData: &data[0],
	}
	var out dataBlob

	r, _, err := proc.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(out.pbData)))

	sh := (*[1 << 30]byte)(unsafe.Pointer(out.pbData))[:out.cbData:out.cbData]
	resultBytes := make([]byte, out.cbData)
	copy(resultBytes, sh)
	return resultBytes, nil
}

func decryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	dll := syscall.NewLazyDLL("crypt32.dll")
	proc := dll.NewProc("CryptUnprotectData")

	in := dataBlob{
		cbData: uint32(len(data)),
		pbData: &data[0],
	}
	var out dataBlob

	r, _, err := proc.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(out.pbData)))

	sh := (*[1 << 30]byte)(unsafe.Pointer(out.pbData))[:out.cbData:out.cbData]
	resultBytes := make([]byte, out.cbData)
	copy(resultBytes, sh)
	return resultBytes, nil
}

func nativeSecureSet(key, val string) bool {
	enc, err := encryptDPAPI([]byte(val))
	if err != nil {
		return false
	}
	path := filepath.Join(os.TempDir(), "qorm-sec-"+key)
	return os.WriteFile(path, enc, 0o600) == nil
}

func nativeSecureGet(key string) string {
	path := filepath.Join(os.TempDir(), "qorm-sec-"+key)
	enc, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	dec, err := decryptDPAPI(enc)
	if err != nil {
		return ""
	}
	return string(dec)
}
