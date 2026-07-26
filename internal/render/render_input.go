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
	typeAttr, onclick := r.submitAttrs(n)
	fmt.Fprintf(&r.sb, `<button id=%q class="qorm-tap"%s style=%q%s%s%s>%s</button>`,
		attrID(n.ID), typeAttr, style, a11y(n), r.enterAttr(n), onclick, html.EscapeString(r.interp(labelOf(n))))
}

// enterAttr marks a button as the ENTER TARGET of the form it sits in.
//
// The gap it closes: a QORM form fires its own action from the form's onPress
// (`onsubmit`), so a form that instead puts the action on a BUTTON has nothing
// wired to Enter — the browser's implicit submission reaches a form whose
// onsubmit is `return false`, and the human's Return key does nothing. The
// button cannot be found by guessing (a form usually holds several, and picking
// "the first one" would fire Cancel as often as Save), so the app names it:
// `submitOnEnter: true` emits data-qorm-enter, and app.js's one delegated
// keydown listener clicks THAT button — re-read from the live DOM at keypress
// time, so the handler index in its onclick is always the current frame's.
//
// Enter inside a textarea stays a newline, and the click runs the button's own
// wiring, which means a `submit: true` button keeps its native validity gate.
// Opt-in: a button without the prop emits exactly the bytes it did before.
func (r *renderer) enterAttr(n *model.Node) string {
	if propBool(n, "submitOnEnter") {
		return ` data-qorm-enter`
	}
	return ""
}

// submitAttrs resolves a button's participation in the enclosing form's NATIVE
// constraint validation (`required` / `pattern` / `maxlength` / type=email…,
// emitted by inputAttrs) and returns the button's `type` attribute plus its
// press wiring.
//
// The problem it solves: a QORM button dispatches through `onclick="qorm(N)"`,
// and a click handler runs BEFORE — and independently of — the browser's
// constraint check, so an action fired that way ignores every native
// constraint. (The form's own `onsubmit` is already protected: a <button> with
// no type attribute defaults to type=submit, so clicking it, or pressing Enter
// in a field, goes through form submission, which the browser refuses to start
// while a constraint fails. The leak is specifically the button's OWN onPress.)
//
// The `submit` prop makes that participation explicit — it is a prop, not the
// HTML-native spelling `type`, because `type` is the node's widget name:
//
//	submit absent  no type attribute and the plain onclick — byte-identical to
//	               the pre-validation output, so existing apps are untouched.
//	submit: true   type="submit". If the button also has an onPress, its onclick
//	               is prefixed with a native validity gate: the action is not
//	               dispatched while the form reports a failing constraint, and
//	               reportValidity() raises the browser's own message bubble on
//	               the first offending field (zero client JS of ours — the gate
//	               is the standard HTMLFormElement API, inline).
//	submit: false  type="button" — the escape hatch for a Cancel/secondary
//	               button inside a form, which would otherwise implicitly submit
//	               it (and, once gated, be blocked by an unrelated invalid field).
//
// Opt-outs, both native: `novalidate: true` on the button emits the standard
// `formnovalidate` attribute (submit this form without validating it — for the
// save-a-draft / validate-on-the-server flow) and drops the gate; and the gate
// itself honours `form.noValidate`, so a form-level opt-out disables it too the
// moment the form renderer emits a `novalidate` attribute.
//
// Both props accept a literal or a `{{ binding }}`, so an agent can drive them
// from state like any other prop.
//
// Two notes on the gate. It is not feature-detected: reportValidity() shipped
// alongside fetch() (Safari 10.1, and long before that everywhere else), and
// the QORM client is already inert without fetch, so the gate can never be the
// sole reason a button goes dead. And it does not cancel the submission it
// guards — a gated button whose form passes validation runs its own action AND
// lets the browser submit, so a form that ALSO carries an onPress dispatches
// both. That is HTML's own semantics for a submit button, kept rather than
// papered over: put the action on the button or on the form, not on both.
func (r *renderer) submitAttrs(n *model.Node) (typeAttr, onclick string) {
	raw, ok := n.Prop("submit")
	if !ok {
		return "", r.pressAttr(n)
	}
	if !r.boolProp(raw) {
		return ` type="button"`, r.pressAttr(n)
	}
	if nv, ok := n.Prop("novalidate"); ok && r.boolProp(nv) {
		return ` type="submit" formnovalidate`, r.pressAttr(n)
	}
	if n.OnPress == nil {
		// Nothing to gate: the press IS the form submission, which the browser
		// already validates before firing the form's onsubmit.
		return ` type="submit"`, ""
	}
	// The gate is spelled with nested ifs rather than `&&` so the attribute value
	// carries no ampersand (an HTML attribute value is character-reference
	// decoded, and a bare `&` there is a parse hazard best not relied on).
	return ` type="submit"`, fmt.Sprintf(
		` onclick="if(this.form){if(!this.form.noValidate){if(!this.form.reportValidity())return;}}qorm(%d)"`,
		r.register(n.OnPress))
}

