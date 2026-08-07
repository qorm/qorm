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

// RenderOpts carries engine-side state (camera, board pan/zoom, …) into
// the HTML renderer. The runtime itself is engine-free; the canvas
// engine owns its BoardState / Interaction on a different struct, and
// the server forwards the slice the HTML path needs at render time. An
// empty Board means "no board active" — children of a `type:"board"`
// root just render at their world coordinates, which is what a board
// without camera follow expects.
type RenderOpts struct {
	Board BoardRender
}

// BoardRender is the slice of canvas BoardState the HTML renderer
// reads (currently just pan + zoom). It mirrors the relevant fields of
// canvas.BoardState so the render package can stay decoupled from the
// canvas engine (the engine imports render, never the other way).
type BoardRender struct {
	PanX, PanY float64
	Zoom       float64
}

type renderer struct {
	rt           *runtime.Runtime
	opts         RenderOpts
	handlers     []Handler
	scope        map[string]any
	rootID       string // entry-scene root id (gets direction:rtl when RTL)
	rtl          bool
	idSuffix     string // per-item suffix so JS-wired widgets stay unique inside renderItem
	sb           strings.Builder
	unknowns     []string
	compChildren []*model.Node // children of the current component instance (for slot)
	compDepth    int
	// render budget (see spendNode in render.go): compDepth alone bounds the
	// DEPTH of component recursion, not the TOTAL work, so a self-referencing
	// component with two children fans out 2^depth times within the depth cap.
	nodesRendered int
	overBudget    bool
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s%s%s>`, attrID(n.ID), r.containerCSS(n), a, r.pressAttr(n), dragAttr(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// board renders an infinite-canvas board root: the children's coordinates
// are in WORLD space (a side-scroller game's 64x12 level), but the
// viewport only shows a window-sized slice. The canvas engine folds the
// pan/zoom into the board's content-group transform; the HTML path has
// no such group, so this renderer wraps the children in a positioned
// div whose transform mirrors that matrix. Without this, the level
// tiles would render at their raw world coordinates (offscreen, far
// from the viewport) and the user would see only the sky background.
//
// Pan / zoom come from the canvas-side BoardState, NOT from the app
// state — the engine keeps these on its Interaction sidecar (a single
// float per axis; mirrors the canvas-mode r.Inter.Board fields). The
// server pushes them into the renderer via the RenderOpts.Board field
// (see render.go) so the runtime, which is engine-free, never grows a
// dependency on canvas.
func (r *renderer) board(n *model.Node) {
	bg := r.containerCSS(n)
	a := a11y(n)
	if n.ID == r.rootID && !strings.Contains(a, "role=") {
		a += ` role="main"`
	}
	// Outer div: the board itself, fixed to the manifest-declared size
	// (or the viewport if the app didn't pin one). background, border
	// radius, padding — all from the node's style. overflow:hidden
	// crops the world to the viewport slice.
	fmt.Fprintf(&r.sb, `<div id=%q style=%q;overflow:hidden;position:relative%s%s%s>`, attrID(n.ID), bg, a, r.pressAttr(n), dragAttr(n))
	// Inner div: the content group, translated by the live PanX / PanY
	// and scaled by the live Zoom. The canvas engine owns these values;
	// the renderer just paints what the engine told it.
	px, py, zoom := r.opts.Board.PanX, r.opts.Board.PanY, r.opts.Board.Zoom
	// Fallback for the web (no-engine) path: many apps — and every
	// example with a board root before the canvas engine integration
	// — don't go through RenderWithOpts, so opts.Board is zero. The
	// app's own qscript has typically been writing cameraX / cameraY
	// to state as part of its physics step; reading those makes the
	// board transform work without the engine, and falls back to 0/0/1
	// when neither source has a value.
	if px == 0 && py == 0 {
		if v, ok := r.rt.State["cameraX"]; ok {
			px = asFloat(v)
		}
		if v, ok := r.rt.State["cameraY"]; ok {
			py = asFloat(v)
		}
	}
	if zoom == 0 {
		zoom = 1
	}
	fmt.Fprintf(&r.sb, `<div style="position:absolute;left:0;top:0;width:1px;height:1px;transform-origin:0 0;transform:translate(%.4fpx,%.4fpx) scale(%.4f);">`, px, py, zoom)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

func (r *renderer) text(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>%s</div>`, attrID(r.nid(n)), style, a11y(n), html.EscapeString(r.interp(n.Text)))
}

