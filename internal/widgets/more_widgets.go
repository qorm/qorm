package widgets

// Canvas ports: rangeslider, pageview, tree, autocomplete.

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("rangeslider", &RangeSlider{
		local: map[*model.Node][2]float64{},
		geoms: map[*model.Node]rangeGeom{},
		which: map[*model.Node]int{},
	})
	canvas.RegisterWidget("pageview", &PageView{
		index: map[*model.Node]int{},
		geoms: map[*model.Node]image.Rectangle{},
	})
	canvas.RegisterWidget("tree", &Tree{
		open: map[string]bool{},
	})
	canvas.RegisterWidget("autocomplete", &Autocomplete{
		open:     map[*model.Node]bool{},
		geoms:    map[*model.Node]acGeom{},
		hoverRow: map[*model.Node]int{},
		inters:   map[*model.Node]*canvas.Interaction{},
	})
}

// ---- rangeslider ------------------------------------------------------------

type rangeGeom struct {
	box    image.Rectangle
	thumbR float64
}

// RangeSlider is a dual-thumb range control bound via low/high props.
type RangeSlider struct {
	mu    sync.Mutex
	local map[*model.Node][2]float64
	geoms map[*model.Node]rangeGeom
	which map[*model.Node]int // 0=lo, 1=hi while dragging
}

func (s *RangeSlider) Measure(_ *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 160 * scale, 32 * scale
}

func (s *RangeSlider) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	thumbR := float64(sliderThumbD * scale / 2)
	s.mu.Lock()
	s.geoms[ln.Node] = rangeGeom{
		box:    image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height),
		thumbR: thumbR,
	}
	s.mu.Unlock()
	lo, hi := s.rangeVals(ln.Node, rt)
	minV, maxV := s.minMax(ln.Node)
	span := maxV - minV
	if span == 0 {
		span = 1
	}
	trackH := float64(sliderTrackH * scale)
	trackY := (float64(ln.Height) - trackH) / 2
	w := float64(ln.Width)
	travel := w - 2*thumbR
	if travel < 1 {
		travel = 1
	}
	loX := thumbR + (lo-minV)/span*travel
	hiX := thumbR + (hi-minV)/span*travel

	g := draw.NewGroup()
	track := draw.NewRect()
	track.Y = trackY
	track.Width, track.Height = w, trackH
	track.BorderRadius = trackH / 2
	track.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	g.AddChild(track)
	if hiX > loX {
		fill := draw.NewRect()
		fill.NoHit = true
		fill.X, fill.Y = loX, trackY
		fill.Width, fill.Height = hiX-loX, trackH
		fill.BorderRadius = trackH / 2
		fill.Fill = formAccent(rt)
		g.AddChild(fill)
	}
	for _, cx := range []float64{loX, hiX} {
		th := draw.NewCircle()
		th.X, th.Y = cx-thumbR, float64(ln.Height)/2-thumbR
		th.Radius = thumbR
		th.Fill = color.RGBA{255, 255, 255, 255}
		th.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
		th.StrokeWidth = float64(scale)
		g.AddChild(th)
	}
	return g
}

func (s *RangeSlider) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	s.mu.Lock()
	geo, ok := s.geoms[n]
	s.mu.Unlock()
	if !ok {
		return false
	}
	switch p.Type {
	case canvas.PointerPress:
		inter.Pressed = n
		inter.Focused = n
		inter.FocusVisible = false
		// Pick nearest thumb.
		lo, hi := s.rangeVals(n, rt)
		minV, maxV := s.minMax(n)
		span := maxV - minV
		if span == 0 {
			span = 1
		}
		w := float64(geo.box.Dx())
		travel := w - 2*geo.thumbR
		if travel < 1 {
			travel = 1
		}
		loX := float64(geo.box.Min.X) + geo.thumbR + (lo-minV)/span*travel
		hiX := float64(geo.box.Min.X) + geo.thumbR + (hi-minV)/span*travel
		which := 0
		if math.Abs(p.X-hiX) < math.Abs(p.X-loX) {
			which = 1
		}
		s.mu.Lock()
		s.which[n] = which
		s.mu.Unlock()
		s.setFromX(n, rt, p.X, which)
		return true
	case canvas.PointerMove:
		if inter.Pressed != n {
			return false
		}
		s.mu.Lock()
		which := s.which[n]
		s.mu.Unlock()
		s.setFromX(n, rt, p.X, which)
		return true
	case canvas.PointerRelease:
		if inter.Pressed != n {
			return false
		}
		s.mu.Lock()
		which := s.which[n]
		delete(s.which, n)
		s.mu.Unlock()
		s.setFromX(n, rt, p.X, which)
		lo, hi := s.rangeVals(n, rt)
		commitFormChange(n, rt, map[string]any{"low": lo, "high": hi})
		return true
	}
	return false
}

