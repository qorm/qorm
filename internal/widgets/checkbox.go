package widgets

// Form widgets for the canvas engine (W9): checkbox, switch, radio, slider,
// select, textarea. They are InteractiveWidgets (canvas/widget.go): the engine
// routes their pointer stream to the widget BEFORE its generic press/hover
// handling, and the widget owns state write-back and onChange dispatch.
//
// Semantics mirrored from the HTML renderer (internal/render/render_input.go):
//   - a value spelled {{state.x}} is a two-way binding: interaction writes the
//     new value back through Runtime.SetStatePath (which keeps the read-only
//     computed namespace refusal, runtime.go:638) AND a declared onChange
//     dispatches in parallel with the new {value} — the same write-back +
//     dispatch pair the HTML client performs (render.go changeAttr, app.js
//     qorm()), and the same pair canvas/input.go commitEdit performs;
//   - with no binding the control is UNCONTROLLED: the widget keeps a local
//     per-node value (the browser holds it in the DOM; the canvas scene graph
//     is rebuilt every frame, so it lives on the widget keyed by the stable
//     model pointer) — onChange still dispatches;
//   - disabled (the style key, nodeDisabled truthiness) suppresses all
//     interaction. v1 trade-off: a disabled form widget swallows the event
//     instead of letting it fall through to an ancestor, because the engine
//     routes to the widget before its generic (fall-through) dispatch.
//
// This file holds the checkbox plus the helpers the whole form family shares.

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("checkbox", &Checkbox{local: map[*model.Node]bool{}})
}

// Checkbox is the square boolean control (HTML: render_input.go:309): an
// 18px rounded box with a check mark, filled with the theme accent while
// checked, plus an optional label to the right (labelOf: label, then text).
type Checkbox struct {
	mu sync.Mutex
	// local holds the checked state of UNBOUND checkboxes (no {{state.x}}
	// value binding) — the uncontrolled case the browser keeps in the DOM.
	local map[*model.Node]bool
}

// Measure reports the box plus label: 18px box, 8px gap, one text line tall.
func (c *Checkbox) Measure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	box := 18 * scale
	fs := formFontSize(n, scale)
	w, h = box, max(box, lineHeight(fs))
	if label := formLabel(n, rt); label != "" {
		w += 8*scale + int(canvas.MeasureText(label, float64(fs)))
	}
	return
}

// Record draws the box (accent-filled with a white check while checked) and
// the label in the text color.
func (c *Checkbox) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	boxS := float64(18 * scale)
	boxY := (float64(ln.Height) - boxS) / 2

	g := draw.NewGroup()
	box := draw.NewRect()
	box.Y = boxY
	box.Width, box.Height = boxS, boxS
	box.BorderRadius = 4 * float64(scale)
	if c.checked(ln.Node, rt) {
		box.Fill = formAccent(rt)
		checkMark(g, 0, boxY, boxS, scale, color.RGBA{255, 255, 255, 255})
	} else {
		box.Fill = themeColor(rt, "cardBg", color.RGBA{255, 255, 255, 255})
		box.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
		box.StrokeWidth = float64(scale)
	}
	g.AddChild(box)

	if label := formLabel(ln.Node, rt); label != "" {
		fs := formFontSizeLN(ln, scale)
		g.AddChild(formText(label, boxS+float64(8*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))
	}
	return g
}

// HandlePointer flips the checked state on press: the new value writes back
// to the bound state path (or the local uncontrolled store) and onChange
// dispatches with {value: bool} — the HTML change wiring pair.
func (c *Checkbox) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction) bool {
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	// Pointer-driven focus (the generic press path's semantics), which also
	// blurs any input edit session via the engine's syncEditSession.
	inter.Focused = n
	inter.FocusVisible = false
	next := !c.checked(n, rt)
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, next)
	} else {
		c.mu.Lock()
		c.local[n] = next
		c.mu.Unlock()
	}
	commitFormChange(n, rt, next)
	return true
}

