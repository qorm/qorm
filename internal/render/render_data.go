package render

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// list renders `renderItem` once per element of the bound `data`. On top of the
// item scope (item/index/first/last, see itemScope) it has four opt-in
// behaviours, each inert unless its prop is present — a list that declares none
// renders byte-identically to before they existed:
//
//   - pagination — `pageSize` (+ `page`, 1-based, typically {{state.page}})
//     renders only one window of the data (see pageWindow). index/first/last
//     stay GLOBAL — they describe the item's position in the full data set, so
//     row numbering keeps counting across pages and `last` means "last row of
//     the data", not "last row on screen". Node ids get the global index as
//     their suffix too, so an element keeps its id whatever page shows it.
//   - separators — `separator` draws a hairline between items (never after the
//     last one, and never across a section boundary), see separatorHTML.
//   - sticky section headers — `groupBy` splits consecutive items into sections
//     and emits a sticky header per section, see sectionHeaderHTML.
//   - pull-to-refresh — `onRefresh` makes the list its own scroll container and
//     wires the same qormRefresh gesture (and spinner) as refreshindicator.
//
// Two gesture/structure conflicts are resolved here rather than shipped broken:
// a grouped list does not wire drag-to-reorder (the section headers are extra
// children, so qormReorder's child indices would not match the data), and a
// reorderable list does not wire pull-to-refresh (both are pointer-drags on the
// same element). Separators never cost an index: they render INSIDE the item's
// wrapper, so the list keeps exactly one element child per item.
func (r *renderer) list(n *model.Node) {
	if n.Template == nil {
		r.container(n)
		return
	}
	all, _ := runtime.EvalBinding(n.Data, r.ctx()).([]any)
	offset, items := r.pageWindow(n, all)
	// Virtualization: `content-visibility:auto` makes the browser skip layout
	// and paint for off-screen items — cheap windowing for long lists with no
	// JS, working with server-rendered HTML. contain-intrinsic-size reserves the
	// scrollbar space so scrolling stays stable.
	virt := propBool(n, "virtualize")
	itemH := propNum(n, "itemHeight", 44)
	wrap := fmt.Sprintf("content-visibility:auto;contain-intrinsic-size:0 %gpx;", itemH)
	sep := r.separatorHTML(n)

	prev := r.scope
	prevSuf := r.idSuffix
	alias, idxKey, firstKey, lastKey := ListAliasNames(propStr(n, "as"))
	scopeAt := func(i int) map[string]any {
		return itemScope(prev, alias, idxKey, firstKey, lastKey, items[i], offset+i, len(all))
	}
	keys := r.sectionKeys(n, items, scopeAt)

	reorderH, refreshH := -1, -1
	if propBool(n, "reorderable") && keys == nil {
		if inv := parseInvokeProp(n, "onReorder"); inv != nil {
			reorderH = r.register(inv)
		}
	}
	if reorderH < 0 && n.ID != "" { // the gesture is bound by element id
		if inv := parseInvokeProp(n, "onRefresh"); inv != nil {
			refreshH = r.register(inv)
		}
	}
	css := r.containerCSS(n) + "flex-direction:column;"
	if refreshH >= 0 {
		css += "overflow-y:auto;overscroll-behavior:contain;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), css)
	if refreshH >= 0 {
		r.refreshSpinner()
	}
	for i := range items {
		r.scope = scopeAt(i)
		r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, offset+i)
		if keys != nil && (i == 0 || keys[i] != keys[i-1]) {
			r.sectionHeaderHTML(n, keys[i])
		}
		// No separator after the last rendered item, nor before a section
		// header (the header is the divider there).
		showSep := sep != "" && i < len(items)-1 && (keys == nil || keys[i+1] == keys[i])
		switch {
		case virt:
			fmt.Fprintf(&r.sb, `<div style=%q>`, wrap)
		case showSep:
			r.sb.WriteString(`<div>`)
		}
		r.node(n.Template)
		if showSep {
			r.sb.WriteString(sep)
		}
		if virt || showSep {
			r.sb.WriteString(`</div>`)
		}
	}
	r.scope = prev
	r.idSuffix = prevSuf
	r.sb.WriteString(`</div>`)
	if reorderH >= 0 && n.ID != "" {
		fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormReorder(document.getElementById(%s),%d)})</script>`, jsStringID(n.ID), reorderH)
	}
	if refreshH >= 0 {
		r.refreshScript(n.ID, refreshH)
	}
}

// pageWindow resolves a list/gridview's built-in pagination: it returns the
// slice of `items` the current page shows plus that slice's offset in the full
// data. `pageSize` is the window length and `page` the 1-based page number —
// both may be literals or bindings ({{state.page}}), which is what makes a
// pagination widget wired to the same state page the list without the app
// hand-slicing its data.
//
// Absent (or non-positive) pageSize means NO pagination: the full data renders,
// exactly as before this existed, so an app that already slices its own data in
// an action keeps working untouched. Out-of-range pages are clamped rather than
// rendering nothing — page 0 or -3 shows the first page and a page past the end
// shows the last one, so a stale/overshot page counter always shows data the
// user can page back from instead of an empty screen.
func (r *renderer) pageWindow(n *model.Node, items []any) (offset int, window []any) {
	size := r.numProp(n, "pageSize")
	if size == nil || *size < 1 || len(items) == 0 {
		return 0, items
	}
	per := int(*size)
	pages := (len(items) + per - 1) / per
	page := 1
	if p := r.numProp(n, "page"); p != nil {
		page = int(*p)
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * per
	end := start + per
	if end > len(items) {
		end = len(items)
	}
	return start, items[start:end]
}

// separatorHTML returns the markup the list draws between two items, or "" when
// the node declares no `separator`. `separator: true` is the default hairline;
// an object form tunes it — {"height":1,"inset":16,"color":"var(--accent)"} —
// where inset is the left indent iOS-style lists use to align the rule with the
// text rather than the screen edge.
func (r *renderer) separatorHTML(n *model.Node) string {
	raw, ok := n.Prop("separator")
	if !ok {
		return ""
	}
	cfg, isCfg := raw.(map[string]any)
	if !isCfg && !asBool(raw) {
		return ""
	}
	color := "var(--sep)"
	if c, ok := cfg["color"].(string); ok && c != "" {
		color = c
	}
	return fmt.Sprintf(`<div style="height:%gpx;background:%s;margin-left:%gpx;flex:none;"></div>`,
		numOrDefault(cfg, "height", 0.5), html.EscapeString(color), numOrDefault(cfg, "inset", 0))
}

// sectionKeys resolves each rendered item's section key from `groupBy`, or nil
// when the list declares none (the ungrouped path stays untouched). The key is
// either a {{binding}} evaluated in that item's own scope ("{{ item.dept }}")
// or — the common case — a bare field name of the item ("dept").
//
// Sections are RUNS of consecutive equal keys, not a regrouping: the renderer
// never reorders data, it only inserts a header where the key changes. A list
// whose data is sorted by the key gets one header per value; unsorted data gets
// a header each time the value flips, which is visible feedback that the data
// wants sorting rather than a silent shuffle behind the app's back.
func (r *renderer) sectionKeys(n *model.Node, items []any, scopeAt func(int) map[string]any) []string {
	group := propStr(n, "groupBy")
	if group == "" || len(items) == 0 {
		return nil
	}
	prev := r.scope
	keys := make([]string, len(items))
	for i, it := range items {
		r.scope = scopeAt(i)
		if strings.Contains(group, "{{") {
			keys[i] = r.interp(group)
			continue
		}
		if obj, ok := it.(map[string]any); ok {
			if v, ok := obj[group]; ok {
				keys[i] = fmt.Sprint(v)
			}
			continue
		}
		keys[i] = fmt.Sprint(it)
	}
	r.scope = prev
	return keys
}

// sectionHeaderHTML writes one section header. It is `position:sticky` by
// default, so the header of the section you are reading stays pinned at the top
// of the scroll port (iOS/Android sectioned-list behaviour) — set
// `sticky: false` for headers that scroll away with their section, and
// `stickyTop` to the height of anything already pinned above the list (an
// appbar) so the header parks under it instead of behind it.
//
// Sticky needs a scrolling ancestor: the header pins inside the nearest
// scrollable box (a `scroll` container, the list itself when `onRefresh` gives
// it overflow, or the page) and unpins when its section scrolls past. The
// background is opaque (var(--bg)) so scrolled-under content cannot show
// through the pinned header.
//
// The label is `sectionHeader` when set — an expression evaluated in the scope
// of the section's first item, e.g. "{{ item.deptLabel }}" — else the key.
func (r *renderer) sectionHeaderHTML(n *model.Node, key string) {
	label := key
	if h := propStr(n, "sectionHeader"); h != "" {
		label = r.interp(h)
	}
	pos := "sticky"
	if v, ok := n.Prop("sticky"); ok && !asBool(v) {
		pos = "static"
	}
	fmt.Fprintf(&r.sb, `<div class="qorm-list-section" style="position:%s;top:%gpx;z-index:1;flex:none;background:var(--bg);color:var(--label2);font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:.02em;padding:6px 14px;">%s</div>`,
		pos, propNum(n, "stickyTop", 0), html.EscapeString(label))
}

// reservedScopeAliases are evaluation-context roots an `as` alias must never
// shadow: binding the item over them would break every {{state.x}} (etc.)
// inside the template. The renderer falls back to the default alias for these
// (and the loader warns at load time), so a bad alias degrades loudly-but-
// safely instead of silently killing state bindings.
var reservedScopeAliases = map[string]bool{
	"state": true, "t": true, "viewport": true, "route": true, "prop": true,
}

// ListAliasNames resolves a list/gridview `as` prop value to the four scope
// names its renderItem template binds: the item alias plus the derived index/
// first/last keys. The default alias keeps the short built-in names (`index`,
// `first`, `last`); a custom alias namespaces them (`as: "row"` → `rowIndex`,
// `rowFirst`, `rowLast`). Namespacing is what lets an aliased nested list
// keep the whole outer scope visible: none of its four keys can collide with
// the outer list's, so {{section.title}} and {{row.name}} work side by side.
// An `as` that is reserved or not a plain identifier (which the expression
// language could never reference) falls back to the default names.
//
// Exported for the loader's static expression checks, so what the loader
// accepts and what the renderer binds can never drift apart.
func ListAliasNames(as string) (alias, idxKey, firstKey, lastKey string) {
	if as == "" || as == "item" || reservedScopeAliases[as] || !isIdent(as) {
		return "item", "index", "first", "last"
	}
	return as, as + "Index", as + "First", as + "Last"
}

// isIdent reports whether s is a plain identifier ([A-Za-z_][A-Za-z0-9_]*) —
// the only names the expression language can reference.
func isIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || (i > 0 && '0' <= c && c <= '9') {
			continue
		}
		return false
	}
	return s != ""
}

// itemScope builds one item's template scope for list/gridview: a copy of the
// outer scope with the item bound under alias plus its 0-based index and
// first/last flags. Writing the four keys after the copy means the innermost
// list wins on a name collision — so a default-named nested list shadows the
// outer item/index/first/last exactly as it always shadowed `item`, while
// everything else in the outer chain (including an outer list's `as` bindings)
// stays visible.
//
// Conflict policy with user data: index/first/last are separate scope keys and
// are never injected into the item value itself, so a data field named
// `index` is untouched — {{item.index}} still reads the data, {{index}} reads
// the loop position. Built-ins always win over an identically named key
// inherited from an outer scope because the innermost iteration is the one
// the template is rendering.
func itemScope(outer map[string]any, alias, idxKey, firstKey, lastKey string, it any, i, total int) map[string]any {
	s := make(map[string]any, len(outer)+4)
	for k, v := range outer {
		s[k] = v
	}
	s[alias] = it
	s[idxKey] = i
	s[firstKey] = i == 0
	s[lastKey] = i == total-1
	return s
}

// tabs renders a header row of tab labels and shows the child panel matching
// the active tab.
//
// UNCONTROLLED is the default and stays exactly what it was: the first tab is
// active and switching is handled client-side by qormTab (no state round-trip,
// no server involvement). Every prop below is opt-in and inert when absent, so
// a tabs node that declares none renders byte-identically to before they
// existed:
//
//   - `active` — the active index, a literal or a binding ({{state.tab}}),
//     clamped into range (see activeIndex). A PLAINLY STATE-BOUND active makes
//     the tabs CONTROLLED: the labels render as <label>-wrapped hidden radios
//     carrying data-state, so a tap writes the tapped index back to that state
//     path over the existing qorm(-1) state-sync — controlled tabs with no
//     action file and no new client JS (the same idiom `segmented` uses).
//   - `onChange` — dispatched with {index, tab} on every tap, for apps that
//     want an action to run (it composes with a bound `active`: the radio still
//     syncs the index, the action still runs).
//   - `scrollable` — the tab bar scrolls horizontally with scroll-snap instead
//     of squeezing tabs to nothing once they overflow. Pure CSS: no gesture JS,
//     so it works on the first paint and inside a static export.
//   - `indicator` / `indicatorColor` — style the active-tab indicator
//     (`underline` default, `pill`, or `none`), see tabIndicator.
//   - `lazy` — render ONLY the active panel instead of all of them (see below).
//
// Lazy is deliberately gated on the tabs being controlled. Client-side
// switching works by toggling the display of panels that are ALREADY in the
// document; drop the inactive ones and a qormTab tap would reveal nothing. So
// `lazy` takes effect only when a tap reaches the server (bound `active` or an
// onChange) and is otherwise ignored — a lazy tabs never degrades into a dead
// UI, it degrades into an eager one.
func (r *renderer) tabs(n *model.Node) {
	labels := stringList(n.Props["tabs"])
	count := len(labels)
	if len(n.Children) > count {
		count = len(n.Children)
	}
	active := r.activeIndex(n, count)
	path := boundPath(propStr(n, "active"))
	lazy := propBool(n, "lazy") && (path != "" || n.OnChange != nil)

	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;")
	r.tabIndicator(n)
	bar := "display:flex;gap:2px;border-bottom:1px solid var(--sep);"
	item := ""
	if propBool(n, "scrollable") {
		// scrollbar-width:none keeps the bar looking like a tab strip rather
		// than a scroller; scroll-snap parks a tab at the leading edge.
		bar += "overflow-x:auto;overflow-y:hidden;scrollbar-width:none;scroll-snap-type:x proximity;"
		item = "flex:none;white-space:nowrap;scroll-snap-align:start;"
	}
	// The controlled form is a real radio group, so it gets the matching ARIA
	// roles; the uncontrolled form keeps its original attribute-for-attribute
	// markup (its buttons are not radios, and annotating them would be a lie).
	role := ""
	if path != "" {
		role = ` role="tablist"`
	}
	fmt.Fprintf(&r.sb, `<div class="qorm-tabbar"%s style=%q>`, role, bar)
	for i, lbl := range labels {
		cls := ""
		if i == active {
			cls = " qorm-tab-active"
		}
		if path == "" {
			click := ` onclick="qormTab(this)"`
			if n.OnChange != nil {
				click = fmt.Sprintf(` onclick="qorm(%d)"`, r.tabHandler(n, i, lbl))
			}
			style := ""
			if item != "" {
				style = fmt.Sprintf(` style=%q`, item)
			}
			fmt.Fprintf(&r.sb, `<button class="qorm-tab%s"%s data-tab="%d"%s>%s</button>`,
				cls, style, i, click, html.EscapeString(lbl))
			continue
		}
		change := ` onchange="qorm(-1)"`
		if n.OnChange != nil {
			change = fmt.Sprintf(` onchange="qorm(%d)"`, r.tabHandler(n, i, lbl))
		}
		checked := ""
		if i == active {
			checked = " checked"
		}
		fmt.Fprintf(&r.sb, `<label class="qorm-tab%s" data-tab="%d" style=%q role="tab" aria-selected="%t"><input type="radio" name=%q value="%d"%s%s%s style="position:absolute;opacity:0;width:0;height:0;">%s</label>`,
			cls, i, "display:inline-flex;align-items:center;position:relative;"+item, i == active,
			attrID(n.ID), i, checked, dataStateAttr(path), change, html.EscapeString(lbl))
	}
	r.sb.WriteString(`</div>`)
	for i, c := range n.Children {
		if lazy && i != active {
			continue
		}
		disp := "none"
		if i == active {
			disp = "block"
		}
		fmt.Fprintf(&r.sb, `<div class="qorm-tabpanel" data-panel="%d" style="display:%s;padding:12px 0;">`, i, disp)
		r.node(c)
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div>`)
}

