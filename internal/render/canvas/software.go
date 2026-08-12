package canvas

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

// SoftwareRenderer is the CPU rasterizer: a flat interpreter over the display
// list drawing into an image.RGBA. Pipeline state is scoped to a single Render
// call. Dirty, when non-empty, limits the clear + paint to that rect (engine
// partial redraw); empty Dirty means full-frame clear and raster.
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
type SoftwareRenderer struct {
	// Dirty limits rasterization to a sub-rect. Empty = full frame.
	Dirty image.Rectangle
}

// Render implements Renderer.
func (r SoftwareRenderer) Render(ops *op.Ops, target *image.RGBA) {
	bounds := target.Bounds()
	dirty := r.Dirty.Intersect(bounds)
	full := dirty.Empty()
	if full {
		dirty = bounds
		// Every full frame starts from a clean white background.
		draw.Draw(target, bounds, &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	} else {
		// Partial redraw: only the dirty rect is reset to white; the rest of
		// the buffer keeps the previous frame's pixels.
		draw.Draw(target, dirty, &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	}

	// Pipeline state. Clips form a stack so nested containers intersect
	// (Save/Restore snapshot the stack; a ClipOp pushes on top). The transform
	// is a full affine matrix: geometry ops post-multiply it and transform
	// their integer coordinates into screen space (matrixScale reads the
	// uniform scale so stroke widths, text font scale and corner radii follow
	// a zoomed board).
	// img is the active draw target — LayerOp redirects to a transparent
	// offscreen buffer until EndLayerOp filters and composites it back.
	img := target
	type layerFrame struct {
		parent *image.RGBA
		params layerParams
	}
	var layers []layerFrame

	currentColor := color.RGBA{0, 0, 0, 255}
	currentMatrix := geom.Identity()
	var clips []op.ClipOp
	// Partial redraw: seed the clip stack with the dirty rect in screen space
	// so every paint path (text/RRect/image) naturally skips outside pixels.
	if !full {
		clips = []op.ClipOp{{Rect: dirty}}
	}
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
		case op.LayerOp:
			// Offscreen layer for CSS filter + blend.
			ms := matrixScale(currentMatrix)
			if ms < 1e-9 {
				ms = 1
			}
			p := layerParamsFromOp(o, ms)
			layers = append(layers, layerFrame{parent: img, params: p})
			img = image.NewRGBA(target.Bounds()) // transparent
		case op.EndLayerOp:
			if len(layers) == 0 {
				break
			}
			frame := layers[len(layers)-1]
			layers = layers[:len(layers)-1]
			endLayerComposite(frame.parent, img, frame.params)
			img = frame.parent
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
			ms := matrixScale(currentMatrix)
			scale := o.Scale * ms
			fill := withOpacity(currentColor, currentOpacity)
			// Decorations under the fill (CSS paint order: shadow → stroke → fill).
			if o.ShadowColor.A > 0 {
				sc := withOpacity(o.ShadowColor, currentOpacity)
				sx := int(math.Round(o.ShadowX * ms))
				sy := int(math.Round(o.ShadowY * ms))
				blur := o.ShadowBlur * ms
				if blur > 0.5 {
					// Soft shadow: a small grid of offset passes with falloff.
					// Cap samples so large blur stays cheap at 60fps.
					r := int(math.Ceil(blur))
					if r > 4 {
						r = 4
					}
					for dy := -r; dy <= r; dy++ {
						for dx := -r; dx <= r; dx++ {
							dist := math.Hypot(float64(dx), float64(dy))
							if dist > blur+0.5 {
								continue
							}
							fall := 1 - dist/(blur+0.5)
							fall = fall * fall
							if fall <= 0 {
								continue
							}
							DrawTextTracking(img, o.Text,
								image.Pt(pos.X+sx+dx, pos.Y+sy+dy),
								withOpacity(sc, fall*0.55), scale, o.Weight, o.LetterSpacing, o.Italic, clips)
						}
					}
				} else {
					DrawTextTracking(img, o.Text, image.Pt(pos.X+sx, pos.Y+sy),
						sc, scale, o.Weight, o.LetterSpacing, o.Italic, clips)
				}
			}
			if o.StrokeColor.A > 0 && o.StrokeWidth > 0 {
				sc := withOpacity(o.StrokeColor, currentOpacity)
				w := o.StrokeWidth * ms
				r := int(math.Ceil(w))
				if r < 1 {
					r = 1
				}
				if r > 4 {
					r = 4
				}
				ww := w * w
				for dy := -r; dy <= r; dy++ {
					for dx := -r; dx <= r; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						if float64(dx*dx+dy*dy) > ww+0.75 {
							continue
						}
						DrawTextTracking(img, o.Text, image.Pt(pos.X+dx, pos.Y+dy),
							sc, scale, o.Weight, o.LetterSpacing, o.Italic, clips)
					}
				}
			}
			DrawTextTracking(img, o.Text, pos, fill, scale, o.Weight, o.LetterSpacing, o.Italic, clips)
			// CSS text-decoration lines after fill (underline / line-through / overline).
			if o.Underline || o.LineThrough || o.Overline {
				fontSize := clampFontSize(scale * 10)
				tw := MeasureTextTracking(o.Text, fontSize, o.LetterSpacing*ms)
				th := int(math.Round(fontSize))
				if th < 1 {
					th = 1
				}
				lineH := int(math.Max(1, math.Round(fontSize/12)))
				drawTextLine := func(y int) {
					for dy := 0; dy < lineH; dy++ {
						for x := 0; x < int(math.Ceil(tw)); x++ {
							px, py := pos.X+x, y+dy
							if clipCoverage(float64(px)+0.5, float64(py)+0.5, clips) > 0 {
								blendOver(img, px, py, fill)
							}
						}
					}
				}
				if o.Underline {
					drawTextLine(pos.Y + th - lineH)
				}
				if o.LineThrough {
					drawTextLine(pos.Y + th/2)
				}
				if o.Overline {
					drawTextLine(pos.Y)
				}
			}
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
			// Local geometry (author space). Sampling in local space after
			// inverse-transform keeps rounded corners correct under rotation
			// and skew — transforming the AABB then using an axis-aligned SDF
			// would draw a fat axis-aligned capsule instead of a rotated rrect.
			lw := float64(o.Rect.Dx())
			lh := float64(o.Rect.Dy())
			if lw <= 0 || lh <= 0 {
				break
			}
			hw, hh := lw/2, lh/2
			s := matrixScale(currentMatrix)
			if s < 1e-9 {
				s = 1
			}
			// Local radius/stroke/shadow (author units); coverage AA uses screen
			// distance ≈ local_d * s.
			radius := o.Radius
			if radius > math.Min(hw, hh) {
				radius = math.Min(hw, hh)
			}
			shadowBlurL := o.ShadowBlur
			shadowXL, shadowYL := o.ShadowX, o.ShadowY
			strokeWidthL := o.StrokeWidth
			cxL := float64(o.Rect.Min.X) + hw
			cyL := float64(o.Rect.Min.Y) + hh

			// Expand local bounds for AA band + shadow + outline, then map to screen AABB.
			padL := 1.0/s + 1.0
			if o.Shadow.A > 0 {
				padL = math.Max(padL, math.Abs(shadowXL)+shadowBlurL+1.0/s+1)
				padL = math.Max(padL, math.Abs(shadowYL)+shadowBlurL+1.0/s+1)
			}
			if strokeWidthL > 0 {
				padL = math.Max(padL, strokeWidthL+1.0/s)
			}
			if o.Outline.A > 0 && o.OutlineWidth > 0 {
				padL = math.Max(padL, o.OutlineOffset+o.OutlineWidth+1.0/s+1)
			}
			localBox := geom.NewBBox(float64(o.Rect.Min.X)-padL, float64(o.Rect.Min.Y)-padL, lw+2*padL, lh+2*padL)
			sb := currentMatrix.TransformBBox(localBox)
			b := image.Rect(int(math.Floor(sb.MinX)), int(math.Floor(sb.MinY)), int(math.Ceil(sb.MaxX)), int(math.Ceil(sb.MaxY)))
			b = b.Intersect(img.Bounds())
			if len(clips) > 0 {
				b = b.Intersect(clipBounds(clips, img))
			}
			if b.Empty() {
				break
			}

			inv, invOK := currentMatrix.Invert()
			// Precompute frosted backdrop once for the screen bounds.
			var frostBuf *image.RGBA
			if o.BackdropBlur > 0 {
				rBlur := int(math.Ceil(o.BackdropBlur * s))
				if rBlur < 1 {
					rBlur = 1
				}
				frostBuf = sampleBoxBlurRegion(img, b, rBlur)
			}

			// Screen-space shadow blur for smoothstep (local blur × scale).
			shadowBlurS := shadowBlurL * s
			strokeWidthS := strokeWidthL * s

			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					clipCov := clipCoverage(float64(x)+0.5, float64(y)+0.5, clips)
					if clipCov <= 0 {
						continue
					}
					px, py := float64(x)+0.5, float64(y)+0.5
					// Map screen pixel → local author space.
					lx, ly := px, py
					if invOK {
						lp := inv.TransformPoint(geom.Point{X: px, Y: py})
						lx, ly = lp.X, lp.Y
					} else {
						// Non-invertible: fall back to axis-aligned AABB path.
						rect := transformRect(currentMatrix, o.Rect)
						lx = (px - float64(rect.Min.X)) * lw / math.Max(1, float64(rect.Dx()))
						ly = (py - float64(rect.Min.Y)) * lh / math.Max(1, float64(rect.Dy()))
						lx += float64(o.Rect.Min.X)
						ly += float64(o.Rect.Min.Y)
					}
					// Outer drop shadow (under fill).
					if o.Shadow.A > 0 && !o.ShadowInset {
						dL := sdRoundBox(lx, ly, cxL+shadowXL, cyL+shadowYL, hw, hh, radius)
						dS := dL * s
						var cov float64
						if shadowBlurS > 0 {
							t := clamp01(1 - dS/shadowBlurS)
							cov = t * t * (3 - 2*t)
						} else {
							cov = clamp01(0.5 - dS)
						}
						if cov > 0 {
							blendOver(img, x, y, withOpacity(o.Shadow, cov*clipCov*currentOpacity))
						}
					}
					dL := sdRoundBox(lx, ly, cxL, cyL, hw, hh, radius)
					dS := dL * s
					// ~1px coverage band at the boundary = antialiased edge.
					if cov := clamp01(0.5 - dS); cov > 0 {
						if frostBuf != nil {
							frost := frostBuf.RGBAAt(x-b.Min.X, y-b.Min.Y)
							if o.BackdropTint.A > 0 {
								frost = blendOverColor(frost, o.BackdropTint)
							}
							blendOver(img, x, y, withOpacity(frost, cov*clipCov*currentOpacity))
						}
						fill := o.Fill
						if len(o.GradientStops) >= 2 {
							// Gradients authored in local box space.
							lox, loy := float64(o.Rect.Min.X), float64(o.Rect.Min.Y)
							if o.GradientConic {
								fill = sampleConicGradient(o.GradientStops, o.GradientStopPos, o.GradientAngle, lx, ly, lox, loy, lw, lh)
							} else if o.GradientRadial {
								fill = sampleRadialGradient(o.GradientStops, o.GradientStopPos, lx, ly, lox, loy, lw, lh)
							} else {
								fill = sampleLinearGradient(o.GradientStops, o.GradientStopPos, o.GradientAngle, lx, ly, lox, loy, lw, lh)
							}
						}
						if fill.A > 0 {
							blendOver(img, x, y, withOpacity(fill, cov*clipCov*currentOpacity))
						}
					}
					// Inset shadow sits on top of the fill (CSS paint order).
					// Soft exterior of the offset shape × original interior
					// (CSS box-shadow: inset; zero offset yields an inner rim).
					if o.Shadow.A > 0 && o.ShadowInset {
						inside := clamp01(0.5 - dS)
						if inside > 0 {
							dOff := sdRoundBox(lx-shadowXL, ly-shadowYL, cxL, cyL, hw, hh, radius) * s
							var rim float64
							if shadowBlurS > 0 {
								// 0 deep inside offset shape (−blur), 1 at/outside edge.
								t := clamp01((dOff + shadowBlurS) / shadowBlurS)
								rim = t * t * (3 - 2*t)
							} else if dOff > 0 {
								rim = 1
							}
							if cov := rim * inside; cov > 0.02 {
								blendOver(img, x, y, withOpacity(o.Shadow, cov*clipCov*currentOpacity))
							}
						}
					}
					if o.Stroke.A > 0 && strokeWidthS > 0 {
						// Stroke sits INSIDE the boundary (CSS border-box).
						if cov := clamp01(0.5-dS) * clamp01(0.5+dS+strokeWidthS); cov > 0 {
							blendOver(img, x, y, withOpacity(o.Stroke, cov*clipCov*currentOpacity))
						}
					}
					// CSS outline: ring OUTSIDE the border box (offset + width).
					if o.Outline.A > 0 && o.OutlineWidth > 0 {
						offS := o.OutlineOffset * s
						owS := o.OutlineWidth * s
						// Outer edge at d = offS+owS, inner at d = offS (in screen SDF).
						// Coverage in the band outside the shape.
						if dS >= offS-0.5 {
							cov := clamp01(0.5+(dS-offS)) * clamp01(0.5-(dS-offS-owS))
							if cov > 0 {
								blendOver(img, x, y, withOpacity(o.Outline, cov*clipCov*currentOpacity))
							}
						}
					}
				}
			}
		}
	}
}