func (s *RangeSlider) minMax(n *model.Node) (minV, maxV float64) {
	minV, maxV = 0, 100
	if v, ok := n.Prop("min"); ok {
		minV = formFloat(v)
	}
	if v, ok := n.Prop("max"); ok {
		maxV = formFloat(v)
	}
	if maxV <= minV {
		maxV = minV + 1
	}
	return minV, maxV
}

func (s *RangeSlider) rangeVals(n *model.Node, rt *runtime.Runtime) (lo, hi float64) {
	minV, maxV := s.minMax(n)
	lo, hi = minV, maxV
	if raw, ok := n.Prop("low"); ok {
		lo = formFloat(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt)))
	}
	if raw, ok := n.Prop("high"); ok {
		hi = formFloat(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt)))
	}
	// Unbound fallback.
	if formBoundPath(fmt.Sprint(n.Props["low"])) == "" && formBoundPath(fmt.Sprint(n.Props["high"])) == "" {
		s.mu.Lock()
		if v, ok := s.local[n]; ok {
			lo, hi = v[0], v[1]
		}
		s.mu.Unlock()
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < minV {
		lo = minV
	}
	if hi > maxV {
		hi = maxV
	}
	return lo, hi
}

func (s *RangeSlider) setFromX(n *model.Node, rt *runtime.Runtime, x float64, which int) {
	s.mu.Lock()
	geo, ok := s.geoms[n]
	s.mu.Unlock()
	if !ok {
		return
	}
	minV, maxV := s.minMax(n)
	span := maxV - minV
	w := float64(geo.box.Dx())
	travel := w - 2*geo.thumbR
	if travel < 1 {
		travel = 1
	}
	frac := (x - float64(geo.box.Min.X) - geo.thumbR) / travel
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	val := minV + frac*span
	if step, ok := n.Prop("step"); ok {
		st := formFloat(step)
		if st > 0 {
			val = minV + math.Round((val-minV)/st)*st
		}
	}
	lo, hi := s.rangeVals(n, rt)
	if which == 0 {
		lo = val
		if lo > hi {
			lo = hi
		}
	} else {
		hi = val
		if hi < lo {
			hi = lo
		}
	}
	// Write bindings.
	if path := formBoundPath(fmt.Sprint(n.Props["low"])); path != "" {
		rt.SetStatePath(path, lo)
	}
	if path := formBoundPath(fmt.Sprint(n.Props["high"])); path != "" {
		rt.SetStatePath(path, hi)
	}
	if formBoundPath(fmt.Sprint(n.Props["low"])) == "" && formBoundPath(fmt.Sprint(n.Props["high"])) == "" {
		s.mu.Lock()
		s.local[n] = [2]float64{lo, hi}
		s.mu.Unlock()
	}
}

// ---- pageview ---------------------------------------------------------------

// PageView is a full-width page carousel (one child page at a time).
type PageView struct {
	mu    sync.Mutex
	index map[*model.Node]int
	geoms map[*model.Node]image.Rectangle
}

func (p *PageView) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w, h = 300*scale, 200*scale
	for _, ch := range n.Children {
		if cln := canvas.Measure(ch, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			if cln.Height > h {
				h = cln.Height
			}
		}
	}
	return w, h
}

func (p *PageView) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return p.record(ln, rt, scale, nil)
}

func (p *PageView) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return p.record(ln, rt, scale, sinks)
}

