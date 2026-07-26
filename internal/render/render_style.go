package render

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// dragAttr marks a node as a window-drag region on desktop (prop "drag": true).
func dragAttr(n *model.Node) string {
	if v, ok := n.Prop("drag"); ok {
		if b, _ := v.(bool); b {
			return ` data-qorm-drag`
		}
	}
	return ""
}

func boundPath(value string) string {
	if m := stateBindRe.FindStringSubmatch(value); m != nil {
		return m[1]
	}
	return ""
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

// iconLabel builds a hardware-widget button's content: an app-authored label
// renders as plain escaped text; when empty, the default is the built-in SVG
// icon (the framework's alternative to emoji) prefixed to defLabel, in a
// centered inline-flex row. The label prop read stays at the call site so the
// API-ref prop extractor (internal/integration) attributes it to the widget.
func iconLabel(label, icon, defLabel string) string {
	if label != "" {
		return html.EscapeString(label)
	}
	return `<span style="display:inline-flex;align-items:center;justify-content:center;gap:8px;">` + iconSVG(icon, 18) + html.EscapeString(defLabel) + `</span>`
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

func borderIf(b bool) string {
	if b {
		return "1px solid var(--sep)"
	}
	return "none"
}

func segStyle(active bool) string {
	if active {
		return "background:var(--surface);color:var(--label);font-weight:600;box-shadow:0 1px 2px rgba(0,0,0,.1);"
	}
	return "color:var(--label2);"
}

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
	// Semantic alias containers (`center`, `start`, `between`, …) carry their
	// name's alignment as the DEFAULT, so the type is self-describing instead of
	// being a plain column that silently needs a `layout.align`. An explicit
	// layout.align / layout.justify still wins (it replaces the default rather
	// than being appended after it, so only one declaration is emitted).
	align, justify := aliasAlign(n.Type)
	if v := layoutStr(n, "align"); v != "" {
		align = flexAlign(v)
	}
	if v := layoutStr(n, "justify"); v != "" {
		justify = flexAlign(v)
	}
	if align != "" {
		fmt.Fprintf(&b, "align-items:%s;", align)
	}
	if justify != "" {
		fmt.Fprintf(&b, "justify-content:%s;", justify)
	}
	// boxCSS already entity-encodes its own (author/bound) values; escape this
	// prefix on its own (constants and whitelisted values today, but this keeps
	// the whole string attribute-safe) and append boxCSS raw so its entities
	// are never double-encoded.
	return styleAttr(b.String()) + r.boxCSS(n)
}

// styleAttr entity-encodes an assembled style-attribute value so an author- or
// bound style value (background, gradient, shadow, cursor, transition,
// fontFamily, ...) cannot break out of the quoted style="..." attribute. Go's
// %q quotes a double quote as \" — but the HTML parser treats the backslash as
// a literal character and the quote still TERMINATES the attribute, so a raw
// value could inject arbitrary attributes (the round-6 id= breakout class; CSS
// url(javascript:) is inert, so the attribute breakout is the live vector).
// html.EscapeString only touches & < > " ', which never occur in legitimate
// CSS property values, and the browser HTML-unescapes the attribute value
// before CSS parsing — so the encoding is transparent: safe values render
// byte-identical and any legitimate special character round-trips.
func styleAttr(css string) string { return html.EscapeString(css) }

// boxCSS renders style + layout properties shared by all node kinds.
// resolveStyle returns a copy of a style/layout map with any `{{ … }}` string
// values evaluated against the current context, so numeric styles (width,
// height, opacity, …) can be bound — the basis for animation and agent-driven
// restyling. The common (binding-free) case returns the input unchanged.
func (r *renderer) resolveStyle(style map[string]any) map[string]any {
	if style == nil {
		return nil
	}
	if !styleHasBinding(style) {
		return style
	}
	out := make(map[string]any, len(style))
	for k, v := range style {
		out[k] = r.resolveStyleVal(v)
	}
	return out
}

// resolveStyleVal evaluates {{ … }} bindings in a style value, recursing into
// nested maps (e.g. margin:{left:"{{…}}"}) and arrays so nested edges bind too.
func (r *renderer) resolveStyleVal(v any) any {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, "{{") {
			return runtime.EvalBinding(t, r.ctx())
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = r.resolveStyleVal(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = r.resolveStyleVal(vv)
		}
		return out
	default:
		return v
	}
}