// layerParams is the resolved CSS filter + blend state for one offscreen layer.
type layerParams struct {
	blur, brightness, contrast, saturate float64
	grayscale, hueRotate, opacity        float64
	dropX, dropY, dropBlur               float64
	dropColor                            color.RGBA
	blend                                string
}

func layerParamsFromOp(o op.LayerOp, matrixScale float64) layerParams {
	b, c, s := o.Brightness, o.Contrast, o.Saturate
	if b == 0 && c == 0 && s == 0 {
		b, c, s = 1, 1, 1
	}
	opac := o.Opacity
	if opac <= 0 {
		opac = 1
	}
	return layerParams{
		blur: o.Blur * matrixScale, brightness: b, contrast: c, saturate: s,
		grayscale: o.Grayscale, hueRotate: o.HueRotate, opacity: opac,
		dropX: o.DropShadowX * matrixScale, dropY: o.DropShadowY * matrixScale,
		dropBlur: o.DropShadowBlur * matrixScale, dropColor: o.DropShadowColor,
		blend: strings.ToLower(strings.TrimSpace(o.BlendMode)),
	}
}

// endLayerComposite applies CSS filter (drop-shadow, blur, color matrix,
// opacity) then blends onto dst with mix-blend-mode.
func endLayerComposite(dst, src *image.RGBA, p layerParams) {
	if dst == nil || src == nil {
		return
	}
	region := alphaBounds(src)
	if region.Empty() {
		return
	}
	// Drop-shadow first (under content): blur source alpha, tint, offset.
	if p.dropColor.A > 0 && (p.dropBlur > 0 || p.dropX != 0 || p.dropY != 0) {
		compositeDropShadow(dst, src, region, p)
	}

	needColor := p.brightness != 1 || p.contrast != 1 || p.saturate != 1 ||
		p.grayscale > 0 || p.hueRotate != 0 || p.opacity != 1
	r := int(math.Ceil(p.blur))
	if r > 24 {
		r = 24
	}

	paint := func(c color.RGBA, x, y int) {
		if needColor {
			c = applyFilterColorEx(c, p)
		}
		if p.blend != "" && p.blend != "normal" {
			blendModeOver(dst, x, y, c, p.blend)
		} else {
			blendOver(dst, x, y, c)
		}
	}

	if r < 1 {
		region = region.Intersect(src.Bounds()).Intersect(dst.Bounds())
		if !needColor && (p.blend == "" || p.blend == "normal") {
			compositeLayer(dst, src, region)
			return
		}
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				c := src.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				paint(c, x, y)
			}
		}
		return
	}
	pad := r * 3
	if pad > 48 {
		pad = 48
	}
	region = image.Rect(region.Min.X-pad, region.Min.Y-pad, region.Max.X+pad, region.Max.Y+pad).Intersect(src.Bounds())
	blurred := gaussianBlurApprox(src, region, r)
	bw, bh := blurred.Bounds().Dx(), blurred.Bounds().Dy()
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			c := blurred.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			paint(c, region.Min.X+x, region.Min.Y+y)
		}
	}
}