// scaffold is Flutter's Scaffold: an appbar child pins to the top, a bottomnav
// child to the bottom, fab children float bottom-right, the rest is the body.
func (r *renderer) scaffold(n *model.Node) {
	style := r.boxCSS(n) + "position:relative;display:flex;flex-direction:column;min-height:100%;"
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), style)
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
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-bottomnav" style=%q role="navigation">`, attrID(n.ID), style)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+style, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// appbar is Flutter's AppBar: leading + title + actions row.
func (r *renderer) appbar(n *model.Node) {
	// background is an author prop landing mid-declaration ("background:%s;"):
	// the CSS-value allowlist (cssValueOr) is what stops a `;` from starting a
	// new declaration — styleAttr only guards the attribute.
	bg := styleAttr(cssValueOr(propStr(n, "background"), "var(--surface)"))
	// The iOS bar is frosted by default; `backdropBlur` retunes the radius (0
	// turns the frost off) without the app having to restyle the whole bar.
	frost := frostCSS(r.backdropBlurPx(n, 20))
	style := fmt.Sprintf("display:flex;align-items:center;gap:6px;height:calc(44px + var(--safe-top, env(safe-area-inset-top, 0px)));padding:var(--safe-top, env(safe-area-inset-top, 0px)) 8px 0 8px;box-sizing:border-box;background:%s;%sborder-bottom:.5px solid var(--sep);", bg, frost)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+style, a11y(n))
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
		attrID(n.ID), style, a11y(n), r.pressAttr(n), html.EscapeString(label))
}

func (r *renderer) link(n *model.Node) {
	// Default is the renderer's own constant — a harmless no-navigation
	// placeholder that keeps an onPress-only link clickable without a scroll
	// jump — not author data, so it bypasses safeURL. An author-supplied href
	// IS untrusted: safeURL allowlists its scheme (a "javascript:"/"data:"
	// href would be stored XSS / phishing the moment a user taps the link).
	href := "javascript:void(0)"
	if v, ok := n.Prop("href"); ok {
		href = safeURL(fmt.Sprint(v))
	}
	style := r.boxCSS(n) + r.textCSS(n) + "cursor:pointer;text-decoration:none;"
	fmt.Fprintf(&r.sb, `<a id=%q href=%q style=%q%s%s>%s</a>`,
		attrID(n.ID), html.EscapeString(href), style, a11y(n), r.pressAttr(n), html.EscapeString(r.interp(labelOf(n))))
}

var stateBindRe = regexp.MustCompile(`^\s*\{\{\s*state\.([a-zA-Z0-9_.]+)\s*\}\}\s*$`)

func (r *renderer) divider(n *model.Node) {
	vertical := propStr(n, "orientation") == "vertical" || n.Type == "verticaldivider"
	line := "height:1px;width:100%;background:var(--sep);margin:8px 0;"
	if vertical {
		line = "width:1px;align-self:stretch;background:var(--sep);margin:0 8px;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q></div>`, attrID(n.ID), r.boxCSS(n)+line)
}

func (r *renderer) spacer(n *model.Node) {
	style := "flex:1 1 auto;"
	if v, ok := numOK(n.Style, "size"); ok {
		style = fmt.Sprintf("width:%gpx;height:%gpx;flex-shrink:0;", v, v)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q></div>`, attrID(n.ID), style)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="dialog"><div style=%q>`, attrID(n.ID), overlay, panel)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
}

// carousel renders a horizontally scroll-snapping row of children. Paging it is
// pure CSS; the two things that are not, and that both props below are, need
// the client:
//
//   - `autoplay` — milliseconds between automatic advances (0/absent = off, the
//     default). Floored at 250ms client-side, the same floor the declarative
//     `timer` widget uses. The client pauses while the pointer is over the
//     track and while the tab is hidden, and wraps at the end.
//   - `indicators` — emit the dot row under the track. The dots are a SIBLING of
//     the scroller (inside it they would scroll away with the content) and carry
//     no state: the client derives the active one from the live scroll position
//     after every scroll and every re-render, so it is right however the slide
//     changed — autoplay, a swipe, a dot tap, or a state-driven re-render.
//
// A carousel that declares neither renders byte-identically to before they
// existed: both markers are absent and the client code no-ops.
func (r *renderer) carousel(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;overflow-x:auto;scroll-snap-type:x mandatory;gap:12px;-webkit-overflow-scrolling:touch;"
	auto := ""
	if ms := int(propNum(n, "autoplay", 0)); ms > 0 {
		auto = fmt.Sprintf(` data-qorm-carousel="%d"`, ms)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), style, auto)
	for _, c := range n.Children {
		r.sb.WriteString(`<div style="scroll-snap-align:start;flex:0 0 auto;">`)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
	if propBool(n, "indicators") {
		r.carouselDots(len(n.Children))
	}
}

// carouselDots emits the indicator row: one button per slide, the first marked
// current because that is the slide a fresh render shows. They are buttons, not
// decoration, so a dot is tappable and reachable by keyboard; the client keys on
// data-qorm-dot for the tap and re-derives aria-current + the fill from the live
// scroll position.
func (r *renderer) carouselDots(n int) {
	r.sb.WriteString(`<div class="qorm-carousel-dots" data-qorm-dots="" style="display:flex;justify-content:center;gap:6px;padding:8px 0;">`)
	for i := 0; i < n; i++ {
		fill, cur := "var(--sep)", "false"
		if i == 0 {
			fill, cur = "var(--accent)", "true"
		}
		fmt.Fprintf(&r.sb, `<button data-qorm-dot="%d" aria-current="%s" aria-label="Slide %d" style="width:7px;height:7px;padding:0;border:none;border-radius:999px;background:%s;cursor:pointer;"></button>`,
			i, cur, i+1, fill)
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
	all, _ := runtime.EvalBinding(n.Data, r.ctx()).([]any)
	offset, items := r.pageWindow(n, all) // same built-in pagination as list
	cols := int(propNum(n, "crossAxisCount", 0))
	var tmpl string
	if cols > 0 {
		tmpl = fmt.Sprintf("repeat(%d,1fr)", cols)
	} else {
		tmpl = fmt.Sprintf("repeat(auto-fill,minmax(%gpx,1fr))", propNum(n, "minItemWidth", 120))
	}
	gap := propNum(n, "spacing", 10)
	style := fmt.Sprintf("display:grid;grid-template-columns:%s;gap:%gpx;grid-auto-rows:min-content;", tmpl, gap)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+style)
	prev := r.scope
	prevSuf := r.idSuffix
	alias, idxKey, firstKey, lastKey := ListAliasNames(propStr(n, "as")) // same item scope as list
	for i, it := range items {
		// index/first/last (and the id suffix) stay global across pages, as in list.
		r.scope = itemScope(prev, alias, idxKey, firstKey, lastKey, it, offset+i, len(all))
		r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, offset+i)
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
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-pageview" style=%q>`, attrID(n.ID), style)
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
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-screens" style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-screens-out" style="font-size:14px;color:var(--label);min-height:20px;white-space:pre-line;font-family:ui-monospace,Menlo,monospace;">—</div>`, attrID(n.ID))
	r.sb.WriteString(`</div>`)
}

// loginItem renders a toggle for launch-at-login (desktop).
func (r *renderer) loginItem(n *model.Node) {
	label := n.Label
	if label == "" {
		label = "Toggle Start at Login"
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-loginitem" data-on="0" style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-loginitem-out" style="font-size:15px;color:var(--label);min-height:20px;">Start at Login: —</div>`, attrID(n.ID))
	fmt.Fprintf(&r.sb, `<button type="button" onclick="qormLoginItem(this)" style="padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s</button>`, html.EscapeString(label))
	r.sb.WriteString(`</div>`)
}

