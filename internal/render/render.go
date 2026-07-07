// Package render turns a live scene tree into HTML + CSS. Layout is expressed
// as CSS flexbox/grid and delegated to the browser. It covers a top-tier widget
// vocabulary — containers, scroll, grid, text, button, link, input, textarea,
// select, checkbox, switch, radio, slider, image, avatar, icon, badge, card,
// divider, spacer, progress, spinner, video, tabs and data-bound lists — plus
// conditional rendering (`if`), onChange events and accessibility attributes.
package render

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// Handler is a press/change handler captured during render. Args are raw
// expression strings; Scope carries per-item bindings (e.g. `item`) so they
// re-evaluate correctly at event time.
type Handler struct {
	Name  string
	Args  map[string]string
	Scope map[string]any
}

// Result is a rendered scene plus the handlers, indexed by the id embedded in
// each element's onclick/onchange.
type Result struct {
	HTML     string
	Handlers []Handler
	// Unknown lists widget types the render didn't recognise (likely typos) — the
	// self-verify surface: the harness/audit reports these so an AI catches its
	// own mistakes. Empty for a clean render.
	Unknown []string
}

type renderer struct {
	rt           *runtime.Runtime
	handlers     []Handler
	scope        map[string]any
	rootID       string // entry-scene root id (gets direction:rtl when RTL)
	rtl          bool
	idSuffix     string // per-item suffix so JS-wired widgets stay unique inside renderItem
	sb           strings.Builder
	unknowns     []string
	compChildren []*model.Node // children of the current component instance (for slot)
	compDepth    int
	// per-render caches: state + the resolved i18n catalog are constant during a
	// single render, so compute them once instead of per bound node.
	catalog map[string]any
	baseCtx map[string]any
}

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
	return Result{HTML: r.sb.String(), Handlers: r.handlers, Unknown: r.unknowns}
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
	if len(r.scope) == 0 {
		if r.baseCtx == nil { // most nodes have no list scope — share one read-only ctx
			r.baseCtx = map[string]any{"state": r.rt.State, "t": r.catalog}
		}
		return r.baseCtx
	}
	m := map[string]any{"state": r.rt.State, "t": r.catalog}
	for k, v := range r.scope {
		m[k] = v
	}
	return m
}

func (r *renderer) interp(s string) string {
	return runtime.Stringify(runtime.EvalBinding(s, r.ctx()))
}

// animationWidgets consume the `animation` prop themselves (via motion), so the
// universal wrap skips them to avoid double-animating.
var animationWidgets = map[string]bool{
	"motion": true, "animated": true, "transition": true, "animatedswitcher": true,
	"fadetransition": true, "slidetransition": true, "scaletransition": true,
	"rotationtransition": true, "sizetransition": true, "hero": true,
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

// wrapAnimation renders n inside a div playing the named entrance effect, so a
// component instance (`{"type":"Card","animation":"fadeup"}`) or any widget
// animates without a `motion` wrapper.
func (r *renderer) wrapAnimation(n *model.Node, effect string) {
	kf := motionKeyframe[strings.ToLower(effect)]
	if kf == "" {
		kf = "qa-fade"
	}
	dur := propNum(n, "duration", 450)
	delay := propNum(n, "delay", 0)
	curve := propStrOr(n, "curve", "cubic-bezier(.34,1.2,.64,1)")
	repeat := propStrOr(n, "repeat", "1")
	fmt.Fprintf(&r.sb, `<div style="animation:%s %gms %s %gms %s both;">`, kf, dur, curve, delay, repeat)
	r.renderInner(n)
	r.sb.WriteString(`</div>`)
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

// dragAttr marks a node as a window-drag region on desktop (prop "drag": true).
func dragAttr(n *model.Node) string {
	if v, ok := n.Prop("drag"); ok {
		if b, _ := v.(bool); b {
			return ` data-qorm-drag`
		}
	}
	return ""
}

func (r *renderer) container(n *model.Node) {
	a := a11y(n)
	if n.ID == r.rootID && !strings.Contains(a, "role=") {
		a += ` role="main"` // landmark for assistive tech
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s%s%s>`, n.ID, r.containerCSS(n), a, r.pressAttr(n), dragAttr(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

func (r *renderer) list(n *model.Node) {
	if n.Template == nil {
		r.container(n)
		return
	}
	items, _ := runtime.EvalBinding(n.Data, r.ctx()).([]any)
	// Virtualization: `content-visibility:auto` makes the browser skip layout
	// and paint for off-screen items — cheap windowing for long lists with no
	// JS, working with server-rendered HTML. contain-intrinsic-size reserves the
	// scrollbar space so scrolling stays stable.
	virt := propBool(n, "virtualize")
	itemH := propNum(n, "itemHeight", 44)
	wrap := fmt.Sprintf("content-visibility:auto;contain-intrinsic-size:0 %gpx;", itemH)

	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.containerCSS(n)+"flex-direction:column;")
	prev := r.scope
	prevSuf := r.idSuffix
	for i, it := range items {
		r.scope = map[string]any{"item": it}
		for k, v := range prev {
			if k != "item" {
				r.scope[k] = v
			}
		}
		r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, i)
		if virt {
			fmt.Fprintf(&r.sb, `<div style=%q>`, wrap)
		}
		r.node(n.Template)
		if virt {
			r.sb.WriteString(`</div>`)
		}
	}
	r.scope = prev
	r.idSuffix = prevSuf
	r.sb.WriteString(`</div>`)
}

// tabs renders a header row of tab labels and shows the child panel matching
// the active tab. Tab switching is handled client-side (no state round-trip).
func (r *renderer) tabs(n *model.Node) {
	labels := stringList(n.Props["tabs"])
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;")
	r.sb.WriteString(`<div class="qorm-tabbar" style="display:flex;gap:4px;border-bottom:1px solid var(--sep);">`)
	for i, lbl := range labels {
		active := ""
		if i == 0 {
			active = " qorm-tab-active"
		}
		fmt.Fprintf(&r.sb, `<button class="qorm-tab%s" data-tab="%d" onclick="qormTab(this)" style="border:none;background:none;padding:10px 16px;cursor:pointer;font-size:14px;">%s</button>`,
			active, i, html.EscapeString(lbl))
	}
	r.sb.WriteString(`</div>`)
	for i, c := range n.Children {
		disp := "none"
		if i == 0 {
			disp = "block"
		}
		fmt.Fprintf(&r.sb, `<div class="qorm-tabpanel" data-panel="%d" style="display:%s;padding:12px 0;">`, i, disp)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// ---- text & interactive ----

func (r *renderer) text(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>%s</div>`, r.nid(n), style, a11y(n), html.EscapeString(r.interp(n.Text)))
}

func (r *renderer) button(n *model.Node) {
	// Flutter button variants: elevated (default) / filled / text / outlined /
	// icon. Author styles still override these defaults via boxCSS/textCSS.
	base := "display:inline-flex;align-items:center;justify-content:center;cursor:pointer;user-select:none;transition:filter .12s;border:none;"
	switch propStr(n, "variant") {
	case "text":
		base += "background:transparent;color:var(--accent);padding:8px 12px;border-radius:10px;font-weight:500;"
	case "outlined":
		base += "background:transparent;color:var(--accent);border:1px solid var(--accent);padding:8px 14px;border-radius:10px;"
	case "elevated":
		base += "background:var(--surface);color:var(--label);padding:11px 18px;border-radius:12px;box-shadow:0 2px 5px rgba(0,0,0,.14);"
	case "icon":
		base += "background:transparent;border-radius:50%;width:40px;height:40px;padding:0;font-size:18px;color:var(--label2);"
	default: // "filled" and unset: the accent-filled default so a bare button looks like a button
		base += "background:var(--accent);color:#fff;padding:11px 18px;border-radius:12px;font-weight:600;"
	}
	style := base + r.boxCSS(n) + r.textCSS(n)
	fmt.Fprintf(&r.sb, `<button id=%q class="qorm-tap" style=%q%s%s>%s</button>`,
		n.ID, style, a11y(n), r.pressAttr(n), html.EscapeString(r.interp(labelOf(n))))
}

// scaffold is Flutter's Scaffold: an appbar child pins to the top, a bottomnav
// child to the bottom, fab children float bottom-right, the rest is the body.
func (r *renderer) scaffold(n *model.Node) {
	style := r.boxCSS(n) + "position:relative;display:flex;flex-direction:column;min-height:100%;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, style)
	var body, bottom, fabs []*model.Node
	hasAppbar := false
	for _, c := range n.Children {
		switch c.Type {
		case "appbar":
			hasAppbar = true
			r.node(c)
		case "bottomnav", "bottomnavigationbar", "navigationbar":
			bottom = append(bottom, c)
		case "fab", "floatingactionbutton":
			fabs = append(fabs, c)
		default:
			body = append(body, c)
		}
	}
	// Without an app bar the body reaches the top of the screen, so it must clear
	// the status bar / notch itself; an app bar already applies the top safe inset.
	topPad := ""
	if !hasAppbar {
		topPad = "padding-top:var(--safe-top, env(safe-area-inset-top, 0px));"
	}
	r.sb.WriteString(`<div class="qorm-body" style="flex:1;min-height:0;overflow:auto;` + topPad + `padding-bottom:var(--safe-bottom, env(safe-area-inset-bottom, 0px));">`)
	for _, c := range body {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	for _, c := range bottom {
		r.node(c)
	}
	if len(fabs) > 0 {
		r.sb.WriteString(`<div style="position:absolute;right:16px;bottom:76px;">`)
		for _, c := range fabs {
			r.node(c)
		}
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// bottomNav is Flutter's BottomNavigationBar/NavigationBar: a row of icon+label
// destinations bound to state; tapping one dispatches onChange with {value}.
func (r *renderer) bottomNav(n *model.Node) {
	cur := r.interp(n.Value)
	style := r.boxCSS(n) + "display:flex;border-top:1px solid var(--sep);background:var(--surface);"
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-bottomnav" style=%q role="navigation">`, n.ID, style)
	for _, it := range r.boundArray(n, "items") {
		obj, _ := it.(map[string]any)
		if obj == nil {
			continue
		}
		val := fmt.Sprint(obj["value"])
		col := "#6b7280"
		if val == cur {
			col = "var(--accent)"
		}
		attr := ""
		if n.OnChange != nil {
			args := map[string]string{"value": val}
			for k, v := range n.OnChange.Args {
				if k != "value" {
					args[k] = v
				}
			}
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: args}))
		}
		iconName := fmt.Sprint(obj["icon"])
		iconHTML := html.EscapeString(iconName)
		if svg := iconSVG(iconName, 22); svg != "" {
			iconHTML = svg
		}
		fmt.Fprintf(&r.sb, `<button class="qorm-navitem" style="flex:1;display:flex;flex-direction:column;align-items:center;gap:2px;padding:8px 0;border:none;background:none;cursor:pointer;color:%s;"%s>`, col, attr)
		fmt.Fprintf(&r.sb, `<span style="font-size:20px;display:inline-flex;align-items:center;">%s</span><span style="font-size:12px;">%s</span></button>`,
			iconHTML, html.EscapeString(fmt.Sprint(obj["label"])))
	}
	r.sb.WriteString(`</div>`)
}

// snackbar is Flutter's SnackBar: a transient bottom banner shown when `open`.
func (r *renderer) snackbar(n *model.Node) {
	if o := propStr(n, "open"); o != "" {
		if v := r.interp(o); v == "" || v == "false" || v == "0" {
			return
		}
	}
	style := r.boxCSS(n) + "position:fixed;left:50%;bottom:20px;transform:translateX(-50%);display:flex;align-items:center;gap:16px;background:#323232;color:#fff;padding:12px 18px;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,.3);z-index:60;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="status">`, n.ID, style)
	fmt.Fprintf(&r.sb, `<span style="font-size:14px;">%s</span>`, html.EscapeString(r.interp(labelOf(n))))
	if act := r.interp(propStr(n, "action")); act != "" {
		fmt.Fprintf(&r.sb, `<button style="background:none;border:none;color:#7cc0ff;font-weight:600;cursor:pointer;"%s>%s</button>`,
			r.pressAttr(n), html.EscapeString(act))
	}
	r.sb.WriteString(`</div>`)
}

// expansionTile is Flutter's ExpansionTile: a header that reveals its children
// (native <details>/<summary>, no JS required).
func (r *renderer) expansionTile(n *model.Node) {
	open := ""
	if propStr(n, "initiallyExpanded") == "true" {
		open = " open"
	}
	fmt.Fprintf(&r.sb, `<details id=%q style=%q%s>`, n.ID, r.boxCSS(n)+"border-bottom:1px solid var(--sep);", open)
	fmt.Fprintf(&r.sb, `<summary style="display:flex;align-items:center;gap:10px;padding:12px 14px;cursor:pointer;list-style:none;">`)
	if lead := r.interp(propStr(n, "leading")); lead != "" {
		fmt.Fprintf(&r.sb, `<span style="font-size:20px;display:inline-flex;align-items:center;">%s</span>`, iconOrText(lead, 20))
	}
	fmt.Fprintf(&r.sb, `<span style="flex:1;font-size:15px;font-weight:500;">%s</span><span style="color:var(--label2);">▾</span></summary>`,
		html.EscapeString(r.interp(labelOf(n))))
	r.sb.WriteString(`<div style="padding:0 14px 12px;">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></details>`)
}

// wrap is Flutter's Wrap: children flow onto multiple lines (flex-wrap).
func (r *renderer) wrap(n *model.Node) {
	gap := propNum(n, "spacing", 8)
	run := propNum(n, "runSpacing", gap)
	style := fmt.Sprintf("display:flex;flex-wrap:wrap;column-gap:%gpx;row-gap:%gpx;", gap, run)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+style, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// listTile is Flutter's ListTile: [leading] title / subtitle [trailing], tappable.
func (r *renderer) listTile(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;align-items:center;gap:14px;padding:10px 14px;"
	if n.OnPress != nil {
		style += "cursor:pointer;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s%s>`, n.ID, style, a11y(n), r.pressAttr(n))
	if lead := r.interp(propStr(n, "leading")); lead != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:22px;flex:none;display:inline-flex;align-items:center;">%s</div>`, iconOrText(lead, 22))
	}
	r.sb.WriteString(`<div style="flex:1;min-width:0;">`)
	fmt.Fprintf(&r.sb, `<div style="font-size:15px;color:var(--label);">%s</div>`, html.EscapeString(r.interp(labelOf(n))))
	if sub := r.interp(propStr(n, "subtitle")); sub != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:13px;color:var(--label2);">%s</div>`, html.EscapeString(sub))
	}
	r.sb.WriteString(`</div>`)
	if tr := r.interp(propStr(n, "trailing")); tr != "" {
		fmt.Fprintf(&r.sb, `<div style="flex:none;color:var(--label2);">%s</div>`, html.EscapeString(tr))
	} else if n.OnPress != nil && propStr(n, "chevron") != "false" {
		r.sb.WriteString(`<div style="flex:none;color:#c4c4c6;font-size:17px;">›</div>`) // iOS disclosure ›
	}
	for _, c := range n.Children { // allow rich trailing/leading widgets
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// appbar is Flutter's AppBar: leading + title + actions row.
func (r *renderer) appbar(n *model.Node) {
	bg := propStrOr(n, "background", "var(--surface)")
	style := fmt.Sprintf("display:flex;align-items:center;gap:6px;height:calc(44px + var(--safe-top, env(safe-area-inset-top, 0px)));padding:var(--safe-top, env(safe-area-inset-top, 0px)) 8px 0 8px;box-sizing:border-box;background:%s;-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-bottom:.5px solid var(--sep);", bg)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+style, a11y(n))
	if lead := r.interp(propStr(n, "leading")); lead != "" {
		fmt.Fprintf(&r.sb, `<div style="min-width:44px;color:var(--accent);font-size:17px;display:inline-flex;align-items:center;">%s</div>`, iconOrText(lead, 20))
	} else {
		r.sb.WriteString(`<div style="min-width:44px;"></div>`)
	}
	fmt.Fprintf(&r.sb, `<div style="flex:1;text-align:center;font-size:17px;font-weight:600;color:var(--label);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">%s</div>`, html.EscapeString(r.interp(labelOf(n))))
	r.sb.WriteString(`<div style="min-width:44px;display:flex;justify-content:flex-end;gap:4px;color:var(--accent);">`)
	for _, c := range n.Children { // action buttons/icons (iOS blue)
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// fab is Flutter's FloatingActionButton: circular, elevated, fixed corner.
func (r *renderer) fab(n *model.Node) {
	label := r.interp(labelOf(n))
	if label == "" {
		label = "+"
	}
	extended := propStr(n, "extended") == "true"
	shape := "width:56px;height:56px;border-radius:50%;font-size:24px;"
	if extended {
		shape = "height:48px;padding:0 20px;border-radius:24px;font-size:15px;font-weight:600;gap:8px;"
	}
	style := r.boxCSS(n) + "display:inline-flex;align-items:center;justify-content:center;border:none;cursor:pointer;background:var(--accent);color:#fff;box-shadow:0 6px 16px rgba(0,0,0,.18);" + shape
	fmt.Fprintf(&r.sb, `<button id=%q class="qorm-tap" style=%q%s%s>%s</button>`,
		n.ID, style, a11y(n), r.pressAttr(n), html.EscapeString(label))
}

func (r *renderer) link(n *model.Node) {
	href := "javascript:void(0)"
	if v, ok := n.Prop("href"); ok {
		href = fmt.Sprint(v)
	}
	style := r.boxCSS(n) + r.textCSS(n) + "cursor:pointer;text-decoration:none;"
	fmt.Fprintf(&r.sb, `<a id=%q href=%q style=%q%s%s>%s</a>`,
		n.ID, html.EscapeString(href), style, a11y(n), r.pressAttr(n), html.EscapeString(r.interp(labelOf(n))))
}

var stateBindRe = regexp.MustCompile(`^\s*\{\{\s*state\.([a-zA-Z0-9_.]+)\s*\}\}\s*$`)

func boundPath(value string) string {
	if m := stateBindRe.FindStringSubmatch(value); m != nil {
		return m[1]
	}
	return ""
}

func (r *renderer) input(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n) + "outline:none;"
	path := boundPath(n.Value)
	inputType := "text"
	if v, ok := n.Prop("inputType"); ok {
		inputType = fmt.Sprint(v)
	} else if strings.Contains(strings.ToLower(n.ID), "password") {
		inputType = "password"
	}
	fmt.Fprintf(&r.sb, `<input id=%q type=%q value=%q placeholder=%q style=%q%s%s%s>`,
		n.ID, inputType, html.EscapeString(r.interp(n.Value)),
		html.EscapeString(n.Placeholder), style, dataStateAttr(path), a11y(n), r.changeAttr(n, path != ""))
}

func (r *renderer) textarea(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n) + "outline:none;resize:vertical;"
	path := boundPath(n.Value)
	rows := int(propNum(n, "rows", 4))
	fmt.Fprintf(&r.sb, `<textarea id=%q rows="%d" placeholder=%q style=%q%s%s%s>%s</textarea>`,
		n.ID, rows, html.EscapeString(n.Placeholder), style, dataStateAttr(path), a11y(n),
		r.changeAttr(n, path != ""), html.EscapeString(r.interp(n.Value)))
}

func (r *renderer) selectBox(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n)
	path := boundPath(n.Value)
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<select id=%q style=%q%s%s%s>`, n.ID, style, dataStateAttr(path), a11y(n), r.changeAttr(n, path != ""))
	for _, opt := range optionList(n.Props["options"]) {
		sel := ""
		if opt.value == cur {
			sel = " selected"
		}
		fmt.Fprintf(&r.sb, `<option value=%q%s>%s</option>`, html.EscapeString(opt.value), sel, html.EscapeString(opt.label))
	}
	r.sb.WriteString(`</select>`)
}

func (r *renderer) checkbox(n *model.Node) {
	path := boundPath(n.Value)
	checked := ""
	if r.checkedState(n) {
		checked = " checked"
	}
	label := html.EscapeString(r.interp(labelOf(n)))
	// iOS switch: a green pill toggle (Cupertino CupertinoSwitch is the standard
	// boolean control).
	if n.Type == "switch" {
		fmt.Fprintf(&r.sb, `<label id=%q style=%q%s>`, n.ID,
			r.boxCSS(n)+"display:inline-flex;align-items:center;gap:10px;cursor:pointer;font-size:15px;", a11y(n))
		if label != "" {
			fmt.Fprintf(&r.sb, `<span style="flex:1;">%s</span>`, label)
		}
		fmt.Fprintf(&r.sb, `<span class="qorm-switch"><input type="checkbox"%s%s%s><span></span></span></label>`,
			checked, dataStateAttr(path), r.changeAttr(n, path != ""))
		return
	}
	fmt.Fprintf(&r.sb, `<label id=%q style=%q%s><input type="checkbox"%s style="width:18px;height:18px;accent-color:var(--accent);"%s%s>%s</label>`,
		n.ID, r.boxCSS(n)+"display:inline-flex;align-items:center;gap:8px;cursor:pointer;", a11y(n),
		checked, dataStateAttr(path), r.changeAttr(n, path != ""), label)
}

func (r *renderer) checkedState(n *model.Node) bool {
	if p := boundPath(n.Value); p != "" {
		return asBool(runtime.EvalBinding(n.Value, r.ctx()))
	}
	if v, ok := n.Prop("checked"); ok {
		return asBool(runtime.EvalBinding(fmt.Sprint(v), r.ctx()))
	}
	return false
}

func (r *renderer) radio(n *model.Node) {
	path := boundPath(n.Value)
	cur := r.interp(n.Value)
	name := n.ID
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:6px;", a11y(n))
	for _, opt := range optionList(n.Props["options"]) {
		checked := ""
		if opt.value == cur {
			checked = " checked"
		}
		fmt.Fprintf(&r.sb, `<label style="display:inline-flex;align-items:center;gap:8px;cursor:pointer;"><input type="radio" name=%q value=%q%s%s%s>%s</label>`,
			name, html.EscapeString(opt.value), checked, dataStateAttr(path), r.changeAttr(n, path != ""),
			html.EscapeString(opt.label))
	}
	r.sb.WriteString(`</div>`)
}

func (r *renderer) slider(n *model.Node) {
	min := propNum(n, "min", 0)
	max := propNum(n, "max", 100)
	step := propNum(n, "step", 1)
	path := boundPath(n.Value)
	val := asFloat(runtime.EvalBinding(n.Value, r.ctx()))
	pct := 0.0
	if max > min {
		pct = (val - min) / (max - min) * 100
	}
	fill := fmt.Sprintf("--pct:%g%%;", pct)
	fmt.Fprintf(&r.sb, `<input id=%q class="qorm-slider" type="range" min=%q max=%q step=%q value=%q style=%q%s%s%s>`,
		n.ID, num(min), num(max), num(step), num(val), r.boxCSS(n)+fill, dataStateAttr(path), a11y(n), r.changeAttr(n, path != ""))
}

// ---- media & decorative ----

func (r *renderer) image(n *model.Node) {
	src := propStr(n, "src")
	style := r.boxCSS(n) + "object-fit:" + propStrOr(n, "fit", "cover") + ";"
	fmt.Fprintf(&r.sb, `<img id=%q src=%q style=%q alt=%q%s>`,
		n.ID, html.EscapeString(src), style, html.EscapeString(propStr(n, "alt")), a11y(n))
}

func (r *renderer) avatar(n *model.Node) {
	size := propNum(n, "size", 40)
	base := fmt.Sprintf("width:%gpx;height:%gpx;border-radius:50%%;overflow:hidden;flex-shrink:0;", size, size)
	if src := propStr(n, "src"); src != "" {
		fmt.Fprintf(&r.sb, `<img id=%q src=%q style=%q alt="">`, n.ID, html.EscapeString(src), r.boxCSS(n)+base+"object-fit:cover;")
		return
	}
	initials := r.interp(propStrOr(n, "initials", propStr(n, "name")))
	if rs := []rune(initials); len(rs) > 2 {
		initials = string(rs[:2]) // rune-safe: don't split a multibyte glyph
	}
	style := r.boxCSS(n) + base + r.textCSS(n) +
		"display:inline-flex;align-items:center;justify-content:center;background:#6366f1;color:#fff;font-weight:600;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>%s</div>`, n.ID, style, html.EscapeString(strings.ToUpper(initials)))
}

func (r *renderer) icon(n *model.Node) {
	name := r.interp(propStrOr(n, "icon", propStrOr(n, "glyph", n.Text)))
	style := r.boxCSS(n) + r.textCSS(n) + "display:inline-flex;align-items:center;justify-content:center;line-height:1;"
	// Prefer a built-in SVG icon (the framework's alternative to emoji); fall
	// back to the raw text/glyph for names we don't ship.
	if svg := iconSVG(name, propNum(n, "size", 22)); svg != "" {
		fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, n.ID, style, a11y(n), svg)
		return
	}
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, n.ID, style, a11y(n), html.EscapeString(name))
}