// compositeDropShadow paints a soft alpha silhouette of src under the content.
func compositeDropShadow(dst, src *image.RGBA, content image.Rectangle, p layerParams) {
	r := int(math.Ceil(p.dropBlur))
	if r > 20 {
		r = 20
	}
	pad := r + int(math.Ceil(math.Abs(p.dropX))) + int(math.Ceil(math.Abs(p.dropY))) + 1
	region := image.Rect(content.Min.X-pad, content.Min.Y-pad, content.Max.X+pad, content.Max.Y+pad).Intersect(src.Bounds())
	if region.Empty() {
		return
	}
	// Build alpha-only silhouette in drop color.
	sil := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			a := src.RGBAAt(x, y).A
			if a == 0 {
				continue
			}
			sc := p.dropColor
			sc.A = uint8(uint32(sc.A) * uint32(a) / 255)
			sil.SetRGBA(x-region.Min.X, y-region.Min.Y, sc)
		}
	}
	local := image.Rect(0, 0, region.Dx(), region.Dy())
	var shadow *image.RGBA
	if r >= 1 {
		shadow = gaussianBlurApprox(sil, local, r)
	} else {
		shadow = sil
	}
	ox := int(math.Round(p.dropX))
	oy := int(math.Round(p.dropY))
	bw, bh := shadow.Bounds().Dx(), shadow.Bounds().Dy()
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			c := shadow.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			blendOver(dst, region.Min.X+x+ox, region.Min.Y+y+oy, c)
		}
	}
}