// dockBadge renders -/+ buttons that set the Dock icon badge (unread count) on
// desktop.
func (r *renderer) dockBadge(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-dockbadge" data-count="0" style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;align-items:stretch;")
	fmt.Fprintf(&r.sb, `<div id="%s-out" class="qorm-dockbadge-out" style="font-size:15px;color:var(--label);min-height:20px;">Badge: 0</div>`, attrID(n.ID))
	r.sb.WriteString(`<div style="display:flex;gap:8px;"><button type="button" onclick="qormBadge(this,-1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--fill);color:var(--label);font-size:20px;font-weight:600;cursor:pointer;">−</button><button type="button" onclick="qormBadge(this,1)" style="flex:1;padding:12px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:20px;font-weight:600;cursor:pointer;">+</button></div>`)
	r.sb.WriteString(`</div>`)
}

// camera captures a photo via the device camera (WebView getUserMedia/file
// capture): a preview image + a shutter button. The captured image is a data
// URL synced into the bound state, so it can be shown or POSTed to a backend.
func (r *renderer) camera(n *model.Node) {
	val := r.interp(n.Value)
	path := boundPath(n.Value)
	label := iconLabel(propStr(n, "label"), "camera", "Take Photo")
	hattr := ""
	if n.OnChange != nil {
		hattr = fmt.Sprintf(` data-h="%d"`, r.register(n.OnChange))
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-camera"%s style=%q>`, attrID(n.ID), hattr,
		r.boxCSS(n)+"display:flex;flex-direction:column;gap:10px;align-items:stretch;")
	disp := "none"
	if val != "" {
		disp = "block"
	}
	// src is a non-navigating media context and val is the capture's data:
	// URL by design — entity-encode so a bound value cannot break out of the
	// attribute (no scheme filter: nothing executes script off an <img> src).
	fmt.Fprintf(&r.sb, `<img class="qorm-cam-preview" alt="" src=%q style="max-width:100%%;border-radius:12px;display:%s;">`, html.EscapeString(val), disp)
	fmt.Fprintf(&r.sb, `<input type="hidden"%s value=%q>`, dataStateAttr(path), html.EscapeString(val))
	// Live camera (desktop/web via getUserMedia — localhost is a secure context);
	// hidden until qormCameraInit shows the live button on capable platforms.
	r.sb.WriteString(`<video class="qorm-cam-video" playsinline muted style="display:none;max-width:100%;border-radius:12px;"></video>`)
	fmt.Fprintf(&r.sb, `<button type="button" class="qorm-cam-live" style="display:none;text-align:center;padding:12px 16px;border:none;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;" onclick="qormCameraLive(this)">%s</button>`, label)
	// A <label> wrapping the file input triggers the camera natively — the most
	// reliable path on iOS (a hidden input + programmatic .click() is blocked).
	fmt.Fprintf(&r.sb, `<label class="qorm-cam-file" style="display:inline-block;text-align:center;padding:12px 16px;border-radius:12px;background:var(--accent);color:var(--on-accent);font-size:16px;font-weight:600;cursor:pointer;">%s<input type="file" accept="image/*" capture="environment" style="position:absolute;width:1px;height:1px;opacity:0;" onchange="qormCamera(this)"></label>`,
		label)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;display:flex;")
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

// ---- month view (calendar grid) ----------------------------------------------

// monthNamesLong / monthWeekdays are the built-in English labels of the
// `monthview` widget; an app localises them with the `heading` and `weekdays`
// props (or by binding them through the i18n catalog).
var monthNamesLong = [12]string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

var monthWeekdays = [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}

// monthDays returns the length of a proleptic-Gregorian month — the leap rule
// spelled out rather than delegated to time.Time, so the whole widget is pure
// arithmetic with no clock anywhere near it (the render-determinism guard).
func monthDays(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if (y%4 == 0 && y%100 != 0) || y%400 == 0 {
			return 29
		}
		return 28
	}
	return 30
}

