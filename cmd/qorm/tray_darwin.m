//go:build darwin && desktop
#import <Cocoa/Cocoa.h>
#import <math.h>
#import <WebKit/WebKit.h>
#import <Security/Security.h>
#import <IOKit/pwr_mgt/IOPMLib.h>
#import <AVFoundation/AVFoundation.h>
#include "_cgo_export.h"

@interface QormTrayTarget : NSObject
@end
@implementation QormTrayTarget
- (void)onClick:(NSMenuItem *)sender { qormTrayClicked((int)sender.tag); }
@end

static QormTrayTarget *gTarget;
static NSStatusItem *gItem;

void qormRunTray(const unsigned char *png, int pngLen, const char **items, int n, const char *tip) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        gTarget = [[QormTrayTarget alloc] init];
        gItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (pngLen > 0) {
            NSData *d = [NSData dataWithBytes:png length:pngLen];
            NSImage *img = [[NSImage alloc] initWithData:d];
            [img setTemplate:YES];
            [img setSize:NSMakeSize(18, 18)];
            gItem.button.image = img;
        } else {
            gItem.button.title = @"Q";
        }
        if (tip) gItem.button.toolTip = [NSString stringWithUTF8String:tip];
        NSMenu *menu = [[NSMenu alloc] init];
        for (int i = 0; i < n; i++) {
            NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:items[i]]
                                                        action:@selector(onClick:)
                                                 keyEquivalent:@""];
            mi.tag = i;
            mi.target = gTarget;
            [menu addItem:mi];
        }
        gItem.menu = menu;
        [NSApp run];
    }
}

void qormSetDockIcon(const unsigned char *png, int len) {
    @autoreleasepool {
        // macOS 26 ABORTS (SIGBUS 0xbad4007) on -[NSApp setApplicationIconImage:]
        // when the process is a bare CLI tool with no bundle (older macOS tolerated
        // it). `qorm run --app` is exactly that. The Dock icon is cosmetic and a
        // packaged .app has its own icon + a bundle id, so only set it when we ARE a
        // bundle; otherwise skip — the window still opens, just with a generic icon.
        // This check touches no GUI/WindowServer state, so it can't itself abort.
        if ([[NSBundle mainBundle] bundleIdentifier] == nil) return;
        [NSApplication sharedApplication];
        NSData *d = [NSData dataWithBytes:png length:len];
        NSImage *img = [[NSImage alloc] initWithData:d];
        if (img) [NSApp setApplicationIconImage:img];
    }
}

// ---- JSON-driven menus (system menu bar + tray), with SF-Symbol icons,
// submenus, and cmd/shift/alt/ctrl shortcuts. Selecting a custom item fires
// qormMenuClicked(id), which the Go side turns into qormEmit('menu'|'tray', id).
@interface QormMenuTarget : NSObject
@end
@implementation QormMenuTarget
- (void)onSelect:(NSMenuItem *)sender {
    NSString *mid = sender.representedObject;
    if (mid) qormMenuClicked((char *)[mid UTF8String]);
}
@end
static QormMenuTarget *gMenuTarget;

// parse "cmd+shift+s" → key "s" + modifier mask
static NSString *qormParseShortcut(NSString *sc, NSUInteger *mods) {
    *mods = 0;
    if (!sc || !sc.length) return @"";
    NSArray *parts = [[sc lowercaseString] componentsSeparatedByString:@"+"];
    NSString *key = @"";
    for (NSString *p in parts) {
        NSString *t = [p stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceCharacterSet]];
        if ([t isEqualToString:@"cmd"] || [t isEqualToString:@"command"] || [t isEqualToString:@"meta"]) *mods |= NSEventModifierFlagCommand;
        else if ([t isEqualToString:@"shift"]) *mods |= NSEventModifierFlagShift;
        else if ([t isEqualToString:@"alt"] || [t isEqualToString:@"option"]) *mods |= NSEventModifierFlagOption;
        else if ([t isEqualToString:@"ctrl"] || [t isEqualToString:@"control"]) *mods |= NSEventModifierFlagControl;
        else key = t;
    }
    return key;
}

