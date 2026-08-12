package canvas

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// MeasureOpts controls CollectMeasure / MeasureScene output.
// Logical=true reports CSS-px (divide physical by scale) so agent checks match
// design-time coordinates and the HTML getBoundingClientRect path at scale 1.
type MeasureOpts struct {
	Logical bool // report x/y/w/h in logical CSS px (default for MeasureScene)
}

// CollectMeasure walks the last rendered graph and emits HTML-compatible
// measurement rows (same shape as app.js qormMeasure → POST /measure).
// Coordinates default to physical device px; pass Logical via CollectMeasureOpts
// for design-time CSS px (HiDPI-safe agent checks).
func (e *Engine) CollectMeasure() []byte {
	return e.CollectMeasureOpts(MeasureOpts{})
}

// CollectMeasureOpts is CollectMeasure with Logical/physical control.
func (e *Engine) CollectMeasureOpts(opts MeasureOpts) []byte {
	if e == nil || e.graphRoot == nil {
		return []byte("[]")
	}
	scale := e.lastScale
	if scale < 1 {
		scale = 1
	}
	// Prefer LayoutNode Abs* (post-entrance, accurate) with style sidecar.
	snaps := map[string]measureSnap{}
	if e.layoutRoot != nil {
		collectLayoutSnaps(e.layoutRoot, snaps)
	}
	var rows []map[string]any
	seen := map[string]bool{}
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		b := n.Base()
		if m := b.Model; m != nil && m.ID != "" && !b.Overlay && !seen[m.ID] {
			seen[m.ID] = true
			row := measureRowFromGraph(m, n, b, snaps[m.ID], scale, opts.Logical, e.RT)
			rows = append(rows, row)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(e.graphRoot)
	// Layout-only ids (hidden zero-size) still appear if graph missed them.
	for id, sn := range snaps {
		if seen[id] {
			continue
		}
		rows = append(rows, measureRowFromSnap(id, sn, scale, opts.Logical, e.RT))
	}
	// Stage row for audit bounds (like measuring qorm-root).
	if e.lastSize.X > 0 && e.lastSize.Y > 0 {
		sw, sh := float64(e.lastSize.X), float64(e.lastSize.Y)
		if opts.Logical {
			sw /= float64(scale)
			sh /= float64(scale)
		}
		rows = append([]map[string]any{{
			"id":      "__stage",
			"tag":     "canvas",
			"type":    "stage",
			"x":       0.0,
			"y":       0.0,
			"w":       roundPx(sw),
			"h":       roundPx(sh),
			"visible": true,
			"text":    "",
			"display": "block",
			"opacity": "1",
			"scale":   scale,
			"logical": opts.Logical,
		}}, rows...)
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return []byte("[]")
	}
	return b
}

type measureSnap struct {
	id                                 string
	typ                                string
	absX, absY, w, h                   int
	fs, fw, pad, br                    int
	opacity                            float64
	entranceOp                         float64 // 1 when settled; multiplies style opacity
	animating                          bool
	textAlign                          string
	color, bg                          color.RGBA
	strokeW                            float64
	stroke                             color.RGBA
	marginT, marginB, marginL, marginR int
	contentW, contentH                 int // scroll overflow
}

func collectLayoutSnaps(ln *LayoutNode, out map[string]measureSnap) {
	if ln == nil || ln.Node == nil {
		return
	}
	if id := ln.Node.ID; id != "" {
		s := ln.Style
		op := s.Opacity
		if op <= 0 {
			op = 1
		}
		entOp := 1.0
		anim := false
		if ln.EntranceActive {
			anim = true
			entOp = ln.EntranceOpacity
			if entOp <= 0 {
				entOp = 0
			}
		}
		out[id] = measureSnap{
			id: id, typ: ln.Node.Type,
			absX: ln.AbsX, absY: ln.AbsY, w: ln.Width, h: ln.Height,
			fs: s.FontSize, fw: s.FontWeight, pad: s.Padding,
			br: int(s.BorderRadius), opacity: op, entranceOp: entOp, animating: anim,
			textAlign: s.TextAlign,
			color:     s.Color, bg: s.Background,
			strokeW: s.StrokeWidth, stroke: s.StrokeColor,
			marginT: s.MarginTop, marginB: s.MarginBot,
			marginL: s.MarginLeft, marginR: s.MarginRight,
			contentW: ln.ContentW, contentH: ln.ContentH,
		}
	}
	for _, c := range ln.Children {
		collectLayoutSnaps(c, out)
	}
}

// effectiveOpacity multiplies style opacity by entrance fade (HTML-like
// computed opacity for visibility).
func effectiveOpacity(styleOp, entranceOp float64, graphOp float64) float64 {
	op := styleOp
	if op <= 0 {
		op = 1
	}
	if entranceOp > 0 && entranceOp < 1 {
		op *= entranceOp
	} else if entranceOp == 0 && styleOp > 0 {
		// Explicit zero entrance (delay window of a fade).
		op = 0
	}
	// Graph opacity already includes entrance when applied; prefer the lower
	// of the two so we never over-claim visibility.
	if graphOp > 0 && graphOp < op {
		op = graphOp
	}
	return op
}

func measureRowFromGraph(m *model.Node, n graph.Node, b *graph.BaseNode, sn measureSnap, scale int, logical bool, rt *runtime.Runtime) map[string]any {
	bb := n.GetBBox()
	x, y := bb.MinX, bb.MinY
	w, h := bb.MaxX-bb.MinX, bb.MaxY-bb.MinY
	// Prefer layout Abs when available (stable before matrix jitter).
	if sn.id != "" {
		x, y = float64(sn.absX), float64(sn.absY)
		w, h = float64(sn.w), float64(sn.h)
	}
	if logical && scale > 1 {
		sf := float64(scale)
		x, y, w, h = x/sf, y/sf, w/sf, h/sf
	}
	styleOp, entOp := 1.0, 1.0
	if sn.id != "" {
		styleOp, entOp = sn.opacity, sn.entranceOp
		if entOp <= 0 && !sn.animating {
			entOp = 1
		}
	}
	op := effectiveOpacity(styleOp, entOp, b.Opacity)
	// HTML: visible only when size and opacity are non-trivial.
	vis := w > 0.5 && h > 0.5 && op > 0.01
	row := map[string]any{
		"id":       m.ID,
		"tag":      "canvas",
		"type":     m.Type,
		"x":        roundPx(x),
		"y":        roundPx(y),
		"w":        roundPx(w),
		"h":        roundPx(h),
		"visible":  vis,
		"text":     measureTextOf(m, rt),
		"display":  "block",
		"opacity":  fmt.Sprintf("%.3g", op),
		"position": "relative",
		"scale":    scale,
		"logical":  logical,
	}
	if sn.animating {
		row["animating"] = true
	}
	if sn.id != "" {
		enrichStyle(row, sn, scale, logical)
	}
	// a11y-ish props mirrored from HTML measure.
	if raw, ok := m.Prop("ariaLabel"); ok {
		row["ariaLabel"] = fmt.Sprint(raw)
	}
	if raw, ok := m.Prop("role"); ok {
		row["role"] = fmt.Sprint(raw)
	} else {
		row["role"] = ""
	}
	row["tabindex"] = ""
	return row
}

func measureRowFromSnap(id string, sn measureSnap, scale int, logical bool, rt *runtime.Runtime) map[string]any {
	x, y, w, h := float64(sn.absX), float64(sn.absY), float64(sn.w), float64(sn.h)
	if logical && scale > 1 {
		sf := float64(scale)
		x, y, w, h = x/sf, y/sf, w/sf, h/sf
	}
	entOp := sn.entranceOp
	if entOp <= 0 && !sn.animating {
		entOp = 1
	}
	op := effectiveOpacity(sn.opacity, entOp, 0)
	row := map[string]any{
		"id": id, "tag": "canvas", "type": sn.typ,
		"x": roundPx(x), "y": roundPx(y), "w": roundPx(w), "h": roundPx(h),
		"visible": w > 0.5 && h > 0.5 && op > 0.01,
		"text":    "", "display": "block",
		"opacity": fmt.Sprintf("%.3g", op),
		"scale":   scale, "logical": logical,
	}
	if sn.animating {
		row["animating"] = true
	}
	enrichStyle(row, sn, scale, logical)
	return row
}

func enrichStyle(row map[string]any, sn measureSnap, scale int, logical bool) {
	div := 1.0
	if logical && scale > 1 {
		div = float64(scale)
	}
	if sn.fs > 0 {
		row["fontSize"] = fmt.Sprintf("%.0fpx", float64(sn.fs)/div)
	}
	if sn.fw > 0 {
		row["fontWeight"] = fmt.Sprintf("%d", sn.fw)
	}
	if sn.textAlign != "" {
		row["textAlign"] = sn.textAlign
	}
	if sn.pad > 0 {
		p := float64(sn.pad) / div
		row["padding"] = fmt.Sprintf("%.0fpx", p)
	}
	if sn.br > 0 {
		row["borderRadius"] = fmt.Sprintf("%.0fpx", float64(sn.br)/div)
	}
	if sn.color.A > 0 {
		row["color"] = cssRGBA(sn.color)
	}
	if sn.bg.A > 0 {
		row["background"] = cssRGBA(sn.bg)
	} else {
		row["background"] = "rgba(0, 0, 0, 0)"
	}
	if sn.strokeW > 0 && sn.stroke.A > 0 {
		row["border"] = fmt.Sprintf("%.0fpx solid %s", sn.strokeW/div, cssRGBA(sn.stroke))
	} else {
		row["border"] = "none"
	}
	// margin shorthand top/right/bottom/left
	if sn.marginT != 0 || sn.marginR != 0 || sn.marginB != 0 || sn.marginL != 0 {
		row["margin"] = fmt.Sprintf("%.0fpx %.0fpx %.0fpx %.0fpx",
			float64(sn.marginT)/div, float64(sn.marginR)/div,
			float64(sn.marginB)/div, float64(sn.marginL)/div)
	}
	// Scroll overflow hints (HTML overflowX/Y boolean-ish).
	if sn.contentW > sn.w {
		row["overflowX"] = true
	}
	if sn.contentH > sn.h {
		row["overflowY"] = true
	}
	row["zIndex"] = "auto"
}

func cssRGBA(c color.RGBA) string {
	if c.A == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.3g)", c.R, c.G, c.B, float64(c.A)/255)
}

