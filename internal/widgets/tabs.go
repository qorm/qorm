package widgets

import (
	"fmt"
	"image"
	"image/color"
	"regexp"
	"strconv"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("tabs", Tabs{})
}

// Tabs is the header-row + active-panel switcher (HTML render_data.go
// tabs()). UNCONTROLLED is the default: the first tab is active and tapping
// a tab switches client-side with no state round-trip — the HTML qormTab
// behaviour, mirrored here by the widget's own per-node active map. An
// `active` prop that is a whole-string {{state.x}} binding makes the tabs
// CONTROLLED: a tap writes the index back to that state path (the HTML radio
// + qorm(-1) data-state sync idiom, here rt.SetStatePath). A literal `active`
// only seeds the uncontrolled default. `onChange` dispatches on every tap
// with {index, tab}, composing with either form (HTML tabHandler).
//
// v1 seam notes: the widget owns panel placement — Record mounts ONLY the
// active panel (canvas.PerformLayout) and drops ln.Children so the generic
// pass does not re-mount every panel on top of the tab bar (the Widget
// contract is leaf-shaped: children would flow at the box origin). The panel
// subtree is laid out with a nil Interaction, so pressed/hovered/focus
// visuals inside a panel do not light up and pointer events inside the panel
// route to this widget (InteractiveWidget owns its subtree's input) which
// ignores them — panel onPress handlers are inert in v1. Handlers and hit
// testing stay wired, so the limitation is visual/dispatch-only.
type Tabs struct{}

// Tab-bar geometry in logical px (× scale): 14px label, 14px horizontal
// padding per tab, 8px vertical padding, a 2px active indicator over the 1px
// separator (the shell's .qorm-tab / .qorm-tab-active rules), and the HTML
// panel's padding:12px 0 top gap.
const (
	tabFontSize   = 14
	tabPadH       = 14
	tabPadV       = 8
	tabIndicatorH = 2
	tabPanelPad   = 12
)

// tabsGeometry is the laid-out tab bar, stashed per node at Record so
// HandlePointer can map a physical click position back to a tab index.
type tabsGeometry struct {
	x, y int   // absolute scene position of the widget's box
	barH int   // tab-bar height, physical px
	tabX []int // each tab's local x start
	tabW []int // each tab's width
}

// tabBindRe matches a whole-string {{state.x}} binding — the same shape the
// HTML boundPath (render_style.go:23) and the canvas input's stateValueBindRe
// recognize.
var tabBindRe = regexp.MustCompile(`^\s*\{\{\s*state\.([a-zA-Z0-9_.]+)\s*\}\}$`)

var (
	tabsMu     sync.Mutex
	tabsActive = map[*model.Node]int{} // uncontrolled active index (qormTab mirror)
	tabsGeom   = map[*model.Node]tabsGeometry{}
)

// Measure reports the bar width (or the active panel's, whichever is wider)
// by the bar height plus the active panel's measured height. The panel is
// measured through the engine's own canvas.Measure; the generic pass measures
// the children again for Record, an accepted double measure (panels are
// small, the pass is allocation-light).
func (Tabs) Measure(n *model.Node, rt *runtime.Runtime, vars map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	labels := tabsLabels(n)
	barW := 0
	for _, lbl := range labels {
		barW += tabWidth(lbl, scale)
	}
	w = barW
	h = tabsBarH(scale)
	if active := tabsActiveIndex(n, nil, rt, tabsCount(n)); active < len(n.Children) {
		if pln := canvas.MeasureScoped(n.Children[active], rt, nil, vars, scale); pln != nil {
			if pln.Width > w {
				w = pln.Width
			}
			h += tabPanelPad*scale + pln.Height
		}
	}
	return w, h
}

// Record builds the tab bar (labels, separator, active indicator) and mounts
// the active panel below it.
func (t Tabs) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return t.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the active panel is
// laid out by the widget itself, so the frame's sinks must flow into that
// PerformLayout call — otherwise a popup widget nested in the panel (a select
// inside a tab) never mounts its overlay.
func (t Tabs) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return t.record(ln, rt, scale, sinks)
}

