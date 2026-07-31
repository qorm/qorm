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
	if n == nil {
		return nil
	}

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
		if t, ok := n.Props["label"].(string); ok {
			ln.Text = t
		}
	}

	for _, child := range n.Children {
		if cln := Measure(child, rt, inter, scale); cln != nil {
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
		contentW = int(float64(len(ln.Text)) * float64(fs) * 0.6)
		contentH = int(float64(fs) * 1.2)
	} else if n.Type == "button" {
		contentW = int(float64(len(ln.Text))*float64(fs)*0.6) + 40*scale
		contentH = int(float64(fs)*1.2) + 20*scale
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
		res := runtime.EvalBinding(s, map[string]any{"state": rt.State})
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
	if ln.Style.Opacity > 0 && ln.Style.Opacity < 1 {
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

		txtW := int(float64(len(ln.Text)) * float64(fs) * 0.6)
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
	if !isRow && ln.Style.Justify == "center" {
		cy += (innerH - totalCH) / 2
	}

	for _, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		
		// Set alignment for cross axis
		if child.Style.Align == "" {
			child.Style.Align = ln.Style.Align // inherit parent alignItems
		}

		cbounds := image.Rect(cx, cy, cx+cw, cy+ch)
		
		if isRow {
			if child.Style.Align == "center" {
				cbounds = image.Rect(cx, cy, cx+cw, cy+innerH)
			}
		} else {
			if child.Style.Align == "center" {
				cbounds = image.Rect(cx, cy, cx+innerW, cy+ch)
			}
		}
		
		childNode := PerformLayout(child, cbounds, inter, rt, scale)
		if childNode != nil {
			group.AddChild(childNode)
		}
		
		if isRow {
			cx += cw + ln.Style.Gap
		} else {
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