// tabHandler registers one tab's onChange dispatch, carrying the tapped tab's
// {index} (0-based) and {tab} label on top of the author's own args — the same
// shape pagination uses for {page}, so an app pairs it with a one-line
// state.set action.
func (r *renderer) tabHandler(n *model.Node, i int, label string) int {
	args := mergeArgs(n.OnChange.Args, "index", strconv.Itoa(i))
	args["tab"] = label
	return r.register(&model.Invoke{Name: n.OnChange.Name, Args: args})
}

// activeIndex resolves an `active` index prop — a literal or a binding — into
// [0, count). Absent means 0 (the pre-existing behaviour of every widget that
// gained this prop), and an out-of-range index CLAMPS rather than selecting
// nothing: a stale or overshot counter still shows a usable tab/panel the user
// can navigate back from, exactly as pageWindow clamps an overshot page.
func (r *renderer) activeIndex(n *model.Node, count int) int {
	v := r.numProp(n, "active")
	if v == nil || count <= 0 {
		return 0
	}
	switch i := int(*v); {
	case i < 0:
		return 0
	case i >= count:
		return count - 1
	default:
		return i
	}
}

// tabIndicator emits the node-scoped CSS that styles the active tab, and
// nothing at all unless `indicator` or `indicatorColor` is declared (so the
// default tabs output is unchanged). A scoped <style> rather than inline styles
// is what lets the indicator survive CLIENT-side switching: qormTab moves the
// qorm-tab-active class between tabs, and a rule keyed on that class follows
// it, where an inline style would stay stuck on the tab that was active when
// the frame was rendered.
//
//   - `underline` (default) — the accent rule under the active tab.
//   - `pill` — a filled rounded capsule (Material/Chrome style).
//   - `none` — label emphasis only, no rule.
//
// !important is needed because the shell's .qorm-tab-active rule declares the
// underline colour !important. The scope is the tabs node's own id, so two tabs
// widgets on one screen can carry different indicators; an id that is not a
// plain identifier (which could not be written as a CSS selector) skips the
// block instead of emitting a broken — or injectable — one, and cssValue keeps
// a hand-written colour from closing the declaration.
func (r *renderer) tabIndicator(n *model.Node) {
	kind, color := propStr(n, "indicator"), cssValue(propStr(n, "indicatorColor"))
	if kind == "" && color == "" {
		return
	}
	if !isIdent(n.ID) {
		return
	}
	if color == "" {
		color = "var(--accent)"
	}
	decl := "border-bottom-color:" + color + " !important;color:" + color + " !important;"
	switch kind {
	case "none":
		decl = "border-bottom-color:transparent !important;color:" + color + " !important;"
	case "pill":
		decl = "border-bottom-color:transparent !important;background:" + color + ";color:#fff !important;border-radius:999px;"
	}
	fmt.Fprintf(&r.sb, `<style>#%s .qorm-tab-active{%s}</style>`, attrID(n.ID), html.EscapeString(decl))
}