// boolProp resolves a boolean-ish prop VALUE that may be a JSON literal (true,
// "true", 1) or a `{{ binding }}` over state — the same resolution checkedState
// applies to `checked`.
func (r *renderer) boolProp(raw any) bool {
	return asBool(runtime.EvalBinding(fmt.Sprint(raw), r.ctx()))
}

// inputAttrs renders the shared native-HTML attribute set of the text-entry
// widgets (input / textarea / textformfield's inner input). All zero-JS: the
// browser enforces maxlength/readonly/required/pattern and picks the soft
// keyboard from inputmode. Every attribute is emitted only when its prop is
// set, so widgets without these props render byte-identically to before.
// `pattern` is an <input>-only attribute in HTML, so textarea skips it
// (inputmode/autocomplete/maxlength/readonly/required/autofocus all apply to
// textarea too). pattern+required open the native constraint-validation
// channel — a JS validation engine is deliberately out of scope here.
func (r *renderer) inputAttrs(n *model.Node, textarea bool) string {
	var b strings.Builder
	if v := int(propNum(n, "maxLength", 0)); v > 0 {
		fmt.Fprintf(&b, ` maxlength="%d"`, v)
	}
	if propBool(n, "autofocus") {
		b.WriteString(` autofocus`)
	}
	if propBool(n, "readonly") {
		b.WriteString(` readonly`)
	}
	if propBool(n, "required") {
		b.WriteString(` required`)
		// `requiredMessage` is the wording the browser's own "please fill this
		// field" bubble shows. It cannot ride on `title` (that one is APPENDED to
		// the pattern-mismatch bubble and says nothing for a missing value), so it
		// travels as data-qorm-error and app.js reconciles it onto the control with
		// setCustomValidity — only while validity.valueMissing is true, so the
		// field goes valid again the moment it is filled and the message can never
		// block a submission on its own.
		if m := r.interp(propStr(n, "requiredMessage")); m != "" {
			fmt.Fprintf(&b, ` data-qorm-error="%s"`, html.EscapeString(m))
		}
	}
	// Explicit "..." quoting + html.EscapeString rather than %q: Go's %q would
	// also backslash-escape the value (`\d+` becomes `\\d+`), which corrupts a
	// regex pattern; entity-encoding alone is what the quoted attribute needs.
	if v := propStr(n, "autocomplete"); v != "" {
		fmt.Fprintf(&b, ` autocomplete="%s"`, html.EscapeString(v))
	}
	if v := propStr(n, "inputMode"); v != "" {
		fmt.Fprintf(&b, ` inputmode="%s"`, html.EscapeString(normalizeInputMode(v)))
	}
	if !textarea {
		if v := propStr(n, "pattern"); v != "" {
			fmt.Fprintf(&b, ` pattern="%s"`, html.EscapeString(v))
		}
	}
	return b.String()
}

