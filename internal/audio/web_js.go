//go:build js && wasm

package audio

import (
	"fmt"
	"sync"
	"syscall/js"
)

// WebSink plays WAVs in the browser via HTMLAudioElement.
//
//	playMusic → one looping element (replaced on the next PlaySrc loop)
//	playSound → fire-and-forget one-shot elements (do not interrupt music)
//	stopMusic → Stop() pauses/clears the music element only
//
// Autoplay policy: if play() rejects (no user gesture yet), log once and
// return nil — games keep running silent until the player taps.
type WebSink struct {
	mu             sync.Mutex
	music          js.Value // HTMLAudioElement or Undefined
	autoplayLogged bool
}

// PlaySrc starts playback of a browser-resolvable URL (BaseDir + relative src).
// loop=true is background music; loop=false is a one-shot SFX.
func (w *WebSink) PlaySrc(url string, loop bool) error {
	if url == "" {
		return fmt.Errorf("audio: empty url")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	audioCtor := js.Global().Get("Audio")
	if audioCtor.IsUndefined() || audioCtor.IsNull() {
		return fmt.Errorf("audio: HTMLAudioElement not available")
	}

	if loop {
		w.stopMusicLocked()
		el := audioCtor.New(url)
		el.Set("loop", true)
		w.music = el
		w.startLocked(el)
		return nil
	}
	el := audioCtor.New(url)
	w.startLocked(el)
	return nil
}

// Play implements Player for decoded PCM by handing a blob: URL to PlaySrc.
// The web runtime prefers PlaySrc(URL) so the browser fetches the WAV
// directly; this path is a fallback for callers that only have a Sound.
func (w *WebSink) Play(s *Sound, loop bool) error {
	if s == nil {
		return fmt.Errorf("audio: nil sound")
	}
	url, err := soundObjectURL(s)
	if err != nil {
		return err
	}
	return w.PlaySrc(url, loop)
}

// Stop implements Player — stops looping music only (one-shots drain alone).
func (w *WebSink) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopMusicLocked()
	return nil
}

func (w *WebSink) stopMusicLocked() {
	if !w.music.Truthy() {
		return
	}
	// Best-effort teardown; any throw is recovered so Stop never panics.
	func() {
		defer func() { _ = recover() }()
		w.music.Call("pause")
		w.music.Set("currentTime", 0)
		w.music.Set("src", "")
	}()
	w.music = js.Undefined()
}

// startLocked calls el.play() and attaches a one-shot catch for autoplay
// rejections. Caller holds w.mu.
func (w *WebSink) startLocked(el js.Value) {
	var p js.Value
	func() {
		defer func() { _ = recover() }()
		p = el.Call("play")
	}()
	if !p.Truthy() {
		return
	}
	// play() returns a Promise in modern browsers.
	then := p.Get("then")
	if !then.Truthy() {
		return
	}
	var catchCb js.Func
	catchCb = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		catchCb.Release()
		w.mu.Lock()
		defer w.mu.Unlock()
		if !w.autoplayLogged {
			w.autoplayLogged = true
			fmt.Println("[qorm audio] play blocked until user gesture (autoplay policy)")
		}
		return nil
	})
	func() {
		defer func() { _ = recover() }()
		p.Call("catch", catchCb)
	}()
}

// soundObjectURL encodes s as a WAV blob URL for HTMLAudioElement.
func soundObjectURL(s *Sound) (string, error) {
	// encodeWAV needs an io.Writer; build the full file in memory.
	n := 44 + len(s.Samples)
	buf := make([]byte, 0, n)
	// Use a thin writer over a growing slice via appendWriter.
	aw := &appendWriter{b: buf}
	if err := encodeWAV(aw, s); err != nil {
		return "", err
	}
	data := aw.b

	uint8 := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(uint8, data)
	// Blob parts must be a JS array of ArrayBufferView.
	parts := js.Global().Get("Array").New(1)
	parts.SetIndex(0, uint8)
	opts := js.Global().Get("Object").New()
	opts.Set("type", "audio/wav")
	blob := js.Global().Get("Blob").New(parts, opts)
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	if url.Type() != js.TypeString {
		return "", fmt.Errorf("audio: createObjectURL failed")
	}
	return url.String(), nil
}

type appendWriter struct{ b []byte }

func (w *appendWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
