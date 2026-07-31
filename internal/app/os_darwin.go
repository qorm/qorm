//go:build darwin && !ios

package app

import (
	"image"
	"unsafe"

	"github.com/qorm/qorm/internal/appkit"
)

var (
	clsNSImage  uintptr
	selSetImage uintptr
)

func lazyInitAppKit() {
	if clsNSImage == 0 {
		clsNSImage = appkit.ObjcGetClass("NSImage")
		selSetImage = appkit.SelRegisterName("setImage:")
	}
}

type windowImpl struct {
	ptr  uintptr
	view uintptr

	// Zero-copy double-buffered presentation: two Go-owned pixel planes are
	// shared with NSBitmapImageReps, which hold the raw plane pointers for
	// the window's lifetime. Go's GC is non-moving and the pix slices stay
	// referenced here, so the planes can never move or vanish under AppKit.
	// The renderer draws into the back plane; Present swaps it to the front
	// synchronously on the main thread.
	pix    [2][]byte
	img    [2]uintptr // NSImage wrapping pix[i]'s rep
	stride int
	front  int // index of the plane currently displayed
	scale  int // device-pixel ratio (set from backingScaleFactor; 0/1 == 1)
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

	// --- Zero-copy presentation surfaces -------------------------------
	// Two NSBitmapImageReps sharing Go-allocated planes, each wrapped in an
	// NSImage. Present() flips which image the NSImageView shows; the
	// renderer draws straight into the other plane — no encode/decode, no
	// per-frame allocation.
	impl := &windowImpl{ptr: winPtr, view: view, stride: width * 4}
	clsBitmapRep := appkit.ObjcGetClass("NSBitmapImageRep")
	selAlloc := appkit.SelRegisterName("alloc")
	selInitRep := appkit.SelRegisterName("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bitmapFormat:bytesPerRow:bitsPerPixel:")
	selAddRep := appkit.SelRegisterName("addRepresentation:")
	selSetCacheMode := appkit.SelRegisterName("setCacheMode:")
	selInvWithSig := appkit.SelRegisterName("invocationWithMethodSignature:")
	selSetSel := appkit.SelRegisterName("setSelector:")
	selSetTgt := appkit.SelRegisterName("setTarget:")
	selSetArg := appkit.SelRegisterName("setArgument:atIndex:")
	selInvoke := appkit.SelRegisterName("invoke")
	selGetRet := appkit.SelRegisterName("getReturnValue:")
	deviceRGB := appkit.NewNSString("NSDeviceRGBColorSpace")
	// initWithSize: takes an NSSize (two CGFloats) — a struct argument, so
	// it must go through NSInvocation on ARM64 (same pattern as the window
	// initWithContentRect above).
	sizeVal := struct{ W, H float64 }{float64(width), float64(height)}
	sizePtr := uintptr(unsafe.Pointer(&sizeVal))
	selInitSize := appkit.SelRegisterName("initWithSize:")
	sigSize := appkit.MsgSend(clsNSImage, sigMsg, selInitSize)
	// initWithBitmapDataPlanes:… takes 12 parameters — beyond the 9-arg
	// SyscallN ceiling — so it goes through NSInvocation argument-by-argument.
	sigRep := appkit.MsgSend(clsBitmapRep, sigMsg, selInitRep)
	for i := 0; i < 2; i++ {
		impl.pix[i] = make([]byte, impl.stride*height)
		planes := [1]uintptr{uintptr(unsafe.Pointer(&impl.pix[i][0]))}

		argPlanes := uintptr(unsafe.Pointer(&planes[0]))
		argW := uintptr(width)
		argH := uintptr(height)
		argBPS := uintptr(8) // bitsPerSample
		argSPP := uintptr(4) // samplesPerPixel
		argAlpha := uintptr(1) // hasAlpha: YES
		argPlanar := uintptr(0) // isPlanar: NO
		// bitmapFormat = NSAlphaNonpremultipliedBitmapFormat (1<<1): the buffer
		// holds STRAIGHT (non-premultiplied) RGBA — the SoftwareRenderer blends
		// in straight space and the opaque fast path is identical either way.
		// Declaring non-premultiplied lets AppKit composite translucent pixels
		// correctly. (The blue-button canary is opaque, so it is unaffected.)
		argFmt := uintptr(2)
		argRow := uintptr(impl.stride)
		argBPP := uintptr(32)

		repAlloc := appkit.MsgSend(clsBitmapRep, selAlloc)
		invRep := appkit.MsgSend(clsNSInvocation, selInvWithSig, sigRep)
		appkit.MsgSend(invRep, selSetSel, selInitRep)
		appkit.MsgSend(invRep, selSetTgt, repAlloc)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argPlanes)), 2)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argW)), 3)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argH)), 4)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argBPS)), 5)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argSPP)), 6)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argAlpha)), 7)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argPlanar)), 8)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&deviceRGB)), 9)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argFmt)), 10)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argRow)), 11)
		appkit.MsgSend(invRep, selSetArg, uintptr(unsafe.Pointer(&argBPP)), 12)
		appkit.MsgSend(invRep, selInvoke)
		var rep uintptr
		appkit.MsgSend(invRep, selGetRet, uintptr(unsafe.Pointer(&rep)))

		imgAlloc := appkit.MsgSend(clsNSImage, selAlloc)
		invImg := appkit.MsgSend(clsNSInvocation, selInvWithSig, sigSize)
		appkit.MsgSend(invImg, selSetSel, selInitSize)
		appkit.MsgSend(invImg, selSetTgt, imgAlloc)
		appkit.MsgSend(invImg, selSetArg, sizePtr, 2)
		appkit.MsgSend(invImg, selInvoke)
		var img uintptr
		appkit.MsgSend(invImg, selGetRet, uintptr(unsafe.Pointer(&img)))
		appkit.MsgSend(img, selAddRep, rep)
		appkit.MsgSend(img, selSetCacheMode, 3) // NSImageCacheNever: draw the rep's live pixels
		impl.img[i] = img
	}
	// Direct call (not performSelectorOnMainThread): the main run loop is
	// not pumping yet — same regime as makeKeyAndOrderFront above.
	appkit.MsgSend(view, selSetImage, impl.img[0])
	// -------------------------------------------------------------------

	w := &Window{
		events: events,
		w:      impl,
		width:  width,
		height: height,
	}
	activeWindow = w
	return w
}

// Backbuffer returns the image backed by the back pixel plane — the buffer
// the next frame must be drawn into. Valid until the next Present.
func (w *Window) Backbuffer() *image.RGBA {
	back := 1 - w.w.front
	return &image.RGBA{
		Pix:    w.w.pix[back],
		Stride: w.w.stride,
		Rect:   image.Rect(0, 0, w.width, w.height),
	}
}

// Present publishes the drawn backbuffer: the image swap runs synchronously
// on the main thread (waitUntilDone:YES), so when Present returns the other
// plane is displayed and the caller may immediately start drawing the next
// frame into the new back plane — no torn frames, no races. setImage: marks
// the NSImageView dirty, so no extra setNeedsDisplay is needed.
func (w *Window) Present() {
	lazyInitAppKit()
	next := 1 - w.w.front
	selPerform := appkit.SelRegisterName("performSelectorOnMainThread:withObject:waitUntilDone:")
	appkit.MsgSend(w.w.view, selPerform, selSetImage, w.w.img[next], 1)
	w.w.front = next
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
					y := float64(activeWindow.Size().Y) - point.Y
					
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
