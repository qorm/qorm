//go:build desktop && linux

package main

// Linux desktop native layer, over DBus (pure Go, no cgo beyond the WebView):
//
//   - tray: exports org.kde.StatusNotifierItem + com.canonical.dbusmenu on the
//     session bus and registers with org.kde.StatusNotifierWatcher (KDE, and
//     GNOME with the AppIndicator extension). No watcher on the bus (stock
//     GNOME) → nativeTray returns and the tray subprocess exits cleanly, same
//     as the old stub behavior.
//   - notifications: org.freedesktop.Notifications.Notify with a "default"
//     action; ActionInvoked routes the click back to notifyClickHandler (the
//     same loop macOS has). No daemon → callers fall back to notify-send.
//
// The pure protocol logic (PNG→ARGB32, menu model, dbusmenu layout) lives in
// traysni.go so it stays unit-testable off Linux. GNOME/KDE on-machine
// acceptance is still pending (tracked in planning/v0.2.0-release-plan.md B1).

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	webview "github.com/qorm/qorm/internal/webview"
)

const (
	sniObjectPath  = dbus.ObjectPath("/StatusNotifierItem")
	sniInterface   = "org.kde.StatusNotifierItem"
	sniWatcherName = "org.kde.StatusNotifierWatcher"
	sniWatcherPath = dbus.ObjectPath("/StatusNotifierWatcher")
	dbusMenuPath   = dbus.ObjectPath("/MenuBar")
	dbusMenuIface  = "com.canonical.dbusmenu"
	notifyDest     = "org.freedesktop.Notifications"
	notifyPath     = dbus.ObjectPath("/org/freedesktop/Notifications")
)

// ---------------------------------------------------------------------------
// Tray: StatusNotifierItem + dbusmenu
// ---------------------------------------------------------------------------

// sniServer implements the org.kde.StatusNotifierItem methods. The item is
// menu-only (ItemIsMenu=true): hosts open the dbusmenu themselves, so the
// activation methods are deliberate no-ops.
type sniServer struct{}

func (s *sniServer) Activate(x, y int32) *dbus.Error          { return nil }
func (s *sniServer) SecondaryActivate(x, y int32) *dbus.Error { return nil }
func (s *sniServer) ContextMenu(x, y int32) *dbus.Error       { return nil }
func (s *sniServer) Scroll(delta int32, orientation string) *dbus.Error {
	return nil
}

// dbusMenuServer implements com.canonical.dbusmenu over a static trayMenu.
// The protocol has no formal spec; method shapes follow libdbusmenu /
// fyne-io/systray: GetLayout returns (revision, recursive layout tree), Event
// handles "clicked", LayoutUpdated signals a menu change (ours never changes,
// so revision stays fixed).
type dbusMenuServer struct {
	mu       sync.Mutex
	menu     *trayMenu
	onSelect func(*trayItem)
	revision uint32
}

func (s *dbusMenuServer) GetLayout(parentID, recursionDepth int32, propertyNames []string) (uint32, menuLayout, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.menu.byID[parentID]
	if !ok {
		return s.revision, menuLayout{Properties: map[string]dbus.Variant{}, Children: []dbus.Variant{}},
			dbus.NewError(dbusMenuIface+".Error.UnknownId", nil)
	}
	return s.revision, it.layoutNode(recursionDepth), nil
}

func (s *dbusMenuServer) GetGroupProperties(ids []int32, propertyNames []string) ([]menuItemProps, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.menu.groupProperties(ids), nil
}

func (s *dbusMenuServer) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.menu.byID[id]; ok {
		if v, ok := it.menuProps()[name]; ok {
			return v, nil
		}
	}
	return dbus.Variant{}, dbus.NewError(dbusMenuIface+".Error.UnknownProperty", nil)
}

// Event routes a "clicked" event to the item's handler (off the DBus dispatch
// goroutine — quit posts HTTP / exits the process).
func (s *dbusMenuServer) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}
	s.mu.Lock()
	it := s.menu.byID[id]
	fn := s.onSelect
	s.mu.Unlock()
	if it != nil && it.id != 0 && !it.separator && len(it.children) == 0 && fn != nil {
		go fn(it)
	}
	return nil
}

// menuEventEntry is one EventGroup element, signature (isvu).
type menuEventEntry struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}

func (s *dbusMenuServer) EventGroup(events []menuEventEntry) ([]int32, *dbus.Error) {
	for _, e := range events {
		s.Event(e.ID, e.EventID, e.Data, e.Timestamp)
	}
	return []int32{}, nil
}

func (s *dbusMenuServer) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

func (s *dbusMenuServer) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}