// gaussianBlurApprox approximates a Gaussian by stacking three separable box
// blurs (W3C / CSS filter:blur quality without a true Gaussian kernel).
func gaussianBlurApprox(img *image.RGBA, region image.Rectangle, radius int) *image.RGBA {
	if radius < 1 {
		return boxBlurRegionZeroPad(img, region, 0)
	}
	// Each pass uses ~radius/√3 so the total variance tracks the CSS radius.
	pass := int(math.Round(float64(radius) / math.Sqrt(3)))
	if pass < 1 {
		pass = 1
	}
	// First pass reads from img[region]; subsequent passes work in local space.
	cur := boxBlurRegionZeroPad(img, region, pass)
	// Re-blur in-place coordinates: cur is origin (0,0) size = region.
	local := image.Rect(0, 0, region.Dx(), region.Dy())
	// boxBlurRegionZeroPad expects absolute coords when img is full-frame;
	// for the local buffer, pass local and treat cur as the image.
	cur = boxBlurRegionLocal(cur, local, pass)
	cur = boxBlurRegionLocal(cur, local, pass)
	return cur
}

// boxBlurRegionLocal blurs an image whose content already lives at (0,0).
func boxBlurRegionLocal(img *image.RGBA, region image.Rectangle, radius int) *image.RGBA {
	// Reuse zero-pad path: region is in img space.
	return boxBlurRegionZeroPad(img, region, radius)
}

