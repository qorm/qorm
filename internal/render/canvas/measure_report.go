package canvas

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/qorm/platform/internal/a11y"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
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
		collectLayoutSnaps(e.layoutRoot, snaps, contrastContext{reason: "no opaque ancestor background"})
	}
	root := e.sceneRoot()
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
			row := measureRowFromGraph(m, n, b, snaps[m.ID], measureSemanticFor(m, e.RT, m == root), scale, opts.Logical, e.RT)
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
		rows = append(rows, measureRowFromSnap(id, sn, measureSemanticFor(sn.node, e.RT, sn.node == root), scale, opts.Logical))
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
	node                               *model.Node
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
	zIndex                             int // 0 = CSS auto
	listVirtWindowed                   bool
	listVirtTotal, listVirtStart, listVirtEnd int
	contrastBG                         color.RGBA
	contrastUnavailable                string
}

type contrastContext struct {
	background   color.RGBA
	reason       string // backdrop uncertainty; an opaque descendant can recover
	effectReason string // subtree effect; remains through every descendant
}

func (c contrastContext) unavailableReason() string {
	if c.effectReason != "" {
		return c.effectReason
	}
	return c.reason
}

func collectLayoutSnaps(ln *LayoutNode, out map[string]measureSnap, inherited contrastContext) {
	if ln == nil || ln.Node == nil {
		return
	}
	ctx := contrastContextFor(ln.Node, ln.Style, inherited)
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
			node: ln.Node, id: id, typ: ln.Node.Type,
			absX: ln.AbsX, absY: ln.AbsY, w: ln.Width, h: ln.Height,
			fs: s.FontSize, fw: s.FontWeight, pad: s.Padding,
			br: int(s.BorderRadius), opacity: op, entranceOp: entOp, animating: anim,
			textAlign: s.TextAlign,
			color:     s.Color, bg: s.Background,
			strokeW: s.StrokeWidth, stroke: s.StrokeColor,
			marginT: s.MarginTop, marginB: s.MarginBot,
			marginL: s.MarginLeft, marginR: s.MarginRight,
			contentW: ln.ContentW, contentH: ln.ContentH,
			zIndex:     s.ZIndex,
			listVirtWindowed: ln.ListVirtWindowed,
			listVirtTotal:    ln.ListVirtTotal,
			listVirtStart:    ln.ListVirtStart,
			listVirtEnd:      ln.ListVirtEnd,
			contrastBG: ctx.background, contrastUnavailable: ctx.unavailableReason(),
		}
	}
	childCtx := ctx
	if isStackType(ln.Node.Type) && len(ln.Children) > 1 {
		// Siblings in a stack can paint beneath one another. Layout ancestry alone
		// cannot identify the pixel behind a descendant, so a solid descendant
		// must re-establish a known background before contrast is available.
		childCtx.reason = "overlapping canvas content"
	}
	for _, c := range ln.Children {
		collectLayoutSnaps(c, out, childCtx)
	}
}

// contrastContextFor resolves the solid colour directly behind a node. It is
// deliberately conservative: a reliable opaque descendant may recover from an
// unknown ancestor, but gradients, raster content and subtree colour effects
// make the final pixel unknowable from layout/style metadata and remain
// unavailable. This is preferable to reporting a plausible-but-false WCAG pass.
func contrastContextFor(n *model.Node, s NodeStyle, inherited contrastContext) contrastContext {
	ctx := inherited
	if len(s.GradientStops) >= 2 {
		ctx.reason = "gradient background"
	} else if s.Background.A == 255 {
		ctx.background = s.Background
		ctx.reason = ""
	} else if s.Background.A > 0 {
		if ctx.reason == "" {
			ctx.background = compositeRGBA(s.Background, ctx.background)
		} else {
			ctx.reason = "translucent background without a known opaque backdrop"
		}
	}

	if n != nil {
		switch strings.ToLower(n.Type) {
		case "image", "photo", "avatar", "video", "webview", "map", "tilemap":
			ctx.reason = "raster or embedded content background"
		}
	}
	if reason := contrastSubtreeEffectUnavailable(s); reason != "" {
		ctx.effectReason = reason
	}
	return ctx
}