// errorEcho projects a textformfield's author-supplied `error` (a reactive
// expression over state) onto the two NATIVE channels the browser's own
// constraint validation uses, so the declarative error and the native one do
// not contradict each other:
//
//   - aria-invalid="true" — the same state assistive tech reads off a natively
//     invalid control, so a field the app considers wrong is announced as wrong
//     even when the browser is satisfied (a cross-field rule, a server verdict).
//   - title — the message the browser APPENDS to its pattern-mismatch bubble.
//     Without it the native bubble says only "please match the requested
//     format"; with it the author's own wording shows up inside the native
//     popup. Skipped when a11y already emitted the node's own `title` — a
//     second title attribute on the same element is a parse error, and the
//     browser keeps the FIRST one, so the echo has to yield rather than
//     silently produce dead markup. The check is on the emitted attributes
//     rather than the prop, so it cannot drift from what a11y decides.
//
// Both ride on the inner <input>, and only when the field opted into native
// validation via `required`/`pattern`: a field using nothing but the reactive
// `error` renders exactly the bytes it did before. The visual side stays as it
// was (red border + footer message), which composes with, rather than replaces,
// the browser's `:invalid` styling.
func (r *renderer) errorEcho(n *model.Node, errText, a11yAttrs string) string {
	if errText == "" || (!propBool(n, "required") && propStr(n, "pattern") == "") {
		return ""
	}
	out := ` aria-invalid="true"`
	if !strings.Contains(a11yAttrs, ` title=`) {
		out += fmt.Sprintf(` title="%s"`, html.EscapeString(errText))
	}
	return out
}

// changeWiring is changeAttr plus the opt-in `debounce`, and every control in
// this file dispatches through it.
//
// Without `debounce` it IS changeAttr — the native `onchange`, which for a text
// field means "when the human leaves it or presses Enter". That is the safe
// default and stays byte-identical.
//
// `debounce: 300` switches the control to per-keystroke dispatch that fires
// 300ms after the LAST keystroke: the wiring a search box wants, and the reason
// this is a prop rather than something the client could infer. It is spelled as
// data attributes instead of an inline handler because the dispatch is one
// delegated `input` listener in app.js (qormDebounceInit) — an inline
// oninput would re-arm a timer per rendered control, and a morph would leave
// the old ones running. The handler index travels on the element and is re-read
// when the timer fires, so a re-render that renumbers the handler table cannot
// dispatch the wrong action; -1 is the plain state-sync (a bound control with no
// onChange), the same convention qorm(-1) uses everywhere else. Leaving the
// field flushes a pending timer, so the last keystrokes are never swallowed.
func (r *renderer) changeWiring(n *model.Node, bound bool) string {
	ms := 0
	if v := r.numProp(n, "debounce"); v != nil && *v > 0 {
		ms = int(*v)
	}
	if ms == 0 {
		return r.changeAttr(n, bound)
	}
	h := -1
	if n.OnChange != nil {
		h = r.register(n.OnChange)
	} else if !bound {
		return "" // nothing to dispatch and nothing to sync: no wiring at all
	}
	return fmt.Sprintf(` data-qorm-debounce="%d" data-qorm-debounce-h="%d"`, ms, h)
}

// normalizeInputMode maps author-friendly aliases onto the HTML inputmode
// vocabulary (none/text/decimal/numeric/tel/search/email/url): "number" is the
// natural spelling for a numeric keypad but the spec token is "numeric", and
// "phone" reads better than "tel". Everything else passes through untouched —
// browsers ignore unknown inputmode values. Note inputmode is deliberately NOT
// derived from inputType: type=email/tel/url/number already select the right
// keyboard on their own (adding inputmode there would be redundant AND change
// the output of existing apps); inputMode exists for the complementary case —
// keep type="text" (no spinner/validation semantics) while still requesting a
// numeric/tel/etc. keyboard, e.g. a PIN or OTP field.
func normalizeInputMode(v string) string {
	switch v {
	case "number":
		return "numeric"
	case "phone":
		return "tel"
	}
	return v
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
	fmt.Fprintf(&r.sb, `<input id=%q type=%q value=%q placeholder=%q style=%q%s%s%s%s>`,
		attrID(n.ID), html.EscapeString(inputType), html.EscapeString(r.interp(n.Value)),
		html.EscapeString(n.Placeholder), style, dataStateAttr(path), a11y(n),
		r.inputAttrs(n, false), r.changeWiring(n, path != ""))
}