// cssValue passes an author-written CSS value through, or returns "" when it
// carries a character that could end the declaration it is written into
// (";" "}" "{") or open markup ("<"). Values reach a <style> block, where HTML
// escaping alone would not stop a value from injecting extra rules, so the
// filter is a strict allowlist: colours, keywords, numbers, functions
// (var(--accent), color-mix(...), #0af, rgb(0 0 0 / 50%)) — nothing else.
func cssValue(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte(" #(),.%-_/", c) >= 0:
		default:
			return ""
		}
	}
	return s
}

// expansionTile is Flutter's ExpansionTile: a header that reveals its children
// (native <details>/<summary>, no JS required).
func (r *renderer) expansionTile(n *model.Node) {
	open := ""
	if propStr(n, "initiallyExpanded") == "true" {
		open = " open"
	}
	fmt.Fprintf(&r.sb, `<details id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+"border-bottom:1px solid var(--sep);", open)
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

// listTile is Flutter's ListTile: [leading] title / subtitle [trailing], tappable.
func (r *renderer) listTile(n *model.Node) {
	style := r.boxCSS(n) + "display:flex;align-items:center;gap:14px;padding:10px 14px;"
	if n.OnPress != nil {
		style += "cursor:pointer;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s%s>`, attrID(n.ID), style, a11y(n), r.pressAttr(n))
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

// table renders a data table from `columns` ([{key,title}] or strings) and
// `data` (bound array of objects or literal). A column may carry `width`
// (number = px, or a CSS string like "30%") applied via a <colgroup>.
// datatable is a richer table: sortable column headers (OnChange dispatches
// {column}, with a ▲/▼ indicator when it matches sortField/sortDir) and
// selectable rows (a checkbox column; OnPress dispatches {key} per row and
// {key:"__all__"} for select-all). rowKey (default "id") identifies rows; the
// sortHandler is a column header's default sort dispatch when the app wires
// no onChange: available when sortField and sortDir are plainly state-bound
// and the array to sort is too — `sortData` (an explicit bound path for apps
// whose `data` is a sliced/filtered window) or the bound `data` itself. It
// registers the runtime's built-in __sort: clicking the sorted column flips
// direction, a new column sorts ascending. An explicit onChange always wins.
func (r *renderer) sortHandler(n *model.Node, column string) (int, bool) {
	data := boundPath(propStr(n, "sortData"))
	if data == "" {
		data = boundPath(propStr(n, "data"))
	}
	field, dir := boundPath(propStr(n, "sortField")), boundPath(propStr(n, "sortDir"))
	if data == "" || field == "" || dir == "" {
		return 0, false
	}
	return r.register(&model.Invoke{Name: runtime.BuiltinSort, Args: map[string]string{
		"data": data, "field": field, "dir": dir, "column": column,
	}}), true
}

// tableChrome is the structural depth table/datatable share, resolved once per
// render. Every field is empty/nil unless its prop is declared, and an
// all-empty chrome writes not one extra byte — so an existing table renders
// exactly as it did before any of this existed:
//
//   - `scrollX` / `maxHeight` — wrap the <table> in its own scroll port. A
//     table is the one widget that legitimately outgrows the screen in BOTH
//     axes; wrapping keeps the overflow inside the widget instead of forcing
//     the whole page to scroll sideways.
//   - `stickyHeader` (+ `stickyTop`) — position:sticky column headers. Sticky
//     pins against the nearest scroll port, which is why it pairs with
//     `maxHeight` (pin inside the table's own port) or works against the page
//     scroll otherwise; `stickyTop` parks the header under anything already
//     pinned above it (an appbar), exactly as the list's section headers do.
//   - column `sticky` — freeze a leading column against the left edge while
//     the rest scrolls under it (see stickyColumns).
//   - `minWidth` — the width the table refuses to squeeze below, which is what
//     gives `scrollX` something to scroll.
//
// It is pure CSS on both counts: no scroll listeners, no measured offsets, no
// second "frozen" table cloned beside the real one.
type tableChrome struct {
	open, close string   // scroll-port wrapper, or "" for no wrapper
	head        string   // sticky-header CSS applied to every <th>
	col         []string // per-column freeze CSS, aligned with optionList
}

// tableWidth is the `minWidth` CSS a table refuses to squeeze below — what
// gives `scrollX` something to scroll — or "" when the prop is absent.
func (r *renderer) tableWidth(n *model.Node) string {
	if w := r.numProp(n, "minWidth"); w != nil {
		return "min-width:" + num(*w) + "px;"
	}
	return ""
}

func (r *renderer) tableChrome(n *model.Node, selectable bool) tableChrome {
	var tc tableChrome
	wrap := ""
	if propBool(n, "scrollX") {
		wrap += "overflow-x:auto;"
	}
	if h := r.numProp(n, "maxHeight"); h != nil {
		wrap += "overflow-y:auto;max-height:" + num(*h) + "px;"
	}
	if wrap != "" {
		tc.open, tc.close = `<div style="`+wrap+`">`, `</div>`
	}
	if propBool(n, "stickyHeader") {
		tc.head = "position:sticky;top:" + num(propNum(n, "stickyTop", 0)) + "px;z-index:2;background:var(--bg);"
	}
	tc.col = stickyColumns(n.Props["columns"], selectable)
	return tc
}

// cellAttr returns the style attribute for column i's <th> (head) or <td>, or
// "" when neither a sticky header nor a frozen column touches it. The corner
// cell — a frozen column's header — is lifted above both so it never slides
// under the cells it is meant to cover.
func (tc tableChrome) cellAttr(i int, head bool) string {
	css := ""
	if head {
		css = tc.head
	}
	if i >= 0 && i < len(tc.col) && tc.col[i] != "" {
		css += tc.col[i]
		if head {
			css += "z-index:3;"
		}
	}
	if css == "" {
		return ""
	}
	return ` style="` + css + `"`
}

// stickyColumns returns, per column, the CSS that freezes it against the left
// edge while the table scrolls under it — or nil when no column declares
// `sticky`, which keeps the no-op path allocation-free and byte-identical.
//
// The left offset is the running sum of the frozen columns already placed
// (plus datatable's 36px checkbox column), so a frozen column wants an explicit
// numeric `width`: a column sized by content, or in %, contributes 0 and the
// next frozen column would land on top of it. Freezing is therefore meant for
// the leading identity column(s), not for arbitrary columns mid-table.
func stickyColumns(v any, selectable bool) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	left, on := 0.0, false
	if selectable {
		left = 36 // .qdt-check is a fixed 36px column
	}
	for _, e := range arr {
		switch t := e.(type) {
		case string:
			out = append(out, "")
		case map[string]any:
			if !asBool(t["sticky"]) {
				out = append(out, "")
				continue
			}
			on = true
			out = append(out, "position:sticky;left:"+num(left)+"px;z-index:1;background:var(--bg);")
			left += pxWidth(colWidth(t["width"]))
		}
	}
	if !on {
		return nil
	}
	return out
}

