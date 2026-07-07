//go:build ignore

package main

import "github.com/qorm/qorm/pkg/qormext"

// The app's OWN native middle-layer, in Go — ONE file, compiled into the desktop
// binary AND the mobile/web WASM. Runs everywhere.
func init() {
	// (1) pure logic → return a line of JS to update the app
	qormext.Register("myBankSDK", func(data map[string]any) string {
		// your real Go logic: HTTP, computation, protocol, backend SDK…
		return `qormOnBankSDK("paid $9.99 via Go middle-layer (compiled in)")`
	})

	// (2) reach hardware DIRECTLY from Go via the framework's low-level bridge —
	// qormext.Native routes to the native hardware bridge (or a Web API); the
	// result comes back to the app's qormOn<X> JS callback.
	qormext.Register("payAndBuzz", func(data map[string]any) string {
		qormext.Native("vibrate", `{"ms":200}`) // Go → framework hardware
		return `qormOnBankSDK("paid + buzzed via Go (direct hardware call)")`
	})
}
