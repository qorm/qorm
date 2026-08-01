package canvas

import (
	"fmt"
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

type LayoutNode struct {
	Node        *model.Node
	Style       NodeStyle
	Text        string
	Width       int
	Height      int
	X           int
	Y           int
	NeedsRedraw bool
	Children    []*LayoutNode
	
	// Retained mode scene graph node backing this layout
	GraphNode   graph.Node
}

// Measure does a bottom-up pass to calculate minimum content sizes. scale is
// the device-pixel ratio: design pixels are multiplied by it so the resulting
// geometry is in physical pixels (HiDPI). Pass 1 for logical == physical.
func Measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int) *LayoutNode {
	return measure(n, rt, inter, scale, n)
}

// measure is the recursive body of Measure; root identifies the scene tree
// for the one-shot unsupported-style-key warnings.
func measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int, root *model.Node) *LayoutNode {
	if n == nil {
		return nil
	}

	// Conditional rendering, mirroring the HTML path (render.go:377 the
	// node() entry check + render.go:771 when): an `if`/`visible`/`show` prop
	// hides the whole subtree; a `when` node swaps in its Then/Else branch —
	// which may itself be conditional or another `when`, hence the loop.
	for {
		if !nodeVisible(n, rt) {
			return nil
		}
		if n.Type != "when" {
			break
		}
		n = whenBranch(n, rt)
		if n == nil {
			return nil
		}
	}

	warnUnsupportedStyleKeys(root, n)

	style := parseStyle(n, rt)
	applyInteractiveOverlay(&style, n, rt, inter)
	style.scaleBy(scale)
	var needsRedraw bool
	if n.Type == "animated_container" {
		style, needsRedraw = UpdateAndGetAnimatedStyle(n.ID, style, rt)
	}

	ln := &LayoutNode{
		Node:        n,
		Style:       style,
		NeedsRedraw: needsRedraw,
	}

	if n.Type == "text" {
		if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStr(t, rt)
		} else if v, ok := n.Props["value"]; ok {
			ln.Text = evalPropStr(v, rt)
		}
	} else if n.Type == "button" {
		// Evaluate bindings in the label too (e.g. "Toggle ({{state.theme}})"),
		// matching text nodes — otherwise the raw template shows literally.
		if t, ok := n.Props["label"]; ok {
			ln.Text = evalPropStr(t, rt)
		} else if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStr(t, rt)
		}
	}

	for _, child := range n.Children {
		if cln := measure(child, rt, inter, scale, root); cln != nil {
			if cln.NeedsRedraw {
				ln.NeedsRedraw = true
			}
			ln.Children = append(ln.Children, cln)
		}
	}

	fs := style.FontSize
	if fs == 0 {
		fs = 14
	}

	contentW, contentH := 0, 0

	if n.Type == "text" {
		contentW = int(MeasureText(ln.Text, float64(fs)))
		contentH = int(float64(fs) * 1.2)
	} else if n.Type == "button" {
		contentW = int(MeasureText(ln.Text, float64(fs))) + 40*scale
		contentH = int(float64(fs)*1.2) + 20*scale
	} else if isStackType(n.Type) {
		// Stack: children share one origin, so the content size is the
		// largest child on each axis (HTML sizes the position:relative
		// container the same way for its in-flow children).
		for _, child := range ln.Children {
			if cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight; cw > contentW {
				contentW = cw
			}
			if ch := child.Height + child.Style.MarginTop + child.Style.MarginBot; ch > contentH {
				contentH = ch
			}
		}
	} else if n.Type == "grid" {
		// Grid: `columns` equal tracks (1fr in HTML, render_style.go:103) —
		// every column is as wide as the widest child; each row as tall as
		// its tallest child. Gap applies between tracks both ways.
		cols := gridColumns(n)
		colW := 0
		for _, child := range ln.Children {
			if cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight; cw > colW {
				colW = cw
			}
		}
		if len(ln.Children) > 0 {
			contentW = cols*colW + (cols-1)*style.Gap
			rows := (len(ln.Children) + cols - 1) / cols
			for r := 0; r < rows; r++ {
				rowH := 0
				for c := 0; c < cols && r*cols+c < len(ln.Children); c++ {
					child := ln.Children[r*cols+c]
					if ch := child.Height + child.Style.MarginTop + child.Style.MarginBot; ch > rowH {
						rowH = ch
					}
				}
				contentH += rowH
				if r > 0 {
					contentH += style.Gap
				}
			}
		}
	} else {
		isRow := n.Type == "row"
		for i, child := range ln.Children {
			cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
			ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
			if isRow {
				contentW += cw
				if i > 0 {
					contentW += style.Gap
				}
				if ch > contentH {
					contentH = ch
				}
			} else {
				contentH += ch
				if i > 0 {
					contentH += style.Gap
				}
				if cw > contentW {
					contentW = cw
				}
			}
		}
	}

	contentW += style.Padding * 2
	contentH += style.Padding * 2

	ln.Width = contentW
	if style.Width > 0 {
		ln.Width = style.Width
	}
	ln.Height = contentH
	if style.Height > 0 {
		ln.Height = style.Height
	}

	return ln
}

