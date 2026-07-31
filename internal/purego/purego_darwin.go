package purego

import (
	"syscall"
	"unsafe"
)

//go:cgo_import_dynamic libc_dlopen dlopen "/usr/lib/libSystem.B.dylib"
func libc_dlopen_trampoline()

//go:cgo_import_dynamic libc_dlsym dlsym "/usr/lib/libSystem.B.dylib"
func libc_dlsym_trampoline()

//go:linkname syscall6 syscall.syscall6
func syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:linkname syscall9 syscall.syscall9
func syscall9(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2 uintptr, err syscall.Errno)

const (
	RTLD_LAZY   = 0x1
	RTLD_NOW    = 0x2
	RTLD_GLOBAL = 0x8
)

func dlopenTrampoline() uintptr
func dlsymTrampoline() uintptr

func Dlopen(path string, mode int) (uintptr, error) {
	var pathPtr *byte
	if path != "" {
		pathBytes := append([]byte(path), 0)
		pathPtr = &pathBytes[0]
	}
	r1, _, err := syscall6(dlopenTrampoline(), uintptr(unsafe.Pointer(pathPtr)), uintptr(mode), 0, 0, 0, 0)
	if r1 == 0 {
		return 0, err
	}
	return r1, nil
}

func Dlsym(handle uintptr, name string) (uintptr, error) {
	nameBytes := append([]byte(name), 0)
	r1, _, err := syscall6(dlsymTrampoline(), handle, uintptr(unsafe.Pointer(&nameBytes[0])), 0, 0, 0, 0)
	if r1 == 0 {
		return 0, err
	}
	return r1, nil
}

func SyscallN(fn uintptr, args ...uintptr) uintptr {
	var a [9]uintptr
	copy(a[:], args)
	if len(args) <= 6 {
		r1, _, _ := syscall6(fn, a[0], a[1], a[2], a[3], a[4], a[5])
		return r1
	}
	r1, _, _ := syscall9(fn, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8])
	return r1
}

// funcPC returns the pointer to the function
func funcPC(f interface{}) uintptr {
	return *(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&f)))
}
