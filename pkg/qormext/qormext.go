// Package qormext is the user middle-layer registry. An app registers its OWN
// native ops (in Go) via native/desktop.go, which the packager compiles INTO
// the app's single executable. The desktop bridge dispatches unknown ops here.
//
// Contract — the app's native/desktop.go:
//
//	package main
//	import "github.com/qorm/qorm/pkg/qormext"
//	func init() {
//	    qormext.Register("myOp", func(data map[string]any) string {
//	        // your Go logic; return one line of JS to run back in the app
//	        return `qormOnMyOp("done")`
//	    })
//	}
package qormext

import (
	"fmt"
	"strconv"
	"strings"
)

// ABIVersion is the plugin contract version of THIS runtime — the shape of the
// qormext API an app's native/desktop.go compiles against (Op signature,
// Register, Emit, the bridge). Bump the major on a breaking change so an app
// authored against an incompatible contract is caught instead of misbehaving.
// An app declares the ABI it expects via `"pluginABI": "1"` in qorm.json; the
// loader compares the major and warns on a mismatch.
const ABIVersion = 1

// CompatibleABI reports whether an app's declared plugin ABI (e.g. "1" or
// "1.2") is major-compatible with this runtime's ABIVersion. An empty/unset
// declaration is always compatible (the app uses no versioned middle-layer).
func CompatibleABI(declared string) bool {
	declared = strings.TrimSpace(declared)
	if declared == "" {
		return true
	}
	major := declared
	if i := strings.IndexByte(declared, '.'); i >= 0 {
		major = declared[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(major))
	if err != nil {
		return false
	}
	return n == ABIVersion
}

// Op handles a custom native op: it receives the qormToNative payload and
// returns a line of JS (e.g. qormOnFoo(...)) to eval back in the app, or "".
type Op func(data map[string]any) string

// Ops is the registry of app-provided custom ops, keyed by op name.
var Ops = map[string]Op{}

// Register adds a custom native op (call from an init() in native/desktop.go).
func Register(name string, fn Op) { Ops[name] = fn }

// Emit pushes a signal onto the frontend event channel: every qormOn(event)
// listener in the UI fires with dataJSON (a JSON value — "null" if empty). This
// is the middle-layer's push side: Go/WASM code tells the UI something changed
// and the frontend just listens, instead of the UI polling. Runs on desktop (via
// the evaluator) and in WASM (via eval) alike.
func Emit(event, dataJSON string) {
	if dataJSON == "" {
		dataJSON = "null"
	}
	CallJS(fmt.Sprintf("qormEmit(%q,%s)", event, dataJSON))
}
