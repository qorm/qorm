package widgets

import (
	"fmt"
	"image/color"
	"math"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("chart", Chart{})
}

// Chart renders a bar / line / area / sparkline series (HTML render_media.
// go:114 — same geometry, same defaults). The series rasterizes through the
// icon set's 4x-supersampled coverage pipeline, so slopes and rounded bar
// tops get the same antialiasing as icons instead of stair-stepping.
type Chart struct{}

// chartDefaultW/H mirror the HTML natural size (render_media.go:118).
const (
	chartDefaultW = 240
	chartDefaultH = 80
)

// Measure reports the styled size, else the HTML natural size (× scale).
func (Chart) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	if v, ok := n.Style["width"].(float64); ok && v > 0 {
		w = int(v) * scale
	}
	if v, ok := n.Style["height"].(float64); ok && v > 0 {
		h = int(v) * scale
	}
	if w <= 0 {
		w = chartDefaultW * scale
	}
	if h <= 0 {
		h = chartDefaultH * scale
	}
	return w, h
}

// Record rasterizes the series into a bitmap node sized to the resolved box.
func (Chart) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	w, h := ln.Width, ln.Height
	if w <= 0 || h <= 0 {
		return nil
	}
	vals := chartValues(ln.Node, rt)
	if len(vals) == 0 {
		return nil
	}
	kind := "bar"
	if raw, ok := ln.Node.Prop("chartType"); ok {
		kind = fmt.Sprint(raw)
	}
	ink := chartInk(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width = float64(w)
	g.Height = float64(h)
	switch kind {
	case "line", "sparkline", "area":
		pts := chartLinePoints(vals, float64(w), float64(h))
		strokeW := 2.0
		if kind == "sparkline" {
			strokeW = 1.5
		}
		if kind == "area" {
			closed := make([]fpoint, 0, len(pts)+2)
			closed = append(closed, pts...)
			closed = append(closed, fpoint{float64(w), float64(h)}, fpoint{0, float64(h)})
			g.AddChild(chartBitmap(closed, nil, w, h, withAlpha(ink, 0.15)))
		}
		g.AddChild(chartStrokeBitmap(pts, strokeW, float64(w), float64(h), ink))
	default: // bar
		subs := chartBarRects(vals, float64(w), float64(h))
		g.AddChild(chartBitmap(nil, subs, w, h, ink))
	}
	return g
}

// chartBitmap rasterizes one closed fill polygon and/or rounded-rect fill
// subpaths into a w×h bitmap at 4x supersampling (the icon pipeline's AA).
func chartBitmap(fill []fpoint, fills []subpath, w, h int, ink color.RGBA) *draw.Image {
	const ss = 4
	W, H := w*ss, h*ss
	cov := make([]uint8, W*H)
	if len(fill) > 0 {
		scaled := make([]fpoint, len(fill))
		for i, p := range fill {
			scaled[i] = fpoint{p.x * ss, p.y * ss}
		}
		fillSubpaths(cov, W, H, []subpath{{pts: scaled, closed: true}})
	}
	if fills != nil {
		scaled := make([]subpath, len(fills))
		for i, sp := range fills {
			pts := make([]fpoint, len(sp.pts))
			for j, p := range sp.pts {
				pts[j] = fpoint{p.x * ss, p.y * ss}
			}
			scaled[i] = subpath{pts: pts, closed: true}
		}
		fillSubpaths(cov, W, H, scaled)
	}
	img := downsampleIcon(cov, W, H, ss, ink)
	node := draw.NewImage()
	node.NoHit = true
	node.Width = float64(w)
	node.Height = float64(h)
	node.Bitmap = img
	node.Fit = "fill"
	return node
}

// chartStrokeBitmap rasterizes a stroked polyline (line/sparkline/area edge).
func chartStrokeBitmap(pts []fpoint, strokeW, w, h float64, ink color.RGBA) *draw.Image {
	const ss = 4
	W, H := int(w)*ss, int(h)*ss
	cov := make([]uint8, W*H)
	scaled := make([]fpoint, len(pts))
	for i, p := range pts {
		scaled[i] = fpoint{p.x * ss, p.y * ss}
	}
	strokeSubpaths(cov, W, H, []subpath{{pts: scaled}}, strokeW*ss)
	img := downsampleIcon(cov, W, H, ss, ink)
	node := draw.NewImage()
	node.NoHit = true
	node.Width = w
	node.Height = h
	node.Bitmap = img
	node.Fit = "fill"
	return node
}

// chartValues reads the data prop: a literal JSON array, or a bound array
// (HTML chartData, render_media.go:146).
func chartValues(n *model.Node, rt *runtime.Runtime) []float64 {
	raw, ok := n.Prop("data")
	if !ok {
		return nil
	}
	switch d := raw.(type) {
	case string:
		arr, ok := runtime.EvalBinding(d, formCtxLn(rt, nil)).([]any)
		if !ok {
			return nil
		}
		return chartToFloats(arr)
	case []any:
		return chartToFloats(d)
	}
	return nil
}

func chartToFloats(arr []any) []float64 {
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		out = append(out, asFloat64(v))
	}
	return out
}

// chartInk resolves the series colour: the author `color` prop (bindable,
// token or hex) wins over the accent default (HTML cssValueOr(var(--accent));
// the canvas palette names it "primary").
func chartInk(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) color.RGBA {
	if raw, ok := n.Prop("color"); ok {
		s := fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln)))
		if c := canvas.ResolveColor(s, rt); c.A > 0 {
			return c
		}
	}
	return themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
}

// chartBarRects builds the per-bar rounded rects (HTML chartBars: 12% gap,
// 76% width, rx 1.5, heights against the series max with a 2px top reserve).
func chartBarRects(vals []float64, w, h float64) []subpath {
	max := vals[0]
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	bw := w / float64(len(vals))
	subs := make([]subpath, 0, len(vals))
	for i, v := range vals {
		bh := (v / max) * (h - 2)
		if bh <= 0 {
			continue
		}
		subs = append(subs, rectPath(float64(i)*bw+bw*0.12, h-bh, bw*0.76, bh, 1.5))
	}
	return subs
}

// chartLinePoints maps the series to polyline points (HTML chartLine:
// min/max range, 2px margins, last point at the right edge).
func chartLinePoints(vals []float64, w, h float64) []fpoint {
	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1
	}
	pts := make([]fpoint, len(vals))
	for i, v := range vals {
		pts[i] = fpoint{
			float64(i) * (w / math.Max(1, float64(len(vals)-1))),
			h - ((v-min)/rng)*(h-4) - 2,
		}
	}
	return pts
}

// withAlpha scales a colour's alpha by f (area fill = 15% of the series ink).
func withAlpha(c color.RGBA, f float64) color.RGBA {
	c.A = uint8(float64(c.A) * f)
	return c
}
