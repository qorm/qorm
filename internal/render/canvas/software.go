package canvas

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/qorm/qorm/internal/geom"
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
	// (Save/Restore snapshot the stack; a ClipOp pushes on top). The transform
	// is a full affine matrix: geometry ops post-multiply it and transform
	// their integer coordinates into screen space (matrixScale reads the
	// uniform scale so stroke widths, text font scale and corner radii follow
	// a zoomed board).
	currentColor := color.RGBA{0, 0, 0, 255}
	currentMatrix := geom.Identity()
	var clips []op.ClipOp
	currentOpacity := 1.0
	currentStrokeWidth := 0.0

	type state struct {
		color       color.RGBA
		matrix      geom.Matrix
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
			currentMatrix = currentMatrix.Multiply(o.M)
		case op.ClipOp:
			pushed := o
			pushed.Rect = transformRect(currentMatrix, o.Rect)
			pushed.Radius = o.Radius * matrixScale(currentMatrix)
			clips = append(clips, pushed)
		case op.SaveOp:
			stack = append(stack, state{
				color:       currentColor,
				matrix:      currentMatrix,
				clips:       append([]op.ClipOp(nil), clips...),
				opacity:     currentOpacity,
				strokeWidth: currentStrokeWidth,
			})
		case op.RestoreOp:
			if len(stack) > 0 {
				last := stack[len(stack)-1]
				currentColor = last.color
				currentMatrix = last.matrix
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
						cov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
						if cov > 0 {
							blendOver(img, x, y, withOpacity(paintColor, cov))
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
			// The stroke width is authored in local units; under a scaled
			// matrix (a zoomed board) it must scale with the geometry, the way
			// a CSS border does under transform.
			w := int(currentStrokeWidth * matrixScale(currentMatrix))
			inner := clips[len(clips)-1] // the shape's own clip defines the stroke geometry
			// A sub-pixel stroke (zoom < 1x) falls through to the SDF branch,
			// whose float coverage renders it as a faint band — the integer
			// edge path below would draw a zero-width (invisible) border.
			if w >= 1 && paintColor.A == 255 && len(clips) == 1 && inner.Radius <= 0 {
				// Opaque rectangular outline: four edges.
				draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Min.X, r.Min.Y+w, r.Min.X+w, r.Max.Y-w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
				draw.Draw(img, image.Rect(r.Max.X-w, r.Min.Y+w, r.Max.X, r.Max.Y-w).Intersect(img.Bounds()), &image.Uniform{paintColor}, image.Point{}, draw.Src)
			} else {
				// SDF stroke: the same border-box coverage RRectOp's stroke
				// uses — an antialiased band at the shape boundary, the inner
				// edge one stroke-width in. This replaces the old binary
				// in/on test, whose rounded corners and semi-transparent
				// strokes stair-stepped. The innermost clip defines the
				// shape; the outer (container) clips still gate each pixel.
				ir := inner.Rect
				hw, hh := float64(ir.Dx())/2, float64(ir.Dy())/2
				if hw <= 0 || hh <= 0 {
					break
				}
				rad := math.Min(inner.Radius, math.Min(hw, hh))
				cx := float64(ir.Min.X) + hw
				cy := float64(ir.Min.Y) + hh
				// Stroke width authored in local units: scale it with the
				// matrix (a zoomed board) so the band stays proportional.
				sw := currentStrokeWidth * matrixScale(currentMatrix)
				outer := clips[:len(clips)-1]
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						px, py := float64(x)+0.5, float64(y)+0.5
						d := sdRoundBox(px, py, cx, cy, hw, hh, rad)
						cov := clamp01(0.5-d) * clamp01(0.5+d+sw)
						if cov <= 0 {
							continue
						}
						cov *= clipCoverage(px, py, outer)
						if cov > 0 {
							blendOver(img, x, y, withOpacity(paintColor, cov))
						}
					}
				}
			}
		case op.TextOp:
			pos := transformPoint(currentMatrix, o.Pos)
			// Font scale multiplies by the matrix's uniform scale so text
			// zooms with the board (scale 1 everywhere else → no change).
			// Sub-1 scale is legal — the rasterizer box-filters the bitmap
			// glyphs down instead of clamping (which overflowed a zoomed-out
			// board's cards with full-size text).
			scale := o.Scale * matrixScale(currentMatrix)
			DrawTextTracking(img, o.Text, pos, withOpacity(currentColor, currentOpacity), scale, o.Weight, o.LetterSpacing, o.Italic, clips)
		case op.ImageOp:
			if o.Src == nil {
				break
			}
			dest := transformRect(currentMatrix, o.Dest)
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
			// Keep exact nearest-neighbour sampling for integral scale factors
			// (it is both faster and preserves crisp pixel art), and use
			// premultiplied bilinear sampling for fractional resize ratios. The
			// latter removes the stair-step edges that were most noticeable on
			// thumbnails and object-fit images.
			bilinear := sw != dw || sh != dh
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					cov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
					if cov <= 0 {
						continue
					}
					var c color.RGBA
					if bilinear {
						c = sampleBilinear(o.Src, float64(sb.Min.X)+(float64(x-dest.Min.X)+0.5)*float64(sw)/float64(dw)-0.5,
							float64(sb.Min.Y)+(float64(y-dest.Min.Y)+0.5)*float64(sh)/float64(dh)-0.5)
					} else {
						sy := sb.Min.Y + (y-dest.Min.Y)*sh/dh
						sx := sb.Min.X + (x-dest.Min.X)*sw/dw
						c = o.Src.RGBAAt(sx, sy)
					}
					if currentOpacity < 1.0 {
						c = withOpacity(c, currentOpacity)
					}
					if cov < 1 {
						c = withOpacity(c, cov)
					}
					blendOver(img, x, y, c)
				}
			}
		case op.RRectOp:
			rect := transformRect(currentMatrix, o.Rect)
			hw, hh := float64(rect.Dx())/2, float64(rect.Dy())/2
			if hw <= 0 || hh <= 0 {
				break
			}
			// Radius, stroke and shadow are authored in local units: scale them
			// with the matrix (a zoomed board) so proportions survive, the way
			// a CSS border/radius does under transform.
			s := matrixScale(currentMatrix)
			radius := clamp01(o.Radius*s/math.Min(hw, hh)) * math.Min(hw, hh)
			shadowBlur := o.ShadowBlur * s
			shadowY := o.ShadowY * s
			strokeWidth := o.StrokeWidth * s
			cx := float64(rect.Min.X) + hw
			cy := float64(rect.Min.Y) + hh

			// Iteration bounds: the fill's ~1px AA band lives just outside
			// Rect; the shadow extends ShadowBlur px beyond its ShadowY offset.
			minX, minY := float64(rect.Min.X)-1, float64(rect.Min.Y)-1
			maxX, maxY := float64(rect.Max.X)+1, float64(rect.Max.Y)+1
			if o.Shadow.A > 0 {
				minX = math.Min(minX, float64(rect.Min.X)-shadowBlur-1)
				minY = math.Min(minY, float64(rect.Min.Y)+shadowY-shadowBlur-1)
				maxX = math.Max(maxX, float64(rect.Max.X)+shadowBlur+1)
				maxY = math.Max(maxY, float64(rect.Max.Y)+shadowY+shadowBlur+1)
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
					clipCov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
					if clipCov <= 0 {
						continue
					}
					px, py := float64(x)+0.5, float64(y)+0.5
					if o.Shadow.A > 0 {
						d := sdRoundBox(px, py, cx, cy+shadowY, hw, hh, radius)
						var cov float64
						if shadowBlur > 0 {
							// Smoothstep falloff over the blur width: zero slope
							// at both ends, so the shadow melts out instead of
							// stepping like the old concentric-rect fake.
							t := clamp01(1 - d/shadowBlur)
							cov = t * t * (3 - 2*t)
						} else {
							cov = clamp01(0.5 - d)
						}
						if cov > 0 {
							blendOver(img, x, y, withOpacity(o.Shadow, cov*clipCov*currentOpacity))
						}
					}
					d := sdRoundBox(px, py, cx, cy, hw, hh, radius)
					if o.Fill.A > 0 {
						// ~1px coverage band at the boundary = antialiased edge.
						if cov := clamp01(0.5 - d); cov > 0 {
							blendOver(img, x, y, withOpacity(o.Fill, cov*clipCov*currentOpacity))
						}
					}
					if o.Stroke.A > 0 && strokeWidth > 0 {
						// Stroke sits INSIDE the boundary (CSS border-box):
						// outer AA at d≈0, inner AA at d≈-width.
						if cov := clamp01(0.5-d) * clamp01(0.5+d+strokeWidth); cov > 0 {
							blendOver(img, x, y, withOpacity(o.Stroke, cov*clipCov*currentOpacity))
						}
					}
				}
			}
		}
	}
}

