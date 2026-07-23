package render

import (
	"fmt"
	"html"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// snackbar is Flutter's SnackBar: a transient bottom banner shown when `open`.
func (r *renderer) snackbar(n *model.Node) {
	if o := propStr(n, "open"); o != "" {
		if v := r.interp(o); v == "" || v == "false" || v == "0" {
			return
		}
	}
	style := r.boxCSS(n) + "position:fixed;left:50%;bottom:20px;transform:translateX(-50%);display:flex;align-items:center;gap:16px;background:#323232;color:#fff;padding:12px 18px;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,.3);z-index:60;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="status">`, attrID(n.ID), style)
	fmt.Fprintf(&r.sb, `<span style="font-size:14px;">%s</span>`, html.EscapeString(r.interp(labelOf(n))))
	if act := r.interp(propStr(n, "action")); act != "" {
		fmt.Fprintf(&r.sb, `<button style="background:none;border:none;color:#7cc0ff;font-weight:600;cursor:pointer;"%s>%s</button>`,
			r.pressAttr(n), html.EscapeString(act))
	}
	r.sb.WriteString(`</div>`)
}

func (r *renderer) badge(n *model.Node) {
	label := r.interp(labelOf(n))
	// Flutter Badge(child): with children, render a corner count/dot over the
	// wrapped child; a "0"/empty count is hidden unless showZero.
	if len(n.Children) > 0 {
		fmt.Fprintf(&r.sb, `<span id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;display:inline-flex;")
		for _, c := range n.Children {
			r.node(c)
		}
		if label != "" && !(label == "0" && propStr(n, "showZero") != "true") {
			dot := "min-width:18px;height:18px;padding:0 5px;border-radius:9px;font-size:11px;"
			if propStr(n, "smallSize") == "true" {
				dot = "width:8px;height:8px;border-radius:4px;"
			}
			// colour is an author prop interpolated into a quoted style attribute.
			bg := styleAttr(propStrOr(n, "color", "#ef4444"))
			fmt.Fprintf(&r.sb, `<span style="position:absolute;top:-6px;right:-6px;display:inline-flex;align-items:center;justify-content:center;background:%s;color:#fff;font-weight:700;box-shadow:0 0 0 2px var(--surface);%s">%s</span>`,
				bg, dot, html.EscapeString(label))
		}
		r.sb.WriteString(`</span>`)
		return
	}
	style := r.boxCSS(n) + r.textCSS(n) +
		"display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;font-size:12px;font-weight:600;background:var(--fill);color:var(--label2);"
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s>%s</span>`, attrID(n.ID), style, a11y(n), html.EscapeString(label))
}

func (r *renderer) progress(n *model.Node) {
	v := asFloat(runtime.EvalBinding(n.Value, r.ctx()))
	if v > 0 && v <= 1 { // accept a 0..1 fraction as well as a 0..100 percentage
		v *= 100
	}
	pct := clampPct(v)
	// colour is an author prop interpolated into a quoted style attribute.
	fill := styleAttr(propStrOr(n, "color", "var(--accent)"))
	track := r.boxCSS(n) + "background:var(--fill);overflow:hidden;border-radius:999px;min-height:8px;width:100%;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="progressbar"><div style="width:%g%%;height:100%%;background:%s;transition:width .2s;"></div></div>`,
		attrID(n.ID), track, pct, fill)
}

func (r *renderer) spinner(n *model.Node) {
	size := propNum(n, "size", 24)
	// colour is an author prop interpolated into a quoted style attribute.
	color := styleAttr(propStrOr(n, "color", "var(--accent)"))
	style := fmt.Sprintf("width:%gpx;height:%gpx;border:3px solid var(--sep);border-top-color:%s;border-radius:50%%;", size, size, color)
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-spin" style=%q role="status" aria-label="loading"></div>`, attrID(n.ID), r.boxCSS(n)+style)
}

// dismissH wires the default dismiss behavior of overlay widgets: when `open`
// is a pure {{state.x}} binding and the app hasn't opted out (dismissable:false),
// it registers the runtime's built-in __dismiss action against that path — so
// backdrop taps, Escape and un-wired cancel buttons close the overlay with no
// app-authored action. An explicit onPress anywhere always wins over this.
func (r *renderer) dismissH(n *model.Node) (int, bool) {
	if v, ok := n.Prop("dismissable"); ok && !asBool(v) {
		return 0, false
	}
	bp := boundPath(propStr(n, "open"))
	if bp == "" {
		return 0, false
	}
	return r.register(&model.Invoke{Name: runtime.BuiltinDismiss, Args: map[string]string{"path": bp}}), true
}

// modal renders an overlay dialog when its `open` binding is truthy.
func (r *renderer) modal(n *model.Node) {
	if !asBool(runtime.EvalBinding(propStr(n, "open"), r.ctx())) {
		return
	}
	overlay := "position:fixed;inset:0;background:rgba(0,0,0,.45);display:flex;align-items:center;justify-content:center;z-index:50;padding:20px;"
	panel := r.boxCSS(n) + "background:var(--surface);border-radius:14px;box-shadow:0 20px 60px rgba(0,0,0,.3);width:min(92vw,560px);max-height:90%;overflow:auto;padding:20px;display:flex;flex-direction:column;gap:12px;"
	backdrop, esc := "", ""
	if h, ok := r.dismissH(n); ok {
		backdrop = fmt.Sprintf(` onclick="if(event.target===this)qorm(%d)"`, h)
		esc = fmt.Sprintf(` data-dismiss-h="%d"`, h)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="dialog" aria-modal="true"%s%s><div style=%q>`, attrID(n.ID), overlay, backdrop, esc, panel)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="alert"><span>%s</span><div style="display:flex;flex-direction:column;gap:2px;">`, attrID(n.ID), style, icon)
	if t := r.interp(propStr(n, "title")); t != "" {
		fmt.Fprintf(&r.sb, `<div style="font-weight:700;">%s</div>`, html.EscapeString(t))
	}
	fmt.Fprintf(&r.sb, `<div>%s</div></div></div>`, html.EscapeString(r.interp(labelOf(n))))
}

// tag renders a pill/chip, optionally removable.
func (r *renderer) tag(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n) + "display:inline-flex;align-items:center;gap:6px;padding:2px 10px;border-radius:999px;background:var(--fill);color:var(--label2);font-size:13px;"
	fmt.Fprintf(&r.sb, `<span id=%q style=%q>%s`, attrID(n.ID), style, html.EscapeString(r.interp(labelOf(n))))
	if n.OnPress != nil { // acts as remove
		fmt.Fprintf(&r.sb, `<button onclick="qorm(%d)" style="border:none;background:none;cursor:pointer;color:inherit;font-size:14px;line-height:1;">×</button>`, r.register(n.OnPress))
	}
	r.sb.WriteString(`</span>`)
}

