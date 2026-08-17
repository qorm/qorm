// Package render turns a live scene tree into HTML + CSS. Layout is expressed
// as CSS flexbox/grid and delegated to the browser. It covers a top-tier widget
// vocabulary — containers, scroll, grid, text, button, link, input, textarea,
// select, checkbox, switch, radio, slider, image, avatar, icon, badge, card,
// divider, spacer, progress, spinner, video, tabs and data-bound lists — plus
// conditional rendering (`if`, responsive `when` over viewport.*), onChange
// events and accessibility attributes.
package render

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// nid returns a node's id made unique within the current list item, so widgets
// wired by document.getElementById (dismissible, contextmenu, refresh, long-
// press) work when repeated in a renderItem.
func (r *renderer) nid(n *model.Node) string { return n.ID + r.idSuffix }

// attrID escapes an id for interpolation into a double-quoted HTML id
// attribute. Node ids come from arbitrary scene JSON, and Go's %q renders a
// double quote as \" — the backslash is literal to an HTML parser, so the
// quote still TERMINATES the attribute and an adversarial id injects markup.
// html.EscapeString entity-encodes the quote (and <, >, &); the browser
// decodes entities back to the raw id in element.id, so this is transparent
// to clients — MCP/measure/a11y key on the model id, not the HTML attribute.
// Use at every id-attribute emission site; never inside a <script>, where the
// browser does not decode entities (getElementById wiring uses jsStringID).
func attrID(id string) string { return html.EscapeString(id) }

// jsStringID returns id as a double-quoted JS string literal safe to embed
// verbatim in a literal <script> body. Go's %q quoting is correct for the JS
// parser (quotes, backslash and control characters are all escaped), but the
// HTML parser — not the JS parser — decides where a <script> ends: a literal
// "</script>" (in fact "</" followed by any ASCII letter) terminates the
// element regardless of the JS string context, and "<!--" enters the legacy
// script-escape states. In an agent-native app node ids are author-set, so an
// id like foo</script><script>alert(1)</script> would break out of the inline
// wiring script. Replacing every "<" with the six-character JS escape
// backslash-u-0-0-3-c removes all close-tag sequences while preserving the
// string's value exactly — JS decodes that escape back to "<" at run time,
// matching the entity-decoded element.id that attrID produces, so the
// getElementById lookup still finds the node. Use in place of %q at every
// getElementById site inside a <script>; never for the id attribute itself
// (the browser does not decode JS escapes in attributes — that is attrID's
// job).
func jsStringID(id string) string {
	return strings.ReplaceAll(strconv.Quote(id), "<", "\\u003c")
}

// safeURL validates a URL before it is emitted into a NAVIGATING context (a
// link's href). Allowed: schemeless URLs (path-, fragment- and
// protocol-relative) and absolute URLs whose scheme is on the allowlist —
// http/https/mailto/tel plus the app's own asset scheme. Every other scheme
// (javascript:, data:, vbscript:, file:, …) is replaced with an inert "#"
// fragment, so an author- or state-supplied href can neither execute script
// nor navigate to a phishing payload. The check is case- and
// whitespace-insensitive, mirroring the WHATWG URL parser: it strips leading/
// trailing C0 controls and spaces and removes ASCII tab/newline/CR wherever
// they appear before extracting the scheme — so "  JavaScript:alert(1)" or
// "java\tscript:alert(1)" cannot slip past a naive prefix check while still
// executing in the browser. Not for <img>/<video>/<audio> src: those are
// non-navigating resource contexts where no scheme executes script and where
// data: URLs are a legitimate transport (recorder/camera media).
func safeURL(u string) string {
	var b strings.Builder
	for _, c := range u {
		if c >= 0x20 && c != 0x7f { // drop C0 controls (incl. \t \n \r) and DEL
			b.WriteRune(c)
		}
	}
	s := strings.TrimSpace(b.String())
	// A scheme exists only when ':' precedes every '/', '?' and '#'; anything
	// else is relative (or empty) and passes through untouched.
	if colon := strings.IndexByte(s, ':'); colon > 0 && !strings.ContainsAny(s[:colon], "/?#") {
		switch strings.ToLower(s[:colon]) {
		case "http", "https", "mailto", "tel", "qormapp":
		default:
			return "#"
		}
	}
	return u
}

