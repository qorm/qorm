package canvas

// canvas → internal/layout bridge: the flexbox subset mapping. measure.go's
// container layout (column/row) goes through layout.Flex — the CSS model —
// instead of the old hand-rolled loop; stack/grid keep their own passes.

import (
	flexlayout "github.com/qorm/qorm/internal/layout"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// flexStyle maps a container's NodeStyle + node props to flexlayout.Style.
// Canvas spellings follow the HTML path (render_style.go): align/justify
// accept center/start/end and the space-* justify values; wrap is a node prop.
func flexStyle(ln *LayoutNode, rt *runtime.Runtime) flexlayout.Style {
	st := flexlayout.Style{
		Gap:     float64(ln.Style.Gap),
		Items:   flexAlign(ln.Style.Align),
		Justify: flexJustify(ln.Style.Justify),
	}
	if ln.Node.Type == "row" {
		st.Direction = flexlayout.Row
	} else {
		st.Direction = flexlayout.Column
	}
	if v, ok := ln.Node.Prop("wrap"); ok && styleTruthy(v) {
		st.Wrap = true
	}
	return st
}

// flexAlign maps align/align-self spellings; "" is Stretch (CSS's initial
// align-items), which the engine only applies to auto cross sizes.
func flexAlign(s string) flexlayout.Align {
	switch s {
	case "center":
		return flexlayout.AlignCenter
	case "start", "flex-start":
		return flexlayout.AlignStart
	case "end", "flex-end":
		return flexlayout.AlignEnd
	default:
		return flexlayout.AlignStretch
	}
}

// flexAlignSelf maps a child's align-self: "" means inherit (the engine's
// AlignInherit sentinel follows the container's align-items).
func flexAlignSelf(s string) flexlayout.Align {
	if s == "" {
		return flexlayout.AlignInherit
	}
	return flexAlign(s)
}

func flexJustify(s string) flexlayout.Justify {
	switch s {
	case "center":
		return flexlayout.JustifyCenter
	case "end", "flex-end":
		return flexlayout.JustifyEnd
	case "space-between", "between":
		return flexlayout.JustifySpaceBetween
	case "space-around", "around":
		return flexlayout.JustifySpaceAround
	case "space-evenly", "evenly":
		return flexlayout.JustifySpaceEvenly
	default:
		return flexlayout.JustifyStart
	}
}

// flexChildren maps measured children to flexlayout.Child. spacer and any
// flexGrow style become flex-grow (the CSS primitive the hand loop could not
// express); everything else keeps intrinsic sizes with margins on both axes.
// flexChildren maps measured children to layout.Child. Cross-axis AUTO sizes
// (no explicit width in a column / no explicit height in a row, and not
// "fill") pass as 0 — that is the value align-items:stretch stretches (CSS
// only stretches auto cross sizes); explicit sizes pass through. spacer and
// flexGrow become flex-grow; margins ride on both axes.
func flexChildren(ln *LayoutNode, rt *runtime.Runtime) []flexlayout.Child {
	row := ln.Node.Type == "row"
	out := make([]flexlayout.Child, len(ln.Children))
	for i, c := range ln.Children {
		ch := flexlayout.Child{
			MarginT:   float64(c.Style.MarginTop),
			MarginR:   float64(c.Style.MarginRight),
			MarginB:   float64(c.Style.MarginBot),
			MarginL:   float64(c.Style.MarginLeft),
			AlignSelf: flexAlignSelf(c.Style.Align),
		}
		if row {
			ch.W = float64(c.Width)
			if c.Style.Height != 0 || c.Style.HeightRaw == "fill" || !stretchable(ln, c) {
				ch.H = float64(c.Height) // sized (explicit or non-stretch align): keep
			}
		} else {
			ch.H = float64(c.Height)
			if c.Style.Width != 0 || c.Style.WidthRaw == "fill" || !stretchable(ln, c) {
				ch.W = float64(c.Width)
			}
		}
		// flex-grow: the spacer widget is grow:1 with a zero basis (HTML's
		// flex-basis:0); a flexGrow style key gives an explicit factor.
		if c.Node.Type == "spacer" {
			ch.Grow = 1
		}
		if g := flexGrowOf(c.Node, rt); g > 0 {
			ch.Grow = g
		}
		out[i] = ch
	}
	return out
}

// flexGrowOf reads the flexGrow style key (HTML: flex-grow, render_style.go),
// default 0. Bound values evaluate.
func flexGrowOf(n *model.Node, rt *runtime.Runtime) float64 {
	v, ok := n.Style["flexGrow"]
	if !ok {
		return 0
	}
	switch t := evalStyleProp(v, rt).(type) {
	case float64:
		return t
	case int:
		return float64(t)
	}
	return 0
}

// flexRectToBounds converts an engine Rect (the child's own box, margins
// excluded) into canvas's margin-box bounds (what performLayout expects: the
// node adds its own margins on top).
func flexRectToBounds(r flexlayout.Rect, c *LayoutNode, padX, padY int) (x0, y0, x1, y1 int) {
	x0 = padX + int(r.X) - c.Style.MarginLeft
	y0 = padY + int(r.Y) - c.Style.MarginTop
	x1 = padX + int(r.X+r.W) + c.Style.MarginRight
	y1 = padY + int(r.Y+r.H) + c.Style.MarginBot
	return
}

// styleTruthy mirrors the prop truthiness used for booleans in styles
// (bool true / "true" / "1" / non-zero number).
func styleTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}

// clampInt applies min/max (0 = unset) to a resolved size.
func clampInt(v, min, max int) int {
	if max > 0 && v > max {
		v = max
	}
	if min > 0 && v < min {
		v = min
	}
	return v
}

// stretchable reports whether a child's effective align resolves to stretch
// on the cross axis — its own align wins, otherwise the container's
// align-items (CSS align-self:auto follows align-items).
func stretchable(ln, c *LayoutNode) bool {
	eff := c.Style.Align
	if eff == "" {
		eff = ln.Style.Align
	}
	return eff == "" || eff == "stretch"
}
