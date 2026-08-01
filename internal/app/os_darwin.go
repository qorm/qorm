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

	// Single-buffer zero-copy presentation: one Go-owned pixel plane shared
	// with an NSBitmapImageRep (which holds the raw pointer for the window's
	// lifetime; Go's GC is non-moving and the slice stays referenced here).
	// The MAIN thread both renders into pix (via the engine) and presents it,
	// so there is no render/display data race and no multi-plane dance — the
	// design that caused the old background-render flicker/blank bugs.
	pix    []byte
	img    uintptr // NSImage wrapping the rep over pix
	rep    uintptr // NSBitmapImageRep over pix (CGImage source each frame)
	layer  uintptr // view's CALayer; we push CGImages into its contents
	stride int
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
	// Keep the NSWindow alive after the user closes it (default is release on
	// close) so Run can poll isVisible on a valid object to detect the close.
	appkit.MsgSend(winPtr, appkit.SelRegisterName("setReleasedWhenClosed:"), 0)

	// --- Zero-copy presentation surface ----------------------------------
	// One Go-allocated pixel plane, wrapped in an NSBitmapImageRep (which holds
	// the raw pointer for the window's lifetime) and presented by pushing the
	// rep's CGImage into the view's layer (see PresentImage) — no encode/decode,
	// no per-frame allocation. The main thread both renders into the plane and
	// presents it, so a single plane is enough.
	// Device-pixel ratio (Retina == 2). Read via valueForKey to dodge the
	// ARM64 float-return ABI issue (a CGFloat comes back in an FP register,
	// which MsgSend's integer return would misread).
	scale := 1
	if v := appkit.MsgSend(winPtr, appkit.SelRegisterName("valueForKey:"), appkit.NewNSString("backingScaleFactor")); v != 0 {
		var f float64
		appkit.MsgSend(v, appkit.SelRegisterName("getValue:"), uintptr(unsafe.Pointer(&f)))
		if int(f) >= 1 {
			scale = int(f)
		}
	}
	physW, physH := width*scale, height*scale

	impl := &windowImpl{ptr: winPtr, view: view, stride: physW * 4, scale: scale}
	// One shared pixel plane; the NSBitmapImageRep wraps it (initWithBitmapData
	// Planes takes a pointer to the plane pointer — it does NOT copy the data).
	impl.pix = make([]byte, impl.stride*physH)
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
	// initWithContentRect above). NSImage size is in POINTS (logical): a hi-res
	// rep mapped into a point-sized image renders crisply on Retina.
	sizeVal := struct{ W, H float64 }{float64(width), float64(height)}
	sizePtr := uintptr(unsafe.Pointer(&sizeVal))
	selInitSize := appkit.SelRegisterName("initWithSize:")
	sigSize := appkit.MsgSend(clsNSImage, sigMsg, selInitSize)
	// initWithBitmapDataPlanes:… takes 12 parameters — beyond the 9-arg
	// SyscallN ceiling — so it goes through NSInvocation argument-by-argument.
	// The rep is in PHYSICAL pixels (the plane the renderer draws into).
	sigRep := appkit.MsgSend(clsBitmapRep, sigMsg, selInitRep)
	planePtr := uintptr(unsafe.Pointer(&impl.pix[0]))
	planes := [1]uintptr{planePtr}
	argPlanes := uintptr(unsafe.Pointer(&planes[0]))
	argW := uintptr(physW)
	argH := uintptr(physH)
	argBPS := uintptr(8)     // bitsPerSample
	argSPP := uintptr(4)     // samplesPerPixel
	argAlpha := uintptr(1)   // hasAlpha: YES
	argPlanar := uintptr(0)  // isPlanar: NO
	// bitmapFormat = NSAlphaNonpremultipliedBitmapFormat (1<<1): the buffer
	// holds STRAIGHT (non-premultiplied) RGBA — the SoftwareRenderer blends in
	// straight space and the opaque fast path is identical either way.
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
	appkit.MsgSend(invRep, selGetRet, uintptr(unsafe.Pointer(&impl.rep)))

	imgAlloc := appkit.MsgSend(clsNSImage, selAlloc)
	invImg := appkit.MsgSend(clsNSInvocation, selInvWithSig, sigSize)
	appkit.MsgSend(invImg, selSetSel, selInitSize)
	appkit.MsgSend(invImg, selSetTgt, imgAlloc)
	appkit.MsgSend(invImg, selSetArg, sizePtr, 2)
	appkit.MsgSend(invImg, selInvoke)
	appkit.MsgSend(invImg, selGetRet, uintptr(unsafe.Pointer(&impl.img)))
	appkit.MsgSend(impl.img, selAddRep, impl.rep)
	appkit.MsgSend(impl.img, selSetCacheMode, 3) // NSImageCacheNever: draw the rep's live pixels
	// Hand the image to the view ONCE. The rep wraps our live pixel plane and
	// the cache mode is "never", so the view always draws the plane's current
	// bytes; the per-frame -display (PresentImage) just forces it to repaint.
	// Without this the view's image is nil and drawRect: paints nothing — the
	// blank-white canvas we saw before.
	appkit.MsgSend(view, selSetImage, impl.img)

	// The initWithFrame: above reused the WINDOW's screen rect, whose X,Y is
	// the window's origin (100,100) — wrong for a subview, whose frame is in
	// its superview's coords and must start at 0,0. Left as-is the image view
	// sat off in the bottom-right corner (mostly clipped), so the visible
	// content area showed only the default white background. Pin the view to
	// the full content bounds (in points). setFrame: takes an NSRect struct →
	// NSInvocation on arm64.
	selSetFrame := appkit.SelRegisterName("setFrame:")
	sigSetFrame := appkit.MsgSend(clsNSImageView, sigMsg, selSetFrame)
	invSetFrame := appkit.MsgSend(clsNSInvocation, selInvWithSig, sigSetFrame)
	appkit.MsgSend(invSetFrame, selSetSel, selSetFrame)
	appkit.MsgSend(invSetFrame, selSetTgt, view)
	contentRect := struct{ X, Y, W, H float64 }{0, 0, float64(width), float64(height)}
	appkit.MsgSend(invSetFrame, selSetArg, uintptr(unsafe.Pointer(&contentRect)), 2)
	appkit.MsgSend(invSetFrame, selInvoke)

	// Make the view layer-backed and keep its layer: PresentImage pushes a
	// CGImage (built from impl.rep's live pixels) into layer.contents — the
	// deterministic AppKit path for "these are my pixels". NSImageView's own
	// drawRect never reliably painted the rep, so we draw via the layer.
	appkit.MsgSend(view, appkit.SelRegisterName("setWantsLayer:"), 1)
	impl.layer = appkit.MsgSend(view, appkit.SelRegisterName("layer"))
	// -------------------------------------------------------------------

	w := &Window{
		w:      impl,
		width:  width,
		height: height,
	}
	activeWindow = w
	return w
}