// monthWeekday returns the weekday of a date as 0=Sunday..6=Saturday
// (Sakamoto's method, valid for the proleptic Gregorian calendar from year 1).
func monthWeekday(y, m, d int) int {
	t := [12]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	if m < 3 {
		y--
	}
	w := (y + y/4 - y/100 + y/400 + t[m-1] + d) % 7
	if w < 0 {
		w += 7
	}
	return w
}

// monthAdd shifts (year, month) by delta months, carrying across the year
// boundary — December +1 is January of the next year, January -1 December of
// the previous one.
func monthAdd(y, m, delta int) (int, int) {
	t := y*12 + (m - 1) + delta
	if t < 0 {
		t = 0
	}
	return t / 12, t%12 + 1
}

// normDay normalises a "YYYY-MM-DD" date and reports whether it is a REAL
// calendar day: 2025-02-29 and 2026-13-01 are rejected rather than silently
// clamped, so an unparseable min/max/selected prop degrades to "not set"
// instead of to a wrong boundary. Normalised days are zero-padded, so the
// widget compares them with plain string ordering.
func normDay(s string) (string, bool) {
	p := strings.Split(strings.TrimSpace(s), "-")
	if len(p) != 3 {
		return "", false
	}
	y, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	d, e3 := strconv.Atoi(p[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return "", false
	}
	if y < 1 || m < 1 || m > 12 || d < 1 || d > monthDays(y, m) {
		return "", false
	}
	return fmtDate(y, m, d), true
}

// normMonth parses a month from "YYYY-MM" or from a full "YYYY-MM-DD" day.
func normMonth(s string) (int, int, bool) {
	p := strings.Split(strings.TrimSpace(s), "-")
	if len(p) != 2 && len(p) != 3 {
		return 0, 0, false
	}
	y, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || y < 1 || m < 1 || m > 12 {
		return 0, 0, false
	}
	return y, m, true
}

// weekStartIndex maps the `weekStart` prop to a 0=Sunday..6=Saturday column
// offset. Names and numbers are both accepted; anything else means Sunday.
func weekStartIndex(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mon", "monday", "1":
		return 1
	case "tue", "tuesday", "2":
		return 2
	case "wed", "wednesday", "3":
		return 3
	case "thu", "thursday", "4":
		return 4
	case "fri", "friday", "5":
		return 5
	case "sat", "saturday", "6":
		return 6
	}
	return 0
}

// monthEvents resolves the `events` prop into date -> marker colour. An entry is
// either a bare "YYYY-MM-DD" string or an object {date, color}; an unparseable
// date is dropped. The colour passes cssValue (render_data.go), the repo's
// strict CSS allowlist, before it is ever written into a declaration — so even
// though the dot's colour lands in a quoted style ATTRIBUTE (where
// html.EscapeString would already be enough), a value carrying ';' '{' '}' '<'
// is discarded rather than escaped, and the widget can never be the source of a
// CSS-rule injection if the markup around it ever moves into a <style> block.
func (r *renderer) monthEvents(n *model.Node) map[string]string {
	out := map[string]string{}
	for _, e := range r.boundArray(n, "events") {
		date, color := "", ""
		switch t := e.(type) {
		case string:
			date = t
		case map[string]any:
			date = r.interp(str(t, "date"))
			color = cssValue(str(t, "color"))
		}
		if d, ok := normDay(date); ok {
			out[d] = color
		}
	}
	return out
}