func roundPx(v float64) float64 {
	if v < 0 {
		return float64(int(v - 0.5))
	}
	return float64(int(v + 0.5))
}

func measureTextOf(n *model.Node, rt *runtime.Runtime) string {
	if n == nil {
		return ""
	}
	var s string
	switch {
	case n.Text != "":
		s = n.Text
	case n.Label != "":
		s = n.Label
	case n.Value != "":
		s = n.Value
	}
	if s == "" {
		return ""
	}
	if rt != nil && strings.Contains(s, "{{") {
		s = evalPropStr(s, rt)
	}
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// MeasureScene is a headless one-shot: layout+paint into an offscreen buffer
// and return CollectMeasure rows in LOGICAL CSS px (scale-independent), which
// is what agent checks and design tokens expect. Entrance animations are
// force-settled so CLI/MCP snapshots are deterministic (no mid-fade boxes).
func MeasureScene(rt *runtime.Runtime, width, height, scale int) []byte {
	return MeasureSceneOpts(rt, width, height, scale, MeasureOpts{Logical: true})
}

// MeasureSceneOpts is MeasureScene with Logical/physical control.
func MeasureSceneOpts(rt *runtime.Runtime, width, height, scale int, opts MeasureOpts) []byte {
	if rt == nil {
		return []byte("[]")
	}
	if width < 1 {
		width = 400
	}
	if height < 1 {
		height = 820
	}
	if scale < 1 {
		scale = 1
	}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(width*scale, height*scale))
	e.DrawFrame(surf)
	e.SettleEntrances()
	e.DrawFrame(surf)
	return e.CollectMeasureOpts(opts)
}

// SettleEntrances rewinds every entrance clock so the next layout frame paints
// fully settled geometry/opacity. Used by MeasureScene for deterministic
// agent verification; live hosts leave entrances alone.
func (e *Engine) SettleEntrances() {
	if e == nil || e.Inter.Entrance == nil {
		return
	}
	past := time.Now().Add(-time.Hour)
	for k, st := range e.Inter.Entrance {
		if st == nil {
			continue
		}
		st.start = past
		e.Inter.Entrance[k] = st
	}
	e.dirty.Store(true)
}
