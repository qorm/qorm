package widgets

// The radio group (HTML: render_input.go:343): ONE radio node renders its
// whole option list as a vertical stack of circle+label rows sharing one
// group name (the node id in HTML); the bound value holds the selected
// option's value, so every radio node bound to the same state path is
// mutually exclusive by construction — selection flows only through state.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("radio", &Radio{
		local: map[*model.Node]string{},
		geoms: map[*model.Node]radioGeom{},
	})
}

// Radio is the single-choice option stack: a 16px circle per option (accent
// dot while selected), 6px between rows, the option label to the right.
// Clicking a row writes that option's value to the bound state path (or the
// uncontrolled store) and dispatches onChange with {value}.
type Radio struct {
	mu sync.Mutex
	// local holds the selected value of UNBOUND radio nodes.
	local map[*model.Node]string
	// geoms is the last laid-out geometry per node (absolute physical px),
	// stashed every Record so HandlePointer can map a press Y to a row.
	geoms map[*model.Node]radioGeom
}

// radioGeom is one radio node's on-screen box plus its uniform row stride.
type radioGeom struct {
	box    image.Rectangle
	stride float64
}

const (
	radioCircleD = 16
	radioGap     = 6
	radioLabelX  = 24
)

// radioRowH is one option row's vertical stride (circle + gap) at scale.
func radioRowH(scale int) int { return (radioCircleD + radioGap) * scale }

// Measure stacks the option rows: max label width plus the circle column,
// rows of 16px with 6px gaps (HTML: flex column, gap 6px).
func (r *Radio) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	opts := formOptions(n.Props["options"])
	if len(opts) == 0 {
		return radioCircleD * scale, radioCircleD * scale
	}
	fs := formFontSize(n, scale)
	w = radioLabelX * scale
	for _, o := range opts {
		if lw := radioLabelX*scale + int(canvas.MeasureText(o.label, float64(fs))); lw > w {
			w = lw
		}
	}
	h = len(opts)*radioCircleD*scale + (len(opts)-1)*radioGap*scale
	return
}

// Record draws one circle + label row per option, the selected row's dot in
// the accent color, and stashes the geometry for row hit-testing.
func (r *Radio) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	r.mu.Lock()
	r.geoms[ln.Node] = radioGeom{
		box:    image.Rect(ln.X, ln.Y, ln.X+ln.Width, ln.Y+ln.Height),
		stride: float64(radioRowH(scale)),
	}
	r.mu.Unlock()

	cur := r.selected(ln.Node, rt)
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	d := float64(radioCircleD * scale)
	stride := float64(radioRowH(scale))

	g := draw.NewGroup()
	for i, o := range formOptions(ln.Node.Props["options"]) {
		y := float64(i) * stride
		ring := draw.NewCircle()
		ring.Y = y
		ring.Radius = d / 2
		ring.Fill = themeColor(rt, "cardBg", color.RGBA{255, 255, 255, 255})
		ring.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
		ring.StrokeWidth = float64(scale)
		g.AddChild(ring)
		if o.value == cur {
			dot := draw.NewCircle()
			dot.NoHit = true
			inner := 5 * float64(scale)
			dot.X = d/2 - inner
			dot.Y = y + d/2 - inner
			dot.Radius = inner
			dot.Fill = formAccent(rt)
			g.AddChild(dot)
		}
		g.AddChild(formText(o.label, float64(radioLabelX*scale), y+(d-float64(lineHeight(fs)))/2, fs, ink))
	}
	return g
}

// HandlePointer maps the press to an option row and selects it.
func (r *Radio) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction) bool {
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
	r.mu.Lock()
	geo, ok := r.geoms[n]
	r.mu.Unlock()
	if !ok || geo.stride <= 0 {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	idx := int((p.Y - float64(geo.box.Min.Y)) / geo.stride)
	if idx < 0 || idx >= len(opts) {
		return true // a press in the trailing gap still focuses, selects nothing
	}
	val := opts[idx].value
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, val)
	} else {
		r.mu.Lock()
		r.local[n] = val
		r.mu.Unlock()
	}
	commitFormChange(n, rt, val)
	return true
}

// selected resolves the current value: the binding, then — uncontrolled —
// the user's local pick wins over the literal value prop (the initial
// selection), empty meaning nothing selected (like HTML).
func (r *Radio) selected(n *model.Node, rt *runtime.Runtime) string {
	if formBoundPath(n.Value) != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtx(rt)))
	}
	r.mu.Lock()
	lv, ok := r.local[n]
	r.mu.Unlock()
	if ok {
		return lv
	}
	if n.Value != "" {
		return formEvalStr(n.Value, rt)
	}
	return ""
}