// iconOrText resolves a string that may name a built-in icon: it returns the
// inline SVG when the name is known, otherwise the escaped raw text. This lets
// string props (leading/avatar/prefix/nav icons) reference icon names instead
// of emoji while still accepting plain text.
func iconOrText(s string, size float64) string {
	if svg := iconSVG(s, size); svg != "" {
		return svg
	}
	return html.EscapeString(s)
}

func (r *renderer) badge(n *model.Node) {
	label := r.interp(labelOf(n))
	// Flutter Badge(child): with children, render a corner count/dot over the
	// wrapped child; a "0"/empty count is hidden unless showZero.
	if len(n.Children) > 0 {
		fmt.Fprintf(&r.sb, `<span id=%q style=%q>`, n.ID, r.boxCSS(n)+"position:relative;display:inline-flex;")
		for _, c := range n.Children {
			r.node(c)
		}
		if label != "" && !(label == "0" && propStr(n, "showZero") != "true") {
			dot := "min-width:18px;height:18px;padding:0 5px;border-radius:9px;font-size:11px;"
			if propStr(n, "smallSize") == "true" {
				dot = "width:8px;height:8px;border-radius:4px;"
			}
			bg := propStrOr(n, "color", "#ef4444")
			fmt.Fprintf(&r.sb, `<span style="position:absolute;top:-6px;right:-6px;display:inline-flex;align-items:center;justify-content:center;background:%s;color:#fff;font-weight:700;box-shadow:0 0 0 2px var(--surface);%s">%s</span>`,
				bg, dot, html.EscapeString(label))
		}
		r.sb.WriteString(`</span>`)
		return
	}
	style := r.boxCSS(n) + r.textCSS(n) +
		"display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;font-size:12px;font-weight:600;background:var(--fill);color:var(--label2);"
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, n.ID, style, a11y(n), html.EscapeString(label))
}

