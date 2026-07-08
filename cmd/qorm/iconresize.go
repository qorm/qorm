package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
)

// resizePNG scales a PNG to n×n pixels with bilinear sampling. Standard
// library only — plan decision 3 (v0.2.0) keeps go.mod dependency-free
// instead of pulling in golang.org/x/image/draw.
func resizePNG(src []byte, n int) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("resize png: invalid target size %d", n)
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("resize png: %w", err)
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return nil, fmt.Errorf("resize png: empty source image")
	}
	// px samples one source pixel as premultiplied 16-bit RGBA (interpolating
	// premultiplied values keeps transparent edges from bleeding dark halos).
	px := func(x, y int) (r, g, bl, a float64) {
		pr, pg, pb, pa := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return float64(pr), float64(pg), float64(pb), float64(pa)
	}
	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(n) - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		y0, y1 := clamp(y0, sh-1), clamp(y0+1, sh-1)
		for x := 0; x < n; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(n) - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			x0, x1 := clamp(x0, sw-1), clamp(x0+1, sw-1)
			r00, g00, b00, a00 := px(x0, y0)
			r10, g10, b10, a10 := px(x1, y0)
			r01, g01, b01, a01 := px(x0, y1)
			r11, g11, b11, a11 := px(x1, y1)
			lerp2 := func(v00, v10, v01, v11 float64) uint8 {
				top := v00 + (v10-v00)*tx
				bot := v01 + (v11-v01)*tx
				return uint8((top+(bot-top)*ty)/257.0 + 0.5)
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o+0] = lerp2(r00, r10, r01, r11)
			dst.Pix[o+1] = lerp2(g00, g10, g01, g11)
			dst.Pix[o+2] = lerp2(b00, b10, b01, b11)
			dst.Pix[o+3] = lerp2(a00, a10, a01, a11)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("resize png: %w", err)
	}
	return out.Bytes(), nil
}

// paddedPNG scales src to content×content and centers it on a transparent
// canvas×canvas image — the layout Android adaptive-icon foregrounds need
// (content well inside the canvas so launcher masks don't clip it).
func paddedPNG(src []byte, canvas, content int) ([]byte, error) {
	if content <= 0 || canvas < content {
		return nil, fmt.Errorf("pad png: invalid canvas %d / content %d", canvas, content)
	}
	inner, err := resizePNG(src, content)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(inner))
	if err != nil {
		return nil, fmt.Errorf("pad png: %w", err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	off := (canvas - content) / 2
	draw.Draw(dst, image.Rect(off, off, off+content, off+content), img, img.Bounds().Min, draw.Over)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("pad png: %w", err)
	}
	return out.Bytes(), nil
}
