package canvas

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/qorm/qorm/internal/op"
)

// Rasterize evaluates a list of operations and draws them to an RGBA image.
func Rasterize(ops *op.Ops, size image.Point) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))

	// Fill with white background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

	// State
	currentColor := color.RGBA{0, 0, 0, 255}
	currentOffset := image.Point{0, 0}
	var currentClip *op.ClipOp
	currentOpacity := 1.0
	currentStrokeWidth := 0.0
	
	type state struct {
		color       color.RGBA
		offset      image.Point
		clip        *op.ClipOp
		opacity     float64
		strokeWidth float64
	}
	var stack []state

	for _, operation := range ops.Operations() {
		switch o := operation.(type) {
		case op.ColorOp:
			currentColor = o.Color
		case op.OpacityOp:
			currentOpacity = currentOpacity * o.Alpha
		case op.StrokeOp:
			currentStrokeWidth = o.Width
		case op.TransformOp:
			currentOffset = currentOffset.Add(o.Offset)
		case op.ClipOp:
			// Apply clipping region
			newClip := o
			newClip.Rect = newClip.Rect.Add(currentOffset)
			currentClip = &newClip
		case op.SaveOp:
			stack = append(stack, state{
				color:       currentColor,
				offset:      currentOffset,
				clip:        currentClip,
				opacity:     currentOpacity,
				strokeWidth: currentStrokeWidth,
			})
		case op.RestoreOp:
			if len(stack) > 0 {
				last := stack[len(stack)-1]
				currentColor = last.color
				currentOffset = last.offset
				currentClip = last.clip
				currentOpacity = last.opacity
				currentStrokeWidth = last.strokeWidth
				stack = stack[:len(stack)-1]
			}
		case op.PaintOp:
			if currentClip != nil {
				r := currentClip.Rect
				paintColor := currentColor
				if currentOpacity < 1.0 {
					paintColor.A = uint8(float64(paintColor.A) * currentOpacity)
				}
				
				if currentClip.Radius <= 0 {
					draw.Draw(img, r, &image.Uniform{paintColor}, image.Point{}, draw.Src)
				} else {
					rad := int(currentClip.Radius)
					for y := r.Min.Y; y < r.Max.Y; y++ {
						for x := r.Min.X; x < r.Max.X; x++ {
							isCorner := false
							cx, cy := 0, 0
							if x < r.Min.X+rad && y < r.Min.Y+rad {
								isCorner = true
								cx, cy = r.Min.X+rad, r.Min.Y+rad
							} else if x >= r.Max.X-rad && y < r.Min.Y+rad {
								isCorner = true
								cx, cy = r.Max.X-rad-1, r.Min.Y+rad
							} else if x < r.Min.X+rad && y >= r.Max.Y-rad {
								isCorner = true
								cx, cy = r.Min.X+rad, r.Max.Y-rad-1
							} else if x >= r.Max.X-rad && y >= r.Max.Y-rad {
								isCorner = true
								cx, cy = r.Max.X-rad-1, r.Max.Y-rad-1
							}
							if isCorner {
								dx := x - cx
								dy := y - cy
								if dx*dx+dy*dy <= rad*rad {
									img.SetRGBA(x, y, paintColor)
								}
							} else {
								img.SetRGBA(x, y, paintColor)
							}
						}
					}
				}
			}
		case op.StrokePaintOp:
			if currentClip != nil && currentStrokeWidth > 0 {
				r := currentClip.Rect
				paintColor := currentColor
				if currentOpacity < 1.0 {
					paintColor.A = uint8(float64(paintColor.A) * currentOpacity)
				}
				
				w := int(currentStrokeWidth)
				
				if currentClip.Radius <= 0 {
					// Top & Bottom
					draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), &image.Uniform{paintColor}, image.Point{}, draw.Src)
					draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), &image.Uniform{paintColor}, image.Point{}, draw.Src)
					// Left & Right
					draw.Draw(img, image.Rect(r.Min.X, r.Min.Y+w, r.Min.X+w, r.Max.Y-w), &image.Uniform{paintColor}, image.Point{}, draw.Src)
					draw.Draw(img, image.Rect(r.Max.X-w, r.Min.Y+w, r.Max.X, r.Max.Y-w), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				} else {
					rad := int(currentClip.Radius)
					rad2 := rad * rad
					innerRad := rad - w
					innerRad2 := innerRad * innerRad
					
					for y := r.Min.Y; y < r.Max.Y; y++ {
						for x := r.Min.X; x < r.Max.X; x++ {
							isCorner := false
							cx, cy := 0, 0
							if x < r.Min.X+rad && y < r.Min.Y+rad {
								isCorner = true
								cx, cy = r.Min.X+rad, r.Min.Y+rad
							} else if x >= r.Max.X-rad && y < r.Min.Y+rad {
								isCorner = true
								cx, cy = r.Max.X-rad-1, r.Min.Y+rad
							} else if x < r.Min.X+rad && y >= r.Max.Y-rad {
								isCorner = true
								cx, cy = r.Min.X+rad, r.Max.Y-rad-1
							} else if x >= r.Max.X-rad && y >= r.Max.Y-rad {
								isCorner = true
								cx, cy = r.Max.X-rad-1, r.Max.Y-rad-1
							}
							if isCorner {
								dx := x - cx
								dy := y - cy
								dist2 := dx*dx + dy*dy
								if dist2 <= rad2 && dist2 >= innerRad2 {
									img.SetRGBA(x, y, paintColor)
								}
							} else {
								// Edge check
								if x < r.Min.X+w || x >= r.Max.X-w || y < r.Min.Y+w || y >= r.Max.Y-w {
									img.SetRGBA(x, y, paintColor)
								}
							}
						}
					}
				}
			}
		case op.TextOp:
			pos := o.Pos.Add(currentOffset)
			scale := o.Scale
			if scale < 1 {
				scale = 1
			}
			DrawText(img, o.Text, pos, currentColor, scale)
		}
	}

	return img
}