static void qormAddItemsT(NSMenu *menu, NSArray *items, id target, SEL action) {
    if (![items isKindOfClass:[NSArray class]]) return;
    for (NSDictionary *it in items) {
        if (![it isKindOfClass:[NSDictionary class]]) continue;
        if ([it[@"separator"] boolValue]) { [menu addItem:[NSMenuItem separatorItem]]; continue; }
        NSString *title = it[@"title"] ?: @"";
        NSUInteger mods = 0;
        NSString *key = qormParseShortcut(it[@"shortcut"], &mods);
        NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
        if (mods) mi.keyEquivalentModifierMask = mods;
        mi.target = target;
        mi.representedObject = it[@"id"];
        NSString *icon = it[@"icon"];
        if ([icon isKindOfClass:[NSString class]] && icon.length) {
            if (@available(macOS 11.0, *)) {
                NSImage *im = [NSImage imageWithSystemSymbolName:icon accessibilityDescription:nil];
                if (im) mi.image = im;
            }
        }
        NSArray *sub = it[@"items"];
        if ([sub isKindOfClass:[NSArray class]] && [sub count]) {
            NSMenu *submenu = [[NSMenu alloc] initWithTitle:title];
            qormAddItemsT(submenu, sub, target, action);
            [mi setSubmenu:submenu];
            mi.action = nil; // a parent that only opens its submenu
        }
        [menu addItem:mi];
    }
}
static void qormAddItems(NSMenu *menu, NSArray *items) {
    qormAddItemsT(menu, items, gMenuTarget, @selector(onSelect:));
}

// Tray target: selecting a tray item calls qormTraySelected(id) on the Go side.
@interface QormTraySel : NSObject
@end
@implementation QormTraySel
- (void)onTraySelect:(NSMenuItem *)sender {
    NSString *mid = sender.representedObject;
    if (mid) qormTraySelected((char *)[mid UTF8String]);
}
@end
static QormTraySel *gTraySel;

// qormRunTrayJSON builds the tray from a JSON menu (icons + submenus) and runs
// the Cocoa loop. Selecting an item routes through qormTraySelected(id).
void qormRunTrayJSON(const unsigned char *png, int pngLen, const char *menuJSON, const char *tip) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        if (!gTraySel) gTraySel = [[QormTraySel alloc] init];
        gItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (pngLen > 0) {
            NSImage *img = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:pngLen]];
            [img setTemplate:YES];
            [img setSize:NSMakeSize(18, 18)];
            gItem.button.image = img;
        } else {
            gItem.button.title = @"Q";
        }
        if (tip) gItem.button.toolTip = [NSString stringWithUTF8String:tip];
        NSMenu *menu = [[NSMenu alloc] init];
        if (menuJSON && menuJSON[0]) {
            NSData *jd = [[NSString stringWithUTF8String:menuJSON] dataUsingEncoding:NSUTF8StringEncoding];
            NSDictionary *tray = [NSJSONSerialization JSONObjectWithData:jd options:0 error:nil];
            if ([tray isKindOfClass:[NSDictionary class]]) qormAddItemsT(menu, tray[@"items"], gTraySel, @selector(onTraySelect:));
        }
        gItem.menu = menu;
        [NSApp run];
    }
}

