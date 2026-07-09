//go:build darwin && desktop

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// captureWindow captures the on-screen window `win` by shelling out to Apple's
// /usr/sbin/screencapture -l<windowNumber>. On macOS 15+/26 every in-process
// capture API is broken — WKWebView takeSnapshot returns white, CGWindowListCreateImage
// was removed (0xbad4007), and SCScreenshotManager gets SIGBUS-killed for an
// unbundled CLI. The system screencapture tool is properly entitled and still
// works (using the invoking terminal's Screen-Recording permission), and -l takes
// a window number even if occluded. The window must be shown so it has a real
// windowNumber. Returns 1 on success.
static int captureWindow(NSWindow *win, const char *out) {
    for (int i = 0; i < 6; i++) // let the compositor commit a frame
        [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                 beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
    long wid = [win windowNumber];
    if (wid <= 0) return 0;
    NSTask *t = [[NSTask alloc] init];
    t.launchPath = @"/usr/sbin/screencapture";
    t.arguments = @[@"-x", @"-o", [NSString stringWithFormat:@"-l%ld", wid], [NSString stringWithUTF8String:out]];
    @try { [t launch]; [t waitUntilExit]; }
    @catch (id e) { return 0; }
    return (t.terminationStatus == 0 &&
            [[NSFileManager defaultManager] fileExistsAtPath:[NSString stringWithUTF8String:out]]) ? 1 : 0;
}

// qormFreezeAnimations disables CSS animations/transitions before a snapshot: an
// offscreen WebView throttles them, so an in-progress entrance animation would be
// captured at its opacity:0 start. Removing them renders each node at its base
// (final) style instead.
static void qormFreezeAnimations(WKWebView *wv) {
    [wv evaluateJavaScript:@"(function(){var s=document.createElement('style');s.textContent='*{animation:none !important;transition:none !important;}';document.documentElement.appendChild(s);})()" completionHandler:nil];
    NSDate *settle = [NSDate dateWithTimeIntervalSinceNow:0.3];
    while ([settle timeIntervalSinceNow] > 0)
        [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                 beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
}

// qormShot renders html in an offscreen WKWebView and writes a PNG to out.
// Returns 1 on success. Synchronous: it spins the run loop until the snapshot
// completes (WebKit is async), so QORM can rasterize its own UI to an image.
static int qormShot(const char* html, int w, int h, const char* out) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        NSRect frame = NSMakeRect(0, 0, w, h);
        WKWebViewConfiguration *cfg = [[WKWebViewConfiguration alloc] init];
        WKWebView *wv = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        NSWindow *win = [[NSWindow alloc] initWithContentRect:frame
                          styleMask:NSWindowStyleMaskBorderless
                            backing:NSBackingStoreBuffered defer:NO];
        [win setContentView:wv];
        // On-screen + key so ScreenCaptureKit can see the window to capture it.
        [win center];
        [win makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        [wv loadHTMLString:[NSString stringWithUTF8String:html] baseURL:nil];

        // let it load, lay out, and paint
        NSDate *loaded = [NSDate dateWithTimeIntervalSinceNow:1.8];
        while ([loaded timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];

        qormFreezeAnimations(wv);
        return captureWindow(win, out);
    }
}

// qormShotURL is like qormShot but loads a live URL (loadRequest), so it can
// capture a running QORM page whose iframe and fetch() need a real origin — e.g.
// the /console collaboration panel. It waits longer for the network + first poll.
static int qormShotURL(const char* url, int w, int h, const char* out) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        NSRect frame = NSMakeRect(0, 0, w, h);
        WKWebViewConfiguration *cfg = [[WKWebViewConfiguration alloc] init];
        WKWebView *wv = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        NSWindow *win = [[NSWindow alloc] initWithContentRect:frame
                          styleMask:NSWindowStyleMaskBorderless
                            backing:NSBackingStoreBuffered defer:NO];
        [win setContentView:wv];
        // On-screen + key: a hidden window throttles JS (so /logwindow never polls)
        // AND ScreenCaptureKit needs it visible to capture.
        [win center];
        [win makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        NSURL *u = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        [wv loadRequest:[NSURLRequest requestWithURL:u]];

        NSDate *loaded = [NSDate dateWithTimeIntervalSinceNow:3.0];
        while ([loaded timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];

        qormFreezeAnimations(wv);
        return captureWindow(win, out);
    }
}
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

func runShot(html string, w, h int, out string) bool {
	ch := C.CString(html)
	co := C.CString(out)
	defer C.free(unsafe.Pointer(ch))
	defer C.free(unsafe.Pointer(co))
	return C.qormShot(ch, C.int(w), C.int(h), co) != 0
}

// runShotURL captures a live URL (e.g. a running app's /logwindow panel).
func runShotURL(url string, w, h int, out string) bool {
	cu := C.CString(url)
	co := C.CString(out)
	defer C.free(unsafe.Pointer(cu))
	defer C.free(unsafe.Pointer(co))
	return C.qormShotURL(cu, C.int(w), C.int(h), co) != 0
}

// runShotLive captures an ALREADY-RUNNING window whose app/title contains `title`.
// It resolves the window id via Python's Quartz binding — which, unlike the cgo
// CoreGraphics binding, does NOT abort (0xbad4007) on macOS 26 — then captures it
// with Apple's screencapture -l. No in-process capture API is touched, so nothing
// gets SIGBUS-killed. Lets a recorder grab the live app + DevTool windows without
// spinning up throwaway WebViews.
func runShotLive(title, out string) bool {
	// Prefer an exact window-title match (so "QORM Premium Counter" picks the app
	// window, not the "QORM Premium Counter — Activity log" one), then fall back to
	// the first substring match on owner+title.
	const py = `import Quartz,sys
t=sys.argv[1].lower()
wl=Quartz.CGWindowListCopyWindowInfo(Quartz.kCGWindowListOptionOnScreenOnly|Quartz.kCGWindowListExcludeDesktopElements,Quartz.kCGNullWindowID)
sub=None
for w in wl:
    name=(w.get('kCGWindowName') or ''); owner=(w.get('kCGWindowOwnerName') or '')
    b=w.get('kCGWindowBounds') or {}
    if b.get('Width',0)<=60: continue
    if name.lower()==t: print(w['kCGWindowNumber']); sys.exit()
    if sub is None and t in (owner+' '+name).lower(): sub=w['kCGWindowNumber']
print(sub if sub is not None else '')`
	idb, err := exec.Command("python3", "-c", py, title).Output()
	id := strings.TrimSpace(string(idb))
	if err != nil || id == "" {
		return false
	}
	return exec.Command("/usr/sbin/screencapture", "-x", "-o", "-l"+id, out).Run() == nil
}

// cmdShot renders a QORM app to a PNG via an offscreen WebKit WebView.
func cmdShot(args []string) int {
	in, out, htmlFile, urlArg, liveArg, w, h := "", "", "", "", "", 440, 720
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		case "--html":
			if i+1 < len(args) {
				i++
				htmlFile = args[i]
			}
		case "--url":
			if i+1 < len(args) {
				i++
				urlArg = args[i]
			}
		case "--live":
			if i+1 < len(args) {
				i++
				liveArg = args[i]
			}
		case "--width":
			if i+1 < len(args) {
				i++
				w, _ = strconv.Atoi(args[i])
			}
		case "--height":
			if i+1 < len(args) {
				i++
				h, _ = strconv.Atoi(args[i])
			}
		default:
			in = args[i]
		}
	}
	if liveArg != "" {
		if out == "" {
			out = "shot.png"
		}
		if !runShotLive(liveArg, out) {
			fmt.Fprintln(os.Stderr, "error: could not capture a live window matching "+strconv.Quote(liveArg)+
				" (is the app running? and grant Screen Recording to your terminal in System Settings › Privacy)")
			return 1
		}
		fmt.Printf("wrote %s (live window %q)\n", out, liveArg)
		return 0
	}
	if urlArg != "" {
		if out == "" {
			out = "shot.png"
		}
		if !runShotURL(urlArg, w, h, out) {
			fmt.Fprintln(os.Stderr, "error: snapshot failed (needs a macOS GUI session)")
			return 1
		}
		fmt.Printf("wrote %s (%dx%d)\n", out, w, h)
		return 0
	}
	if in == "" && htmlFile == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm shot <app-dir> -o out.png [--width W --height H]\n       qorm shot --html page.html -o out.png\n       qorm shot --url http://127.0.0.1:PORT/logwindow -o out.png")
		return 2
	}
	var html string
	if htmlFile != "" {
		b, err := os.ReadFile(htmlFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		html = string(b)
		if out == "" {
			out = strings.TrimSuffix(filepath.Base(htmlFile), filepath.Ext(htmlFile)) + ".png"
		}
	} else {
		app, err := loader.LoadDir(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		rt := qrt.New(app)
		html = server.Page(rt, render.Render(rt).HTML, 0)
		if out == "" {
			out = strings.TrimRight(filepath.Base(in), "/") + ".png"
		}
	}
	if !runShot(html, w, h, out) {
		fmt.Fprintln(os.Stderr, "error: snapshot failed (needs a macOS GUI session)")
		return 1
	}
	fmt.Printf("wrote %s (%dx%d)\n", out, w, h)
	return 0
}