func (t Tabs) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	n := ln.Node
	labels := tabsLabels(n)
	count := tabsCount(n)
	active := tabsActiveIndex(n, ln, rt, count)
	barH := tabsBarH(scale)
	fs := float64(tabFontSize * scale)
	txtH := int(fs * 1.2)

	accent := themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	inactive := themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})

	g := draw.NewGroup()
	geom := tabsGeometry{x: ln.X, y: ln.Y, barH: barH}

	tx := 0
	for i, lbl := range labels {
		tw := tabWidth(lbl, scale)
		txt := draw.NewText()
		txt.Content = lbl
		txt.FontSize = fs
		if i == active {
			txt.Fill = accent // .qorm-tab-active colours the label too
		} else {
			txt.Fill = inactive
		}
		txt.X = float64(tx + tabPadH*scale)
		txt.Y = float64((barH - tabIndicatorH*scale - txtH) / 2)
		g.AddChild(txt)
		geom.tabX = append(geom.tabX, tx)
		geom.tabW = append(geom.tabW, tw)
		tx += tw
	}

	// The bar's bottom separator spans the full box (border-bottom:1px solid
	// var(--sep)); the active tab's indicator bar (accent, !important in the
	// shell) paints over it.
	sep := draw.NewRect()
	sep.Y = float64(barH - scale)
	sep.Width = float64(ln.Width)
	sep.Height = float64(scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)
	if active < len(geom.tabX) {
		ind := draw.NewRect()
		ind.X = float64(geom.tabX[active])
		ind.Y = float64(barH - tabIndicatorH*scale)
		ind.Width = float64(geom.tabW[active])
		ind.Height = float64(tabIndicatorH * scale)
		ind.Fill = accent
		g.AddChild(ind)
	}

	// Mount ONLY the active panel: the generic pass would flow every child at
	// the box origin, on top of the bar (v1 leaf semantics). ln.Children is
	// rebuilt by Measure every frame, so nil-ing it here is frame-local.
	if active < len(n.Children) {
		for _, cln := range ln.Children {
			if cln.Node != n.Children[active] {
				continue
			}
			// align-items:stretch semantics for the tabs column: an auto-width
			// panel grows to the tabs width (it never shrinks).
			if cln.Width < ln.Width {
				cln.Width = ln.Width
			}
			top := barH + tabPanelPad*scale
			bounds := image.Rect(0, top, ln.Width, top+cln.Height+cln.Style.MarginTop+cln.Style.MarginBot)
			if pn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), sinks.Inter, rt, scale, sinks); pn != nil {
				g.AddChild(pn)
			}
			break
		}
	}
	ln.Children = nil

	tabsMu.Lock()
	tabsGeom[n] = geom
	tabsMu.Unlock()
	return g
}

// HandlePointer maps a press inside the tab bar to a tab switch. Taps outside
// the bar (the panel) are ignored — see the type doc for the v1 input
// limitation.
func (Tabs) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) (redraw bool) {
	if p.Type != canvas.PointerPress {
		return false
	}
	tabsMu.Lock()
	geom, ok := tabsGeom[n]
	tabsMu.Unlock()
	if !ok {
		return false
	}
	lx := int(p.X) - geom.x
	ly := int(p.Y) - geom.y
	if ly < 0 || ly >= geom.barH {
		return false
	}
	idx := -1
	for i := range geom.tabX {
		if lx >= geom.tabX[i] && lx < geom.tabX[i]+geom.tabW[i] {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}

	labels := tabsLabels(n)
	if path := tabsBoundPath(n); path != "" {
		// Controlled: write the index back to the bound state path (the HTML
		// radio's data-state + qorm(-1) sync), which the next frame reads.
		rt.SetStatePath(path, float64(idx))
	} else {
		tabsMu.Lock()
		tabsActive[n] = idx
		tabsMu.Unlock()
	}
	if n.OnChange != nil {
		// HTML tabHandler: author args plus {index, tab} on every tap.
		args := make(map[string]any, len(n.OnChange.Args)+2)
		ctx := map[string]any{"state": rt.State}
		for k, v := range n.OnChange.Args {
			args[k] = runtime.EvalBinding(v, ctx)
		}
		args["index"] = strconv.Itoa(idx)
		if idx < len(labels) {
			args["tab"] = labels[idx]
		}
		rt.Dispatch(n.OnChange.Name, args)
	}
	return true
}

// tabsLabels reads the `tabs` prop as a string list (HTML stringList).
func tabsLabels(n *model.Node) []string {
	arr, ok := n.Props["tabs"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

// tabsCount is the clamp range for the active index: the label count or the
// panel (children) count, whichever is larger (HTML render_data.go:552-555).
func tabsCount(n *model.Node) int {
	count := len(tabsLabels(n))
	if len(n.Children) > count {
		count = len(n.Children)
	}
	return count
}

// tabsBoundPath returns the state path a whole-string-bound `active` prop
// writes back to, or "" — the controlled-tabs test.
func tabsBoundPath(n *model.Node) string {
	raw, ok := n.Prop("active")
	if !ok {
		return ""
	}
	if m := tabBindRe.FindStringSubmatch(fmt.Sprint(raw)); m != nil {
		return m[1]
	}
	return ""
}

// tabsActiveIndex resolves the active tab into [0, count): the bound state
// value for controlled tabs, else the widget's uncontrolled record, else the
// literal `active` default, else 0 — clamped rather than selecting nothing
// (HTML activeIndex, render_data.go:643).
func tabsActiveIndex(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime, count int) int {
	if count <= 0 {
		return 0
	}
	i := 0
	if tabsBoundPath(n) != "" {
		raw, _ := n.Prop("active")
		i = int(asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln))))
	} else {
		tabsMu.Lock()
		v, ok := tabsActive[n]
		tabsMu.Unlock()
		if ok {
			i = v
		} else if raw, has := n.Prop("active"); has {
			// A literal `active` seeds the uncontrolled default (HTML: it
			// renders client-side qormTab switching with that initial tab).
			i = int(asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln))))
		}
	}
	if i < 0 {
		return 0
	}
	if i >= count {
		return count - 1
	}
	return i
}

// tabsBarH is the tab bar's height in physical px: one text line plus
// vertical padding plus the indicator zone.
func tabsBarH(scale int) int {
	return int(float64(tabFontSize*scale)*1.2) + 2*tabPadV*scale + tabIndicatorH*scale
}

// tabWidth is one tab's width in physical px: label text plus horizontal
// padding on both sides.
func tabWidth(label string, scale int) int {
	return int(canvas.MeasureText(label, float64(tabFontSize*scale))) + 2*tabPadH*scale
}
