package canvas

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// CollectMeasure walks the last rendered graph and emits HTML-compatible
// measurement rows (same shape as app.js qormMeasure → POST /measure):
// id, x, y, w, h, visible, text, plus a few style fields from LayoutNode
// style when available. Agents use this for qorm_measure / qorm_check_layout
// against the pure-Go canvas path (not the WebView DOM).
func (e *Engine) CollectMeasure() []byte {
	if e == nil || e.graphRoot == nil {
		return []byte("[]")
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
			bb := n.GetBBox()
			w := bb.MaxX - bb.MinX
			h := bb.MaxY - bb.MinY
			vis := w > 0.5 && h > 0.5 && b.Opacity > 0.01
			row := map[string]any{
				"id":      m.ID,
				"tag":     "canvas",
				"type":    m.Type,
				"x":       roundPx(bb.MinX),
				"y":       roundPx(bb.MinY),
				"w":       roundPx(w),
				"h":       roundPx(h),
				"visible": vis,
				"text":    measureTextOf(m, e.RT),
				"opacity": fmt.Sprintf("%.3g", b.Opacity),
			}
			if b.Opacity <= 0 {
				row["opacity"] = "0"
			}
			rows = append(rows, row)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(e.graphRoot)
	b, err := json.Marshal(rows)
	if err != nil {
		return []byte("[]")
	}
	return b
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
// and return CollectMeasure rows. Used by `qorm measure` on pure-Go builds
// (no WebView). scale defaults to 1; width/height are logical stage size.
func MeasureScene(rt *runtime.Runtime, width, height, scale int) []byte {
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
	// Drive a couple of frames so entrance clocks settle enough for boxes.
	e.DrawFrame(surf)
	e.DrawFrame(surf)
	return e.CollectMeasure()
}
