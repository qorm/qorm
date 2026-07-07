//go:build js && wasm

// Command qorm-wasm is the standalone client-side QORM runtime: it renders and
// dispatches entirely in the browser/WebView with no server, so a QORM app can
// ship as static assets inside an installable package (PWA / APK / IPA). The
// server build stays the collaboration surface; this build is the shipped app.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/pkg/qormext"
)

var (
	rt       *runtime.Runtime
	handlers []render.Handler
)

func main() {
	js.Global().Set("qormInit", js.FuncOf(qormInit))
	js.Global().Set("qormEvent", js.FuncOf(qormEvent))
	js.Global().Set("qormSetState", js.FuncOf(qormSetState))
	js.Global().Set("qormWasmOp", js.FuncOf(qormWasmOp))
	select {} // keep the Go runtime alive for the JS callbacks
}

// qormWasmOp(op, dataJSON) runs the app's Go middle-layer op — registered in
// native/desktop.go via qormext, compiled INTO this WASM — and returns a line
// of JS to eval (e.g. qormOnFoo(...)), or "" if no such op. This is how the
// user's ONE Go middle-layer runs on mobile/web WebViews (same registry the
// desktop binary uses natively).
func qormWasmOp(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return ""
	}
	fn := qormext.Ops[args[0].String()]
	if fn == nil {
		return ""
	}
	var data map[string]any
	if len(args) > 1 && args[1].Type() == js.TypeString {
		json.Unmarshal([]byte(args[1].String()), &data)
	}
	return fn(data)
}

// qormInit(bundleJSON) loads the embedded app bundle and returns the first render.
func qormInit(_ js.Value, args []js.Value) any {
	b, err := bundle.Unmarshal([]byte(args[0].String()))
	if err != nil {
		return errResult(err)
	}
	rt = runtime.New(b.ToApp())
	return renderNow()
}

// qormEvent(h, inputsJSON) folds any bound input values into state, dispatches
// handler h, and returns the re-render — the client-side twin of /event.
func qormEvent(_ js.Value, args []js.Value) any {
	if rt == nil {
		return errResult(nil)
	}
	if len(args) > 1 && args[1].Type() == js.TypeString {
		var inputs map[string]any
		if json.Unmarshal([]byte(args[1].String()), &inputs) == nil {
			for k, v := range inputs {
				rt.State[k] = v
			}
		}
	}
	h := args[0].Int()
	if h >= 0 && h < len(handlers) {
		hd := handlers[h]
		if hd.Name != "" {
			ctx := map[string]any{"state": rt.State}
			for k, v := range hd.Scope {
				ctx[k] = v
			}
			ev := map[string]any{}
			for name, e := range hd.Args {
				ev[name] = runtime.EvalBinding(e, ctx)
			}
			rt.Dispatch(hd.Name, ev)
		}
	}
	return renderNow()
}

// qormSetState(path, valueJSON) sets a top-level state key (for host bridges).
func qormSetState(_ js.Value, args []js.Value) any {
	if rt == nil || len(args) < 2 {
		return errResult(nil)
	}
	var v any
	json.Unmarshal([]byte(args[1].String()), &v)
	rt.State[args[0].String()] = v
	return renderNow()
}

func renderNow() any {
	res := render.Render(rt)
	handlers = res.Handlers
	dir := "ltr"
	if rt.IsRTL() {
		dir = "rtl"
	}
	return map[string]any{"html": res.HTML, "theme": rt.CurrentTheme(), "dir": dir, "locale": rt.CurrentLocale()}
}

func errResult(err error) any {
	msg := "not initialized"
	if err != nil {
		msg = err.Error()
	}
	return map[string]any{"html": "<div style=\"padding:20px;color:#b91c1c\">qorm: " + msg + "</div>", "theme": "apple", "dir": "ltr"}
}
