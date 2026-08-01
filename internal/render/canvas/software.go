package canvas

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/qorm/qorm/internal/op"
)

// SoftwareRenderer is the CPU rasterizer: a flat interpreter over the display
// list drawing into an image.RGBA. Stateless — all pipeline state is scoped to
// a single Render call.
//
// Buffer convention: the target image.RGBA holds STRAIGHT (non-premultiplied)
// RGBA bytes — every write goes through blendOver (or the opaque fast path,
// which is identical for straight vs premultiplied), and the platform Surface
// declares the matching non-premultiplied bitmap format. This keeps the alpha
// math unambiguous: blends are computed in straight space in Go and never rely
// on draw.Over's premultiplied assumptions.
//
// Remaining limits (later milestones): the 5x7 bitmap font has no
// shaping/kerning/wrapping.
type SoftwareRenderer struct{}

// Render implements Renderer.
func (SoftwareRenderer) Render(ops *op.Ops, img *image.RGBA) {
	// Every frame starts from a clean white background (opaque → straight).
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

	// Pipeline state. Clips form a stack so nested containers intersect
	// (Save/Restore snapshot the stack; a ClipOp pushes on top).
	currentColor := color.RGBA{0, 0, 0, 255}
	currentOffset := image.Point{0, 0}
	var clips []op.ClipOp
	currentOpacity := 1.0
	currentStrokeWidth := 0.0

	type state struct {
		color       color.RGBA
		offset      image.Point
		clips       []op.ClipOp
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
			pushed := o
			pushed.Rect = pushed.Rect.Add(currentOffset)
			clips = append(clips, pushed)
		case op.SaveOp:
			stack = append(stack, state{
				color:       currentColor,
				offset:      currentOffset,
				clips:       append([]op.ClipOp(nil), clips...),
				opacity:     currentOpacity,
				strokeWidth: currentStrokeWidth,
			})
		case op.RestoreOp:
			if len(stack) > 0 {
				last := stack[len(stack)-1]
				currentColor = last.color
				currentOffset = last.offset
				clips = last.clips
				currentOpacity = last.opacity
				currentStrokeWidth = last.strokeWidth
				stack = stack[:len(stack)-1]
			}
		case op.PaintOp:
			if len(clips) == 0 {
				break
			}
			paintColor := withOpacity(currentColor, currentOpacity)
			r := clipBounds(clips, img)
			if r.Empty() {
				break
			}
			if paintColor.A == 255 && len(clips) == 1 && clips[0].Radius <= 0 {
				// Opaque, single rectangular clip: straight == premultiplied,
				// so the vectorized draw is correct and fast.
				draw.Draw(img, r, &image.Uniform{paintColor}, image.Point{}, draw.Src)
			} else {
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						if inAllClips(x, y, clips) {
							blendOver(img, x, y, paintColor)
						}
					}
				}
			}
		case op.StrokePaintOp:
			if len(clips) == 0 || currentStrokeWidth <= 0 {
				break
			}
			paintColor := withOpacity(currentColor, currentOpacity)
			r := clipBounds(clips, img)
			if r.Empty() {
				break
			}
			w := int(currentStrokeWidth)
			inner := clips[len(clips)-1] // the shape's own clip defines the stroke geometry
			if paintColor.A == 255 && len(clips) == 1 && inner.Radius <= 0 {
				// Opaque rectangular outline: four edges.
				draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Min.X, r.Min.Y+w, r.Min.X+w, r.Max.Y-w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Max.X-w, r.Min.Y+w, r.Max.X, r.Max.Y-w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
			} else {
				ir := inner.Rect
				rad := int(inner.Radius)
				rad2 := rad * rad
				innerRad := rad - w
				innerRad2 := innerRad * innerRad
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						if !inAllClips(x, y, clips) {
							continue
						}
						onStroke := false
						if rad <= 0 {
							onStroke = x < ir.Min.X+w || x >= ir.Max.X-w || y < ir.Min.Y+w || y >= ir.Max.Y-w
						} else {
							cx, cy, corner := cornerCenter(x, y, ir, rad)
							if corner {
								d2 := (x-cx)*(x-cx) + (y-cy)*(y-cy)
								onStroke = d2 <= rad2 && d2 >= innerRad2
							} else {
								onStroke = x < ir.Min.X+w || x >= ir.Max.X-w || y < ir.Min.Y+w || y >= ir.Max.Y-w
							}
						}
						if onStroke {
							blendOver(img, x, y, paintColor)
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
			DrawText(img, o.Text, pos, withOpacity(currentColor, currentOpacity), scale, clips)
		case op.ImageOp:
			if o.Src == nil {
				break
			}
			dest := o.Dest.Add(currentOffset)
			sb := o.Src.Bounds()
			sw, sh := sb.Dx(), sb.Dy()
			dw, dh := dest.Dx(), dest.Dy()
			if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
				break
			}
			r := dest.Intersect(img.Bounds())
			if len(clips) > 0 {
				r = r.Intersect(clipBounds(clips, img))
			}
			if r.Empty() {
				break
			}
			// Nearest-neighbour sampling: integer-only, deterministic and
			// exact for the pixel-assertion tests. Bilinear would buy smooth
			// downscales but needs premultiplied-space interpolation (the
			// buffer is straight) and a fixed-point pipeline — deferred until
			// a real use case demands it.
			for y := r.Min.Y; y < r.Max.Y; y++ {
				sy := sb.Min.Y + (y-dest.Min.Y)*sh/dh
				for x := r.Min.X; x < r.Max.X; x++ {
					if !inAllClips(x, y, clips) {
						continue
					}
					sx := sb.Min.X + (x-dest.Min.X)*sw/dw
					c := o.Src.RGBAAt(sx, sy)
					if currentOpacity < 1.0 {
						c = withOpacity(c, currentOpacity)
					}
					blendOver(img, x, y, c)
				}
			}
		case op.RRectOp:
			rect := o.Rect.Add(currentOffset)
			hw, hh := float64(rect.Dx())/2, float64(rect.Dy())/2
			if hw <= 0 || hh <= 0 {
				break
			}
			radius := clamp01(o.Radius/math.Min(hw, hh)) * math.Min(hw, hh)
			cx := float64(rect.Min.X) + hw
			cy := float64(rect.Min.Y) + hh

			// Iteration bounds: the fill's ~1px AA band lives just outside
			// Rect; the shadow extends ShadowBlur px beyond its ShadowY offset.
			minX, minY := float64(rect.Min.X)-1, float64(rect.Min.Y)-1
			maxX, maxY := float64(rect.Max.X)+1, float64(rect.Max.Y)+1
			if o.Shadow.A > 0 {
				minX = math.Min(minX, float64(rect.Min.X)-o.ShadowBlur-1)
				minY = math.Min(minY, float64(rect.Min.Y)+o.ShadowY-o.ShadowBlur-1)
				maxX = math.Max(maxX, float64(rect.Max.X)+o.ShadowBlur+1)
				maxY = math.Max(maxY, float64(rect.Max.Y)+o.ShadowY+o.ShadowBlur+1)
			}
			b := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY)))
			b = b.Intersect(img.Bounds())
			if len(clips) > 0 {
				b = b.Intersect(clipBounds(clips, img))
			}
			if b.Empty() {
				break
			}

			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if !inAllClips(x, y, clips) {
						continue
					}
					px, py := float64(x)+0.5, float64(y)+0.5
					if o.Shadow.A > 0 {
						d := sdRoundBox(px, py, cx, cy+o.ShadowY, hw, hh, radius)
						var cov float64
						if o.ShadowBlur > 0 {
							// Smoothstep falloff over the blur width: zero slope
							// at both ends, so the shadow melts out instead of
							// stepping like the old concentric-rect fake.
							t := clamp01(1 - d/o.ShadowBlur)
							cov = t * t * (3 - 2*t)
						} else {
							cov = clamp01(0.5 - d)
						}
						if cov > 0 {
							blendOver(img, x, y, withOpacity(o.Shadow, cov*currentOpacity))
						}
					}
					d := sdRoundBox(px, py, cx, cy, hw, hh, radius)
					if o.Fill.A > 0 {
						// ~1px coverage band at the boundary = antialiased edge.
						if cov := clamp01(0.5 - d); cov > 0 {
							blendOver(img, x, y, withOpacity(o.Fill, cov*currentOpacity))
						}
					}
					if o.Stroke.A > 0 && o.StrokeWidth > 0 {
						// Stroke sits INSIDE the boundary (CSS border-box):
						// outer AA at d≈0, inner AA at d≈-width.
						if cov := clamp01(0.5-d) * clamp01(0.5+d+o.StrokeWidth); cov > 0 {
							blendOver(img, x, y, withOpacity(o.Stroke, cov*currentOpacity))
						}
					}
				}
			}
		}
	}
}

