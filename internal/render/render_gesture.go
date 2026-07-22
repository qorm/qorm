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
	out := val
	if out == "" {
		out = "Location not set"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-location" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-loc-out" style="font-size:15px;color:var(--label);min-height:20px;">%s</div>`, n.ID, html.EscapeString(out))
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), val)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormGeo(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, iconLabel(propStr(n, "label"), "location", "Get Location"))
	r.sb.WriteString(`</div>`)
}

// sensors streams device orientation (accelerometer/gyroscope) live. On iOS it
// requests DeviceOrientation permission on the enable tap.
func (r *renderer) sensors(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-motion" style=%q>`, n.ID, r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-motion-out" style="font-family:ui-monospace,Menlo,monospace;font-size:15px;color:var(--label);min-height:20px;">tilt: —</div>`, n.ID)
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormMotion(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, iconLabel(propStr(n, "label"), "compass", "Enable Motion"))
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
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormRec(this)" style="padding:12px 16px;border:none;border-radius:12px;background:var(--danger);color:#fff;font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, iconLabel(propStr(n, "label"), "mic", "Record"))
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

// draggable is Flutter's Draggable / LongPressDraggable: its child can be picked
// up and dropped onto a dragtarget, carrying the string payload in its `data`
// prop (bindable, exposed as data-qorm-drag). The receiving dragtarget's onDrop
// fires with that payload as {{ _dragData }}. Driven by pointer events (via a
// single delegated document handler, qormDragInit) so it works in the desktop
// WebView and on touch — HTML5 drag-and-drop is unreliable there.
func (r *renderer) draggable(n *model.Node) {
	data := r.interp(propStr(n, "data"))
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-draggable" data-qorm-drag=%q style=%q%s>`,
		r.nid(n), html.EscapeString(data), r.boxCSS(n)+"cursor:grab;touch-action:none;", a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	// idempotent: sets up the delegated document listener once (survives re-render
	// morphs, which don't re-run inline scripts). A closure defers the lookup to
	// fire time, so it works even if app.js loads after this inline script.
	r.sb.WriteString(`<script>setTimeout(function(){qormDragInit()})</script>`)
}

// dragTarget is Flutter's DragTarget: a drop zone (data-qorm-drop=<handler>) that
// dispatches onDrop (or onPress) with the dropped draggable's payload as
// {{ _dragData }}. It highlights (.qorm-dragover) while a drag hovers over it.
// No script of its own — the delegated draggable handler finds it by class.
func (r *renderer) dragTarget(n *model.Node) {
	h := -1
	if d := parseInvokeProp(n, "onDrop"); d != nil {
		h = r.register(d)
	} else if n.OnPress != nil {
		h = r.register(n.OnPress)
	}
	drop := ""
	if h >= 0 {
		drop = fmt.Sprintf(` data-qorm-drop="%d"`, h)
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-droptarget"%s style=%q%s>`, r.nid(n), drop, r.boxCSS(n), a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// swipeActions is a list row that reveals trailing action buttons on a left
// swipe (iOS Mail style): swipe to open, tap an action to fire it and close,
// tap the content or swipe back to close. actions: [{label,icon,color,name,args}].
func (r *renderer) swipeActions(n *model.Node) {
	id := r.nid(n)
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-swa" style=%q>`, id, r.boxCSS(n)+"position:relative;overflow:hidden;")
	r.sb.WriteString(`<div class="qorm-swa-actions" style="position:absolute;top:0;right:0;bottom:0;display:flex;">`)
	if raw, ok := n.Prop("actions"); ok {
		if arr, ok := raw.([]any); ok {
			for _, a := range arr {
				m, ok := a.(map[string]any)
				if !ok {
					continue
				}
				h := -1
				if name := str(m, "name"); name != "" {
					inv := &model.Invoke{Name: name, Args: map[string]string{}}
					if args, ok := m["args"].(map[string]any); ok {
						for k, v := range args {
							inv.Args[k] = fmt.Sprint(v)
						}
					}
					h = r.register(inv)
				}
				color := colorStr(m, "color")
				if color == "" {
					color = "var(--danger)"
				}
				inner := ""
				if icon := str(m, "icon"); icon != "" {
					if svg := iconSVG(icon, 20); svg != "" {
						inner = svg
					}
				}
				if label := str(m, "label"); label != "" {
					inner += `<span>` + html.EscapeString(label) + `</span>`
				}
				onclick := ""
				if h >= 0 {
					onclick = fmt.Sprintf(` onclick="qorm(%d)"`, h)
				}
				fmt.Fprintf(&r.sb, `<button class="qorm-swa-act qorm-tap" style="display:flex;flex-direction:column;align-items:center;justify-content:center;gap:3px;min-width:76px;border:none;cursor:pointer;background:%s;color:#fff;font-size:13px;font-weight:600;"%s>%s</button>`, color, onclick, inner)
			}
		}
	}
	r.sb.WriteString(`</div>`)
	r.sb.WriteString(`<div class="qorm-swa-content" style="position:relative;background:var(--surface);touch-action:pan-y;">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
	fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormSwipeActions(document.getElementById(%q))})</script>`, id)
}