func evalPropStr(val any, rt *runtime.Runtime) string {
	if s, ok := val.(string); ok && rt != nil {
		res := runtime.EvalBinding(s, evalCtx(rt))
		if res == nil {
			return ""
		}
		if str, ok := res.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", res)
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// evalCtx is the binding scope canvas evaluates node text, style values and
// conditions against: `state` plus `viewport` (Layout feeds the live surface
// size into rt.Viewport). The HTML renderer additionally offers the
// t/route/computed roots (render.go:345); those are not wired in the native
// engine yet, so bindings over them miss and expand to "".
func evalCtx(rt *runtime.Runtime) map[string]any {
	if rt == nil {
		return map[string]any{}
	}
	return map[string]any{"state": rt.State, "viewport": rt.ViewportVars()}
}

// nodeVisible evaluates an `if` / `visible` / `show` condition (default
// true) — the same keys, first-match order and truthiness as the HTML
// renderer's visible() (render.go:782).
func nodeVisible(n *model.Node, rt *runtime.Runtime) bool {
	for _, key := range []string{"if", "visible", "show"} {
		if raw, ok := n.Prop(key); ok {
			return truthy(runtime.EvalBinding(fmt.Sprint(raw), evalCtx(rt)))
		}
	}
	return true
}

// nodeMounted reports whether target is reachable from n through the
// currently-visible tree: every node on the path passes its own
// if/visible/show, and each `when` ancestor's selected branch still contains
// target. Used to re-validate a held identity (e.g. keyboard focus) against a
// tree whose conditions have since flipped.
func nodeMounted(n, target *model.Node, rt *runtime.Runtime) bool {
	if n == nil || !nodeVisible(n, rt) {
		return false
	}
	if n == target {
		return true
	}
	if n.Type == "when" {
		return nodeMounted(whenBranch(n, rt), target, rt)
	}
	for _, c := range n.Children {
		if nodeMounted(c, target, rt) {
			return true
		}
	}
	return false
}

// whenBranch mirrors the HTML when() (render.go:771): a truthy Condition
// selects the Then subtree, anything else — including a missed binding or an
// unknown viewport — selects the Else subtree (possibly nil).
func whenBranch(n *model.Node, rt *runtime.Runtime) *model.Node {
	branch := n.Else
	if n.Condition != "" && truthy(runtime.EvalBinding(n.Condition, evalCtx(rt))) {
		branch = n.Then
	}
	return branch
}

// truthy matches the HTML renderer's asBool (render_style.go:1042): bools
// pass through, numbers are true when non-zero, strings only when "true"/"1";
// everything else (nil, maps, missed bindings) is false.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "true" || t == "1"
	}
	return false
}

// isStackType reports the layer-container spellings (the types the HTML path
// marks position:relative, render_style.go:115): children share one origin
// and paint in declaration order, later siblings on top.
func isStackType(t string) bool { return t == "stack" || t == "absolute" }

// gridColumns reads a grid's column count from the `columns` prop (HTML:
// propNum(n, "columns", 2), render_style.go:104), clamped to [1, maxGridColumns].
// The upper clamp is load-bearing, not cosmetic: a huge JSON float (1e19)
// converts to int platform-dependently (arm64 saturates to maxInt64), and
// len(children)+cols-1 would then overflow in the row-count math below.
const maxGridColumns = 4096

func gridColumns(n *model.Node) int {
	cols := 2
	if v, ok := n.Prop("columns"); ok {
		switch c := v.(type) {
		case float64:
			cols = int(c)
		case int:
			cols = c
		}
	}
	if cols < 1 {
		cols = 1
	}
	if cols > maxGridColumns {
		cols = maxGridColumns
	}
	return cols
}