func contrastSubtreeEffectUnavailable(s NodeStyle) string {
	if s.BackdropBlur > 0 {
		return "backdrop blur"
	}
	if mode := strings.TrimSpace(strings.ToLower(s.MixBlendMode)); mode != "" && mode != "normal" {
		return "blend mode"
	}
	if s.FilterBlur != 0 || nonIdentityFilter(s.FilterBrightness) || nonIdentityFilter(s.FilterContrast) ||
		nonIdentityFilter(s.FilterSaturate) || s.FilterGrayscale != 0 || s.FilterHueRotate != 0 ||
		s.FilterInvert != 0 || s.FilterSepia != 0 || nonIdentityFilter(s.FilterOpacity) {
		return "colour-altering filter"
	}
	if s.Tint.A > 0 && (s.Tint.R != 255 || s.Tint.G != 255 || s.Tint.B != 255 || s.Tint.A != 255) {
		return "subtree tint"
	}
	if s.Opacity > 0 && s.Opacity < 0.999 {
		return "subtree opacity"
	}
	if s.MaskFade != "" {
		return "soft mask"
	}
	return ""
}

func nonIdentityFilter(v float64) bool {
	return v != 0 && math.Abs(v-1) > 0.0001
}

func compositeRGBA(fg, bg color.RGBA) color.RGBA {
	a := float64(fg.A) / 255
	return color.RGBA{
		R: uint8(math.Round(float64(fg.R)*a + float64(bg.R)*(1-a))),
		G: uint8(math.Round(float64(fg.G)*a + float64(bg.G)*(1-a))),
		B: uint8(math.Round(float64(fg.B)*a + float64(bg.B)*(1-a))),
		A: 255,
	}
}

type measureSemantic struct {
	role           string
	accessibleName string
	ariaLabel      string // explicit ariaLabel only; visible text is not an aria-label
	state          map[string]any
}

// measureSemanticFor derives semantics from the model node that actually
// produced this graph/layout row. Doing it per mounted node avoids an inactive
// `when` branch with the same id overwriting the live branch's role/name/state.
// A shallow copy lets the canonical a11y package map the widget role in O(1)
// without recursively auditing the node's subtree for every measured row.
func measureSemanticFor(n *model.Node, rt *runtime.Runtime, isRoot bool) measureSemantic {
	if n == nil {
		return measureSemantic{}
	}
	shallow := *n
	shallow.Children = nil
	shallow.Template = nil
	shallow.Then = nil
	shallow.Else = nil
	role := ""
	if tree := a11y.Build(&shallow); tree != nil && tree.Root != nil {
		role = tree.Root.Role
	}
	if raw, ok := n.Prop("role"); ok {
		if explicit := semanticString(raw, rt); explicit != "" {
			role = explicit
		}
	}
	if role == "" && isRoot {
		role = "main"
	}
	aria := ""
	if raw, ok := n.Prop("ariaLabel"); ok {
		aria = semanticString(raw, rt)
	}
	return measureSemantic{
		role: role, accessibleName: semanticName(n, aria, rt), ariaLabel: aria,
		state: semanticState(n, role, rt),
	}
}

func semanticName(n *model.Node, explicit string, rt *runtime.Runtime) string {
	if explicit != "" {
		return explicit
	}
	for _, raw := range []any{n.Label, n.Text, n.Placeholder} {
		if s := semanticString(raw, rt); s != "" {
			return s
		}
	}
	for _, key := range []string{"alt", "tooltip", "title", "label"} {
		if raw, ok := n.Prop(key); ok {
			if s := semanticString(raw, rt); s != "" {
				return s
			}
		}
	}
	return ""
}

func semanticState(n *model.Node, role string, rt *runtime.Runtime) map[string]any {
	state := map[string]any{}
	disabled := nodeDisabled(n, rt)
	if raw, ok := n.Prop("disabled"); ok {
		disabled = disabled || semanticTruthy(evalStyleProp(raw, rt))
	}
	state["disabled"] = disabled
	if role == "checkbox" || role == "switch" || role == "radio" {
		checked := false
		if raw, ok := n.Prop("checked"); ok {
			checked = semanticTruthy(evalStyleProp(raw, rt))
		} else if n.Value != "" {
			checked = semanticTruthy(evalStyleProp(n.Value, rt))
		}
		state["checked"] = checked
	}
	if raw, ok := n.Prop("required"); ok {
		state["required"] = semanticTruthy(evalStyleProp(raw, rt))
	}
	if role == "textbox" && semanticSecureInput(n, rt) {
		// Never export a credential through measure/MCP. The protected marker is
		// useful semantic state; the actual value and even its length stay private.
		state["protected"] = true
	} else if role == "textbox" && n.Value != "" {
		state["value"] = semanticString(n.Value, rt)
	}
	return state
}