func (r *renderer) textarea(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n) + "outline:none;resize:vertical;"
	path := boundPath(n.Value)
	rows := int(propNum(n, "rows", 4))
	fmt.Fprintf(&r.sb, `<textarea id=%q rows="%d" placeholder=%q style=%q%s%s%s%s>%s</textarea>`,
		attrID(n.ID), rows, html.EscapeString(n.Placeholder), style, dataStateAttr(path), a11y(n),
		r.inputAttrs(n, true), r.changeWiring(n, path != ""), html.EscapeString(r.interp(n.Value)))
}

func (r *renderer) selectBox(n *model.Node) {
	style := r.boxCSS(n) + r.textCSS(n)
	path := boundPath(n.Value)
	cur := r.interp(n.Value)
	fmt.Fprintf(&r.sb, `<select id=%q style=%q%s%s%s>`, attrID(n.ID), style, dataStateAttr(path), a11y(n), r.changeWiring(n, path != ""))
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
		fmt.Fprintf(&r.sb, `<label id=%q style=%q%s>`, attrID(n.ID),
			r.boxCSS(n)+"display:inline-flex;align-items:center;gap:10px;cursor:pointer;font-size:15px;", a11y(n))
		if label != "" {
			fmt.Fprintf(&r.sb, `<span style="flex:1;">%s</span>`, label)
		}
		fmt.Fprintf(&r.sb, `<span class="qorm-switch"><input type="checkbox"%s%s%s><span></span></span></label>`,
			checked, dataStateAttr(path), r.changeWiring(n, path != ""))
		return
	}
	fmt.Fprintf(&r.sb, `<label id=%q style=%q%s><input type="checkbox"%s style="width:18px;height:18px;accent-color:var(--accent);"%s%s>%s</label>`,
		attrID(n.ID), r.boxCSS(n)+"display:inline-flex;align-items:center;gap:8px;cursor:pointer;", a11y(n),
		checked, dataStateAttr(path), r.changeWiring(n, path != ""), label)
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
	// The radio-group name is the node id; like the id attribute it is
	// author-controlled and lands in a quoted HTML attribute, so it needs the
	// same entity-encoding (%q alone leaves the quote-breakout open).
	name := attrID(n.ID)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:6px;", a11y(n))
	for _, opt := range optionList(n.Props["options"]) {
		checked := ""
		if opt.value == cur {
			checked = " checked"
		}
		fmt.Fprintf(&r.sb, `<label style="display:inline-flex;align-items:center;gap:8px;cursor:pointer;"><input type="radio" name=%q value=%q%s%s%s>%s</label>`,
			name, html.EscapeString(opt.value), checked, dataStateAttr(path), r.changeWiring(n, path != ""),
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
		attrID(n.ID), num(min), num(max), num(step), num(val), r.boxCSS(n)+fill, dataStateAttr(path), a11y(n), r.changeWiring(n, path != ""))
}

// field wraps a control with a label, required marker, and a conditional error
// (shown when the `error` binding is non-empty) or help text.
func (r *renderer) field(n *model.Node) {
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:5px;")
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
// so the existing state-fold mechanism applies). With `multiple: true` it is
// Flutter's ToggleButtons — see segmentedMulti.
func (r *renderer) segmented(n *model.Node) {
	if propBool(n, "multiple") {
		r.segmentedMulti(n)
		return
	}
	path := boundPath(n.Value)
	cur := r.interp(n.Value)
	changeAttr := r.changeWiring(n, path != "")
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-seg" style=%q role="radiogroup">`, attrID(n.ID),
		r.boxCSS(n)+"display:inline-flex;background:var(--fill);border-radius:8px;padding:3px;gap:2px;")
	for _, opt := range optionList(n.Props["options"]) {
		checked := ""
		if opt.value == cur {
			checked = " checked"
		}
		fmt.Fprintf(&r.sb, `<label style="position:relative;"><input type="radio" name=%q value=%q%s%s%s style="position:absolute;opacity:0;width:0;height:0;"><span style="display:inline-block;padding:6px 14px;border-radius:6px;font-size:13px;cursor:pointer;%s">%s</span></label>`,
			attrID(n.ID), html.EscapeString(opt.value), checked, dataStateAttr(path), changeAttr,
			segStyle(opt.value == cur), html.EscapeString(opt.label))
	}
	r.sb.WriteString(`</div>`)
}

