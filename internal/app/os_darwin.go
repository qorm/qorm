//go:build darwin && !ios

package app

import (
	"bytes"
	"image"
	"image/png"
	"unsafe"

	"github.com/qorm/qorm/internal/appkit"
)

var (
	clsNSData        uintptr
	selDataWithBytes uintptr
	clsNSImage       uintptr
	selInitWithData  uintptr
	selInitWithFrame uintptr
	selSetImage      uintptr
)

func lazyInitAppKit() {
	if clsNSData == 0 {
		clsNSData = appkit.ObjcGetClass("NSData")
		selDataWithBytes = appkit.SelRegisterName("dataWithBytes:length:")
		clsNSImage = appkit.ObjcGetClass("NSImage")
		selInitWithData = appkit.SelRegisterName("initWithData:")
		selInitWithFrame = appkit.SelRegisterName("initWithFrame:")
		selSetImage = appkit.SelRegisterName("setImage:")
	}
}

type windowImpl struct {
	ptr  uintptr
	view uintptr
}

// darwinKeyCodes maps macOS ANSI virtual keycodes to normalized key names.
// The traversal keys (tab/return/escape/space/delete/arrows) are layout-invariant;
// the alphanumerics follow the ANSI layout (JIS/ISO differ on a few letter keys).
var darwinKeyCodes = map[int]string{
	48: "tab", 36: "return", 76: "return", 53: "escape", 49: "space", 51: "delete",
	123: "left", 124: "right", 125: "down", 126: "up",
	0: "a", 1: "s", 2: "d", 3: "f", 4: "h", 5: "g", 6: "z", 7: "x", 8: "c", 9: "v",
	11: "b", 12: "q", 13: "w", 14: "e", 15: "r", 16: "y", 17: "t",
	31: "o", 32: "u", 34: "i", 35: "p", 37: "l", 38: "j", 40: "k", 45: "n", 46: "m",
	18: "1", 19: "2", 20: "3", 21: "4", 22: "6", 23: "5", 25: "9", 26: "7", 28: "8", 29: "0",
}

var activeWindow *Window