// skeleton renders a shimmering loading placeholder.
func (r *renderer) skeleton(n *model.Node) {
	style := r.boxCSS(n) + "min-height:14px;border-radius:6px;"
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-skel" style=%q aria-hidden="true"></div>`, attrID(n.ID), style)
}

// menu renders a trigger label plus a client-toggled dropdown panel. An `items`
// prop ([{label, icon?, disabled?, onPress?}], parsed like dialogActions) renders
// action rows inside the panel: a row dispatches its onPress via the standard
// qorm(h) mechanism, while a disabled row renders dimmed with no handler. Icon
// names resolve against the built-in icon set. Children still render below the
// items, so a children-built menu keeps working.
func (r *renderer) menu(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-menu" style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;display:inline-block;")
	fmt.Fprintf(&r.sb, `<button onclick="qormMenu(this)" style="display:inline-flex;align-items:center;gap:6px;padding:8px 12px;border:1px solid var(--sep);border-radius:8px;background:var(--surface);cursor:pointer;font-size:14px;">%s ▾</button>`,
		html.EscapeString(r.interp(labelOf(n))))
	r.sb.WriteString(`<div class="qorm-menu-panel" style="display:none;position:absolute;top:100%;left:0;margin-top:4px;background:var(--surface);border:1px solid var(--sep);border-radius:8px;box-shadow:0 8px 24px rgba(0,0,0,.12);min-width:160px;z-index:40;padding:4px;">`)
	if raw, ok := n.Prop("items"); ok {
		if items, ok := raw.([]any); ok {
			r.menuItems(items)
		}
	}
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// menuItems renders the rows of a menu's `items` prop.
func (r *renderer) menuItems(items []any) {
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		disabled := asBool(m["disabled"])
		style := "display:flex;align-items:center;gap:8px;padding:8px 10px;border-radius:6px;font-size:14px;"
		attr := ""
		if disabled {
			style += "opacity:.45;cursor:default;"
		} else {
			style += "cursor:pointer;"
			if op, ok := m["onPress"].(map[string]any); ok {
				inv := &model.Invoke{Name: str(op, "name"), Args: map[string]string{}}
				if a, ok := op["args"].(map[string]any); ok {
					for k, v := range a {
						inv.Args[k] = fmt.Sprint(v)
					}
				}
				attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(inv))
			}
		}
		icon := `<span style="width:18px;"></span>`
		if svg := iconSVG(str(m, "icon"), 15); svg != "" {
			icon = `<span style="width:18px;display:inline-flex;justify-content:center;color:var(--label2);">` + svg + `</span>`
		}
		fmt.Fprintf(&r.sb, `<div style=%q%s>%s<span>%s</span></div>`, style, attr, icon, html.EscapeString(r.interp(str(m, "label"))))
	}
}

