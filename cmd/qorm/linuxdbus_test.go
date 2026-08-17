//go:build desktop && linux

package main

// Runtime acceptance for the Linux DBus native layer (tray / notifications /
// Secret Service). These need a real session bus — and, for secrets, an
// unlocked keyring — so they are gated behind QORM_LINUX_DBUS_TEST=1 and skip
// everywhere else (CI compile jobs, plain `go test`). Run them with:
//
//	dbus-run-session -- sh -c '
//	  echo -n "test" | gnome-keyring-daemon --unlock --components=secrets >/dev/null
//	  QORM_LINUX_DBUS_TEST=1 go test -tags desktop -count=1 -run TestLinuxDBus -v ./cmd/qorm/'
//
// The notification daemon and StatusNotifierWatcher are mocked in-process via
// python3-gi (see the inline scripts), so the tests exercise the real DBus
// protocol without a desktop session.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	dbus "github.com/godbus/dbus/v5"
)

func requireDBusGate(t *testing.T) {
	t.Helper()
	if os.Getenv("QORM_LINUX_DBUS_TEST") != "1" {
		t.Skip("set QORM_LINUX_DBUS_TEST=1 (with a session bus) to run Linux DBus acceptance tests")
	}
}

// startMock launches an inline python3-gi DBus mock and waits for it to print
// READY. Its stdout lines are streamed to out.
func startMock(t *testing.T, script string, out chan<- string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("python3", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("mock stdout: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mock: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			if line == "READY" {
				close(ready)
				continue
			}
			select {
			case out <- line:
			default:
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("mock did not become READY")
	}
	return cmd
}

const mockCommon = `
import sys
import gi
from gi.repository import Gio, GLib
def own(name, xml, on_call):
    node = Gio.DBusNodeInfo.new_for_xml(xml)
    def on_bus(conn, _n):
        for iface in node.interfaces:
            conn.register_object(PATH, iface, on_call, None, None)
        print('READY', flush=True)
    Gio.bus_own_name(Gio.BusType.SESSION, name, Gio.BusNameOwnerFlags.NONE,
                     on_bus, None, lambda *a: sys.exit(1))
    GLib.MainLoop().run()
`

// TestLinuxDBusSecretService round-trips a value through the real Secret
// Service (gnome-keyring) — set, get, overwrite, and a missing key.
func TestLinuxDBusSecretService(t *testing.T) {
	requireDBusGate(t)
	key := fmt.Sprintf("qorm-test-%d", os.Getpid())
	if !nativeSecureSet(key, "v1") {
		t.Fatal("nativeSecureSet(v1) returned false — is gnome-keyring unlocked on this bus?")
	}
	if got := nativeSecureGet(key); got != "v1" {
		t.Fatalf("nativeSecureGet = %q, want v1", got)
	}
	if !nativeSecureSet(key, "v2") {
		t.Fatal("nativeSecureSet(v2) returned false")
	}
	if got := nativeSecureGet(key); got != "v2" {
		t.Fatalf("after overwrite nativeSecureGet = %q, want v2", got)
	}
	if got := nativeSecureGet(key + "-missing"); got != "" {
		t.Fatalf("missing key returned %q, want empty", got)
	}
}

// TestLinuxDBusNotify posts a notification against a mock daemon and verifies
// (a) the Notify call carries title/body, (b) ActionInvoked("default") routes
// back into notifyClickHandler with the app's ident.
func TestLinuxDBusNotify(t *testing.T) {
	requireDBusGate(t)
	lines := make(chan string, 8)
	startMock(t, `PATH='/org/freedesktop/Notifications'`+mockCommon+`
XML = """
<node><interface name='org.freedesktop.Notifications'>
<method name='Notify'>
 <arg type='s' direction='in'/><arg type='u' direction='in'/><arg type='s' direction='in'/>
 <arg type='s' direction='in'/><arg type='s' direction='in'/><arg type='as' direction='in'/>
 <arg type='a{sv}' direction='in'/><arg type='i' direction='in'/>
 <arg type='u' direction='out'/>
</method>
<method name='GetCapabilities'><arg type='as' direction='out'/></method>
<method name='GetServerInformation'>
 <arg type='s' direction='out'/><arg type='s' direction='out'/>
 <arg type='s' direction='out'/><arg type='s' direction='out'/>
</method>
<signal name='ActionInvoked'><arg type='u'/><arg type='s'/></signal>
</interface></node>"""
def on_call(conn, sender, path, iface, method, params, inv):
    if method == 'Notify':
        a = params.unpack()
        print('NOTIFY|' + a[3] + '|' + a[4] + '|' + ','.join(a[5]), flush=True)
        inv.return_value(GLib.Variant('(u)', (42,)))
        def fire():
            conn.emit_signal(None, PATH, 'org.freedesktop.Notifications',
                             'ActionInvoked', GLib.Variant('(us)', (42, 'default')))
            return False
        GLib.timeout_add(300, fire)
    elif method == 'GetCapabilities':
        inv.return_value(GLib.Variant('(as)', (['actions'],)))
    elif method == 'GetServerInformation':
        inv.return_value(GLib.Variant('(ssss)', ('mock', 'qorm', '1', '1.2')))
own('org.freedesktop.Notifications', XML, on_call)
`, lines)

	clicked := make(chan string, 1)
	old := notifyClickHandler
	notifyClickHandler = func(id string) { clicked <- id }
	t.Cleanup(func() { notifyClickHandler = old })

	nativeNotify("Hello", "World", "ident-7")

	select {
	case l := <-lines:
		if !strings.HasPrefix(l, "NOTIFY|Hello|World|") {
			t.Fatalf("daemon saw %q", l)
		}
		if !strings.Contains(l, "default") {
			t.Fatalf("Notify carried no default action: %q", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mock daemon never received Notify")
	}
	select {
	case id := <-clicked:
		if id != "ident-7" {
			t.Fatalf("click routed ident %q, want ident-7", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ActionInvoked did not reach notifyClickHandler")
	}
}

// TestLinuxDBusTray registers the tray against a mock StatusNotifierWatcher,
// then talks to the exported item like a shell would: reads the dbusmenu
// layout and clicks an entry.
func TestLinuxDBusTray(t *testing.T) {
	requireDBusGate(t)
	lines := make(chan string, 8)
	startMock(t, `PATH='/StatusNotifierWatcher'`+mockCommon+`
XML = """
<node><interface name='org.kde.StatusNotifierWatcher'>
<method name='RegisterStatusNotifierItem'><arg type='s' direction='in'/></method>
<property name='IsStatusNotifierHostRegistered' type='b' access='read'/>
</interface></node>"""
def on_call(conn, sender, path, iface, method, params, inv):
    if method == 'RegisterStatusNotifierItem':
        print('REGISTERED|' + sender + '|' + params.unpack()[0], flush=True)
        inv.return_value(None)
own('org.kde.StatusNotifierWatcher', XML, on_call)
`, lines)

	clicks := make(chan int, 1)
	go nativeTray(appIcon(192), []string{"Open", "Quit Test"}, "qorm test", func(i int) { clicks <- i })

	var itemService string
	select {
	case l := <-lines:
		parts := strings.Split(l, "|")
		if len(parts) != 3 || parts[0] != "REGISTERED" {
			t.Fatalf("watcher saw %q", l)
		}
		itemService = parts[1] // unique bus name of the registering connection
	case <-time.After(10 * time.Second):
		t.Fatal("tray never registered with the watcher")
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		t.Fatalf("session bus: %v", err)
	}
	menu := conn.Object(itemService, dbus.ObjectPath("/MenuBar"))
	var revision uint32
	var layout interface{}
	if err := menu.Call("com.canonical.dbusmenu.GetLayout", 0, int32(0), int32(-1), []string{}).
		Store(&revision, &layout); err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	if s := fmt.Sprintf("%v", layout); !strings.Contains(s, "Open") || !strings.Contains(s, "Quit Test") {
		t.Fatalf("layout missing menu labels: %s", s)
	}

	// Click the first entry (built-in menus map id 1 → onClick(0)).
	if err := menu.Call("com.canonical.dbusmenu.Event", 0,
		int32(1), "clicked", dbus.MakeVariant(""), uint32(0)).Err; err != nil {
		t.Fatalf("Event(clicked): %v", err)
	}
	select {
	case i := <-clicks:
		if i != 0 {
			t.Fatalf("clicked index %d, want 0", i)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("menu click never reached onClick")
	}
}