// Render budget. compDepth (renderInner) caps how DEEPLY component
// instantiation may nest; it does not cap how MUCH is instantiated. A
// self-referencing component whose template holds two instances of itself fans
// out 2^depth nodes while never exceeding the depth cap — the whole tree is
// "shallow" and astronomically large. That render never returns in practice,
// and every caller that renders (POST /event, the SSE catch-up, /poll, an MCP
// patch) does so while holding the server mutex, so one such component wedges
// the entire server, not just one request.
//
// So the render is also charged per node and per output byte. Both limits sit
// far above anything real: the largest example in the repo renders ~100 KB from
// well under a thousand nodes, leaving ~50x/~80x headroom, and the budget is
// per-render (a fresh renderer per RenderScene), so it can never accumulate
// across frames.
const (
	// maxRenderNodes bounds total node() invocations — the fan-out limit, and
	// the one that also catches a bomb whose nodes emit no output at all.
	maxRenderNodes = 50000
	// maxRenderBytes bounds the assembled HTML, so a modest node count that
	// each emit a large payload cannot blow up memory either.
	maxRenderBytes = 8 << 20
)

// budgetMarker is emitted once, in place of the first node that did not fit.
// It is inert markup (no handlers, no script) and carries the diagnostic
// attribute the harness/audit can assert on.
const budgetMarker = `<div data-qorm-truncated="budget"></div>`

// BudgetExceeded is the Result.Unknown entry reported when a render was cut
// short by the budget above. It rides the existing self-verify channel, so
// `qorm check`, the MCP audit and the examples sweep all surface it without a
// new plumbing path.
const BudgetExceeded = "render-budget-exceeded"

// spendNode charges one node against the render budget and reports whether the
// caller may proceed. Once the budget is gone the render DEGRADES: the marker
// is appended once, BudgetExceeded is recorded in Result.Unknown, and every
// subsequent node is skipped — enclosing widgets still write their own closing
// tags, so the emitted HTML stays well-formed and merely truncated. It never
// panics and never errors, matching how an unknown widget type degrades.
func (r *renderer) spendNode() bool {
	if r.overBudget {
		return false
	}
	r.nodesRendered++
	if r.nodesRendered > maxRenderNodes || r.sb.Len() > maxRenderBytes {
		r.overBudget = true
		r.sb.WriteString(budgetMarker)
		r.unknowns = append(r.unknowns, BudgetExceeded)
		return false
	}
	return true
}

// Render renders the entry scene of a runtime.
func Render(rt *runtime.Runtime) Result { return RenderSceneWithOpts(rt, "", RenderOpts{}) }

// RenderWithOpts is the engine-aware Render: pass the live Board state so
// a `type:"board"` root's children are placed in viewport space. Used by
// the server (which owns the canvas engine) and by RenderSubtreeWithOpts.
func RenderWithOpts(rt *runtime.Runtime, opts RenderOpts) Result {
	return RenderSceneWithOpts(rt, "", opts)
}

// RenderScene renders a specific scene by id (empty / unknown falls back to the
// entry scene) — lets a desktop window show a different scene of the same app.
func RenderScene(rt *runtime.Runtime, sceneID string) Result {
	return RenderSceneWithOpts(rt, sceneID, RenderOpts{})
}

// RenderSceneWithOpts is the engine-aware RenderScene: the live Board state
// travels through opts so a board scene's camera pan / zoom is reflected
// in the HTML.
func RenderSceneWithOpts(rt *runtime.Runtime, sceneID string, opts RenderOpts) Result {
	r := &renderer{rt: rt, scope: map[string]any{}, opts: opts}
	root := rt.App.EntryRoot()
	if sceneID != "" {
		if sc := rt.App.Scenes[sceneID]; sc != nil {
			root = sc
		}
	}
	// A blocked runtime has no scene its guards admit — not even the entry one.
	// The unknown-id fallback above would otherwise render exactly the scene the
	// guard refused, so every host (server, WASM, playground) leaks it through
	// this one line. Rendering nothing is the only honest answer; the empty-root
	// branch below already emits the placeholder.
	if rt.Blocked() {
		root = nil
	}
	if root != nil {
		r.rootID = root.ID
		r.rtl = rt.IsRTL()
		r.node(root)
	} else {
		r.sb.WriteString(`<div style="padding:24px;color:#888">no scene to render</div>`)
	}
	html := r.sb.String()
	// Tag the scene root with its id so the client can play a page transition when
	// navigation swaps in a different scene (the morph recreates a changed root).
	key := sceneID
	if key == "" {
		key = "entry"
	}
	if strings.HasPrefix(html, "<") {
		if i := strings.IndexAny(html, " >"); i > 0 {
			// key comes from the sceneID parameter (arbitrary caller/author
			// input), so entity-encode it like every other attribute value.
			html = html[:i] + ` data-scene="` + attrID(key) + `"` + html[i:]
		}
	}
	return Result{HTML: html, Handlers: r.handlers, Unknown: r.unknowns}
}