func (r *renderer) divider(n *model.Node) {
	vertical := propStr(n, "orientation") == "vertical" || n.Type == "verticaldivider"
	line := "height:1px;width:100%;background:var(--sep);margin:8px 0;"
	if vertical {
		line = "width:1px;align-self:stretch;background:var(--sep);margin:0 8px;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q></div>`, n.ID, r.boxCSS(n)+line)
}

func (r *renderer) spacer(n *model.Node) {
	style := "flex:1 1 auto;"
	if v, ok := numOK(n.Style, "size"); ok {
		style = fmt.Sprintf("width:%gpx;height:%gpx;flex-shrink:0;", v, v)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q></div>`, n.ID, style)
}

func (r *renderer) progress(n *model.Node) {
	v := asFloat(runtime.EvalBinding(n.Value, r.ctx()))
	if v > 0 && v <= 1 { // accept a 0..1 fraction as well as a 0..100 percentage
		v *= 100
	}
	pct := clampPct(v)
	fill := propStrOr(n, "color", "var(--accent)")
	track := r.boxCSS(n) + "background:var(--fill);overflow:hidden;border-radius:999px;min-height:8px;width:100%;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="progressbar"><div style="width:%g%%;height:100%%;background:%s;transition:width .2s;"></div></div>`,
		n.ID, track, pct, fill)
}

func (r *renderer) spinner(n *model.Node) {
	size := propNum(n, "size", 24)
	color := propStrOr(n, "color", "var(--accent)")
	style := fmt.Sprintf("width:%gpx;height:%gpx;border:3px solid var(--sep);border-top-color:%s;border-radius:50%%;", size, size, color)
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-spin" style=%q role="status" aria-label="loading"></div>`, n.ID, r.boxCSS(n)+style)
}

// chart renders a bar / line / area / sparkline as inline SVG. Data is a bound
// array ("{{state.series}}") or a literal number array in the `data` prop.
func (r *renderer) chart(n *model.Node) {
	vals := r.chartData(n)
	w := numOrDefault(n.Style, "width", 240)
	h := numOrDefault(n.Style, "height", 80)
	color := propStrOr(n, "color", "var(--accent)")
	var inner string
	switch propStrOr(n, "chartType", "bar") {
	case "line", "sparkline", "area":
		inner = chartLine(vals, w, h, color, propStrOr(n, "chartType", "line"))
	default:
		inner = chartBars(vals, w, h, color)
	}
	// width:100% so the chart scales to its container (a fixed px width would
	// overflow narrow cards); the viewBox keeps the path coordinates crisp.
	extra := ""
	if m := colorStr(n.Style, "margin"); m != "" {
		_ = m
	}
	// width attribute (natural size) + max-width:100% — fills up to its natural
	// width, caps at the container (no overflow), and keeps a non-zero
	// max-content so it never collapses a shrink-to-fit parent to 0.
	fmt.Fprintf(&r.sb, `<svg id=%q width="%g" height="%g" viewBox="0 0 %g %g" preserveAspectRatio="none" style="display:block;max-width:100%%;height:%gpx;%s">%s</svg>`,
		n.ID, w, h, w, h, h, extra, inner)
}

func (r *renderer) chartData(n *model.Node) []float64 {
	raw, _ := n.Prop("data")
	switch d := raw.(type) {
	case string:
		if arr, ok := runtime.EvalBinding(d, r.ctx()).([]any); ok {
			return toFloats(arr)
		}
	case []any:
		return toFloats(d)
	}
	return nil
}

func (r *renderer) video(n *model.Node) {
	fmt.Fprintf(&r.sb, `<video id=%q src=%q controls style=%q></video>`,
		n.ID, html.EscapeString(propStr(n, "src")), r.boxCSS(n))
}

// table renders a data table from `columns` ([{key,title}] or strings) and
// `data` (bound array of objects or literal).
// datatable is a richer table: sortable column headers (OnChange dispatches
// {column}, with a ▲/▼ indicator when it matches sortField/sortDir) and
// selectable rows (a checkbox column; OnPress dispatches {key} per row and
// {key:"__all__"} for select-all). rowKey (default "id") identifies rows; the
// bound `selected` array holds the chosen keys.
func (r *renderer) datatable(n *model.Node) {
	cols := optionList(n.Props["columns"])
	rows := r.boundArray(n, "data")
	rowKey := propStrOr(n, "rowKey", "id")
	selectable := propBool(n, "selectable") || n.OnPress != nil
	selSet := map[string]bool{}
	for _, k := range r.boundArray(n, "selected") {
		selSet[fmt.Sprint(k)] = true
	}
	sortField := r.interp(propStr(n, "sortField"))
	sortDir := r.interp(propStr(n, "sortDir"))
	arrow := " ⇅"
	if sortDir == "desc" {
		arrow = " ▼"
	} else {
		arrow = " ▲"
	}
	allSel := len(rows) > 0
	for _, row := range rows {
		if obj, ok := row.(map[string]any); ok && !selSet[fmt.Sprint(obj[rowKey])] {
			allSel = false
			break
		}
	}
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-datatable" style=%q><thead><tr>`, n.ID, r.boxCSS(n))
	if selectable {
		box := checkboxCell(allSel)
		if n.OnPress != nil {
			idx := r.register(&model.Invoke{Name: n.OnPress.Name, Args: mergeArgs(n.OnPress.Args, "key", "__all__")})
			fmt.Fprintf(&r.sb, `<th class="qdt-check"><span onclick="qorm(%d)" style="cursor:pointer;font-size:16px;">%s</span></th>`, idx, box)
		} else {
			r.sb.WriteString(`<th class="qdt-check"></th>`)
		}
	}
	for _, c := range cols {
		if n.OnChange != nil {
			ind := ""
			if c.value == sortField {
				ind = arrow
			} else {
				ind = " ⇅"
			}
			idx := r.register(&model.Invoke{Name: n.OnChange.Name, Args: mergeArgs(n.OnChange.Args, "column", c.value)})
			fmt.Fprintf(&r.sb, `<th><button class="qdt-sort" onclick="qorm(%d)">%s<span style="opacity:.6;">%s</span></button></th>`,
				idx, html.EscapeString(c.label), strings.TrimSpace(ind))
		} else {
			r.sb.WriteString("<th>" + html.EscapeString(c.label) + "</th>")
		}
	}
	r.sb.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		obj, _ := row.(map[string]any)
		keyVal := fmt.Sprint(obj[rowKey])
		sel := selSet[keyVal]
		cls := ""
		if sel {
			cls = ` class="qdt-sel"`
		}
		fmt.Fprintf(&r.sb, "<tr%s>", cls)
		if selectable {
			box := checkboxCell(sel)
			if n.OnPress != nil {
				idx := r.register(&model.Invoke{Name: n.OnPress.Name, Args: mergeArgs(n.OnPress.Args, "key", keyVal)})
				fmt.Fprintf(&r.sb, `<td class="qdt-check"><span onclick="qorm(%d)" style="cursor:pointer;font-size:16px;">%s</span></td>`, idx, box)
			} else {
				fmt.Fprintf(&r.sb, `<td class="qdt-check">%s</td>`, box)
			}
		}
		for _, c := range cols {
			r.sb.WriteString("<td>" + html.EscapeString(fmt.Sprint(obj[c.value])) + "</td>")
		}
		r.sb.WriteString("</tr>")
	}
	r.sb.WriteString("</tbody></table>")
}

// checkboxCell renders a small square checkbox glyph without emoji: an empty
// bordered box when unchecked, or an accent-filled box with a check icon when
// checked.
func checkboxCell(checked bool) string {
	if checked {
		return `<span style="display:inline-flex;align-items:center;justify-content:center;width:16px;height:16px;border-radius:3px;background:var(--accent);color:#fff;box-sizing:border-box;">` + iconSVG("check", 11) + `</span>`
	}
	return `<span style="display:inline-block;width:16px;height:16px;border-radius:3px;border:1.5px solid var(--sep);box-sizing:border-box;"></span>`
}

// mergeArgs copies an invoke's args and sets key=val (val is a literal).
func mergeArgs(base map[string]string, key, val string) map[string]string {
	out := map[string]string{key: val}
	for k, v := range base {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func (r *renderer) table(n *model.Node) {
	cols := optionList(n.Props["columns"]) // reuse {value,label} shape: value=key, label=title
	rows := r.boundArray(n, "data")
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-table" style=%q><thead><tr>`, n.ID, r.boxCSS(n))
	for _, c := range cols {
		if n.OnChange != nil { // sortable: header dispatches onChange with {column}
			args := map[string]string{"column": c.value}
			for k, v := range n.OnChange.Args {
				args[k] = v
			}
			idx := r.register(&model.Invoke{Name: n.OnChange.Name, Args: args})
			fmt.Fprintf(&r.sb, `<th><button onclick="qorm(%d)" style="background:none;border:none;font:inherit;font-weight:700;cursor:pointer;padding:0;color:inherit;">%s ⇅</button></th>`,
				idx, html.EscapeString(c.label))
		} else {
			r.sb.WriteString("<th>" + html.EscapeString(c.label) + "</th>")
		}
	}
	r.sb.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		obj, _ := row.(map[string]any)
		r.sb.WriteString("<tr>")
		for _, c := range cols {
			r.sb.WriteString("<td>" + html.EscapeString(fmt.Sprint(obj[c.value])) + "</td>")
		}
		r.sb.WriteString("</tr>")
	}
	r.sb.WriteString("</tbody></table>")
}

// modal renders an overlay dialog when its `open` binding is truthy.
func (r *renderer) modal(n *model.Node) {
	if !asBool(runtime.EvalBinding(propStr(n, "open"), r.ctx())) {
		return
	}
	overlay := "position:fixed;inset:0;background:rgba(0,0,0,.45);display:flex;align-items:center;justify-content:center;z-index:50;padding:20px;"
	panel := r.boxCSS(n) + "background:var(--surface);border-radius:14px;box-shadow:0 20px 60px rgba(0,0,0,.3);width:min(92vw,560px);max-height:90%;overflow:auto;padding:20px;display:flex;flex-direction:column;gap:12px;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="dialog" aria-modal="true"><div style=%q>`, n.ID, overlay, panel)
	if t := r.interp(propStr(n, "title")); t != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:18px;font-weight:700;">%s</div>`, html.EscapeString(t))
	}
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// alert renders a colored info/success/warning/error banner.
func (r *renderer) alert(n *model.Node) {
	bg, fg, icon := alertColors(propStrOr(n, "variant", "info"))
	style := r.boxCSS(n) + fmt.Sprintf("display:flex;gap:10px;align-items:flex-start;padding:12px 14px;border-radius:12px;background:%s;color:%s;", bg, fg)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="alert"><span>%s</span><div style="display:flex;flex-direction:column;gap:2px;">`, n.ID, style, icon)
	if t := r.interp(propStr(n, "title")); t != "" {
		fmt.Fprintf(&r.sb, `<div style="font-weight:700;">%s</div>`, html.EscapeString(t))
	}
	fmt.Fprintf(&r.sb, `<div>%s</div></div></div>`, html.EscapeString(r.interp(labelOf(n))))
}

func alertColors(v string) (bg, fg, icon string) {
	switch v {
	case "success":
		return "color-mix(in srgb,var(--success) 15%,transparent)", "var(--success)", iconSVG("check", 18)
	case "warning":
		return "color-mix(in srgb,var(--warning) 18%,transparent)", "var(--warning)", iconSVG("alert", 18)
	case "error", "danger":
		return "color-mix(in srgb,var(--danger) 15%,transparent)", "var(--danger)", iconSVG("x", 18)
	default:
		return "color-mix(in srgb,var(--accent) 13%,transparent)", "var(--accent)", iconSVG("info", 18)
	}
}

// tag renders a pill/chip, optionally removable.
func (r *renderer) tag(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n) + "display:inline-flex;align-items:center;gap:6px;padding:2px 10px;border-radius:999px;background:var(--fill);color:var(--label2);font-size:13px;"
	fmt.Fprintf(&r.sb, `<span id=%q style=%q>%s`, n.ID, style, html.EscapeString(r.interp(labelOf(n))))
	if n.OnPress != nil { // acts as remove
		fmt.Fprintf(&r.sb, `<button onclick="qorm(%d)" style="border:none;background:none;cursor:pointer;color:inherit;font-size:14px;line-height:1;">×</button>`, r.register(n.OnPress))
	}
	r.sb.WriteString(`</span>`)
}

// skeleton renders a shimmering loading placeholder.
func (r *renderer) skeleton(n *model.Node) {
	style := r.boxCSS(n) + "min-height:14px;border-radius:6px;"
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-skel" style=%q aria-hidden="true"></div>`, n.ID, style)
}

