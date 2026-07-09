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
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// nid returns a node's id made unique within the current list item, so widgets
// wired by document.getElementById (dismissible, contextmenu, refresh, long-
// press) work when repeated in a renderItem.
func (r *renderer) nid(n *model.Node) string { return n.ID + r.idSuffix }

// Render renders the entry scene of a runtime.
func Render(rt *runtime.Runtime) Result { return RenderScene(rt, "") }

// RenderScene renders a specific scene by id (empty / unknown falls back to the
// entry scene) — lets a desktop window show a different scene of the same app.
func RenderScene(rt *runtime.Runtime, sceneID string) Result {
	r := &renderer{rt: rt, scope: map[string]any{}}
	root := rt.App.EntryRoot()
	if sceneID != "" {
		if sc := rt.App.Scenes[sceneID]; sc != nil {
			root = sc
		}
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
			html = html[:i] + ` data-scene="` + key + `"` + html[i:]
		}
	}
	return Result{HTML: html, Handlers: r.handlers, Unknown: r.unknowns}
}

// renderComponent instantiates an app-defined component: the instance node's
// props/text/label/value become {{prop.x}} inside the template, its children
// fill any {type:slot}, and its id suffixes the template ids so repeated uses
// stay unique.
func (r *renderer) renderComponent(n *model.Node, comp *model.Node) {
	prevScope, prevKids, prevSuf := r.scope, r.compChildren, r.idSuffix
	prop := map[string]any{}
	for k, v := range n.Props {
		prop[k] = v
	}
	if n.Text != "" {
		prop["text"] = n.Text
	}
	if n.Label != "" {
		prop["label"] = n.Label
	}
	if n.Value != "" {
		prop["value"] = n.Value
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

func (r *renderer) ctx() map[string]any {
	if r.catalog == nil {
		r.catalog = r.rt.Catalog() // resolve the i18n catalog once per render, not per node
	}
	if r.viewport == nil {
		r.viewport = r.rt.ViewportVars() // constant during a single render too
	}
	if len(r.scope) == 0 {
		if r.baseCtx == nil { // most nodes have no list scope — share one read-only ctx
			r.baseCtx = map[string]any{"state": r.rt.State, "t": r.catalog, "viewport": r.viewport}
		}
		return r.baseCtx
	}
	m := map[string]any{"state": r.rt.State, "t": r.catalog, "viewport": r.viewport}
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
	if !r.visible(n) {
		return
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

// renderInner dispatches a node to its renderer (component instantiation or the
// built-in widget switch), after node() has handled visibility and animation.
func (r *renderer) renderInner(n *model.Node) {
	if comp, ok := r.rt.App.Components[n.Type]; ok && comp != nil && r.compDepth < 32 {
		r.renderComponent(n, comp)
		return
	}
	switch n.Type {
	case "slot":
		for _, c := range r.compChildren {
			r.node(c)
		}
	case "when":
		r.when(n)
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
	case "animatedcontainer", "animatedpadding", "animatedalign", "animatedpositioned":
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
	case "textformfield":
		r.textFormField(n)
	case "circularprogress", "circularprogressindicator":
		r.circularProgress(n)
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
		n.ID, html.EscapeString(n.Type), r.containerCSS(n), a11y(n))
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
	scope := make(map[string]any, len(r.scope))
	for k, v := range r.scope {
		scope[k] = v
	}
	r.handlers = append(r.handlers, Handler{Name: inv.Name, Args: inv.Args, Scope: scope})
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
