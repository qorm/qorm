package render

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

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

	reorderH := -1
	if propBool(n, "reorderable") {
		if inv := parseInvokeProp(n, "onReorder"); inv != nil {
			reorderH = r.register(inv)
		}
	}
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
	if reorderH >= 0 && n.ID != "" {
		fmt.Fprintf(&r.sb, `<script>setTimeout(function(){qormReorder(document.getElementById(%q),%d)})</script>`, n.ID, reorderH)
	}
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

// table renders a data table from `columns` ([{key,title}] or strings) and
// `data` (bound array of objects or literal). A column may carry `width`
// (number = px, or a CSS string like "30%") applied via a <colgroup>.
// datatable is a richer table: sortable column headers (OnChange dispatches
// {column}, with a ▲/▼ indicator when it matches sortField/sortDir) and
// selectable rows (a checkbox column; OnPress dispatches {key} per row and
// {key:"__all__"} for select-all). rowKey (default "id") identifies rows; the
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
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-datatable" style=%q>`, n.ID, r.boxCSS(n))
	r.sb.WriteString(colGroup(colWidths(n.Props["columns"]), selectable))
	r.sb.WriteString("<thead><tr>")
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

func (r *renderer) table(n *model.Node) {
	cols := optionList(n.Props["columns"]) // reuse {value,label} shape: value=key, label=title
	rows := r.boundArray(n, "data")
	fmt.Fprintf(&r.sb, `<table id=%q class="qorm-table" style=%q>`, n.ID, r.boxCSS(n))
	r.sb.WriteString(colGroup(colWidths(n.Props["columns"]), false))
	r.sb.WriteString("<thead><tr>")
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
// through as CSS (a bare numeric string still means px).
func colWidth(v any) string {
	switch t := v.(type) {
	case float64:
		return num(t) + "px"
	case string:
		if _, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return t + "px"
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