void qormSetAppMenu(const char *appName, const char *menuJSON) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        if (!gMenuTarget) gMenuTarget = [[QormMenuTarget alloc] init];
        NSString *name = [NSString stringWithUTF8String:appName];
        NSMenu *mainMenu = [[NSMenu alloc] init];

        NSMenuItem *appItem = [[NSMenuItem alloc] init];
        [mainMenu addItem:appItem];
        NSMenu *appMenu = [[NSMenu alloc] init];
        [appMenu addItemWithTitle:[@"About " stringByAppendingString:name] action:@selector(orderFrontStandardAboutPanel:) keyEquivalent:@""];
        [appMenu addItem:[NSMenuItem separatorItem]];
        [appMenu addItemWithTitle:[@"Hide " stringByAppendingString:name] action:@selector(hide:) keyEquivalent:@"h"];
        [appMenu addItem:[NSMenuItem separatorItem]];
        [appMenu addItemWithTitle:[@"Quit " stringByAppendingString:name] action:@selector(terminate:) keyEquivalent:@"q"];
        [appItem setSubmenu:appMenu];

        // App-defined groups (platforms.desktop.menu) sit between App and Edit.
        if (menuJSON && menuJSON[0]) {
            NSData *jd = [[NSString stringWithUTF8String:menuJSON] dataUsingEncoding:NSUTF8StringEncoding];
            NSArray *groups = [NSJSONSerialization JSONObjectWithData:jd options:0 error:nil];
            if ([groups isKindOfClass:[NSArray class]]) {
                for (NSDictionary *g in groups) {
                    if (![g isKindOfClass:[NSDictionary class]]) continue;
                    NSString *gt = g[@"title"] ?: @"";
                    NSMenuItem *gi = [[NSMenuItem alloc] init];
                    [mainMenu addItem:gi];
                    NSMenu *gm = [[NSMenu alloc] initWithTitle:gt];
                    qormAddItems(gm, g[@"items"]);
                    [gi setSubmenu:gm];
                }
            }
        }

        NSMenuItem *editItem = [[NSMenuItem alloc] init];
        [mainMenu addItem:editItem];
        NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
        [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
        [editMenu addItem:[NSMenuItem separatorItem]];
        [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
        [editItem setSubmenu:editMenu];

        NSMenuItem *winItem = [[NSMenuItem alloc] init];
        [mainMenu addItem:winItem];
        NSMenu *winMenu = [[NSMenu alloc] initWithTitle:@"Window"];
        [winMenu addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
        [winMenu addItemWithTitle:@"Zoom" action:@selector(performZoom:) keyEquivalent:@""];
        [winItem setSubmenu:winMenu];
        [NSApp setWindowsMenu:winMenu];

        [NSApp setMainMenu:mainMenu];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [NSApp activateIgnoringOtherApps:YES];
    }
}

void qormSetBadge(const char *label) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        NSString *s = (label && label[0]) ? [NSString stringWithUTF8String:label] : nil;
        [[NSApp dockTile] setBadgeLabel:s];
    }
}

#import <ServiceManagement/ServiceManagement.h>

int qormSetLoginItem(int enabled) {
    if (@available(macOS 13.0, *)) {
        NSError *err = nil;
        BOOL ok;
        if (enabled) ok = [SMAppService.mainAppService registerAndReturnError:&err];
        else ok = [SMAppService.mainAppService unregisterAndReturnError:&err];
        return (ok && !err) ? 1 : 0;
    }
    return -1;
}

int qormLoginItemEnabled(void) {
    if (@available(macOS 13.0, *)) {
        return (SMAppService.mainAppService.status == SMAppServiceStatusEnabled) ? 1 : 0;
    }
    return -1;
}

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
@interface QormNotifDelegate : NSObject <NSUserNotificationCenterDelegate>
@end
@implementation QormNotifDelegate
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)c shouldPresentNotification:(NSUserNotification *)n { return YES; }
- (void)userNotificationCenter:(NSUserNotificationCenter *)c didActivateNotification:(NSUserNotification *)n {
    goNotifyClicked((char *)[(n.identifier ?: @"") UTF8String]);
}
@end
static QormNotifDelegate *gNotifDelegate;

void qormNotify(const char *title, const char *body, const char *ident) {
    @autoreleasepool {
        NSUserNotificationCenter *c = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (!gNotifDelegate) { gNotifDelegate = [[QormNotifDelegate alloc] init]; c.delegate = gNotifDelegate; }
        NSUserNotification *n = [[NSUserNotification alloc] init];
        n.title = [NSString stringWithUTF8String:title];
        n.informativeText = [NSString stringWithUTF8String:body];
        n.identifier = [NSString stringWithUTF8String:ident];
        n.soundName = NSUserNotificationDefaultSoundName;
        [c deliverNotification:n];
    }
}
#pragma clang diagnostic pop