// registerSNI announces our item to the StatusNotifierWatcher. Passing the
// object path (fyne-io/systray does the same) lets the watcher pair it with
// our unique bus name; KDE and the GNOME AppIndicator extension both accept it.
func registerSNI(conn *dbus.Conn) error {
	return conn.Object(sniWatcherName, sniWatcherPath).
		Call(sniWatcherName+".RegisterStatusNotifierItem", 0, string(sniObjectPath)).Err
}

// runSNITray exports the tray objects, registers with the watcher and blocks
// for the life of the tray process (re-registering if the watcher restarts).
// Returns an error — without blocking — when the session bus or the watcher is
// unavailable, so the caller can exit gracefully.
func runSNITray(pngIcon []byte, tip string, menu *trayMenu, onSelect func(*trayItem)) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	if _, err := conn.RequestName(fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid()),
		dbus.NameFlagDoNotQueue); err != nil {
		return err
	}

	// The menu object: methods + org.freedesktop.DBus.Properties + introspection.
	ms := &dbusMenuServer{menu: menu, onSelect: onSelect, revision: 1}
	if err := conn.Export(ms, dbusMenuPath, dbusMenuIface); err != nil {
		return err
	}
	menuPropsSpec := map[string]map[string]*prop.Prop{dbusMenuIface: {
		"Version":       constProp(uint32(3)),
		"Status":        constProp("normal"),
		"TextDirection": constProp("ltr"),
		"IconThemePath": constProp(""),
	}}
	menuProps, err := prop.Export(conn, dbusMenuPath, menuPropsSpec)
	if err != nil {
		return err
	}
	exportIntrospection(conn, dbusMenuPath, introspect.Interface{
		Name: dbusMenuIface,
		Methods: []introspect.Method{
			{Name: "GetLayout", Args: []introspect.Arg{
				{Name: "parentId", Type: "i", Direction: "in"},
				{Name: "recursionDepth", Type: "i", Direction: "in"},
				{Name: "propertyNames", Type: "as", Direction: "in"},
				{Name: "revision", Type: "u", Direction: "out"},
				{Name: "layout", Type: "(ia{sv}av)", Direction: "out"},
			}},
			{Name: "GetGroupProperties", Args: []introspect.Arg{
				{Name: "ids", Type: "ai", Direction: "in"},
				{Name: "propertyNames", Type: "as", Direction: "in"},
				{Name: "properties", Type: "a(ia{sv})", Direction: "out"},
			}},
			{Name: "GetProperty", Args: []introspect.Arg{
				{Name: "id", Type: "i", Direction: "in"},
				{Name: "name", Type: "s", Direction: "in"},
				{Name: "value", Type: "v", Direction: "out"},
			}},
			{Name: "Event", Args: []introspect.Arg{
				{Name: "id", Type: "i", Direction: "in"},
				{Name: "eventId", Type: "s", Direction: "in"},
				{Name: "data", Type: "v", Direction: "in"},
				{Name: "timestamp", Type: "u", Direction: "in"},
			}},
			{Name: "EventGroup", Args: []introspect.Arg{
				{Name: "events", Type: "a(isvu)", Direction: "in"},
				{Name: "idErrors", Type: "ai", Direction: "out"},
			}},
			{Name: "AboutToShow", Args: []introspect.Arg{
				{Name: "id", Type: "i", Direction: "in"},
				{Name: "needUpdate", Type: "b", Direction: "out"},
			}},
			{Name: "AboutToShowGroup", Args: []introspect.Arg{
				{Name: "ids", Type: "ai", Direction: "in"},
				{Name: "updatesNeeded", Type: "ai", Direction: "out"},
				{Name: "idErrors", Type: "ai", Direction: "out"},
			}},
		},
		Signals: []introspect.Signal{
			{Name: "LayoutUpdated", Args: []introspect.Arg{
				{Name: "revision", Type: "u"}, {Name: "parent", Type: "i"},
			}},
			{Name: "ItemsPropertiesUpdated", Args: []introspect.Arg{
				{Name: "updatedProps", Type: "a(ia{sv})"}, {Name: "removedProps", Type: "a(ias)"},
			}},
		},
		Properties: menuProps.Introspection(dbusMenuIface),
	})

	// The StatusNotifierItem object.
	if err := conn.Export(&sniServer{}, sniObjectPath, sniInterface); err != nil {
		return err
	}
	icons := []sniPixmap{}
	if px, err := pngToARGB32(pngIcon); err == nil {
		icons = append(icons, px)
	}
	sniPropsSpec := map[string]map[string]*prop.Prop{sniInterface: {
		"Category":            constProp("ApplicationStatus"),
		"Id":                  constProp("qorm"),
		"Title":               constProp(tip),
		"Status":              constProp("Active"),
		"WindowId":            constProp(uint32(0)),
		"IconName":            constProp(""),
		"IconPixmap":          constProp(icons),
		"OverlayIconName":     constProp(""),
		"OverlayIconPixmap":   constProp([]sniPixmap{}),
		"AttentionIconName":   constProp(""),
		"AttentionIconPixmap": constProp([]sniPixmap{}),
		"AttentionMovieName":  constProp(""),
		"ToolTip":             constProp(sniToolTip{Title: tip, IconPixmaps: []sniPixmap{}}),
		"ItemIsMenu":          constProp(true),
		"Menu":                constProp(dbusMenuPath),
	}}
	sniProps, err := prop.Export(conn, sniObjectPath, sniPropsSpec)
	if err != nil {
		return err
	}
	exportIntrospection(conn, sniObjectPath, introspect.Interface{
		Name: sniInterface,
		Methods: []introspect.Method{
			{Name: "Activate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
			{Name: "SecondaryActivate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
			{Name: "ContextMenu", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
			{Name: "Scroll", Args: []introspect.Arg{{Name: "delta", Type: "i", Direction: "in"}, {Name: "orientation", Type: "s", Direction: "in"}}},
		},
		Signals: []introspect.Signal{
			{Name: "NewIcon"}, {Name: "NewTitle"}, {Name: "NewToolTip"},
			{Name: "NewStatus", Args: []introspect.Arg{{Name: "status", Type: "s"}}},
		},
		Properties: sniProps.Introspection(sniInterface),
	})

	// No watcher on the bus (stock GNOME without the AppIndicator extension) →
	// graceful exit, matching the old stub semantics.
	if err := registerSNI(conn); err != nil {
		return err
	}

	// Re-register when the watcher restarts (host crash, plasmashell reload),
	// then block for the life of the tray process. If the signal watch cannot
	// be installed, just block — the parent-death watchdog in runTray exits us.
	if err := conn.AddMatchSignal(
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, sniWatcherName),
	); err == nil {
		ch := make(chan *dbus.Signal, 16)
		conn.Signal(ch)
		for sig := range ch {
			if sig.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(sig.Body) == 3 {
				if newOwner, _ := sig.Body[2].(string); newOwner != "" {
					registerSNI(conn)
				}
			}
		}
		return nil
	}
	select {}
}

func constProp(v interface{}) *prop.Prop {
	return &prop.Prop{Value: v, Emit: prop.EmitConst}
}

// exportIntrospection publishes org.freedesktop.DBus.Introspectable for one of
// our objects (some hosts introspect before calling).
func exportIntrospection(conn *dbus.Conn, path dbus.ObjectPath, iface introspect.Interface) {
	node := &introspect.Node{
		Name: string(path),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			iface,
		},
	}
	conn.Export(introspect.NewIntrospectable(node), path, "org.freedesktop.DBus.Introspectable")
}