// NewWindow creates a new native window.
func NewWindow(title string, width, height int) *Window {
	lazyInitAppKit()
	clsNSApp := appkit.ObjcGetClass("NSApplication")
	app := appkit.MsgSend(clsNSApp, appkit.SelRegisterName("sharedApplication"))
	appkit.MsgSend(app, appkit.SelRegisterName("setActivationPolicy:"), 0)

	clsNSWindow := appkit.ObjcGetClass("NSWindow")
	alloc := appkit.MsgSend(clsNSWindow, appkit.SelRegisterName("alloc"))

	// Use NSInvocation to bypass ARM64 floating-point register ABI issues
	// We need to call: [alloc initWithContentRect:rect styleMask:15 backing:2 defer:0]
	sigMsg := appkit.SelRegisterName("instanceMethodSignatureForSelector:")
	sig := appkit.MsgSend(clsNSWindow, sigMsg, appkit.SelRegisterName("initWithContentRect:styleMask:backing:defer:"))
	
	clsNSInvocation := appkit.ObjcGetClass("NSInvocation")
	inv := appkit.MsgSend(clsNSInvocation, appkit.SelRegisterName("invocationWithMethodSignature:"), sig)
	
	selInitWindow := appkit.SelRegisterName("initWithContentRect:styleMask:backing:defer:")
	appkit.MsgSend(inv, appkit.SelRegisterName("setSelector:"), selInitWindow)
	appkit.MsgSend(inv, appkit.SelRegisterName("setTarget:"), alloc)
	
	rect := struct{ X, Y, W, H float64 }{100, 100, float64(width), float64(height)}
	rectPtr := uintptr(unsafe.Pointer(&rect))
	appkit.MsgSend(inv, appkit.SelRegisterName("setArgument:atIndex:"), rectPtr, 2)
	
	styleMask := uintptr(15)
	appkit.MsgSend(inv, appkit.SelRegisterName("setArgument:atIndex:"), uintptr(unsafe.Pointer(&styleMask)), 3)
	
	backing := uintptr(2)
	appkit.MsgSend(inv, appkit.SelRegisterName("setArgument:atIndex:"), uintptr(unsafe.Pointer(&backing)), 4)
	
	deferFlag := uintptr(0)
	appkit.MsgSend(inv, appkit.SelRegisterName("setArgument:atIndex:"), uintptr(unsafe.Pointer(&deferFlag)), 5)
	
	appkit.MsgSend(inv, appkit.SelRegisterName("invoke"))
	
	var winPtr uintptr
	appkit.MsgSend(inv, appkit.SelRegisterName("getReturnValue:"), uintptr(unsafe.Pointer(&winPtr)))

	appkit.MsgSend(winPtr, appkit.SelRegisterName("setTitle:"), appkit.NewNSString(title))

	// Now for NSImageView
	clsNSImageView := appkit.ObjcGetClass("NSImageView")
	viewAlloc := appkit.MsgSend(clsNSImageView, appkit.SelRegisterName("alloc"))
	
	sigView := appkit.MsgSend(clsNSImageView, sigMsg, appkit.SelRegisterName("initWithFrame:"))
	invView := appkit.MsgSend(clsNSInvocation, appkit.SelRegisterName("invocationWithMethodSignature:"), sigView)
	appkit.MsgSend(invView, appkit.SelRegisterName("setSelector:"), appkit.SelRegisterName("initWithFrame:"))
	appkit.MsgSend(invView, appkit.SelRegisterName("setTarget:"), viewAlloc)
	appkit.MsgSend(invView, appkit.SelRegisterName("setArgument:atIndex:"), rectPtr, 2)
	appkit.MsgSend(invView, appkit.SelRegisterName("invoke"))
	
	var view uintptr
	appkit.MsgSend(invView, appkit.SelRegisterName("getReturnValue:"), uintptr(unsafe.Pointer(&view)))
	
	appkit.MsgSend(winPtr, appkit.SelRegisterName("setContentView:"), view)

	appkit.MsgSend(winPtr, appkit.SelRegisterName("makeKeyAndOrderFront:"), 0)
	appkit.MsgSend(winPtr, appkit.SelRegisterName("setAcceptsMouseMovedEvents:"), 1)
	appkit.MsgSend(app, appkit.SelRegisterName("activateIgnoringOtherApps:"), 1)

	events := make(chan Event, 10)
	// Send initial frame event
	events <- FrameEvent{Size: image.Pt(width, height)}

	w := &Window{
		events: events,
		w:      &windowImpl{ptr: winPtr, view: view},
		width:  width,
		height: height,
	}
	activeWindow = w
	return w
}

func (w *Window) UpdateImage(img image.Image) error {
	lazyInitAppKit()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	data := buf.Bytes()
	nsData := appkit.MsgSend(clsNSData, selDataWithBytes, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))

	imgAlloc := appkit.MsgSend(clsNSImage, appkit.SelRegisterName("alloc"))
	nsImage := appkit.MsgSend(imgAlloc, selInitWithData, nsData)

	// Update UI on main thread
	selPerform := appkit.SelRegisterName("performSelectorOnMainThread:withObject:waitUntilDone:")
	appkit.MsgSend(w.w.view, selPerform, selSetImage, nsImage, 0)
	
	return nil
}