// accordion renders collapsible sections; each child container's `title` prop is
// the header. First section is open; toggling is client-side.
func (r *renderer) accordion(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;border:1px solid var(--sep);border-radius:10px;overflow:hidden;")
	for i, c := range n.Children {
		open := i == 0
		disp := "none"
		if open {
			disp = "block"
		}
		fmt.Fprintf(&r.sb, `<button class="qorm-acc" onclick="qormAcc(this)" style="text-align:left;border:none;border-top:%s;background:var(--surface);padding:12px 14px;cursor:pointer;font-weight:600;font-size:14px;">%s</button>`,
			borderIf(i > 0), html.EscapeString(r.interp(propStr(c, "title"))))
		fmt.Fprintf(&r.sb, `<div class="qorm-acc-panel" style="display:%s;padding:12px 14px;">`, disp)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

func borderIf(b bool) string {
	if b {
		return "1px solid var(--sep)"
	}
	return "none"
}

// rating renders filled/empty stars from `value` out of `max` (default 5).
func (r *renderer) rating(n *model.Node) {
	val := int(asFloat(runtime.EvalBinding(propStr(n, "value"), r.ctx())))
	max := int(propNum(n, "max", 5))
	style := r.boxCSS(n) + "display:inline-flex;gap:2px;font-size:" + num(propNum(n, "size", 18)) + "px;color:#f59e0b;"
	sz := propNum(n, "size", 18)
	fmt.Fprintf(&r.sb, `<span id=%q style=%q role="img" aria-label="%d of %d">`, n.ID, style, val, max)
	for i := 1; i <= max; i++ {
		if i <= val {
			r.sb.WriteString(iconSVG("star", sz))
		} else {
			r.sb.WriteString(`<span style="color:var(--sep);">` + iconSVG("star", sz) + `</span>`)
		}
	}
	r.sb.WriteString(`</span>`)
}

// steps renders a horizontal stepper; `steps` is a label array, `active` the
// current index.
func (r *renderer) steps(n *model.Node) {
	labels := stringList(n.Props["steps"])
	active := int(asFloat(runtime.EvalBinding(propStr(n, "active"), r.ctx())))
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;align-items:center;gap:6px;")
	for i, lbl := range labels {
		done := i <= active
		circleBg, circleFg := "var(--sep)", "var(--label2)"
		lblColor := "var(--label2)"
		if done {
			circleBg, circleFg, lblColor = "var(--accent)", "#fff", "var(--label)"
		}
		fmt.Fprintf(&r.sb, `<div style="display:flex;align-items:center;gap:6px;"><span style="width:24px;height:24px;border-radius:50%%;background:%s;color:%s;display:inline-flex;align-items:center;justify-content:center;font-size:12px;font-weight:700;">%d</span><span style="font-size:13px;color:%s;">%s</span></div>`,
			circleBg, circleFg, i+1, lblColor, html.EscapeString(lbl))
		if i < len(labels)-1 {
			r.sb.WriteString(`<span style="flex:1;height:1px;background:var(--sep);min-width:16px;"></span>`)
		}
	}
	r.sb.WriteString(`</div>`)
}

// breadcrumb renders a path from an `items` label array.
func (r *renderer) breadcrumb(n *model.Node) {
	items := stringList(n.Props["items"])
	sep := propStrOr(n, "separator", "/")
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;gap:8px;align-items:center;font-size:14px;color:var(--label2);")
	for i, it := range items {
		color := "#6b7280"
		if i == len(items)-1 {
			color = "#111827"
		}
		fmt.Fprintf(&r.sb, `<span style="color:%s;">%s</span>`, color, html.EscapeString(it))
		if i < len(items)-1 {
			fmt.Fprintf(&r.sb, `<span style="color:var(--sep);">%s</span>`, html.EscapeString(sep))
		}
	}
	r.sb.WriteString(`</div>`)
}

// pagination renders prev / page-numbers / next; each dispatches the node's
// onPress action with a {page} arg.
func (r *renderer) pagination(n *model.Node) {
	page := int(asFloat(runtime.EvalBinding(propStr(n, "page"), r.ctx())))
	total := int(propNum(n, "total", 1))
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;gap:6px;align-items:center;")
	btn := func(label string, target int, disabled, active bool) {
		style := "min-width:32px;height:32px;border:1px solid var(--sep);border-radius:6px;background:var(--surface);cursor:pointer;font-size:14px;"
		if active {
			style += "background:var(--accent);color:#fff;border-color:var(--accent);"
		}
		if disabled {
			style += "opacity:.5;cursor:default;"
		}
		onclick := ""
		if !disabled && n.OnPress != nil {
			idx := r.register(&model.Invoke{Name: n.OnPress.Name, Args: map[string]string{"page": strconv.Itoa(target)}})
			onclick = fmt.Sprintf(` onclick="qorm(%d)"`, idx)
		}
		fmt.Fprintf(&r.sb, `<button style=%q%s>%s</button>`, style, onclick, html.EscapeString(label))
	}
	btn("‹", page-1, page <= 1, false)
	for p := 1; p <= total; p++ {
		btn(strconv.Itoa(p), p, false, p == page)
	}
	btn("›", page+1, page >= total, false)
	r.sb.WriteString(`</div>`)
}

// menu renders a trigger label plus a client-toggled dropdown panel of children.
func (r *renderer) menu(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-menu" style=%q>`, n.ID, r.boxCSS(n)+"position:relative;display:inline-block;")
	fmt.Fprintf(&r.sb, `<button onclick="qormMenu(this)" style="display:inline-flex;align-items:center;gap:6px;padding:8px 12px;border:1px solid var(--sep);border-radius:8px;background:var(--surface);cursor:pointer;font-size:14px;">%s ▾</button>`,
		html.EscapeString(r.interp(labelOf(n))))
	r.sb.WriteString(`<div class="qorm-menu-panel" style="display:none;position:absolute;top:100%;left:0;margin-top:4px;background:var(--surface);border:1px solid var(--sep);border-radius:8px;box-shadow:0 8px 24px rgba(0,0,0,.12);min-width:160px;z-index:40;padding:4px;">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// tree renders a nested, natively-collapsible view from `data` ([{label,children}]).
func (r *renderer) tree(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"font-size:14px;")
	for _, it := range r.boundArray(n, "data") {
		r.treeItem(it)
	}
	r.sb.WriteString(`</div>`)
}

func (r *renderer) treeItem(v any) {
	obj, _ := v.(map[string]any)
	label := ""
	if obj != nil {
		label = fmt.Sprint(obj["label"])
	} else {
		label = fmt.Sprint(v)
	}
	kids, _ := obj["children"].([]any)
	if len(kids) == 0 {
		fmt.Fprintf(&r.sb, `<div style="padding:3px 0 3px 18px;">%s</div>`, html.EscapeString(label))
		return
	}
	fmt.Fprintf(&r.sb, `<details open><summary style="cursor:pointer;padding:3px 0;">%s</summary><div style="padding-left:16px;">`, html.EscapeString(label))
	for _, c := range kids {
		r.treeItem(c)
	}
	r.sb.WriteString(`</div></details>`)
}

// drawer renders an off-canvas panel (state-controlled `open`) anchored to a side.
func (r *renderer) drawer(n *model.Node) {
	if !asBool(runtime.EvalBinding(propStr(n, "open"), r.ctx())) {
		return
	}
	side := propStrOr(n, "side", "right")
	anchor := "right:0;top:0;bottom:0;"
	if side == "left" {
		anchor = "left:0;top:0;bottom:0;"
	}
	overlay := "position:fixed;inset:0;background:rgba(0,0,0,.4);z-index:60;"
	panel := r.boxCSS(n) + "position:absolute;" + anchor + "width:min(80%,320px);background:var(--surface);box-shadow:0 0 40px rgba(0,0,0,.25);padding:20px;overflow:auto;display:flex;flex-direction:column;gap:12px;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="dialog"><div style=%q>`, n.ID, overlay, panel)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// carousel renders a horizontally scroll-snapping row of children.
func (r *renderer) carousel(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;overflow-x:auto;scroll-snap-type:x mandatory;gap:12px;-webkit-overflow-scrolling:touch;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, style)
	for _, c := range n.Children {
		r.sb.WriteString(`<div style="scroll-snap-align:start;flex:0 0 auto;">`)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// timeline renders a vertical dotted timeline from `items` ([{title,text}]).
func (r *renderer) timeline(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;")
	items := r.boundArray(n, "items")
	for i, it := range items {
		obj, _ := it.(map[string]any)
		title, textv := "", ""
		if obj != nil {
			title, textv = fmt.Sprint(obj["title"]), fmt.Sprint(obj["text"])
		} else {
			title = fmt.Sprint(it)
		}
		line := "flex:1;width:2px;background:var(--sep);"
		if i == len(items)-1 {
			line = ""
		}
		fmt.Fprintf(&r.sb, `<div style="display:flex;gap:12px;">`+
			`<div style="display:flex;flex-direction:column;align-items:center;"><span style="width:12px;height:12px;border-radius:50%%;background:var(--accent);flex-shrink:0;margin-top:3px;"></span><span style="%s"></span></div>`+
			`<div style="padding-bottom:16px;"><div style="font-weight:600;font-size:14px;color:var(--label);">%s</div><div style="font-size:13px;color:var(--label2);">%s</div></div></div>`,
			line, html.EscapeString(title), html.EscapeString(textv))
	}
	r.sb.WriteString(`</div>`)
}

// field wraps a control with a label, required marker, and a conditional error
// (shown when the `error` binding is non-empty) or help text.
func (r *renderer) field(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:5px;")
	if label := r.interp(propStr(n, "label")); label != "" {
		star := ""
		if propBool(n, "required") {
			star = `<span style="color:#ef4444;"> *</span>`
		}
		fmt.Fprintf(&r.sb, `<label style="font-size:13px;font-weight:600;color:var(--label2);">%s%s</label>`, html.EscapeString(label), star)
	}
	for _, c := range n.Children {
		r.node(c)
	}
	if err := r.interp(propStr(n, "error")); err != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:12px;color:#ef4444;">%s</div>`, html.EscapeString(err))
	} else if help := r.interp(propStr(n, "help")); help != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:12px;color:var(--label2);">%s</div>`, html.EscapeString(help))
	}
	r.sb.WriteString(`</div>`)
}

// stat renders a metric: big value, label, and an optional colored delta.
func (r *renderer) stat(n *model.Node) {
	value := r.interp(propStr(n, "value"))
	if value == "" {
		value = r.interp(n.Text)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:2px;")
	if label := r.interp(propStr(n, "label")); label != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:12px;color:var(--label2);text-transform:uppercase;letter-spacing:.04em;">%s</div>`, html.EscapeString(label))
	}
	fmt.Fprintf(&r.sb, `<div style="font-size:28px;font-weight:800;color:var(--label);">%s</div>`, html.EscapeString(value))
	if delta := r.interp(propStr(n, "delta")); delta != "" {
		col := "#6b7280"
		switch propStr(n, "deltaType") {
		case "up", "positive", "success":
			col = "#16a34a"
		case "down", "negative", "error":
			col = "#dc2626"
		}
		fmt.Fprintf(&r.sb, `<div style="font-size:13px;font-weight:600;color:%s;">%s</div>`, col, html.EscapeString(delta))
	}
	r.sb.WriteString(`</div>`)
}

// empty renders a centered empty-state with icon, title and text.
func (r *renderer) empty(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;flex-direction:column;align-items:center;justify-content:center;gap:6px;padding:32px;text-align:center;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, style)
	emptyIcon := propStrOr(n, "icon", "inbox")
	if svg := iconSVG(emptyIcon, 40); svg != "" {
		fmt.Fprintf(&r.sb, `<div style="opacity:.6;color:var(--label2);">%s</div>`, svg)
	} else {
		fmt.Fprintf(&r.sb, `<div style="font-size:40px;opacity:.6;">%s</div>`, html.EscapeString(emptyIcon))
	}
	if title := r.interp(propStr(n, "title")); title != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:16px;font-weight:600;color:var(--label2);">%s</div>`, html.EscapeString(title))
	}
	if text := r.interp(labelOf(n)); text != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:14px;color:var(--label2);">%s</div>`, html.EscapeString(text))
	}
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// segmented is a horizontal single-choice control bound to state (styled radios,
// so the existing state-fold mechanism applies).
func (r *renderer) segmented(n *model.Node) {
	path := boundPath(n.Value)
	cur := r.interp(n.Value)
	changeAttr := r.changeAttr(n, path != "")
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-seg" style=%q role="radiogroup">`, n.ID,
		r.boxCSS(n)+"display:inline-flex;background:var(--fill);border-radius:8px;padding:3px;gap:2px;")
	for _, opt := range optionList(n.Props["options"]) {
		checked := ""
		if opt.value == cur {
			checked = " checked"
		}
		fmt.Fprintf(&r.sb, `<label style="position:relative;"><input type="radio" name=%q value=%q%s%s%s style="position:absolute;opacity:0;width:0;height:0;"><span style="display:inline-block;padding:6px 14px;border-radius:6px;font-size:13px;cursor:pointer;%s">%s</span></label>`,
			n.ID, html.EscapeString(opt.value), checked, dataStateAttr(path), changeAttr,
			segStyle(opt.value == cur), html.EscapeString(opt.label))
	}
	r.sb.WriteString(`</div>`)
}

func segStyle(active bool) string {
	if active {
		return "background:var(--surface);color:var(--label);font-weight:600;box-shadow:0 1px 2px rgba(0,0,0,.1);"
	}
	return "color:var(--label2);"
}

// descriptions renders a two-column key/value list from `items` ([{label,value}]).
func (r *renderer) descriptions(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:grid;grid-template-columns:auto 1fr;gap:8px 16px;font-size:14px;")
	for _, it := range r.boundArray(n, "items") {
		obj, _ := it.(map[string]any)
		if obj == nil {
			continue
		}
		fmt.Fprintf(&r.sb, `<div style="color:var(--label2);">%s</div><div style="color:var(--label);">%s</div>`,
			html.EscapeString(fmt.Sprint(obj["label"])), html.EscapeString(fmt.Sprint(obj["value"])))
	}
	r.sb.WriteString(`</div>`)
}

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

func (r *renderer) containerCSS(n *model.Node) string {
	var b strings.Builder
	if n.Type == "grid" {
		cols := int(propNum(n, "columns", 2))
		fmt.Fprintf(&b, "display:grid;grid-template-columns:repeat(%d,1fr);", cols)
	} else {
		b.WriteString("display:flex;")
	}
	if r.rtl && n.ID == r.rootID {
		b.WriteString("direction:rtl;") // inherited by descendants; flips flex rows + text
	}
	switch n.Type {
	case "row":
		b.WriteString("flex-direction:row;")
	case "stack", "absolute":
		b.WriteString("position:relative;flex-direction:column;")
	case "grid":
		// handled above (display:grid set before the switch)
	default:
		b.WriteString("flex-direction:column;")
	}
	if n.Type == "scroll" {
		if propStr(n, "orientation") == "horizontal" {
			b.WriteString("flex-direction:row;overflow-x:auto;")
		} else {
			b.WriteString("overflow-y:auto;")
		}
	}
	if n.Type == "card" {
		b.WriteString("background:var(--surface);border-radius:14px;box-shadow:0 1px 3px rgba(0,0,0,.08),0 1px 2px rgba(0,0,0,.06);padding:16px;")
	}
	if propBool(n, "wrap") {
		b.WriteString("flex-wrap:wrap;")
	}
	if v := layoutStr(n, "align"); v != "" {
		fmt.Fprintf(&b, "align-items:%s;", flexAlign(v))
	}
	if v := layoutStr(n, "justify"); v != "" {
		fmt.Fprintf(&b, "justify-content:%s;", flexAlign(v))
	}
	b.WriteString(r.boxCSS(n))
	return b.String()
}

// boxCSS renders style + layout properties shared by all node kinds.
// resolveStyle returns a copy of a style/layout map with any `{{ … }}` string
// values evaluated against the current context, so numeric styles (width,
// height, opacity, …) can be bound — the basis for animation and agent-driven
// restyling. The common (binding-free) case returns the input unchanged.
func (r *renderer) resolveStyle(style map[string]any) map[string]any {
	if style == nil {
		return nil
	}
	bound := false
	for _, v := range style {
		if sv, ok := v.(string); ok && strings.Contains(sv, "{{") {
			bound = true
			break
		}
	}
	if !bound {
		return style
	}
	out := make(map[string]any, len(style))
	for k, v := range style {
		if sv, ok := v.(string); ok && strings.Contains(sv, "{{") {
			out[k] = runtime.EvalBinding(sv, r.ctx())
		} else {
			out[k] = v
		}
	}
	return out
}

func (r *renderer) boxCSS(n *model.Node) string {
	var b strings.Builder
	b.WriteString("box-sizing:border-box;")
	s := r.resolveStyle(n.Style)
	lay := r.resolveStyle(n.Layout)
	writeSize(&b, "width", pick(lay, "width"), pick(s, "width"))
	writeSize(&b, "height", pick(lay, "height"), pick(s, "height"))
	writeNum(&b, "min-width", s, "minWidth")
	writeNum(&b, "max-width", s, "maxWidth")
	writeNum(&b, "min-height", s, "minHeight")
	writeNum(&b, "max-height", s, "maxHeight")
	if v, ok := numOK(s, "flexGrow"); ok {
		css(&b, "flex-grow", v, ";flex-basis:0;")
	}
	if v, ok := numOK(s, "aspectRatio"); ok {
		css(&b, "aspect-ratio", v, ";")
	}
	if bg := colorStr(s, "background"); bg != "" {
		fmt.Fprintf(&b, "background:%s;", bg)
	}
	if g := colorStr(s, "gradient"); g != "" {
		fmt.Fprintf(&b, "background:%s;", g)
	}
	if v, ok := numOK(s, "borderRadius"); ok {
		css(&b, "border-radius", v, "px;")
	}
	if bw, ok := numOK(s, "borderWidth"); ok {
		bc := colorStr(s, "borderColor")
		if bc == "" {
			bc = "var(--sep)"
		}
		fmt.Fprintf(&b, "border:%gpx solid %s;", bw, bc)
	}
	if v, ok := numOK(s, "gap"); ok {
		css(&b, "gap", v, "px;")
	}
	if v, ok := numOK(s, "opacity"); ok {
		css(&b, "opacity", v, ";")
	}
	if sh := colorStr(s, "shadow"); sh != "" {
		fmt.Fprintf(&b, "box-shadow:%s;", sh)
	} else if propBool(n, "elevated") {
		b.WriteString("box-shadow:0 4px 12px rgba(0,0,0,.12);")
	}
	if pos := colorStr(s, "position"); pos != "" {
		fmt.Fprintf(&b, "position:%s;", pos)
		for _, edge := range []string{"top", "right", "bottom", "left"} {
			writeNum(&b, edge, s, edge)
		}
	}
	if v := colorStr(s, "cursor"); v != "" {
		fmt.Fprintf(&b, "cursor:%s;", v)
	}
	if v := colorStr(s, "transition"); v != "" {
		fmt.Fprintf(&b, "transition:%s;", v)
	}
	writeEdges(&b, "padding", pick(s, "padding"))
	writeEdges(&b, "margin", pick(s, "margin"))
	return b.String()
}

func (r *renderer) textCSS(n *model.Node) string {
	var b strings.Builder
	s := n.Style
	if v := colorStr(s, "color"); v != "" {
		fmt.Fprintf(&b, "color:%s;", v)
	}
	if v, ok := numOK(s, "fontSize"); ok {
		css(&b, "font-size", v, "px;")
	} else {
		b.WriteString("font-size:15px;")
	}
	if v, ok := numOK(s, "fontWeight"); ok {
		css(&b, "font-weight", v, ";")
	}
	if v := colorStr(s, "fontFamily"); v != "" {
		fmt.Fprintf(&b, "font-family:%s;", v)
	}
	if v, ok := numOK(s, "lineHeight"); ok {
		css(&b, "line-height", v, ";")
	}
	if v, ok := numOK(s, "letterSpacing"); ok {
		css(&b, "letter-spacing", v, "px;")
	}
	if v := colorStr(s, "fontStyle"); v != "" {
		fmt.Fprintf(&b, "font-style:%s;", v)
	}
	if v := colorStr(s, "textDecoration"); v != "" {
		fmt.Fprintf(&b, "text-decoration:%s;", v)
	}
	if v := colorStr(s, "textTransform"); v != "" {
		fmt.Fprintf(&b, "text-transform:%s;", v)
	}
	if v, ok := numOK(s, "lineClamp"); ok {
		css(&b, "display:-webkit-box;-webkit-line-clamp", v, ";-webkit-box-orient:vertical;overflow:hidden;")
	} else if propBool(n, "ellipsis") {
		b.WriteString("white-space:nowrap;overflow:hidden;text-overflow:ellipsis;")
	}
	if v := str(s, "textAlign"); v != "" {
		fmt.Fprintf(&b, "text-align:%s;justify-content:%s;", v, flexAlign(v))
	}
	return b.String()
}

// ---- attribute helpers ----

