//go:build js && wasm

package widgets

import (
	"image"
	"syscall/js"
)

func startVideoDecoder(v *Video, w, h int) {
	v.mu.Lock()
	if v.playing {
		v.mu.Unlock()
		return
	}
	v.playing = true
	src := v.src
	v.mu.Unlock()

	if src == "" {
		startFallbackDecoder(v, w, h)
		return
	}

	doc := js.Global().Get("document")
	video := doc.Call("createElement", "video")
	video.Set("src", src)
	video.Set("autoplay", true)
	video.Set("loop", true)
	video.Set("muted", true) // per user request:只做视频，不考虑音频
	video.Set("crossOrigin", "anonymous")

	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", w)
	canvas.Set("height", h)
	ctx := canvas.Call("getContext", "2d", map[string]interface{}{
		"willReadFrequently": true,
	})

	frameSize := w * h * 4
	buf := make([]byte, frameSize)

	var render js.Func
	render = js.FuncOf(func(this js.Value, args []js.Value) any {
		// readyState >= 2 (HAVE_CURRENT_DATA)
		if video.Get("readyState").Int() >= 2 {
			ctx.Call("drawImage", video, 0, 0, w, h)
			imgData := ctx.Call("getImageData", 0, 0, w, h)
			data := imgData.Get("data")
			js.CopyBytesToGo(buf, data)

			img := image.NewRGBA(image.Rect(0, 0, w, h))
			copy(img.Pix, buf)
			v.AppendFrame(img)
		}
		js.Global().Call("requestAnimationFrame", render)
		return nil
	})

	video.Call("play")
	js.Global().Call("requestAnimationFrame", render)
}
