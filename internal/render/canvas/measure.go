package canvas

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	flexlayout "github.com/qorm/platform/internal/layout"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
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
	// CaretVisible is the blink phase for the editing caret (input.go): the
	// engine keeps animating while a session is live, so this flips over time.
	CaretVisible bool
	// MarkedText is the IME composition preview drawn after the value with an
	// underline (input.go), empty when no composition is in flight.
	MarkedText string

	// Entrance animation overlay (entrance.go): when EntranceActive, the
	// node's group gets EntranceOpacity multiplied in, (EntranceDX,
	// EntranceDY) translation, EntranceScale about center, and
	// EntranceRotation (radians) about center this frame.
	EntranceActive   bool
	EntranceOpacity  float64
	EntranceDX       float64
	EntranceDY       float64
	EntranceScale    float64 // 0 or 1 = identity
	EntranceRotation float64 // radians

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

	// List window virtualization (list.go): when virtualize:"window" (or
	// virtualize:true + itemHeight) runs inside a scroll viewport, measure
	// records how many data rows exist vs how many were laid out this frame.
	ListVirtWindowed bool
	ListVirtTotal    int
	ListVirtStart    int
	ListVirtEnd      int // exclusive; rendered rows = End - Start

	// EvalVars carries the repeat-instance evaluation scope (item/index/…) to
	// every descendant of the instance — ItemScope stays root-only for the
	// event sidecar, EvalVars is for prop evaluation (widgets' formCtx merge).
	EvalVars map[string]any

	// UnderBoard is true when this node is a board or lives under one. List
	// items with absolute x/y then frustum-cull against the camera so a
	// side-scroller does not measure/record tiles and sprites the window
	// cannot see.
	UnderBoard bool

	// Wrapped holds a text node's folded lines when the single-line measure
	// exceeded the container's available width (wrap.go). Nil = unwrapped.
	Wrapped []string

	// Retained mode scene graph node backing this layout
	GraphNode graph.Node

	RichTextSpans []graph.Span
}

// Measure does a bottom-up pass to calculate minimum content sizes. scale is
// the device-pixel ratio: design pixels are multiplied by it so the resulting
// geometry is in physical pixels (HiDPI). Pass 1 for logical == physical.
func Measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int) *LayoutNode {
	return measure(n, rt, inter, scale, n, nil, false, nil)
}

// MeasureScoped is Measure with a list-instance scope: widgets that measure
// their own subtree (card, tabs panels) must use it with the vars the
// registry passes them, or bindings like {{item.label}} evaluate empty
// (the scope never reaches the plain entry).
func MeasureScoped(n *model.Node, rt *runtime.Runtime, inter *Interaction, vars map[string]any, scale int) *LayoutNode {
	return measure(n, rt, inter, scale, n, &listScope{vars: vars}, false, nil)
}

