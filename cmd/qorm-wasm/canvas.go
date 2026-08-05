//go:build js && wasm && qorm_canvas

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"syscall/js"

	"github.com/qorm/qorm/internal/playcore"
	"github.com/qorm/qorm/internal/render/canvas"
)

var (
	cvsEngine  *canvas.Engine
	cvsSurface *canvas.HeadlessSurface
)

func init() {
	js.Global().Set("qormCanvasInit", js.FuncOf(qormCanvasInit))
	js.Global().Set("qormCanvasFrame", js.FuncOf(qormCanvasFrame))
	js.Global().Set("qormCanvasPtr", js.FuncOf(qormCanvasPtr))
	js.Global().Set("qormCanvasKey", js.FuncOf(qormCanvasKey))
	js.Global().Set("qormCanvasInitFromBundle", js.FuncOf(qormCanvasInitFromBundle))
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
	cvsSurface.ScaleFactor = 2
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

	pixels := js.Global().Get("Uint8ClampedArray").New(len(buf.Pix))
	js.CopyBytesToJS(pixels, buf.Pix)
	imgData := js.Global().Get("ImageData").New(pixels, w, h)

	canvasEl := args[0]
	lw, lh := w/2, h/2 // logical pixels at 2x scale
	canvasEl.Set("width", lw)
	canvasEl.Set("height", lh)
	canvasEl.Get("style").Set("width", fmt.Sprintf("%dpx", lw))
	canvasEl.Get("style").Set("height", fmt.Sprintf("%dpx", lh))
	ctx := canvasEl.Call("getContext", "2d")
	ctx.Call("putImageData", imgData, 0, 0)
	return true
}

// qormCanvasPtr(typ, x, y) forwards a pointer event.
func qormCanvasPtr(_ js.Value, args []js.Value) any {
	if cvsEngine == nil || len(args) < 3 { return false }
	typ := args[0].String()
	x, y := args[1].Float(), args[2].Float()
	var pt canvas.PointerType
	switch typ {
	case "press":  pt = canvas.PointerPress
	case "release": pt = canvas.PointerRelease
	default:       pt = canvas.PointerMove
	}
	return cvsEngine.HandlePointer(canvas.PointerInput{Type: pt, X: x, Y: y, Buttons: 1})
}

// qormCanvasKey(key, down) forwards a keyboard event.
func qormCanvasKey(_ js.Value, args []js.Value) any {
	if cvsEngine == nil || len(args) < 1 { return false }
	key := args[0].String()
	down := true
	if len(args) > 1 { down = args[1].Bool() }
	return cvsEngine.HandleKey(canvas.KeyInput{Key: key, Down: down})
}

// qormCanvasInitFromBundle(docsJSON) compiles a doc array and prepares the
// canvas engine in one call.
func qormCanvasInitFromBundle(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return map[string]any{"err": "missing docs JSON"}
	}
	var docs []map[string]any
	if err := json.Unmarshal([]byte(args[0].String()), &docs); err != nil {
		return map[string]any{"err": "invalid JSON: " + err.Error()}
	}
	res := playcore.CompileDocs(docs)
	if len(res.Diagnostics) > 0 && res.HTML == "" {
		return map[string]any{"err": "compile failed", "diagnostics": res.Diagnostics}
	}
	adopt(res.RT)
	handlers = res.Handlers

	size := image.Pt(420, 680)
	cvsSurface = canvas.NewHeadlessSurface(size)
	cvsSurface.Logical = size
	cvsSurface.ScaleFactor = 2
	cvsEngine = canvas.NewEngine(res.RT, canvas.SoftwareRenderer{})
	return nil
}
