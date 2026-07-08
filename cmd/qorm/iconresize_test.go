package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// solidPNG encodes a w×h PNG filled with c.
func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return img
}

func TestResizePNGSizes(t *testing.T) {
	src := solidPNG(t, 100, 100, color.NRGBA{R: 200, G: 40, B: 10, A: 255})
	for _, n := range []int{48, 72, 96, 144, 192, 432} {
		out, err := resizePNG(src, n)
		if err != nil {
			t.Fatalf("resize to %d: %v", n, err)
		}
		img := decodePNG(t, out)
		if img.Bounds().Dx() != n || img.Bounds().Dy() != n {
			t.Fatalf("resize to %d: got %v", n, img.Bounds())
		}
		// a solid source must stay (approximately) that color
		r, g, b, a := img.At(n/2, n/2).RGBA()
		if r>>8 != 200 || g>>8 != 40 || b>>8 != 10 || a>>8 != 255 {
			t.Fatalf("resize to %d: center pixel = %d,%d,%d,%d", n, r>>8, g>>8, b>>8, a>>8)
		}
	}
}

func TestResizePNGUpscale(t *testing.T) {
	src := solidPNG(t, 4, 4, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	out, err := resizePNG(src, 64)
	if err != nil {
		t.Fatalf("upscale: %v", err)
	}
	if img := decodePNG(t, out); img.Bounds().Dx() != 64 {
		t.Fatalf("upscale: got %v", img.Bounds())
	}
}

func TestResizePNGRejectsNonPNG(t *testing.T) {
	if _, err := resizePNG([]byte("definitely not a png"), 48); err == nil {
		t.Fatal("want error for non-PNG input")
	}
	if _, err := resizePNG(nil, 48); err == nil {
		t.Fatal("want error for nil input")
	}
}

func TestResizePNGRejectsBadSize(t *testing.T) {
	src := solidPNG(t, 8, 8, color.NRGBA{A: 255})
	for _, n := range []int{0, -1} {
		if _, err := resizePNG(src, n); err == nil {
			t.Fatalf("want error for size %d", n)
		}
	}
}

func TestPaddedPNG(t *testing.T) {
	src := solidPNG(t, 64, 64, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	out, err := paddedPNG(src, 432, 432*66/100)
	if err != nil {
		t.Fatalf("pad: %v", err)
	}
	img := decodePNG(t, out)
	if img.Bounds().Dx() != 432 || img.Bounds().Dy() != 432 {
		t.Fatalf("pad: got %v", img.Bounds())
	}
	if _, _, _, a := img.At(2, 2).RGBA(); a != 0 {
		t.Fatalf("pad: corner should be transparent, alpha=%d", a)
	}
	if _, _, _, a := img.At(216, 216).RGBA(); a>>8 != 255 {
		t.Fatalf("pad: center should be opaque, alpha=%d", a>>8)
	}
}

func TestPaddedPNGRejectsBadGeometry(t *testing.T) {
	src := solidPNG(t, 8, 8, color.NRGBA{A: 255})
	if _, err := paddedPNG(src, 100, 200); err == nil {
		t.Fatal("want error when content exceeds canvas")
	}
	if _, err := paddedPNG(src, 100, 0); err == nil {
		t.Fatal("want error for zero content")
	}
}