// Backbuffer returns the single shared pixel plane the engine renders into.
// Only the main thread ever touches it (render + present), so no sync needed.
func (w *Window) Backbuffer() *image.RGBA {
	return &image.RGBA{
		Pix:    w.w.pix,
		Stride: w.w.stride,
		Rect:   image.Rect(0, 0, w.width*w.w.scale, w.height*w.w.scale),
	}
}

// PresentImage publishes the pixels the engine just wrote into Backbuffer. It
// asks the rep (which wraps that buffer) for its CGImage — a CGImageRef over
// the rep's live pixels, vended by AppKit so we need no CoreGraphics dlsym —
// and sets it as the view layer's contents. That is the deterministic AppKit
// path for pushing our own pixels to screen: NSImageView's drawRect never
// reliably painted the rep, so we draw through the layer instead. Setting
// layer.contents implicitly invalidates the layer, so the next display cycle
// composites it; no explicit setNeedsDisplay/display is required. Must be
// called from the main thread (the caller — the main loop — is it). The
// returned CGImage is owned by the rep (we do NOT release it); the layer
// retains it for the frame it is drawn.
func (w *Window) PresentImage() {
	impl := w.w
	if impl.layer == 0 || impl.rep == 0 {
		return
	}
	cg := appkit.MsgSend(impl.rep, appkit.SelRegisterName("CGImage"))
	if cg == 0 {
		return
	}
	appkit.MsgSend(impl.layer, appkit.SelRegisterName("setContents:"), cg)
}