// RenderSubtree renders a specific node subtree by node ID within the active scene.
// Returns the isolated HTML, event handlers and unknowns for that subtree.
func RenderSubtree(rt *runtime.Runtime, nodeID string) Result {
	return RenderSubtreeWithOpts(rt, nodeID, RenderOpts{})
}

// RenderSubtreeWithOpts is the engine-aware variant of RenderSubtree: the
// server passes in the live Board state (pan / zoom) so a `type:"board"`
// subtree renders its children in viewport space instead of world space.
// Tests and the old "no engine" path stay on RenderSubtree.
func RenderSubtreeWithOpts(rt *runtime.Runtime, nodeID string, opts RenderOpts) Result {
	r := &renderer{rt: rt, scope: map[string]any{}, opts: opts}
	if rt == nil || rt.App == nil {
		return Result{HTML: `<div style="padding:24px;color:#888">no app</div>`}
	}
	root := rt.App.EntryRoot()
	if sceneID := rt.CurrentScene(); sceneID != "" {
		if sc := rt.App.Scenes[sceneID]; sc != nil {
			root = sc
		}
	}
	target := findNodeInTree(root, nodeID)
	if target != nil {
		r.rootID = target.ID
		r.rtl = rt.IsRTL()
		r.node(target)
	} else {
		r.sb.WriteString(`<div style="padding:24px;color:#888">node not found</div>`)
	}
	return Result{HTML: r.sb.String(), Handlers: r.handlers, Unknown: r.unknowns}
}

// RenderNodeDiff renders an isolated node subtree and formats it as a morph template payload for SSE live updates.
func RenderNodeDiff(rt *runtime.Runtime, nodeID string) Result {
	res := RenderSubtree(rt, nodeID)
	res.HTML = fmt.Sprintf(`<template data-morph-target="%s">%s</template>`, html.EscapeString(nodeID), res.HTML)
	return res
}

func findNodeInTree(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if got := findNodeInTree(c, id); got != nil {
			return got
		}
	}
	for _, b := range []*model.Node{n.Then, n.Else, n.Template} {
		if got := findNodeInTree(b, id); got != nil {
			return got
		}
	}
	return nil
}