// pxWidth is the pixel value of a normalized column width ("72px"), or 0 for a
// width that is unset or not expressed in pixels.
func pxWidth(w string) float64 {
	if !strings.HasSuffix(w, "px") {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(w, "px"), 64)
	if err != nil {
		return 0
	}
	return f
}

// cellTemplates maps a column key to the child node that renders that column's
// cells, so a table cell can be a WIDGET (a badge, an avatar, a button, a
// nested list) instead of the escaped fmt.Sprint of the field. A table's
// children were previously ignored, so declaring one is purely additive:
//
//	{"type":"datatable","columns":[{"value":"name"},{"value":"state"}],
//	 "children":[{"type":"tag","column":"state","label":"{{row.state}}"}]}
//
// The template renders in the ROW's scope, built by the same itemScope the list
// uses: the row object under `as` (default `row`, so `{{row.name}}`) plus the
// derived index/first/last keys (`rowIndex`, `rowFirst`, `rowLast` — see
// ListAliasNames), and one more binding, `cell`, holding this cell's own
// {value, column, index}. All of them are dotted paths ({{row.x}},
// {{cell.value}}) because a bare {{cell}} is what the loader's static check
// flags as a missing state./prop. prefix.
//
// A column with no template keeps the plain-text cell, so tables mix the two
// freely; a template naming a column that does not exist is simply never
// reached, and the first declaration wins if two children claim one column.
func (r *renderer) cellTemplates(n *model.Node) map[string]*model.Node {
	var m map[string]*model.Node
	for _, c := range n.Children {
		col := propStr(c, "column")
		if col == "" {
			continue
		}
		if m == nil {
			m = make(map[string]*model.Node, len(n.Children))
		}
		if _, dup := m[col]; !dup {
			m[col] = c
		}
	}
	return m
}

