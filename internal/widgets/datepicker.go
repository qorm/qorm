package widgets

// The date picker (HTML: render_widgets.go:426) — three side-by-side columns
// (month, day, year); tapping a row updates the bound date ("YYYY-MM-DD") and
// dispatches onChange. The selected row of each column is highlighted with the
// accent color; the day clamps to the month's length. A wheel-scroll variant
// is a later milestone — this is the tappable-column form.

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

// DatePicker is the three-column date selector.
type DatePicker struct {
	mu    sync.Mutex
	geoms map[*model.Node]*dpGeo
}

func init() {
	canvas.RegisterWidget("datepicker", &DatePicker{geoms: map[*model.Node]*dpGeo{}})
	canvas.RegisterWidget("cupertinodatepicker", &DatePicker{geoms: map[*model.Node]*dpGeo{}})
}

// dpGeo is the laid-out column/row geometry in physical px, stashed by Record
// so HandlePointer maps the screen-space press to a (column, row) at the same
// scale the columns were drawn.
type dpGeo struct {
	colW, rowH float64
}

func (d *DatePicker) geo(n *model.Node) *dpGeo {
	d.mu.Lock()
	defer d.mu.Unlock()
	g := d.geoms[n]
	if g == nil {
		g = &dpGeo{}
		d.geoms[n] = g
	}
	return g
}

const (
	dpColW = 76 // one column's width
	dpRowH = 24 // one row's height
	dpGap  = 4  // gap between columns
	dpMaxD = 31 // the widest column's row count (days)
)

// dpMonths are the month labels (the HTML wheel's spellings).
var dpMonths = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// dpParseDate splits a "YYYY-MM-DD" value; out-of-range fields fall back to
// today-ish defaults (the HTML parseDate3 uses 2026-07-01; the canvas keeps
// the same spelling but defaults to 2026-01-01 so no date logic is clocked).
func dpParseDate(s string) (y, m, d int) {
	y, m, d = 2026, 1, 1
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return
	}
	if v, err := strconv.Atoi(parts[0]); err == nil && v > 0 {
		y = v
	}
	if v, err := strconv.Atoi(parts[1]); err == nil && v >= 1 && v <= 12 {
		m = v
	}
	if v, err := strconv.Atoi(parts[2]); err == nil && v >= 1 && v <= 31 {
		d = v
	}
	return
}

func dpFmtDate(y, m, d int) string { return fmt.Sprintf("%04d-%02d-%02d", y, m, d) }

// dpMonthDays returns the length of a proleptic-Gregorian month (leap rule
// spelled out — no clock, pure arithmetic, like the HTML monthDays).
func dpMonthDays(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if (y%4 == 0 && y%100 != 0) || y%400 == 0 {
			return 29
		}
		return 28
	}
	return 30
}

// dpYearRange reads the minYear/maxYear props (defaults 2020..2035 like the
// HTML wheel) and returns the inclusive range.
func dpYearRange(n *model.Node, rt *runtime.Runtime) (min, max int) {
	min, max = 2020, 2035
	if raw, ok := n.Prop("minYear"); ok {
		if v := formFloat(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil))); v > 0 {
			min = int(v)
		}
	}
	if raw, ok := n.Prop("maxYear"); ok {
		if v := formFloat(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil))); v > 0 {
			max = int(v)
		}
	}
	if max < min {
		max = min
	}
	return
}

// Measure reports the three columns' size: the widest column (31 day rows)
// tall, three columns wide with gaps.
func (*DatePicker) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	_, maxY := dpYearRange(n, rt)
	rows := max(dpMaxD, maxY-2020+1)
	return (3*dpColW + 2*dpGap) * scale, rows * dpRowH * scale
}

// Record draws the three columns; each selected row gets the accent band.
func (d *DatePicker) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	y, m, day := dpParseDate(formEvalStr(ln.Node.Value, rt))
	minY, maxY := dpYearRange(ln.Node, rt)
	ink := formInk(ln.Node, ln, rt)
	fs := formFontSizeLN(ln, scale)
	colW := float64(dpColW * scale)
	rowH := float64(dpRowH * scale)
	g := d.geo(ln.Node)
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

	months := make([]string, 12)
	for i := range months {
		months[i] = dpMonths[i]
	}
	days := make([]string, dpMaxD)
	for i := range days {
		days[i] = strconv.Itoa(i + 1)
	}
	years := make([]string, 0, maxY-minY+1)
	for yr := minY; yr <= maxY; yr++ {
		years = append(years, strconv.Itoa(yr))
	}

	drawCol(0, months, m-1)
	drawCol(float64((dpColW+dpGap)*scale), days, day-1)
	drawCol(float64(2*(dpColW+dpGap)*scale), years, y-minY)
	return grp
}

// HandlePointer maps the press to a column + row and updates the date.
func (*DatePicker) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	// The press is in scene px; the widget's own box starts at ln.AbsX/AbsY
	// (the frame param is the same box).
	x := p.X - float64(frame.Min.X)
	y := p.Y - float64(frame.Min.Y)
	if x < 0 || y < 0 {
		return false
	}
	col := int(x / float64((dpColW+dpGap)))
	row := int(y / float64(dpRowH))
	if col < 0 || col > 2 {
		return false
	}
	curY, curM, curD := dpParseDate(formEvalStr(n.Value, rt))
	minY, maxY := dpYearRange(n, rt)
	switch col {
	case 0: // month
		if row < 0 || row > 11 {
			return false
		}
		curM = row + 1
	case 1: // day
		if row < 0 || row > dpMaxD-1 {
			return false
		}
		curD = row + 1
	case 2: // year
		if row < 0 || row > maxY-minY {
			return false
		}
		curY = minY + row
	}
	if curD > dpMonthDays(curY, curM) {
		curD = dpMonthDays(curY, curM)
	}
	inter.Focused = n
	inter.FocusVisible = false
	val := dpFmtDate(curY, curM, curD)
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, val)
	} else {
		// Uncontrolled: keep the local pick in the widget's own map? The date
		// picker has no local store in v1 — an unbound picker is read-only.
		return true
	}
	commitFormChange(n, rt, val)
	return true
}

// Inline keeps the picker's content size in flex containers.
func (DatePicker) Inline() {}