// transformPoint maps an integer op coordinate through the current matrix,
// rounding to the nearest pixel (the graph layer already truncated to ints, so
// this is not a regression — fractional pan/zoom positions land within 1px).
func transformPoint(m geom.Matrix, p image.Point) image.Point {
	q := m.TransformPoint(geom.Point{X: float64(p.X), Y: float64(p.Y)})
	return image.Pt(int(math.Round(q.X)), int(math.Round(q.Y)))
}

// transformRect maps an integer rect through the current matrix. translate×
// uniform-scale (all the graph layer emits) keeps it axis-aligned; TransformBBox
// covers the general case too.
func transformRect(m geom.Matrix, r image.Rectangle) image.Rectangle {
	b := m.TransformBBox(geom.NewBBox(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())))
	return image.Rect(int(math.Round(b.MinX)), int(math.Round(b.MinY)), int(math.Round(b.MaxX)), int(math.Round(b.MaxY)))
}

// matrixScale returns the matrix's uniform scale — the length of its image of
// the unit x-axis. Identity and pure translations yield 1; a board zoom of z
// yields z (rotation is never emitted, so A is the scale itself).
func matrixScale(m geom.Matrix) float64 {
	return math.Hypot(m.A, m.B)
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
// by inAllClips). Clip rects are already in screen space — the ClipOp handler
// transforms them by the current matrix at emit time so all downstream
// consumers (clipBounds, clipCoverage, draw.Draw targets) share one frame.
func clipBounds(clips []op.ClipOp, img *image.RGBA) image.Rectangle {
	if len(clips) == 0 {
		return img.Bounds()
	}
	r := clips[0].Rect
	for _, c := range clips[1:] {
		r = r.Intersect(c.Rect)
	}
	return r.Intersect(img.Bounds())
}

// clipCoverage returns the combined coverage of the active clips at a pixel
// centre. Clip rects are already in screen space (ClipOp transforms them
// at emit time). Rectangular clips remain exact; rounded clips use the same
// signed distance field as RRectOp, giving image/text content a soft one-pixel
// edge instead of the visibly jagged binary corner produced by inAllClips.
func clipCoverage(px, py float64, clips []op.ClipOp) float64 {
	coverage := 1.0
	for _, c := range clips {
		if px < float64(c.Rect.Min.X) || px >= float64(c.Rect.Max.X) ||
			py < float64(c.Rect.Min.Y) || py >= float64(c.Rect.Max.Y) {
			return 0
		}
		if c.Radius <= 0 {
			continue
		}
		hw := float64(c.Rect.Dx()) / 2
		hh := float64(c.Rect.Dy()) / 2
		if hw <= 0 || hh <= 0 {
			return 0
		}
		radius := math.Min(c.Radius, math.Min(hw, hh))
		d := sdRoundBox(px, py, float64(c.Rect.Min.X)+hw, float64(c.Rect.Min.Y)+hh, hw, hh, radius)
		coverage *= clamp01(0.5 - d)
		if coverage <= 0 {
			return 0
		}
	}
	return coverage
}

// sampleBilinear samples straight-alpha RGBA in premultiplied space. Doing
// the interpolation this way avoids dark fringes when a transparent image is
// resized and then composited over a non-white component background.
func sampleBilinear(src *image.RGBA, fx, fy float64) color.RGBA {
	b := src.Bounds()
	if b.Empty() {
		return color.RGBA{}
	}
	fx = math.Max(float64(b.Min.X), math.Min(float64(b.Max.X-1), fx))
	fy = math.Max(float64(b.Min.Y), math.Min(float64(b.Max.Y-1), fy))
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	x1, y1 := x0+1, y0+1
	if x1 >= b.Max.X {
		x1 = b.Max.X - 1
	}
	if y1 >= b.Max.Y {
		y1 = b.Max.Y - 1
	}
	tx, ty := fx-float64(x0), fy-float64(y0)
	var pr, pg, pb, pa float64
	for _, s := range []struct {
		x, y int
		w    float64
	}{{x0, y0, (1 - tx) * (1 - ty)}, {x1, y0, tx * (1 - ty)}, {x0, y1, (1 - tx) * ty}, {x1, y1, tx * ty}} {
		c := src.RGBAAt(s.x, s.y)
		a := float64(c.A) / 255
		pr += float64(c.R) * a * s.w
		pg += float64(c.G) * a * s.w
		pb += float64(c.B) * a * s.w
		pa += a * s.w
	}
	if pa <= 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8(math.Round(pr / pa)),
		G: uint8(math.Round(pg / pa)),
		B: uint8(math.Round(pb / pa)),
		A: uint8(math.Round(pa * 255)),
	}
}

// inAllClips and cornerCenter were removed with the binary stroke path:
// StrokePaintOp's slow branch now uses the same SDF coverage as RRectOp.

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
