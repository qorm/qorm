//go:build darwin && desktop
#import <Cocoa/Cocoa.h>
#import <math.h>
#import <WebKit/WebKit.h>
#include "_cgo_export.h"

@interface QormWV : NSObject <WKScriptMessageHandler, WKUIDelegate>
@property (strong) NSWindow *window;
@property (strong) WKWebView *web;
@property (copy) NSString *wid;
@end
@implementation QormWV
- (void)userContentController:(WKUserContentController *)ucc didReceiveScriptMessage:(WKScriptMessage *)msg {
    NSString *json;
    if ([msg.body isKindOfClass:[NSString class]]) {
        json = msg.body;
    } else {
        NSData *d = [NSJSONSerialization dataWithJSONObject:msg.body options:0 error:nil];
        json = d ? [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] : @"{}";
    }
    goDesktopMessage((char *)[self.wid UTF8String], (char *)[json UTF8String]);
}
- (void)webView:(WKWebView *)wv requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)o
    initiatedByFrame:(WKFrameInfo *)f type:(WKMediaCaptureType)t
    decisionHandler:(void (^)(WKPermissionDecision))dh API_AVAILABLE(macos(12.0)) { dh(WKPermissionDecisionGrant); }
@end

// ARC keeps everything in gWins alive; QormWV owns its window+web via strong refs.
static NSMutableDictionary<NSString *, QormWV *> *gWins;

void *qormWVOpen(const char *cwid, const char *title, const char *url, int w, int h, int chromeless, int transparent) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        if (!gWins) gWins = [[NSMutableDictionary alloc] init];
        NSString *wid = [NSString stringWithUTF8String:cwid];
        QormWV *v = [[QormWV alloc] init];
        v.wid = wid;
        NSRect frame = NSMakeRect(200, 200, w, h);
        NSUInteger style = chromeless
            ? (NSWindowStyleMaskBorderless | NSWindowStyleMaskResizable)
            : (NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable);
        v.window = [[NSWindow alloc] initWithContentRect:frame styleMask:style backing:NSBackingStoreBuffered defer:NO];
        v.window.title = [NSString stringWithUTF8String:title];
        v.window.releasedWhenClosed = NO;
        v.window.restorable = NO;
        if (chromeless) v.window.movableByWindowBackground = YES;
        WKWebViewConfiguration *cfg = [[WKWebViewConfiguration alloc] init];
        [cfg.userContentController addScriptMessageHandler:v name:@"qormdesktop"];
        WKUserScript *us = [[WKUserScript alloc]
            initWithSource:@"window.qormDesktop=function(j){window.webkit.messageHandlers.qormdesktop.postMessage(j);};"
            injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
        [cfg.userContentController addUserScript:us];
        if (chromeless || transparent) {
            NSString *hideCss = @"var s=document.createElement('style');s.textContent='html,body{overflow:hidden !important;background:transparent !important;margin:0}::-webkit-scrollbar{width:0;height:0;display:none}';(document.head||document.documentElement).appendChild(s);";
            WKUserScript *hus = [[WKUserScript alloc] initWithSource:hideCss injectionTime:WKUserScriptInjectionTimeAtDocumentEnd forMainFrameOnly:YES];
            [cfg.userContentController addUserScript:hus];
        }
        cfg.preferences.javaScriptCanOpenWindowsAutomatically = YES;
        v.web = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        v.web.UIDelegate = v;
        if (transparent) {
            v.window.opaque = NO;
            v.window.backgroundColor = [NSColor clearColor];
            v.window.hasShadow = chromeless ? NO : YES;
            @try { [v.web setValue:@NO forKey:@"drawsBackground"]; } @catch (id e) {}
        }
        v.window.contentView = v.web;
        gWins[wid] = v;
        NSString *u = [NSString stringWithUTF8String:url];
        dispatch_async(dispatch_get_main_queue(), ^{
            [v.web loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:u]]];
            [v.window makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
        });
        return (__bridge void *)v.window;
    }
}
void qormWVEval(const char *cwid, const char *js) {
    NSString *wid = [NSString stringWithUTF8String:cwid];
    NSString *s = [NSString stringWithUTF8String:js];
    dispatch_async(dispatch_get_main_queue(), ^{ QormWV *v = gWins[wid]; if (v) [v.web evaluateJavaScript:s completionHandler:nil]; });
}
void qormWVWake(void) { dispatch_async(dispatch_get_main_queue(), ^{ goDesktopDrain(); }); }
void qormWVMove(const char *cwid, int x, int y, int w, int h) {
    NSString *wid = [NSString stringWithUTF8String:cwid];
    dispatch_async(dispatch_get_main_queue(), ^{
        QormWV *v = gWins[wid];
        if (v) { CGFloat sh = [NSScreen mainScreen].frame.size.height; [v.window setFrame:NSMakeRect(x, sh - y - h, w, h) display:YES animate:NO]; }
    });
}
void qormWVOp(const char *cwid, const char *cop) {
    NSString *wid = [NSString stringWithUTF8String:cwid];
    NSString *op = [NSString stringWithUTF8String:cop];
    dispatch_async(dispatch_get_main_queue(), ^{
        QormWV *v = gWins[wid];
        if (!v) return;
        if ([op isEqualToString:@"focus"]) { [v.window makeKeyAndOrderFront:nil]; [NSApp activateIgnoringOtherApps:YES]; }
        else if ([op isEqualToString:@"minimize"]) [v.window miniaturize:nil];
        else if ([op isEqualToString:@"pin"]) v.window.level = NSFloatingWindowLevel;
        else if ([op isEqualToString:@"unpin"]) v.window.level = NSNormalWindowLevel;
        else if ([op isEqualToString:@"tile"]) {
            NSArray *keys = [gWins allKeys];
            int n = (int)keys.count;
            if (n == 0) return;
            NSRect vis = [NSScreen mainScreen].visibleFrame;
            int cols = (int)ceil(sqrt((double)n));
            int rows = (int)ceil((double)n / cols);
            CGFloat cw = vis.size.width / cols, ch = vis.size.height / rows;
            int i = 0;
            for (NSString *kk in keys) {
                QormWV *win = gWins[kk];
                int c = i % cols, rr = i / cols;
                [win.window setFrame:NSMakeRect(vis.origin.x + c * cw + 4, vis.origin.y + vis.size.height - (rr + 1) * ch + 4, cw - 8, ch - 8) display:YES animate:NO];
                i++;
            }
        }
        else if ([op isEqualToString:@"close"]) { [v.window close]; [gWins removeObjectForKey:wid]; }
    });
}
const char *qormWVGetFrame(const char *cwid) {
    QormWV *v = gWins[[NSString stringWithUTF8String:cwid]];
    if (!v) return strdup("");
    NSRect f = v.window.frame;
    CGFloat sh = [NSScreen mainScreen].frame.size.height;
    return strdup([[NSString stringWithFormat:@"%d,%d,%d,%d", (int)f.origin.x, (int)(sh - f.origin.y - f.size.height), (int)f.size.width, (int)f.size.height] UTF8String]);
}
const char *qormWVList(void) {
    if (!gWins || gWins.count == 0) return strdup("[]");
    NSMutableArray *ids = [NSMutableArray array];
    for (NSString *k in gWins) [ids addObject:[NSString stringWithFormat:@"\"%@\"", k]];
    return strdup([[NSString stringWithFormat:@"[%@]", [ids componentsJoinedByString:@","]] UTF8String]);
}
void qormWVRun(void) { [NSApp run]; }

