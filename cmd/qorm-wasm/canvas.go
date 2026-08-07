//go:build js && wasm && qorm_canvas

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"sync"
	"syscall/js"

	"github.com/qorm/qorm/internal/playcore"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/theme"
)

var (
	cvsEngine  *canvas.Engine
	cvsSurface *canvas.HeadlessSurface
)

// preloadedAssets is a sync-on-read, async-on-write cache for asset bytes.
// Populated by qormCanvasPreloadAssets BEFORE the first frame is drawn so
// the canvas engine never enters its negative cache for a known-good URL.
// The pure-Go sync read is keyed off the same resolved URL the engine would
// otherwise build (BaseDir + src), so no path translation is needed.
//
// preloadFailed is a *diagnostic ledger only* — it tracks which URLs the
// preloader couldn't fetch, for log output. wasmReadFile does NOT consult
// it: the preloader is a hint, the canvas engine's soft negative cache
// (image.go: isTransientReadErr) is authoritative. Gating on preloadFailed
// here would short-circuit the sync-XHR fallback for URLs the preloader
// failed on, the engine would see the synthetic "preload failed" error as
// a transient miss, and the asset would stay broken forever.
var preloadedAssetsMu sync.RWMutex
var preloadedAssets = map[string][]byte{}
var preloadFailed = map[string]bool{} // diagnostic only — see comment above

func init() {
	// Override filesystem readers so the canvas engine loads PNGs + theme
	// JSON via JavaScript's HTTP layer instead of os.ReadFile. The reader
	// consults a preloaded byte cache first (populated by
	// qormCanvasPreloadAssets, a Promise<ArrayBuffer> fan-in) and falls
	// back to a sync XHR for anything that wasn't pre-warmed (e.g. a scene
	// that references an asset the host forgot to list).
	canvas.SetImageReadFile(wasmReadFile)
	theme.SetThemeReadFile(wasmReadFile)
	js.Global().Set("qormCanvasInit", js.FuncOf(qormCanvasInit))
	js.Global().Set("qormCanvasFrame", js.FuncOf(qormCanvasFrame))
	js.Global().Set("qormCanvasPtr", js.FuncOf(qormCanvasPtr))
	js.Global().Set("qormCanvasKey", js.FuncOf(qormCanvasKey))
	js.Global().Set("qormCanvasInitFromBundle", js.FuncOf(qormCanvasInitFromBundle))
	js.Global().Set("qormCanvasPreloadAssets", js.FuncOf(qormCanvasPreloadAssets))
	js.Global().Set("qormGetState", js.FuncOf(qormGetState))
}

// wasmReadFile is the disk-read seam. Order:
//  1. Preloaded cache (populated by qormCanvasPreloadAssets) — fast path
//     that the WASM measure/layout passes hit every frame.
//  2. Sync XHR as the fallback for anything the preloader missed (e.g. a
//     scene that references an asset the host forgot to list, or a URL the
//     preloader fetch() flaked on). The XHR is wrapped in a recover()
//     because Chrome's deprecation warnings on main-thread sync XHR
//     occasionally surface as a synchronous throw, not a status code —
//     without the recover the whole engine panics.
//
// Note: we deliberately do NOT short-circuit on preloadFailed[path]. The
// preloader is a hint; the canvas engine's soft negative cache
// (isTransientReadErr) is the only authoritative retry-vs-cache decision.
// If the preloader failed once (network blip, 5xx) and the engine then
// sees a transient miss every frame, the engine will keep retrying sync
// XHR — which is exactly the right behaviour.
func wasmReadFile(path string) ([]byte, error) {
	preloadedAssetsMu.RLock()
	if b, ok := preloadedAssets[path]; ok {
		preloadedAssetsMu.RUnlock()
		return b, nil
	}
	preloadedAssetsMu.RUnlock()
	return wasmSyncRead(path)
}

