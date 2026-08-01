package widgets

import (
	"fmt"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("progress", Progress{})
}

// Progress is the determinate linear progress bar (HTML render_feedback.go
// progress()): a fully rounded track (background var(--fill), border-radius
// 999px, min-height 8px, width 100%) with an accent fill whose width is the
// clamped percentage.
type Progress struct{}

// progressBarHeight is the track's content height in logical px (the HTML
// track's min-height:8px). The content width is a nominal default — real apps
// size the bar with style width / width:fill, which the generic sizing pass
// applies on top.
const (
	progressBarHeight = 8
	progressBarWidth  = 100
)

// Measure reports the nominal track size (physical px at scale).
func (Progress) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return progressBarWidth * scale, progressBarHeight * scale
}

// Record builds the track pill plus the fill pill clipped to the current
// percentage of the laid-out width.
func (Progress) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	radius := float64(ln.Height) / 2 // a full pill, like CSS 999px

	track := draw.NewRect()
	track.Width = float64(ln.Width)
	track.Height = float64(ln.Height)
	track.BorderRadius = radius
	// HTML paints the track var(--fill); the canvas palette's fill-grade token
	// is inputBg (themes/*.json carry no "fill" key).
	track.Fill = themeColor(rt, "inputBg", color.RGBA{229, 229, 234, 255})

	g := draw.NewGroup()
	g.AddChild(track)

	if pct := progressPct(ln.Node, rt); pct > 0 {
		fill := draw.NewRect()
		fill.Width = float64(ln.Width) * pct / 100
		fill.Height = float64(ln.Height)
		fill.BorderRadius = radius
		fill.Fill = progressFillColor(ln.Node, rt)
		g.AddChild(fill)
	}
	return g
}

// progressPct resolves the fill percentage from the node's Value (bindable,
// exactly like the HTML path's EvalBinding(n.Value)). HTML accepts a 0..1
// fraction as well as a 0..100 percentage; a declared `max` prop instead
// reads value in max's units (the slider convention, render_input.go:365 —
// the HTML progress itself declares no max). The result clamps to [0,100]
// (HTML clampPct).
func progressPct(n *model.Node, rt *runtime.Runtime) float64 {
	v := asFloat64(runtime.EvalBinding(n.Value, map[string]any{"state": rt.State}))
	if raw, ok := n.Prop("max"); ok {
		if max := asFloat64(raw); max > 0 {
			return clampPct(v / max * 100)
		}
	}
	if v > 0 && v <= 1 { // a 0..1 fraction reads as a percentage
		v *= 100
	}
	return clampPct(v)
}

// progressFillColor resolves the fill colour: the author `color` prop
// (bindable; a palette token or hex — theme.GetColor parses both) wins over
// the accent default (HTML: cssValueOr(prop, "var(--accent)")); the canvas
// palette names the accent "primary".
func progressFillColor(n *model.Node, rt *runtime.Runtime) color.RGBA {
	if raw, ok := n.Prop("color"); ok {
		s := fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), map[string]any{"state": rt.State}))
		if rt != nil && rt.Theme != nil {
			if c, ok := rt.Theme.GetColor(s); ok {
				return c
			}
		}
	}
	return themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
}

// asFloat64 mirrors the HTML renderer's asFloat (render_style.go:1024):
// float64/int pass through, true is 1, strings scanf a number.
func asFloat64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case bool:
		if t {
			return 1
		}
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%g", &f)
		return f
	}
	return 0
}

// clampPct mirrors the HTML clampPct (render_style.go:739).
func clampPct(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 100 {
		return 100
	}
	return f
}
