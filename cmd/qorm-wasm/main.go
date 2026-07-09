//go:build js && wasm

// Command qorm-wasm is the standalone client-side QORM runtime: it renders and
// dispatches entirely in the browser/WebView with no server, so a QORM app can
// ship as static assets inside an installable package (PWA / APK / IPA). The
// server build stays the collaboration surface; this build is the shipped app.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"syscall/js"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/ota"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/pkg/qormext"
)

var (
	rt       *runtime.Runtime
	handlers []render.Handler
	// currentBundleJSON is the canonical (bundle.Marshal) encoding of the bundle
	// the runtime is executing, so qormCheckUpdate can skip a no-op re-apply.
	currentBundleJSON string
)

func main() {
	js.Global().Set("qormInit", js.FuncOf(qormInit))
	js.Global().Set("qormEvent", js.FuncOf(qormEvent))
	js.Global().Set("qormSetState", js.FuncOf(qormSetState))
	js.Global().Set("qormWasmOp", js.FuncOf(qormWasmOp))
	js.Global().Set("qormCheckUpdate", js.FuncOf(qormCheckUpdate))
	js.Global().Set("qormOTARollback", js.FuncOf(qormOTARollback))
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

// qormInit(bundleJSON[, source]) loads an app bundle and returns the first
// render. source says where the bytes came from: "ota" (localStorage
// "qorm.ota.bundle"), "prev" ("qorm.ota.prev"), or absent/"" for the bundle.json
// shipped inside the package.
//
// Verification policy (security model, planning/v0.2.0-release-plan.md §五 B3):
//   - OTA-origin bundles ("ota"/"prev") crossed the network after install and
//     MUST pass bundle.VerifyWithRevocation against the trust key injected via
//     window.__QORM_UPDATE__. A failure DROPS that localStorage level and returns
//     {err:...} so the boot script falls through to the next level
//     (prev -> packaged bundle.json).
//   - The packaged bundle keeps today's behavior — integrity/authenticity are
//     not re-checked here because it arrived through the install channel
//     (store-signed IPA/APK, PWA origin + TLS), which is the same channel this
//     WASM binary itself came from: a signature check by code from an
//     untrusted channel would add no trust.
func qormInit(_ js.Value, args []js.Value) any {
	source := ""
	if len(args) > 1 && args[1].Type() == js.TypeString {
		source = args[1].String()
	}
	fromOTA := source == "ota" || source == "prev"
	b, err := bundle.Unmarshal([]byte(args[0].String()))
	if err == nil && fromOTA {
		if cfg, cErr := readUpdateConfig(); cErr != nil {
			// Stored OTA bundle but no (valid) trust key: unverifiable, refuse it.
			err = cErr
		} else {
			err = bundle.VerifyWithRevocation(b, cfg.trust, nil)
		}
	}
	if err != nil {
		if fromOTA {
			dropOTALevel(source)
			return map[string]any{"err": "qorm ota: " + source + " bundle rejected: " + err.Error()}
		}
		return errResult(err)
	}
	rt = runtime.New(b.ToApp())
	if data, mErr := bundle.Marshal(b); mErr == nil {
		currentBundleJSON = string(data)
	}
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

// TODO(P1.1 responsive `when`): read js.Global() innerWidth/innerHeight here
// (rt.Viewport = runtime.Viewport{W, H}) before rendering, so responsive
// `when` nodes resolve against the real client viewport in the standalone
// WASM build too. Deferred — this file is concurrently reworked by the B3
// (OTA verification) task; until then viewport is 0x0 offline and `when`
// renders its `else` branch (the documented unknown-viewport semantics).
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

// ---- OTA client (browser/WebView side of `qorm updates`) ----

// localStorage keys for the OTA client. bundle/prev hold whole bundle JSON
// texts (level 1 and 2 of the boot fallback chain); client is this install's
// stable rollout-bucketing id.
const (
	otaKeyBundle = "qorm.ota.bundle"
	otaKeyPrev   = "qorm.ota.prev"
	otaKeyClient = "qorm.ota.client"
)

// otaMaxBundle caps what we persist: browsers commonly quota localStorage at
// ~5 MB per origin, and a failed setItem near the quota can also evict nothing
// while still throwing. Bundles beyond ~4 MB are refused up front with a clear
// message instead of half-applying.
const otaMaxBundle = 4 << 20

// otaConfig is the parsed window.__QORM_UPDATE__ injected by the packager
// (server.OfflineHTML): the update server URL, the app's rollout id, and the
// ed25519 public key every OTA bundle must be signed with.
type otaConfig struct {
	url, app string
	trust    ed25519.PublicKey
}

// readUpdateConfig reads and validates window.__QORM_UPDATE__. A missing or
// keyless config disables OTA entirely — the same fail-closed "no trusted key,
// no update" model as the live server's /update endpoint.
func readUpdateConfig() (*otaConfig, error) {
	u := js.Global().Get("__QORM_UPDATE__")
	if u.Type() != js.TypeObject {
		return nil, fmt.Errorf("no update server configured (window.__QORM_UPDATE__ missing)")
	}
	str := func(k string) string {
		v := u.Get(k)
		if v.Type() != js.TypeString {
			return ""
		}
		return v.String()
	}
	cfg := &otaConfig{url: strings.TrimRight(str("url"), "/"), app: str("app")}
	if cfg.url == "" || cfg.app == "" {
		return nil, fmt.Errorf("update config needs url and app")
	}
	raw, err := base64.StdEncoding.DecodeString(str("trust"))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("update config has no valid trust key (base64 ed25519 public key) — OTA stays off")
	}
	cfg.trust = ed25519.PublicKey(raw)
	return cfg, nil
}

// qormCheckUpdate() asks the configured update server which bundle this client
// should run, verifies it, persists it (current -> qorm.ota.prev, new ->
// qorm.ota.bundle) and hot-swaps the runtime. Returns a JS Promise resolving
// with the fresh render (same shape as qormInit; {updated:false} if already
// current) or rejecting with an error string. Every failure leaves the running
// app and its stored bundles untouched — rollback by inaction.
func qormCheckUpdate(js.Value, []js.Value) any {
	return promise(func(resolve, reject func(any)) {
		cfg, err := readUpdateConfig()
		if err != nil {
			reject("qorm ota: " + err.Error())
			return
		}
		source := cfg.url + "/resolve?app=" + url.QueryEscape(cfg.app) + "&client=" + url.QueryEscape(otaClientID())
		next, err := ota.FetchVerified(source, cfg.trust, nil)
		if err != nil {
			reject("qorm ota: " + err.Error())
			return
		}
		data, err := bundle.Marshal(next)
		if err != nil {
			reject("qorm ota: " + err.Error())
			return
		}
		if len(data) > otaMaxBundle {
			reject(fmt.Sprintf("qorm ota: bundle is %.1f MB — over the ~4 MB localStorage budget, not applied (ship a smaller bundle or use a native store)", float64(len(data))/(1<<20)))
			return
		}
		if string(data) == currentBundleJSON {
			resolve(map[string]any{"updated": false})
			return
		}
		// Persist BEFORE swapping so a crash right after still reboots into the
		// new version; keep the old level for qormOTARollback.
		if cur := lsGet(otaKeyBundle); cur != "" {
			if err := lsSet(otaKeyPrev, cur); err != nil {
				reject("qorm ota: could not keep rollback copy: " + err.Error())
				return
			}
		}
		if err := lsSet(otaKeyBundle, string(data)); err != nil {
			reject("qorm ota: could not persist bundle: " + err.Error())
			return
		}
		rt = runtime.New(next.ToApp())
		currentBundleJSON = string(data)
		res := renderNow().(map[string]any)
		res["updated"] = true
		resolve(res)
	})
}

// qormOTARollback() restores the previous OTA bundle: qorm.ota.prev becomes the
// active stored bundle and the runtime hot-swaps to it. Synchronous (no
// network). Returns the fresh render, or {err:...} when there is nothing to
// roll back to or prev no longer verifies.
func qormOTARollback(js.Value, []js.Value) any {
	prev := lsGet(otaKeyPrev)
	if prev == "" {
		return map[string]any{"err": "qorm ota: no previous bundle to roll back to"}
	}
	b, err := bundle.Unmarshal([]byte(prev))
	if err == nil {
		if cfg, cErr := readUpdateConfig(); cErr != nil {
			err = cErr
		} else {
			err = bundle.VerifyWithRevocation(b, cfg.trust, nil)
		}
	}
	if err != nil {
		lsRemove(otaKeyPrev) // unusable — drop the level
		return map[string]any{"err": "qorm ota: previous bundle rejected: " + err.Error()}
	}
	if err := lsSet(otaKeyBundle, prev); err != nil {
		return map[string]any{"err": "qorm ota: " + err.Error()}
	}
	lsRemove(otaKeyPrev)
	rt = runtime.New(b.ToApp())
	if data, mErr := bundle.Marshal(b); mErr == nil {
		currentBundleJSON = string(data)
	}
	return renderNow()
}

// otaClientID returns this install's stable random client id (so the update
// server's staged rollout buckets the device deterministically), generating and
// persisting one on first use.
func otaClientID() string {
	if id := lsGet(otaKeyClient); id != "" {
		return id
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "anon"
	}
	id := hex.EncodeToString(buf)
	_ = lsSet(otaKeyClient, id) // best effort — a per-boot id still resolves
	return id
}

// dropOTALevel removes a rejected localStorage bundle level so the next boot
// goes straight to the level below it.
func dropOTALevel(source string) {
	if source == "prev" {
		lsRemove(otaKeyPrev)
	} else {
		lsRemove(otaKeyBundle)
	}
}

// promise wraps run in a JS Promise whose work happens on a NEW goroutine.
// That hop is mandatory, not a style choice: net/http on js/wasm blocks the
// calling goroutine on the JS fetch, and if that goroutine is the one servicing
// the JS callback (the event loop's re-entry into Go), the single-threaded WASM
// scheduler deadlocks waiting on itself.
func promise(run func(resolve, reject func(any))) js.Value {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, args []js.Value) any {
		res, rej := args[0], args[1]
		go func() {
			defer executor.Release()
			run(func(v any) { res.Invoke(v) }, func(v any) { rej.Invoke(v) })
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

// lsGet reads a localStorage key, returning "" when absent or when storage is
// unavailable (sandboxed WebViews can throw on any localStorage access).
func lsGet(key string) (val string) {
	defer func() { _ = recover() }()
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return ""
	}
	v := ls.Call("getItem", key)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

// lsSet writes a localStorage key, converting storage exceptions (quota,
// sandbox) into errors instead of panics.
func lsSet(key, val string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("localStorage set %s: %v", key, r)
		}
	}()
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return fmt.Errorf("localStorage unavailable")
	}
	ls.Call("setItem", key, val)
	return nil
}

// lsRemove deletes a localStorage key, ignoring storage exceptions.
func lsRemove(key string) {
	defer func() { _ = recover() }()
	ls := js.Global().Get("localStorage")
	if ls.Truthy() {
		ls.Call("removeItem", key)
	}
}