// detailTemplate is the child marked `detail: true` — the expandable row body
// rendered under every row inside a native <details>, so row expansion costs no
// JS and no round-trip. Its `label` is the disclosure line's text.
func (r *renderer) detailTemplate(n *model.Node) *model.Node {
	for _, c := range n.Children {
		if propBool(c, "detail") {
			return c
		}
	}
	return nil
}

// tableCells writes one row's <td>s: a cell template's widget where the column
// has one, the escaped field value otherwise. The `cell` binding is written
// into the row scope the caller installed — templated cells are the only path
// that touches the scope, so a template-less table never builds one.
func (r *renderer) tableCells(cols []option, obj map[string]any, tpls map[string]*model.Node, tc tableChrome) {
	for i, c := range cols {
		attr := tc.cellAttr(i, false)
		tpl := tpls[c.value]
		if tpl == nil {
			r.sb.WriteString("<td" + attr + ">" + html.EscapeString(fmt.Sprint(obj[c.value])) + "</td>")
			continue
		}
		r.scope["cell"] = map[string]any{"value": obj[c.value], "column": c.value, "index": i}
		r.sb.WriteString("<td" + attr + ">")
		r.node(tpl)
		r.sb.WriteString("</td>")
	}
}

// detailRow writes a row's expandable body as a full-width <details> row.
func (r *renderer) detailRow(tpl *model.Node, span int) {
	fmt.Fprintf(&r.sb, `<tr><td colspan="%d" style="padding:0 12px 8px;border-bottom:1px solid var(--sep);"><details><summary style="cursor:pointer;list-style:none;color:var(--label2);font-size:13px;padding:4px 0;">%s ▾</summary><div style="padding:6px 0;">`,
		span, html.EscapeString(r.interp(labelOf(tpl))))
	r.node(tpl)
	r.sb.WriteString(`</div></details></td></tr>`)
}