// styleHasBinding reports whether v contains a {{ … }} binding anywhere, incl.
// nested maps/arrays, so the binding-free common case skips the copy.
func styleHasBinding(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, "{{")
	case map[string]any:
		for _, vv := range t {
			if styleHasBinding(vv) {
				return true
			}
		}
	case []any:
		for _, vv := range t {
			if styleHasBinding(vv) {
				return true
			}
		}
	}
	return false
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
	if v, ok := numOK(s, "flexShrink"); ok {
		css(&b, "flex-shrink", v, ";")
	}
	if v := colorStr(s, "alignSelf"); v != "" {
		fmt.Fprintf(&b, "align-self:%s;", alignSelfCSS(v))
	}
	if v, ok := numOK(s, "aspectRatio"); ok {
		css(&b, "aspect-ratio", v, ";")
	}
	if v, ok := numOK(s, "zIndex"); ok {
		css(&b, "z-index", v, ";")
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
	pseudoStateCSS(&b, s)
	backdropCSS(&b, s)
	// Entity-encode the assembled value: the colour/string style keys ride
	// straight from author/bound input (see styleAttr), so an unencoded double
	// quote would break out of the style="..." attribute at the emission site.
	return styleAttr(b.String())
}

// Pseudo-state custom properties. Names are deliberately short (they repeat on
// every styled node) and mutually non-prefixing, because the shell selects on a
// substring of the style attribute: `--qorm-dis` must not also match the
// disabled-opacity variable, hence `--qorm-dop`.
const (
	varHoverBG      = "--qorm-hov-bg"
	varHoverFG      = "--qorm-hov-fg"
	varHoverOpacity = "--qorm-hov-op"
	varPressScale   = "--qorm-prs-sc"
	varPressOpacity = "--qorm-prs-op"
	varFocusBorder  = "--qorm-foc-bc"
	varDisabled     = "--qorm-dis"
	varDisabledOp   = "--qorm-dop"
	varBackdropBlur = "--qorm-bdb"
	varBackdropTint = "--qorm-bdt"
)

// maxBackdropBlur caps the frosted-glass radius. backdrop-filter re-rasterises
// everything painted behind the element on every frame, so an unbounded author
// value (or a bound one an agent drives) is a GPU trap on a phone; 120px is
// already far past the point where more blur is visually distinguishable.
const maxBackdropBlur = 120

// backdropCSS emits the frosted-glass style keys — `backdropBlur` (radius in
// px) and the optional `backdropTint` (the translucent fill the blur shows
// through) — as CSS custom properties on the node itself. Like the
// pseudo-state keys above, the VISUAL lives in fixed rules in the HTML shell
// (internal/server/server.go) that match on the variable being present, which
// buys three things an inline declaration cannot:
//
//   - the `-webkit-` prefixed and unprefixed properties are written once, in
//     the stylesheet, instead of on every frosted node;
//   - the blur sits inside an @supports guard, with a SOLID fallback outside
//     it, so a browser without backdrop-filter renders an opaque panel rather
//     than an unreadable see-through one (the degradation the key promises);
//   - being pure CSS it survives every DOM morph, exactly like the hover /
//     pressed visuals.
//
// The rules are deliberately NOT !important, so an author's own `background`
// (an inline declaration) still wins over the tint.
func backdropCSS(b *strings.Builder, s map[string]any) {
	v, ok := numOK(s, "backdropBlur")
	if !ok || v <= 0 {
		return
	}
	if v > maxBackdropBlur {
		v = maxBackdropBlur
	}
	css(b, varBackdropBlur, v, "px;")
	if t := colorStr(s, "backdropTint"); t != "" {
		fmt.Fprintf(b, "%s:%s;", varBackdropTint, t)
	}
}

