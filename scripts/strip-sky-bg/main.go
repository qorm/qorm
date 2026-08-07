// strip-sky-bg processes mavis sprite assets: downscales to a target pixel
// size (default 16, the cellSize) and turns any pixel that matches the
// stage sky color into a transparent one so the sprite composites cleanly
// on top of the var(--sky) board background.
//
// Usage: strip-sky-bg [--size N] <png>...
//
// The downscale uses nearest-neighbour (sprite art, no anti-aliasing);
// alpha-zero output keeps the result a true RGBA PNG.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

const (
	skyR = 0x5C
	skyG = 0x94
	skyB = 0xFC
	tol  = 64 // generous tolerance: the AI sprite generator blends the
	// sky background into the sprite's antialiased edges, so a hard
	// #5C94FC match leaves a visible halo. The cap at 64 (out of 255)
	// is wide enough to cover the blend without swallowing actual
	// sprite pixels (mario's red, brick's brown, etc. are all
	// further than 64 from sky on at least one channel).
)

func main() {
	size := flag.Int("size", 16, "downscale target edge length (px)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: strip-sky-bg [--size N] <png>...")
		os.Exit(2)
	}
	for _, p := range flag.Args() {
		if err := process(p, *size); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", p, err)
			os.Exit(1)
		}
		fmt.Printf("processed: %s\n", p)
	}
}

func process(path string, size int) error {
	img, err := decodeAny(path)
	if err != nil {
		return err
	}
	// The AI sprite generator emits 1024x1024 PNGs (or sometimes JPEGs
	// wrapped in a .png name); downscale to the cell size first so the
	// engine's image cache stores the cheap small bitmap.
	small := downscaleNearest(img, size, size)
	// Walk the small bitmap, leaving sky pixels transparent.
	b := small.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := small.At(x, y)
			r, g, bl, a := c.RGBA()
			if a == 0 {
				continue
			}
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			dr := int(r8) - skyR
			dg := int(g8) - skyG
			db := int(b8) - skyB
			if dr*dr+dg*dg+db*db <= tol*tol {
				continue // alpha stays 0
			}
			out.SetRGBA(x, y, color.RGBA{r8, g8, b8, 255})
		}
	}
	return writePNG(path, out)
}

func decodeAny(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err == nil {
		return img, nil
	}
	// Fall back: the file may be a JPEG saved with a .png suffix.
	f2, err2 := os.Open(path)
	if err2 != nil {
		return nil, err
	}
	defer f2.Close()
	if j, _, jerr := image.Decode(f2); jerr == nil {
		return j, nil
	} else {
		return nil, fmt.Errorf("decode (also tried jpeg): %v / %v", err, jerr)
	}
}

func downscaleNearest(src image.Image, w, h int) image.Image {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// NEAREST (no filtering): pixel art needs hard edges, otherwise
	// neighbour-averaging washes out the 1-px NES detail.
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*srcH/h
		for x := 0; x < w; x++ {
			sx := sb.Min.X + x*srcW/w
			r, g, bl, a := src.At(sx, sy).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

func writePNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	f.Close()
	if !strings.HasSuffix(path, ".png") {
		_ = os.Remove(tmp)
		return fmt.Errorf("not a .png: %s", path)
	}
	return os.Rename(tmp, path)
}