// monthNavHandler registers the handler behind one of the month-view arrows and
// returns its index, or -1 for "no handler" (the arrow then renders natively
// disabled rather than dead).
//
// Two ways to wire paging, in order:
//
//   - `onMonthChange` — an invoke prop ({name,args}) or a bare action name; the
//     target month arrives as a `month` arg ("YYYY-MM"), the same
//     register-once-per-target trick bottomnav uses for `value`.
//   - failing that, the day handler `onChange`, with the nearest day of the
//     target month as `value`. Because `month` defaults to the month of
//     `selected`, moving the selection moves the grid — so a calendar wired with
//     the ONE action it needs anyway (record the picked date) gets working
//     prev/next for free, with no new event channel.
func (r *renderer) monthNavHandler(n *model.Node, y, m int, sel string) int {
	inv := parseInvokeProp(n, "onMonthChange")
	if inv == nil {
		if raw, ok := n.Prop("onMonthChange"); ok {
			if name, ok := raw.(string); ok && strings.TrimSpace(name) != "" {
				inv = &model.Invoke{Name: strings.TrimSpace(name)}
			}
		}
	}
	if inv != nil {
		return r.register(&model.Invoke{Name: inv.Name,
			Args: mergeArgs(inv.Args, "month", fmt.Sprintf("%04d-%02d", y, m))})
	}
	if n.OnChange == nil {
		return -1
	}
	d := 1
	if _, _, sd, ok := splitDay(sel); ok {
		d = sd
	}
	if d > monthDays(y, m) {
		d = monthDays(y, m)
	}
	return r.register(&model.Invoke{Name: n.OnChange.Name,
		Args: mergeArgs(n.OnChange.Args, "value", fmtDate(y, m, d))})
}

// splitDay splits an already-normalised "YYYY-MM-DD" into its parts.
func splitDay(s string) (y, m, d int, ok bool) {
	p := strings.Split(s, "-")
	if len(p) != 3 {
		return 0, 0, 0, false
	}
	y, _ = strconv.Atoi(p[0])
	m, _ = strconv.Atoi(p[1])
	d, _ = strconv.Atoi(p[2])
	return y, m, d, true
}

// monthNavButton renders one header arrow. The glyph is the built-in
// chevron-right icon, mirrored with scaleX(-1) for the previous-month arrow, so
// the widget needs no icon the icon set does not already carry.
func (r *renderer) monthNavButton(h int, label string, flip bool) {
	style := "display:inline-flex;align-items:center;justify-content:center;width:30px;height:30px;flex:none;border:none;border-radius:8px;background:var(--fill);color:var(--label);cursor:pointer;padding:0;"
	if flip {
		style += "transform:scaleX(-1);"
	}
	attr := " disabled"
	if h >= 0 {
		attr = fmt.Sprintf(` onclick="qorm(%d)"`, h)
	} else {
		style += "opacity:.35;cursor:default;"
	}
	fmt.Fprintf(&r.sb, `<button type="button" aria-label=%q style=%q%s>%s</button>`,
		html.EscapeString(label), style, attr, iconSVG("chevron-right", 17))
}

