package widgets

import (
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("spacer", Spacer{})
}

// Spacer is the layout gap (HTML render_widgets.go:234: `flex:1 1 auto`, or
// a fixed size×size box when style.size is set). The canvas v1 layout has NO
// flex channel — no flexGrow, no remaining-space distribution, no cross-axis
// stretch (html-parity §3.7: "缺 …flexGrow…"; a widget never sees its parent
// or siblings) — so the expanding form cannot eat leftover space here. It
// degrades honestly to a 0×0 no-op; the fixed-size form is exact.
//
// Documented divergence from HTML: multiple spacers do NOT split remaining
// space evenly (each measures 0); use style.size, explicit heights, or
// layout.justify instead. Implementing true flex needs an engine layout
// channel, which is outside the widget seam.
type Spacer struct{}

// Measure reports the fixed square for style.size (HTML numOK(n.Style,
// "size")), else 0×0 — see the type doc for the flex degradation.
func (Spacer) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	if v, ok := styleNumber(n, "size"); ok && v > 0 {
		return v * scale, v * scale
	}
	return 0, 0
}

// Record draws nothing: a spacer is pure negative space in both HTML and
// canvas.
func (Spacer) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return nil
}