// bound `selected` array holds the chosen keys. Columns accept `width` too.
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
	// Sorted column: persistent accent chevron (▼ desc / ▲ asc). Unsorted:
	// the indicator only appears on header hover (macOS Finder convention).
	ind := "&#8250;"
	indCls := "qorm-sort-ind"
	if sortDir == "desc" {
		ind = "▾"
	} else {
		ind = "▴"
	}
	allSel := len(rows) > 0
	for _, row := range rows {
		if obj, ok := row.(map[string]any); ok && !selSet[fmt.Sprint(obj[rowKey])] {
			allSel = false
			break
		}
	}
	tc := r.tableChrome(n, selectable)
	tpls, detail := r.cellTemplates(n), r.detailTemplate(n)
	r.sb.WriteString(tc.open)
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-datatable" style=%q>`, attrID(n.ID), r.boxCSS(n)+r.tableWidth(n))
	r.sb.WriteString(colGroup(colWidths(n.Props["columns"]), selectable))
	r.sb.WriteString("<thead><tr>")
	if selectable {
		box := checkboxCell(allSel)
		if n.OnPress != nil {
			idx := r.register(&model.Invoke{Name: n.OnPress.Name, Args: mergeArgs(n.OnPress.Args, "key", "__all__")})
			fmt.Fprintf(&r.sb, `<th class="qdt-check"%s><span onclick="qorm(%d)" style="cursor:pointer;font-size:16px;">%s</span></th>`, tc.cellAttr(-1, true), idx, box)
		} else {
			fmt.Fprintf(&r.sb, `<th class="qdt-check"%s></th>`, tc.cellAttr(-1, true))
		}
	}
	for i, c := range cols {
		attr := tc.cellAttr(i, true)
		idx, sortable := -1, false
		if n.OnChange != nil {
			idx = r.register(&model.Invoke{Name: n.OnChange.Name, Args: mergeArgs(n.OnChange.Args, "column", c.value)})
			sortable = true
		} else if h, def := r.sortHandler(n, c.value); def {
			idx, sortable = h, true
		}
		if !sortable {
			r.sb.WriteString("<th" + attr + ">" + html.EscapeString(c.label) + "</th>")
			continue
		}
		cls, glyph := indCls, "&#8250;"
		if c.value == sortField {
			cls, glyph = indCls+" on", ind
		}
		fmt.Fprintf(&r.sb, `<th%s><button class="qdt-sort" onclick="qorm(%d)">%s<span class="%s">%s</span></button></th>`,
			attr, idx, html.EscapeString(c.label), cls, glyph)
	}
	r.sb.WriteString("</tr></thead><tbody>")
	prev, prevSuf := r.scope, r.idSuffix
	alias, idxKey, firstKey, lastKey := ListAliasNames(propStrOr(n, "as", "row"))
	for ri, row := range rows {
		obj, _ := row.(map[string]any)
		if tpls != nil || detail != nil {
			r.scope = itemScope(prev, alias, idxKey, firstKey, lastKey, row, ri, len(rows))
			r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, ri)
		}
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
				fmt.Fprintf(&r.sb, `<td class="qdt-check"%s><span onclick="qorm(%d)" style="cursor:pointer;font-size:16px;">%s</span></td>`, tc.cellAttr(-1, false), idx, box)
			} else {
				fmt.Fprintf(&r.sb, `<td class="qdt-check"%s>%s</td>`, tc.cellAttr(-1, false), box)
			}
		}
		r.tableCells(cols, obj, tpls, tc)
		r.sb.WriteString("</tr>")
		if detail != nil {
			span := len(cols)
			if selectable {
				span++
			}
			r.detailRow(detail, span)
		}
	}
	r.scope, r.idSuffix = prev, prevSuf
	r.sb.WriteString("</tbody></table>")
	r.sb.WriteString(tc.close)
}

