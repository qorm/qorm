package render

import (
	"fmt"
	"html"

	"github.com/qorm/qorm/internal/model"
)

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
		initJS = fmt.Sprintf(`<script>setTimeout(function(){qormLong(document.getElementById(%q),%d)})</script>`, r.nid(n), r.register(lp))
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
		fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormSwipe(document.getElementById(%q),%d)})</script>`, r.nid(n), h)
	}
}
