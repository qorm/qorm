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

// jsStr renders s as a JS string literal. It escapes everything that would
// terminate the literal, break it across lines, or be an illegal raw
// character inside it: quote, backslash, newline, carriage return, tab, the
// other C0 control characters (generic \u00XX, mirroring a JSON escaper),
// and the U+2028/U+2029 line separators.
func jsStr(s string) string {
	const hex = "0123456789abcdef"
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case ' ':
			b = append(b, '\\', 'u', '2', '0', '2', '8')
		case ' ':
			b = append(b, '\\', 'u', '2', '0', '2', '9')
		default:
			if r < 0x20 {
				b = append(b, '\\', 'u', '0', '0', hex[r>>4], hex[r&0xf])
			} else {
				b = append(b, string(r)...)
			}
		}
	}
	return string(append(b, '"'))
}