// applyFilterColor is the classic 3-param path (tests / callers).
func applyFilterColor(c color.RGBA, brightness, contrast, saturate float64) color.RGBA {
	return applyFilterColorEx(c, layerParams{
		brightness: brightness, contrast: contrast, saturate: saturate, opacity: 1,
	})
}

// applyFilterColorEx implements CSS filter color ops + opacity on a
// straight-alpha pixel.
func applyFilterColorEx(c color.RGBA, p layerParams) color.RGBA {
	if c.A == 0 {
		return c
	}
	rf, gf, bf := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	if p.brightness != 1 {
		rf *= p.brightness
		gf *= p.brightness
		bf *= p.brightness
	}
	if p.contrast != 1 {
		rf = (rf-0.5)*p.contrast + 0.5
		gf = (gf-0.5)*p.contrast + 0.5
		bf = (bf-0.5)*p.contrast + 0.5
	}
	if p.saturate != 1 {
		y := 0.2126*rf + 0.7152*gf + 0.0722*bf
		rf = y + (rf-y)*p.saturate
		gf = y + (gf-y)*p.saturate
		bf = y + (bf-y)*p.saturate
	}
	if p.grayscale > 0 {
		y := 0.2126*rf + 0.7152*gf + 0.0722*bf
		g := clamp01(p.grayscale)
		rf = rf*(1-g) + y*g
		gf = gf*(1-g) + y*g
		bf = bf*(1-g) + y*g
	}
	if p.hueRotate != 0 {
		rf, gf, bf = hueRotateRGB(rf, gf, bf, p.hueRotate)
	}
	a := c.A
	if p.opacity != 1 {
		a = uint8(clamp01(float64(a)/255*p.opacity) * 255)
	}
	return color.RGBA{
		R: uint8(clamp01(rf) * 255),
		G: uint8(clamp01(gf) * 255),
		B: uint8(clamp01(bf) * 255),
		A: a,
	}
}