// qormCenterWindow centers the app window on the screen under the mouse cursor
// (natural multi-monitor behavior: the app opens where you're looking).
void qormCenterWindow(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        NSWindow *win = app.keyWindow ?: app.windows.firstObject;
        if (!win) return;
        NSPoint mouse = [NSEvent mouseLocation];
        NSScreen *target = [NSScreen mainScreen];
        for (NSScreen *s in [NSScreen screens]) {
            if (NSPointInRect(mouse, s.frame)) { target = s; break; }
        }
        NSRect sf = target.visibleFrame, wf = win.frame;
        CGFloat x = sf.origin.x + (sf.size.width - wf.size.width) / 2;
        CGFloat y = sf.origin.y + (sf.size.height - wf.size.height) / 2;
        [win setFrameOrigin:NSMakePoint(x, y)];
    }
}

// qormScreenInfo returns the displays as a JSON array [{w,h,scale,main}].
const char *qormScreenInfo(void) {
    @autoreleasepool {
        NSMutableArray *arr = [NSMutableArray array];
        NSScreen *mainScreen = [NSScreen mainScreen];
        for (NSScreen *s in [NSScreen screens]) {
            NSRect f = s.frame;
            [arr addObject:[NSString stringWithFormat:@"{\"w\":%d,\"h\":%d,\"scale\":%g,\"main\":%@}",
                (int)f.size.width, (int)f.size.height, s.backingScaleFactor, (s == mainScreen ? @"true" : @"false")]];
        }
        NSString *json = [NSString stringWithFormat:@"[%@]", [arr componentsJoinedByString:@","]];
        return strdup([json UTF8String]);
    }
}

static BOOL qormFrameOnScreen(NSRect f) {
    for (NSScreen *s in [NSScreen screens]) {
        if (NSIntersectsRect(f, s.frame)) return YES;
    }
    return NO;
}

// qormWindowFrame returns the app window frame as "x,y,w,h" (Cocoa coords).
const char *qormWindowFrame(void) {
    @autoreleasepool {
        NSWindow *win = [NSApp keyWindow] ?: [NSApp windows].firstObject;
        if (!win) return strdup("");
        NSRect f = win.frame;
        return strdup([[NSString stringWithFormat:@"%d,%d,%d,%d",
            (int)f.origin.x, (int)f.origin.y, (int)f.size.width, (int)f.size.height] UTF8String]);
    }
}

// qormSetWindowFrame restores a saved frame, but only if it still lands on a
// connected screen (else it centers — handles an unplugged second monitor).
void qormSetWindowFrame(int x, int y, int w, int h) {
    @autoreleasepool {
        NSWindow *win = [NSApp keyWindow] ?: [NSApp windows].firstObject;
        if (!win) return;
        NSRect f = NSMakeRect(x, y, w, h);
        if (qormFrameOnScreen(f)) { [win setFrame:f display:YES]; }
        else { qormCenterWindow(); }
    }
}

#import <WebKit/WebKit.h>

@interface QormWebUIDelegate : NSObject <WKUIDelegate>
@end
@implementation QormWebUIDelegate
- (void)webView:(WKWebView *)webView
    requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin
                           initiatedByFrame:(WKFrameInfo *)frame
                                       type:(WKMediaCaptureType)type
                            decisionHandler:(void (^)(WKPermissionDecision))decisionHandler
    API_AVAILABLE(macos(12.0)) {
    decisionHandler(WKPermissionDecisionGrant);
}
@end
static QormWebUIDelegate *gWebUIDelegate;

static WKWebView *qormFindWebView(NSView *v) {
    if ([v isKindOfClass:[WKWebView class]]) return (WKWebView *)v;
    for (NSView *sub in v.subviews) {
        WKWebView *found = qormFindWebView(sub);
        if (found) return found;
    }
    return nil;
}

void qormGrantMedia(void *window) {
    @autoreleasepool {
        NSWindow *win = (__bridge NSWindow *)window;
        WKWebView *webView = qormFindWebView(win.contentView);
        if (@available(macOS 12.0, *)) {
            if (webView) {
                if (!gWebUIDelegate) gWebUIDelegate = [[QormWebUIDelegate alloc] init];
                webView.UIDelegate = gWebUIDelegate;
            }
        }
    }
}

#import <LocalAuthentication/LocalAuthentication.h>

