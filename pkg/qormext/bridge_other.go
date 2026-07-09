//go:build !(js && wasm)

package qormext

// On non-WASM builds (the desktop binary) the middle-layer reaches the WebView
// through an evaluator the host registers (nativeEval on the app window).
var evaluator func(string)

// SetEvaluator wires the host's JS evaluator (desktop only).
func SetEvaluator(fn func(string)) { evaluator = fn }

// CallJS runs a line of JS in the app's WebView (from the Go middle-layer).
func CallJS(script string) {
	if evaluator != nil {
		evaluator(script)
	}
}

// Native triggers a framework low-level op (hardware bridge / built-in) directly
// from Go: e.g. Native("bluetoothScan", "{}") reaches the native bridge or Web
// API. Results arrive at the app's qormOn<X> JS callback.
func Native(op, dataJSON string) {
	if dataJSON == "" {
		dataJSON = "{}"
	}
	CallJS("qormToNative(" + jsStr(op) + "," + dataJSON + ")")
}

func jsStr(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b = append(b, '\\')
		}
		b = append(b, string(r)...)
	}
	return string(append(b, '"'))
}