// hueRotateRGB rotates hue by deg degrees (CSS hue-rotate / SVG matrix).
func hueRotateRGB(r, g, b, deg float64) (float64, float64, float64) {
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	rr := r*(0.213+0.787*cos-0.213*sin) + g*(0.715-0.715*cos-0.715*sin) + b*(0.072-0.072*cos+0.928*sin)
	gg := r*(0.213-0.213*cos+0.143*sin) + g*(0.715+0.285*cos+0.140*sin) + b*(0.072-0.072*cos-0.283*sin)
	bb := r*(0.213-0.213*cos-0.787*sin) + g*(0.715-0.715*cos+0.715*sin) + b*(0.072+0.928*cos+0.072*sin)
	return rr, gg, bb
}

// blendModeOver composites src over dst with a CSS mix-blend-mode.
func blendModeOver(dst *image.RGBA, x, y int, src color.RGBA, mode string) {
	if src.A == 0 {
		return
	}
	if x < dst.Bounds().Min.X || y < dst.Bounds().Min.Y || x >= dst.Bounds().Max.X || y >= dst.Bounds().Max.Y {
		return
	}
	db := dst.RGBAAt(x, y)
	// Blend RGB in straight space, then alpha-composite result over backdrop.
	sr, sg, sb := float64(src.R)/255, float64(src.G)/255, float64(src.B)/255
	dr, dg, dbv := float64(db.R)/255, float64(db.G)/255, float64(db.B)/255
	var br, bg, bb float64
	switch mode {
	case "multiply":
		br, bg, bb = sr*dr, sg*dg, sb*dbv
	case "screen":
		br, bg, bb = 1-(1-sr)*(1-dr), 1-(1-sg)*(1-dg), 1-(1-sb)*(1-dbv)
	case "darken":
		br, bg, bb = math.Min(sr, dr), math.Min(sg, dg), math.Min(sb, dbv)
	case "lighten":
		br, bg, bb = math.Max(sr, dr), math.Max(sg, dg), math.Max(sb, dbv)
	case "overlay":
		br, bg, bb = overlayChan(dr, sr), overlayChan(dg, sg), overlayChan(dbv, sb)
	default:
		blendOver(dst, x, y, src)
		return
	}
	// Mix blended RGB with source alpha, then over-composite onto backdrop.
	blended := color.RGBA{
		R: uint8(clamp01(br) * 255),
		G: uint8(clamp01(bg) * 255),
		B: uint8(clamp01(bb) * 255),
		A: src.A,
	}
	blendOver(dst, x, y, blended)
}

func overlayChan(b, s float64) float64 {
	if b < 0.5 {
		return 2 * b * s
	}
	return 1 - 2*(1-b)*(1-s)
}

// alphaBounds returns the tight AABB of non-zero-alpha pixels in img.
func alphaBounds(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func compositeLayer(dst, src *image.RGBA, region image.Rectangle) {
	region = region.Intersect(src.Bounds()).Intersect(dst.Bounds())
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			c := src.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			blendOver(dst, x, y, c)
		}
	}
}