// segmentedMulti is the multiple:true form of segmented (ToggleButtons): the
// bound value path holds an ARRAY of selected values and an option is "on"
// while its value is a member. Clicking an option dispatches the node's
// onChange with {value} — pair it with a state.toggle step to flip membership,
// so selection still flows only through state (no data-state on the options:
// the client must not overwrite the array with a scalar).
func (r *renderer) segmentedMulti(n *model.Node) {
	sel := map[string]bool{}
	if arr, ok := runtime.EvalBinding(n.Value, r.ctx()).([]any); ok {
		for _, v := range arr {
			sel[runtime.Stringify(v)] = true
		}
	}
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-seg" style=%q role="group">`, attrID(n.ID),
		r.boxCSS(n)+"display:inline-flex;background:var(--fill);border-radius:8px;padding:3px;gap:2px;")
	for _, opt := range optionList(n.Props["options"]) {
		on := sel[opt.value]
		attr := ""
		if n.OnChange != nil {
			attr = fmt.Sprintf(` onclick="qorm(%d)"`, r.register(&model.Invoke{Name: n.OnChange.Name, Args: mergeArgs(n.OnChange.Args, "value", opt.value)}))
		}
		fmt.Fprintf(&r.sb, `<span role="button" aria-pressed="%t" style="display:inline-block;padding:6px 14px;border-radius:6px;font-size:13px;cursor:pointer;%s"%s>%s</span>`,
			on, segStyle(on), attr, html.EscapeString(opt.label))
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
	fmt.Fprintf(&r.sb, `<span id=%q style=%q%s%s>`, attrID(n.ID), r.boxCSS(n)+style, a11y(n), r.pressAttr(n))
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
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, attrID(n.ID), r.boxCSS(n)+"position:relative;height:32px;", a11y(n))
	track := "position:absolute;left:0;right:0;top:14px;width:100%;margin:0;-webkit-appearance:none;background:transparent;pointer-events:none;"
	fmt.Fprintf(&r.sb, `<input type="range" min=%q max=%q step=%q value=%q style=%q class="qorm-range-lo"%s%s>`,
		num(min), num(max), num(step), num(lo), track, dataStateAttr(loPath), r.changeWiring(n, loPath != ""))
	fmt.Fprintf(&r.sb, `<input type="range" min=%q max=%q step=%q value=%q style=%q class="qorm-range-hi"%s%s>`,
		num(min), num(max), num(step), num(hi), track, dataStateAttr(hiPath), r.changeWiring(n, hiPath != ""))
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
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-menu" style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;display:inline-block;")
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
// <datalist> of suggestions; the value two-way-binds to state. `options` may be
// a literal array or a binding to a state array (e.g. "{{state.suggestions}}").
func (r *renderer) autocomplete(n *model.Node) {
	path := boundPath(n.Value)
	listID := n.ID + "-ac"
	style := r.boxCSS(n)
	if style == "" {
		style = "height:40px;padding:0 12px;border:1px solid var(--sep);border-radius:8px;font-size:14px;"
	}
	fmt.Fprintf(&r.sb, `<input id=%q list=%q value=%q placeholder=%q style=%q%s%s%s>`,
		attrID(n.ID), attrID(listID), html.EscapeString(r.interp(n.Value)), html.EscapeString(n.Placeholder),
		style, dataStateAttr(path), a11y(n), r.changeWiring(n, path != ""))
	fmt.Fprintf(&r.sb, `<datalist id=%q>`, attrID(listID))
	for _, o := range optionList(r.boundArray(n, "options")) {
		lbl := o.label
		if lbl == "" {
			lbl = o.value
		}
		fmt.Fprintf(&r.sb, `<option value=%q>`, html.EscapeString(lbl))
	}
	r.sb.WriteString(`</datalist>`)
}

