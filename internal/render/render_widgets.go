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
	catalog  map[string]any
	baseCtx  map[string]any
	viewport map[string]any // viewport.* vars for responsive `when` conditions
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

func (r *renderer) text(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>%s</div>`, r.nid(n), style, a11y(n), html.EscapeString(r.interp(n.Text)))
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

// timepicker is a Cupertino-style 2-wheel time picker (hour / minute) — the
// time analogue of datepicker. Each wheel item, when clicked, dispatches
// onChange with the full recomposed "HH:MM" value (keeping the other wheel's
// current value), so it works with the standard onChange mechanism without
// extra JS. minuteStep (default 1) spaces the minute wheel.
func (r *renderer) timepicker(n *model.Node) {
	h, m := parseTime2(r.interp(n.Value))
	step := int(propNum(n, "minuteStep", 1))
	if step < 1 {
		step = 1
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID, r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;display:flex;")
	// shared center selection band + top/bottom fades (iOS look)
	r.sb.WriteString(`<div style="position:absolute;left:6px;right:6px;top:72px;height:36px;background:var(--fill);border-radius:8px;pointer-events:none;z-index:0;"></div>`)
	r.sb.WriteString(`<div style="position:absolute;inset:0;pointer-events:none;z-index:2;background:linear-gradient(var(--surface),transparent 30%,transparent 70%,var(--surface));"></div>`)
	// hour wheel 0..23
	hopts := make([]dwItem, 24)
	for i := 0; i < 24; i++ {
		hopts[i] = dwItem{label: fmt.Sprintf("%02d", i), value: fmtTime(i, m)}
	}
	r.dateWheel(n, hopts, h, "1")
	// minute wheel 0..59 in minuteStep increments
	mopts := make([]dwItem, 0, 60/step)
	for i := 0; i < 60; i += step {
		mopts = append(mopts, dwItem{label: fmt.Sprintf("%02d", i), value: fmtTime(h, i)})
	}
	r.dateWheel(n, mopts, m/step, "1")
	r.sb.WriteString(`</div>`)
}

// parseTime2 parses "HH:MM" into ints, with sane fallbacks.
func parseTime2(s string) (h, m int) {
	h, m = 9, 0
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) == 2 {
		if v, err := strconv.Atoi(parts[0]); err == nil && v >= 0 && v <= 23 {
			h = v
		}
		if v, err := strconv.Atoi(parts[1]); err == nil && v >= 0 && v <= 59 {
			m = v
		}
	}
	return
}

func fmtTime(h, m int) string { return fmt.Sprintf("%02d:%02d", h, m) }

func (r *renderer) picker(n *model.Node) {
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;", a11y(n))
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
	fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormCtx(document.getElementById(%q))})</script>`, r.nid(n))
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
		fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormRefresh(document.getElementById(%q),%d)})</script>`, r.nid(n), h)
	}
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

// backButton is Flutter/iOS BackButton: a leading chevron that pops the
// navigation stack. With no explicit onPress it drives the URL router via
// history.back() (→ popstate → /navigate), matching the browser Back button; an
// optional label (iOS-style "Back" text) sits next to the chevron.
func (r *renderer) backButton(n *model.Node) {
	glyph := `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg>`
	r.navButton(n, glyph, r.interp(labelOf(n)), "Back")
}

// closeButton is Flutter/iOS CloseButton: an "×" that dismisses. Like
// backButton it defaults to history.back() when no onPress is given.
func (r *renderer) closeButton(n *model.Node) {
	r.navButton(n, iconSVG("x", 22), "", "Close")
}

// navButton renders an icon-only nav affordance (back/close): a 44pt tap target
// whose default action is history.back(), overridable by onPress. aria names the
// button when the app supplies no ariaLabel, so the icon stays accessible.
func (r *renderer) navButton(n *model.Node, glyph, label, aria string) {
	onclick := ` onclick="history.back()"`
	if n.OnPress != nil {
		onclick = r.pressAttr(n)
	}
	al := a11y(n)
	if !strings.Contains(al, "aria-label") {
		al += fmt.Sprintf(` aria-label=%q`, aria)
	}
	style := r.boxCSS(n) + "display:inline-flex;align-items:center;gap:2px;min-width:44px;min-height:44px;padding:0 6px;border:none;background:none;cursor:pointer;color:var(--accent);font-size:17px;"
	fmt.Fprintf(&r.sb, `<button id=%q style=%q%s%s>%s`, r.nid(n), style, al, onclick, glyph)
	if label != "" {
		fmt.Fprintf(&r.sb, `<span>%s</span>`, html.EscapeString(label))
	}
	r.sb.WriteString(`</button>`)
}

// form is Flutter's Form / an HTML <form>: it groups input fields and fires its
// action on submit — Enter in a field or a native submit button — via the form's
// onPress (authored as the submit handler). Field-level validation stays
// declarative (each field's bound `error`), and the app gates submission by
// binding its submit button's disabled state; the form itself submits
// unconditionally. `return false` stops the browser's page reload.
func (r *renderer) form(n *model.Node) {
	submit := ` onsubmit="return false"`
	if n.OnPress != nil {
		submit = fmt.Sprintf(` onsubmit="qorm(%d);return false"`, r.register(n.OnPress))
	}
	fmt.Fprintf(&r.sb, `<form id=%q style=%q%s%s>`, r.nid(n), r.containerCSS(n), a11y(n), submit)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</form>`)
}