// checked resolves the current state the HTML checkedState way
// (render_input.go:333): the value binding first, then — for the
// uncontrolled case — the user's local toggle wins over the `checked` prop,
// which is only the INITIAL state (the HTML checked attribute vs the live
// property).
func (c *Checkbox) checked(n *model.Node, rt *runtime.Runtime) bool {
	if formBoundPath(n.Value) != "" {
		return formTruthy(runtime.EvalBinding(n.Value, formCtx(rt)))
	}
	c.mu.Lock()
	lv, ok := c.local[n]
	c.mu.Unlock()
	if ok {
		return lv
	}
	if v, ok := n.Prop("checked"); ok {
		return formTruthy(runtime.EvalBinding(fmt.Sprint(v), formCtx(rt)))
	}
	return false
}

// checkMark paints a check inside the S×S box at (bx,by) as a staircase of
// axis-aligned rects — the draw layer has no path or rotation primitive, so
// the diagonal strokes are approximated pixel by pixel (at 18px the eye reads
// it cleanly). Decorative: NoHit.
func checkMark(g *draw.Group, bx, by, s float64, scale int, ink color.RGBA) {
	thick := float64(max(2, 2*scale))
	step := float64(max(1, scale))
	pts := [][2]float64{{0.24, 0.55}, {0.43, 0.72}, {0.78, 0.30}}
	for seg := 0; seg < len(pts)-1; seg++ {
		x0, y0 := bx+pts[seg][0]*s, by+pts[seg][1]*s
		x1, y1 := bx+pts[seg+1][0]*s, by+pts[seg+1][1]*s
		steps := int(math.Abs(x1-x0) / step)
		if steps < 1 {
			steps = 1
		}
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			r := draw.NewRect()
			r.NoHit = true
			r.X = x0 + (x1-x0)*t
			r.Y = y0 + (y1-y0)*t - thick/2
			r.Width = step
			r.Height = thick
			r.Fill = ink
			g.AddChild(r)
		}
	}
}

// ---------------------------------------------------------------------------
// Shared form helpers (the whole family composes these).
// ---------------------------------------------------------------------------

// formStateBindRe matches a whole-string state binding — the same shape the
// HTML renderer's boundPath and canvas's stateValueBindRe recognize
// (render_widgets.go:223, canvas/input.go).
var formStateBindRe = regexp.MustCompile(`^\s*\{\{\s*state\.([a-zA-Z0-9_.]+)\s*\}\}$`)

// formBoundPath returns the state path a value binding writes back to, or ""
// when the value is not a whole-string {{state.x}} binding.
func formBoundPath(value string) string {
	if m := formStateBindRe.FindStringSubmatch(value); m != nil {
		return m[1]
	}
	return ""
}

// formCtx is the binding evaluation context for form props (state root), the
// same context badge uses.
func formCtx(rt *runtime.Runtime) map[string]any {
	if rt == nil {
		return map[string]any{}
	}
	return map[string]any{"state": rt.State}
}