// boxBlurRegionZeroPad is a separable box blur treating out-of-bounds as
// transparent (correct for filter:blur of isolated content).
func boxBlurRegionZeroPad(img *image.RGBA, region image.Rectangle, radius int) *image.RGBA {
	region = region.Intersect(img.Bounds())
	w, h := region.Dx(), region.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if region.Empty() || radius < 1 {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out.SetRGBA(x, y, img.RGBAAt(region.Min.X+x, region.Min.Y+y))
			}
		}
		return out
	}
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	src := img.Bounds()
	// Horizontal pass.
	for y := 0; y < h; y++ {
		sy := region.Min.Y + y
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum, n uint32
			for dx := -radius; dx <= radius; dx++ {
				xx := region.Min.X + x + dx
				if xx < src.Min.X || xx >= src.Max.X || sy < src.Min.Y || sy >= src.Max.Y {
					n++ // transparent pad
					continue
				}
				c := img.RGBAAt(xx, sy)
				rSum += uint32(c.R) * uint32(c.A)
				gSum += uint32(c.G) * uint32(c.A)
				bSum += uint32(c.B) * uint32(c.A)
				aSum += uint32(c.A)
				n++
			}
			if n == 0 || aSum == 0 {
				continue
			}
			// Un-premultiply average.
			tmp.SetRGBA(x, y, color.RGBA{
				uint8(rSum / aSum),
				uint8(gSum / aSum),
				uint8(bSum / aSum),
				uint8(aSum / n),
			})
		}
	}
	// Vertical pass.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum, n uint32
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					n++
					continue
				}
				c := tmp.RGBAAt(x, yy)
				rSum += uint32(c.R) * uint32(c.A)
				gSum += uint32(c.G) * uint32(c.A)
				bSum += uint32(c.B) * uint32(c.A)
				aSum += uint32(c.A)
				n++
			}
			if n == 0 || aSum == 0 {
				continue
			}
			out.SetRGBA(x, y, color.RGBA{
				uint8(rSum / aSum),
				uint8(gSum / aSum),
				uint8(bSum / aSum),
				uint8(aSum / n),
			})
		}
	}
	return out
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

// sampleLinearGradient interpolates stops along a CSS-angle axis through the
// rect. CSS: 0deg = to top, 90deg = to right (converted to math radians).
// stopPos is optional 0..1 positions (same length as stops); nil = even.
func sampleLinearGradient(stops []color.RGBA, stopPos []float64, angle, px, py, ox, oy, w, h float64) color.RGBA {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	if len(stops) == 1 {
		return stops[0]
	}
	if w <= 0 || h <= 0 {
		return stops[0]
	}
	// CSS angle → direction vector (0° points up).
	a := math.Mod(angle, 360)
	if a < 0 {
		a += 360
	}
	rad := a * math.Pi / 180
	// CSS: 0deg = north, increases clockwise; math.Sin/Cos use CCW from east.
	// Convert: css 0 → (0,-1), css 90 → (1,0).
	dx := math.Sin(rad)
	dy := -math.Cos(rad)
	// Project pixel relative to rect center onto the gradient axis, normalize
	// by the projected half-extent of the rect corners.
	cx, cy := ox+w/2, oy+h/2
	// Half-diagonal projection length along (dx,dy).
	half := 0.5 * (math.Abs(dx)*w + math.Abs(dy)*h)
	if half < 1e-6 {
		return stops[0]
	}
	t := ((px-cx)*dx + (py-cy)*dy) / half
	// Map [-1,1] → [0,1]
	t = (t + 1) / 2
	return sampleGradientT(stops, stopPos, t)
}

// sampleConicGradient interpolates stops by angle around the box center.
// fromDeg is CSS degrees (0 = from top, increasing clockwise).
func sampleConicGradient(stops []color.RGBA, stopPos []float64, fromDeg, px, py, ox, oy, w, h float64) color.RGBA {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	if len(stops) == 1 {
		return stops[0]
	}
	cx, cy := ox+w/2, oy+h/2
	// Math atan2: 0 = east, CCW. CSS conic: 0 = north, CW.
	ang := math.Atan2(px-cx, cy-py) // from north, CW → range (-π,π]
	if ang < 0 {
		ang += 2 * math.Pi
	}
	// Normalize to [0,1) then rotate by fromDeg.
	t := ang / (2 * math.Pi)
	t -= fromDeg / 360
	t = math.Mod(t, 1)
	if t < 0 {
		t += 1
	}
	return sampleGradientT(stops, stopPos, t)
}

// sampleRadialGradient interpolates stops by distance from the box center
// to the farther corner (CSS circle farthest-corner approximation).
func sampleRadialGradient(stops []color.RGBA, stopPos []float64, px, py, ox, oy, w, h float64) color.RGBA {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	if len(stops) == 1 {
		return stops[0]
	}
	if w <= 0 || h <= 0 {
		return stops[0]
	}
	cx, cy := ox+w/2, oy+h/2
	// Radius to farthest corner.
	r := math.Hypot(w/2, h/2)
	if r < 1e-6 {
		return stops[0]
	}
	t := math.Hypot(px-cx, py-cy) / r
	return sampleGradientT(stops, stopPos, t)
}

