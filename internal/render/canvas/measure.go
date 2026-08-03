package canvas

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	flexlayout "github.com/qorm/qorm/internal/layout"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

type LayoutNode struct {
	Node   *model.Node
	Style  NodeStyle
	Text   string
	Width  int
	Height int
	X      int
	Y      int
	// AbsX/AbsY are the node's absolute physical-px origin, after all
	// ancestor transforms (including scroll offsets) have been applied.
	AbsX        int
	AbsY        int
	NeedsRedraw bool
	Children    []*LayoutNode

	// Input widget overlay (type "input" only, input.go): Placeholder marks
	// Text as the placeholder rather than a value; Editing/Cursor carry the
	// live edit session so PerformLayout can paint the caret, and
	// SelStart/SelEnd the selection so it can paint the highlight.
	Placeholder bool
	Editing     bool
	Cursor      int
	SelStart    int
	SelEnd      int

	// Entrance animation overlay (entrance.go): when EntranceActive, the
	// node's group gets EntranceOpacity multiplied in and (EntranceDX,
	// EntranceDY) added to its position this frame.
	EntranceActive  bool
	EntranceOpacity float64
	EntranceDX      float64
	EntranceDY      float64

	// ItemIndex is the repeat instance this node belongs to: every node
	// measured under one list item carries the item's index, so PerformLayout
	// can stamp interaction flags onto the matching instance only (the model
	// pointer is the shared template). 0 outside a list.
	ItemIndex int
	// ItemScope is set on a repeat instance's ROOT only (the template node's
	// LayoutNode): the item's evaluation scope, recorded into the engine's
	// dispatch sidecar for handler argument evaluation.
	ItemScope map[string]any
	// ContentH is a scroll viewport's full content height before an explicit
	// or fill height clamps the box (scroll nodes only): HandleScroll clamps
	// the offset against exactly this.
	ContentH int
	// ContentW is a scroll viewport's full content width before an explicit or
	// fill width clamps the box (scroll nodes only): the horizontal axis of
	// the same clamp.
	ContentW int

	// EvalVars carries the repeat-instance evaluation scope (item/index/…) to
	// every descendant of the instance — ItemScope stays root-only for the
	// event sidecar, EvalVars is for prop evaluation (widgets' formCtx merge).
	EvalVars map[string]any

	// Wrapped holds a text node's folded lines when the single-line measure
	// exceeded the container's available width (wrap.go). Nil = unwrapped.
	Wrapped []string

	// Retained mode scene graph node backing this layout
	GraphNode graph.Node
}

// Measure does a bottom-up pass to calculate minimum content sizes. scale is
// the device-pixel ratio: design pixels are multiplied by it so the resulting
// geometry is in physical pixels (HiDPI). Pass 1 for logical == physical.
func Measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int) *LayoutNode {
	return measure(n, rt, inter, scale, n, nil)
}

// MeasureScoped is Measure with a list-instance scope: widgets that measure
// their own subtree (card, tabs panels) must use it with the vars the
// registry passes them, or bindings like {{item.label}} evaluate empty
// (the scope never reaches the plain entry).
func MeasureScoped(n *model.Node, rt *runtime.Runtime, inter *Interaction, vars map[string]any, scale int) *LayoutNode {
	return measure(n, rt, inter, scale, n, &listScope{vars: vars})
}