func a11y(n *model.Node) string {
	var b strings.Builder
	if v, ok := n.Prop("role"); ok {
		fmt.Fprintf(&b, ` role=%q`, html.EscapeString(fmt.Sprint(v)))
	}
	if v, ok := n.Prop("ariaLabel"); ok {
		fmt.Fprintf(&b, ` aria-label=%q`, html.EscapeString(fmt.Sprint(v)))
	}
	if v, ok := n.Prop("title"); ok {
		fmt.Fprintf(&b, ` title=%q`, html.EscapeString(fmt.Sprint(v)))
	}
	if v, ok := n.Prop("tooltip"); ok {
		fmt.Fprintf(&b, ` data-tooltip=%q`, html.EscapeString(fmt.Sprint(v)))
	}
	return b.String()
}

func dataStateAttr(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(` data-state=%q`, path)
}

// ---- value/style helpers ----

type option struct{ value, label string }

func optionList(v any) []option {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]option, 0, len(arr))
	for _, e := range arr {
		switch t := e.(type) {
		case string:
			out = append(out, option{t, t})
		case map[string]any:
			val := fmt.Sprint(t["value"])
			lbl, _ := t["label"].(string)
			if lbl == "" {
				lbl = val
			}
			out = append(out, option{val, lbl})
		}
	}
	return out
}

func stringList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

func labelOf(n *model.Node) string {
	if n.Label != "" {
		return n.Label
	}
	return n.Text
}

// css writes "prop:<float><suffix>" without fmt reflection (Fprintf %g is a hot
// alloc in the per-node CSS builders). strconv 'g'/-1 matches %g exactly.
func css(b *strings.Builder, prop string, v float64, suffix string) {
	b.WriteString(prop)
	b.WriteByte(':')
	var buf [24]byte
	b.Write(strconv.AppendFloat(buf[:0], v, 'g', -1, 64))
	b.WriteString(suffix)
}

func writeSize(b *strings.Builder, dim string, vals ...any) {
	for _, v := range vals {
		switch t := v.(type) {
		case string:
			if t == "fill" {
				fmt.Fprintf(b, "%s:100%%;", dim)
				return
			}
		case float64:
			fmt.Fprintf(b, "%s:%gpx;", dim, t)
			return
		}
	}
}

func writeNum(b *strings.Builder, prop string, m map[string]any, key string) {
	if v, ok := numOK(m, key); ok {
		fmt.Fprintf(b, "%s:%gpx;", prop, v)
	}
}

func writeEdges(b *strings.Builder, prop string, v any) {
	switch t := v.(type) {
	case float64:
		fmt.Fprintf(b, "%s:%gpx;", prop, t)
	case map[string]any:
		fmt.Fprintf(b, "%s:%gpx %gpx %gpx %gpx;", prop,
			asFloat(t["top"]), asFloat(t["right"]), asFloat(t["bottom"]), asFloat(t["left"]))
	}
}

func flexAlign(v string) string {
	switch v {
	case "center":
		return "center"
	case "start", "left", "top":
		return "flex-start"
	case "end", "right", "bottom":
		return "flex-end"
	case "between":
		return "space-between"
	case "around":
		return "space-around"
	case "evenly":
		return "space-evenly"
	case "stretch":
		return "stretch"
	}
	return "flex-start"
}

func clampPct(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 100 {
		return 100
	}
	return f
}

func pick(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}
func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
func layoutStr(n *model.Node, key string) string { return str(n.Layout, key) }
func colorStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}
func numOK(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	f, ok := m[key].(float64)
	return f, ok
}
func propNum(n *model.Node, key string, def float64) float64 {
	if v, ok := n.Prop(key); ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return def
}
func propStr(n *model.Node, key string) string {
	if v, ok := n.Prop(key); ok {
		return fmt.Sprint(v)
	}
	return ""
}
func propStrOr(n *model.Node, key, def string) string {
	if s := propStr(n, key); s != "" {
		return s
	}
	return def
}
func propBool(n *model.Node, key string) bool {
	if v, ok := n.Prop(key); ok {
		return asBool(v)
	}
	return false
}
func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case bool:
		if t {
			return 1
		}
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%g", &f)
		return f
	}
	return 0
}
func asBool(v any) bool {
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
func num(f float64) string { return fmt.Sprintf("%g", f) }

// ---- chart helpers ----

func numOrDefault(m map[string]any, key string, def float64) float64 {
	if v, ok := numOK(m, key); ok {
		return v
	}
	return def
}

func toFloats(arr []any) []float64 {
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		out = append(out, asFloat(v))
	}
	return out
}

func chartBars(vals []float64, w, h float64, color string) string {
	if len(vals) == 0 {
		return ""
	}
	max := vals[0]
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	bw := w / float64(len(vals))
	var b strings.Builder
	for i, v := range vals {
		bh := (v / max) * (h - 2)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="1.5"/>`,
			float64(i)*bw+bw*0.12, h-bh, bw*0.76, bh, color)
	}
	return b.String()
}

func chartLine(vals []float64, w, h float64, color, kind string) string {
	if len(vals) < 2 {
		return ""
	}
	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1
	}
	pts := make([]string, len(vals))
	for i, v := range vals {
		x := float64(i) * (w / float64(len(vals)-1))
		y := h - ((v-min)/rng)*(h-4) - 2
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	stroke := "2"
	if kind == "sparkline" {
		stroke = "1.5"
	}
	line := fmt.Sprintf(`<polyline fill="none" stroke="%s" stroke-width="%s" stroke-linejoin="round" stroke-linecap="round" points="%s"/>`,
		color, stroke, strings.Join(pts, " "))
	if kind == "area" {
		area := fmt.Sprintf(`<polygon fill="%s" fill-opacity="0.15" points="%s %.1f,%.1f 0,%.1f"/>`,
			color, strings.Join(pts, " "), w, h, h)
		return area + line
	}
	return line
}

// controlTile is Flutter's SwitchListTile/CheckboxListTile/RadioListTile: a
// title/subtitle row with a bound control. The control reuses the existing
// switch/checkbox/radio rendering (state fold applies).
func (r *renderer) controlTile(n *model.Node) {
	kind := "switch"
	switch n.Type {
	case "checkboxlisttile":
		kind = "checkbox"
	case "radiolisttile":
		kind = "radio"
	}
	fmt.Fprintf(&r.sb, `<label id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;align-items:center;gap:12px;padding:8px 14px;cursor:pointer;")
	ctrl := func() {
		path := boundPath(n.Value)
		checked := ""
		switch kind {
		case "radio":
			val := propStr(n, "value")
			if r.interp(n.Value) == val {
				checked = " checked"
			}
			fmt.Fprintf(&r.sb, `<input type="radio" name=%q value=%q style="accent-color:var(--accent);width:18px;height:18px;"%s%s%s>`, path, val, checked, dataStateAttr(path), r.changeAttr(n, path != ""))
		case "switch":
			if truthyStrCT(r.interp(n.Value)) {
				checked = " checked"
			}
			fmt.Fprintf(&r.sb, `<span class="qorm-switch"><input type="checkbox"%s%s%s><span></span></span>`, checked, dataStateAttr(path), r.changeAttr(n, path != ""))
		default:
			if truthyStrCT(r.interp(n.Value)) {
				checked = " checked"
			}
			fmt.Fprintf(&r.sb, `<input type="checkbox" style="accent-color:var(--accent);width:18px;height:18px;"%s%s%s>`, checked, dataStateAttr(path), r.changeAttr(n, path != ""))
		}
	}
	if kind != "switch" { // leading control for checkbox/radio
		ctrl()
	}
	r.sb.WriteString(`<span style="flex:1;min-width:0;">`)
	fmt.Fprintf(&r.sb, `<span style="display:block;font-size:15px;color:var(--label);">%s</span>`, html.EscapeString(r.interp(labelOf(n))))
	if sub := r.interp(propStr(n, "subtitle")); sub != "" {
		fmt.Fprintf(&r.sb, `<span style="display:block;font-size:13px;color:var(--label2);">%s</span>`, html.EscapeString(sub))
	}
	r.sb.WriteString(`</span>`)
	if kind == "switch" { // trailing control
		ctrl()
	}
	r.sb.WriteString(`</label>`)
}

func truthyStrCT(s string) bool { return s != "" && s != "false" && s != "0" }

// chip is Flutter's chip family: a compact rounded element. `selected` (a
// binding) toggles the highlighted state; onPress fires on tap (ChoiceChip/
// FilterChip); a delete × dispatches onChange (InputChip). An optional avatar/
// leading glyph and, for a selected filter chip, a check icon are shown.
func (r *renderer) chip(n *model.Node) {
	selected := false
	if s := propStr(n, "selected"); s != "" {
		selected = truthyStrChip(r.interp(s))
	}
	bg, fg, border := "var(--fill)", "#3730a3", "1px solid transparent"
	if selected {
		bg, fg, border = "var(--accent)", "#ffffff", "1px solid var(--accent)"
	}
	style := fmt.Sprintf("display:inline-flex;align-items:center;gap:6px;padding:5px 12px;border-radius:16px;font-size:13px;background:%s;color:%s;border:%s;cursor:pointer;", bg, fg, border)
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s%s>`, n.ID, r.boxCSS(n)+style, a11y(n), r.pressAttr(n))
	if selected && (n.Type == "filterchip" || propStr(n, "showCheck") == "true") {
		r.sb.WriteString(`<span style="display:inline-flex;align-items:center;">` + iconSVG("check", 12) + `</span>`)
	}
	if av := r.interp(propStr(n, "avatar")); av != "" {
		fmt.Fprintf(&r.sb, `<span style="font-size:15px;display:inline-flex;align-items:center;">%s</span>`, iconOrText(av, 15))
	}
	fmt.Fprintf(&r.sb, `<span>%s</span>`, html.EscapeString(r.interp(labelOf(n))))
	if n.OnChange != nil || n.Type == "inputchip" { // delete affordance
		del := ""
		if n.OnChange != nil {
			del = fmt.Sprintf(` onclick="event.stopPropagation();qorm(%d)"`, r.register(n.OnChange))
		}
		fmt.Fprintf(&r.sb, `<span style="margin-left:2px;opacity:.7;font-weight:700;"%s>×</span>`, del)
	}
	r.sb.WriteString(`</span>`)
}

func truthyStrChip(s string) bool { return s != "" && s != "false" && s != "0" }

// rangeSlider is Flutter's RangeSlider: two thumbs bound to a low/high pair of
// state paths, rendered as two overlaid range inputs sharing a track.
func (r *renderer) rangeSlider(n *model.Node) {
	min := propNum(n, "min", 0)
	max := propNum(n, "max", 100)
	step := propNum(n, "step", 1)
	loPath := boundPath(propStr(n, "low"))
	hiPath := boundPath(propStr(n, "high"))
	lo := asFloat(runtime.EvalBinding(propStr(n, "low"), r.ctx()))
	hi := asFloat(runtime.EvalBinding(propStr(n, "high"), r.ctx()))
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"position:relative;height:32px;")
	track := "position:absolute;left:0;right:0;top:14px;width:100%;margin:0;-webkit-appearance:none;background:transparent;pointer-events:none;"
	fmt.Fprintf(&r.sb, `<input type="range" min=%q max=%q step=%q value=%q style=%q class="qorm-range-lo"%s%s>`,
		num(min), num(max), num(step), num(lo), track, dataStateAttr(loPath), r.changeAttr(n, loPath != ""))
	fmt.Fprintf(&r.sb, `<input type="range" min=%q max=%q step=%q value=%q style=%q class="qorm-range-hi"%s%s>`,
		num(min), num(max), num(step), num(hi), track, dataStateAttr(hiPath), r.changeAttr(n, hiPath != ""))
	// filled segment between lo and hi
	span := max - min
	if span == 0 {
		span = 1
	}
	l := (lo - min) / span * 100
	w := (hi - lo) / span * 100
	fmt.Fprintf(&r.sb, `<div style="position:absolute;top:15px;height:4px;border-radius:2px;background:var(--accent);left:%g%%;width:%g%%;"></div>`, l, w)
	r.sb.WriteString(`</div>`)
}

// gridView is Flutter's GridView: renders renderItem for each data element in a
// responsive CSS grid (crossAxisCount columns, or auto-fill by minItemWidth).
func (r *renderer) gridView(n *model.Node) {
	if n.Template == nil {
		r.container(n)
		return
	}
	items, _ := runtime.EvalBinding(n.Data, r.ctx()).([]any)
	cols := int(propNum(n, "crossAxisCount", 0))
	var tmpl string
	if cols > 0 {
		tmpl = fmt.Sprintf("repeat(%d,1fr)", cols)
	} else {
		tmpl = fmt.Sprintf("repeat(auto-fill,minmax(%gpx,1fr))", propNum(n, "minItemWidth", 120))
	}
	gap := propNum(n, "spacing", 10)
	style := fmt.Sprintf("display:grid;grid-template-columns:%s;gap:%gpx;", tmpl, gap)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+style)
	prev := r.scope
	prevSuf := r.idSuffix
	for i, it := range items {
		r.scope = map[string]any{"item": it}
		for k, v := range prev {
			if k != "item" {
				r.scope[k] = v
			}
		}
		r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, i)
		r.node(n.Template)
	}
	r.scope = prev
	r.idSuffix = prevSuf
	r.sb.WriteString(`</div>`)
}

// materialStepper is Flutter's Stepper: a vertical list of steps with a title,
// a connector, and the active step's content plus continue/cancel controls.
// The active index comes from `active` (a binding); step titles from `steps`,
// content from children (one per step).
func (r *renderer) materialStepper(n *model.Node) {
	active := int(asFloat(runtime.EvalBinding(propStr(n, "active"), r.ctx())))
	titles := r.boundArray(n, "steps")
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;")
	for i, t := range titles {
		done := i < active
		cur := i == active
		circleBg, circleFg := "var(--sep)", "#fff"
		if done {
			circleBg = "#16a34a"
		} else if cur {
			circleBg = "var(--accent)"
		}
		markHTML := html.EscapeString(num(float64(i + 1)))
		if done {
			markHTML = iconSVG("check", 14)
		}
		r.sb.WriteString(`<div style="display:flex;gap:12px;">`)
		// index column: circle + connector line
		r.sb.WriteString(`<div style="display:flex;flex-direction:column;align-items:center;">`)
		fmt.Fprintf(&r.sb, `<div style="width:26px;height:26px;border-radius:50%%;background:%s;color:%s;display:flex;align-items:center;justify-content:center;font-size:13px;font-weight:600;flex:none;">%s</div>`,
			circleBg, circleFg, markHTML)
		if i < len(titles)-1 {
			r.sb.WriteString(`<div style="flex:1;width:2px;background:var(--sep);min-height:16px;"></div>`)
		}
		r.sb.WriteString(`</div>`)
		// body: title + (if active) content
		weight := "400"
		if cur {
			weight = "600"
		}
		fmt.Fprintf(&r.sb, `<div style="flex:1;padding-bottom:12px;"><div style="font-size:15px;font-weight:%s;color:var(--label);">%s</div>`,
			weight, html.EscapeString(fmt.Sprint(t)))
		if cur && i < len(n.Children) {
			r.sb.WriteString(`<div style="margin-top:8px;">`)
			r.node(n.Children[i])
			r.sb.WriteString(`</div>`)
		}
		r.sb.WriteString(`</div></div>`)
	}
	r.sb.WriteString(`</div>`)
}

// pageView is Flutter's PageView: full-width children with horizontal
// scroll-snap, so each child is a swipeable page.
func (r *renderer) pageView(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;overflow-x:auto;scroll-snap-type:x mandatory;scroll-behavior:smooth;"
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-pageview" style=%q>`, n.ID, style)
	for _, c := range n.Children {
		r.sb.WriteString(`<div style="flex:0 0 100%;scroll-snap-align:start;min-width:0;">`)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// dropdownButton is Flutter's DropdownButton: a Material-styled trigger showing
// the selected option's label; tapping opens a menu whose items dispatch
// onChange with {value} (and set the bound state path). Distinct from the plain
// native <select>.
func (r *renderer) dropdownButton(n *model.Node) {
	cur := r.interp(n.Value)
	label := cur
	for _, o := range optionList(n.Props["options"]) {
		if o.value == cur && o.label != "" {
			label = o.label
		}
	}
	if label == "" {
		label = propStrOr(n, "hint", "Select…")
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-menu" style=%q>`, n.ID, r.boxCSS(n)+"position:relative;display:inline-block;")
	fmt.Fprintf(&r.sb, `<button onclick="qormMenu(this)" style="display:inline-flex;align-items:center;gap:8px;justify-content:space-between;min-width:140px;padding:9px 12px;border:1px solid var(--sep);border-radius:8px;background:var(--surface);cursor:pointer;font-size:14px;">%s<span style="color:var(--label2);">▾</span></button>`,
		html.EscapeString(label))
	r.sb.WriteString(`<div class="qorm-menu-panel" style="display:none;position:absolute;top:100%;left:0;margin-top:4px;background:var(--surface);border:1px solid var(--sep);border-radius:8px;box-shadow:0 8px 24px rgba(0,0,0,.12);min-width:100%;z-index:40;padding:4px;">`)
	for _, o := range optionList(n.Props["options"]) {
		sel := ""
		if o.value == cur {
			sel = "background:var(--fill);font-weight:600;"
		}
		attr := ""
		if n.OnChange != nil {
			args := map[string]string{"value": o.value}
			for k, v := range n.OnChange.Args {
				if k != "value" {
					args[k] = v
				}
			}
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: args}))
		}
		fmt.Fprintf(&r.sb, `<div role="option" style="padding:8px 10px;border-radius:6px;cursor:pointer;font-size:14px;%s"%s>%s</div>`,
			sel, attr, html.EscapeString(o.label))
	}
	r.sb.WriteString(`</div></div>`)
}

