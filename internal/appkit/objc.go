package appkit

import (
	"unsafe"

	"github.com/qorm/qorm/internal/purego"
)

var (
	libobjc uintptr
	appkit  uintptr
)

var (
	objc_getClass    uintptr
	sel_registerName uintptr
	objc_msgSend     uintptr
)

func init() {
	var err error
	libobjc, err = purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	appkit, err = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}

	objc_getClass, _ = purego.Dlsym(libobjc, "objc_getClass")
	sel_registerName, _ = purego.Dlsym(libobjc, "sel_registerName")
	objc_msgSend, _ = purego.Dlsym(libobjc, "objc_msgSend")
}

func ObjcGetClass(name string) uintptr {
	return purego.SyscallN(objc_getClass, stringPtr(name))
}

func SelRegisterName(name string) uintptr {
	return purego.SyscallN(sel_registerName, stringPtr(name))
}

func MsgSend(receiver, sel uintptr, args ...uintptr) uintptr {
	callArgs := make([]uintptr, len(args)+2)
	callArgs[0] = receiver
	callArgs[1] = sel
	copy(callArgs[2:], args)
	return purego.SyscallN(objc_msgSend, callArgs...)
}

func stringPtr(s string) uintptr {
	if s == "" {
		return 0
	}
	b := append([]byte(s), 0)
	return uintptr(unsafe.Pointer(&b[0]))
}

func NewNSString(s string) uintptr {
	cls := ObjcGetClass("NSString")
	sel := SelRegisterName("stringWithUTF8String:")
	return MsgSend(cls, sel, stringPtr(s))
}
