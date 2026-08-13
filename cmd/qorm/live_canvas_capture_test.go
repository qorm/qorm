//go:build !desktop && darwin

package main

import (
	"image"
	"image/color"
	"testing"
)

func TestEncodeLiveCanvasCaptureFullSurfaceAndNodeClip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 60))
	img.SetRGBA(4, 5, color.RGBA{R: 250, G: 20, B: 10, A: 255})
	measure := []byte(`[
		{"id":"card","x":-2,"y":5,"w":20,"h":15,"visible":true},
		{"id":"hidden","x":1,"y":1,"w":10,"h":10,"visible":false}
	]`)
	captured, err := encodeLiveCanvasCapture(img, 2, measure, "card")
	if err != nil {
		t.Fatal(err)
	}
	if captured.Width != 80 || captured.Height != 60 || captured.Scale != 2 || len(captured.PNG) < 8 {
		t.Fatalf("capture metadata = %+v, png bytes=%d", captured, len(captured.PNG))
	}
	if got := captured.Clip; got == nil || got.X != 0 || got.Y != 5 || got.W != 18 || got.H != 15 || !got.ClippedToSurface {
		t.Fatalf("clip = %+v", got)
	}
	if _, err := encodeLiveCanvasCapture(img, 1, measure, "hidden"); err == nil {
		t.Fatal("invisible node must fail loudly")
	}
	if _, err := encodeLiveCanvasCapture(img, 1, measure, "missing"); err == nil {
		t.Fatal("missing node must fail loudly")
	}
}