// sampleGradientT maps t∈[0,1] onto stops (even spacing or explicit positions).
func sampleGradientT(stops []color.RGBA, stopPos []float64, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	n := len(stops)
	if n < 2 {
		return stops[0]
	}
	// Explicit positions.
	if len(stopPos) == n {
		// Ensure monotonic positions.
		pos := make([]float64, n)
		copy(pos, stopPos)
		for i := 1; i < n; i++ {
			if pos[i] < pos[i-1] {
				pos[i] = pos[i-1]
			}
		}
		if t <= pos[0] {
			return stops[0]
		}
		if t >= pos[n-1] {
			return stops[n-1]
		}
		for i := 0; i < n-1; i++ {
			if t >= pos[i] && t <= pos[i+1] {
				span := pos[i+1] - pos[i]
				f := 0.0
				if span > 1e-9 {
					f = (t - pos[i]) / span
				}
				return lerpRGBA(stops[i], stops[i+1], f)
			}
		}
		return stops[n-1]
	}
	// Even spacing.
	pos := t * float64(n-1)
	i := int(pos)
	if i >= n-1 {
		return stops[n-1]
	}
	f := pos - float64(i)
	return lerpRGBA(stops[i], stops[i+1], f)
}

func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: uint8(float64(a.A)*(1-t) + float64(b.A)*t + 0.5),
	}
}

// sampleBoxBlur averages the neighborhood of (x,y) already painted in img.
func sampleBoxBlur(img *image.RGBA, x, y, radius int) color.RGBA {
	if img == nil || radius < 1 {
		return img.RGBAAt(x, y)
	}
	b := img.Bounds()
	var r, g, bl, a, n uint32
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			xx, yy := x+dx, y+dy
			if xx < b.Min.X || yy < b.Min.Y || xx >= b.Max.X || yy >= b.Max.Y {
				continue
			}
			c := img.RGBAAt(xx, yy)
			r += uint32(c.R)
			g += uint32(c.G)
			bl += uint32(c.B)
			a += uint32(c.A)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	return color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)}
}

// sampleBoxBlurRegion returns a separable (H then V) box-blur of img over
// region, as a new RGBA with origin at (0,0) = region.Min. Much smoother frost
// than a single-pixel sampleBoxBlur, at O(n·radius) instead of O(n·radius²).
func sampleBoxBlurRegion(img *image.RGBA, region image.Rectangle, radius int) *image.RGBA {
	region = region.Intersect(img.Bounds())
	if region.Empty() || radius < 1 {
		out := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				out.SetRGBA(x-region.Min.X, y-region.Min.Y, img.RGBAAt(x, y))
			}
		}
		return out
	}
	w, h := region.Dx(), region.Dy()
	// Expand source sample for blur kernel.
	src := img.Bounds()
	// Horizontal pass into tmp (same size as region).
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := region.Min.Y + y
		for x := 0; x < w; x++ {
			var r, g, bl, a, n uint32
			for dx := -radius; dx <= radius; dx++ {
				xx := region.Min.X + x + dx
				if xx < src.Min.X || xx >= src.Max.X {
					continue
				}
				c := img.RGBAAt(xx, sy)
				r += uint32(c.R)
				g += uint32(c.G)
				bl += uint32(c.B)
				a += uint32(c.A)
				n++
			}
			if n == 0 {
				continue
			}
			tmp.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	// Vertical pass tmp → out.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, a, n uint32
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					// Clamp to edge of tmp (approximation outside region).
					cy := yy
					if cy < 0 {
						cy = 0
					}
					if cy >= h {
						cy = h - 1
					}
					c := tmp.RGBAAt(x, cy)
					r += uint32(c.R)
					g += uint32(c.G)
					bl += uint32(c.B)
					a += uint32(c.A)
					n++
					continue
				}
				c := tmp.RGBAAt(x, yy)
				r += uint32(c.R)
				g += uint32(c.G)
				bl += uint32(c.B)
				a += uint32(c.A)
				n++
			}
			if n == 0 {
				continue
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return out
}

// blendOverColor composites src over dst (both straight alpha).
func blendOverColor(dst, src color.RGBA) color.RGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8((float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA),
		G: uint8((float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA),
		B: uint8((float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA),
		A: uint8(outA * 255),
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