// gestureDetector is Flutter's GestureDetector/InkWell: wraps children and wires
// tap (onPress), double-tap (onDoubleTap) and long-press (onLongPress) to
// action dispatches.
func (r *renderer) gestureDetector(n *model.Node) {
	var attrs, initJS string
	if n.OnPress != nil {
		attrs += fmt.Sprintf(` onclick="qorm(%d)"`, r.register(n.OnPress))
	}
	if dbl := parseInvokeProp(n, "onDoubleTap"); dbl != nil {
		attrs += fmt.Sprintf(` ondblclick="qorm(%d)"`, r.register(dbl))
	}
	if lp := parseInvokeProp(n, "onLongPress"); lp != nil {
		initJS = fmt.Sprintf(`<script>qormLong(document.getElementById(%q),%d)</script>`, r.nid(n), r.register(lp))
	}
	cursor := ""
	if attrs != "" || initJS != "" {
		cursor = "cursor:pointer;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s%s>`, r.nid(n), r.boxCSS(n)+cursor, a11y(n), attrs)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	r.sb.WriteString(initJS)
}

// autocomplete is Flutter's Autocomplete: a text field backed by a native
// <datalist> of suggestions; the value two-way-binds to state.
func (r *renderer) autocomplete(n *model.Node) {
	path := boundPath(n.Value)
	listID := n.ID + "-ac"
	style := r.boxCSS(n)
	if style == "" {
		style = "height:40px;padding:0 12px;border:1px solid var(--sep);border-radius:8px;font-size:14px;"
	}
	fmt.Fprintf(&r.sb, `<input id=%q list=%q value=%q placeholder=%q style=%q%s%s%s>`,
		n.ID, listID, html.EscapeString(r.interp(n.Value)), html.EscapeString(n.Placeholder),
		style, dataStateAttr(path), a11y(n), r.changeAttr(n, path != ""))
	fmt.Fprintf(&r.sb, `<datalist id=%q>`, listID)
	for _, o := range optionList(n.Props["options"]) {
		lbl := o.label
		if lbl == "" {
			lbl = o.value
		}
		fmt.Fprintf(&r.sb, `<option value=%q>`, html.EscapeString(lbl))
	}
	r.sb.WriteString(`</datalist>`)
}

// parseInvokeProp reads an invoke ({name,args}) from an arbitrary node prop.
func parseInvokeProp(n *model.Node, key string) *model.Invoke {
	raw, ok := n.Prop(key)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	inv := &model.Invoke{Name: str(m, "name"), Args: map[string]string{}}
	if args, ok := m["args"].(map[string]any); ok {
		for k, v := range args {
			inv.Args[k] = fmt.Sprint(v)
		}
	}
	if inv.Name == "" {
		return nil
	}
	return inv
}

// textFormField is Flutter's TextFormField with InputDecoration: label, prefix/
// suffix, helper/counter, and reactive validation — the `error` binding (an
// expression over state, e.g. matches(...) ? "" : "Invalid") shows inline and
// reddens the border the moment state changes.
func (r *renderer) textFormField(n *model.Node) {
	path := boundPath(n.Value)
	errText := r.interp(propStr(n, "error"))
	invalid := errText != ""
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:4px;")
	if label := r.interp(propStr(n, "label")); label != "" {
		fmt.Fprintf(&r.sb, `<label style="font-size:13px;font-weight:600;color:var(--label2);">%s</label>`, html.EscapeString(label))
	}
	border := "var(--sep)"
	if invalid {
		border = "#ef4444"
	}
	fmt.Fprintf(&r.sb, `<div style="display:flex;align-items:center;gap:8px;border:1px solid %s;border-radius:8px;padding:0 10px;height:40px;background:var(--surface);">`, border)
	if pre := r.interp(propStr(n, "prefix")); pre != "" {
		fmt.Fprintf(&r.sb, `<span style="color:var(--label2);display:inline-flex;align-items:center;">%s</span>`, iconOrText(pre, 16))
	}
	itype := propStrOr(n, "inputType", "text")
	fmt.Fprintf(&r.sb, `<input type=%q value=%q placeholder=%q style="flex:1;border:none;outline:none;font-size:14px;background:transparent;"%s%s%s>`,
		itype, html.EscapeString(r.interp(n.Value)), html.EscapeString(n.Placeholder), dataStateAttr(path), a11y(n), r.changeAttr(n, path != ""))
	if suf := r.interp(propStr(n, "suffix")); suf != "" {
		fmt.Fprintf(&r.sb, `<span style="color:var(--label2);">%s</span>`, html.EscapeString(suf))
	}
	r.sb.WriteString(`</div>`)
	// footer: helper/error on the left, counter on the right
	r.sb.WriteString(`<div style="display:flex;justify-content:space-between;font-size:12px;">`)
	if invalid {
		fmt.Fprintf(&r.sb, `<span style="color:#ef4444;">%s</span>`, html.EscapeString(errText))
	} else if help := r.interp(propStr(n, "helper")); help != "" {
		fmt.Fprintf(&r.sb, `<span style="color:var(--label2);">%s</span>`, html.EscapeString(help))
	} else {
		r.sb.WriteString(`<span></span>`)
	}
	if maxLen := int(propNum(n, "maxLength", 0)); maxLen > 0 {
		fmt.Fprintf(&r.sb, `<span style="color:var(--label2);">%d/%d</span>`, len([]rune(r.interp(n.Value))), maxLen)
	}
	r.sb.WriteString(`</div></div>`)
}

// circularProgress is Flutter's CircularProgressIndicator: an SVG ring. With a
// `value` (0..1) it is determinate (an arc); without, it spins indeterminately.
func (r *renderer) circularProgress(n *model.Node) {
	size := propNum(n, "size", 44)
	stroke := propNum(n, "stroke", 4)
	rad := (size - stroke) / 2
	circ := 2 * 3.14159265 * rad
	color := propStrOr(n, "color", "var(--accent)")
	cx := size / 2
	fmt.Fprintf(&r.sb, `<svg id=%q width="%g" height="%g" viewBox="0 0 %g %g" style=%q%s>`,
		n.ID, size, size, size, size, r.boxCSS(n), a11y(n))
	fmt.Fprintf(&r.sb, `<circle cx="%g" cy="%g" r="%g" fill="none" stroke="var(--sep)" stroke-width="%g"/>`, cx, cx, rad, stroke)
	if v := propStr(n, "value"); v != "" {
		frac := asFloat(runtime.EvalBinding(v, r.ctx()))
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		off := circ * (1 - frac)
		fmt.Fprintf(&r.sb, `<circle cx="%g" cy="%g" r="%g" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round" stroke-dasharray="%g" stroke-dashoffset="%g" transform="rotate(-90 %g %g)"/>`,
			cx, cx, rad, color, stroke, circ, off, cx, cx)
	} else {
		// indeterminate: a quarter arc that spins
		fmt.Fprintf(&r.sb, `<circle cx="%g" cy="%g" r="%g" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round" stroke-dasharray="%g %g" transform="rotate(-90 %g %g)"><animateTransform attributeName="transform" type="rotate" from="0 %g %g" to="360 %g %g" dur="1s" repeatCount="indefinite"/></circle>`,
			cx, cx, rad, color, stroke, circ/4, circ, cx, cx, cx, cx, cx, cx)
	}
	r.sb.WriteString(`</svg>`)
}

// dialogAction is one button in an iOS dialog/action sheet.
type dialogAction struct {
	label, style string
	inv          *model.Invoke
}

// dialogActions parses an `actions` prop ([{label,style,onPress}]) into buttons.
func (r *renderer) dialogActions(n *model.Node, key string) []dialogAction {
	raw, _ := n.Prop(key)
	arr, _ := raw.([]any)
	var out []dialogAction
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		da := dialogAction{label: r.interp(str(m, "label")), style: str(m, "style")}
		if op, ok := m["onPress"].(map[string]any); ok {
			da.inv = &model.Invoke{Name: str(op, "name"), Args: map[string]string{}}
			if a, ok := op["args"].(map[string]any); ok {
				for k, v := range a {
					da.inv.Args[k] = fmt.Sprint(v)
				}
			}
		}
		out = append(out, da)
	}
	return out
}

func (r *renderer) actionColor(style string) string {
	switch style {
	case "destructive":
		return "var(--danger)"
	default:
		return "var(--accent)"
	}
}

// alertDialog is Cupertino's CupertinoAlertDialog: a centered rounded card with
// title, message and stacked/side-by-side action buttons. Shown while `open`.
func (r *renderer) alertDialog(n *model.Node) {
	if o := propStr(n, "open"); o != "" {
		if v := r.interp(o); v == "" || v == "false" || v == "0" {
			return
		}
	}
	r.sb.WriteString(`<div style="position:fixed;inset:0;background:rgba(0,0,0,.28);display:flex;align-items:center;justify-content:center;z-index:70;">`)
	fmt.Fprintf(&r.sb, `<div id=%q style="width:270px;background:var(--surface);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-radius:14px;overflow:hidden;text-align:center;">`, n.ID)
	r.sb.WriteString(`<div style="padding:18px 16px 14px;">`)
	if t := r.interp(propStr(n, "title")); t != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:17px;font-weight:600;color:#000;">%s</div>`, html.EscapeString(t))
	}
	if m := r.interp(propStr(n, "message")); m != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:13px;color:#000;margin-top:4px;">%s</div>`, html.EscapeString(m))
	}
	r.sb.WriteString(`</div>`)
	actions := r.dialogActions(n, "actions")
	sideBySide := len(actions) == 2
	if sideBySide {
		r.sb.WriteString(`<div style="display:flex;border-top:.5px solid var(--sep);">`)
	} else {
		r.sb.WriteString(`<div style="display:flex;flex-direction:column;">`)
	}
	for i, a := range actions {
		sep := "border-top:.5px solid var(--sep);"
		if sideBySide && i > 0 {
			sep = "border-left:.5px solid var(--sep);"
		}
		weight := "400"
		if a.style == "cancel" {
			weight = "600"
		}
		attr := ""
		if a.inv != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(a.inv))
		}
		fmt.Fprintf(&r.sb, `<button style="flex:1;padding:12px;background:none;border:none;%sfont-size:17px;font-weight:%s;color:%s;cursor:pointer;"%s>%s</button>`,
			sep, weight, r.actionColor(a.style), attr, html.EscapeString(a.label))
	}
	r.sb.WriteString(`</div></div></div>`)
}

// actionSheet is Cupertino's CupertinoActionSheet: a bottom sheet of actions
// with an optional destructive item and a separated Cancel. Shown while `open`.
func (r *renderer) actionSheet(n *model.Node) {
	if o := propStr(n, "open"); o != "" {
		if v := r.interp(o); v == "" || v == "false" || v == "0" {
			return
		}
	}
	r.sb.WriteString(`<div class="qorm-sheet" style="position:fixed;inset:0;background:rgba(0,0,0,.28);display:flex;align-items:flex-end;justify-content:center;z-index:70;padding:8px;">`)
	fmt.Fprintf(&r.sb, `<div id=%q style="width:100%%;max-width:400px;">`, n.ID)
	// group card
	r.sb.WriteString(`<div style="background:var(--surface);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-radius:14px;overflow:hidden;text-align:center;">`)
	if t := r.interp(propStr(n, "title")); t != "" {
		fmt.Fprintf(&r.sb, `<div style="padding:14px 16px;font-size:13px;color:#3c3c4399;border-bottom:.5px solid var(--sep);">%s</div>`, html.EscapeString(t))
	}
	for _, a := range r.dialogActions(n, "actions") {
		attr := ""
		if a.inv != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(a.inv))
		}
		fmt.Fprintf(&r.sb, `<button style="width:100%%;padding:16px;background:none;border:none;border-top:.5px solid var(--sep);font-size:20px;color:%s;cursor:pointer;"%s>%s</button>`,
			r.actionColor(a.style), attr, html.EscapeString(a.label))
	}
	r.sb.WriteString(`</div>`)
	// separated cancel
	if c := r.dialogActions(n, "cancel"); len(c) > 0 {
		attr := ""
		if c[0].inv != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(c[0].inv))
		}
		fmt.Fprintf(&r.sb, `<button style="width:100%%;margin-top:8px;padding:16px;background:var(--surface);border:none;border-radius:14px;font-size:20px;font-weight:600;color:var(--accent);cursor:pointer;"%s>%s</button>`,
			attr, html.EscapeString(c[0].label))
	}
	r.sb.WriteString(`</div></div>`)
}