// measure is the recursive body of Measure; root identifies the scene tree
// for the one-shot unsupported-style-key warnings, and sc carries the repeat
// scope when measuring inside a list item (nil outside lists, list.go).
func measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int, root *model.Node, sc *listScope) *LayoutNode {
	if n == nil {
		return nil
	}

	// Conditional rendering, mirroring the HTML path (render.go:377 the
	// node() entry check + render.go:771 when): an `if`/`visible`/`show` prop
	// hides the whole subtree; a `when` node swaps in its Then/Else branch —
	// which may itself be conditional or another `when`, hence the loop.
	for {
		if !nodeVisibleScope(n, rt, sc) {
			return nil
		}
		if n.Type != "when" {
			break
		}
		n = whenBranchScope(n, rt, sc)
		if n == nil {
			return nil
		}
	}

	// JSON components (components.go): an instance node measures its template
	// in a scope carrying the evaluated props, the instance's children
	// filling the template's slots — the HTML renderComponent contract.
	if name := componentRef(rt, n); name != "" {
		depth := 0
		idx := 0
		if sc != nil {
			depth = sc.compDepth
			idx = sc.index
		}
		if depth < maxCompDepth {
			clone, vars := instantiateComponent(n, rt.App.Components[name], name, evalCtxScope(rt, sc), rt)
			return measure(clone, rt, inter, scale, root, &listScope{vars: vars, index: idx, compDepth: depth + 1})
		}
	}

	warnUnsupportedStyleKeys(root, n)

	style := parseStyle(n, rt, sc)
	applyInteractiveOverlay(&style, n, rt, interForInstance(inter, sc))
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
	if sc != nil {
		ln.ItemIndex = sc.index
		// The evaluation scope rides EVERY descendant of the instance (the
		// root-only ItemScope keys the event sidecar and must stay unique):
		// a widget that evaluates its own props (badge label, avatar
		// initials) reads EvalVars to see {{item.x}} — previously those
		// bindings expanded to nil inside repeat templates.
		ln.EvalVars = sc.vars
	}
	// Entrance animation (the `animation` prop): a running entrance must keep
	// frames coming, so it joins NeedsRedraw here and lands on the group in
	// PerformLayout.
	if ep := entranceFor(n, ln.ItemIndex, rt, inter, time.Now()); ep.running {
		ln.NeedsRedraw = true
		ln.EntranceActive = true
		ln.EntranceOpacity, ln.EntranceDX, ln.EntranceDY = ep.opacity, ep.dx, ep.dy
	}

	if n.Type == "text" {
		if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		} else if v, ok := n.Props["value"]; ok {
			ln.Text = evalPropStrScope(v, rt, sc)
		}
	} else if n.Type == "button" {
		// Evaluate bindings in the label too (e.g. "Toggle ({{state.theme}})"),
		// matching text nodes — otherwise the raw template shows literally.
		if t, ok := n.Props["label"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		} else if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		}
	} else if n.Type == "input" {
		ln.Text, ln.Placeholder = inputDisplayText(n, rt, inter)
		if s := editSession(inter, n); s != nil {
			if sc == nil || inter.FocusedItem == sc.index {
				ln.Editing = true
				ln.Cursor = s.Cursor
				ln.SelStart, ln.SelEnd = s.SelStart, s.SelEnd
			} else {
				// The live edit session belongs to a SIBLING repeat instance
				// (the session key is the shared template pointer, input.go):
				// this instance shows the bound value, not the shared buffer.
				if v := evalPropStrScope(n.Value, rt, sc); v != "" {
					ln.Text, ln.Placeholder = v, false
				} else {
					ln.Text, ln.Placeholder = n.Placeholder, true
				}
			}
		}
	}

	if (n.Type == "list" || n.Type == "gridview") && n.Template != nil {
		// Repeat: the template replaces the children (HTML render_data.go:113)
		// — each item instantiates one measured subtree under its item scope.
		// gridview repeats the same way (render_widgets.go gridView); the grid
		// branches below lay the items out in columns.
		for _, cln := range measureListItems(n, rt, inter, scale, root, sc) {
			if cln.NeedsRedraw {
				ln.NeedsRedraw = true
			}
			ln.Children = append(ln.Children, cln)
		}
	} else {
		for _, child := range n.Children {
			if cln := measure(child, rt, inter, scale, root, sc); cln != nil {
				if cln.NeedsRedraw {
					ln.NeedsRedraw = true
				}
				ln.Children = append(ln.Children, cln)
			}
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
	} else if n.Type == "input" {
		// Single-line field: one line of text tall; an empty value keeps a
		// usable default width (browsers size an empty field to ~20 chars).
		contentW = int(MeasureText(ln.Text, float64(fs)))
		if min := minInputWidth * scale; contentW < min {
			contentW = min
		}
		contentH = int(float64(fs) * 1.2)
	} else if n.Type == "image" {
		// Intrinsic size (scaled); an explicit style width/height overrides
		// via the generic sizing below, and RecordImage gets the resolved box.
		contentW, contentH = MeasureImage(n, rt, scale)
	} else if w, ok := LookupWidget(n.Type); ok {
		// A registered widget (built-in library or app-defined custom
		// component): v1 leaf semantics — it measures itself; children flow
		// through the generic layout but do not count toward its size. The
		// list-instance scope (if any) threads through so its subtree
		// bindings evaluate (MeasureScoped).
		var vars map[string]any
		if sc != nil {
			vars = sc.vars
		}
		contentW, contentH = w.Measure(n, rt, vars, scale)
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
	} else if n.Type == "grid" || n.Type == "gridview" {
		// Grid: `columns` equal tracks (1fr in HTML, render_style.go:103) —
		// every column is as wide as the widest child; each row as tall as
		// its tallest child. Gap applies between tracks both ways. A gridview
		// takes its track count from crossAxisCount (see gridColumns).
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

	if isScrollType(n.Type) {
		// A scroll viewport's box may be clamped below its content (that is
		// the point of scrolling): keep the full content size for the offset
		// clamp, before the explicit/fill size applies below.
		ln.ContentH = contentH
		ln.ContentW = contentW
	}

	ln.Width = contentW
	if style.Width > 0 {
		ln.Width = style.Width
	}
	ln.Height = contentH
	if style.Height > 0 {
		ln.Height = style.Height
	}
	// Min/max constraints clamp the resolved box last (CSS order: content and
	// explicit sizes first, then clamp).
	if style.MaxWidth > 0 && ln.Width > style.MaxWidth {
		ln.Width = style.MaxWidth
	}
	if style.MinWidth > 0 && ln.Width < style.MinWidth {
		ln.Width = style.MinWidth
	}
	if style.MaxHeight > 0 && ln.Height > style.MaxHeight {
		ln.Height = style.MaxHeight
	}
	if style.MinHeight > 0 && ln.Height < style.MinHeight {
		ln.Height = style.MinHeight
	}

	return ln
}

func evalPropStr(val any, rt *runtime.Runtime) string {
	return evalPropStrScope(val, rt, nil)
}

func evalPropStrScope(val any, rt *runtime.Runtime, sc *listScope) string {
	if s, ok := val.(string); ok && rt != nil {
		res := runtime.EvalBinding(s, evalCtxScope(rt, sc))
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

// evalCtxScope overlays a repeat scope on evalCtx: inside a list item the
// item/index/first/last keys join state and viewport, the innermost list
// winning collisions (HTML itemScope parity, render_data.go:485). The scope's
// vars were built this frame from this frame's outer context, so they are
// returned as-is.
func evalCtxScope(rt *runtime.Runtime, sc *listScope) map[string]any {
	if sc != nil {
		return sc.vars
	}
	return evalCtx(rt)
}

// evalCtx is the binding scope canvas evaluates node text, style values and
// conditions against: `state`, `viewport` (Layout feeds the live surface
// size into rt.Viewport), `t` (the i18n catalog) and `route` (route
// params) — the same roots the HTML renderer offers (render.go:345), so a
// scene written for the browser reads identically in the native engine.
func evalCtx(rt *runtime.Runtime) map[string]any {
	if rt == nil {
		return map[string]any{}
	}
	ctx := map[string]any{
		"state":    rt.State,
		"viewport": rt.ViewportVars(),
		"route":    rt.RouteParams,
	}
	if rt.App != nil {
		ctx["t"] = rt.Catalog()
	}
	return ctx
}

// nodeVisible evaluates an `if` / `visible` / `show` condition (default
// true) — the same keys, first-match order and truthiness as the HTML
// renderer's visible() (render.go:782).
func nodeVisible(n *model.Node, rt *runtime.Runtime) bool {
	return nodeVisibleScope(n, rt, nil)
}

// nodeVisibleScope is nodeVisible inside a repeat scope: item conditions such
// as `if: "{{item.done}}"` evaluate against the instance's scope.
func nodeVisibleScope(n *model.Node, rt *runtime.Runtime, sc *listScope) bool {
	for _, key := range []string{"if", "visible", "show"} {
		if raw, ok := n.Prop(key); ok {
			return truthy(runtime.EvalBinding(fmt.Sprint(raw), evalCtxScope(rt, sc)))
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
	return whenBranchScope(n, rt, nil)
}

// whenBranchScope is whenBranch inside a repeat scope (list.go).
func whenBranchScope(n *model.Node, rt *runtime.Runtime, sc *listScope) *model.Node {
	branch := n.Else
	if n.Condition != "" && truthy(runtime.EvalBinding(n.Condition, evalCtxScope(rt, sc))) {
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
	// A grid declares `columns`; a gridview declares `crossAxisCount` (the
	// HTML renderer's name for the same track count).
	prop := "columns"
	if n.Type == "gridview" {
		prop = "crossAxisCount"
	}
	if v, ok := n.Prop(prop); ok {
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

// contentW returns a scroll viewport's content box width: the natural content
// width, but at least the viewport itself — a vertical scroll whose children
// stretch to the box still lays them out at the box width.
func contentW(ln *LayoutNode) int {
	if cw := ln.ContentW; cw > ln.Width {
		return cw
	}
	return ln.Width
}

// boardChildVisible reports whether a board child's screen box (its board-space
// cbounds under the live pan/zoom) can reach the viewport. The child's box is
// mapped through the same content matrix the rasterizer applies, with a margin
// sized to a note's drop shadow so an edge-hugging note doesn't pop.
func boardChildVisible(cb image.Rectangle, inter *Interaction, viewport image.Rectangle) bool {
	z := inter.Board.Zoom
	if z <= 0 {
		z = 1
	}
	margin := 24 * z
	sx := inter.Board.PanX + float64(cb.Min.X)*z - margin
	sy := inter.Board.PanY + float64(cb.Min.Y)*z - margin
	ex := inter.Board.PanX + float64(cb.Max.X)*z + margin
	ey := inter.Board.PanY + float64(cb.Max.Y)*z + margin
	return ex >= 0 && ey >= 0 && sx <= float64(viewport.Dx()) && sy <= float64(viewport.Dy())
}

// PerformLayout does the top-down pass, building the scene graph. inter and
// rt stamp interaction state and resolve theme-driven decorations; scale is
// the device-pixel ratio (used for the focus-ring insets so its visual width
// stays constant in physical pixels).
func PerformLayout(ln *LayoutNode, bounds image.Rectangle, inter *Interaction, rt *runtime.Runtime, scale int) graph.Node {
	return performLayout(ln, bounds, image.Point{}, inter, rt, scale, nil, nil)
}

// PerformLayoutWithSinks is PerformLayout plus the frame's side channels: a
// container widget that lays out children itself (ChildLayoutWidget) forwards
// the sinks it was handed so nested widgets' overlays and repeat-item
// identities keep flowing to the frame. absOrigin is the container's own
// absolute scene position (its ln.AbsX/AbsY): the delegated subtree's
// AbsX/AbsY accumulate from it, so an OverlayWidget nested inside positions
// its popup in SCENE coordinates, not container-relative ones. sinks may be
// nil (== PerformLayout).
func PerformLayoutWithSinks(ln *LayoutNode, bounds image.Rectangle, absOrigin image.Point, inter *Interaction, rt *runtime.Runtime, scale int, sinks *LayoutSinks) graph.Node {
	if sinks == nil {
		return performLayout(ln, bounds, absOrigin, inter, rt, scale, nil, nil)
	}
	return performLayout(ln, bounds, absOrigin, inter, rt, scale, sinks.items, sinks.overlays)
}

// performLayout is the recursive body of PerformLayout; items, when non-nil,
// collects the repeat-instance sidecar (list.go) the engine uses at event
// time, and overlays collects any popup nodes to append after the normal
// tree — Layout allocates both, layout-only callers pass nil.
func performLayout(ln *LayoutNode, bounds image.Rectangle, absOrigin image.Point, inter *Interaction, rt *runtime.Runtime, scale int, items map[graph.Node]itemInstance, overlays *[]graph.Node) graph.Node {
	if ln == nil {
		return nil
	}

	if ln.Style.WidthRaw == "fill" {
		ln.Width = bounds.Dx() - ln.Style.MarginLeft - ln.Style.MarginRight
	}
	// Auto width never grows past the container's available width: a block
	// box takes the available width and its content overflows INSIDE (CSS).
	// Without this clamp one over-wide child (an unwrapped long text)
	// balloons every ancestor and, through flex stretch, every sibling —
	// netdemo's hint texts pushed the body column to 3702px and the buttons'
	// centred labels off-screen with it.
	if ln.Style.WidthRaw == "" && ln.Style.Width <= 0 {
		if avail := bounds.Dx() - ln.Style.MarginLeft - ln.Style.MarginRight; avail >= 0 && ln.Width > avail {
			ln.Width = avail
		}
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
	ln.AbsX = absOrigin.X + x
	ln.AbsY = absOrigin.Y + y

	group := graph.NewGroup()
	group.X = float64(x)
	group.Y = float64(y)
	group.Width = float64(ln.Width)
	group.Height = float64(ln.Height)
	group.Model = ln.Node
	ln.GraphNode = group
	if ln.EntranceActive {
		group.Opacity *= ln.EntranceOpacity
		group.X += ln.EntranceDX
		group.Y += ln.EntranceDY
		ln.AbsX += int(math.Round(ln.EntranceDX))
		ln.AbsY += int(math.Round(ln.EntranceDY))
	}
	if inter != nil {
		// Repeat instances share the template's model pointer, so a flag
		// lands only when the identity's companion index matches the instance
		// (list.go); outside lists both sides are 0 and this is the plain
		// pointer comparison it always was.
		group.Pressed = inter.Pressed == ln.Node && inter.PressedItem == ln.ItemIndex
		group.Hovered = inter.Hovered == ln.Node && inter.HoveredItem == ln.ItemIndex
		group.Focused = inter.Focused == ln.Node && inter.FocusedItem == ln.ItemIndex
	}
	if items != nil && ln.ItemScope != nil {
		// Repeat instance root: record the dispatch sidecar (index for
		// identity, vars for handler argument evaluation).
		items[group] = itemInstance{index: ln.ItemIndex, vars: ln.ItemScope}
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

	isScroll := isScrollType(ln.Node.Type)
	if isScroll {
		// Viewport: clip the descendants to the box — clipNode paints the
		// ClipOp as the first child (the group's own Save/Restore pops it),
		// and Clip makes graph.HitTest refuse the very pixels that got cut
		// (scroll.go).
		group.Clip = true
		group.AddChild(newClipNode(float64(ln.Width), float64(ln.Height)))
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

	if ln.Node.Type == "input" {
		// Inputs draw their own text block (left-aligned, placeholder color,
		// caret overlay) — the generic block below centers button labels.
		layoutInput(ln, group, rt, scale)
	} else if ln.Node.Type == "image" {
		// Images mount their own shape (fit-computed dest rect, clip/opacity
		// handled by the rasterizer); a broken src records a placeholder box.
		if im := RecordImage(ln.Node, rt, ln.Width, ln.Height, ln.Style.BorderRadius); im != nil {
			group.AddChild(im)
		}
	} else if w, ok := LookupWidget(ln.Node.Type); ok {
		// A registered widget mounts the shape it built (see Widget.Record).
		// A container that lays out children itself gets the frame's sinks so
		// overlays and repeat identities nested in its panel keep flowing.
		var shape graph.Node
		if cw, yes := w.(ChildLayoutWidget); yes {
			shape = cw.RecordWithSinks(ln, rt, scale, &LayoutSinks{items: items, overlays: overlays})
		} else {
			shape = w.Record(ln, rt, scale)
		}
		if shape != nil {
			group.AddChild(shape)
		}
		if overlays != nil {
			if ow, ok := w.(OverlayWidget); ok && ow.OverlayOpen(ln.Node, rt) {
				if overlay := ow.OverlayRecord(ln, rt, scale, image.Pt(ln.AbsX, ln.AbsY)); overlay != nil {
					if items != nil && (ln.ItemIndex != 0 || ln.ItemScope != nil) {
						items[overlay] = itemInstance{index: ln.ItemIndex, vars: ln.ItemScope}
					}
					*overlays = append(*overlays, overlay)
				}
			}
		}
	} else if ln.Text != "" {
		fs := ln.Style.FontSize
		if fs == 0 {
			fs = 14
		}

		txtH := textLineH(fs)
		c := ln.Style.Color
		if c.A == 0 {
			c = color.RGBA{255, 255, 255, 255}
		}

		if len(ln.Wrapped) > 0 {
			// Folded block text (wrap.go): one graph text per line, all
			// left-aligned at the box origin — a wrapped paragraph has no
			// centre alignment in v1.
			for i, line := range ln.Wrapped {
				textNode := graph.NewText()
				textNode.X = 0
				textNode.Y = float64(i * txtH)
				textNode.Content = line
				textNode.Fill = c
				textNode.FontSize = float64(fs)
				textNode.FontWeight = ln.Style.FontWeight
				group.AddChild(textNode)
			}
		} else {
			txtW := int(MeasureText(ln.Text, float64(fs)))
			tx := 0
			if ln.Style.TextAlign == "center" || ln.Node.Type == "button" {
				tx = (ln.Width - txtW) / 2
			}
			ty := (ln.Height - txtH) / 2
			textNode := graph.NewText()
			textNode.X = float64(tx)
			textNode.Y = float64(ty)
			textNode.Content = ln.Text
			textNode.Fill = c
			textNode.FontSize = float64(fs)
			textNode.FontWeight = ln.Style.FontWeight
			group.AddChild(textNode)
		}
	}

	cx := ln.Style.Padding
	cy := ln.Style.Padding
	isRow := ln.Node.Type == "row"
	isStack := isStackType(ln.Node.Type)
	isGrid := ln.Node.Type == "grid" || ln.Node.Type == "gridview"

	// A scroll viewport's children mount into one content group so the whole
	// scrolled body shifts as a unit (its X/Y translations are the offsets). It
	// is the viewport's ONLY group child — scrollContentOf relies on that. The
	// content box is the NATURAL content size, so children wider than the
	// viewport overflow and scroll horizontally.
	sink := group
	var content *graph.Group
	if isScroll {
		content = graph.NewGroup()
		content.Width = float64(contentW(ln))
		content.Height = float64(ln.ContentH)
		pos := scrollOffsetPos(ln, inter)
		content.X = -math.Round(pos.X)
		content.Y = -math.Round(pos.Y)
		sink = content
	}

	// An infinite-canvas board is a fixed window-sized frame whose CONTENT
	// mounts into one transformed group: pan/zoom live on that group's matrix
	// (the graph layer folds it into GlobalTransform for hit testing; the
	// rasterizer applies the same matrix), while the board's own background
	// stays fixed in screen space.
	isBoard := ln.Node.Type == "board"
	var boardContent *graph.Group
	if isBoard && inter != nil && inter.Board.Active {
		boardContent = graph.NewGroup()
		boardContent.X = inter.Board.PanX
		boardContent.Y = inter.Board.PanY
		boardContent.ScaleX = inter.Board.Zoom
		boardContent.ScaleY = inter.Board.Zoom
		sink = boardContent
	}

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
	if isScroll {
		// Scroll content lays out at its NATURAL width, so a child wider than
		// the viewport overflows (and scrolls horizontally) instead of being
		// shrunk to fit.
		innerW = contentW(ln) - ln.Style.Padding*2
	}
	innerH := ln.Height - ln.Style.Padding*2

	// Justify is the flex kernel's job (flexStyle forwards it; the kernel
	// supports all six values, this hand loop only knew center). Applying it
	// here too double-offsets every child (the form example's centred form
	// landed one justify-height too low). cx/cy stay at the padding corner.

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

	// CSS flexbox (internal/layout kernel): computed once per container, then
	// each default-case child takes its resolved box — align-items, the six
	// justify values, wrap, and flex-grow all live in the engine now.
	var flexRects []flexlayout.Rect
	if !isStack && !isGrid && !isBoard && len(ln.Children) > 0 {
		lines := flexlayout.Flex(float64(innerW), float64(innerH), flexStyle(ln, rt), flexChildren(ln, rt))
		for _, line := range lines {
			flexRects = append(flexRects, line.Rects...)
		}
	}

	for i, child := range ln.Children {
		// CSS align-items does NOT inherit into a child's align-self (that is
		// what the removed "inherit parent alignItems" line did — a template
		// row with layout.align:center, meaning center its own children, was
		// also centred in the list instead of stretching full width). A child
		// self-aligns only via its own alignSelf key.

		var cbounds image.Rectangle
		if child.Style.HasPos {
			// Out of flow — absolute position at the content-box origin (an
			// infinite-canvas board's coordinate model): the child neither
			// consumes flex space nor reflows siblings, and its size is its
			// own (content measure or explicit width/height). Works inside any
			// container type, not just a board.
			cbounds = image.Rect(cx+child.Style.PosX, cy+child.Style.PosY,
				cx+child.Style.PosX+child.Width, cy+child.Style.PosY+child.Height)
		} else {
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
				if child.Style.Align == "" {
					// The stack is NOT a flex container: its align/justify ARE
					// each child's positioning props (the flex-row conflation
					// does not apply here).
					child.Style.Align = ln.Style.Align
				}
				cbounds = image.Rect(cx, cy, cx+innerW, cy+innerH)
			case isBoard:
				// A non-positioned board child (rare — the board protocol is
				// absolute x/y) sits at the content-box origin.
				cbounds = image.Rect(cx, cy, cx+child.Width, cy+child.Height)
			case isGrid:
				col, row := i%gridCols, i/gridCols
				gx := cx + col*(gridColW+ln.Style.Gap)
				gy := cy
				for r := 0; r < row; r++ {
					gy += gridRowH[r] + ln.Style.Gap
				}
				// CSS grid items stretch across their auto-sized track. The grid
				// track is already equal-width, but the measured child keeps its
				// intrinsic content width unless we resolve that auto width here;
				// leaving it untouched makes cards shrink to their labels (for
				// example, the three stats cards in the gallery).
				if child.Style.Width == 0 && (child.Style.WidthRaw == "fill" || stretchable(ln, child)) {
					child.Width = gridColW - child.Style.MarginLeft - child.Style.MarginRight
					if child.Width < 0 {
						child.Width = 0
					}
					child.Width = clampInt(child.Width, child.Style.MinWidth, child.Style.MaxWidth)
				}
				cbounds = image.Rect(gx, gy, gx+gridColW, gy+gridRowH[row])
			default:
				r := flexRects[i]
				x0, y0, x1, y1 := flexRectToBounds(r, child, cx, cy)
				cbounds = image.Rect(x0, y0, x1, y1)
				// The flex box is resolved (stretch/grow applied): write it back
				// (clamped) so the group box matches the engine's answer.
				child.Width = clampInt(int(r.W), child.Style.MinWidth, child.Style.MaxWidth)
				child.Height = clampInt(int(r.H), child.Style.MinHeight, child.Style.MaxHeight)
			}
		}

		// Viewport cull (infinite canvas): an off-screen board child builds no
		// subtree and draws nothing — the plane is unbounded, so without this
		// every frame records + rasterizes notes the window can't see. The
		// margin keeps a note whose shadow/ring reaches into the viewport from
		// popping in/out at the edge.
		if isBoard && inter != nil && inter.Board.Active && !boardChildVisible(cbounds, inter, bounds) {
			continue
		}

		childAbsOrigin := image.Pt(ln.AbsX, ln.AbsY)
		if isScroll {
			// The content is shifted by the offsets, so a child's SCENE
			// position is its box position minus the scroll (overlays and
			// absolute-positioned children stay glued to their content).
			pos := scrollOffsetPos(ln, inter)
			childAbsOrigin.X -= int(math.Round(pos.X))
			childAbsOrigin.Y -= int(math.Round(pos.Y))
		}
		childNode := performLayout(child, cbounds, childAbsOrigin, inter, rt, scale, items, overlays)
		if childNode != nil {
			sink.AddChild(childNode)
		}
	}

	if content != nil {
		// The offset translations were set at creation; the clip mounted above
		// cuts whatever leaves the viewport. Scrollbars paint after the content
		// so they sit on top.
		group.AddChild(content)
		if inter != nil {
			addScrollbars(ln, group, scrollOffsetPos(ln, inter), scale)
		}
	}
	if boardContent != nil {
		// The transformed canvas (pan/zoom) mounts LAST so its notes paint —
		// and hit-test — above the fixed background.
		group.AddChild(boardContent)
	}

	// Keyboard focus ring (focus-visible): only drawn when focus was
	// established by the keyboard, offset 3px outside the node body.
	// NoHit keeps the oversized ring from stealing pointer hits.
	if inter != nil && inter.Focused == ln.Node && inter.FocusVisible && inter.FocusedItem == ln.ItemIndex {
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