// searchbar is Flutter's SearchBar + SearchAnchor: a text field with an
// anchored results panel. `value` two-way-binds the query text; `hint` is the
// placeholder. `items` ([{label, detail?, icon?}] — a literal array or a
// binding to a state array, e.g. "{{state.results}}") renders the result
// entries. While the input is focused and non-empty the panel lists the items
// whose label contains the query (client-side filtering in qormSearch);
// clicking an entry fills the input and dispatches onSelect with the entry's
// {label} as a plain string. Escape or clicking away closes the panel.
func (r *renderer) searchbar(n *model.Node) {
	path := boundPath(n.Value)
	onSelect := parseInvokeProp(n, "onSelect")
	fmt.Fprintf(&r.sb, `<div id=%q class="qorm-search" style=%q>`, attrID(n.ID), r.boxCSS(n)+"position:relative;display:inline-block;")
	fmt.Fprintf(&r.sb, `<input type="text" value=%q placeholder=%q autocomplete="off" style="width:100%%;min-width:220px;box-sizing:border-box;height:40px;padding:0 12px;border:1px solid var(--sep);border-radius:8px;font-size:14px;outline:none;"%s%s%s onfocus="qormSearch(this)" oninput="qormSearch(this)" onblur="qormSearchBlur(this)" onkeydown="qormSearchKey(this,event)">`,
		html.EscapeString(r.interp(n.Value)), html.EscapeString(propStrOr(n, "hint", n.Placeholder)),
		dataStateAttr(path), a11y(n), r.changeWiring(n, path != ""))
	r.sb.WriteString(`<div class="qorm-search-panel" style="display:none;position:absolute;top:100%;left:0;right:0;margin-top:4px;background:var(--surface);border:1px solid var(--sep);border-radius:8px;box-shadow:0 8px 24px rgba(0,0,0,.12);max-height:260px;overflow-y:auto;z-index:40;padding:4px;">`)
	for _, it := range r.boundArray(n, "items") {
		m, ok := it.(map[string]any)
		if !ok { // a bare string is a label-only entry
			m = map[string]any{"label": fmt.Sprint(it)}
		}
		label := r.interp(str(m, "label"))
		attr := ""
		if onSelect != nil {
			attr = fmt.Sprintf(` onclick="qormSearchPick(this,%d)"`, r.register(&model.Invoke{Name: onSelect.Name, Args: mergeArgs(onSelect.Args, "label", label)}))
		}
		fmt.Fprintf(&r.sb, `<div class="qorm-search-item" data-label=%q style="display:flex;align-items:center;gap:8px;padding:8px 10px;border-radius:6px;cursor:pointer;font-size:14px;"%s>`,
			html.EscapeString(label), attr)
		if svg := iconSVG(str(m, "icon"), 15); svg != "" {
			fmt.Fprintf(&r.sb, `<span style="width:18px;display:inline-flex;justify-content:center;color:var(--label2);flex:none;">%s</span>`, svg)
		}
		fmt.Fprintf(&r.sb, `<span>%s</span>`, html.EscapeString(label))
		if d := r.interp(str(m, "detail")); d != "" {
			fmt.Fprintf(&r.sb, `<span style="margin-left:auto;color:var(--label2);font-size:12px;">%s</span>`, html.EscapeString(d))
		}
		r.sb.WriteString(`</div>`)
	}
	r.sb.WriteString(`</div></div>`)
}

// textFormField is Flutter's TextFormField with InputDecoration: label, prefix/
// suffix, helper/counter, and reactive validation — the `error` binding (an
// expression over state, e.g. matches(...) ? "" : "Invalid") shows inline and
// reddens the border the moment state changes.
func (r *renderer) textFormField(n *model.Node) {
	path := boundPath(n.Value)
	errText := r.interp(propStr(n, "error"))
	invalid := errText != ""
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, attrID(n.ID), r.boxCSS(n)+"display:flex;flex-direction:column;gap:4px;")
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
	al := a11y(n)
	// inputAttrs rides on the inner input: its maxlength truncates natively at
	// the same limit the footer counter below displays.
	fmt.Fprintf(&r.sb, `<input type=%q value=%q placeholder=%q style="flex:1;border:none;outline:none;font-size:14px;background:transparent;"%s%s%s%s%s>`,
		html.EscapeString(itype), html.EscapeString(r.interp(n.Value)), html.EscapeString(n.Placeholder), dataStateAttr(path), al,
		r.inputAttrs(n, false), r.errorEcho(n, errText, al), r.changeWiring(n, path != ""))
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
