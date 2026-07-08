//go:build desktop && windows

package main

import (
	"math"
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

// --- Core Audio (WASAPI) master volume via raw COM vtable calls -------------
//
// No cgo, no third-party COM package: ole32.dll entry points via NewLazyDLL
// (matching the DPAPI code above) and hand-indexed vtable slots. GUID values
// are the canonical constants from the Windows SDK headers:
//
//	mmdeviceapi.h:     CLSID_MMDeviceEnumerator {BCDE0395-E52F-467C-8E3D-C4579291692E}
//	mmdeviceapi.h:     IID_IMMDeviceEnumerator  {A95664D2-9614-4F35-A746-DE8DB63617E6}
//	endpointvolume.h:  IID_IAudioEndpointVolume {5CDF2C82-841E-4546-9722-0CF74078229A}
//
// comGUID mirrors the in-memory GUID layout: Data1/2/3 are native-endian
// integer fields (little-endian on x86/ARM Windows), Data4 is a raw byte
// array — so the fields are written exactly as they read in registry form.
type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidMMDeviceEnumerator = comGUID{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = comGUID{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = comGUID{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}
)

const (
	clsctxAll        = 0x17       // CLSCTX_INPROC_SERVER|INPROC_HANDLER|LOCAL_SERVER|REMOTE_SERVER
	rpcEChangedMode  = 0x80010106 // COM already initialized on this thread with another model
	comSOK           = 0
	comSFalse        = 1
	vtblRelease      = 2 // IUnknown::Release
	vtblGetDefaultEP = 4 // IMMDeviceEnumerator::GetDefaultAudioEndpoint
	vtblActivate     = 3 // IMMDevice::Activate
	vtblSetScalar    = 7 // IAudioEndpointVolume::SetMasterVolumeLevelScalar
	vtblGetScalar    = 9 // IAudioEndpointVolume::GetMasterVolumeLevelScalar
)

var (
	ole32              = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
	procCoCreateInst   = ole32.NewProc("CoCreateInstance")
)

// comCall invokes the method at vtable slot `slot` on a COM interface pointer
// (first pointer-sized word of the object is the vtable). Returns the HRESULT.
func comCall(obj unsafe.Pointer, slot int, args ...uintptr) uint32 {
	vtbl := *(**[64]uintptr)(obj)
	r, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(obj)}, args...)...)
	return uint32(r)
}

func comRelease(obj unsafe.Pointer) { comCall(obj, vtblRelease) }

// withAudioEndpointVolume runs f with an activated IAudioEndpointVolume for
// the default render/console device, releasing every interface on the way out.
// Returns false when any step of the COM chain (or f itself) fails.
func withAudioEndpointVolume(f func(aev unsafe.Pointer) bool) bool {
	hr, _, _ := procCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	switch uint32(hr) {
	case comSOK, comSFalse:
		defer procCoUninitialize.Call() // balance our successful init
	case rpcEChangedMode:
		// Thread already lives in an STA — COM is usable, just don't uninit.
	default:
		return false
	}

	var enum unsafe.Pointer
	hr, _, _ = procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0, // no aggregation
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enum)),
	)
	if uint32(hr) != comSOK || enum == nil {
		return false
	}
	defer comRelease(enum)

	// GetDefaultAudioEndpoint(eRender=0, eConsole=0, &dev)
	var dev unsafe.Pointer
	if comCall(enum, vtblGetDefaultEP, 0, 0, uintptr(unsafe.Pointer(&dev))) != comSOK || dev == nil {
		return false
	}
	defer comRelease(dev)

	// Activate(IID_IAudioEndpointVolume, CLSCTX_ALL, nil, &aev)
	var aev unsafe.Pointer
	if comCall(dev, vtblActivate, uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)), clsctxAll, 0, uintptr(unsafe.Pointer(&aev))) != comSOK || aev == nil {
		return false
	}
	defer comRelease(aev)

	return f(aev)
}

// nativeVolumeGet reads the master output volume (0..1) via IAudioEndpointVolume.
func nativeVolumeGet() (float64, bool) {
	var level float32
	ok := withAudioEndpointVolume(func(aev unsafe.Pointer) bool {
		return comCall(aev, vtblGetScalar, uintptr(unsafe.Pointer(&level))) == comSOK
	})
	if !ok {
		return 0, false
	}
	return float64(level), true
}

// nativeVolumeSet sets the master output volume (0..1) via IAudioEndpointVolume.
func nativeVolumeSet(v float64) bool {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return withAudioEndpointVolume(func(aev unsafe.Pointer) bool {
		// SetMasterVolumeLevelScalar(float fLevel, LPCGUID pguidEventContext=nil).
		// Go's asmstdcall mirrors the first four integer args into XMM0-3, so
		// passing the raw float32 bits lands fLevel in XMM1 as the x64 ABI wants.
		return comCall(aev, vtblSetScalar, uintptr(math.Float32bits(float32(v))), 0) == comSOK
	})
}
