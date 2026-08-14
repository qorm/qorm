package widgets

// The time picker (HTML: render_widgets.go:864) — two side-by-side columns
// (hour 0..23, minute 0..59 in minuteStep increments); tapping a row updates
// the bound time ("HH:MM") and dispatches onChange. Same tappable-column form
// as the date picker; a wheel-scroll variant is a later milestone.

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("timepicker", &TimePicker{geoms: map[*model.Node]*tpGeo{}})
	canvas.RegisterWidget("cupertinotimepicker", &TimePicker{geoms: map[*model.Node]*tpGeo{}})
}

// TimePicker is the two-column time selector.
type TimePicker struct {
	mu    sync.Mutex
	geoms map[*model.Node]*tpGeo
}

// tpGeo is the laid-out column geometry in physical px (stashed by Record,
// reused by HandlePointer at the same scale).
type tpGeo struct {
	colW, rowH float64
}

func (t *TimePicker) geo(n *model.Node) *tpGeo {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.geoms[n]
	if g == nil {
		g = &tpGeo{}
		t.geoms[n] = g
	}
	return g
}

const (
	tpColW  = 76 // one column's width
	tpRowH  = 24 // one row's height
	tpGap   = 4  // gap between columns
	tpHours = 24
	tpMins  = 60
)

func tpParse(s string) (h, m int) {
	h, m = 9, 0
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return
	}
	if v, err := strconv.Atoi(parts[0]); err == nil && v >= 0 && v <= 23 {
		h = v
	}
	if v, err := strconv.Atoi(parts[1]); err == nil && v >= 0 && v <= 59 {
		m = v
	}
	return
}

func tpFmt(h, m int) string { return fmt.Sprintf("%02d:%02d", h, m) }

// tpStep reads the minuteStep prop (default 1), clamped to [1,60].
func tpStep(n *model.Node, rt *runtime.Runtime) int {
	step := 1
	if raw, ok := n.Prop("minuteStep"); ok {
		if v := formFloat(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil))); v >= 1 {
			step = int(v)
		}
	}
	if step > 60 {
		step = 60
	}
	return step
}

// Measure reports the two columns' size: the HOUR column (24 rows) tall so no
// hour is clipped — the minute column (60/step rows) draws fewer rows and
// leaves the tail blank — two columns wide with a gap.
func (*TimePicker) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return (2*tpColW + tpGap) * scale, tpHours * tpRowH * scale
}

// Record draws the two columns; each selected row gets the accent band.
func (t *TimePicker) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	h, m := tpParse(formEvalStr(ln.Node.Value, rt))
	step := tpStep(ln.Node, rt)
	ink := formInk(ln.Node, ln, rt)
	fs := formFontSizeLN(ln, scale)
	colW := float64(tpColW * scale)
	rowH := float64(tpRowH * scale)
	g := t.geo(ln.Node)
	g.colW = colW
	g.rowH = rowH

	grp := draw.NewGroup()
	drawCol := func(x float64, labels []string, selected int) {
		for i, label := range labels {
			cy := float64(i) * rowH
			if i == selected {
				band := draw.NewRect()
				band.X = x + 4*float64(scale)
				band.Y = cy + 2*float64(scale)
				band.Width = colW - 8*float64(scale)
				band.Height = rowH - 4*float64(scale)
				band.BorderRadius = 6 * float64(scale)
				band.Fill = formAccent(rt)
				grp.AddChild(band)
			}
			grp.AddChild(formText(label, x+(colW-float64(int(canvas.MeasureText(label, float64(fs)))))/2,
				cy+(rowH-float64(lineHeight(fs)))/2, fs, ink))
		}
	}

	hours := make([]string, tpHours)
	for i := range hours {
		hours[i] = fmt.Sprintf("%02d", i)
	}
	mins := make([]string, 0, tpMins/step)
	for i := 0; i < tpMins; i += step {
		mins = append(mins, fmt.Sprintf("%02d", i))
	}

	drawCol(0, hours, h)
	drawCol(float64((tpColW+tpGap)*scale), mins, m/step)
	return grp
}

// HandlePointer maps the press to an hour/minute row and updates the time.
func (t *TimePicker) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	x := p.X - float64(frame.Min.X)
	y := p.Y - float64(frame.Min.Y)
	if x < 0 || y < 0 {
		return false
	}
	g := t.geo(n)
	col := int(x / float64((tpColW + tpGap)))
	row := int(y / g.rowH)
	if col < 0 || col > 1 {
		return false
	}
	curH, curM := tpParse(formEvalStr(n.Value, rt))
	step := tpStep(n, rt)
	switch col {
	case 0: // hour
		if row < 0 || row > tpHours-1 {
			return false
		}
		curH = row
	case 1: // minute (in step increments)
		maxRow := tpMins/step - 1
		if row < 0 || row > maxRow {
			return false
		}
		curM = row * step
	}
	inter.Focused = n
	inter.FocusVisible = false
	val := tpFmt(curH, curM)
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, val)
	} else {
		return true // unbound: read-only
	}
	commitFormChange(n, rt, val)
	return true
}

// Inline keeps the picker's content size in flex containers.
func (*TimePicker) Inline() {}
