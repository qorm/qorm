//go:build !darwin && !(js && wasm)

package widgets

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