// renderComponent instantiates an app-defined component: the instance node's
// props/text/label/value become {{prop.x}} inside the template, its children
// fill any {type:slot} (named or default — see slot), and its id suffixes the
// template ids so repeated uses stay unique.
//
// Prop values are evaluated in the INSTANCE's scope before injection: a value
// like "{{state.open}}" or "{{item.name}}" (or an outer "{{prop.x}}" when
// components nest) resolves to its live value at instantiation time, so the
// template's {{prop.x}} bindings, `if` conditions and invoke names see real
// data rather than an unevaluated binding string. Whole-string bindings keep
// their type (EvalBinding's typed path: bool/number/list/object survive as
// such — visible()'s asBool then works on a bool prop with no special case),
// mixed text interpolates to a string, and non-string JSON literals (true,
// 42, [...]) pass through typed and untouched.
//
// A spec-style nested "props":{...} object on the instance is equivalent to
// top-level keys and wins on conflict (planning/spec/json-format-spec.md).
//
// name is the component being instantiated (n.Type, or the resolved `ref` of a
// {"type":"component","ref":…} instance). It is what the declared schema — when
// the component has one — is looked up by: props the instance omits are filled
// from their declared defaults, after everything the instance did supply, so a
// default can never shadow a real value.
func (r *renderer) renderComponent(n *model.Node, comp *model.Node, name string) {
	prevScope, prevKids, prevSuf := r.scope, r.compChildren, r.idSuffix
	ctx := r.ctx() // the instance's own scope — prop values evaluate here
	evalProp := func(v any) any {
		if s, ok := v.(string); ok {
			return runtime.EvalBinding(s, ctx)
		}
		return v
	}
	prop := map[string]any{}
	for k, v := range n.Props {
		if k == "props" {
			continue // the nested props object merges below, not exposed raw
		}
		prop[k] = evalProp(v)
	}
	if n.Text != "" {
		prop["text"] = evalProp(n.Text)
	}
	if n.Label != "" {
		prop["label"] = evalProp(n.Label)
	}
	if n.Value != "" {
		prop["value"] = evalProp(n.Value)
	}
	if pm, ok := n.Props["props"].(map[string]any); ok {
		for k, v := range pm {
			prop[k] = evalProp(v)
		}
	}
	// Declared defaults fill the gaps. A default is a literal from the
	// component definition, so it is injected as-is (no instance-scope
	// evaluation): the definition cannot see the instance's scope.
	if sc := r.rt.App.ComponentSchemas[name]; sc != nil {
		for k, spec := range sc.Props {
			if spec.Default == nil {
				continue
			}
			if _, given := prop[k]; !given {
				prop[k] = spec.Default
			}
		}
	}
	ns := make(map[string]any, len(prevScope)+1)
	for k, v := range prevScope {
		ns[k] = v
	}
	ns["prop"] = prop
	r.scope = ns
	r.compChildren = n.Children
	if n.ID != "" {
		r.idSuffix = prevSuf + "_" + n.ID
	}
	r.compDepth++
	r.node(comp)
	r.compDepth--
	r.scope, r.compChildren, r.idSuffix = prevScope, prevKids, prevSuf
}

// slot renders the component-instance children that belong to this slot. A
// named slot ({"type":"slot","name":"header"} — the spec's slot form) takes
// the instance children declaring slot:"header"; the default (unnamed) slot
// takes the children with no slot attribution — which for an app that never
// names slots is ALL instance children, preserving the original single-slot
// behavior. When no instance child fills the slot, the slot's own children
// render as its fallback (default) content.
func (r *renderer) slot(n *model.Node) {
	name := propStr(n, "name")
	filled := false
	for _, c := range r.compChildren {
		if propStr(c, "slot") == name {
			filled = true
			r.node(c)
		}
	}
	if !filled {
		for _, c := range n.Children {
			r.node(c)
		}
	}
}

func (r *renderer) ctx() map[string]any {
	if r.catalog == nil {
		r.catalog = r.rt.Catalog() // resolve the i18n catalog once per render, not per node
	}
	if r.viewport == nil {
		r.viewport = r.rt.ViewportVars() // constant during a single render too
	}
	if r.breakpoint == nil {
		r.breakpoint = r.rt.BreakpointVars()
	}
	if len(r.scope) == 0 {
		if r.baseCtx == nil { // most nodes have no list scope — share one read-only ctx
			r.baseCtx = map[string]any{"state": r.rt.State, "t": r.catalog, "viewport": r.viewport, "breakpoint": r.breakpoint, "route": r.rt.RouteParams, "computed": r.rt.ComputedVars()}
		}
		return r.baseCtx
	}
	m := map[string]any{"state": r.rt.State, "t": r.catalog, "viewport": r.viewport, "breakpoint": r.breakpoint, "route": r.rt.RouteParams, "computed": r.rt.ComputedVars()}
	for k, v := range r.scope {
		m[k] = v
	}
	return m
}

func (r *renderer) interp(s string) string {
	return runtime.Stringify(runtime.EvalBinding(s, r.ctx()))
}

// node dispatches a node to its renderer, honouring conditional visibility. Any
// node — a built-in widget OR a component instance — carrying an `animation` prop
// is wrapped in that entrance effect, so animation is a cross-cutting property
// rather than something only the `motion` widget offers.
func (r *renderer) node(n *model.Node) {
	if !r.spendNode() {
		return
	}
	if !r.visible(n) {
		return
	}
	// A bound `type` is resolved BEFORE anything dispatches on it (the
	// animation-widget check included), so a dynamically typed node behaves
	// exactly like the static node it resolves to.
	if strings.Contains(n.Type, "{{") {
		n = r.resolveType(n)
	}
	if !animationWidgets[n.Type] {
		if raw := propStr(n, "animation"); raw != "" {
			if effect := r.interp(raw); effect != "" {
				r.wrapAnimation(n, effect)
				return
			}
		}
	}
	r.renderInner(n)
}