// listSection is Cupertino's CupertinoListSection: an inset grouped list — an
// uppercase header over a rounded surface card whose children are separated by
// inset hairlines (the standard iOS Settings-style list).
func (r *renderer) listSection(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"padding:16px;")
	if h := r.interp(propStr(n, "header")); h != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:13px;color:var(--label2);text-transform:uppercase;letter-spacing:.02em;padding:0 16px 6px;">%s</div>`, html.EscapeString(h))
	}
	r.sb.WriteString(`<div style="background:var(--surface);border-radius:10px;overflow:hidden;">`)
	for i, c := range n.Children {
		if i > 0 {
			r.sb.WriteString(`<div style="height:.5px;background:var(--sep);margin-left:16px;"></div>`)
		}
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	if f := r.interp(propStr(n, "footer")); f != "" {
		fmt.Fprintf(&r.sb, `<div style="font-size:13px;color:var(--label2);padding:6px 16px 0;">%s</div>`, html.EscapeString(f))
	}
	r.sb.WriteString(`</div>`)
}

// activityIndicator is Cupertino's CupertinoActivityIndicator: eight tapered
// spokes ticking around (the iOS spinner).
func (r *renderer) activityIndicator(n *model.Node) {
	size := propNum(n, "size", 20)
	fmt.Fprintf(&r.sb, `<span id=%q class="qorm-activity" style=%q><svg width="%g" height="%g" viewBox="0 0 20 20">`,
		n.ID, r.boxCSS(n), size, size)
	for i := 0; i < 8; i++ {
		op := 0.25 + 0.75*float64(i)/7
		fmt.Fprintf(&r.sb, `<rect x="9" y="2" width="2" height="5" rx="1" fill="var(--label2)" opacity="%g" transform="rotate(%d 10 10)"/>`, op, i*45)
	}
	r.sb.WriteString(`</svg></span>`)
}

// picker is Cupertino's CupertinoPicker: a scroll-snap wheel with a highlighted
// center band; tapping an option selects it (dispatches onChange with {value}).
// screens shows the connected displays (multi-monitor awareness) on desktop.
func (r *renderer) screens(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-screens" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-screens-out" style="font-size:14px;color:var(--label);min-height:20px;white-space:pre-line;font-family:ui-monospace,Menlo,monospace;">—</div>`, n.ID)
	r.sb.WriteString(`</div>`)
}

// loginItem renders a toggle for launch-at-login (desktop).
func (r *renderer) loginItem(n *model.Node) {
	label := n.Label
	if label == "" {
		label = "Toggle Start at Login"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-loginitem" data-on="0" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-loginitem-out" style="font-size:15px;color:var(--label);min-height:20px;">Start at Login: —</div>`, n.ID)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormLoginItem(this)" style="padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(label))
	r.sb.WriteString(`</div>`)
}

// dockBadge renders -/+ buttons that set the Dock icon badge (unread count) on
// desktop.
func (r *renderer) dockBadge(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-dockbadge" data-count="0" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-dockbadge-out" style="font-size:15px;color:var(--label);min-height:20px;">Badge: 0</div>`, n.ID)
	r.sb.WriteString(`<div style="display:flex;gap:8px;"><button type="button" onclick="qormBadge(this,-1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--fill);color:var(--label);font-size:20px;font-weight:600;cursor:pointer;">−</button><button type="button" onclick="qormBadge(this,1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:20px;font-weight:600;cursor:pointer;">+</button></div>`)
	r.sb.WriteString(`</div>`)
}

// notify renders a button that fires a native OS notification (desktop) or the
// Web Notification API (browser). Title/body come from the node.
func (r *renderer) notify(n *model.Node) {
	title := n.Placeholder
	if title == "" {
		title = "QORM"
	}
	body := n.Text
	if body == "" {
		body = "Hello from your QORM app \U0001f44b"
	}
	label := n.Label
	if label == "" {
		label = "\U0001f514 Send Notification"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-notify" data-title=%q data-body=%q style=%q>`, n.ID, title, body, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-notify-out" style="font-size:15px;color:var(--label);min-height:20px;">%s</div>`, n.ID, "")
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormNotify(this)" style="padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(label))
	r.sb.WriteString(`</div>`)
}

// hwAdjust renders a hardware widget with -/+ buttons and a live readout
// (volume, brightness).
func (r *renderer) hwAdjust(n *model.Node, kind, jsFn string) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-%s" style=%q>`, n.ID, kind, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-%s-out" style="font-size:15px;color:var(--label);min-height:20px;">—</div>`, n.ID, kind)
	fmt.Fprintf(&r.sb, `<div style="display:flex;gap:8px;"><button type="button" onclick="%s(this,-1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--fill);color:var(--label);font-size:20px;font-weight:600;cursor:pointer;">−</button><button type="button" onclick="%s(this,1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:20px;font-weight:600;cursor:pointer;">+</button></div>`, jsFn, jsFn)
	r.sb.WriteString(`</div>`)
}

// hwList renders a hardware widget that lists results (bluetooth scan, wifi
// info): a scrolling output area + a scan button wired to a native bridge op.
func (r *renderer) hwList(n *model.Node, kind, jsFn, defLabel string) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-%s" style=%q>`, n.ID, kind, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-%s-out" style="font-size:14px;color:var(--label);min-height:20px;white-space:pre-line;font-family:ui-monospace,Menlo,monospace;">—</div>`, n.ID, kind)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="%s(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`,
		jsFn, html.EscapeString(propStrOr(n, "label", defLabel)))
	r.sb.WriteString(`</div>`)
}

// biometric triggers Face ID / Touch ID / fingerprint via the native bridge
// and shows the result; the outcome is synced into bound state.
func (r *renderer) biometric(n *model.Node) {
	path := boundPath(n.Value)
	val := r.interp(n.Value)
	out := val
	if out == "" {
		out = "Not authenticated"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-biometric" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-bio-out" style="font-size:15px;color:var(--label);min-height:20px;">%s</div>`, n.ID, html.EscapeString(out))
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), val)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormBio(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(propStrOr(n, "label", "Authenticate")))
	r.sb.WriteString(`</div>`)
}

// location reads the device GPS (navigator.geolocation): a button fetches the
// current position, shows it, and syncs "lat, lng" into the bound state.
func (r *renderer) location(n *model.Node) {
	path := boundPath(n.Value)
	val := r.interp(n.Value)
	label := propStrOr(n, "label", "\U0001F4CD Get Location")
	out := val
	if out == "" {
		out = "Location not set"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-location" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-loc-out" style="font-size:15px;color:var(--label);min-height:20px;">%s</div>`, n.ID, html.EscapeString(out))
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), val)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormGeo(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(label))
	r.sb.WriteString(`</div>`)
}

// sensors streams device orientation (accelerometer/gyroscope) live. On iOS it
// requests DeviceOrientation permission on the enable tap.
func (r *renderer) sensors(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-motion" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-motion-out" style="font-family:ui-monospace,Menlo,monospace;font-size:15px;color:var(--label);min-height:20px;">tilt: —</div>`, n.ID)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormMotion(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(propStrOr(n, "label", "\U0001F9ED Enable Motion")))
	r.sb.WriteString(`</div>`)
}

// recorder captures audio (getUserMedia + MediaRecorder): a record/stop toggle,
// an inline player, and the recording synced (as a data URL) into bound state.
func (r *renderer) recorder(n *model.Node) {
	path := boundPath(n.Value)
	val := r.interp(n.Value)
	disp := "none"
	if val != "" {
		disp = "block"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-recorder" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<audio class="qorm-rec-audio" controls src=%q style="width:100%%;display:%s;"></audio>`, val, disp)
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), val)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormRec(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--danger);color:#fff;font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(propStrOr(n, "label", "\U0001F3A4 Record")))
	r.sb.WriteString(`</div>`)
}

// camera captures a photo via the device camera (WebView getUserMedia/file
// capture): a preview image + a shutter button. The captured image is a data
// URL synced into the bound state, so it can be shown or POSTed to a backend.
func (r *renderer) camera(n *model.Node) {
	val := r.interp(n.Value)
	path := boundPath(n.Value)
	label := propStrOr(n, "label", "\U0001F4F7 Take Photo")
	hattr := ""
	if n.OnChange != nil {
		hattr = fmt.Sprintf(` data-h="%d"`, r.register(n.OnChange))
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-camera"%s style=%q>`, n.ID, hattr,
		r.boxCSS(n)+"display:flex;flex-direction:column;gap:10px;align-items:stretch;")
	disp := "none"
	if val != "" {
		disp = "block"
	}
	fmt.Fprintf(&r.sb, `<img class="qorm-cam-preview" alt="" src=%q style="max-width:100%%;border-radius:12px;display:%s;">`, val, disp)
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), val)
	// Live camera (desktop/web via getUserMedia — localhost is a secure context);
	// hidden until qormCameraInit shows the live button on capable platforms.
	r.sb.WriteString(`<video class="qorm-cam-video" playsinline muted style="display:none;max-width:100%;border-radius:12px;"></video>`)
	fmt.Fprintf(&r.sb, `<button type="button" class="qorm-cam-live" style="display:none;text-align:center;padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;" onclick="qormCameraLive(this)">%s</button>`, html.EscapeString(label))
	// A <label> wrapping the file input triggers the camera natively — the most
	// reliable path on iOS (a hidden input + programmatic .click() is blocked).
	fmt.Fprintf(&r.sb, `<label class="qorm-cam-file" style="display:inline-block;text-align:center;padding:12px 16px;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s<input type="file" accept="image/*" capture="environment" style="position:absolute;width:1px;height:1px;opacity:0;" onchange="qormCamera(this)"></label>`,
		html.EscapeString(label))
	r.sb.WriteString(`</div>`)
}

// datepicker is a Cupertino-style 3-wheel date picker (month / day / year).
// Each wheel item, when clicked, dispatches onChange with the full recomposed
// date (keeping the other two wheels' current values), so it works with the
// standard onChange mechanism without extra JS.
func (r *renderer) datepicker(n *model.Node) {
	y, m, d := parseDate3(r.interp(n.Value))
	minY := int(propNum(n, "minYear", 2020))
	maxY := int(propNum(n, "maxYear", 2035))
	if maxY < minY {
		maxY = minY
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;display:flex;")
	// shared center selection band + top/bottom fades (iOS look)
	r.sb.WriteString(`<div style="position:absolute;left:6px;right:6px;top:72px;height:36px;background:var(--fill);border-radius:8px;pointer-events:none;z-index:0;"></div>`)
	r.sb.WriteString(`<div style="position:absolute;inset:0;pointer-events:none;z-index:2;background:linear-gradient(var(--surface),transparent 30%,transparent 70%,var(--surface));"></div>`)
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	// month wheel (flex 1.2 — wider for the label)
	mopts := make([]dwItem, 12)
	for i := 0; i < 12; i++ {
		mopts[i] = dwItem{label: months[i], value: fmtDate(y, i+1, d)}
	}
	r.dateWheel(n, mopts, m-1, "1.3")
	// day wheel 1..31
	dopts := make([]dwItem, 31)
	for i := 0; i < 31; i++ {
		dopts[i] = dwItem{label: strconv.Itoa(i + 1), value: fmtDate(y, m, i+1)}
	}
	r.dateWheel(n, dopts, d-1, "0.7")
	// year wheel minY..maxY
	yopts := make([]dwItem, 0, maxY-minY+1)
	for yr := minY; yr <= maxY; yr++ {
		yopts = append(yopts, dwItem{label: strconv.Itoa(yr), value: fmtDate(yr, m, d)})
	}
	r.dateWheel(n, yopts, y-minY, "1")
	r.sb.WriteString(`</div>`)
}

type dwItem struct{ label, value string }

// dateWheel renders one scroll-snap column of a datepicker.
func (r *renderer) dateWheel(n *model.Node, opts []dwItem, sel int, grow string) {
	fmt.Fprintf(&r.sb, `<div style="flex:%s;height:100%%;overflow-y:auto;scroll-snap-type:y mandatory;padding:72px 0;position:relative;z-index:1;scrollbar-width:none;">`, grow)
	for i, o := range opts {
		attr := ""
		if n.OnChange != nil {
			args := map[string]string{"value": o.value}
			for k, v := range n.OnChange.Args {
				if k != "value" {
					args[k] = v
				}
			}
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: args}))
		}
		weight, col := "400", "var(--label2)"
		if i == sel {
			weight, col = "600", "var(--label)"
		}
		fmt.Fprintf(&r.sb, `<div style="height:36px;display:flex;align-items:center;justify-content:center;scroll-snap-align:center;font-size:19px;font-weight:%s;color:%s;cursor:pointer;"%s>%s</div>`,
			weight, col, attr, html.EscapeString(o.label))
	}
	r.sb.WriteString(`</div>`)
}

// parseDate3 parses "YYYY-MM-DD" into ints, with sane fallbacks.
func parseDate3(s string) (y, m, d int) {
	y, m, d = 2026, 7, 1
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) == 3 {
		if v, err := strconv.Atoi(parts[0]); err == nil && v > 0 {
			y = v
		}
		if v, err := strconv.Atoi(parts[1]); err == nil && v >= 1 && v <= 12 {
			m = v
		}
		if v, err := strconv.Atoi(parts[2]); err == nil && v >= 1 && v <= 31 {
			d = v
		}
	}
	return
}

func fmtDate(y, m, d int) string { return fmt.Sprintf("%04d-%02d-%02d", y, m, d) }

func (r *renderer) picker(n *model.Node) {
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;")
	// center selection band
	r.sb.WriteString(`<div style="position:absolute;left:0;right:0;top:72px;height:36px;background:var(--fill);border-radius:8px;pointer-events:none;"></div>`)
	r.sb.WriteString(`<div style="height:100%;overflow-y:auto;scroll-snap-type:y mandatory;padding:72px 0;">`)
	for _, o := range optionList(n.Props["options"]) {
		attr := ""
		if n.OnChange != nil {
			args := map[string]string{"value": o.value}
			for k, v := range n.OnChange.Args {
				if k != "value" {
					args[k] = v
				}
			}
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: args}))
		}
		weight := "400"
		col := "var(--label)"
		if o.value == cur {
			weight = "600"
		} else {
			col = "var(--label2)"
		}
		fmt.Fprintf(&r.sb, `<div style="height:36px;display:flex;align-items:center;justify-content:center;scroll-snap-align:center;font-size:20px;font-weight:%s;color:%s;cursor:pointer;"%s>%s</div>`,
			weight, col, attr, html.EscapeString(o.label))
	}
	r.sb.WriteString(`</div></div>`)
}