// offstage is Flutter's Offstage: it keeps its child in the tree but drops it
// from layout/paint when `offstage` is truthy (default true, matching Flutter).
// Unlike if/visible/show (which omit the node entirely), Offstage renders the
// subtree so its ids stay wired — useful for pre-mounting a to-be-revealed panel.
func (r *renderer) offstage(n *model.Node) {
	off := true
	if raw, ok := n.Prop("offstage"); ok {
		off = asBool(runtime.EvalBinding(fmt.Sprint(raw), r.ctx()))
	}
	style := r.boxCSS(n)
	if off {
		style += "display:none;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, r.nid(n), style, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// ignorePointer is Flutter's IgnorePointer/AbsorbPointer: the whole subtree is
// transparent to pointer events (taps pass through to whatever is beneath).
// display:contents keeps the wrapper out of layout, so children lay out exactly
// as if unwrapped; pointer-events:none then inherits down the DOM subtree.
func (r *renderer) ignorePointer(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, r.nid(n), r.boxCSS(n)+"display:contents;pointer-events:none;", a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// navigationDrawer is Material's NavigationDrawer: a vertical list of icon+label
// destinations bound to state; tapping dispatches onChange with {value}. Distinct
// from `drawer` (an overlay panel of arbitrary children) — this is the
// destination list itself, full-width pill rows with the active one highlighted.
func (r *renderer) navigationDrawer(n *model.Node) {
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="navigation">`, r.nid(n),
		r.boxCSS(n)+"display:flex;flex-direction:column;gap:2px;padding:12px;background:var(--surface);min-width:200px;")
	for _, it := range r.boundArray(n, "items") {
		obj, _ := it.(map[string]any)
		if obj == nil {
			continue
		}
		val := fmt.Sprint(obj["value"])
		col, bg := "var(--label)", "transparent"
		if val == cur {
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
		fmt.Fprintf(&r.sb, `<button style="display:flex;align-items:center;gap:12px;padding:12px 16px;border:none;border-radius:28px;cursor:pointer;text-align:left;background:%s;color:%s;font-size:14px;"%s>`, bg, col, attr)
		fmt.Fprintf(&r.sb, `<span style="display:inline-flex;align-items:center;">%s</span>%s</button>`,
			iconOrText(fmt.Sprint(obj["icon"]), 22), html.EscapeString(fmt.Sprint(obj["label"])))
	}
	r.sb.WriteString(`</div>`)
}

// bottomAppBar is Material's BottomAppBar: a bottom-pinned toolbar holding action
// children (icons/buttons), with a hairline top border like the iOS tab bar and
// the bottom safe-area inset applied.
func (r *renderer) bottomAppBar(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;align-items:center;gap:8px;padding:8px 12px;min-height:56px;background:var(--surface);border-top:.5px solid var(--sep);padding-bottom:calc(8px + var(--safe-bottom, env(safe-area-inset-bottom, 0px)));"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s role="toolbar">`, r.nid(n), style, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// limitedBox is Flutter's LimitedBox: it caps its child via maxWidth / maxHeight
// (px, read from style or props); a plain flow container otherwise.
func (r *renderer) limitedBox(n *model.Node) {
	lim := ""
	if w, ok := numOK(n.Style, "maxWidth"); ok {
		lim += fmt.Sprintf("max-width:%gpx;", w)
	} else if w := propNum(n, "maxWidth", -1); w >= 0 {
		lim += fmt.Sprintf("max-width:%gpx;", w)
	}
	if h, ok := numOK(n.Style, "maxHeight"); ok {
		lim += fmt.Sprintf("max-height:%gpx;", h)
	} else if h := propNum(n, "maxHeight", -1); h >= 0 {
		lim += fmt.Sprintf("max-height:%gpx;", h)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, r.nid(n), r.boxCSS(n)+lim, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// indexedStack is Flutter's IndexedStack: it mounts every child but paints only
// the one at `index` (bindable, default 0). Hidden children keep their DOM/ids
// and state — the reason to reach for this over swapping subtrees: a wizard step
// or tab body that must not lose its inputs when you flip away and back.
func (r *renderer) indexedStack(n *model.Node) {
	idx := 0
	if raw, ok := n.Prop("index"); ok {
		idx = int(asFloat(runtime.EvalBinding(fmt.Sprint(raw), r.ctx())))
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, r.nid(n), r.boxCSS(n)+"position:relative;", a11y(n))
	for i, c := range n.Children {
		disp := "display:none;"
		if i == idx {
			disp = ""
		}
		fmt.Fprintf(&r.sb, `<div style="%s">`, disp)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// selectableText is Flutter's SelectableText: text the user can select/copy.
func (r *renderer) selectableText(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>%s</div>`, n.ID,
		r.boxCSS(n)+r.textCSS(n)+"user-select:text;-webkit-user-select:text;cursor:text;",
		html.EscapeString(r.interp(n.Text)))
}