// gLAContext keeps the LAContext alive for the whole async evaluation — a local
// var would be released the instant qormBiometric returns, cancelling the
// in-flight authentication so the reply comes back as failure even after a
// correct Touch ID.
static LAContext *gLAContext;
void qormBiometric(void) {
    @autoreleasepool {
        // Retain the context for the whole async evaluation (a local would be
        // released on return, cancelling the in-flight auth).
        gLAContext = [[LAContext alloc] init];
        LAContext *ctx = gLAContext;
        NSError *err = nil;
        // DeviceOwnerAuthentication (biometrics OR device password) — the
        // biometrics-only policy hangs without ever firing its reply on this
        // macOS; this variant completes and adds a password fallback.
        if ([ctx canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&err]) {
            [NSApp activateIgnoringOtherApps:YES]; // the system cancels for a background app
            [ctx evaluatePolicy:LAPolicyDeviceOwnerAuthentication
                localizedReason:@"Authenticate to continue"
                          reply:^(BOOL success, NSError *e) {
                const char *m = success ? "Touch ID" : [(e ? e.localizedDescription : @"failed") UTF8String];
                goBiometricResult(success ? 1 : 0, (char *)m);
            }];
        } else {
            goBiometricResult(0, (char *)[(err ? err.localizedDescription : @"biometrics unavailable") UTF8String]);
        }
    }
}

#import <CoreWLAN/CoreWLAN.h>

const char *qormWifiInfo(void) {
    @autoreleasepool {
        CWInterface *iface = [[CWWiFiClient sharedWiFiClient] interface];
        NSString *json;
        if (iface && iface.ssid) {
            json = [NSString stringWithFormat:@"{\"ssid\":\"%@\",\"rssi\":%ld}", iface.ssid, (long)iface.rssiValue];
        } else {
            json = @"{\"error\":\"Wi-Fi SSID unavailable (grant Location access)\"}";
        }
        return strdup([json UTF8String]);
    }
}

#import <CoreBluetooth/CoreBluetooth.h>

@interface QormBTScanner : NSObject <CBCentralManagerDelegate>
@property (strong) CBCentralManager *central;
@property (strong) NSMutableDictionary *found;
@property BOOL scanning;
@end
@implementation QormBTScanner
- (void)centralManagerDidUpdateState:(CBCentralManager *)c {
    goBluetoothState(c.state == CBManagerStatePoweredOn ? 1 : 0);
    if (self.scanning && c.state == CBManagerStatePoweredOn) {
        [c scanForPeripheralsWithServices:nil options:nil];
    }
}
- (void)centralManager:(CBCentralManager *)c didDiscoverPeripheral:(CBPeripheral *)p
     advertisementData:(NSDictionary *)ad RSSI:(NSNumber *)rssi {
    NSString *name = p.name ?: @"";
    self.found[p.identifier.UUIDString] =
        [NSString stringWithFormat:@"{\"name\":\"%@\",\"rssi\":%@}", name, rssi];
}
- (void)reportScan {
    [self.central stopScan];
    self.scanning = NO;
    NSString *json = [NSString stringWithFormat:@"[%@]", [self.found.allValues componentsJoinedByString:@","]];
    goBluetoothScan((char *)[json UTF8String]);
}
@end
static QormBTScanner *gBT;
static void qormBTInit(void) {
    if (!gBT) { gBT = [[QormBTScanner alloc] init]; gBT.found = [NSMutableDictionary dictionary]; }
    if (!gBT.central) gBT.central = [[CBCentralManager alloc] initWithDelegate:gBT queue:nil];
}
void qormBluetoothScan(void) {
    qormBTInit();
    [gBT.found removeAllObjects];
    gBT.scanning = YES;
    if (gBT.central.state == CBManagerStatePoweredOn) [gBT.central scanForPeripheralsWithServices:nil options:nil];
    [gBT performSelector:@selector(reportScan) withObject:nil afterDelay:5.0];
}
void qormBluetoothState(void) {
    qormBTInit();
    if (gBT.central.state != CBManagerStateUnknown)
        goBluetoothState(gBT.central.state == CBManagerStatePoweredOn ? 1 : 0);
}

void qormDisableRestore(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        for (NSWindow *w in [NSApp windows]) { w.restorable = NO; }
    }
}