// monthView renders ONE month as a 7-column grid — the picker a booking or
// scheduling app needs and that neither `datepicker` (a Cupertino scroll wheel)
// nor `calendar` (the iOS/Android add-an-event hardware bridge, see
// internal/capability) provides.
//
// Everything is a binding, and nothing is a new event channel:
//
//	selected  the chosen day, "YYYY-MM-DD" (bindable)
//	month     the month on screen, "YYYY-MM" or "YYYY-MM-DD" (bindable);
//	          defaults to the month of `selected`
//	events    [ "YYYY-MM-DD" | {date,color} ] — a dot under the day (bindable)
//	min, max  the selectable range; days outside render natively disabled and
//	          an arrow whose whole target month is out of range disables too
//	today     the day to ring (a prop, not the clock: render must stay
//	          byte-deterministic for the same state)
//	weekStart "sunday" (default) … "saturday", or 0..6
//	weekdays  seven column labels; `heading` overrides the "July 2026" header
//	onChange  fires with the clicked day as `value` — the same contract
//	          datepicker/picker use, so existing actions work unchanged
//
// Days from the neighbouring months fill the leading/trailing cells and stay
// selectable (dimmed), which is what a user expects when a booking straddles a
// month boundary; showAdjacent:false blanks them instead.
//
// With no `month` and no parseable `selected` the grid opens on 2026-07, the
// same fixed epoch datepicker's parseDate3 falls back to — a widget may not read
// the clock, or the same state would render differently on two machines and the
// determinism guard (and OTA bundle hashes) would break.
func (r *renderer) monthView(n *model.Node) {
	sel, hasSel := normDay(r.interp(propStr(n, "selected")))
	y, m, ok := normMonth(r.interp(propStr(n, "month")))
	if !ok {
		if y, m, ok = normMonth(sel); !ok {
			y, m = 2026, 7
		}
	}
	minD, hasMin := normDay(r.interp(propStr(n, "min")))
	maxD, hasMax := normDay(r.interp(propStr(n, "max")))
	today, hasToday := normDay(r.interp(propStr(n, "today")))
	events := r.monthEvents(n)
	start := weekStartIndex(r.interp(propStr(n, "weekStart")))
	showAdj := propStr(n, "showAdjacent") != "false"

	// `heading`, not `title`: a11y() already claims `title` for the native HTML
	// title attribute on every widget, so reusing it here would silently give the
	// calendar a browser tooltip as well as a header.
	head := r.interp(propStr(n, "heading"))
	if head == "" {
		head = fmt.Sprintf("%s %d", monthNamesLong[m-1], y)
	}
	labels := stringList(n.Props["weekdays"])
	if len(labels) != 7 {
		labels = monthWeekdays[:]
	}

	py, pm := monthAdd(y, m, -1)
	ny, nm := monthAdd(y, m, 1)
	prevH, nextH := -1, -1
	if !hasMin || fmtDate(py, pm, monthDays(py, pm)) >= minD {
		prevH = r.monthNavHandler(n, py, pm, sel)
	}
	if !hasMax || fmtDate(ny, nm, 1) <= maxD {
		nextH = r.monthNavHandler(n, ny, nm, sel)
	}

	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-monthview" style=%q%s>`,
		attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:8px;", a11y(n))

	// header: prev / month title / next
	r.sb.WriteString(`<div style="display:flex;align-items:center;gap:8px;">`)
	r.monthNavButton(prevH, "Previous month", true)
	fmt.Fprintf(&r.sb, `<div style="flex:1;text-align:center;font-size:15px;font-weight:600;color:var(--label);">%s</div>`,
		html.EscapeString(head))
	r.monthNavButton(nextH, "Next month", false)
	r.sb.WriteString(`</div>`)

	// weekday header row + day grid share the same 7-column track.
	r.sb.WriteString(`<div style="display:grid;grid-template-columns:repeat(7,1fr);gap:2px;">`)
	for i := 0; i < 7; i++ {
		fmt.Fprintf(&r.sb, `<div style="text-align:center;font-size:11px;font-weight:600;color:var(--label2);padding-bottom:2px;">%s</div>`,
			html.EscapeString(labels[(start+i)%7]))
	}
	days := monthDays(y, m)
	lead := (monthWeekday(y, m, 1) - start + 7) % 7
	total := ((lead + days + 6) / 7) * 7
	for i := 0; i < total; i++ {
		off := i - lead
		cy, cm, cd, adj := y, m, off+1, false
		switch {
		case off < 0:
			cy, cm = py, pm
			cd, adj = monthDays(py, pm)+off+1, true
		case off >= days:
			cy, cm = ny, nm
			cd, adj = off-days+1, true
		}
		if adj && !showAdj {
			r.sb.WriteString(`<div style="height:38px;"></div>`)
			continue
		}
		date := fmtDate(cy, cm, cd)
		blocked := (hasMin && date < minD) || (hasMax && date > maxD)
		isSel := hasSel && date == sel

		cell := "display:flex;flex-direction:column;align-items:center;justify-content:center;gap:3px;height:38px;padding:0;border:none;border-radius:9px;background:none;font-family:inherit;font-size:14px;font-variant-numeric:tabular-nums;color:var(--label);"
		if adj {
			cell += "color:var(--label2);opacity:.55;"
		}
		switch {
		case isSel:
			cell += "background:var(--accent);color:var(--on-accent);font-weight:600;opacity:1;"
		case hasToday && date == today:
			cell += "box-shadow:inset 0 0 0 1.5px var(--accent);font-weight:600;"
		}
		attr, aria := "", ""
		switch {
		case blocked:
			// A native `disabled` button is inert to mouse, keyboard and
			// assistive tech alike — no handler is registered for it at all.
			attr = " disabled"
			cell += "opacity:.28;cursor:default;"
		case n.OnChange != nil:
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{
				Name: n.OnChange.Name, Args: mergeArgs(n.OnChange.Args, "value", date)}))
			cell += "cursor:pointer;"
		}
		if isSel {
			aria = ` aria-current="date"`
		}
		fmt.Fprintf(&r.sb, `<button type="button" data-date=%q aria-label=%q%s%s style=%q>%d`,
			html.EscapeString(date), html.EscapeString(date), aria, attr, cell, cd)
		if color, marked := events[date]; marked {
			dot := "var(--accent)"
			if color != "" {
				dot = color
			}
			if isSel {
				dot = "var(--on-accent)"
			}
			fmt.Fprintf(&r.sb, `<span style="width:5px;height:5px;border-radius:50%%;background:%s;"></span>`, styleAttr(dot))
		} else {
			r.sb.WriteString(`<span style="width:5px;height:5px;"></span>`)
		}
		r.sb.WriteString(`</button>`)
	}
	r.sb.WriteString(`</div></div>`)
}

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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;display:flex;")
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+"position:relative;height:180px;min-height:180px;flex-shrink:0;overflow:hidden;", a11y(n))
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
			attrID(id), ctxItemCSS, iconHTML, html.EscapeString(title))
	}
}

