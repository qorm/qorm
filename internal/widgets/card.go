package widgets

import (
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("card", Card{})
}

// Card is the surfaced content container (HTML containerCSS card:
// background var(--surface), border-radius 14-16px, padding 16px,
// render_style.go:129 — the canvas theme carries the same defaults as
// Components["card"]: cardBg / radius 16 / padding 16, theme.go:207).
// Children flow through the generic engine layout; the widget owns the
// type-level defaults: it measures its subtree (the registry seam makes a
// widget's children NOT count toward its size, canvas/widget.go) and paints
// the rounded background when the theme did not already supply one (the
// generic container chrome paints ln.Style.Background/BorderRadius first, so
// a themed card needs no shape from Record at all).
type Card struct{}

const (
	cardPadding = 16
	cardRadius  = 16
)

// cardPaddingSet reports whether the card's padding comes from config (theme
// Components["card"] or an author style key) rather than the 16px default.
// Only literal numbers resolve — a bound padding ("{{state.p}}") is treated
// as unset here while parseStyle does evaluate it, so a bound card padding
// measures loose by (16 - value)px. Documented edge; literal paddings (the
// common case) are exact.
func cardPaddingSet(n *model.Node, rt *runtime.Runtime) bool {
	if _, ok := styleNumber(n, "padding"); ok {
		return true
	}
	if rt != nil && rt.Theme != nil {
		if comp, ok := rt.Theme.Components["card"]; ok && comp.Padding != nil {
			return true
		}
	}
	return false
}

// Measure reports the column-stacked children content plus padding, using
// canvas.Measure per child (the exported engine entry). Conditional children
// measure nil and drop out, matching the engine's container pass.
func (Card) Measure(n *model.Node, rt *runtime.Runtime, vars map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	pad, gap := 0, 0
	if rt != nil && rt.Theme != nil {
		if comp, ok := rt.Theme.Components["card"]; ok {
			if comp.Padding != nil {
				pad = *comp.Padding
			}
			if comp.Gap != nil {
				gap = *comp.Gap
			}
		}
	}
	if v, ok := styleNumber(n, "padding"); ok {
		pad = v
	}
	if v, ok := styleNumber(n, "gap"); ok {
		gap = v
	}

	contentW, contentH, count := 0, 0, 0
	for _, c := range n.Children {
		cln := canvas.MeasureScoped(c, rt, nil, vars, scale) // scoped: {{item.*}} bindings must evaluate
		if cln == nil {
			continue
		}
		if cw := cln.Width + cln.Style.MarginLeft + cln.Style.MarginRight; cw > contentW {
			contentW = cw
		}
		contentH += cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		if count > 0 {
			contentH += gap * scale
		}
		count++
	}

	// canvas.measure adds style.Padding*2 on top of a widget's report
	// (measure.go:248), so report only the padding the engine will NOT add:
	// nothing when theme/author set one, the 16px default otherwise.
	if !cardPaddingSet(n, rt) {
		pad = cardPadding
		contentW += pad * 2 * scale
		contentH += pad * 2 * scale
	}
	return contentW, contentH
}

// Record injects the type-level padding default (children mount at
// cx/cy = ln.Style.Padding AFTER Record returns, measure.go:597) and paints
// the rounded background only when the theme left it transparent.
func (Card) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Style.Padding == 0 && !cardPaddingSet(ln.Node, rt) {
		ln.Style.Padding = cardPadding * scale
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if ln.Style.Background.A > 0 {
		return nil // generic chrome already paints bg + radius (+stroke/shadow)
	}
	radius := ln.Style.BorderRadius
	if radius == 0 {
		radius = cardRadius * float64(scale)
	}
	bg := draw.NewRect()
	bg.Width = float64(ln.Width)
	bg.Height = float64(ln.Height)
	bg.BorderRadius = radius
	bg.Fill = themeColor(rt, "cardBg", color.RGBA{255, 255, 255, 255})
	return bg
}

// styleNumber reads a literal numeric style key (JSON numbers arrive as
// float64), mirroring the parseStyle conversions. Bound strings resolve
// through parseStyle but not here — see cardPaddingSet's note.
func styleNumber(n *model.Node, key string) (int, bool) {
	if n.Style == nil {
		return 0, false
	}
	switch v := n.Style[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}
