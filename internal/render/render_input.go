package render

import (
	"fmt"
	"html"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+"position:relative;height:32px;", a11y(n))
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