func (r *renderer) contextMenu(n *model.Node) {
	// Desktop right-click menu (items): positioned at the cursor, icons +
	// submenus, selection fires qormEmit('context', {id}).
	if raw, ok := n.Prop("items"); ok {
		if items, ok := raw.([]any); ok && len(items) > 0 {
			fmt.Fprintf(&r.sb, `<div id=%q class="qorm-ctxmenu" style=%q>`, attrID(r.nid(n)), r.boxCSS(n)+"position:relative;")
			for _, c := range n.Children {
				r.node(c)
			}
			// menuStyle is a raw DECLARATION LIST by contract, not a single
			// value, so it is the one CSS input that is not value-filtered —
			// cssRawDecls documents why that is safe here (propStr never
			// evaluates bindings, so no state / http / MCP-set-state value can
			// reach it) and still rejects the part that reaches off the page
			// (url() beacons, comment truncation). styleAttr keeps it inside the
			// quoted attribute.
			panelStyle := "display:none;position:fixed;z-index:80;min-width:200px;background:var(--surface);border-radius:12px;box-shadow:0 10px 40px rgba(0,0,0,.28);padding:6px;border:.5px solid var(--sep);-webkit-backdrop-filter:blur(20px);backdrop-filter:blur(20px);" + styleAttr(cssRawDecls(propStr(n, "menuStyle")))
			r.sb.WriteString(`<div class="qorm-ctxmenu-panel" style="` + panelStyle + `">`)
			r.ctxItems(items)
			r.sb.WriteString(`</div></div>`)
			return
		}
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-ctx" style=%q>`, attrID(r.nid(n)), r.boxCSS(n)+"position:relative;")
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
	fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormCtx(document.getElementById(%s))})</script>`, jsStringID(r.nid(n)))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(r.nid(n)), r.boxCSS(n)+"overflow-y:auto;overscroll-behavior:contain;")
	r.refreshSpinner()
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
	if h >= 0 {
		r.refreshScript(r.nid(n), h)
	}
}

// refreshSpinner writes the pull-to-refresh affordance: a collapsed row at the
// top of a scroll container that qormRefresh grows and fades in as the finger
// drags past the threshold. It must be the container's FIRST child (that is
// what qormRefresh finds and animates).
//
// Shared by refreshindicator and list's built-in onRefresh, together with
// refreshScript below, so both surface the exact same gesture, markup and
// client helper — the list does not reimplement pull-to-refresh, it exposes it.
func (r *renderer) refreshSpinner() {
	r.sb.WriteString(`<div class="qorm-refresh-spin" style="height:0;opacity:0;display:flex;align-items:center;justify-content:center;overflow:hidden;transition:height .2s;"><span class="qorm-activity"><svg width="20" height="20" viewBox="0 0 20 20">`)
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&r.sb, `<rect x="9" y="2" width="2" height="5" rx="1" fill="var(--label2)" opacity="%g" transform="rotate(%d 10 10)"/>`, 0.25+0.75*float64(i)/7, i*45)
	}
	r.sb.WriteString(`</svg></span></div>`)
}

// refreshScript binds the qormRefresh drag gesture on the element with the
// given id to handler h.
func (r *renderer) refreshScript(id string, h int) {
	fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormRefresh(document.getElementById(%s),%d)})</script>`, jsStringID(id), h)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+fmt.Sprintf("aspect-ratio:%g;overflow:hidden;", ratio))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// richText is Flutter's RichText / Text.rich: a paragraph of styled spans
// ([{text, color, fontSize, fontWeight, italic, underline}]).
func (r *renderer) richText(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"line-height:1.5;")
	spans, _ := n.Prop("spans")
	arr, _ := spans.([]any)
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		var st strings.Builder
		// The span colour is per-item author data written into a declaration of
		// the assembled style attribute, so it needs the CSS-value allowlist:
		// styleAttr (applied to the whole builder below) entity-encodes, which
		// stops an attribute breakout but not a `;` opening the next
		// declaration — `#000;position:fixed;…;width:100vw;height:100vh` would
		// otherwise turn one text span into a full-screen overlay. A rejected
		// colour drops just its own declaration; the span keeps its size,
		// weight and text.
		if c := cssStyleValue(str(m, "color")); c != "" {
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
		// the span colour is author input interpolated into a quoted style
		// attribute: entity-encode the assembled declarations (styleAttr).
		fmt.Fprintf(&r.sb, `<span style="%s">%s</span>`, styleAttr(st.String()), html.EscapeString(r.interp(str(m, "text"))))
	}
	r.sb.WriteString(`</div>`)
}