void qormFixWindow(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        for (NSWindow *w in [NSApp windows]) {
            if (!w.isVisible) continue;
            w.movable = NO;
            NSSize s = w.frame.size;
            w.minSize = s;
            w.maxSize = s;
        }
    }
}

void qormMoveWindow(int x, int y, int w, int h) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        NSWindow *win = nil;
        for (NSWindow *cand in [NSApp windows]) { if (cand.isVisible) { win = cand; break; } }
        if (!win) return;
        [win setFrame:NSMakeRect(x, y, w, h) display:YES animate:NO];
    }
}

// ---- display brightness via the private DisplayServices framework (dlopen'd so
// there is no link-time dependency; works on Apple Silicon where IOKit doesn't) ----
#include <dlfcn.h>
typedef int (*QormDSGet)(CGDirectDisplayID, float *);
typedef int (*QormDSSet)(CGDirectDisplayID, float);
static void *qormDSHandle(void) {
    static void *h = NULL; static int tried = 0;
    if (!tried) { tried = 1; h = dlopen("/System/Library/PrivateFrameworks/DisplayServices.framework/DisplayServices", RTLD_LAZY); }
    return h;
}
double qormGetBrightness(void) {
    void *h = qormDSHandle(); if (!h) return -1;
    QormDSGet fn = (QormDSGet)dlsym(h, "DisplayServicesGetBrightness"); if (!fn) return -1;
    float b = -1; if (fn(CGMainDisplayID(), &b) != 0) return -1; return (double)b;
}
int qormSetBrightness(double v) {
    void *h = qormDSHandle(); if (!h) return -1;
    QormDSSet fn = (QormDSSet)dlsym(h, "DisplayServicesSetBrightness"); if (!fn) return -2;
    if (v < 0) v = 0; if (v > 1) v = 1;
    return fn(CGMainDisplayID(), (float)v);
}

// ---- native macOS share sheet (NSSharingServicePicker) ----
void qormShareText(const char* text) {
    NSString *s = [NSString stringWithUTF8String:(text ? text : "")];
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSSharingServicePicker *picker = [[NSSharingServicePicker alloc] initWithItems:@[s]];
            NSWindow *win = [NSApp keyWindow];
            if (!win) win = [[NSApp windows] firstObject];
            if (!win) return;
            NSView *v = win.contentView;
            NSRect anchor = NSMakeRect(v.bounds.size.width/2.0, v.bounds.size.height - 8, 1, 1);
            [picker showRelativeToRect:anchor ofView:v preferredEdge:NSRectEdgeMinY];
        }
    });
}

// ---- real-time system volume listener: fires the instant the volume changes
// (hardware keys, menu bar, anywhere), pushing the new level to the UI ----
#import <CoreAudio/CoreAudio.h>
#import <AudioToolbox/AudioServices.h>
static AudioObjectID qormOutDev(void) {
    AudioObjectID d = kAudioObjectUnknown; UInt32 sz = sizeof(d);
    AudioObjectPropertyAddress a = { kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain };
    AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &sz, &d);
    return d;
}
static AudioObjectPropertyAddress qormVolAddr(void) {
    AudioObjectPropertyAddress a = { kAudioHardwareServiceDeviceProperty_VirtualMainVolume, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain };
    return a;
}
static OSStatus qormVolCB(AudioObjectID obj, UInt32 n, const AudioObjectPropertyAddress addrs[], void *ctx) {
    Float32 v = 0; UInt32 sz = sizeof(v); AudioObjectPropertyAddress a = qormVolAddr();
    if (AudioObjectGetPropertyData(obj, &a, 0, NULL, &sz, &v) == noErr) goVolumeChanged((double)v);
    return noErr;
}
static AudioObjectPropertyAddress qormMuteAddr(void) {
    AudioObjectPropertyAddress a = { kAudioDevicePropertyMute, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain };
    return a;
}
static OSStatus qormMuteCB(AudioObjectID obj, UInt32 n, const AudioObjectPropertyAddress addrs[], void *ctx) {
    UInt32 m = 0; UInt32 sz = sizeof(m); AudioObjectPropertyAddress a = qormMuteAddr();
    if (AudioObjectGetPropertyData(obj, &a, 0, NULL, &sz, &m) == noErr) goMuteChanged((int)(m ? 1 : 0));
    return noErr;
}
int qormReadMute(void) {
    AudioObjectID d = qormOutDev(); if (d == kAudioObjectUnknown) return -1;
    UInt32 m = 0; UInt32 sz = sizeof(m); AudioObjectPropertyAddress a = qormMuteAddr();
    if (AudioObjectHasProperty(d, &a) && AudioObjectGetPropertyData(d, &a, 0, NULL, &sz, &m) == noErr) return m ? 1 : 0;
    return -1;
}
void qormWatchVolume(void) {
    AudioObjectID d = qormOutDev(); if (d == kAudioObjectUnknown) return;
    AudioObjectPropertyAddress a = qormVolAddr();
    AudioObjectAddPropertyListener(d, &a, qormVolCB, NULL);
    AudioObjectPropertyAddress mu = qormMuteAddr();
    AudioObjectAddPropertyListener(d, &mu, qormMuteCB, NULL);
}