func (p *PageView) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil || ln.Node == nil || len(ln.Children) == 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	kids := ln.Children
	n := len(kids)
	i := 0
	p.mu.Lock()
	i = p.index[ln.Node]
	if i >= n {
		i = n - 1
		p.index[ln.Node] = i
	}
	p.geoms[ln.Node] = image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)
	p.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Clip = true
	child := kids[i]
	bounds := image.Rect(0, 0, ln.Width, ln.Height)
	if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
		g.AddChild(cn)
	}
	ln.Children = nil
	return g
}

func (p *PageView) HandlePointer(n *model.Node, rt *runtime.Runtime, pi canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) || pi.Type != canvas.PointerPress {
		return false
	}
	count := len(n.Children)
	if count == 0 {
		return false
	}
	p.mu.Lock()
	box := p.geoms[n]
	cur := p.index[n]
	p.mu.Unlock()
	if box.Empty() {
		box = frame
	}
	relX := int(pi.X) - box.Min.X
	w := box.Dx()
	if w < 1 {
		w = 1
	}
	if relX < w/3 {
		cur--
	} else if relX > 2*w/3 {
		cur++
	} else {
		cur = (cur + 1) % count
	}
	if cur < 0 {
		cur = 0
	}
	if cur >= count {
		cur = count - 1
	}
	p.mu.Lock()
	p.index[n] = cur
	p.mu.Unlock()
	return true
}

// ---- tree -------------------------------------------------------------------

// Tree renders hierarchical data with expand/collapse.
type Tree struct {
	mu   sync.Mutex
	open map[string]bool // key = path of labels
}

func (t *Tree) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	items := t.flat(n, rt, scale, fs)
	w = 200 * scale
	for _, it := range items {
		tw := it.depth*16*scale + int(canvas.MeasureText(it.label, float64(fs))) + 24*scale
		if tw > w {
			w = tw
		}
	}
	h = len(items) * lineHeight(fs)
	if h < 1 {
		h = lineHeight(fs)
	}
	return w, h
}

type treeRow struct {
	label string
	path  string
	depth int
	leaf  bool
	open  bool
}

func (t *Tree) flat(n *model.Node, rt *runtime.Runtime, scale, fs int) []treeRow {
	arr := boundArray(n, rt, "data")
	if len(arr) == 0 {
		return nil
	}
	defOpen := !propTruthy(n, "collapsed")
	var out []treeRow
	var walk func(items []any, depth int, prefix string)
	walk = func(items []any, depth int, prefix string) {
		for i, it := range items {
			label, kids, nodeOpen := treeNode(it, defOpen)
			path := prefix + "/" + label + fmt.Sprintf("#%d", i)
			leaf := len(kids) == 0
			open := nodeOpen
			if !leaf {
				t.mu.Lock()
				if v, ok := t.open[path]; ok {
					open = v
				}
				t.mu.Unlock()
			}
			out = append(out, treeRow{label: label, path: path, depth: depth, leaf: leaf, open: open})
			if !leaf && open {
				walk(kids, depth+1, path)
			}
		}
	}
	walk(arr, 0, "")
	_ = scale
	_ = fs
	return out
}

func treeNode(v any, defOpen bool) (label string, kids []any, open bool) {
	open = defOpen
	switch t := v.(type) {
	case map[string]any:
		label = fmt.Sprint(t["label"])
		if label == "" || label == "<nil>" {
			label = fmt.Sprint(t)
		}
		kids, _ = t["children"].([]any)
		if e, ok := t["expanded"]; ok {
			switch b := e.(type) {
			case bool:
				open = b
			case string:
				open = b == "true" || b == "1"
			}
		}
	default:
		label = fmt.Sprint(v)
	}
	return label, kids, open
}

func (t *Tree) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	rows := t.flat(ln.Node, rt, scale, fs)
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	lh := lineHeight(fs)
	for i, row := range rows {
		x := float64(row.depth*16*scale + 4*scale)
		y := float64(i * lh)
		prefix := "• "
		if !row.leaf {
			if row.open {
				prefix = "▾ "
			} else {
				prefix = "▸ "
			}
		}
		g.AddChild(formText(prefix+row.label, x, y, fs, ink))
	}
	return g
}

