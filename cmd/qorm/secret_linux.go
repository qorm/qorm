//go:build desktop && linux

package main

// Linux secure storage over the DBus Secret Service (org.freedesktop.secrets:
// GNOME Keyring, KWallet ≥ 5.97, KeePassXC). Items are stored in the default
// collection with attributes {"service":"qorm","key":<key>} and label
// "qorm/<key>"; an existing item for the same attributes is replaced. When no
// Secret Service is running (or the collection stays locked), we refuse rather
// than store plaintext — nativeSecureSet returns false, nativeSecureGet "".
//
// This file also carries the window hooks winapi_other.go stubs off-Linux:
// setWindowPos / startWindowDrag (GTK path still pending) and the volume
// stubs (Linux volume runs through pactl in desktopHardwareLinux).

import (
	"errors"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
)

// setWindowPos: webview_go has no window-move API and no GTK path is wired yet.
func setWindowPos(hwnd unsafe.Pointer, x, y, w, h int) {}

// startWindowDrag: no GTK drag path wired yet.
func startWindowDrag(hwnd unsafe.Pointer) {}

// nativeVolumeGet/Set: no native master-volume API wired on Linux; the desktop
// bridge uses pactl (see desktopHardwareLinux).
func nativeVolumeGet() (float64, bool) { return 0, false }
func nativeVolumeSet(v float64) bool   { return false }

const (
	secretsDest    = "org.freedesktop.secrets"
	secretsPath    = dbus.ObjectPath("/org/freedesktop/secrets")
	secretsSvc     = "org.freedesktop.Secret.Service"
	secretsColl    = "org.freedesktop.Secret.Collection"
	secretsItem    = "org.freedesktop.Secret.Item"
	secretsSession = "org.freedesktop.Secret.Session"
	secretsPrompt  = "org.freedesktop.Secret.Prompt"
)

// dbusSecret is the Secret Service secret struct, signature (oayays).
type dbusSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// qormSecretAttrs are the lookup attributes for one of our items.
func qormSecretAttrs(key string) map[string]string {
	return map[string]string{"service": "qorm", "key": key}
}

// openSecretSession opens a plain-transfer session (the bus itself is a local
// unix socket; "plain" is what libsecret uses without a DH exchange).
func openSecretSession(conn *dbus.Conn) (dbus.ObjectPath, error) {
	var out dbus.Variant
	var session dbus.ObjectPath
	err := conn.Object(secretsDest, secretsPath).
		Call(secretsSvc+".OpenSession", 0, "plain", dbus.MakeVariant("")).
		Store(&out, &session)
	return session, err
}

func closeSecretSession(conn *dbus.Conn, session dbus.ObjectPath) {
	conn.Object(secretsDest, session).Call(secretsSession+".Close", 0)
}

// defaultCollection resolves the "default" alias to the user's collection.
func defaultCollection(conn *dbus.Conn) (dbus.ObjectPath, error) {
	var col dbus.ObjectPath
	if err := conn.Object(secretsDest, secretsPath).
		Call(secretsSvc+".ReadAlias", 0, "default").Store(&col); err != nil {
		return "", err
	}
	if col == "/" || col == "" {
		return "", errors.New("secret service has no default collection")
	}
	return col, nil
}

// objectLocked reads the Locked property of a collection or item.
func objectLocked(conn *dbus.Conn, path dbus.ObjectPath, iface string) bool {
	v, err := conn.Object(secretsDest, path).GetProperty(iface + ".Locked")
	if err != nil {
		return false
	}
	locked, _ := v.Value().(bool)
	return locked
}

// unlockObjects asks the service to unlock paths, completing a prompt (the
// keyring password dialog) when one is required.
func unlockObjects(conn *dbus.Conn, paths []dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var promptPath dbus.ObjectPath
	if err := conn.Object(secretsDest, secretsPath).
		Call(secretsSvc+".Unlock", 0, paths).Store(&unlocked, &promptPath); err != nil {
		return err
	}
	return completePrompt(conn, promptPath)
}

