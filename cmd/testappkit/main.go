//go:build darwin && !ios

package main

import (
	"fmt"
	"runtime"
	"unsafe"
	"github.com/qorm/qorm/internal/appkit"
)

func main() {
	runtime.LockOSThread()
	clsNSApp := appkit.ObjcGetClass("NSApplication")
	app := appkit.MsgSend(clsNSApp, appkit.SelRegisterName("sharedApplication"))

	// Manual event loop using nextEventMatchingMask
	clsNSDate := appkit.ObjcGetClass("NSDate")
	selDistantFuture := appkit.SelRegisterName("distantFuture")
	selNextEvent := appkit.SelRegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	selType := appkit.SelRegisterName("type")
	
	distantFuture := appkit.MsgSend(clsNSDate, selDistantFuture)
	defaultMode := appkit.NewNSString("kCFRunLoopDefaultMode")

	fmt.Println("Please click the mouse somewhere...")
	for {
		event := appkit.MsgSend(app, selNextEvent,
			^uintptr(0), // NSAnyEventMask
			distantFuture,
			defaultMode,
			1, // YES (dequeue)
		)

		if event != 0 {
			eventType := appkit.MsgSend(event, selType)
			if eventType == 1 { // NSLeftMouseDown
				fmt.Println("MouseDown detected!")
				
				// Try KVC
				locValue := appkit.MsgSend(event, appkit.SelRegisterName("valueForKey:"), appkit.NewNSString("locationInWindow"))
				if locValue != 0 {
					var point struct { X, Y float64 }
					appkit.MsgSend(locValue, appkit.SelRegisterName("getValue:"), uintptr(unsafe.Pointer(&point)))
					fmt.Printf("Point via KVC: %+v\n", point)
				} else {
					fmt.Println("KVC returned nil")
				}
				
				return
			}
			appkit.MsgSend(app, appkit.SelRegisterName("sendEvent:"), event)
		}
	}
}