func (t *Tree) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	fs := formFontSize(n, 1)
	// Need scale from frame roughly — use 1 if unknown.
	scale := 1
	if frame.Dy() > 0 {
		// best effort
	}
	rows := t.flat(n, rt, scale, fs*scale)
	if len(rows) == 0 {
		return false
	}
	lh := lineHeight(fs * scale)
	if lh < 1 {
		lh = 1
	}
	relY := int(p.Y) - frame.Min.Y
	idx := relY / lh
	if idx < 0 || idx >= len(rows) {
		return false
	}
	row := rows[idx]
	if row.leaf {
		return false
	}
	t.mu.Lock()
	t.open[row.path] = !row.open
	t.mu.Unlock()
	return true
}

// ---- autocomplete -----------------------------------------------------------

// Autocomplete is a single-line field with a filtered options dropdown
// (HTML datalist-style). Shares edit session via editableType.
type Autocomplete struct {
	mu       sync.Mutex
	open     map[*model.Node]bool
	geoms    map[*model.Node]acGeom
	hoverRow map[*model.Node]int
	inters   map[*model.Node]*canvas.Interaction
}

type acGeom struct {
	box   image.Rectangle
	rowH  int
	panel image.Rectangle
}

func (a *Autocomplete) cacheInter(n *model.Node, inter *canvas.Interaction) {
	if inter == nil {
		return
	}
	a.mu.Lock()
	a.inters[n] = inter
	a.mu.Unlock()
}

func (a *Autocomplete) sessionFor(n *model.Node) *canvas.InputState {
	a.mu.Lock()
	inter := a.inters[n]
	a.mu.Unlock()
	if inter == nil || inter.Input == nil || inter.Input.Node != n {
		return nil
	}
	return inter.Input
}

func (*Autocomplete) Measure(n *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 220 * scale, 40 * scale
}

func (a *Autocomplete) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	a.mu.Lock()
	a.geoms[ln.Node] = acGeom{box: image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)}
	a.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	chrome := draw.NewRect()
	chrome.Width, chrome.Height = float64(ln.Width), float64(ln.Height)
	chrome.BorderRadius = 8 * float64(scale)
	chrome.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	chrome.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	chrome.StrokeWidth = float64(scale)
	g.AddChild(chrome)
	fs := formFontSizeLN(ln, scale)
	pad := float64(12 * scale)
	text, ph := a.display(ln.Node, rt)
	ink := formInk(ln.Node, ln, rt)
	if ph {
		ink = themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	}
	if text != "" {
		g.AddChild(formText(text, pad, (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, ink))
	}
	if sess := a.sessionFor(ln.Node); sess != nil {
		if int(time.Since(sess.BlinkStart)/(500*time.Millisecond))%2 == 0 {
			cx := pad + float64(int(canvas.MeasureText(string(sess.Runes[:min(sess.Cursor, len(sess.Runes))]), float64(fs))))
			caret := draw.NewRect()
			caret.NoHit = true
			caret.X = cx
			caret.Y = float64(8 * scale)
			caret.Width = float64(scale)
			if caret.Width < 1 {
				caret.Width = 1
			}
			caret.Height = float64(ln.Height - 16*scale)
			caret.Fill = formInk(ln.Node, ln, rt)
			g.AddChild(caret)
		}
	}
	return g
}

func (a *Autocomplete) display(n *model.Node, rt *runtime.Runtime) (string, bool) {
	if sess := a.sessionFor(n); sess != nil {
		return string(sess.Runes), len(sess.Runes) == 0
	}
	v := formEvalStr(n.Value, rt)
	if v != "" {
		return v, false
	}
	return n.Placeholder, true
}

func (a *Autocomplete) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	if formDisabled(n, rt) {
		return false
	}
	a.mu.Lock()
	open := a.open[n]
	a.mu.Unlock()
	return open && len(a.filtered(n, rt)) > 0
}