// completePrompt executes a Secret Service prompt ("/" means none needed) and
// waits for its Completed signal — the user typing the keyring password. A
// dismissed or timed-out prompt is an error (the caller then refuses the op).
func completePrompt(conn *dbus.Conn, path dbus.ObjectPath) error {
	if path == "/" || path == "" {
		return nil
	}
	match := []dbus.MatchOption{
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(secretsPrompt),
		dbus.WithMatchMember("Completed"),
	}
	if err := conn.AddMatchSignal(match...); err != nil {
		return err
	}
	defer conn.RemoveMatchSignal(match...)
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	if call := conn.Object(secretsDest, path).Call(secretsPrompt+".Prompt", 0, ""); call.Err != nil {
		return call.Err
	}
	timeout := time.NewTimer(2 * time.Minute) // the user may be typing a password
	defer timeout.Stop()
	for {
		select {
		case sig := <-ch:
			if sig == nil || sig.Path != path || sig.Name != secretsPrompt+".Completed" {
				continue
			}
			if len(sig.Body) > 0 {
				if dismissed, _ := sig.Body[0].(bool); dismissed {
					return errors.New("keyring unlock prompt dismissed")
				}
			}
			return nil
		case <-timeout.C:
			return errors.New("keyring unlock prompt timed out")
		}
	}
}

// nativeSecureSet stores key=val in the default keyring collection. Returns
// false — refusing, never writing plaintext — when the Secret Service is
// missing or the collection cannot be unlocked.
func nativeSecureSet(key, val string) bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	session, err := openSecretSession(conn)
	if err != nil {
		return false
	}
	defer closeSecretSession(conn, session)
	col, err := defaultCollection(conn)
	if err != nil {
		return false
	}
	if objectLocked(conn, col, secretsColl) {
		if unlockObjects(conn, []dbus.ObjectPath{col}) != nil {
			return false
		}
	}
	props := map[string]dbus.Variant{
		secretsItem + ".Label":      dbus.MakeVariant("qorm/" + key),
		secretsItem + ".Attributes": dbus.MakeVariant(qormSecretAttrs(key)),
	}
	secret := dbusSecret{
		Session:     session,
		Parameters:  []byte{},
		Value:       []byte(val),
		ContentType: "text/plain; charset=utf8",
	}
	var item, promptPath dbus.ObjectPath
	if err := conn.Object(secretsDest, col).
		Call(secretsColl+".CreateItem", 0, props, secret, true). // replace=true
		Store(&item, &promptPath); err != nil {
		return false
	}
	return completePrompt(conn, promptPath) == nil
}

// nativeSecureGet reads the value stored for key ("" when absent, locked
// beyond recovery, or no Secret Service is running).
func nativeSecureGet(key string) string {
	conn, err := dbus.SessionBus()
	if err != nil {
		return ""
	}
	session, err := openSecretSession(conn)
	if err != nil {
		return ""
	}
	defer closeSecretSession(conn, session)

	var unlocked, locked []dbus.ObjectPath
	if err := conn.Object(secretsDest, secretsPath).
		Call(secretsSvc+".SearchItems", 0, qormSecretAttrs(key)).
		Store(&unlocked, &locked); err != nil {
		return ""
	}
	if len(unlocked) == 0 && len(locked) > 0 {
		if unlockObjects(conn, locked) == nil {
			for _, p := range locked {
				if !objectLocked(conn, p, secretsItem) {
					unlocked = append(unlocked, p)
				}
			}
		}
	}
	if len(unlocked) == 0 {
		return ""
	}
	var secret dbusSecret
	if err := conn.Object(secretsDest, unlocked[0]).
		Call(secretsItem+".GetSecret", 0, session).Store(&secret); err != nil {
		return ""
	}
	return string(secret.Value)
}