// largeTitle is the iOS large-title navigation bar (CupertinoSliverNavigationBar
// / Flutter's SliverAppBar): a compact bar row over a big bold title,
// translucent with a hairline.
//
// It COLLAPSES on scroll. The compact bar is position:sticky, so the big title
// scrolls up and disappears behind it with no JS and no modern-CSS support
// required at all; the cross-fade that swaps the big title for the compact one
// is a CSS scroll-driven animation over the big title's own view() timeline
// (see the .qorm-lt rules in the HTML shell), with qormLargeTitleSync in app.js
// as the fallback for browsers that lack scroll-driven animations. Because both
// paths are keyed off classes the SERVER renders, the behaviour costs nothing at
// dispatch time and cannot be lost by a DOM morph.
//
// `collapsible:false` restores the plain static bar (no sticky, no second
// title) for a header that is not sitting at the top of a scroll view.
func (r *renderer) largeTitle(n *model.Node) {
	// background is an author prop landing mid-declaration, in TWO places (the
	// wrapper and the compact bar): CSS-value allowlist first (see appbar), then
	// the attribute encoding.
	bg := styleAttr(cssValueOr(propStr(n, "background"), "var(--bg)"))
	frost := frostCSS(r.backdropBlurPx(n, 20))
	collapsible := propStr(n, "collapsible") != "false"
	title := html.EscapeString(r.interp(labelOf(n)))

	// Marker + collapse classes ride on the SERVER's markup, so the whole effect
	// (sticky bar, scroll-driven cross-fade, and the app.js fallback's lookup)
	// re-establishes itself on every render with nothing to reconcile.
	mark, barCls, barPos, bigCls := "", "", "", ""
	if collapsible {
		mark = ` class="qorm-lt" data-qorm-largetitle`
		barCls = ` class="qorm-lt-bar"`
		// The hairline starts transparent: iOS shows no rule between the compact
		// bar and the big title while the header is expanded, and fades one in
		// as the bar takes over. The wrapper's own border sits under the whole
		// block and scrolls away with it.
		barPos = "position:sticky;top:0;z-index:20;border-bottom:.5px solid transparent;"
		bigCls = ` class="qorm-lt-big"`
	}
	fmt.Fprintf(&r.sb, `<div id=%q%s style=%q>`, attrID(r.nid(n)), mark,
		r.boxCSS(n)+fmt.Sprintf("background:%s;%sborder-bottom:.5px solid var(--sep);", bg, frost))
	// Compact bar: the sticky row that stays on screen once the big title has
	// scrolled away. It repeats the background + frost because the big title
	// passes BEHIND it and it must stay opaque enough to hide it.
	fmt.Fprintf(&r.sb, `<div%s style=%q>`, barCls, fmt.Sprintf(
		"%sdisplay:flex;align-items:center;gap:6px;height:36px;padding:0 12px;color:var(--accent);background:%s;%s", barPos, bg, frost))
	if collapsible {
		r.sb.WriteString(`<div style="min-width:44px;"></div>`)
		fmt.Fprintf(&r.sb, `<div class="qorm-lt-mini" style="flex:1;min-width:0;text-align:center;font-size:17px;font-weight:600;color:var(--label);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">%s</div>`, title)
	}
	r.sb.WriteString(`<div style="min-width:44px;display:flex;justify-content:flex-end;align-items:center;gap:6px;">`)
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div></div>`)
	// large title
	fmt.Fprintf(&r.sb, `<div%s style="padding:0 16px 10px;font-size:34px;font-weight:700;letter-spacing:-.02em;color:var(--label);">%s</div>`, bigCls, title)
	if sub := r.interp(propStr(n, "subtitle")); sub != "" {
		fmt.Fprintf(&r.sb, `<div style="padding:0 16px 10px;font-size:15px;color:var(--label2);margin-top:-6px;">%s</div>`, html.EscapeString(sub))
	}
	r.sb.WriteString(`</div>`)
}

// navigationRail is Flutter's NavigationRail: a vertical destination bar for
// wide (desktop) layouts; tapping dispatches onChange with {value}.
func (r *renderer) navigationRail(n *model.Node) {
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID),
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
	fmt.Fprintf(&r.sb, `<button id=%q style=%q%s%s>%s`, attrID(r.nid(n)), style, al, onclick, glyph)
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
	// novalidate turns off native constraint validation for the whole form —
	// the gate a submit button emits reads form.noValidate, so this switches
	// both off together (submit-then-validate-server-side flows).
	nov := ""
	if asBool(r.interp(propStr(n, "novalidate"))) {
		nov = ` novalidate`
	}
	fmt.Fprintf(&r.sb, `<form id=%q style=%q%s%s%s>`, attrID(r.nid(n)), r.containerCSS(n), a11y(n), nov, submit)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(r.nid(n)), style, a11y(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(r.nid(n)), r.boxCSS(n)+"display:contents;pointer-events:none;", a11y(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q role="navigation">`, attrID(r.nid(n)),
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s role="toolbar">`, attrID(r.nid(n)), style, a11y(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(r.nid(n)), r.boxCSS(n)+lim, a11y(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(r.nid(n)), r.boxCSS(n)+"position:relative;", a11y(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>%s</div>`, attrID(n.ID),
		r.boxCSS(n)+r.textCSS(n)+"user-select:text;-webkit-user-select:text;cursor:text;",
		html.EscapeString(r.interp(n.Text)))
}