// resolveType resolves a node whose `type` is itself a binding —
// {"type":"{{item.kind}}"} — against the CURRENT scope, so one renderItem
// template can render a different widget per item (chat bubble / image / system
// notice) instead of a stack of `when`-gated alternatives that grows with every
// new kind. The loader keeps the binding verbatim (it cannot know the widget
// statically), and this is where it becomes a widget name.
//
// It returns a shallow COPY carrying the resolved type and never mutates the
// scene node: the same template node is re-rendered for every list item, each
// item may resolve to a different widget, and the tree is shared across renders.
//
// The resolved name re-enters the ordinary dispatch in renderInner, so it is
// bound by exactly the same rules as a static type — app components first, then
// the built-in widget switch, then the unknown() fallback — and gains no way to
// bypass the escaping every renderer applies to ids, props and text.
// An expression that fails to evaluate yields "" (EvalBinding's contract) and a
// mixed form may still carry braces; in both cases the raw binding stays as the
// type, matches no case, and the node degrades through unknown() — tagged
// data-qorm-unknown and reported in Result.Unknown — rather than panicking or
// silently vanishing.
func (r *renderer) resolveType(n *model.Node) *model.Node {
	t := strings.TrimSpace(r.interp(n.Type))
	if t == "" || strings.Contains(t, "{{") {
		return n
	}
	cp := *n
	cp.Type = t
	return &cp
}

