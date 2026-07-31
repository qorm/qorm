package canvas

import (
	"image"

	"github.com/qorm/qorm/internal/op"
)

// Rasterize evaluates a display list into a freshly allocated RGBA image.
// Convenience wrapper over SoftwareRenderer for one-shot rendering; the frame
// loop renders into a persistent Surface buffer via Renderer directly.
func Rasterize(ops *op.Ops, size image.Point) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	SoftwareRenderer{}.Render(ops, img)
	return img
}