// ---- Dock menu (app-icon quick actions): tapping an item fires the 'shortcut'
// event on the frontend bus via goShortcutSelected ----
@interface QormAppDelegate : NSObject <NSApplicationDelegate>
@property (strong) NSMenu *dockMenu;
@end
@implementation QormAppDelegate
- (NSMenu *)applicationDockMenu:(NSApplication *)sender { return self.dockMenu; }
- (void)qormShortcut:(NSMenuItem *)item {
    if (item.representedObject) goShortcutSelected((char *)[item.representedObject UTF8String]);
}
@end
static QormAppDelegate *gAppDel;
void qormSetDockMenu(const char *cjson) {
    NSString *json = [NSString stringWithUTF8String:cjson];
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSData *d = [json dataUsingEncoding:NSUTF8StringEncoding];
            NSArray *items = [NSJSONSerialization JSONObjectWithData:d options:0 error:nil];
            if (![items isKindOfClass:[NSArray class]] || items.count == 0) return;
            if (!gAppDel) gAppDel = [[QormAppDelegate alloc] init];
            NSMenu *menu = [[NSMenu alloc] init];
            for (id o in items) {
                if (![o isKindOfClass:[NSDictionary class]]) continue;
                NSDictionary *it = o;
                NSString *title = it[@"title"] ? it[@"title"] : it[@"id"];
                if (!title) continue;
                NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:@selector(qormShortcut:) keyEquivalent:@""];
                mi.representedObject = it[@"id"];
                mi.target = gAppDel;
                [menu addItem:mi];
            }
            gAppDel.dockMenu = menu;
            if (!NSApp.delegate) NSApp.delegate = gAppDel;
        }
    });
}

// ---- chromeless window dragging: JS drag-region reports pointer deltas; the
// window origin follows (macOS y is flipped, so subtract dy) ----
static NSPoint gQormDragOrigin;
void qormWinDragStart(const char *cwid) {
    NSString *wid = [NSString stringWithUTF8String:cwid];
    dispatch_async(dispatch_get_main_queue(), ^{
        QormWV *v = gWins[wid];
        if (v) gQormDragOrigin = v.window.frame.origin;
    });
}
void qormWinDragMove(const char *cwid, int dx, int dy) {
    NSString *wid = [NSString stringWithUTF8String:cwid];
    dispatch_async(dispatch_get_main_queue(), ^{
        QormWV *v = gWins[wid];
        if (v) [v.window setFrameOrigin:NSMakePoint(gQormDragOrigin.x + dx, gQormDragOrigin.y - dy)];
    });
}