func (r *renderer) table(n *model.Node) {
	cols := optionList(n.Props["columns"]) // reuse {value,label} shape: value=key, label=title
	rows := r.boundArray(n, "data")
	sortField := r.interp(propStr(n, "sortField"))
	sortDir := r.interp(propStr(n, "sortDir"))
	tc := r.tableChrome(n, false)
	tpls, detail := r.cellTemplates(n), r.detailTemplate(n)
	r.sb.WriteString(tc.open)
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-table" style=%q>`, attrID(n.ID), r.boxCSS(n)+r.tableWidth(n))
	r.sb.WriteString(colGroup(colWidths(n.Props["columns"]), false))
	r.sb.WriteString("<thead><tr>")
	for i, c := range cols {
		attr := tc.cellAttr(i, true)
		idx, sortable := -1, false
		if n.OnChange != nil { // app-wired sort: header dispatches onChange with {column}
			args := map[string]string{"column": c.value}
			for k, v := range n.OnChange.Args {
				args[k] = v
			}
			idx = r.register(&model.Invoke{Name: n.OnChange.Name, Args: args})
			sortable = true
		} else if h, def := r.sortHandler(n, c.value); def {
			idx, sortable = h, true
		}
		if !sortable {
			r.sb.WriteString("<th" + attr + ">" + html.EscapeString(c.label) + "</th>")
			continue
		}
		// macOS Finder convention: the chevron only shows on hover, except on
		// the sorted column (persistent accent ▴ asc / ▾ desc).
		cls, glyph := "qorm-sort-ind", "&#8250;"
		if c.value == sortField && sortField != "" {
			cls = "qorm-sort-ind on"
			if sortDir == "desc" {
				glyph = "▾"
			} else {
				glyph = "▴"
			}
		}
		fmt.Fprintf(&r.sb, `<th%s><button class="qdt-sort" onclick="qorm(%d)">%s<span class="%s">%s</span></button></th>`,
			attr, idx, html.EscapeString(c.label), cls, glyph)
	}
	r.sb.WriteString("</tr></thead><tbody>")
	prev, prevSuf := r.scope, r.idSuffix
	alias, idxKey, firstKey, lastKey := ListAliasNames(propStrOr(n, "as", "row"))
	for ri, row := range rows {
		obj, _ := row.(map[string]any)
		if tpls != nil || detail != nil {
			r.scope = itemScope(prev, alias, idxKey, firstKey, lastKey, row, ri, len(rows))
			r.idSuffix = fmt.Sprintf("%s-%d", prevSuf, ri)
		}
		r.sb.WriteString("<tr>")
		r.tableCells(cols, obj, tpls, tc)
		r.sb.WriteString("</tr>")
		if detail != nil {
			r.detailRow(detail, len(cols))
		}
	}
	r.scope, r.idSuffix = prev, prevSuf
	r.sb.WriteString("</tbody></table>")
	r.sb.WriteString(tc.close)
}

// colWidths returns the optional per-column `width` of a table columns prop,
// aligned with optionList's output ("" for width-less columns and for the
// plain-string column form).
func colWidths(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		switch t := e.(type) {
		case string:
			out = append(out, "")
		case map[string]any:
			out = append(out, colWidth(t["width"]))
		}
	}
	return out
}

// colWidth normalizes a column `width`: a number means px, a string passes
// through as CSS (a bare numeric string still means px). A numeric string is
// trimmed before appending px so " 50 " yields valid CSS ("50px"), not the
// untrimmed " 50 px" the browser drops as malformed.
func colWidth(v any) string {
	switch t := v.(type) {
	case float64:
		return num(t) + "px"
	case string:
		trimmed := strings.TrimSpace(t)
		if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return trimmed + "px"
		}
		return t
	}
	return ""
}

// colGroup emits a <colgroup> sizing the columns when any carries a width —
// otherwise "", and the table lays out as before. extraLeading prepends an
// unsized <col> for datatable's checkbox column.
func colGroup(widths []string, extraLeading bool) string {
	anyW := false
	for _, w := range widths {
		if w != "" {
			anyW = true
			break
		}
	}
	if !anyW {
		return ""
	}
	var b strings.Builder
	b.WriteString("<colgroup>")
	if extraLeading {
		b.WriteString("<col>")
	}
	for _, w := range widths {
		if w == "" {
			b.WriteString("<col>")
		} else {
			fmt.Fprintf(&b, `<col style="width:%s">`, html.EscapeString(w))
		}
	}
	b.WriteString("</colgroup>")
	return b.String()
}