// frostCSS returns the frosted-glass declarations (both the `-webkit-` prefixed
// and the standard property) for an explicit radius, or "" when there is no
// blur to apply. Used by the widgets that frost their OWN panel inline —
// appbar, largetitle — where the radius is a built-in part of the iOS look
// rather than an author style key; `backdropBlur` on such a node overrides the
// built-in default (0 turns the frost off entirely).
func frostCSS(px float64) string {
	if px <= 0 {
		return ""
	}
	return fmt.Sprintf("-webkit-backdrop-filter:blur(%gpx);backdrop-filter:blur(%gpx);", px, px)
}

// backdropBlurPx reads the `backdropBlur` style key for a widget that frosts
// its own panel, falling back to that widget's built-in radius. The style map
// is resolved first, so the radius can be a `{{ … }}` binding like any other
// numeric style.
func (r *renderer) backdropBlurPx(n *model.Node, def float64) float64 {
	v, ok := numOK(r.resolveStyle(n.Style), "backdropBlur")
	if !ok {
		return def
	}
	if v < 0 {
		return 0
	}
	if v > maxBackdropBlur {
		return maxBackdropBlur
	}
	return v
}

// pseudoStateCSS emits the hover / pressed / focus / disabled style keys as CSS
// custom properties on the node itself. The visual is applied by the fixed
// rules in the HTML shell (internal/server/server.go), which match on the
// variable being present — `[style*="--qorm-hov-bg"]:hover { … }` — and mark
// their declarations !important, since an inline style otherwise outranks any
// stylesheet rule. That split keeps the state visuals declarative and
// JS-free (so a DOM morph can never drop them) while the author value stays
// inside the style attribute, where styleAttr already entity-encodes it — a
// pseudo-state value is exactly as contained as `background` is.
//
// Numeric keys go through numOK, so they are float64 by construction and can
// carry nothing but a number.
func pseudoStateCSS(b *strings.Builder, s map[string]any) {
	if s == nil {
		return
	}
	if v := colorStr(s, "hoverBackground"); v != "" {
		fmt.Fprintf(b, "%s:%s;", varHoverBG, v)
	}
	if v := colorStr(s, "hoverColor"); v != "" {
		fmt.Fprintf(b, "%s:%s;", varHoverFG, v)
	}
	if v, ok := numOK(s, "hoverOpacity"); ok {
		css(b, varHoverOpacity, v, ";")
	}
	if v, ok := numOK(s, "pressedScale"); ok {
		css(b, varPressScale, v, ";")
	}
	if v, ok := numOK(s, "pressedOpacity"); ok {
		css(b, varPressOpacity, v, ";")
	}
	if v := colorStr(s, "focusBorderColor"); v != "" {
		fmt.Fprintf(b, "%s:%s;", varFocusBorder, v)
	}
	if v, ok := numOK(s, "disabledOpacity"); ok {
		css(b, varDisabledOp, v, ";")
	}
	if styleDisabled(s) {
		fmt.Fprintf(b, "%s:1;", varDisabled)
	}
}

// styleDisabled reads the `disabled` style key. It is a marker, not a value:
// the shell rule keyed on it dims the node, blocks pointer events and shows the
// not-allowed cursor, and a11y pairs it with aria-disabled.
func styleDisabled(s map[string]any) bool {
	if s == nil {
		return false
	}
	v, ok := s["disabled"]
	return ok && asBool(v)
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
	// Entity-encode like boxCSS (see styleAttr): the string keys interpolate
	// author/bound values raw into the quoted style attribute.
	return styleAttr(b.String())
}

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
	// The `disabled` style key is a state, not just a look: pair its visual (see
	// pseudoStateCSS) with the ARIA state so assistive tech announces it. There
	// is no native `disabled` attribute to set here — a11y is shared by every
	// element kind (div/a/button/…), and the attribute is only valid on form
	// controls, so the generic aria form plus the shell's pointer-events:none is
	// the honest equivalent. Read raw (unresolved) so this stays a pure
	// node->attributes function; a `{{ … }}` binding still drives the visual
	// through boxCSS, which resolves.
	if styleDisabled(n.Style) {
		b.WriteString(` aria-disabled="true"`)
	}
	return b.String()
}