// wasmSyncRead is the legacy sync XHR fallback. The negative cache lives
// in the canvas engine (image.go) — keeping it here would create two
// competing caches. If the XHR throws (Chrome deprecation noise, CSP
// denial, network race), we recover and report a synthetic HTTP 0 so the
// engine's normal "image failed to load" placeholder path takes over.
func wasmSyncRead(path string) ([]byte, error) {
	var sendErr error
	var xhr js.Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				sendErr = fmt.Errorf("XHR panic: %v", r)
			}
		}()
		xhr = js.Global().Get("XMLHttpRequest").New()
		xhr.Call("open", "GET", path, false) // false = synchronous
		// MUST set responseType BEFORE send() for binary data — default is
		// text/string and silently corrupts PNG bytes.
		xhr.Set("responseType", "arraybuffer")
		xhr.Call("send", nil)
	}()
	if sendErr != nil {
		return nil, sendErr
	}
	status := xhr.Get("status").Int()
	if status != 200 {
		return nil, fmt.Errorf("HTTP %d fetching %s", status, path)
	}
	resp := xhr.Get("response")
	if resp.IsUndefined() || resp.IsNull() {
		return nil, fmt.Errorf("empty response for %s", path)
	}
	// response is an ArrayBuffer; convert to Go []byte.
	arr := js.Global().Get("Uint8Array").New(resp)
	n := arr.Get("length").Int()
	if n == 0 {
		return nil, fmt.Errorf("zero-length response for %s", path)
	}
	buf := make([]byte, n)
	js.CopyBytesToGo(buf, arr)
	return buf, nil
}

// qormCanvasPreloadAssets(urlsJSON) returns a Promise<{loaded:number,
// failed:number}> after fanning out an async fetch for every URL. The
// engine's measure pass then finds the bytes in preloadedAssets and
// never enters its negative cache for those URLs.
//
// Why a preloader and not "just use async fetch on every loadImage":
// loadImage is called from the synchronous measure/layout passes
// (every frame, 30+ widgets). Bridging it to async would require
// suspend/resume on the goroutine for every read, which is far more
// invasive and has worse worst-case latency than a one-time preload.
func qormCanvasPreloadAssets(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.Global().Get("Promise").Call("reject", "usage: qormCanvasPreloadAssets(urlsJSON)")
	}
	var urls []string
	if err := json.Unmarshal([]byte(args[0].String()), &urls); err != nil {
		return js.Global().Get("Promise").Call("reject", "invalid JSON: "+err.Error())
	}
	if len(urls) == 0 {
		return js.Global().Get("Promise").Call("resolve", js.ValueOf(`{"loaded":0,"failed":0}`))
	}

	promiseCtor := js.Global().Get("Promise")
	return promiseCtor.New(js.FuncOf(func(_ js.Value, pArgs []js.Value) any {
		resolve, reject := pArgs[0], pArgs[1]

		// Pending counter lives in a closure so each Promise has its own
		// fan-in. resolved/failed are reported in the final payload.
		var (
			mu          sync.Mutex
			pending     = len(urls)
			loadedCount = 0
			failedCount = 0
		)
		maybeFinish := func() {
			mu.Lock()
			defer mu.Unlock()
			if pending > 0 {
				return
			}
			payload := fmt.Sprintf(`{"loaded":%d,"failed":%d}`, loadedCount, failedCount)
			if failedCount > 0 && loadedCount == 0 {
				reject.Invoke(payload)
			} else {
				resolve.Invoke(payload)
			}
		}
		onOne := func(url string, ok bool, data []byte, errStr string) {
			preloadedAssetsMu.Lock()
			if ok {
				preloadedAssets[url] = data
			} else {
				preloadFailed[url] = true
			}
			preloadedAssetsMu.Unlock()
			mu.Lock()
			pending--
			if ok {
				loadedCount++
			} else {
				failedCount++
				fmt.Printf("[wasm preload] %s failed: %s\n", url, errStr)
			}
			mu.Unlock()
			maybeFinish()
		}

		for _, u := range urls {
			u := u // capture per-iteration
			fetch := js.Global().Get("fetch")
			p := fetch.Invoke(u)

			// Go disallows `cb := js.FuncOf(func(){ defer cb.Release(); ... })` —
			// the closure can't reference the variable it's being assigned to
			// via :=. Declare first, then assign, so the closure can see it.
			var thenCb js.Func
			thenCb = js.FuncOf(func(_ js.Value, tArgs []js.Value) any {
				resp := tArgs[0]
				ok := resp.Get("ok").Bool()
				status := resp.Get("status").Int()
				thenCb.Release()
				if !ok {
					onOne(u, false, nil, fmt.Sprintf("HTTP %d", status))
					return js.Undefined()
				}
				bufPromise := resp.Call("arrayBuffer")
				var bufThenCb js.Func
				bufThenCb = js.FuncOf(func(_ js.Value, bArgs []js.Value) any {
					bufThenCb.Release()
					ab := bArgs[0]
					arr := js.Global().Get("Uint8Array").New(ab)
					n := arr.Get("length").Int()
					if n == 0 {
						onOne(u, false, nil, "zero-length arrayBuffer")
						return js.Undefined()
					}
					data := make([]byte, n)
					js.CopyBytesToGo(data, arr)
					onOne(u, true, data, "")
					return js.Undefined()
				})
				bufPromise.Call("then", bufThenCb)
				return js.Undefined()
			})
			var catchCb js.Func
			catchCb = js.FuncOf(func(_ js.Value, cArgs []js.Value) any {
				catchCb.Release()
				msg := "fetch error"
				if len(cArgs) > 0 && !cArgs[0].IsUndefined() && !cArgs[0].IsNull() {
					msg = cArgs[0].String()
				}
				onOne(u, false, nil, msg)
				return js.Undefined()
			})
			p.Call("then", thenCb)
			p.Call("catch", catchCb)
		}
		return js.Undefined()
	}))
}