// measure is the recursive body of Measure; root identifies the scene tree
// for the one-shot unsupported-style-key warnings, and sc carries the repeat
// scope when measuring inside a list item (nil outside lists, list.go).
func measure(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int, root *model.Node, sc *listScope, underBoard bool, scrollCtx *listScrollCtx) *LayoutNode {
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

	// Bound type ({{state.kind}}) must resolve before component lookup /
	// unknown-type trapping — same contract as HTML render.resolveType.
	n = resolveBoundType(n, rt, sc)

	// Node errorBoundary: try the protected subtree under a trap, fall back
	// on unknown widget / panic (error_boundary.go).
	if n.ErrorBoundary != nil {
		return measureErrorBoundary(n, rt, inter, scale, root, sc, underBoard, scrollCtx)
	}

	// Under a trap, an unrecognised type fails the enclosing boundary instead
	// of degrading to a silent flex container (HTML unknown() + boundaryTrap).
	if trap := scopeTrap(sc); trap != nil && !canvasTypeKnown(rt, n.Type) {
		trap.trip(fmt.Sprintf("unknown widget %q", n.Type))
		return nil
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
			return measure(clone, rt, inter, scale, root, &listScope{vars: vars, index: idx, compDepth: depth + 1, trap: scopeTrap(sc)}, underBoard, scrollCtx)
		}
	}

	warnUnsupportedStyleKeys(root, n)

	style := parseStyle(n, rt, sc)
	applyInteractiveOverlay(&style, n, rt, interForInstance(inter, sc))
	style.scaleBy(scale)
	var needsRedraw bool
	// animatedcontainer is the HTML/camelCase spelling of animated_container.
	if n.Type == "animated_container" || n.Type == "animatedcontainer" {
		style, needsRedraw = UpdateAndGetAnimatedStyle(n.ID, style, rt)
	} else if style.Transition > 0 {
		// A declarative transition animates interaction-effect changes
		// (hover/press background + opacity) instead of snapping them. The key
		// disambiguates repeat instances that share the template ID; the
		// needsRedraw return keeps the engine animating until the tween lands.
		key := n.ID
		if sc != nil {
			key += fmt.Sprintf("@%d", sc.index)
		}
		style, needsRedraw = UpdateAndGetAnimatedStyleD(key, style, rt, style.Transition)
	}

	childBoard := underBoard || n.Type == "board"
	ln := &LayoutNode{
		Node:        n,
		Style:       style,
		NeedsRedraw: needsRedraw,
		UnderBoard:  childBoard,
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
	now := time.Now()
	if ep := entranceFor(n, ln.ItemIndex, rt, inter, now); ep.running {
		ln.NeedsRedraw = true
		ln.EntranceActive = true
		ln.EntranceOpacity, ln.EntranceDX, ln.EntranceDY = ep.opacity, ep.dx, ep.dy
		ln.EntranceScale, ln.EntranceRotation = ep.scale, ep.rotation
	}
	// Game-style feedback FX (`fx` prop) + DOTween Sequence (`timeline`):
	// composed onto the same transform channels as entrance.
	composeMotion := func(dx, dy, rot, scale, opacity float64, keepAlive bool) {
		if keepAlive {
			ln.NeedsRedraw = true
		}
		if !ln.EntranceActive {
			ln.EntranceActive = true
			ln.EntranceOpacity = 1
			ln.EntranceScale = 1
		}
		ln.EntranceDX += dx
		ln.EntranceDY += dy
		ln.EntranceRotation += rot
		if scale > 0 && scale != 1 {
			if ln.EntranceScale <= 0 {
				ln.EntranceScale = 1
			}
			ln.EntranceScale *= scale
		}
		if opacity >= 0 && opacity < 1 {
			ln.EntranceOpacity *= opacity
		}
	}
	if fp := fxFor(n, ln.ItemIndex, rt, inter, now); fp.running {
		composeMotion(fp.dx, fp.dy, fp.rotation, fp.scale, fp.opacity, true)
	}
	if tp := timelineFor(n, ln.ItemIndex, rt, inter, now); tp.active {
		// Hold end pose after finish (DOTween default); only keepAlive while running.
		composeMotion(tp.dx, tp.dy, tp.rotation, tp.scale, tp.opacity, tp.running)
	}

	if n.Type == "text" {
		if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		} else if v, ok := n.Props["value"]; ok {
			ln.Text = evalPropStrScope(v, rt, sc)
		}
		ln.Text = applyTextTransform(ln.Text, style.TextTransform)
	} else if n.Type == "button" {
		// Evaluate bindings in the label too (e.g. "Toggle ({{state.theme}})"),
		// matching text nodes — otherwise the raw template shows literally.
		if t, ok := n.Props["label"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		} else if t, ok := n.Props["text"]; ok {
			ln.Text = evalPropStrScope(t, rt, sc)
		}
		ln.Text = applyTextTransform(ln.Text, style.TextTransform)
	} else if n.Type == "richtext" {
		if spansRaw, ok := n.Prop("spans"); ok {
			if spansList, ok := spansRaw.([]any); ok {
				for _, spRaw := range spansList {
					if spMap, ok := spRaw.(map[string]any); ok {
						span := graph.Span{}
						if text, ok := spMap["text"]; ok {
							span.Content = evalPropStrScope(text, rt, sc)
						} else if content, ok := spMap["content"]; ok {
							span.Content = evalPropStrScope(content, rt, sc)
						}

						fs2 := style.FontSize
						if fs2 == 0 {
							fs2 = 14
						}
						span.FontSize = float64(fs2)
						if v, ok := spMap["fontSize"].(float64); ok {
							span.FontSize = v
						} else if v, ok := spMap["fontSize"].(int); ok {
							span.FontSize = float64(v)
						}

						span.Fill = style.Color
						if c, ok := spMap["color"].(string); ok {
							span.Fill = parseColor(c)
						}

						span.FontWeight = style.FontWeight
						if fw, ok := spMap["fontWeight"].(float64); ok {
							span.FontWeight = int(fw)
						} else if fw, ok := spMap["fontWeight"].(int); ok {
							span.FontWeight = fw
						}

						span.LetterSpacing = style.LetterSpacing
						if ls, ok := spMap["letterSpacing"].(float64); ok {
							span.LetterSpacing = ls
						}

						span.StrokeColor = style.TextStrokeColor
						if c, ok := spMap["textStrokeColor"].(string); ok {
							span.StrokeColor = parseColor(c)
						}
						span.StrokeWidth = style.TextStrokeWidth
						if w, ok := spMap["textStrokeWidth"].(float64); ok {
							span.StrokeWidth = w
						} else if w, ok := spMap["textStrokeWidth"].(int); ok {
							span.StrokeWidth = float64(w)
						}

						span.ShadowColor = style.TextShadowColor
						if c, ok := spMap["textShadowColor"].(string); ok {
							span.ShadowColor = parseColor(c)
						}
						span.ShadowBlur = style.TextShadowBlur
						if b, ok := spMap["textShadowBlur"].(float64); ok {
							span.ShadowBlur = b
						} else if b, ok := spMap["textShadowBlur"].(int); ok {
							span.ShadowBlur = float64(b)
						}
						span.ShadowX = style.TextShadowX
						if x, ok := spMap["textShadowX"].(float64); ok {
							span.ShadowX = x
						} else if x, ok := spMap["textShadowX"].(int); ok {
							span.ShadowX = float64(x)
						}
						span.ShadowY = style.TextShadowY
						if y, ok := spMap["textShadowY"].(float64); ok {
							span.ShadowY = y
						} else if y, ok := spMap["textShadowY"].(int); ok {
							span.ShadowY = float64(y)
						}

						ln.RichTextSpans = append(ln.RichTextSpans, span)
					}
				}
			}
		}
	} else if n.Type == "input" {
		ln.Text, ln.Placeholder = inputDisplayText(n, rt, inter)
		if s := editSession(inter, n); s != nil {
			if sc == nil || inter.FocusedItem == sc.index {
				ln.Editing = true
				ln.Cursor = s.Cursor
				ln.SelStart, ln.SelEnd = s.SelStart, s.SelEnd
				ln.CaretVisible = caretVisible(s, time.Now())
				ln.MarkedText = string(s.MarkedText)
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
		kids, virt := measureListItems(n, rt, inter, scale, root, sc, childBoard, scrollCtx)
		if virt.Windowed {
			ln.ListVirtWindowed = true
			ln.ListVirtTotal = virt.Total
			ln.ListVirtStart = virt.Start
			ln.ListVirtEnd = virt.End
		}
		for _, cln := range kids {
			if cln.NeedsRedraw {
				ln.NeedsRedraw = true
			}
			ln.Children = append(ln.Children, cln)
		}
	} else {
		childScroll := scrollCtx
		if isScrollType(n.Type) {
			port := float64(style.Height - 2*style.Padding)
			if port < 0 {
				port = 0
			}
			top := 0.0
			if inter != nil && inter.ScrollOffsets != nil {
				top = inter.ScrollOffsets[n].Y
			}
			childScroll = &listScrollCtx{scrollNode: n, portH: port, scrollTop: top}
		}
		for _, child := range n.Children {
			if cln := measure(child, rt, inter, scale, root, sc, childBoard, childScroll); cln != nil {
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
		contentW = int(MeasureTextTracking(ln.Text, float64(fs), style.LetterSpacing))
		contentH = textLineHM(fs, style.LineHeight)
	} else if n.Type == "richtext" {
		for _, span := range ln.RichTextSpans {
			w := int(MeasureTextTracking(span.Content, span.FontSize, span.LetterSpacing))
			contentW += w
			h := textLineHM(int(span.FontSize), style.LineHeight)
			if h > contentH {
				contentH = h
			}
		}
	} else if n.Type == "button" {
		contentW = int(MeasureTextTracking(ln.Text, float64(fs), style.LetterSpacing)) + 40*scale
		contentH = textLineHM(fs, style.LineHeight) + 20*scale
	} else if n.Type == "input" {
		// Single-line field: one line of text tall; an empty value keeps a
		// usable default width (browsers size an empty field to ~20 chars).
		contentW = int(MeasureTextTracking(ln.Text, float64(fs), style.LetterSpacing))
		if min := minInputWidth * scale; contentW < min {
			contentW = min
		}
		contentH = textLineHM(fs, style.LineHeight)
	} else if n.Type == "image" {
		// Intrinsic size (scaled); an explicit style width/height overrides
		// via the generic sizing below, and RecordImage gets the resolved box.
		contentW, contentH = MeasureImage(n, rt, scale, evalCtxScope(rt, sc))
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
	} else if (n.Type == "list" || n.Type == "gridview") && listItemsAllAbs(ln.Children) {
		// Board tile/sprite lists are absolutely positioned: do not stack
		// them as a column (N×tileH tall). Size is the union of children.
		for _, child := range ln.Children {
			if cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight; cw > contentW {
				contentW = cw
			}
			if ch := child.Height + child.Style.MarginTop + child.Style.MarginBot; ch > contentH {
				contentH = ch
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
	// CSS aspect-ratio supplies the missing axis when exactly one dimension is
	// authored. Both explicit axes remain authoritative; with neither explicit,
	// intrinsic content remains authoritative.
	if style.AspectRatio > 0 {
		switch {
		case style.Width > 0 && style.Height <= 0:
			ln.Height = int(math.Round(float64(ln.Width) / style.AspectRatio))
			ln.Style.Height = ln.Height // derived axis is definite for flex
		case style.Height > 0 && style.Width <= 0:
			ln.Width = int(math.Round(float64(ln.Height) * style.AspectRatio))
			ln.Style.Width = ln.Width // derived axis is definite for flex
		}
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

// evalPropStrWithVars evaluates a binding with an explicit vars overlay (the
// repeat-instance scope: item/index/…). Used by imageSrc when the image sits
// inside a gridview/list renderItem template.
func evalPropStrWithVars(val any, rt *runtime.Runtime, vars map[string]any) string {
	if s, ok := val.(string); ok && rt != nil {
		ctx := evalCtx(rt)
		for k, v := range vars {
			ctx[k] = v
		}
		res := runtime.EvalBinding(s, ctx)
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
	// A scope that only carries an errorBoundary trap (vars nil) must still
	// see live state/viewport — otherwise {{state.x}} type bindings resolve
	// to nil and trip the boundary as unknown widget "<nil>".
	if sc != nil && sc.vars != nil {
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
	if n.Template != nil {
		// A focused list item lives in the renderItem template; without this,
		// Enter/Space activation (which re-checks nodeMounted) would always
		// refuse a template-focused node even with live instances.
		return nodeMounted(n.Template, target, rt)
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
// and paint by zIndex then declaration order (later equal-z siblings on top).
func isStackType(t string) bool { return t == "stack" || t == "absolute" }

// gridColumns reads a grid's column count from the `columns` prop (HTML:
// propNum(n, "columns", 2), render_style.go:104), clamped to [1, maxGridColumns].
// The upper clamp is load-bearing, not cosmetic: a huge JSON float (1e19)
// converts to int platform-dependently (on 32-bit and on 64-bit platforms
// whose int conversion wraps for out-of-range floats, int(1e19) can come out
// NEGATIVE — which the < 1 clamp then maps to 1 instead of maxGridColumns),
// and len(children)+cols-1 would then overflow in the row-count math below.
const maxGridColumns = 4096

// clampGridCols normalises any prop value (float64 / int) into the legal
// [1, maxGridColumns] range. Doing the float-side clamp BEFORE the int
// conversion is what stops int(1e19)'s wraparound from sneaking through the
// bottom clamp as 1.
func clampGridCols(c any) int {
	switch v := c.(type) {
	case float64:
		if math.IsNaN(v) || v < 1 {
			return 1
		}
		if v > float64(maxGridColumns) {
			return maxGridColumns
		}
		return int(v)
	case int:
		if v < 1 {
			return 1
		}
		if v > maxGridColumns {
			return maxGridColumns
		}
		return v
	}
	return 0 // caller will default to 1 via the lower clamp below
}

func gridColumns(n *model.Node) int {
	cols := 2
	// A grid declares `columns`; a gridview declares `crossAxisCount` (the
	// HTML renderer's name for the same track count).
	prop := "columns"
	if n.Type == "gridview" {
		prop = "crossAxisCount"
	}
	if v, ok := n.Prop(prop); ok {
		cols = clampGridCols(v)
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

	// A board root with a `cameraTarget` prop has its pan rewritten every
	// frame so the target stays at the configured screen position. Runs at
	// the top of performLayout so the board content group (line 891) reads
	// the freshly-set PanX/PanY and not the previous frame's. Side-scroller
	// games (mario, metroid, sonic) declare this once; the engine does the
	// per-frame follow so the app's qscript stays focused on game logic.
	if ln.Node != nil && ln.Node.Type == "board" {
		applyBoardCamera(ln.Node, rt, inter, bounds.Size(), scale)
		// An infinite-canvas board's frame spans the viewport in BOTH axes.
		// Its children are absolutely positioned (x/y) and out of flow, so
		// they don't contribute to the board's intrinsic size — without
		// this override the measure pass would shrink the board to its
		// child column-height (2 stacked notes → 80px tall on a 400x400
		// surface, see board_test.go). Filling both axes here matches the
		// contract documented in layout.go's board comment and the test
		// assertion in TestBoardViewportSpansAndTransforms.
		if ln.Style.WidthRaw == "" && ln.Style.Width <= 0 {
			ln.Width = bounds.Dx() - ln.Style.MarginLeft - ln.Style.MarginRight
		}
		if ln.Style.HeightRaw == "" && ln.Style.Height <= 0 {
			ln.Height = bounds.Dy() - ln.Style.MarginTop - ln.Style.MarginBot
		}
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
	// Compose entrance + pressed/hover scale into one center-pivoted
	// transform (graph: Scale then Rotate then Translate). Pure translation
	// still updates AbsX/AbsY so hit-testing tracks the painted pixels.
	entScale := 1.0
	entRot := 0.0
	entDX, entDY := 0.0, 0.0
	if ln.EntranceActive {
		group.Opacity *= ln.EntranceOpacity
		entDX, entDY = ln.EntranceDX, ln.EntranceDY
		if ln.EntranceScale > 0 {
			entScale = ln.EntranceScale
		}
		entRot = ln.EntranceRotation
		ln.AbsX += int(math.Round(entDX))
		ln.AbsY += int(math.Round(entDY))
	}
	pressScale := 1.0
	if inter != nil {
		// Repeat instances share the template's model pointer, so a flag
		// lands only when the identity's companion index matches the instance
		// (list.go); outside lists both sides are 0 and this is the plain
		// pointer comparison it always was.
		group.Pressed = inter.Pressed == ln.Node && inter.PressedItem == ln.ItemIndex
		group.Hovered = inter.Hovered == ln.Node && inter.HoveredItem == ln.ItemIndex
		group.Focused = inter.Focused == ln.Node && inter.FocusedItem == ln.ItemIndex
		// EffectiveScale is resolved by applyInteractiveOverlay and may carry
		// an in-flight transition tween so `transition` animates pressed scale.
		pressScale = ln.Style.EffectiveScale
		if pressScale <= 0 {
			pressScale = 1
		}
	}
	styleScaleX := 1.0
	if ln.Style.ScaleX != 0 {
		styleScaleX = ln.Style.ScaleX
	} else if ln.Style.Scale != 0 {
		styleScaleX = ln.Style.Scale
	}
	styleScaleY := 1.0
	if ln.Style.ScaleY != 0 {
		styleScaleY = ln.Style.ScaleY
	} else if ln.Style.Scale != 0 {
		styleScaleY = ln.Style.Scale
	}
	if ln.Style.FlipX {
		styleScaleX = -styleScaleX
	}
	if ln.Style.FlipY {
		styleScaleY = -styleScaleY
	}
	totalScaleX := entScale * pressScale * styleScaleX
	totalScaleY := entScale * pressScale * styleScaleY
	totalRot := entRot + ln.Style.Rotate*math.Pi/180
	// Canvas-only: CSS skewX/skewY (degrees) → graph shear (radians).
	skewX := ln.Style.SkewX * math.Pi / 180
	skewY := ln.Style.SkewY * math.Pi / 180
	if totalScaleX != 1 || totalScaleY != 1 || totalRot != 0 || entDX != 0 || entDY != 0 || skewX != 0 || skewY != 0 {
		// Pivot scale+rotation about transform-origin (default center) so
		// pop/spin/rotate match CSS. Independent ScaleX/ScaleY (incl. negative
		// flip) share that pivot. Layout box is unchanged.
		ox, oy := parseTransformOrigin(ln.Style.TransformOrigin, float64(ln.Width), float64(ln.Height))
		sx, sy := totalScaleX, totalScaleY
		cos, sin := math.Cos(totalRot), math.Sin(totalRot)
		scx, scy := sx*ox, sy*oy
		rx := cos*scx - sin*scy
		ry := sin*scx + cos*scy
		group.X = float64(x) + entDX + ox - rx
		group.Y = float64(y) + entDY + oy - ry
		group.ScaleX = sx
		group.ScaleY = sy
		group.Rotation = totalRot
		group.SkewX = skewX
		group.SkewY = skewY
	}
	// FLIP layout motion: when transition + layoutMotion, ease absolute
	// box jumps (shared-element style) instead of snapping.
	if ln.Style.LayoutMotion && ln.Node != nil && ln.Node.ID != "" && ln.Style.Transition > 0 {
		flipKey := ln.Node.ID
		if ln.ItemIndex != 0 {
			flipKey += fmt.Sprintf("@%d", ln.ItemIndex)
		}
		fdx, fdy, fsx, fsy, flipRun := applyLayoutFLIP(flipKey,
			float64(ln.AbsX), float64(ln.AbsY), float64(ln.Width), float64(ln.Height),
			ln.Style.Transition, ln.Style.TransitionEasing)
		if flipRun {
			ln.NeedsRedraw = true
			group.X += fdx
			group.Y += fdy
			if fsx > 0 {
				group.ScaleX *= fsx
			}
			if fsy > 0 {
				group.ScaleY *= fsy
			}
		}
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
	// CSS filter stack → offscreen layer in graph.Group.Draw.
	group.FilterBlur = ln.Style.FilterBlur
	group.FilterBrightness = ln.Style.FilterBrightness
	group.FilterContrast = ln.Style.FilterContrast
	group.FilterSaturate = ln.Style.FilterSaturate
	group.FilterGrayscale = ln.Style.FilterGrayscale
	group.FilterHueRotate = ln.Style.FilterHueRotate
	group.FilterInvert = ln.Style.FilterInvert
	group.FilterSepia = ln.Style.FilterSepia
	group.FilterOpacity = ln.Style.FilterOpacity
	group.Tint = ln.Style.Tint
	group.DropShadowX = ln.Style.DropShadowX
	group.DropShadowY = ln.Style.DropShadowY
	group.DropShadowBlur = ln.Style.DropShadowBlur
	group.DropShadowColor = ln.Style.DropShadowColor
	group.MixBlendMode = ln.Style.MixBlendMode
	if ln.Style.MaskFade != "" {
		group.MaskFade = ln.Style.MaskFade
		group.MaskFadeSize = ln.Style.MaskFadeSize
		if group.MaskFadeSize <= 0 {
			group.MaskFadeSize = 48
		}
	}
	if ln.Style.LayerCache && ln.Node != nil && ln.Node.ID != "" {
		// Static layer cache: key by id + content fingerprint (size + filters
		// + first line of text). Invalidates when any of those change.
		group.LayerCacheKey = ln.Node.ID
		group.LayerCacheFP = layerContentFP(ln)
	}
	// Propagate scroll-snap style keys onto the model Style map so the scroll
	// path can read them without re-parsing (author may only set NodeStyle via
	// cascade). Prefer existing author keys.
	if ln.Style.ScrollSnapType != "" && ln.Node != nil {
		if ln.Node.Style == nil {
			ln.Node.Style = map[string]any{}
		}
		if _, ok := ln.Node.Style["scrollSnapType"]; !ok {
			ln.Node.Style["scrollSnapType"] = ln.Style.ScrollSnapType
		}
	}
	if ln.Style.ScrollSnapAlign != "" && ln.Node != nil {
		if ln.Node.Style == nil {
			ln.Node.Style = map[string]any{}
		}
		if _, ok := ln.Node.Style["scrollSnapAlign"]; !ok {
			ln.Node.Style["scrollSnapAlign"] = ln.Style.ScrollSnapAlign
		}
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
	} else if ln.Style.ClipPath != "" {
		// CSS clip-path (circle / ellipse / inset / polygon) — paint clip + HitTest box.
		group.Clip = true
		if cn := clipNodeFromPath(ln.Style.ClipPath, float64(ln.Width), float64(ln.Height)); cn != nil {
			group.AddChild(cn)
		}
	} else if ln.Style.Overflow == "hidden" || ln.Style.Overflow == "clip" {
		// CSS overflow:hidden — clip children to the box; borderRadius makes
		// a rounded clip so card chrome matches the painted fill.
		group.Clip = true
		group.AddChild(newClipNodeR(float64(ln.Width), float64(ln.Height), ln.Style.BorderRadius))
	}

	hasBg := ln.Style.Background.A > 0 || len(ln.Style.GradientStops) >= 2 || ln.Style.BackdropBlur > 0
	hasStroke := ln.Style.StrokeColor.A > 0 && ln.Style.StrokeWidth > 0
	hasShadow := ln.Style.BoxShadowColor.A > 0
	hasOutline := ln.Style.OutlineColor.A > 0 && ln.Style.OutlineWidth > 0

	if hasBg || hasStroke || hasShadow || hasOutline {
		bg := graph.NewRect()
		bg.X = 0
		bg.Y = 0
		bg.Width = float64(ln.Width)
		bg.Height = float64(ln.Height)
		bg.Fill = ln.Style.Background
		bg.GradientStops = ln.Style.GradientStops
		bg.GradientStopPos = ln.Style.GradientStopPos
		bg.GradientAngle = ln.Style.GradientAngle
		bg.GradientRadial = ln.Style.GradientRadial
		bg.GradientConic = ln.Style.GradientConic
		bg.BackdropBlur = ln.Style.BackdropBlur
		bg.BackdropTint = ln.Style.BackdropTint
		bg.BorderRadius = float64(ln.Style.BorderRadius)
		if ln.Style.OutlineWidth > 0 && ln.Style.OutlineColor.A > 0 {
			bg.OutlineColor = ln.Style.OutlineColor
			bg.OutlineWidth = ln.Style.OutlineWidth
			bg.OutlineOffset = ln.Style.OutlineOffset
		}

		if hasStroke {
			bg.Stroke = ln.Style.StrokeColor
			bg.StrokeWidth = ln.Style.StrokeWidth
			bg.StrokeDasharray = ln.Style.StrokeDasharray
			bg.StrokeDashoffset = ln.Style.StrokeDashoffset
		}

		if hasShadow {
			bg.ShadowColor = ln.Style.BoxShadowColor
			bg.ShadowBlur = float64(ln.Style.BoxShadowBlur)
			bg.ShadowX = float64(ln.Style.BoxShadowX)
			bg.ShadowY = float64(ln.Style.BoxShadowY)
			bg.ShadowInset = ln.Style.BoxShadowInset
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
		if im := RecordImage(ln.Node, rt, ln.Width, ln.Height, ln.Style.BorderRadius, ln.EvalVars); im != nil {
			if gi, ok := im.(*graph.Image); ok && strings.EqualFold(ln.Style.ImageRendering, "pixelated") {
				gi.Pixelated = true
			}
			group.AddChild(im)
		}
	} else if w, ok := LookupWidget(ln.Node.Type); ok {
		// A registered widget mounts the shape it built (see Widget.Record).
		// A container that lays out children itself gets the frame's sinks so
		// overlays and repeat identities nested in its panel keep flowing.
		var shape graph.Node
		if cw, yes := w.(ChildLayoutWidget); yes {
			shape = cw.RecordWithSinks(ln, rt, scale, &LayoutSinks{items: items, overlays: overlays, Inter: inter})
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
	} else if ln.Node.Type == "richtext" && len(ln.RichTextSpans) > 0 {
		rtg := graph.NewRichText()
		rtg.Spans = ln.RichTextSpans
		group.AddChild(rtg)
	} else if ln.Text != "" {
		fs := ln.Style.FontSize
		if fs == 0 {
			fs = 14
		}

		txtH := textLineHM(fs, ln.Style.LineHeight)
		c := ln.Style.Color
		if c.A == 0 {
			c = color.RGBA{255, 255, 255, 255}
		}
		italic := ln.Style.FontStyle == "italic" || ln.Style.FontStyle == "oblique"
		ls := ln.Style.LetterSpacing

		if len(ln.Wrapped) > 0 {
			// Folded block text (wrap.go): one graph text per line, all
			// left-aligned at the box origin — a wrapped paragraph has no
			// centre alignment in v1.
			lines := ln.Wrapped
			// Multi-line ellipsis: lineClamp N, or box height with textOverflow.
			if txtH > 0 {
				maxLines := 0
				if ln.Style.LineClamp > 0 {
					maxLines = ln.Style.LineClamp
				} else if ln.Style.TextOverflow == "ellipsis" && ln.Height > 0 {
					maxLines = ln.Height / txtH
				}
				if maxLines < 1 && ln.Style.LineClamp > 0 {
					maxLines = 1
				}
				if maxLines > 0 && len(lines) > maxLines {
					kept := make([]string, maxLines)
					copy(kept, lines[:maxLines-1])
					// Last line: prefix of the remaining text, ellipsized to width.
					rest := strings.Join(lines[maxLines-1:], "")
					if ln.Width > 0 {
						kept[maxLines-1] = ellipsizeText(rest, float64(fs), ls, ln.Width)
					} else {
						kept[maxLines-1] = ellipsizeText(rest, float64(fs), ls, int(MeasureTextTracking(lines[maxLines-1], float64(fs), ls)))
					}
					lines = kept
				}
			}
			for i, line := range lines {
				textNode := graph.NewText()
				textNode.X = 0
				textNode.Y = float64(i * txtH)
				textNode.Content = line
				textNode.Fill = c
				textNode.FontSize = float64(fs)
				textNode.FontWeight = ln.Style.FontWeight
				textNode.LetterSpacing = ls
				textNode.Italic = italic
				applyTextDecor(textNode, ln.Style)
				group.AddChild(textNode)
			}
		} else {
			content := ln.Text
			// Single-line ellipsis when text overflows the laid-out box.
			if ln.Style.TextOverflow == "ellipsis" && ln.Width > 0 {
				content = ellipsizeText(content, float64(fs), ls, ln.Width)
			}
			txtW := int(MeasureTextTracking(content, float64(fs), ls))
			tx := 0
			if ln.Style.TextAlign == "center" || ln.Node.Type == "button" {
				tx = (ln.Width - txtW) / 2
			}
			ty := (ln.Height - txtH) / 2
			textNode := graph.NewText()
			textNode.X = float64(tx)
			textNode.Y = float64(ty)
			textNode.Content = content
			textNode.Fill = c
			textNode.FontSize = float64(fs)
			textNode.FontWeight = ln.Style.FontWeight
			textNode.LetterSpacing = ls
			textNode.Italic = italic
			applyTextDecor(textNode, ln.Style)
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
	runFlex := !isStack && !isGrid && !isBoard && len(ln.Children) > 0
	if runFlex && listItemsAllAbs(ln.Children) {
		// Absolutely positioned children do not participate in flex. A
		// board tile/sprite list is all HasPos — skip the O(n) flex
		// solve that would otherwise run every frame for hundreds of tiles.
		runFlex = false
	}
	if runFlex {
		lines := flexlayout.Flex(float64(innerW), float64(innerH), flexStyle(ln, rt), flexChildren(ln, rt, innerW, innerH))
		for _, line := range lines {
			flexRects = append(flexRects, line.Rects...)
		}
	}

	// Widget children from this loop only: clip/bg/scrollbars/focus rings
	// stay in the order they were already AddChild'd. Sorted by zIndex
	// (canvas-only; 0 = auto) so lower paints first and Group.HitTest
	// (reverse walk) still matches paint order.
	type zChild struct {
		z int
		n graph.Node
	}
	var zKids []zChild
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
			//
			// PosX/PosY are float64 (sub-pixel, see style.go) — a 60fps
			// physics tick (mario) wants the sprite to land between two
			// pixels so the motion is smooth, not 1-pixel-snapped. We round
			// here because the rest of the layout pipeline (image.Rect,
			// hit testing) is integer — the float survives only as the
			// pre-round value, the pixel is the integer.
			px, py := child.Style.PosX, child.Style.PosY
			if child.Style.HasRight && !child.Style.HasPosX {
				px = float64(innerW-child.Width) - child.Style.PosRight
			}
			if child.Style.HasBottom && !child.Style.HasPosY {
				py = float64(innerH-child.Height) - child.Style.PosBottom
			}
			ix, iy := int(math.Round(px)), int(math.Round(py))
			cbounds = image.Rect(cx+ix, cy+iy, cx+ix+child.Width, cy+iy+child.Height)
		} else {
			switch {
			case isStack:
				// Layered: every child gets the full content box at the same
				// origin. zIndex (then document order) is the paint/hit order
				// — later equal-z siblings paint on top. The child's own
				// align/justify (the stack's, inherited) positions it inside
				// the box. HTML places such children with position+top/left
				// (render_style.go:293), which canvas does not implement —
				// those keys warn as unsupported instead of degrading silently.
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
		//
		// EXCEPTION: a list child of a board is a WRAPPER around its repeat
		// instances — the list's own cbounds is a zero-rect at (0,0) (the
		// board's default-case flex fallback has no flexRects to consume),
		// and the cull uses cbounds + PanX. With PanX non-zero the list's
		// screen box is entirely off-canvas, the cull drops the list, and
		// EVERY instance vanishes — even ones that are right in the middle
		// of the viewport. This is the 2026-08-07 "mario/raiden level
		// disappears once the camera scrolls" regression: the list's
		// children (the actual rendered tiles) are checked individually by
		// their own layout pass, but the parent list was already gone, so
		// they never made it into the graph. Skip the cull for list /
		// gridview children and let the per-instance pass handle it.
		isList := child.Node != nil && (child.Node.Type == "list" || child.Node.Type == "gridview")
		if isBoard && inter != nil && inter.Board.Active && !isList && !boardChildVisible(cbounds, inter, bounds) {
			continue
		}
		// List/stack children under a board are not direct board kids, so
		// the cull above never sees them. Use the surface viewport (not
		// the list's own box) so off-camera tiles and sprites skip record.
		if !isBoard && ln.UnderBoard && child.Style.HasPos && inter != nil && inter.Board.Active && rt != nil {
			screen := image.Rect(0, 0, rt.Viewport.W, rt.Viewport.H)
			if screen.Dx() > 0 && screen.Dy() > 0 && !boardChildVisible(cbounds, inter, screen) {
				continue
			}
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
			zKids = append(zKids, zChild{z: child.Style.ZIndex, n: childNode})
		}
	}
	sort.SliceStable(zKids, func(i, j int) bool { return zKids[i].z < zKids[j].z })
	for _, zc := range zKids {
		sink.AddChild(zc.n)
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
		if ln.Style.HasFocusBorderColor {
			ring.Stroke = ln.Style.FocusBorderColor
		}
		ring.StrokeWidth = 2 * float64(s)
		group.AddChild(ring)
	}

	return group
}

// applyTextDecor copies glyph stroke/shadow/decoration fields from NodeStyle
// onto a graph text node (software paints shadow → stroke → fill → lines).
func applyTextDecor(t *graph.Text, s NodeStyle) {
	t.StrokeColor = s.TextStrokeColor
	t.StrokeWidth = s.TextStrokeWidth
	t.ShadowColor = s.TextShadowColor
	t.ShadowBlur = s.TextShadowBlur
	t.ShadowX = s.TextShadowX
	t.ShadowY = s.TextShadowY
	dec := s.TextDecoration
	t.Underline = strings.Contains(dec, "underline")
	t.LineThrough = strings.Contains(dec, "line-through")
	t.Overline = strings.Contains(dec, "overline")
}

// clipNodeFromPath builds a paint-time clip leaf from CSS clip-path.
func clipNodeFromPath(raw string, w, h float64) *clipNode {
	kind, rx, ry, inset, rad, poly, evenOdd, ok := parseClipPath(raw, w, h)
	if !ok {
		return nil
	}
	switch kind {
	case "ellipse":
		return newClipEllipse(w, h, rx, ry)
	case "inset":
		c := newClipNodeR(float64(inset.Dx()), float64(inset.Dy()), rad)
		c.X = float64(inset.Min.X)
		c.Y = float64(inset.Min.Y)
		return c
	case "polygon":
		return newClipPolygon(w, h, poly, evenOdd)
	default:
		return nil
	}
}

// layerContentFP is a cheap content fingerprint for static layer cache keys.
func layerContentFP(ln *LayoutNode) uint64 {
	if ln == nil {
		return 0
	}
	// FNV-1a over size, style filters, and text.
	h := uint64(14695981039346656037)
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	mix(uint64(ln.Width))
	mix(uint64(ln.Height))
	mix(uint64(ln.Style.FilterBlur * 1000))
	mix(uint64(ln.Style.FilterBrightness * 1000))
	mix(uint64(ln.Style.FilterContrast * 1000))
	mix(uint64(ln.Style.FilterSaturate * 1000))
	mix(uint64(ln.Style.FilterGrayscale * 1000))
	mix(uint64(ln.Style.FilterHueRotate * 1000))
	mix(uint64(ln.Style.FilterInvert * 1000))
	mix(uint64(ln.Style.FilterSepia * 1000))
	mix(uint64(ln.Style.Tint.R)<<24 | uint64(ln.Style.Tint.G)<<16 | uint64(ln.Style.Tint.B)<<8 | uint64(ln.Style.Tint.A))
	mix(uint64(ln.Style.MaskFadeSize * 100))
	for _, r := range ln.Text {
		mix(uint64(r))
	}
	for _, c := range ln.Children {
		if c != nil {
			mix(layerContentFP(c))
		}
	}
	return h
}

// applyTextTransform implements CSS text-transform on a display string.
func applyTextTransform(s, mode string) string {
	switch mode {
	case "uppercase":
		return strings.ToUpper(s)
	case "lowercase":
		return strings.ToLower(s)
	case "capitalize":
		// Title-case first letter of each whitespace-separated word.
		parts := strings.Fields(s)
		for i, p := range parts {
			if p == "" {
				continue
			}
			r := []rune(p)
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
			if len(r) > 1 {
				parts[i] = string(r[0]) + strings.ToLower(string(r[1:]))
			} else {
				parts[i] = string(r[0])
			}
		}
		// Preserve original spacing loosely by rejoining with single spaces.
		return strings.Join(parts, " ")
	default:
		return s
	}
}
