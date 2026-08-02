//go:build darwin && !ios

package app

import (
	"image"
	"unicode"
	"unsafe"

	"github.com/qorm/qorm/internal/appkit"
)

type windowImpl struct {
	ptr  uintptr
	view uintptr

	// Single-buffer zero-copy presentation: one Go-owned pixel plane shared
	// with an NSBitmapImageRep (which holds the raw pointer for the window's
	// lifetime; Go's GC is non-moving and the slice stays referenced here).
	// The MAIN thread both renders into pix (via the engine) and presents it,
	// so there is no render/display data race and no multi-plane dance — the
	// design that caused the old background-render flicker/blank bugs.
	pix     []byte
	keepPix []byte  // previous plane, retained one generation (see Resize)
	rep     uintptr // NSBitmapImageRep over pix (CGImage source each frame)
	layer   uintptr // view's CALayer; we push CGImages into its contents
	stride  int
	scale   int // device-pixel ratio (set from backingScaleFactor; 0/1 == 1)
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
	selInvWithSig := appkit.SelRegisterName("invocationWithMethodSignature:")
	selSetSel := appkit.SelRegisterName("setSelector:")
	selSetTgt := appkit.SelRegisterName("setTarget:")
	selSetArg := appkit.SelRegisterName("setArgument:atIndex:")
	selInvoke := appkit.SelRegisterName("invoke")
	impl.rep = newBitmapRep(impl)

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
	// Track window resizes: the view (and with it the layer we present into)
	// follows the contentView — NSViewWidthSizable|NSViewHeightSizable.
	appkit.MsgSend(view, appkit.SelRegisterName("setAutoresizingMask:"), 18)

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

// LiveSize returns the window's CURRENT content size in points, read from the
// view's frame — the view is the contentView and tracks the window through
// its autoresizing mask. (Size is only the creation-time size.)
func (w *Window) LiveSize() image.Point {
	if fv := appkit.MsgSend(w.w.view, appkit.SelRegisterName("valueForKey:"), appkit.NewNSString("frame")); fv != 0 {
		var frame struct{ X, Y, W, H float64 }
		appkit.MsgSend(fv, appkit.SelRegisterName("getValue:"), uintptr(unsafe.Pointer(&frame)))
		return image.Pt(int(frame.W), int(frame.H))
	}
	return image.Pt(w.width, w.height)
}

// ContentView returns the window's content view (the NSImageView the canvas
// presents into) as an opaque AppKit handle. Hosts that layer native platform
// views over the canvas (the canvaswebview build's WKWebView overlays) add
// their subviews to it; pure-Go hosts never need it.
func (w *Window) ContentView() uintptr { return w.w.view }

// Resize rebuilds the pixel plane for a new content size, re-reading the
// backing scale factor too (the window may have crossed displays). The engine
// re-lays out and re-renders on its next frame (the caller marks it dirty);
// the next PresentImage vends a rep over the new plane. The OLD plane stays
// referenced one generation (keepPix): a CGImage still sitting on the layer
// may read from it, and Go must not free memory AppKit can touch.
func (w *Window) Resize(wPts, hPts int) {
	if wPts < 1 || hPts < 1 {
		return // degenerate size (mid-minimize): keep the current plane (R6-A)
	}
	impl := w.w
	if v := appkit.MsgSend(impl.ptr, appkit.SelRegisterName("valueForKey:"), appkit.NewNSString("backingScaleFactor")); v != 0 {
		var f float64
		appkit.MsgSend(v, appkit.SelRegisterName("getValue:"), uintptr(unsafe.Pointer(&f)))
		if int(f) >= 1 {
			impl.scale = int(f)
		}
	}
	w.width, w.height = wPts, hPts
	physW, physH := wPts*impl.scale, hPts*impl.scale
	if physW > 0 && physH > 0 && physW*4 == impl.stride && len(impl.pix) == impl.stride*physH {
		return // nothing to rebuild (scale unchanged, same physical size)
	}
	impl.keepPix = impl.pix
	impl.stride = physW * 4
	impl.pix = make([]byte, impl.stride*physH)
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

// newBitmapRep wraps the window's shared pixel plane in a FRESH
// NSBitmapImageRep. initWithBitmapDataPlanes:… takes 12 parameters — beyond
// the 9-arg SyscallN ceiling — so it goes through NSInvocation
// argument-by-argument. The rep is in PHYSICAL pixels (the plane the renderer
// draws into) and does NOT copy the data (planes holds the plane pointer).
//
// A fresh rep is needed per present because a rep caches the CGImage it
// vends: rep.CGImage freezes whatever the plane held when THAT rep first
// produced one, so a long-lived rep can only ever show a setup-time snapshot
// (the blank window). The rep itself is a small object over the shared plane
// — this is an object alloc, not a pixel copy.
func newBitmapRep(impl *windowImpl) uintptr {
	clsBitmapRep := appkit.ObjcGetClass("NSBitmapImageRep")
	clsNSInvocation := appkit.ObjcGetClass("NSInvocation")
	selAlloc := appkit.SelRegisterName("alloc")
	selInitRep := appkit.SelRegisterName("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bitmapFormat:bytesPerRow:bitsPerPixel:")
	selInvWithSig := appkit.SelRegisterName("invocationWithMethodSignature:")
	selMsg := appkit.SelRegisterName("instanceMethodSignatureForSelector:")
	selSetSel := appkit.SelRegisterName("setSelector:")
	selSetTgt := appkit.SelRegisterName("setTarget:")
	selSetArg := appkit.SelRegisterName("setArgument:atIndex:")
	selInvoke := appkit.SelRegisterName("invoke")
	selGetRet := appkit.SelRegisterName("getReturnValue:")
	deviceRGB := appkit.NewNSString("NSDeviceRGBColorSpace")

	physW := impl.stride / 4
	physH := len(impl.pix) / impl.stride
	planePtr := uintptr(unsafe.Pointer(&impl.pix[0]))
	planes := [1]uintptr{planePtr}
	argPlanes := uintptr(unsafe.Pointer(&planes[0]))
	argW := uintptr(physW)
	argH := uintptr(physH)
	argBPS := uintptr(8)    // bitsPerSample
	argSPP := uintptr(4)    // samplesPerPixel
	argAlpha := uintptr(1)  // hasAlpha: YES
	argPlanar := uintptr(0) // isPlanar: NO
	// bitmapFormat = NSAlphaNonpremultipliedBitmapFormat (1<<1): the buffer
	// holds STRAIGHT (non-premultiplied) RGBA — the SoftwareRenderer blends in
	// straight space and the opaque fast path is identical either way.
	argFmt := uintptr(2)
	argRow := uintptr(impl.stride)
	argBPP := uintptr(32)

	sigRep := appkit.MsgSend(clsBitmapRep, selMsg, selInitRep)
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
	return rep
}

// PresentImage publishes the pixels the engine just wrote into Backbuffer. It
// vends a FRESH rep over the live plane (see newBitmapRep) and pushes its
// CGImage — a snapshot of the plane's CURRENT bytes — as the view layer's
// contents. Setting layer.contents implicitly invalidates the layer, so the
// next display cycle composites it; no explicit setNeedsDisplay/display is
// required. Must be called from the main thread (the caller — the main loop —
// is it).
//
// Ownership: the layer retains the CGImage it is given; the previous rep is
// released AFTER its CGImage has been replaced on the layer, so no frame ever
// shows a stale or freed image. The per-frame autorelease traffic (the
// NSInvocation dance) is bounded by an explicit pool.
func (w *Window) PresentImage() {
	impl := w.w
	if impl.layer == 0 {
		return
	}
	pool := appkit.MsgSend(appkit.MsgSend(appkit.ObjcGetClass("NSAutoreleasePool"), appkit.SelRegisterName("alloc")), appkit.SelRegisterName("init"))
	rep := newBitmapRep(impl)
	if rep != 0 {
		if cg := appkit.MsgSend(rep, appkit.SelRegisterName("CGImage")); cg != 0 {
			appkit.MsgSend(impl.layer, appkit.SelRegisterName("setContents:"), cg)
			if impl.rep != 0 {
				appkit.MsgSend(impl.rep, appkit.SelRegisterName("release"))
			}
			impl.rep = rep
		} else {
			appkit.MsgSend(rep, appkit.SelRegisterName("release"))
		}
	}
	appkit.MsgSend(pool, appkit.SelRegisterName("drain"))
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
					// Printable character for text input (KeyDown only):
					// -characters is modifier-aware (shift is applied); control
					// keys report private-use runes (U+F700–F8FF) or
					// unprintables, which are dropped — Key carries those.
					var r rune
					if kt == KeyDown {
						if chars := appkit.MsgSend(event, appkit.SelRegisterName("characters")); chars != 0 {
							for _, rr := range objcString(chars, 16) {
								if unicode.IsPrint(rr) && (rr < 0xF700 || rr > 0xF8FF) {
									r = rr
									break
								}
							}
						}
					}
					onEvent(KeyEvent{Type: kt, Code: int(keyCode), Key: darwinKeyCodes[int(keyCode)], Shift: shift, Rune: r})
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

// objcString copies an NSString into a Go string via -getCString:maxLength:
// encoding: (NSUTF8StringEncoding = 4), writing into a Go-owned buffer — the
// only unsafe.Pointer conversion is Go→C, never a uintptr read-back.
func objcString(ns uintptr, max int) string {
	if ns == 0 {
		return ""
	}
	buf := make([]byte, max)
	ok := appkit.MsgSend(ns, appkit.SelRegisterName("getCString:maxLength:encoding:"),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(max), 4)
	if ok == 0 {
		return ""
	}
	n := 0
	for n < max && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

// SetCursor sets the window's mouse cursor: 0 = arrow, 1 = I-beam (text),
// 2 = pointing hand (mirroring canvas.CursorHint's ordinals, kept in cmd so
// the app package stays engine-free). Called on the main thread from the
// event loop's hover path.
func (w *Window) SetCursor(which int) {
	var sel uintptr
	switch which {
	case 1:
		sel = appkit.SelRegisterName("IBeamCursor")
	case 2:
		sel = appkit.SelRegisterName("pointingHandCursor")
	default:
		sel = appkit.SelRegisterName("arrowCursor")
	}
	cls := appkit.ObjcGetClass("NSCursor")
	cur := appkit.MsgSend(cls, sel)
	if cur != 0 {
		appkit.MsgSend(cur, appkit.SelRegisterName("set"))
	}
}