// qormCanvasInit prepares the canvas engine from the runtime already loaded
// by qormCompile. Call this after qormCompile, then start the frame loop.
func qormCanvasInit(_ js.Value, args []js.Value) any {
	if rt == nil {
		return map[string]any{"err": "no runtime — call qormCompile first"}
	}
	size := image.Pt(420, 680)
	cvsSurface = canvas.NewHeadlessSurface(size)
	cvsSurface.Logical = size
	cvsSurface.ScaleFactor = 1
	cvsEngine = canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return nil
}

// qormCanvasFrame(canvasEl) renders one frame into the given HTML <canvas>.
func qormCanvasFrame(_ js.Value, args []js.Value) any {
	if cvsEngine == nil || cvsSurface == nil || len(args) < 1 {
		return false
	}
	cvsEngine.DrawFrame(cvsSurface)
	buf := cvsSurface.Frame()
	w, h := buf.Bounds().Dx(), buf.Bounds().Dy()

	// 1:1 mapping: canvas pixel buffer == canvas element size
	canvasEl := args[0]
	canvasEl.Set("width", w)
	canvasEl.Set("height", h)

	pixels := js.Global().Get("Uint8ClampedArray").New(len(buf.Pix))
	js.CopyBytesToJS(pixels, buf.Pix)
	imgData := js.Global().Get("ImageData").New(pixels, w, h)
	ctx := canvasEl.Call("getContext", "2d")
	ctx.Call("putImageData", imgData, 0, 0)
	return true
}

// qormCanvasPtr(typ, x, y) forwards a pointer event.
func qormCanvasPtr(_ js.Value, args []js.Value) any {
	if cvsEngine == nil || len(args) < 3 {
		return false
	}
	typ := args[0].String()
	x, y := args[1].Float(), args[2].Float()
	var pt canvas.PointerType
	switch typ {
	case "press":
		pt = canvas.PointerPress
	case "release":
		pt = canvas.PointerRelease
	default:
		pt = canvas.PointerMove
	}
	return cvsEngine.HandlePointer(canvas.PointerInput{Type: pt, X: x, Y: y, Buttons: 1})
}

// qormCanvasKey(key, down) forwards a keyboard event.
func qormCanvasKey(_ js.Value, args []js.Value) any {
	if cvsEngine == nil || len(args) < 1 {
		return false
	}
	key := args[0].String()
	down := true
	if len(args) > 1 {
		down = args[1].Bool()
	}
	return cvsEngine.HandleKey(canvas.KeyInput{Key: key, Down: down})
}

