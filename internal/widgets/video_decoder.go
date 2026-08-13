//go:build (!darwin || !cgo) && !(js && wasm)

package widgets

// startVideoDecoder is the pure-Go fallback decoder. It covers non-darwin
// hosts and darwin builds WITHOUT cgo: the native AVFoundation decoder
// (video_decoder_darwin.go) imports "C", so a CGO_ENABLED=0 cross-compile to
// darwin must not depend on it — otherwise `startVideoDecoder` is undefined
// and the whole binary fails to build (the v0.9.1 cross-compile regression).
func startVideoDecoder(v *Video, w, h int) {
	v.mu.Lock()
	if v.playing {
		v.mu.Unlock()
		return
	}
	v.playing = true
	v.mu.Unlock()

	// Default fallback to the pure Go plasma simulator on unsupported native OSes
	startFallbackDecoder(v, w, h)
}
