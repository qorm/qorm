package canvas

import (
	"strconv"
	"strings"
	"sync"
)

// NativeInvoker executes a native capability op — the canvas counterpart of
// the HTML bridge's qormToNative — and reports the result the way the JS
// bridge does: a callback name (qormOn<Stem>) and one argument (a string or
// bool). The host installs it once (SetNativeInvoker); implementations run
// on the render thread and must be fast (long-running ops need an async
// upgrade, documented in internal/widgets/hardware.go).
type NativeInvoker func(op string, data map[string]any, cb func(name string, arg any))

var (
	nativeMu sync.RWMutex
	nativeFn NativeInvoker
)

// SetNativeInvoker installs the host's native bridge (the desktop binary's
// compiled-in ops). Nil (the default) means no native capabilities:
// capability widgets render their quiet degradation instead of pretending.
func SetNativeInvoker(fn NativeInvoker) {
	nativeMu.Lock()
	defer nativeMu.Unlock()
	nativeFn = fn
}

// NativeAvailable reports whether a native bridge is installed.
func NativeAvailable() bool {
	nativeMu.RLock()
	defer nativeMu.RUnlock()
	return nativeFn != nil
}

// InvokeNative runs one op through the host bridge. It is a no-op when no
// bridge is installed (cb never fires).
func InvokeNative(op string, data map[string]any, cb func(name string, arg any)) {
	nativeMu.RLock()
	fn := nativeFn
	nativeMu.RUnlock()
	if fn != nil {
		fn(op, data, cb)
	}
}

// ParseNativeCallback splits a JS-bridge callback string —
// qormOnNetwork("{\"online\":true}") or qormOnOpenUrl(true) — into its
// callback name and first argument (string or bool).
func ParseNativeCallback(js string) (name string, arg any, ok bool) {
	js = strings.TrimSpace(js)
	open := strings.Index(js, "(")
	if open <= 0 || !strings.HasSuffix(js, ")") {
		return "", nil, false
	}
	name = js[:open]
	body := strings.TrimSpace(js[open+1 : len(js)-1])
	// First argument only: a quoted string, a boolean, or a bare number.
	if body == "" {
		return name, nil, true
	}
	if body[0] == '"' {
		if s, err := strconv.Unquote(body); err == nil {
			return name, s, true
		}
		// Unquoted content after the first string (extra args) is dropped in
		// v1 — the callback contract carries one payload.
		return "", nil, false
	}
	switch body {
	case "true":
		return name, true, true
	case "false":
		return name, false, true
	}
	if f, err := strconv.ParseFloat(body, 64); err == nil {
		return name, f, true
	}
	return "", nil, false
}