// withOpacity returns c with its alpha multiplied by op (straight color).
func withOpacity(c color.RGBA, op float64) color.RGBA {
	if op >= 1.0 {
		return c
	}
	if op <= 0.0 {
		return color.RGBA{}
	}
	c.A = uint8(float64(c.A) * op)
	return c
}

// clipBounds returns the intersection of all clip rects with the image bounds:
// the bounding box a paint can possibly touch (per-pixel rounding still tested
// by inAllClips).
func clipBounds(clips []op.ClipOp, img *image.RGBA) image.Rectangle {
	r := clips[0].Rect
	for _, c := range clips[1:] {
		r = r.Intersect(c.Rect)
	}
	return r.Intersect(img.Bounds())
}

// inAllClips reports whether (x,y) lies inside every clip, each clip's rounded
// corners included — this is what makes nested clips intersect.
func inAllClips(x, y int, clips []op.ClipOp) bool {
	for _, c := range clips {
		r := c.Rect
		if x < r.Min.X || x >= r.Max.X || y < r.Min.Y || y >= r.Max.Y {
			return false
		}
		if rad := int(c.Radius); rad > 0 {
			if cx, cy, corner := cornerCenter(x, y, r, rad); corner {
				dx, dy := x-cx, y-cy
				if dx*dx+dy*dy > rad*rad {
					return false
				}
			}
		}
	}
	return true
}