// Main starts the application event loop. It must be called on the main OS thread.
func Main() {
	clsNSApp := appkit.ObjcGetClass("NSApplication")
	app := appkit.MsgSend(clsNSApp, appkit.SelRegisterName("sharedApplication"))
	appkit.MsgSend(app, appkit.SelRegisterName("finishLaunching"))

	// Manual event loop using nextEventMatchingMask
	clsNSDate := appkit.ObjcGetClass("NSDate")
	selDistantFuture := appkit.SelRegisterName("distantFuture")
	selNextEvent := appkit.SelRegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	selType := appkit.SelRegisterName("type")
	selSendEvent := appkit.SelRegisterName("sendEvent:")
	selValueForKey := appkit.SelRegisterName("valueForKey:")
	selGetValue := appkit.SelRegisterName("getValue:")
	locKey := appkit.NewNSString("locationInWindow")

	distantFuture := appkit.MsgSend(clsNSDate, selDistantFuture)
	defaultMode := appkit.NewNSString("kCFRunLoopDefaultMode")

	for {
		// Wait for next event
		event := appkit.MsgSend(app, selNextEvent,
			^uintptr(0), // NSAnyEventMask
			distantFuture,
			defaultMode,
			1, // YES (dequeue)
		)

		if event != 0 {
			eventType := appkit.MsgSend(event, selType)

			// NSLeftMouseDown = 1, NSLeftMouseUp = 2, NSMouseMoved = 5, NSLeftMouseDragged = 6
			if eventType == 1 || eventType == 2 || eventType == 5 || eventType == 6 {
				locValue := appkit.MsgSend(event, selValueForKey, locKey)
				if locValue != 0 && activeWindow != nil {
					var point struct{ X, Y float64 }
					appkit.MsgSend(locValue, selGetValue, uintptr(unsafe.Pointer(&point)))
					
					ptType := PointerMove
					if eventType == 1 {
						ptType = PointerPress
					} else if eventType == 2 {
						ptType = PointerRelease
					}
					
					// Convert AppKit coordinates (bottom-left origin) to top-left
					// Use the real window content height (bottom-left origin flip).
					_, wh := activeWindow.Size()
					y := float64(wh) - point.Y
					
					select {
					case activeWindow.events <- PointerEvent{
						Type:     ptType,
						Position: image.Pt(int(point.X), int(y)),
					}:
					default:
					}
				}
			} else if eventType == 10 || eventType == 11 {
				// NSKeyDown = 10, NSKeyUp = 11
				selKeyCode := appkit.SelRegisterName("keyCode")
				keyCode := appkit.MsgSend(event, selKeyCode)

				kt := KeyDown
				if eventType == 11 {
					kt = KeyUp
				}

				// Read the shift modifier: [event valueForKey:@"modifierFlags"] → NSNumber.
				shift := false
				modVal := appkit.MsgSend(event, selValueForKey, appkit.NewNSString("modifierFlags"))
				if modVal != 0 {
					var flags uintptr
					appkit.MsgSend(modVal, selGetValue, uintptr(unsafe.Pointer(&flags)))
					shift = flags&(1<<17) != 0 // NSEventModifierFlagShift
				}

				if activeWindow != nil {
					select {
					case activeWindow.events <- KeyEvent{
						Type:  kt,
						Code:  int(keyCode),
						Key:   darwinKeyCodes[int(keyCode)],
						Shift: shift,
					}:
					default:
					}
				}
			} else if eventType == 22 {
				// NSScrollWheel
				
				// scrollingDeltaX/Y return CGFloat. AppKit MsgSend needs fpret for floats?
				// Actually, MsgSend returning struct/float on ARM64 is tricky, but we can try 
				// valueForKey: for safety or just use deltaX/deltaY which might be doubles.
				// Since we are pure go, let's use valueForKey: to be safe from ABI issues.
				valX := appkit.MsgSend(event, selValueForKey, appkit.NewNSString("scrollingDeltaX"))
				valY := appkit.MsgSend(event, selValueForKey, appkit.NewNSString("scrollingDeltaY"))
				
				var dx, dy float64
				if valX != 0 {
					appkit.MsgSend(valX, selGetValue, uintptr(unsafe.Pointer(&dx)))
				}
				if valY != 0 {
					appkit.MsgSend(valY, selGetValue, uintptr(unsafe.Pointer(&dy)))
				}
				
				if activeWindow != nil && (dx != 0 || dy != 0) {
					select {
					case activeWindow.events <- ScrollEvent{
						DeltaX: dx,
						DeltaY: dy,
					}:
					default:
					}
				}
			}

			appkit.MsgSend(app, selSendEvent, event)
		}
	}
}