// ---- system modes: low-power + appearance (dark) ----
const char* qormSystemModes(void) {
    BOOL low = NO;
    if (@available(macOS 12.0, *)) low = [[NSProcessInfo processInfo] isLowPowerModeEnabled];
    NSString *best = [[NSApp effectiveAppearance] bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
    BOOL dark = [best isEqualToString:NSAppearanceNameDarkAqua];
    NSString *json = [NSString stringWithFormat:@"{\"lowPower\":%@,\"darkMode\":%@,\"airplane\":null,\"dnd\":null}", low?@"true":@"false", dark?@"true":@"false"];
    return strdup([json UTF8String]);
}

// ---- real-time brightness listener (private DisplayServices notification).
// The callback reads ONLY the display id (ARM64 arg0 in x0, safe regardless of
// how many trailing args the private API actually passes) then re-reads the
// current level itself — no dependence on the notification payload's layout. ----
static void qormBrightCB(CGDirectDisplayID d, CFStringRef notif, void *info, CFDictionaryRef data) {
    void *h = qormDSHandle(); if (!h) return;
    QormDSGet g = (QormDSGet)dlsym(h, "DisplayServicesGetBrightness"); if (!g) return;
    float b = -1; if (g(d, &b) == 0 && b >= 0) goBrightnessChanged((double)b);
}
void qormWatchBrightness(void) {
    void *h = qormDSHandle(); if (!h) return;
    typedef int (*RegFn)(CGDirectDisplayID, uint64_t, void *);
    RegFn reg = (RegFn)dlsym(h, "DisplayServicesRegisterForBrightnessChangeNotifications");
    if (!reg) return;
    reg(CGMainDisplayID(), 0, (void *)qormBrightCB);
}

void qormClipboardSet(const char* text) {
    NSString *s = [NSString stringWithUTF8String:(text ? text : "")];
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            [pb declareTypes:@[NSPasteboardTypeString] owner:nil];
            [pb setString:s forType:NSPasteboardTypeString];
        }
    });
}

const char* qormClipboardGet(void) {
    __block NSString *s = @"";
    dispatch_sync(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSPasteboard *pb = [NSPasteboard generalPasteboard];
            s = [pb stringForType:NSPasteboardTypeString];
        }
    });
    return strdup(s ? [s UTF8String] : "");
}

void qormOpenURL(const char* url) {
    NSString *s = [NSString stringWithUTF8String:(url ? url : "")];
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSURL *u = [NSURL URLWithString:s];
            if (u) [[NSWorkspace sharedWorkspace] openURL:u];
        }
    });
}

const char* qormOSVersion(void) {
    @autoreleasepool {
        NSOperatingSystemVersion v = [[NSProcessInfo processInfo] operatingSystemVersion];
        NSString *ver = [NSString stringWithFormat:@"%ld.%ld.%ld", (long)v.majorVersion, (long)v.minorVersion, (long)v.patchVersion];
        return strdup([ver UTF8String]);
    }
}