// renderInner dispatches a node to its renderer (component instantiation or the
// built-in widget switch), after node() has handled visibility and animation.
func (r *renderer) renderInner(n *model.Node) {
	if comp, ok := r.rt.App.Components[n.Type]; ok && comp != nil && r.compDepth < 32 {
		r.renderComponent(n, comp, n.Type)
		return
	}
	// The spec's explicit instance form: {"type":"component","ref":"panel"}
	// (ref may carry the canonical "component://" prefix, and may itself be a
	// binding, resolved against the current scope like any other prop). An
	// unresolvable ref falls through to the generic `component` container
	// below, which is what a bare {"type":"component"} has always rendered as.
	if n.Type == "component" && r.compDepth < 32 {
		if name := model.ComponentRefName(r.interp(propStr(n, "ref"))); name != "" {
			if comp, ok := r.rt.App.Components[name]; ok && comp != nil {
				r.renderComponent(n, comp, name)
				return
			}
		}
	}
	switch n.Type {
	case "slot":
		r.slot(n)
	case "when":
		r.when(n)
	case "timer":
		r.timer(n)
	case "text":
		r.text(n)
	case "button":
		r.button(n)
	case "link":
		r.link(n)
	case "input":
		r.input(n)
	case "textarea":
		r.textarea(n)
	case "select", "dropdown":
		r.selectBox(n)
	case "checkbox", "switch":
		r.checkbox(n)
	case "radio":
		r.radio(n)
	case "slider":
		r.slider(n)
	case "image":
		r.image(n)
	case "avatar":
		r.avatar(n)
	case "icon":
		r.icon(n)
	case "badge":
		r.badge(n)
	case "divider":
		r.divider(n)
	case "verticaldivider":
		r.divider(n)
	case "spacer":
		r.spacer(n)
	case "progress":
		r.progress(n)
	case "spinner":
		r.spinner(n)
	case "activityindicator", "cupertinoactivityindicator":
		r.activityIndicator(n)
	case "picker", "cupertinopicker":
		r.picker(n)
	case "datepicker", "cupertinodatepicker":
		r.datepicker(n)
	case "timepicker", "cupertinotimepicker":
		r.timepicker(n)
	case "monthview", "calendarview", "datepickercalendar":
		r.monthView(n)
	case "tooltip":
		r.tooltip(n)
	case "camera":
		r.camera(n)
	case "location", "geolocation":
		r.location(n)
	case "sensors":
		r.sensors(n)
	case "recorder", "audiorecorder":
		r.recorder(n)
	case "biometric", "faceid", "fingerprint":
		r.biometric(n)
	case "bluetooth":
		r.hwList(n, "bluetooth", "qormBluetooth", "Scan Bluetooth")
	case "wifi":
		r.hwList(n, "wifi", "qormWifi", "Wi-Fi Info")
	case "nfc":
		r.hwList(n, "nfc", "qormNfc", "Read NFC Tag")
	case "volume":
		r.hwAdjust(n, "volume", "qormVol")
	case "brightness":
		r.hwAdjust(n, "brightness", "qormBright")
	case "vibrate":
		r.hwList(n, "vibrate", "qormVibrate", "Vibrate")
	case "torch", "flashlight":
		r.hwList(n, "torch", "qormTorch", "Toggle Flashlight")
	case "battery":
		r.hwList(n, "battery", "qormBattery", "Battery Level")
	case "screenshot", "screencapture":
		r.hwList(n, "screenshot", "qormScreenshot", "Take Screenshot")
	case "screenrecord", "screenrecording":
		r.hwList(n, "screenrecord", "qormScreenRecord", "Start Recording")
	case "share":
		r.hwList(n, "share", "qormShare", "Share")
	case "clipboard":
		r.hwList(n, "clipboard", "qormClipboard", "Copy to Clipboard")
	case "deviceinfo":
		r.hwList(n, "deviceinfo", "qormDeviceInfo", "Device Info")
	case "network":
		r.hwList(n, "network", "qormNetwork", "Network Status")
	case "keepawake", "wakelock":
		r.hwList(n, "keepawake", "qormKeepAwake", "Keep Screen Awake")
	case "haptics":
		r.hwList(n, "haptics", "qormHaptic", "Haptic Feedback")
	case "storage":
		r.hwList(n, "storage", "qormStorage", "Save to Storage")
	case "stt", "speechinput":
		r.hwList(n, "stt", "qormListen", "Start Listening")
	case "securestorage", "keychain":
		r.hwList(n, "securestorage", "qormSecureSave", "Save Securely")
	case "filepicker", "file":
		r.hwList(n, "filepicker", "qormPickFile", "Pick File")
	case "photopicker", "photo":
		r.hwList(n, "photopicker", "qormPickPhoto", "Pick Photo")
	case "videocapture":
		r.hwList(n, "videocapture", "qormRecordVideo", "Record Video")
	case "qrscan", "barcode":
		r.hwList(n, "qrscan", "qormScanQR", "Scan QR")
	case "orientation":
		r.hwList(n, "orientation", "qormOrientation", "Lock Portrait")
	case "tts", "speak":
		r.hwList(n, "tts", "qormSpeak", "Speak")
	case "compass", "heading":
		r.hwList(n, "compass", "qormHeading", "Start Compass")
	case "proximity":
		r.hwList(n, "proximity", "qormProximity", "Start Proximity")
	case "pedometer":
		r.hwList(n, "pedometer", "qormPedometer", "Start Pedometer")
	case "barometer":
		r.hwList(n, "barometer", "qormBarometer", "Start Barometer")
	case "contacts":
		r.hwList(n, "contacts", "qormPickContact", "Pick Contact")
	case "calendar":
		r.hwList(n, "calendar", "qormAddEvent", "Add Event")
	case "systemmodes", "modes":
		r.hwList(n, "systemmodes", "qormGetModes", "Read Modes")
	case "insets", "safearea":
		r.hwList(n, "insets", "qormGetInsets", "Read Safe-Area Insets")
	case "openurl", "openlink":
		r.hwList(n, "openurl", "qormOpenUrl", "Open URL")
	case "notify":
		r.notify(n)
	case "dockbadge":
		r.dockBadge(n)
	case "loginitem", "startatlogin":
		r.loginItem(n)
	case "screens", "displays":
		r.screens(n)
	case "chart":
		r.chart(n)
	case "video":
		r.video(n)
	case "webview":
		r.webview(n)
	case "list":
		r.list(n)
	case "tabs":
		r.tabs(n)
	case "table":
		r.table(n)
	case "datatable":
		r.datatable(n)
	case "modal", "dialog":
		r.modal(n)
	case "alert", "banner":
		r.alert(n)
	case "tag":
		r.tag(n)
	case "skeleton":
		r.skeleton(n)
	case "accordion":
		r.accordion(n)
	case "rating":
		r.rating(n)
	case "steps", "stepper":
		r.steps(n)
	case "breadcrumb":
		r.breadcrumb(n)
	case "pagination":
		r.pagination(n)
	case "menu":
		r.menu(n)
	case "tree":
		r.tree(n)
	case "drawer":
		r.drawer(n)
	case "carousel":
		r.carousel(n)
	case "timeline":
		r.timeline(n)
	case "field", "formfield":
		r.field(n)
	case "stat", "metric":
		r.stat(n)
	case "empty":
		r.empty(n)
	case "segmented", "slidingsegmentedcontrol", "cupertinoslidingsegmentedcontrol":
		r.segmented(n)
	case "swipeactions", "swipeaction":
		r.swipeActions(n)
	case "dismissible":
		r.dismissible(n)
	case "contextmenu", "cupertinocontextmenu":
		r.contextMenu(n)
	case "refreshindicator":
		r.refreshIndicator(n)
	// animated_container is the snake_case spelling used by the canvas backend
	// and shared scene JSON; both spellings render identically.
	case "animatedcontainer", "animated_container", "animatedpadding", "animatedalign", "animatedpositioned":
		r.animatedContainer(n)
	case "animatedopacity":
		r.animatedOpacity(n)
	case "transform", "rotatedbox":
		r.transform(n)
	case "aspectratio":
		r.aspectRatio(n)
	case "richtext":
		r.richText(n)
	case "motion", "animated", "transition", "animatedswitcher",
		"fadetransition", "slidetransition", "scaletransition",
		"rotationtransition", "sizetransition", "hero":
		r.motion(n)
	case "descriptions", "keyvalue":
		r.descriptions(n)
	case "wrap":
		r.wrap(n)
	case "listtile", "listitem":
		r.listTile(n)
	case "listsection", "cupertinolistsection":
		r.listSection(n)
	case "appbar":
		r.appbar(n)
	case "largetitle", "cupertinolargetitle", "sliverappbar":
		r.largeTitle(n)
	case "navigationrail":
		r.navigationRail(n)
	case "selectabletext":
		r.selectableText(n)
	case "backbutton":
		r.backButton(n)
	case "closebutton":
		r.closeButton(n)
	case "form":
		r.form(n)
	case "offstage":
		r.offstage(n)
	case "ignorepointer", "absorbpointer":
		r.ignorePointer(n)
	case "indexedstack":
		r.indexedStack(n)
	case "draggable", "longpressdraggable":
		r.draggable(n)
	case "dragtarget", "droptarget":
		r.dragTarget(n)
	case "navigationdrawer":
		r.navigationDrawer(n)
	case "bottomappbar":
		r.bottomAppBar(n)
	case "limitedbox":
		r.limitedBox(n)
	case "fab", "floatingactionbutton":
		r.fab(n)
	case "scaffold":
		r.scaffold(n)
	case "bottomnav", "bottomnavigationbar", "navigationbar":
		r.bottomNav(n)
	case "snackbar":
		r.snackbar(n)
	case "expansiontile", "expansionpanel":
		r.expansionTile(n)
	case "switchlisttile", "checkboxlisttile", "radiolisttile":
		r.controlTile(n)
	case "chip", "inputchip", "choicechip", "filterchip":
		r.chip(n)
	case "rangeslider":
		r.rangeSlider(n)
	case "alertdialog", "cupertinoalertdialog":
		r.alertDialog(n)
	case "actionsheet", "cupertinoactionsheet":
		r.actionSheet(n)
	case "sheet", "bottomsheet", "draggablesheet", "draggablescrollablesheet", "modalbottomsheet":
		r.sheet(n)
	case "gridview":
		r.gridView(n)
	case "materialstepper":
		r.materialStepper(n)
	case "pageview":
		r.pageView(n)
	case "dropdownbutton":
		r.dropdownButton(n)
	case "gesturedetector", "gesture", "inkwell":
		r.gestureDetector(n)
	case "autocomplete":
		r.autocomplete(n)
	case "searchbar":
		r.searchbar(n)
	case "textformfield":
		r.textFormField(n)
	case "circularprogress", "circularprogressindicator":
		r.circularProgress(n)
	case "tilemap":
		r.tilemap(n)
	case "path":
		r.path(n)
	case "board":
		r.board(n)
	case "row", "column", "columns", "stack", "vstack", "hstack", "zstack", "absolute",
		"scroll", "scrollview", "grid", "card", "component", "flex", "box",
		"div", "container", "group", "view", "fragment", "wrapper", "panel",
		"body", "content", "main", "section", "header", "footer", "aside", "nav",
		"center", "start", "end", "between", "around", "evenly", "stretch":
		r.container(n)
	default:
		r.unknown(n)
	}
}