// cornerCenter returns the centre of the rounded corner that (x,y) falls into,
// and whether (x,y) is in a corner quadrant at all (straight edges are not).
func cornerCenter(x, y int, r image.Rectangle, rad int) (int, int, bool) {
	switch {
	case x < r.Min.X+rad && y < r.Min.Y+rad:
		return r.Min.X + rad, r.Min.Y + rad, true
	case x >= r.Max.X-rad && y < r.Min.Y+rad:
		return r.Max.X - rad - 1, r.Min.Y + rad, true
	case x < r.Min.X+rad && y >= r.Max.Y-rad:
		return r.Min.X + rad, r.Max.Y - rad - 1, true
	case x >= r.Max.X-rad && y >= r.Max.Y-rad:
		return r.Max.X - rad - 1, r.Max.Y - rad - 1, true
	}
	return 0, 0, false
}

// blendOver composites the straight source color c over the pixel at (x,y)
// using Porter-Duff src-over in straight space. Opaque sources replace.
func blendOver(img *image.RGBA, x, y int, c color.RGBA) {
	if c.A == 0 {
		return
	}
	if c.A == 255 {
		img.SetRGBA(x, y, c)
		return
	}
	sa := uint32(c.A)
	d := img.RGBAAt(x, y)
	da := uint32(d.A)
	outA := sa + da*(255-sa)/255
	if outA == 0 {
		img.SetRGBA(x, y, color.RGBA{})
		return
	}
	outR := (uint32(c.R)*sa + uint32(d.R)*da*(255-sa)/255) / outA
	outG := (uint32(c.G)*sa + uint32(d.G)*da*(255-sa)/255) / outA
	outB := (uint32(c.B)*sa + uint32(d.B)*da*(255-sa)/255) / outA
	img.SetRGBA(x, y, color.RGBA{uint8(outR), uint8(outG), uint8(outB), uint8(outA)})
}

// sdRoundBox returns the signed distance from point (px,py) to the boundary
// of a box centered at (cx,cy) with half extents (hw,hh) and corner radius r
// — negative inside, positive outside (iq's rounded-box SDF).
func sdRoundBox(px, py, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(px-cx) - (hw - r)
	qy := math.Abs(py-cy) - (hh - r)
	ox := math.Max(qx, 0)
	oy := math.Max(qy, 0)
	return math.Min(math.Max(qx, qy), 0) + math.Hypot(ox, oy) - r
}