// circularProgress is Flutter's CircularProgressIndicator: an SVG ring. With a
// `value` (0..1) it is determinate (an arc); without, it spins indeterminately.
func (r *renderer) circularProgress(n *model.Node) {
	size := propNum(n, "size", 44)
	stroke := propNum(n, "stroke", 4)
	rad := (size - stroke) / 2
	circ := 2 * 3.14159265 * rad
	// colour is an author prop interpolated into a quoted SVG stroke
	// attribute: entity-encode the value (not the surrounding constant
	// markup) so a double quote cannot break out and inject attributes.
	color := html.EscapeString(propStrOr(n, "color", "var(--accent)"))
	cx := size / 2
	fmt.Fprintf(&r.sb, `<svg id=%q width="%g" height="%g" viewBox="0 0 %g %g" style=%q%s>`,
		attrID(n.ID), size, size, size, size, r.boxCSS(n), a11y(n))
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

// alertDialog is Cupertino's CupertinoAlertDialog: a centered rounded card with
// title, message and stacked/side-by-side action buttons. Shown while `open`.
func (r *renderer) alertDialog(n *model.Node) {
	if o := propStr(n, "open"); o != "" {
		if v := r.interp(o); v == "" || v == "false" || v == "0" {
			return
		}
	}
	r.sb.WriteString(`<div style="position:fixed;inset:0;background:rgba(0,0,0,.28);display:flex;align-items:center;justify-content:center;z-index:70;">`)
	fmt.Fprintf(&r.sb, `<div id=%q style="width:270px;background:var(--surface);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);border-radius:14px;overflow:hidden;text-align:center;">`, attrID(n.ID))
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
		} else if a.style == "cancel" {
			// an un-wired cancel button closes the dialog by default
			if h, ok := r.dismissH(n); ok {
				attr = fmt.Sprintf(` onclick="qorm(%d)"`, h)
			}
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
	backdrop, esc := "", ""
	if h, ok := r.dismissH(n); ok {
		backdrop = fmt.Sprintf(` onclick="if(event.target===this)qorm(%d)"`, h)
		esc = fmt.Sprintf(` data-dismiss-h="%d"`, h)
	}
	fmt.Fprintf(&r.sb, `<div class="qorm-sheet" style="position:fixed;inset:0;background:rgba(0,0,0,.28);display:flex;align-items:flex-end;justify-content:center;z-index:70;padding:8px;"%s%s>`, backdrop, esc)
	fmt.Fprintf(&r.sb, `<div id=%q style="width:100%%;max-width:400px;">`, attrID(n.ID))
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
	// separated cancel — without an onPress it closes the sheet by default
	if c := r.dialogActions(n, "cancel"); len(c) > 0 {
		attr := ""
		if c[0].inv != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(c[0].inv))
		} else if h, ok := r.dismissH(n); ok {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, h)
		}
		fmt.Fprintf(&r.sb, `<button style="width:100%%;margin-top:8px;padding:16px;background:var(--surface);border:none;border-radius:14px;font-size:20px;font-weight:600;color:var(--accent);cursor:pointer;"%s>%s</button>`,
			attr, html.EscapeString(c[0].label))
	}
	r.sb.WriteString(`</div></div>`)
}

// activityIndicator is Cupertino's CupertinoActivityIndicator: eight tapered
// spokes ticking around (the iOS spinner).
func (r *renderer) activityIndicator(n *model.Node) {
	size := propNum(n, "size", 20)
	fmt.Fprintf(&r.sb, `<span id=%q class="qorm-activity" style=%q><svg width="%g" height="%g" viewBox="0 0 20 20">`,
		attrID(n.ID), r.boxCSS(n), size, size)
	for i := 0; i < 8; i++ {
		op := 0.25 + 0.75*float64(i)/7
		fmt.Fprintf(&r.sb, `<rect x="9" y="2" width="2" height="5" rx="1" fill="var(--label2)" opacity="%g" transform="rotate(%d 10 10)"/>`, op, i*45)
	}
	r.sb.WriteString(`</svg></span>`)
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
		// Plain text: the OS renders the notification body, so neither the
		// framework's SVG icons nor emoji can appear here.
		body = "Hello from your QORM app"
	}
	// data-title/data-body are HTML attributes, so they need html.EscapeString
	// (not just %q, whose backslash quoting an HTML parser ignores — a raw quote
	// would terminate the attribute and let an untrusted title/body inject markup).
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-notify" data-title=%q data-body=%q style=%q>`, attrID(n.ID), html.EscapeString(title), html.EscapeString(body), r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-notify-out" style="font-size:15px;color:var(--label);min-height:20px;">%s</div>`, attrID(n.ID), "")
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormNotify(this)" style="padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, iconLabel(n.Label, "bell", "Send Notification"))
	r.sb.WriteString(`</div>`)
}