func (a *Autocomplete) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !a.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	opts := a.filtered(ln.Node, rt)
	if len(opts) > 8 {
		opts = opts[:8]
	}
	rowH := 36 * scale
	menuPad := 4 * scale
	panelH := 2*menuPad + len(opts)*rowH
	panelW := ln.Width
	panelX, panelY := ln.AbsX, ln.AbsY+ln.Height+4*scale
	stageW, stageH := panelX+panelW+8, panelY+panelH+8

	a.mu.Lock()
	geo := a.geoms[ln.Node]
	geo.rowH = rowH
	geo.panel = image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)
	a.geoms[ln.Node] = geo
	hover := a.hoverRow[ln.Node]
	a.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(stageW), float64(stageH)
	g.Model = ln.Node
	g.Overlay = true
	backdrop := draw.NewRect()
	backdrop.Width, backdrop.Height = float64(stageW), float64(stageH)
	g.AddChild(backdrop)
	panel := draw.NewRect()
	panel.X, panel.Y = float64(panelX), float64(panelY)
	panel.Width, panel.Height = float64(panelW), float64(panelH)
	panel.BorderRadius = 8 * float64(scale)
	panel.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panel.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	panel.StrokeWidth = float64(scale)
	panel.ShadowColor = color.RGBA{0, 0, 0, 32}
	panel.ShadowBlur = 12 * float64(scale)
	panel.ShadowY = 4 * float64(scale)
	g.AddChild(panel)
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	for i, o := range opts {
		y := float64(panelY + menuPad + i*rowH)
		if i == hover {
			sel := draw.NewRect()
			sel.NoHit = true
			sel.X, sel.Y = float64(panelX+menuPad), y
			sel.Width = float64(panelW - 2*menuPad)
			sel.Height = float64(rowH)
			sel.BorderRadius = 6 * float64(scale)
			sel.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
			g.AddChild(sel)
		}
		g.AddChild(formText(o, float64(panelX+menuPad+8*scale), y+float64(8*scale), fs, ink))
	}
	return g
}

func (a *Autocomplete) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	a.cacheInter(n, inter)
	a.mu.Lock()
	geo := a.geoms[n]
	a.mu.Unlock()

	if p.Type == canvas.PointerPress && !geo.panel.Empty() && image.Pt(int(p.X), int(p.Y)).In(geo.panel) {
		idx := (int(p.Y) - geo.panel.Min.Y - 4) / max(geo.rowH, 1)
		opts := a.filtered(n, rt)
		if idx >= 0 && idx < len(opts) {
			label := opts[idx]
			if path := formBoundPath(n.Value); path != "" {
				rt.SetStatePath(path, label)
			}
			a.mu.Lock()
			a.open[n] = false
			a.mu.Unlock()
			commitFormChange(n, rt, label)
			return true
		}
	}
	if p.Type == canvas.PointerMove && !geo.panel.Empty() && image.Pt(int(p.X), int(p.Y)).In(geo.panel) {
		idx := (int(p.Y) - geo.panel.Min.Y - 4) / max(geo.rowH, 1)
		a.mu.Lock()
		a.hoverRow[n] = idx
		a.mu.Unlock()
		return true
	}
	if p.Type == canvas.PointerPress {
		inter.Focused = n
		inter.FocusVisible = false
		a.mu.Lock()
		a.open[n] = true
		a.mu.Unlock()
		return true
	}
	return false
}

func (a *Autocomplete) OnFocused(n *model.Node, inter *canvas.Interaction) {
	a.cacheInter(n, inter)
	a.mu.Lock()
	a.open[n] = true
	a.mu.Unlock()
}

func (a *Autocomplete) filtered(n *model.Node, rt *runtime.Runtime) []string {
	q := ""
	if sess := a.sessionFor(n); sess != nil {
		q = strings.ToLower(string(sess.Runes))
	} else {
		q = strings.ToLower(formEvalStr(n.Value, rt))
	}
	// Literal array or binding ({{state.suggestions}}), same as HTML path.
	opts := formOptions(boundArray(n, rt, "options"))
	if len(opts) == 0 {
		opts = formOptions(n.Props["options"])
	}
	var out []string
	for _, o := range opts {
		lbl := o.label
		if lbl == "" {
			lbl = o.value
		}
		if q == "" || strings.Contains(strings.ToLower(lbl), q) {
			out = append(out, lbl)
		}
	}
	return out
}
