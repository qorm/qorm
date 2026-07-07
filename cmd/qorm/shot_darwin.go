//go:build darwin && desktop

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

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
        [wv loadHTMLString:[NSString stringWithUTF8String:html] baseURL:nil];

        // let it load, lay out, and paint
        NSDate *loaded = [NSDate dateWithTimeIntervalSinceNow:1.6];
        while ([loaded timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];

        qormFreezeAnimations(wv);

        __block int ok = 0, done = 0;
        WKSnapshotConfiguration *sc = [[WKSnapshotConfiguration alloc] init];
        [wv takeSnapshotWithConfiguration:sc completionHandler:^(NSImage *img, NSError *err) {
            if (img) {
                CGImageRef cg = [img CGImageForProposedRect:NULL context:nil hints:nil];
                if (cg) {
                    NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:cg];
                    NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                    if ([png writeToFile:[NSString stringWithUTF8String:out] atomically:YES]) ok = 1;
                }
            }
            done = 1;
        }];
        NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:6.0];
        while (!done && [deadline timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
        return ok;
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
        // A hidden window throttles JS timers/fetch, so /logwindow would never
        // poll /log; show it (without stealing key focus) so its JS runs.
        [win orderFrontRegardless];
        NSURL *u = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        [wv loadRequest:[NSURLRequest requestWithURL:u]];

        NSDate *loaded = [NSDate dateWithTimeIntervalSinceNow:3.0];
        while ([loaded timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];

        qormFreezeAnimations(wv);

        __block int ok = 0, done = 0;
        WKSnapshotConfiguration *sc = [[WKSnapshotConfiguration alloc] init];
        [wv takeSnapshotWithConfiguration:sc completionHandler:^(NSImage *img, NSError *err) {
            if (img) {
                CGImageRef cg = [img CGImageForProposedRect:NULL context:nil hints:nil];
                if (cg) {
                    NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:cg];
                    NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                    if ([png writeToFile:[NSString stringWithUTF8String:out] atomically:YES]) ok = 1;
                }
            }
            done = 1;
        }];
        NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:6.0];
        while (!done && [deadline timeIntervalSinceNow] > 0)
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                                     beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
        return ok;
    }
}
*/
import "C"

import (
	"fmt"
	"os"
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

// cmdShot renders a QORM app to a PNG via an offscreen WebKit WebView.
func cmdShot(args []string) int {
	in, out, htmlFile, urlArg, w, h := "", "", "", "", 440, 720
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
