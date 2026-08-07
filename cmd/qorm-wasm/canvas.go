//go:build js && wasm && qorm_canvas

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"syscall/js"

	"github.com/qorm/qorm/internal/playcore"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/theme"
)

var (
	cvsEngine  *canvas.Engine
	cvsSurface *canvas.HeadlessSurface
)

func init() {
	// Override the image file reader so the canvas engine loads PNGs
	// via JavaScript's fetch() instead of os.ReadFile (no fs in WASM).
	canvas.SetImageReadFile(wasmReadFile)
	js.Global().Set("qormCanvasInit", js.FuncOf(qormCanvasInit))
	js.Global().Set("qormCanvasFrame", js.FuncOf(qormCanvasFrame))
	js.Global().Set("qormCanvasPtr", js.FuncOf(qormCanvasPtr))
	js.Global().Set("qormCanvasKey", js.FuncOf(qormCanvasKey))
	js.Global().Set("qormCanvasInitFromBundle", js.FuncOf(qormCanvasInitFromBundle))
}

// wasmReadFile fetches a URL via JavaScript's fetch() and returns the
// response body as bytes. The WAIT version blocks the current goroutine
// — the calling context (DrawFrame on rAF) is the JS event loop thread,
// so a synchronous XMLHttpRequest or a fetch+await pattern is safe here
// as long as it completes within a frame budget (a 32×32 PNG is ~200 B).
func wasmReadFile(path string) ([]byte, error) {
	// Use synchronous XMLHttpRequest — simpler and works in the WASM
	// main thread (rAF callback) without deadlocking the scheduler.
	xhr := js.Global().Get("XMLHttpRequest").New()
	xhr.Call("open", "GET", path, false) // false = synchronous
	// MUST set responseType BEFORE send() for binary data — default is
	// text/string and silently corrupts PNG bytes.
	xhr.Set("responseType", "arraybuffer")
	xhr.Call("send", nil)
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
	buf := make([]byte, n)
	js.CopyBytesToGo(buf, arr)
	return buf, nil
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
// explicitly discarded so a game switch starts fresh.
func qormCanvasInitFromBundle(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return map[string]any{"err": "usage: qormCanvasInitFromBundle(docsJSON, baseURL [, width, height])"}
	}
	var docs []map[string]any
	if err := json.Unmarshal([]byte(args[0].String()), &docs); err != nil {
		return map[string]any{"err": "invalid JSON: " + err.Error()}
	}
	baseURL := args[1].String()

	// Discard previous engine + surface + image cache so stale
	// timers/state/negative-cache don't survive the game switch.
	cvsEngine = nil
	cvsSurface = nil
	rt = nil
	handlers = nil
	canvas.ResetImageCache()

	res := playcore.CompileDocs(docs)
	if len(res.Diagnostics) > 0 && res.HTML == "" {
		return map[string]any{"err": "compile failed", "diagnostics": res.Diagnostics}
	}
	// Wire the BaseDir so resolveImageSrc knows where images live.
	if res.RT.App != nil && baseURL != "" {
		res.RT.App.BaseDir = baseURL
	}
	// In WASM there is no filesystem — themes must be resolved from the doc
	// list (for custom theme files) or from built-in defaults.
	themeApplied := false
	for _, d := range docs {
		if t, _ := d["type"].(string); t == "theme" {
			themeName, _ := d["name"].(string)
			th := &theme.Theme{Name: themeName, ParsedColors: make(map[string]color.RGBA)}
			if colors, ok := d["colors"].(map[string]any); ok {
				th.Colors = make(map[string]string, len(colors))
				for k, v := range colors {
					if s, ok := v.(string); ok {
						th.Colors[k] = s
						if c, pok := parseHexColor(s); pok {
							th.ParsedColors[k] = c
						}
					}
				}
			}
			res.RT.Theme = th
			themeApplied = true
			break
		}
	}
	if !themeApplied {
		res.RT.Theme = theme.GetDefault()
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

// parseHexColor decodes a hex string like #RRGGBB or #RRGGBBAA into
// a color.RGBA. Inline copy of theme.parseColor (unexported).
func parseHexColor(hex string) (color.RGBA, bool) {
	if len(hex) > 1 && hex[0] == '#' {
		hex = hex[1:]
	}
	var r, g, b uint8
	var a uint8 = 255
	switch len(hex) {
	case 6:
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		r = uint8(n >> 16)
		g = uint8(n >> 8)
		b = uint8(n)
	case 8:
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		r = uint8(n >> 24)
		g = uint8(n >> 16)
		b = uint8(n >> 8)
		a = uint8(n)
	default:
		return color.RGBA{}, false
	}
	return color.RGBA{r, g, b, a}, true
}