// nativeTray shows the built-in tray menu; blocks until the process exits
// (runTray spawns this in the dedicated __tray subprocess). Returns without
// blocking when no StatusNotifierWatcher is available.
func nativeTray(png []byte, items []string, tip string, onClick func(int)) {
	if onClick == nil {
		return
	}
	menu := newTrayMenu(items)
	if err := runSNITray(png, tip, menu, func(it *trayItem) {
		onClick(int(it.id) - 1) // built-in menu ids are 1..n in item order
	}); err != nil {
		fmt.Fprintf(os.Stderr, "qorm: no system tray available (%v)\n", err)
	}
}

// nativeTrayJSON builds the tray from the app's JSON menu config; selections
// route through traySelected(id), exactly like the macOS tray.
func nativeTrayJSON(png []byte, menuJSON, tip string) {
	menu, err := trayMenuFromJSON(menuJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qorm: bad tray menu config (%v)\n", err)
		return
	}
	if err := runSNITray(png, tip, menu, func(it *trayItem) {
		if it.strID != "" {
			traySelected(it.strID)
		}
	}); err != nil {
		fmt.Fprintf(os.Stderr, "qorm: no system tray available (%v)\n", err)
	}
}

// ---------------------------------------------------------------------------
// Notifications: org.freedesktop.Notifications + click loop
// ---------------------------------------------------------------------------

var (
	notifyMu       sync.Mutex
	notifyPending  = map[uint32]string{} // notification id → app ident
	notifyWatching bool
)

