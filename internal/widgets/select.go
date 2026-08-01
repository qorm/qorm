package widgets

// The select (HTML: render_input.go:294 — the native <select>): a current-
// value box with a dropdown indicator over the options prop.
//
// Documented downgrade (no overlay infrastructure in the canvas engine — no
// z-layer above the scene graph, see html-parity §3.6): instead of a popup
// menu, clicking CYCLES the selection through the options (wrapping), writing
// each new value back and dispatching onChange — the same state channel the
// HTML select uses, with a degraded picker UI.

import (
	"fmt"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("select", &Select{local: map[*model.Node]string{}})
}

// Select is the single-value picker box: the selected option's label (or the
// raw value, or — while empty — the first option, the browser's default
// display), a stepped-triangle indicator at the right edge, and click-to-
// cycle selection.
type Select struct {
	mu sync.Mutex
	// local holds UNBOUND selects' values (see Checkbox.local).
	local map[*model.Node]string
}

const (
	selectPadX  = 12
	selectPadY  = 8
	selectMinW  = 120
	selectGlyph = 18 // indicator column width
)

// Measure sizes the box to the widest option label plus padding and the
// indicator column (min 120px wide, one text line plus padding tall).
func (s *Select) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	lw := 0
	for _, o := range formOptions(n.Props["options"]) {
		if w := int(canvas.MeasureText(o.label, float64(fs))); w > lw {
			lw = w
		}
	}
	if w := int(canvas.MeasureText(s.display(n, rt), float64(fs))); w > lw {
		lw = w
	}
	w = lw + (2*selectPadX+selectGlyph)*scale
	if min := selectMinW * scale; w < min {
		w = min
	}
	h = lineHeight(fs) + 2*selectPadY*scale
	return
}

// Record draws the input-style chrome, the current label and the indicator.
func (s *Select) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()

	chrome := draw.NewRect()
	chrome.Width = float64(ln.Width)
	chrome.Height = float64(ln.Height)
	chrome.BorderRadius = 10 * float64(scale)
	chrome.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	chrome.Stroke = themeColor(rt, "inputBorder", color.RGBA{198, 198, 200, 255})
	chrome.StrokeWidth = float64(scale)
	g.AddChild(chrome)

	fs := formFontSizeLN(ln, scale)
	g.AddChild(formText(s.display(ln.Node, rt), float64(selectPadX*scale),
		(float64(ln.Height)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))

	// The dropdown indicator: a small downward stepped triangle (three
	// centered bars — the draw layer has no rotation or polygon primitive,
	// so this is the honest chevron stand-in). Decorative: NoHit.
	ink := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	cx := float64(ln.Width - (selectPadX+selectGlyph/2)*scale)
	cy := float64(ln.Height) / 2
	rowH := float64(2 * scale)
	for i, rw := range []int{10, 6, 2} {
		w := float64(rw * scale)
		bar := draw.NewRect()
		bar.NoHit = true
		bar.X = cx - w/2
		bar.Y = cy - float64(3*scale) + float64(i)*(rowH+float64(scale))
		bar.Width = w
		bar.Height = rowH
		bar.Fill = ink
		g.AddChild(bar)
	}
	return g
}

// HandlePointer cycles to the option after the current one (wrapping at the
// end) on every press — the no-overlay selection mechanism — writing the new
// value back and dispatching onChange with {value}.
func (s *Select) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction) bool {
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	opts := formOptions(n.Props["options"])
	if len(opts) == 0 {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	cur := s.value(n, rt)
	idx := -1
	for i, o := range opts {
		if o.value == cur {
			idx = i
			break
		}
	}
	next := opts[(idx+1)%len(opts)].value
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, next)
	} else {
		s.mu.Lock()
		s.local[n] = next
		s.mu.Unlock()
	}
	commitFormChange(n, rt, next)
	return true
}

// value resolves the current value: the binding, else the uncontrolled store,
// else the literal value (may be empty — the display falls back then).
func (s *Select) value(n *model.Node, rt *runtime.Runtime) string {
	if formBoundPath(n.Value) != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtx(rt)))
	}
	s.mu.Lock()
	lv, ok := s.local[n]
	s.mu.Unlock()
	if ok {
		return lv
	}
	return formEvalStr(n.Value, rt)
}

// display resolves what the box shows: the selected option's label, the raw
// value when it matches no option, or the first option while the value is
// empty (the browser's default selection display).
func (s *Select) display(n *model.Node, rt *runtime.Runtime) string {
	cur := s.value(n, rt)
	opts := formOptions(n.Props["options"])
	for _, o := range opts {
		if o.value == cur {
			return o.label
		}
	}
	if cur != "" {
		return cur
	}
	if len(opts) > 0 {
		return opts[0].label
	}
	return ""
}
