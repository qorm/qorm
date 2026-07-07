//go:build js && wasm

package qormext

import "syscall/js"

// CallJS runs a line of JS in the app's WebView (from the Go middle-layer).
func CallJS(script string) { js.Global().Call("eval", script) }

// Native triggers a framework low-level op (hardware bridge / built-in) directly
// from Go: e.g. Native("bluetoothScan", "{}") reaches the native bridge or Web
// API. Results arrive at the app's qormOn<X> JS callback.
func Native(op, dataJSON string) {
	if dataJSON == "" {
		dataJSON = "{}"
	}
	js.Global().Call("qormToNative", op, js.Global().Get("JSON").Call("parse", dataJSON))
}