// formTruthy mirrors the HTML renderer's asBool (render_style.go:1042).
func formTruthy(v any) bool {
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

// formFloat mirrors the HTML renderer's asFloat (render_style.go:1024).
func formFloat(v any) float64 {
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

// formEvalStr evaluates a prop/binding to its display string.
func formEvalStr(val any, rt *runtime.Runtime) string {
	s, ok := val.(string)
	if !ok {
		return ""
	}
	if rt == nil {
		return s
	}
	res := runtime.EvalBinding(s, formCtx(rt))
	if res == nil {
		return ""
	}
	if str, ok := res.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", res)
}

// formDisabled mirrors canvas's nodeDisabled (interaction.go — unexported
// there): the `disabled` style key, static or bound, suppresses interaction.
func formDisabled(n *model.Node, rt *runtime.Runtime) bool {
	if n == nil {
		return false
	}
	v, ok := n.Style["disabled"]
	if !ok {
		return false
	}
	if s, isStr := v.(string); isStr && rt != nil {
		v = runtime.EvalBinding(s, formCtx(rt))
	}
	return formTruthy(v)
}

// commitFormChange dispatches the node's onChange with {value} — the invoke
// style canvas/input.go commitEdit uses: the value is seeded and the action's
// own args (evaluated against live state, a binding-capable action name
// resolved first) win on collision.
func commitFormChange(n *model.Node, rt *runtime.Runtime, value any) {
	evt := n.OnChange
	if evt == nil || rt == nil {
		return
	}
	ctx := formCtx(rt)
	name := evt.Name
	if strings.Contains(name, "{{") {
		name = runtime.Stringify(runtime.EvalBinding(name, ctx))
	}
	args := map[string]any{"value": value}
	for k, v := range evt.Args {
		args[k] = runtime.EvalBinding(v, ctx)
	}
	rt.Dispatch(name, args)
}

// formAccent resolves the accent color form controls paint their on-state
// with: theme "accent", then theme "primary" (the default theme's spelling),
// then the iOS blue literal.
func formAccent(rt *runtime.Runtime) color.RGBA {
	return themeColor(rt, "accent", themeColor(rt, "primary", color.RGBA{0, 122, 255, 255}))
}

// formInk picks the label text color: an explicit style color wins, else the
// theme's text color (parseStyle defaults Color to white, which would be
// invisible on the scene background).
func formInk(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) color.RGBA {
	if n != nil {
		if _, ok := n.Style["color"]; ok && ln != nil && ln.Style.Color.A > 0 {
			return ln.Style.Color
		}
	}
	return themeColor(rt, "text", color.RGBA{29, 29, 31, 255})
}

// formLabel resolves the control's label the HTML labelOf way
// (render_style.go:587): the label field, then text, bindings evaluated.
func formLabel(n *model.Node, rt *runtime.Runtime) string {
	if n.Label != "" {
		return formEvalStr(n.Label, rt)
	}
	return formEvalStr(n.Text, rt)
}

// formFontSize resolves the font size for Measure (no LayoutNode yet): the
// style fontSize, else the engine default 14, in physical px.
func formFontSize(n *model.Node, scale int) int {
	if scale < 1 {
		scale = 1
	}
	if n != nil {
		if v, ok := n.Style["fontSize"]; ok {
			if fs := int(formFloat(v)); fs > 0 {
				return fs * scale
			}
		}
	}
	return 14 * scale
}

// formFontSizeLN is formFontSize for Record: the resolved (already scaled)
// style font size, else the default.
func formFontSizeLN(ln *canvas.LayoutNode, scale int) int {
	if ln != nil && ln.Style.FontSize > 0 {
		return ln.Style.FontSize
	}
	return formFontSize(nil, scale)
}

// lineHeight is the engine's text line height (fs*1.2, canvas/measure.go).
func lineHeight(fs int) int { return int(float64(fs) * 1.2) }

// formText builds one left-aligned text run at (x, y). Decorative: NoHit —
// the widget's own box owns the pointer stream.
func formText(content string, x, y float64, fs int, ink color.RGBA) *draw.Text {
	t := draw.NewText()
	t.NoHit = true
	t.Content = content
	t.FontSize = float64(fs)
	t.Fill = ink
	t.X = x
	t.Y = y
	return t
}

// formOption is one parsed options entry (value/label pair).
type formOption struct {
	value string
	label string
}

// formOptions parses an `options` prop the HTML optionList way
// (render_style.go:547): strings become value==label; maps take value (or
// key) plus label (or title, or the value).
func formOptions(v any) []formOption {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]formOption, 0, len(arr))
	for _, e := range arr {
		switch t := e.(type) {
		case string:
			out = append(out, formOption{t, t})
		case map[string]any:
			val := fmt.Sprint(t["value"])
			if val == "" || val == "<nil>" {
				val = fmt.Sprint(t["key"])
			}
			lbl, _ := t["label"].(string)
			if lbl == "" {
				lbl, _ = t["title"].(string)
			}
			if lbl == "" {
				lbl = val
			}
			out = append(out, formOption{val, lbl})
		}
	}
	return out
}