// qormCanvasInitFromBundle(docsJSON, baseURL, width, height) compiles a doc
// array, sets the app's BaseDir to the given URL (so images resolve), and
// prepares the canvas engine at the given size. Previous engine + runtime are
// explicitly discarded so a game switch starts fresh. Preloaded assets from
// the previous game are also flushed — a 404 on the old game's URL is not
// the same as a 404 on the new game's URL.
func qormCanvasInitFromBundle(_ js.Value, args []js.Value) (ret any) {
	defer func() {
		if r := recover(); r != nil {
			ret = map[string]any{"err": fmt.Sprintf("init panic: %v", r)}
		}
	}()
	if len(args) < 2 {
		return map[string]any{"err": "usage: qormCanvasInitFromBundle(docsJSON, baseURL [, width, height])"}
	}
	var docs []map[string]any
	if err := json.Unmarshal([]byte(args[0].String()), &docs); err != nil {
		return map[string]any{"err": "invalid JSON: " + err.Error()}
	}
	baseURL := args[1].String()

	// Discard previous engine + surface + image cache + preloaded bytes so
	// stale timers/state/negative-cache don't survive the game switch.
	cvsEngine = nil
	cvsSurface = nil
	rt = nil
	handlers = nil
	canvas.ResetImageCache()
	preloadedAssetsMu.Lock()
	preloadedAssets = map[string][]byte{}
	preloadFailed = map[string]bool{}
	preloadedAssetsMu.Unlock()

	res := playcore.CompileDocs(docs)
	if len(res.Diagnostics) > 0 && res.HTML == "" {
		return map[string]any{"err": "compile failed", "diagnostics": res.Diagnostics}
	}
	// Wire the BaseDir so resolveImageSrc knows where images live. Mark the
	// app as Web (App.Web = true) too: without it, the resolver can't tell
	// the WASM URL prefix (e.g. "/games/raiden/") apart from a real native
	// path (e.g. "/var/folders/.../T/..." — both start with "/") and applies
	// the filesystem jail check to the browser case, which would refuse
	// every relative image src.
	if res.RT.App != nil && baseURL != "" {
		res.RT.App.BaseDir = baseURL
		res.RT.App.Web = true
	}
	adopt(res.RT)
	handlers = res.Handlers

	w, h := 420, 680
	if len(args) >= 4 {
		w = args[2].Int()
		h = args[3].Int()
	}
	size := image.Pt(w, h)
	cvsSurface = canvas.NewHeadlessSurface(size)
	cvsSurface.Logical = size
	cvsSurface.ScaleFactor = 1
	cvsEngine = canvas.NewEngine(res.RT, canvas.SoftwareRenderer{})
	// Tell the engine the current theme is already loaded so it
	// skips the os.ReadFile probe for custom themes in WASM.
	if res.RT.Theme != nil && res.RT.Theme.Name != "" {
		cvsEngine.SetThemeLoaded(res.RT.Theme.Name)
	}

	return nil
}

// qormGetState returns a JSON snapshot of the current runtime state, for
// debugging the games page from a browser devtools / Playwright. The
// "minimal" mode is what you usually want — it skips the heavy arrays
// (rows, viewTiles, etc.) and adds the engine-side diagnostics (camera
// pan, image cache, preloader counts) that aren't visible from
// rt.State. The default (no arg) is the raw rt.State marshalled to
// JSON; use that when you need the full viewTiles/rows data and can
// afford the wire cost.
//
// NOT for production use — the full state is on the Go heap and a
// naive json.Marshal of a deeply-nested viewTiles can stutter.
func qormGetState(_ js.Value, args []js.Value) any {
	if rt == nil {
		return "null"
	}
	if len(args) > 0 && args[0].String() == "minimal" {
		out := map[string]any{}
		if m, ok := rt.State["mario"].(map[string]any); ok {
			out["mario"] = m
		}
		if s, ok := rt.State["status"].(string); ok {
			out["status"] = s
		}
		if v, ok := rt.State["timeLeft"].(float64); ok {
			out["timeLeft"] = v
		}
		if t, ok := rt.State["viewTiles"].([]any); ok {
			out["viewTiles_len"] = len(t)
			hist := map[string]int{}
			minX, maxX := 1e9, -1e9
			for _, it := range t {
				if m, ok := it.(map[string]any); ok {
					if k, ok := m["kind"].(string); ok {
						hist[k]++
					}
					if x, ok := m["x"].(float64); ok {
						if x < minX {
							minX = x
						}
						if x > maxX {
							maxX = x
						}
					}
				}
			}
			out["viewTiles_hist"] = hist
			if minX < maxX {
				out["viewTiles_xRange"] = []float64{minX, maxX}
			}
		}
		// Engine-side state the JS host can't see otherwise.
		preloadedAssetsMu.RLock()
		out["preloaded_count"] = len(preloadedAssets)
		out["preloadFailed_count"] = len(preloadFailed)
		preloadedAssetsMu.RUnlock()
		out["imageCache_keys"] = canvas.ImageCacheKeys()
		if cvsEngine != nil {
			out["PanX"] = cvsEngine.Inter.Board.PanX
			out["PanY"] = cvsEngine.Inter.Board.PanY
			out["Zoom"] = cvsEngine.Inter.Board.Zoom
			if cvsSurface != nil {
				sz := cvsSurface.Size()
				out["viewportW"] = sz.X
				out["viewportH"] = sz.Y
			}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return fmt.Sprintf(`{"err":%q}`, err.Error())
		}
		return string(b)
	}
	b, err := json.Marshal(rt.State)
	if err != nil {
		return fmt.Sprintf(`{"err":%q}`, err.Error())
	}
	return string(b)
}