// unknown renders an UNRECOGNISED widget type as a plain container (so it never
// visibly breaks the app) but tags it with a data-qorm-unknown attribute so the
// self-verify harness + measure/check can flag a real typo (e.g. "colunm")
// programmatically — the north star (the AI catches its own mistakes) without
// disfiguring the UI for a human.
func (r *renderer) unknown(n *model.Node) {
	r.unknowns = append(r.unknowns, n.Type)
	fmt.Fprintf(&r.sb, `<div id=%q data-qorm-unknown=%q style=%q%s>`,
		attrID(n.ID), html.EscapeString(n.Type), r.containerCSS(n), r.a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// when renders a responsive conditional node: its condition — typically over
// viewport.width / viewport.height / viewport.orientation — selects the `then`
// subtree when truthy and the `else` subtree otherwise. Unlike the
// `if`/`visible`/`show` prop (see visible below), which shows or hides ONE
// node in place, `when` swaps between two ALTERNATIVE subtrees. While the
// viewport is unknown (0x0 — e.g. the server's first frame before the client
// reports its size) the condition evaluates falsy and `else` renders.
func (r *renderer) when(n *model.Node) {
	branch := n.Else
	if n.Condition != "" && asBool(runtime.EvalBinding(n.Condition, r.ctx())) {
		branch = n.Then
	}
	if branch != nil {
		r.node(branch)
	}
}

// visible evaluates an `if` / `visible` / `show` condition (default true).
func (r *renderer) visible(n *model.Node) bool {
	for _, key := range []string{"if", "visible", "show"} {
		if raw, ok := n.Prop(key); ok {
			return asBool(runtime.EvalBinding(fmt.Sprint(raw), r.ctx()))
		}
	}
	return true
}

// ---- containers ----

// ---- text & interactive ----

// ---- media & decorative ----

// boundArray resolves a node prop that is a bound array expression or a literal.
func (r *renderer) boundArray(n *model.Node, key string) []any {
	raw, _ := n.Prop(key)
	switch d := raw.(type) {
	case string:
		if arr, ok := runtime.EvalBinding(d, r.ctx()).([]any); ok {
			return arr
		}
	case []any:
		return d
	}
	return nil
}

// ---- handler registration ----

func (r *renderer) register(inv *model.Invoke) int {
	// Callback props: an invoke name may itself be a binding — a component
	// template writes onPress:{name:"{{prop.onConfirm}}"} and the instance
	// passes onConfirm:"saveItem". Resolve it against the current scope at
	// REGISTRATION time, so the handler table (and every dispatch path that
	// reads it) carries the final action name; dispatching never has to guess
	// whether a name is literal or bound.
	name := inv.Name
	if strings.Contains(name, "{{") {
		name = r.interp(name)
	}
	scope := make(map[string]any, len(r.scope))
	for k, v := range r.scope {
		scope[k] = v
	}
	r.handlers = append(r.handlers, Handler{Name: name, Args: inv.Args, Scope: scope})
	return len(r.handlers) - 1
}

func (r *renderer) pressAttr(n *model.Node) string {
	if n.OnPress == nil {
		return ""
	}
	return fmt.Sprintf(` onclick="qorm(%d)"`, r.register(n.OnPress))
}

// changeAttr wires an onChange action, or a no-op state-sync (qorm(-1)) when the
// control is bound to state but has no explicit onChange.
func (r *renderer) changeAttr(n *model.Node, bound bool) string {
	if n.OnChange != nil {
		return fmt.Sprintf(` onchange="qorm(%d)"`, r.register(n.OnChange))
	}
	if bound {
		return ` onchange="qorm(-1)"`
	}
	return ""
}

// ---- CSS assembly ----

// ---- attribute helpers ----

// ---- value/style helpers ----

// ---- chart helpers ----
