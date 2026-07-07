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
	if v := layoutStr(n, "align"); v != "" {
		fmt.Fprintf(&b, "align-items:%s;", flexAlign(v))
	}
	if v := layoutStr(n, "justify"); v != "" {
		fmt.Fprintf(&b, "justify-content:%s;", flexAlign(v))
	}
	b.WriteString(r.boxCSS(n))
	return b.String()
}

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
	if v, ok := numOK(s, "aspectRatio"); ok {
		css(&b, "aspect-ratio", v, ";")
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
	return b.String()
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
	return b.String()
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
		case float64:
			fmt.Fprintf(b, "%s:%gpx;", dim, t)
			return
		}
	}
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