// nativeNotify posts a desktop notification whose click routes back to
// notifyClickHandler (DBus ActionInvoked). Falls back to notify-send (no click
// loop) when no notification daemon answers on the bus.
func nativeNotify(title, body, ident string) {
	if err := dbusNotify(title, body, ident); err != nil {
		exec.Command("notify-send", title, body).Run()
	}
}

// dbusNotify sends Notify with a "default" action and remembers the returned
// id so ActionInvoked can be mapped back to the app's ident.
func dbusNotify(title, body, ident string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	notifyMu.Lock()
	if !notifyWatching {
		if err := conn.AddMatchSignal(
			dbus.WithMatchInterface(notifyDest),
			dbus.WithMatchObjectPath(notifyPath),
		); err == nil {
			ch := make(chan *dbus.Signal, 16)
			conn.Signal(ch)
			go watchNotifySignals(ch)
			notifyWatching = true
		}
	}
	notifyMu.Unlock()

	var id uint32
	if err := conn.Object(notifyDest, notifyPath).Call(notifyDest+".Notify", 0,
		"qorm",      // app_name
		uint32(0),   // replaces_id
		"",          // app_icon
		title, body, // summary, body
		[]string{"default", "Open"}, // actions: clicking the body fires "default"
		map[string]dbus.Variant{},   // hints
		int32(-1),                   // expire_timeout: daemon default
	).Store(&id); err != nil {
		return err
	}
	notifyMu.Lock()
	notifyPending[id] = ident
	notifyMu.Unlock()
	return nil
}

// watchNotifySignals routes ActionInvoked to notifyClickHandler and drops
// closed notifications from the pending map. Non-notification signals on the
// shared channel are ignored.
func watchNotifySignals(ch chan *dbus.Signal) {
	for sig := range ch {
		if sig == nil {
			continue
		}
		switch sig.Name {
		case notifyDest + ".ActionInvoked":
			if len(sig.Body) < 2 {
				continue
			}
			id, _ := sig.Body[0].(uint32)
			action, _ := sig.Body[1].(string)
			notifyMu.Lock()
			ident, ok := notifyPending[id]
			delete(notifyPending, id)
			notifyMu.Unlock()
			if ok && action == "default" && notifyClickHandler != nil {
				notifyClickHandler(ident)
			}
		case notifyDest + ".NotificationClosed":
			if len(sig.Body) < 1 {
				continue
			}
			id, _ := sig.Body[0].(uint32)
			notifyMu.Lock()
			delete(notifyPending, id)
			notifyMu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// Remaining native hooks: still stubs on Linux (same semantics tray_other.go
// had before the split; brightness/volume/clipboard/… run through the tool
// paths in desktopHardwareLinux instead).
// ---------------------------------------------------------------------------

func setDockIcon(png []byte) {}

func setAppMenu(appName, menuJSON string) {}

func setDockBadge(label string) {}

func setLoginItem(enabled bool) bool { return false }
func loginItemEnabled() bool         { return false }

func centerWindow()      {}
func screenInfo() string { return "[]" }

func windowFrame() string           { return "" }
func setWindowFrame(x, y, w, h int) {}

func grantMedia(window unsafe.Pointer) {}

func nativeBiometric() {}

func wifiDesktopInfo() string { return "{\"error\":\"not supported\"}" }

func nativeBluetoothScan()  {}
func nativeBluetoothState() {}

func disableRestore() {}

func fixWindow()                    {}
func moveMainWindow(x, y, w, h int) {}

func nativeBrightnessGet() (float64, bool) { return 0, false }
func nativeBrightnessSet(v float64) int    { return -1 }

func nativeShare(text string) {}

var volumeWatchHandler func(float64)

func nativeWatchVolume() {}

var muteWatchHandler func(bool)

func nativeReadMute() int { return -1 }

func nativeSystemModes() string { return "{}" }

var brightnessWatchHandler func(float64)

func nativeWatchBrightness() {}

var shortcutHandler func(string)

func nativeSetDockMenu(json string) {}

func nativeWinDragStart(id string) {
	var wv webview.WebView
	if id == "main" || id == "" {
		wv = appWebView
	} else {
		winMu.Lock()
		wv = activeWindows[id]
		winMu.Unlock()
	}
	if wv == nil {
		return
	}
	if hwnd := wv.Window(); hwnd != nil {
		startWindowDrag(hwnd)
	}
}

func nativeWinDragMove(id string, dx, dy int) {}

func nativeClipboardSet(text string) {}
func nativeClipboardGet() string     { return "" }
func nativeOpenURL(url string)       {}
func nativeOSVersion() string        { return "" }
func nativeSetKeepAwake(on bool)     {}
func nativeSpeak(text string)        {}
func nativeSpeakStop()               {}
func nativeScreenshot() string       { return "" }