// accordion stacks its children as headed panels, one open at a time. Which
// one starts open is `active` — a literal or a binding, clamped into range and
// defaulting to the first panel, so an accordion that declares nothing renders
// as it always did. Toggling stays client-side (qormAcc).
func (r *renderer) accordion(n *model.Node) {
	active := r.activeIndex(n, len(n.Children))
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;border:1px solid var(--sep);border-radius:10px;overflow:hidden;")
	for i, c := range n.Children {
		open := i == active
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

// rating renders filled/empty stars from `value` out of `max` (default 5).
func (r *renderer) rating(n *model.Node) {
	val := int(asFloat(runtime.EvalBinding(propStr(n, "value"), r.ctx())))
	max := int(propNum(n, "max", 5))
	style := r.boxCSS(n) + "display:inline-flex;gap:2px;font-size:" + num(propNum(n, "size", 18)) + "px;color:#f59e0b;"
	sz := propNum(n, "size", 18)
	fmt.Fprintf(&r.sb, `<span id=%q style=%q role="img" aria-label="%d of %d">`, attrID(n.ID), style, val, max)
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;align-items:center;gap:6px;")
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;gap:8px;align-items:center;font-size:14px;color:var(--label2);")
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;gap:6px;align-items:center;")
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

// tree renders a nested, natively-collapsible view from `data`
// ([{label,children}]). Every branch is expanded by default; `collapsed: true`
// flips the default for the whole tree, and a node's own `expanded` field
// overrides it either way — so a big tree can ship folded with the path to the
// interesting node already open. Collapsing is native <details>, so it survives
// with no JS and no state round-trip.
func (r *renderer) tree(n *model.Node) {
	open := !propBool(n, "collapsed")
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-tree" style=%q>`, attrID(n.ID), r.boxCSS(n)+"font-size:14px;")
	for _, it := range r.boundArray(n, "data") {
		r.treeItem(it, open)
	}
	r.sb.WriteString(`</div>`)
}

func (r *renderer) treeItem(v any, def bool) {
	obj, _ := v.(map[string]any)
	label := ""
	if obj != nil {
		label = fmt.Sprint(obj["label"])
	} else {
		label = fmt.Sprint(v)
	}
	kids, _ := obj["children"].([]any)
	if len(kids) == 0 {
		fmt.Fprintf(&r.sb, `<div class="qorm-tree-leaf">%s</div>`, html.EscapeString(label))
		return
	}
	// A node's own `expanded` decides only that node: its descendants keep the
	// tree default, so re-opening a folded branch does not reveal a subtree
	// that was folded only because its parent was.
	open := def
	if e, ok := obj["expanded"]; ok {
		open = asBool(e)
	}
	att := ""
	if open {
		att = " open"
	}
	fmt.Fprintf(&r.sb, `<details class="qorm-tree-n"%s><summary class="qorm-tree-sum">%s</summary><div class="qorm-tree-kids">`, att, html.EscapeString(label))
	for _, c := range kids {
		r.treeItem(c, def)
	}
	r.sb.WriteString(`</div></details>`)
}

// timeline renders a vertical dotted timeline from `items` ([{title,text}]).
// Three optional per-item fields give an event its own identity without the app
// dropping to a hand-built list: `color` (the marker's colour — status per
// event, not one accent for the whole feed), `icon` (a built-in icon name drawn
// inside the marker, which grows it to a 20px disc) and `time` (a small
// timestamp line above the title). An item that carries none of them renders
// byte-identically to before they existed. `color` is filtered through cssValue
// and escaped: it lands in a style attribute, so it may not carry a quote.
func (r *renderer) timeline(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;")
	items := r.boundArray(n, "items")
	for i, it := range items {
		obj, _ := it.(map[string]any)
		title, textv := "", ""
		if obj != nil {
			title, textv = fmt.Sprint(obj["title"]), fmt.Sprint(obj["text"])
		} else {
			title = fmt.Sprint(it)
		}
		color := "var(--accent)"
		if c, ok := obj["color"].(string); ok && cssValue(c) != "" {
			color = html.EscapeString(c)
		}
		marker := `<span style="width:12px;height:12px;border-radius:50%;background:` + color + `;flex-shrink:0;margin-top:3px;"></span>`
		if name, ok := obj["icon"].(string); ok {
			if svg := iconSVG(name, 12); svg != "" {
				marker = `<span style="display:inline-flex;align-items:center;justify-content:center;width:20px;height:20px;border-radius:50%;background:` +
					color + `;color:#fff;flex-shrink:0;">` + svg + `</span>`
			}
		}
		stamp := ""
		if ts, ok := obj["time"].(string); ok && ts != "" {
			stamp = `<div style="font-size:12px;color:var(--label2);">` + html.EscapeString(ts) + `</div>`
		}
		line := "flex:1;width:2px;background:var(--sep);"
		if i == len(items)-1 {
			line = ""
		}
		fmt.Fprintf(&r.sb, `<div style="display:flex;gap:12px;">`+
			`<div style="display:flex;flex-direction:column;align-items:center;">%s<span style="%s"></span></div>`+
			`<div style="padding-bottom:16px;">%s<div style="font-weight:600;font-size:14px;color:var(--label);">%s</div><div style="font-size:13px;color:var(--label2);">%s</div></div></div>`,
			marker, line, stamp, html.EscapeString(title), html.EscapeString(textv))
	}
	r.sb.WriteString(`</div>`)
}

// stat renders a metric: big value, label, and an optional colored delta.
func (r *renderer) stat(n *model.Node) {
	value := r.interp(propStr(n, "value"))
	if value == "" {
		value = r.interp(n.Text)
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:2px;")
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), style)
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

// descriptions renders a two-column key/value list from `items` ([{label,value}]).
func (r *renderer) descriptions(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:grid;grid-template-columns:auto 1fr;gap:8px 16px;font-size:14px;")
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

// materialStepper is Flutter's Stepper: a vertical list of steps with a title,
// a connector, and the active step's content plus continue/cancel controls.
// The active index comes from `active` (a binding); step titles from `steps`,
// content from children (one per step).
func (r *renderer) materialStepper(n *model.Node) {
	active := int(asFloat(runtime.EvalBinding(propStr(n, "active"), r.ctx())))
	titles := r.boundArray(n, "steps")
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;")
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

// listSection is Cupertino's CupertinoListSection: an inset grouped list — an
// uppercase header over a rounded surface card whose children are separated by
// inset hairlines (the standard iOS Settings-style list).
func (r *renderer) listSection(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"padding:16px;")
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