static IOPMAssertionID gKeepAwakeAssertionID = 0;
void qormKeepAwake(int on) {
    if (on) {
        if (gKeepAwakeAssertionID == 0) {
            CFStringRef reason = CFSTR("QORM Keep Awake");
            IOPMAssertionCreateWithName(kIOPMAssertionTypeNoDisplaySleep, kIOPMAssertionLevelOn, reason, &gKeepAwakeAssertionID);
        }
    } else {
        if (gKeepAwakeAssertionID != 0) {
            IOPMAssertionRelease(gKeepAwakeAssertionID);
            gKeepAwakeAssertionID = 0;
        }
    }
}

// gSpeechSynth is held in a static so it stays retained for the life of the
// process — releasing an AVSpeechSynthesizer mid-speech cuts the utterance off
// (same pattern as gLAContext above).
static AVSpeechSynthesizer *gSpeechSynth;
void qormSpeak(const char* text) {
    NSString *s = [NSString stringWithUTF8String:(text ? text : "")];
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            if (!gSpeechSynth) gSpeechSynth = [[AVSpeechSynthesizer alloc] init];
            // NSSpeechSynthesizer's startSpeakingString: interrupted any
            // in-flight speech; speakUtterance: queues instead, so stop first
            // to keep the old cut-in behavior.
            [gSpeechSynth stopSpeakingAtBoundary:AVSpeechBoundaryImmediate];
            [gSpeechSynth speakUtterance:[AVSpeechUtterance speechUtteranceWithString:s]];
        }
    });
}

void qormSpeakStop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            if (gSpeechSynth) [gSpeechSynth stopSpeakingAtBoundary:AVSpeechBoundaryImmediate];
        }
    });
}

const char* qormScreenshot(void) {
    // CGDisplayCreateImage is obsoleted in macOS 15.0. Fallback to CLI screencapture.
    return strdup("");
}

float qormGetSystemVolume(void) {
    AudioObjectID d = qormOutDev();
    if (d == kAudioObjectUnknown) return -1.0;
    AudioObjectPropertyAddress a = qormVolAddr();
    Float32 v = 0.0;
    UInt32 sz = sizeof(v);
    OSStatus err = AudioObjectGetPropertyData(d, &a, 0, NULL, &sz, &v);
    return err == noErr ? (float)v : -1.0;
}

int qormSetSystemVolume(float volume) {
    AudioObjectID d = qormOutDev();
    if (d == kAudioObjectUnknown) return 0;
    AudioObjectPropertyAddress a = qormVolAddr();
    Float32 v = (Float32)volume;
    UInt32 sz = sizeof(v);
    OSStatus err = AudioObjectSetPropertyData(d, &a, 0, NULL, sz, &v);
    return err == noErr ? 1 : 0;
}

int qormSecureSet(const char* key, const char* val) {
    @autoreleasepool {
        NSString *k = [NSString stringWithUTF8String:(key ? key : "")];
        NSString *v = [NSString stringWithUTF8String:(val ? val : "")];
        NSData *vData = [v dataUsingEncoding:NSUTF8StringEncoding];
        
        NSDictionary *query = @{
            (id)kSecClass: (id)kSecClassGenericPassword,
            (id)kSecAttrAccount: k,
            (id)kSecAttrService: @"qorm"
        };
        
        SecItemDelete((__bridge CFDictionaryRef)query);
        
        NSMutableDictionary *attrs = [query mutableCopy];
        attrs[(id)kSecValueData] = vData;
        
        OSStatus status = SecItemAdd((__bridge CFDictionaryRef)attrs, NULL);
        return status == errSecSuccess ? 1 : 0;
    }
}

const char* qormSecureGet(const char* key) {
    @autoreleasepool {
        NSString *k = [NSString stringWithUTF8String:(key ? key : "")];
        NSDictionary *query = @{
            (id)kSecClass: (id)kSecClassGenericPassword,
            (id)kSecAttrAccount: k,
            (id)kSecAttrService: @"qorm",
            (id)kSecReturnData: @YES,
            (id)kSecMatchLimit: (id)kSecMatchLimitOne
        };
        
        CFTypeRef result = NULL;
        OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
        if (status == errSecSuccess && result) {
            NSData *d = (__bridge_transfer NSData *)result;
            NSString *s = [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
            return strdup(s ? [s UTF8String] : "");
        }
        return strdup("");
    }
}