func dataStateAttr(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(` data-state=%q`, path)
}

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
			if u, ok := sizeUnit(t); ok {
				fmt.Fprintf(b, "%s:%s;", dim, u)
				return
			}
		case float64:
			fmt.Fprintf(b, "%s:%gpx;", dim, t)
			return
		}
	}
}

// sizeUnit parses a string size with an explicit CSS unit — "50%", "30vw",
// "40vh", "120px" — and returns it re-rendered from the parsed number plus the
// constant unit, so an arbitrary author string never reaches the style
// attribute verbatim (the same normalize-don't-trust shape as the numeric px
// path). Anything that is not <number><unit> reports ok=false and stays
// ignored, preserving the existing behavior for unknown strings ("wrap", ...);
// "fill" and plain numbers keep their fast paths in writeSize.
func sizeUnit(s string) (string, bool) {
	for _, u := range [...]string{"%", "vw", "vh", "px"} {
		if strings.HasSuffix(s, u) {
			f, err := strconv.ParseFloat(strings.TrimSuffix(s, u), 64)
			if err != nil {
				return "", false
			}
			return num(f) + u, true
		}
	}
	return "", false
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
	case "baseline":
		return "baseline"
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

// aliasAlign gives the semantic alias container types their namesake alignment
// as a default (containerCSS applies it unless the author writes an explicit
// layout.align / layout.justify). These types render as a flex COLUMN, so:
//
//   - center / start / end pack content on BOTH axes, matching what the name
//     promises — `{"type":"center"}` centers, full stop.
//   - between / around / evenly are distribution words: they only set
//     justify-content and leave the cross axis at its stretch default.
//   - stretch pins the cross axis (already the CSS default, emitted for
//     explicitness so the type reads the same as it renders).
//
// Every returned value is a flexAlign constant, so nothing author-controlled
// reaches the style attribute here.
func aliasAlign(nodeType string) (align, justify string) {
	switch nodeType {
	case "center", "start", "end":
		return flexAlign(nodeType), flexAlign(nodeType)
	case "between", "around", "evenly":
		return "", flexAlign(nodeType)
	case "stretch":
		return flexAlign(nodeType), ""
	}
	return "", ""
}

// alignSelfCSS maps the alignSelf style key onto CSS align-self, reusing the
// layout keyword vocabulary (start/center/end/stretch/baseline, plus the
// left/top/right/bottom synonyms flexAlign already accepts) and adding "auto"
// (the CSS default: defer to the parent's align-items). Only whitelisted
// constants come back, so the value is safe in the style attribute by
// construction — and boxCSS styleAttr-escapes its whole output anyway.
func alignSelfCSS(v string) string {
	if v == "auto" {
		return "auto"
	}
	return flexAlign(v)
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
	// colour is an author prop interpolated into a quoted SVG fill attribute:
	// entity-encode the value (not the surrounding constant markup) so a
	// double quote cannot break out of the attribute and inject attributes.
	color = html.EscapeString(color)
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
	// colour is an author prop interpolated into quoted SVG stroke/fill
	// attributes: entity-encode the value (not the surrounding constant
	// markup) so a double quote cannot break out and inject attributes.
	color = html.EscapeString(color)
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

func truthyStrCT(s string) bool { return s != "" && s != "false" && s != "0" }

func truthyStrChip(s string) bool { return s != "" && s != "false" && s != "0" }

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

// dialogAction is one button in an iOS dialog/action sheet.
type dialogAction struct {
	label, style string
	inv          *model.Invoke
}

func (r *renderer) actionColor(style string) string {
	switch style {
	case "destructive":
		return "var(--danger)"
	default:
		return "var(--accent)"
	}
}