// dismissible is Flutter's Dismissible: swipe the content left to reveal a
// destructive background and, past threshold, collapse the row and dispatch
// onDismissed (wired via the qormSwipe client helper).
func (r *renderer) dismissible(n *model.Node) {
	h := -1
	if n.OnPress != nil {
		h = r.register(n.OnPress)
	} else if d := parseInvokeProp(n, "onDismissed"); d != nil {
		h = r.register(d)
	}
	icon := propStrOr(n, "icon", "trash")
	iconHTML := html.EscapeString(icon)
	if svg := iconSVG(icon, 20); svg != "" {
		iconHTML = svg
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, r.nid(n), r.boxCSS(n)+"position:relative;transition:height .2s,opacity .2s;")
	fmt.Fprintf(&r.sb, `<div style="position:absolute;inset:0;background:var(--danger);display:flex;align-items:center;justify-content:flex-end;padding:0 22px;color:#fff;font-size:20px;">%s</div>`, iconHTML)
	r.sb.WriteString(`<div class="qorm-dismiss-content" style="position:relative;background:var(--surface);touch-action:pan-y;">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
	if h >= 0 {
		fmt.Fprintf(&r.sb, `<script>qormSwipe(document.getElementById(%q),%d)</script>`, r.nid(n), h)
	}
}

// contextMenu is Cupertino's CupertinoContextMenu: long-press the child to open
// an overlay of actions (each dispatches its onPress; destructive = red).
// ctxItemCSS styles one right-click menu row.
const ctxItemCSS = "display:flex;align-items:center;gap:9px;width:100%;padding:7px 10px;background:none;border:none;border-radius:7px;text-align:left;font-size:13px;color:var(--label);cursor:pointer;white-space:nowrap;"

// ctxItems builds the right-click menu rows (recursive for submenus).
func (r *renderer) ctxItems(items []any) {
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if b, _ := m["separator"].(bool); b {
			r.sb.WriteString(`<div style="height:.5px;background:var(--sep);margin:5px 8px;"></div>`)
			continue
		}
		id, _ := m["id"].(string)
		title, _ := m["title"].(string)
		icon, _ := m["icon"].(string)
		iconHTML := `<span style="width:18px;"></span>`
		if icon != "" {
			if svg := iconSVG(icon, 15); svg != "" {
				iconHTML = `<span style="width:18px;display:inline-flex;justify-content:center;color:var(--label2);">` + svg + `</span>`
			}
		}
		if sub, ok := m["items"].([]any); ok && len(sub) > 0 {
			r.sb.WriteString(`<div class="qorm-ctxmenu-sub" style="position:relative;">`)
			fmt.Fprintf(&r.sb, `<button class="qorm-ctxmenu-item qorm-tap" style="%s">%s<span style="flex:1;">%s</span>%s</button>`,
				ctxItemCSS, iconHTML, html.EscapeString(title), iconSVG("chevron-right", 13))
			r.sb.WriteString(`<div class="qorm-ctxmenu-panel qorm-ctxmenu-subpanel" style="display:none;position:absolute;left:100%;top:-6px;min-width:180px;background:var(--surface);border-radius:12px;box-shadow:0 10px 40px rgba(0,0,0,.28);padding:6px;border:.5px solid var(--sep);z-index:81;">`)
			r.ctxItems(sub)
			r.sb.WriteString(`</div></div>`)
			continue
		}
		fmt.Fprintf(&r.sb, `<button class="qorm-ctxmenu-item qorm-tap" data-id=%q style="%s">%s<span style="flex:1;">%s</span></button>`,
			id, ctxItemCSS, iconHTML, html.EscapeString(title))
	}
}

func (r *renderer) contextMenu(n *model.Node) {
	// Desktop right-click menu (items): positioned at the cursor, icons +
	// submenus, selection fires qormEmit('context', {id}).
	if raw, ok := n.Prop("items"); ok {
		if items, ok := raw.([]any); ok && len(items) > 0 {
			fmt.Fprintf(&r.sb, `<div id=%q class="qorm-ctxmenu" style=%q>`, r.nid(n), r.boxCSS(n)+"position:relative;")
			for _, c := range n.Children {
				r.node(c)
			}
			panelStyle := "display:none;position:fixed;z-index:80;min-width:200px;background:var(--surface);border-radius:12px;box-shadow:0 10px 40px rgba(0,0,0,.28);padding:6px;border:.5px solid var(--sep);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);" + propStr(n, "menuStyle")
			r.sb.WriteString(`<div class="qorm-ctxmenu-panel" style="` + panelStyle + `">`)
			r.ctxItems(items)
			r.sb.WriteString(`</div></div>`)
			return
		}
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-ctx" style=%q>`, r.nid(n), r.boxCSS(n)+"position:relative;")
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`<div class="qorm-ctx-panel" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,.28);z-index:70;align-items:center;justify-content:center;" onclick="this.style.display='none'">`)
	r.sb.WriteString(`<div style="min-width:220px;background:var(--surface);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-radius:14px;overflow:hidden;">`)
	for i, a := range r.dialogActions(n, "actions") {
		sep := ""
		if i > 0 {
			sep = "border-top:.5px solid var(--sep);"
		}
		attr := ""
		if a.inv != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(a.inv))
		}
		fmt.Fprintf(&r.sb, `<button style="width:100%%;padding:14px 16px;background:none;border:none;%stext-align:left;font-size:17px;color:%s;cursor:pointer;"%s>%s</button>`,
			sep, r.actionColor(a.style), attr, html.EscapeString(a.label))
	}
	r.sb.WriteString(`</div></div></div>`)
	fmt.Fprintf(&r.sb, `<script>qormCtx(document.getElementById(%q))</script>`, r.nid(n))
}

// refreshIndicator is Flutter's RefreshIndicator: pull the scroll content down
// past a threshold to dispatch onRefresh (via the qormRefresh helper).
func (r *renderer) refreshIndicator(n *model.Node) {
	h := -1
	if d := parseInvokeProp(n, "onRefresh"); d != nil {
		h = r.register(d)
	} else if n.OnPress != nil {
		h = r.register(n.OnPress)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, r.nid(n), r.boxCSS(n)+"overflow-y:auto;overscroll-behavior:contain;")
	r.sb.WriteString(`<div class="qorm-refresh-spin" style="height:0;opacity:0;display:flex;align-items:center;justify-content:center;overflow:hidden;transition:height .2s;"><span class="qorm-activity"><svg width="20" height="20" viewBox="0 0 20 20">`)
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&r.sb, `<rect x="9" y="2" width="2" height="5" rx="1" fill="var(--label2)" opacity="%g" transform="rotate(%d 10 10)"/>`, 0.25+0.75*float64(i)/7, i*45)
	}
	r.sb.WriteString(`</svg></span></div>`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	if h >= 0 {
		fmt.Fprintf(&r.sb, `<script>qormRefresh(document.getElementById(%q),%d)</script>`, r.nid(n), h)
	}
}

// animatedContainer is Flutter's AnimatedContainer (and AnimatedPadding/Align/
// Positioned): a container whose style transitions smoothly whenever a bound
// style value changes — so an agent flipping state animates in the live session.
func (r *renderer) animatedContainer(n *model.Node) {
	dur := propNum(n, "duration", 300)
	curve := propStrOr(n, "curve", "cubic-bezier(.4,0,.2,1)")
	trans := fmt.Sprintf("transition:all %gms %s;", dur, curve)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+trans, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// animatedOpacity is Flutter's AnimatedOpacity: fades children to the bound
// `opacity` (0..1) over `duration`.
func (r *renderer) animatedOpacity(n *model.Node) {
	dur := propNum(n, "duration", 300)
	op := 1.0
	if v := propStr(n, "opacity"); v != "" {
		op = asFloat(runtime.EvalBinding(v, r.ctx()))
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID,
		r.boxCSS(n)+fmt.Sprintf("opacity:%g;transition:opacity %gms cubic-bezier(.4,0,.2,1);", op, dur))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// motionKeyframe maps a widget type / effect name to a keyframe in the catalog.
var motionKeyframe = map[string]string{
	"fade": "qa-fade", "fadeup": "qa-fadeup", "fadedown": "qa-fadedown",
	"slideup": "qa-slideup", "slidedown": "qa-slidedown", "slideleft": "qa-slideleft", "slideright": "qa-slideright",
	"scale": "qa-scale", "zoomout": "qa-zoomout", "rotate": "qa-rotate", "flip": "qa-flip",
	"pop": "qa-pop", "bounce": "qa-bounce", "shake": "qa-shake", "pulse": "qa-pulse", "spin": "qa-spin", "size": "qa-size",
	// Flutter transition widgets → sensible default effect
	"fadetransition": "qa-fade", "slidetransition": "qa-slideup", "scaletransition": "qa-scale",
	"rotationtransition": "qa-rotate", "sizetransition": "qa-size", "hero": "qa-scale",
	"animatedswitcher": "qa-fade", "transition": "qa-fade", "animated": "qa-fade", "motion": "qa-fade",
}

// motion plays a named entrance/attention animation on its children. The effect
// comes from the `animation` prop (bindable — so an agent switches the whole
// implementation by changing state) or is derived from the widget type. Since a
// server re-render remounts the subtree, changing state replays the animation
// live. Covers Flutter's Fade/Slide/Scale/Rotation/Size transitions, Hero and
// AnimatedSwitcher, plus attention effects (bounce/shake/pulse).
func (r *renderer) motion(n *model.Node) {
	effect := r.interp(propStr(n, "animation"))
	if effect == "" {
		effect = n.Type
	}
	kf := motionKeyframe[strings.ToLower(effect)]
	if kf == "" {
		kf = "qa-fade"
	}
	dur := propNum(n, "duration", 450)
	delay := propNum(n, "delay", 0)
	curve := propStrOr(n, "curve", "cubic-bezier(.34,1.2,.64,1)")
	repeat := propStrOr(n, "repeat", "1")
	anim := fmt.Sprintf("animation:%s %gms %s %gms %s both;", kf, dur, curve, delay, repeat)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+anim, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// transform is Flutter's Transform (and RotatedBox): applies rotate/scale/
// translate/skew to its children. Every value is bindable, so an agent can spin
// or scale a widget by setting state — smooth when paired with a transition.
func (r *renderer) transform(n *model.Node) {
	var parts []string
	if v := r.numProp(n, "rotate"); v != nil {
		parts = append(parts, fmt.Sprintf("rotate(%gdeg)", *v))
	}
	if v := r.numProp(n, "scale"); v != nil {
		parts = append(parts, fmt.Sprintf("scale(%g)", *v))
	}
	if v := r.numProp(n, "scaleX"); v != nil {
		parts = append(parts, fmt.Sprintf("scaleX(%g)", *v))
	}
	if v := r.numProp(n, "scaleY"); v != nil {
		parts = append(parts, fmt.Sprintf("scaleY(%g)", *v))
	}
	if x, y := r.numProp(n, "translateX"), r.numProp(n, "translateY"); x != nil || y != nil {
		xv, yv := 0.0, 0.0
		if x != nil {
			xv = *x
		}
		if y != nil {
			yv = *y
		}
		parts = append(parts, fmt.Sprintf("translate(%gpx,%gpx)", xv, yv))
	}
	if v := r.numProp(n, "skew"); v != nil {
		parts = append(parts, fmt.Sprintf("skew(%gdeg)", *v))
	}
	tf := ""
	if len(parts) > 0 {
		tf = "transform:" + strings.Join(parts, " ") + ";transform-origin:center;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+tf, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// numProp evaluates a numeric prop that may be a literal or a {{ }} binding;
// returns nil when absent.
func (r *renderer) numProp(n *model.Node, key string) *float64 {
	raw, ok := n.Prop(key)
	if !ok {
		return nil
	}
	var f float64
	switch t := raw.(type) {
	case float64:
		f = t
	case string:
		if t == "" {
			return nil
		}
		f = asFloat(runtime.EvalBinding(t, r.ctx()))
	default:
		return nil
	}
	return &f
}

// aspectRatio is Flutter's AspectRatio: constrains children to a width:height
// ratio.
func (r *renderer) aspectRatio(n *model.Node) {
	ratio := propNum(n, "ratio", 1)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+fmt.Sprintf("aspect-ratio:%g;overflow:hidden;", ratio))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// richText is Flutter's RichText / Text.rich: a paragraph of styled spans
// ([{text, color, fontSize, fontWeight, italic, underline}]).
func (r *renderer) richText(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"line-height:1.5;")
	spans, _ := n.Prop("spans")
	arr, _ := spans.([]any)
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		var st strings.Builder
		if c := str(m, "color"); c != "" {
			fmt.Fprintf(&st, "color:%s;", c)
		}
		if fs, ok := numOK(m, "fontSize"); ok {
			fmt.Fprintf(&st, "font-size:%gpx;", fs)
		}
		if fw, ok := numOK(m, "fontWeight"); ok {
			fmt.Fprintf(&st, "font-weight:%g;", fw)
		}
		if b, _ := m["italic"].(bool); b {
			st.WriteString("font-style:italic;")
		}
		if b, _ := m["underline"].(bool); b {
			st.WriteString("text-decoration:underline;")
		}
		fmt.Fprintf(&r.sb, `<span style="%s">%s</span>`, st.String(), html.EscapeString(r.interp(str(m, "text"))))
	}
	r.sb.WriteString(`</div>`)
}

// largeTitle is the iOS large-title navigation bar (CupertinoSliverNavigationBar):
// a compact bar row over a big bold title, translucent with a hairline.
func (r *renderer) largeTitle(n *model.Node) {
	bg := propStrOr(n, "background", "var(--bg)")
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID,
		r.boxCSS(n)+fmt.Sprintf("background:%s;-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-bottom:.5px solid var(--sep);", bg))
	// compact action row
	r.sb.WriteString(`<div style="display:flex;align-items:center;justify-content:flex-end;gap:6px;height:36px;padding:0 12px;color:var(--accent);">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	// large title
	fmt.Fprintf(&r.sb, `<div style="padding:0 16px 10px;font-size:34px;font-weight:700;letter-spacing:-.02em;color:var(--label);">%s</div>`,
		html.EscapeString(r.interp(labelOf(n))))
	if sub := r.interp(propStr(n, "subtitle")); sub != "" {
		fmt.Fprintf(&r.sb, `<div style="padding:0 16px 10px;font-size:15px;color:var(--label2);margin-top:-6px;">%s</div>`, html.EscapeString(sub))
	}
	r.sb.WriteString(`</div>`)
}

// navigationRail is Flutter's NavigationRail: a vertical destination bar for
// wide (desktop) layouts; tapping dispatches onChange with {value}.
func (r *renderer) navigationRail(n *model.Node) {
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID,
		r.boxCSS(n)+"display:flex;flex-direction:column;align-items:stretch;gap:4px;padding:12px 8px;border-right:.5px solid var(--sep);background:var(--surface);")
	for _, it := range r.boundArray(n, "items") {
		obj, _ := it.(map[string]any)
		if obj == nil {
			continue
		}
		val := fmt.Sprint(obj["value"])
		active := val == cur
		col, bg := "var(--label2)", "transparent"
		if active {
			col, bg = "var(--accent)", "color-mix(in srgb,var(--accent) 12%, transparent)"
		}
		attr := ""
		if n.OnChange != nil {
			args := map[string]string{"value": val}
			for k, v := range n.OnChange.Args {
				if k != "value" {
					args[k] = v
				}
			}
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: args}))
		}
		fmt.Fprintf(&r.sb, `<button style="display:flex;flex-direction:column;align-items:center;gap:3px;padding:10px 6px;border:none;border-radius:10px;cursor:pointer;background:%s;color:%s;"%s>`, bg, col, attr)
		fmt.Fprintf(&r.sb, `<span style="font-size:20px;display:inline-flex;align-items:center;">%s</span><span style="font-size:11px;">%s</span></button>`,
			iconOrText(fmt.Sprint(obj["icon"]), 20), html.EscapeString(fmt.Sprint(obj["label"])))
	}
	r.sb.WriteString(`</div>`)
}

// selectableText is Flutter's SelectableText: text the user can select/copy.
func (r *renderer) selectableText(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>%s</div>`, n.ID,
		r.boxCSS(n)+r.textCSS(n)+"user-select:text;-webkit-user-select:text;cursor:text;",
		html.EscapeString(r.interp(n.Text)))
}