// PerformLayout does the top-down pass, building the scene graph. inter and
// rt stamp interaction state and resolve theme-driven decorations; scale is
// the device-pixel ratio (used for the focus-ring insets so its visual width
// stays constant in physical pixels).
func PerformLayout(ln *LayoutNode, bounds image.Rectangle, inter *Interaction, rt *runtime.Runtime, scale int) graph.Node {
	if ln == nil {
		return nil
	}

	if ln.Style.WidthRaw == "fill" {
		ln.Width = bounds.Dx() - ln.Style.MarginLeft - ln.Style.MarginRight
	}
	if ln.Style.HeightRaw == "fill" {
		ln.Height = bounds.Dy() - ln.Style.MarginTop - ln.Style.MarginBot
	}

	x := bounds.Min.X + ln.Style.MarginLeft
	y := bounds.Min.Y + ln.Style.MarginTop

	availW := bounds.Dx() - ln.Style.MarginLeft - ln.Style.MarginRight
	availH := bounds.Dy() - ln.Style.MarginTop - ln.Style.MarginBot

	if ln.Style.Align == "center" {
		x += (availW - ln.Width) / 2
	}

	if ln.Style.Justify == "center" {
		y += (availH - ln.Height) / 2
	}

	ln.X = x
	ln.Y = y

	group := graph.NewGroup()
	group.X = float64(x)
	group.Y = float64(y)
	group.Width = float64(ln.Width)
	group.Height = float64(ln.Height)
	group.Model = ln.Node
	if inter != nil {
		group.Pressed = inter.Pressed == ln.Node
		group.Hovered = inter.Hovered == ln.Node
		group.Focused = inter.Focused == ln.Node
	}
	if ln.Style.Opacity < 1 {
		// Node-level opacity (style key, theme pressedOpacity or an
		// animation): the group's OpacityOp multiplies through the whole
		// subtree in the rasterizer — the same group-alpha semantics CSS
		// opacity has. 0 hides the subtree visually (it still hit-tests,
		// like CSS opacity:0).
		group.Opacity = ln.Style.Opacity
	}

	if ln.Node.OnPress != nil {
		group.OnPress = ln.Node.OnPress
	}
	if ln.Node.OnCollide != nil {
		group.OnCollide = ln.Node.OnCollide
	}
	if ln.Node.OnKeyDown != nil {
		group.OnKeyDown = ln.Node.OnKeyDown
	}
	if ln.Node.OnKeyUp != nil {
		group.OnKeyUp = ln.Node.OnKeyUp
	}
	if ln.Node.OnHoverIn != nil {
		group.OnHoverIn = ln.Node.OnHoverIn
	}
	if ln.Node.OnHoverOut != nil {
		group.OnHoverOut = ln.Node.OnHoverOut
	}
	if ln.Node.OnTouchStart != nil {
		group.OnTouchStart = ln.Node.OnTouchStart
	}
	if ln.Node.OnTouchMove != nil {
		group.OnTouchMove = ln.Node.OnTouchMove
	}
	if ln.Node.OnTouchEnd != nil {
		group.OnTouchEnd = ln.Node.OnTouchEnd
	}

	hasBg := ln.Style.Background.A > 0
	hasStroke := ln.Style.StrokeColor.A > 0 && ln.Style.StrokeWidth > 0
	hasShadow := ln.Style.BoxShadowColor.A > 0
	
	if hasBg || hasStroke || hasShadow {
		bg := graph.NewRect()
		bg.X = 0
		bg.Y = 0
		bg.Width = float64(ln.Width)
		bg.Height = float64(ln.Height)
		bg.Fill = ln.Style.Background
		bg.BorderRadius = float64(ln.Style.BorderRadius)
		
		if hasStroke {
			bg.Stroke = ln.Style.StrokeColor
			bg.StrokeWidth = ln.Style.StrokeWidth
		}
		
		if hasShadow {
			bg.ShadowColor = ln.Style.BoxShadowColor
			bg.ShadowBlur = float64(ln.Style.BoxShadowBlur)
			bg.ShadowY = float64(ln.Style.BoxShadowY)
		}
		
		group.AddChild(bg)
	}

	if ln.Text != "" {
		fs := ln.Style.FontSize
		if fs == 0 {
			fs = 14
		}

			txtW := int(MeasureText(ln.Text, float64(fs)))
		txtH := int(float64(fs) * 1.2)
		
		tx := 0
		if ln.Style.TextAlign == "center" || ln.Node.Type == "button" {
			tx = (ln.Width - txtW) / 2
		}
		
		ty := (ln.Height - txtH) / 2

		c := ln.Style.Color
		if c.A == 0 {
			c = color.RGBA{255, 255, 255, 255}
		}

		textNode := graph.NewText()
		textNode.X = float64(tx)
		textNode.Y = float64(ty)
		textNode.Content = ln.Text
		textNode.Fill = c
		textNode.FontSize = float64(fs)
		group.AddChild(textNode)
	}

	cx := ln.Style.Padding
	cy := ln.Style.Padding
	isRow := ln.Node.Type == "row"
	isStack := isStackType(ln.Node.Type)
	isGrid := ln.Node.Type == "grid"

	totalCW, totalCH := 0, 0
	for i, child := range ln.Children {
		if isRow {
			totalCW += child.Width + child.Style.MarginLeft + child.Style.MarginRight
			if i > 0 {
				totalCW += ln.Style.Gap
			}
		} else {
			totalCH += child.Height + child.Style.MarginTop + child.Style.MarginBot
			if i > 0 {
				totalCH += ln.Style.Gap
			}
		}
	}

	innerW := ln.Width - ln.Style.Padding*2
	innerH := ln.Height - ln.Style.Padding*2

	if isRow && ln.Style.Justify == "center" {
		cx += (innerW - totalCW) / 2
	}
	if !isRow && !isStack && !isGrid && ln.Style.Justify == "center" {
		cy += (innerH - totalCH) / 2
	}

	// Grid track geometry: columns share the inner width equally (1fr), never
	// narrower than the widest child; row heights come from the tallest child
	// in each row, matching the measure pass.
	gridCols, gridColW := 0, 0
	var gridRowH []int
	if isGrid && len(ln.Children) > 0 {
		gridCols = gridColumns(ln.Node)
		gridColW = (innerW - (gridCols-1)*ln.Style.Gap) / gridCols
		for _, child := range ln.Children {
			if cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight; cw > gridColW {
				gridColW = cw
			}
		}
		if gridColW < 0 {
			gridColW = 0
		}
		gridRowH = make([]int, (len(ln.Children)+gridCols-1)/gridCols)
		for i, child := range ln.Children {
			ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
			if r := i / gridCols; ch > gridRowH[r] {
				gridRowH[r] = ch
			}
		}
	}

	for i, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot

		// Set alignment for cross axis
		if child.Style.Align == "" {
			child.Style.Align = ln.Style.Align // inherit parent alignItems
		}

		var cbounds image.Rectangle
		switch {
		case isStack:
			// Layered: every child gets the full content box at the same
			// origin; declaration order is the z-order (later siblings paint
			// — and hit-test — on top). The child's own align/justify (the
			// stack's, inherited) positions it inside the box. HTML places
			// such children with position+top/left (render_style.go:293),
			// which canvas does not implement — those keys warn as
			// unsupported instead of degrading silently.
			if child.Style.Justify == "" {
				child.Style.Justify = ln.Style.Justify
			}
			cbounds = image.Rect(cx, cy, cx+innerW, cy+innerH)
		case isGrid:
			col, row := i%gridCols, i/gridCols
			gx := cx + col*(gridColW+ln.Style.Gap)
			gy := cy
			for r := 0; r < row; r++ {
				gy += gridRowH[r] + ln.Style.Gap
			}
			cbounds = image.Rect(gx, gy, gx+gridColW, gy+gridRowH[row])
		default:
			cbounds = image.Rect(cx, cy, cx+cw, cy+ch)

			if isRow {
				if child.Style.Align == "center" {
					cbounds = image.Rect(cx, cy, cx+cw, cy+innerH)
				}
			} else {
				if child.Style.Align == "center" {
					cbounds = image.Rect(cx, cy, cx+innerW, cy+ch)
				}
			}
		}

		childNode := PerformLayout(child, cbounds, inter, rt, scale)
		if childNode != nil {
			group.AddChild(childNode)
		}

		if isRow {
			cx += cw + ln.Style.Gap
		} else if !isStack && !isGrid {
			cy += ch + ln.Style.Gap
		}
	}

	// Keyboard focus ring (focus-visible): only drawn when focus was
	// established by the keyboard, offset 3px outside the node body.
	// NoHit keeps the oversized ring from stealing pointer hits.
	if inter != nil && inter.Focused == ln.Node && inter.FocusVisible {
		s := scale
		if s < 1 {
			s = 1
		}
		ring := graph.NewRect()
		ring.NoHit = true
		ring.X = -3 * float64(s)
		ring.Y = -3 * float64(s)
		ring.Width = float64(ln.Width + 6*s)
		ring.Height = float64(ln.Height + 6*s)
		ring.BorderRadius = ln.Style.BorderRadius + 3*float64(s)
		ring.Stroke = resolveFocusColor(rt)
		ring.StrokeWidth = 2 * float64(s)
		group.AddChild(ring)
	}

	return group
}