// Run runs the AppKit event loop on the (already main-locked) thread. For each
// AppKit event it decodes, it calls onEvent on the same thread; after every
// iteration (event or ~16ms poll timeout) it calls onTick — the host renders
// there, on the main thread, which is what makes the native canvas repaint
// reliably without the render/display race of a background render goroutine.
// When the window is closed the loop returns (the window is kept alive via
// setReleasedWhenClosed:NO, so polling isVisible is safe); minimization is NOT
// an exit — the window is hidden but still exists. After Run returns the
// caller (launchWindow → cmdRun) lets the process exit, and with it the HTTP
// goroutine: nothing keeps spinning after the window is gone.
func Run(onEvent func(Event), onTick func()) {
	clsNSApp := appkit.ObjcGetClass("NSApplication")
	app := appkit.MsgSend(clsNSApp, appkit.SelRegisterName("sharedApplication"))
	appkit.MsgSend(app, appkit.SelRegisterName("finishLaunching"))

	// Manual event loop using nextEventMatchingMask
	clsNSDate := appkit.ObjcGetClass("NSDate")
	selNextEvent := appkit.SelRegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	selType := appkit.SelRegisterName("type")
	selSendEvent := appkit.SelRegisterName("sendEvent:")
	selValueForKey := appkit.SelRegisterName("valueForKey:")
	selGetValue := appkit.SelRegisterName("getValue:")
	locKey := appkit.NewNSString("locationInWindow")

	defaultMode := appkit.NewNSString("kCFRunLoopDefaultMode")
	// Poll with a ~16ms deadline so MCP/state-driven redraws (no user event)
	// still show promptly. The deadline is a CGFloat (float register on arm64)
	// → build it via NSInvocation (class method → methodSignatureForSelector:).
	clsNSInvocation := appkit.ObjcGetClass("NSInvocation")
	selSinceNow := appkit.SelRegisterName("dateWithTimeIntervalSinceNow:")
	sigSinceNow := appkit.MsgSend(clsNSDate, appkit.SelRegisterName("methodSignatureForSelector:"), selSinceNow)
	deadlineInv := appkit.MsgSend(clsNSInvocation, appkit.SelRegisterName("invocationWithMethodSignature:"), sigSinceNow)
	appkit.MsgSend(deadlineInv, appkit.SelRegisterName("setSelector:"), selSinceNow)
	appkit.MsgSend(deadlineInv, appkit.SelRegisterName("setTarget:"), clsNSDate)
	pollInterval := 0.016
	appkit.MsgSend(deadlineInv, appkit.SelRegisterName("setArgument:atIndex:"), uintptr(unsafe.Pointer(&pollInterval)), 2)
	selRetPtr := appkit.SelRegisterName("getReturnValue:")
	selIsVisible := appkit.SelRegisterName("isVisible")
	selIsMiniaturized := appkit.SelRegisterName("isMiniaturized")

	// Events are decoded and dispatched to onEvent on this same thread; the
	// host's onTick (render + present) then runs every iteration. The loop
	// exits when the window is closed so the process can shut down with it.
	for {
		// Window closed (red button / Cmd-W)? isVisible goes NO on both close
		// and miniaturize — only close also leaves isMiniaturized NO. The
		// window outlives its close (setReleasedWhenClosed:NO), so polling it
		// here cannot touch a released object.
		if activeWindow != nil &&
			appkit.MsgSend(activeWindow.w.ptr, selIsVisible) == 0 &&
			appkit.MsgSend(activeWindow.w.ptr, selIsMiniaturized) == 0 {
			return
		}

		appkit.MsgSend(deadlineInv, appkit.SelRegisterName("invoke"))
		var deadline uintptr
		appkit.MsgSend(deadlineInv, selRetPtr, uintptr(unsafe.Pointer(&deadline)))

		event := appkit.MsgSend(app, selNextEvent,
			^uintptr(0), // NSAnyEventMask
			deadline,
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

					if onEvent != nil {
						onEvent(PointerEvent{Type: ptType, Position: image.Pt(int(point.X), int(y))})
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

				if activeWindow != nil && onEvent != nil {
					onEvent(KeyEvent{Type: kt, Code: int(keyCode), Key: darwinKeyCodes[int(keyCode)], Shift: shift})
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
				
				if activeWindow != nil && (dx != 0 || dy != 0) && onEvent != nil {
					onEvent(ScrollEvent{DeltaX: dx, DeltaY: dy})
				}
			}

			appkit.MsgSend(app, selSendEvent, event)
		}

		// Host tick (render + present) runs every iteration on the main thread.
		if onTick != nil {
			onTick()
		}
	}
}