func semanticSecureInput(n *model.Node, rt *runtime.Runtime) bool {
	if secureInput(n) {
		return true
	}
	for _, key := range []string{"secure", "password"} {
		if raw, ok := n.Prop(key); ok && semanticTruthy(evalStyleProp(raw, rt)) {
			return true
		}
	}
	return false
}

func semanticString(raw any, rt *runtime.Runtime) string {
	if raw == nil {
		return ""
	}
	v := evalStyleProp(raw, rt)
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func semanticTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x != "" && x != "false" && x != "0"
	default:
		return v != nil
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

func measureRowFromGraph(m *model.Node, n graph.Node, b *graph.BaseNode, sn measureSnap, sem measureSemantic, scale int, logical bool, rt *runtime.Runtime) map[string]any {
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
		enrichContrast(row, sn)
	}
	enrichSemantics(row, sem)
	enrichHostLimits(row, m)
	enrichListVirtual(row, sn)
	row["tabindex"] = ""
	return row
}

func enrichListVirtual(row map[string]any, sn measureSnap) {
	if !sn.listVirtWindowed || sn.listVirtTotal <= 0 {
		return
	}
	rendered := sn.listVirtEnd - sn.listVirtStart
	if rendered < 0 {
		rendered = 0
	}
	row["listVirtualization"] = map[string]any{
		"windowed": true,
		"total":    sn.listVirtTotal,
		"start":    sn.listVirtStart,
		"end":      sn.listVirtEnd,
		"rendered": rendered,
	}
}

func measureRowFromSnap(id string, sn measureSnap, sem measureSemantic, scale int, logical bool) map[string]any {
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
	enrichContrast(row, sn)
	enrichSemantics(row, sem)
	enrichHostLimits(row, sn.node)
	enrichListVirtual(row, sn)
	row["tabindex"] = ""
	return row
}

func enrichSemantics(row map[string]any, sem measureSemantic) {
	row["role"] = sem.role
	row["accessibleName"] = sem.accessibleName
	// Keep this field aligned with the DOM getAttribute("aria-label") report:
	// it means an explicit ariaLabel, not a name obtained from visible text.
	row["ariaLabel"] = sem.ariaLabel
	row["semanticState"] = sem.state
	for _, key := range []string{"disabled", "checked", "required", "value", "protected"} {
		if value, ok := sem.state[key]; ok {
			row[key] = value
		}
	}
}

// enrichHostLimits records what this canvas host cannot honour on n, so an
// agent reading measure/check does not treat HTML-path fidelity as proven.
// Keys already in canvasStyleKeys are consumed; the rest are ignored at paint.
// webview is a placeholder on every canvas host that is not -tags canvaswebview.
func enrichHostLimits(row map[string]any, n *model.Node) {
	if n == nil {
		return
	}
	var limits []string
	for k := range n.Style {
		if !canvasStyleKeys[k] {
			limits = append(limits, "style."+k+" ignored")
		}
	}
	switch strings.ToLower(n.Type) {
	case "webview":
		limits = append(limits, "webview placeholder (native overlay needs -tags canvaswebview)")
	}
	if len(limits) == 0 {
		return
	}
	sort.Strings(limits)
	row["hostLimits"] = limits
}

func enrichContrast(row map[string]any, sn measureSnap) {
	if sn.contrastUnavailable != "" {
		row["contrastUnavailable"] = sn.contrastUnavailable
		return
	}
	text, _ := row["text"].(string)
	if strings.TrimSpace(text) == "" {
		row["contrastUnavailable"] = "no rendered text foreground"
		return
	}
	if sn.color.A == 0 {
		row["contrastUnavailable"] = "foreground colour unavailable"
		return
	}
	row["effectiveBackground"] = cssRGBA(sn.contrastBG)
	foreground := sn.color
	if foreground.A < 255 {
		foreground = compositeRGBA(foreground, sn.contrastBG)
	}
	ratio := wcagContrast(foreground, sn.contrastBG)
	row["contrast"] = math.Round(ratio*100) / 100
}

func wcagContrast(a, b color.RGBA) float64 {
	la, lb := wcagLuminance(a), wcagLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func wcagLuminance(c color.RGBA) float64 {
	linear := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
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
	if sn.zIndex != 0 {
		row["zIndex"] = sn.zIndex
	} else {
		row["zIndex"] = "auto"
	}
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
	if semanticSecureInput(n, rt) {
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
