package widgets

import (
	"image"
	"image/color"
	"math"
	"time"
)

func startFallbackDecoder(v *Video, w, h int) {
	go func() {
		tick := time.NewTicker(time.Millisecond * 33) // ~30 FPS
		defer tick.Stop()
		t := 0
		
		bg := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				bg.SetRGBA(x, y, color.RGBA{
					R: uint8(x * 255 / w),
					G: uint8(y * 255 / h),
					B: 128,
					A: 255,
				})
			}
		}

		for range tick.C {
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			copy(img.Pix, bg.Pix)
			
			bx := (t * 5) % (w * 2)
			if bx > w {
				bx = w*2 - bx
			}
			by := int(float64(h)/2 + math.Sin(float64(t)*0.1)*float64(h)/3)
			
			for y := by - 25; y < by + 25; y++ {
				for x := bx - 25; x < bx + 25; x++ {
					if x >= 0 && x < w && y >= 0 && y < h {
						img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
					}
				}
			}
			t++
			v.AppendFrame(img)
		}
	}()
}
