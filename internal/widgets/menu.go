package widgets

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("menu", &Menu{open: map[*model.Node]bool{}, geoms: map[*model.Node]*menuGeo{}})
}

// Menu is a dropdown trigger (its label + a chevron) with a floating panel of
// `items` ([{label, onPress {name, args}, disabled}]) plus optional children
// (HTML render_feedback.go menu). Clicking the trigger toggles the panel;
// clicking an item dispatches its onPress and closes; a press outside closes.
// The panel is an overlay, positioned below the trigger.
type Menu struct {
	mu    sync.Mutex
	open  map[*model.Node]bool
	geoms map[*model.Node]*menuGeo
}

// menuGeo carries the last-drawn trigger box and panel rect so HandlePointer
// can map a press to the trigger / an item / outside.
type menuGeo struct {
	trigger image.Rectangle // the trigger button (screen px)
	panel   image.Rectangle // the dropdown panel (screen px)
	rowH    int
	padTop  int
}

const menuRowH = 32

func (m *Menu) isOpen(n *model.Node) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.open[n]
}

func (m *Menu) setOpen(n *model.Node, v bool) {
	m.mu.Lock()
	m.open[n] = v
	m.mu.Unlock()
}

func (m *Menu) geo(n *model.Node) *menuGeo {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.geoms[n]
	if g == nil {
		g = &menuGeo{}
		m.geoms[n] = g
	}
	return g
}

// menuItem is one dropdown row parsed from the `items` prop.
type menuItem struct {
	label    string
	onPress  *model.Invoke
	disabled bool
}

func parseMenuItems(n *model.Node) []menuItem {
	var out []menuItem
	raw, ok := n.Prop("items")
	if !ok {
		return out
	}
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		item := menuItem{}
		if v, ok := m["label"].(string); ok {
			item.label = v
		}
		if v, ok := m["disabled"].(bool); ok {
			item.disabled = v
		}
		if op, ok := m["onPress"].(map[string]any); ok {
			if name, ok := op["name"].(string); ok {
				item.onPress = &model.Invoke{Name: name}
				if args, ok := op["args"].(map[string]any); ok {
					item.onPress.Args = make(map[string]string, len(args))
					for k, v := range args {
						item.onPress.Args[k] = fmt.Sprint(v)
					}
				}
			}
		}
		out = append(out, item)
	}
	return out
}

// Measure reports the trigger's size (its label plus the chevron).
func (*Menu) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	label := formLabel(n, rt)
	if label == "" {
		label = "Menu"
	}
	return int(canvas.MeasureText(label, float64(fs))) + 24*scale, lineHeight(fs) + 16*scale
}

// Record renders the trigger button (the panel is the overlay).
func (m *Menu) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	fs := formFontSizeLN(ln, scale)
	label := formLabel(ln.Node, rt)
	if label == "" {
		label = "Menu"
	}
	g := draw.NewGroup()
	btn := draw.NewRect()
	btn.Width = float64(ln.Width)
	btn.Height = float64(ln.Height)
	btn.BorderRadius = 8 * float64(scale)
	btn.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	btn.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	btn.StrokeWidth = float64(scale)
	g.AddChild(btn)
	g.AddChild(formText(label, float64(12*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))
	g.AddChild(formText("▾", float64(ln.Width-22*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on the trigger
// toggles the panel; a press on an open item dispatches it and closes; a press
// outside closes.
func (m *Menu) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	g := m.geo(n)
	items := parseMenuItems(n)
	if !m.isOpen(n) {
		// Pressed the trigger? Open the panel (record the trigger box; the
		// panel geometry is computed by OverlayRecord on the next frame).
		if p.X >= float64(frame.Min.X) && p.X <= float64(frame.Max.X) &&
			p.Y >= float64(frame.Min.Y) && p.Y <= float64(frame.Max.Y) {
			m.setOpen(n, true)
			g.trigger = frame
			return true
		}
		return false
	}
	// Open: an item click dispatches and closes; anything else closes.
	m.setOpen(n, false)
	if p.X >= float64(g.panel.Min.X) && p.X <= float64(g.panel.Max.X) &&
		p.Y >= float64(g.panel.Min.Y) && p.Y <= float64(g.panel.Max.Y) {
		idx := (int(p.Y) - g.panel.Min.Y - g.padTop) / g.rowH
		if idx >= 0 && idx < len(items) && !items[idx].disabled && items[idx].onPress != nil {
			dispatchInvoke(items[idx].onPress, rt)
		}
	}
	return true
}

// OverlayOpen reports whether the dropdown panel is mounted.
func (m *Menu) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return m.isOpen(n)
}

// OverlayRecord draws the dropdown panel under the trigger.
func (m *Menu) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !m.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	items := parseMenuItems(ln.Node)
	g := m.geo(ln.Node)
	rowH := menuRowH * scale
	panelH := len(items)*rowH + 8*scale
	// The panel is positioned from the CURRENT trigger box (the layout's
	// AbsX/AbsY), not the open-press frame — a scroll or state-driven move
	// while open must keep the panel glued to the trigger.
	panelW := 160 * scale
	panelX := ln.AbsX
	panelY := ln.AbsY + ln.Height + 4*scale
	stageW, stageH := overlayStageSize(rt, scale, panelX+panelW, panelY+panelH)
	if panelY+panelH > stageH && ln.AbsY-4*scale-panelH >= 0 {
		panelY = ln.AbsY - 4*scale - panelH // flip above when it would leave the viewport
	}
	panel := image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)
	g.panel = panel
	g.rowH = rowH
	g.padTop = 4 * scale
	root := draw.NewGroup()
	root.Width = float64(stageW)
	root.Height = float64(stageH)
	root.Model = ln.Node
	root.Overlay = true

	backdrop := draw.NewRect()
	backdrop.Width = float64(stageW)
	backdrop.Height = float64(stageH)
	root.AddChild(backdrop)

	panelRect := draw.NewRect()
	panelRect.X = float64(panel.Min.X)
	panelRect.Y = float64(panel.Min.Y)
	panelRect.Width = float64(panel.Dx())
	panelRect.Height = float64(panel.Dy())
	panelRect.BorderRadius = 8 * float64(scale)
	panelRect.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panelRect.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	panelRect.StrokeWidth = float64(scale)
	panelRect.ShadowColor = color.RGBA{0, 0, 0, 60}
	panelRect.ShadowBlur = 16 * float64(scale)
	panelRect.ShadowY = 6 * float64(scale)
	root.AddChild(panelRect)

	fs := 14 * scale
	ink := formInk(ln.Node, ln, rt)
	rowY := panel.Min.Y + 4*scale
	for _, it := range items {
		c := ink
		if it.disabled {
			c = themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
		}
		if it.label != "" {
			root.AddChild(formText(it.label, float64(panel.Min.X+10*scale),
				float64(rowY)+(float64(rowH)-float64(lineHeight(fs)))/2, fs, c))
		}
		rowY += rowH
	}
	return root
}
