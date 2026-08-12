package widgets

// Canvas ports for remaining htmlOnlyCoreAllowlist types:
// alertdialog, actionsheet, descriptions, materialstepper, monthview,
// motion, picker, rating, refreshindicator, selectabletext, transform.
// (dropdownbutton registers via Select in select.go.)

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("alertdialog", &AlertDialog{geoms: map[*model.Node]*alertGeo{}})
	canvas.RegisterWidget("cupertinoalertdialog", &AlertDialog{geoms: map[*model.Node]*alertGeo{}})
	canvas.RegisterWidget("actionsheet", &ActionSheet{geoms: map[*model.Node]*actionSheetGeo{}})
	canvas.RegisterWidget("cupertinoactionsheet", &ActionSheet{geoms: map[*model.Node]*actionSheetGeo{}})
	canvas.RegisterWidget("descriptions", Descriptions{})
	canvas.RegisterWidget("keyvalue", Descriptions{})
	canvas.RegisterWidget("materialstepper", MaterialStepper{})
	canvas.RegisterWidget("monthview", &MonthView{geoms: map[*model.Node]*monthGeo{}})
	canvas.RegisterWidget("calendarview", &MonthView{geoms: map[*model.Node]*monthGeo{}})
	canvas.RegisterWidget("datepickercalendar", &MonthView{geoms: map[*model.Node]*monthGeo{}})
	canvas.RegisterWidget("motion", Motion{})
	canvas.RegisterWidget("animated", Motion{})
	canvas.RegisterWidget("transition", Motion{})
	canvas.RegisterWidget("animatedswitcher", Motion{})
	canvas.RegisterWidget("transform", Transform{})
	canvas.RegisterWidget("rotatedbox", Transform{})
	canvas.RegisterWidget("picker", &Picker{local: map[*model.Node]string{}, geoms: map[*model.Node]*pickerGeo{}})
	canvas.RegisterWidget("cupertinopicker", &Picker{local: map[*model.Node]string{}, geoms: map[*model.Node]*pickerGeo{}})
	canvas.RegisterWidget("rating", &Rating{local: map[*model.Node]int{}, geoms: map[*model.Node]*ratingGeo{}})
	canvas.RegisterWidget("refreshindicator", &RefreshIndicator{
		dragY:  map[*model.Node]float64{},
		startY: map[*model.Node]float64{},
	})
	canvas.RegisterWidget("selectabletext", &SelectableText{inters: map[*model.Node]*canvas.Interaction{}})
}

// ---- shared dialog actions --------------------------------------------------

type dlgAction struct {
	label, style string
	inv          *model.Invoke
}

func parseDlgActions(n *model.Node, key string, rt *runtime.Runtime) []dlgAction {
	raw, ok := n.Prop(key)
	if !ok {
		return nil
	}
	arr, _ := raw.([]any)
	var out []dlgAction
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		label := formEvalStr(fmt.Sprint(m["label"]), rt)
		style, _ := m["style"].(string)
		var inv *model.Invoke
		if op, ok := m["onPress"]; ok {
			inv = propInvokeWidget(op)
			if inv == nil {
				if s, ok := op.(string); ok && s != "" {
					inv = &model.Invoke{Name: s}
				}
			}
		}
		out = append(out, dlgAction{label: label, style: style, inv: inv})
	}
	return out
}

func actionInk(style string, rt *runtime.Runtime) color.RGBA {
	if style == "destructive" {
		return themeColor(rt, "danger", color.RGBA{255, 59, 48, 255})
	}
	return formAccent(rt)
}

// ---- alertdialog ------------------------------------------------------------

type alertGeo struct {
	panel   image.Rectangle
	actions []image.Rectangle
	idxs    []int // action indices
}

// AlertDialog is a centered modal card with title, message and action buttons.
type AlertDialog struct {
	mu    sync.Mutex
	geoms map[*model.Node]*alertGeo
}

func (a *AlertDialog) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 270 * scale, 160 * scale
}

func (a *AlertDialog) Record(_ *canvas.LayoutNode, _ *runtime.Runtime, _ int) draw.Node {
	return nil
}

func (a *AlertDialog) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return modalOpen(n, rt)
}

func (a *AlertDialog) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, _ image.Point) draw.Node {
	if ln == nil || !a.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	panelW := 270 * scale
	if panelW > stageW-40*scale {
		panelW = stageW - 40*scale
	}
	actions := parseDlgActions(ln.Node, "actions", rt)
	title := formTitle(ln.Node, rt)
	msg := ""
	if raw, ok := ln.Node.Prop("message"); ok {
		msg = formEvalStr(fmt.Sprint(raw), rt)
	}
	fs := 17 * scale
	msgFS := 13 * scale
	bodyH := 18 * scale
	if title != "" {
		bodyH += lineHeight(fs) + 4*scale
	}
	if msg != "" {
		bodyH += lineHeight(msgFS) + 4*scale
	}
	btnH := 44 * scale
	rows := 1
	if len(actions) != 2 {
		rows = max(1, len(actions))
	}
	panelH := bodyH + rows*btnH
	panelX := (stageW - panelW) / 2
	panelY := (stageH - panelH) / 2
	if panelY < 10*scale {
		panelY = 10 * scale
	}

	g := draw.NewGroup()
	g.Width, g.Height = float64(stageW), float64(stageH)
	g.Model = ln.Node
	g.Overlay = true
	bg := draw.NewRect()
	bg.Width, bg.Height = float64(stageW), float64(stageH)
	bg.Fill = color.RGBA{0, 0, 0, 115}
	g.AddChild(bg)
	panel := draw.NewRect()
	panel.X, panel.Y = float64(panelX), float64(panelY)
	panel.Width, panel.Height = float64(panelW), float64(panelH)
	panel.BorderRadius = 14 * float64(scale)
	panel.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	g.AddChild(panel)

	y := float64(panelY + 18*scale)
	ink := themeColor(rt, "text", color.RGBA{0, 0, 0, 255})
	if title != "" {
		tw := int(canvas.MeasureText(title, float64(fs)))
		g.AddChild(formText(title, float64(panelX+(panelW-tw)/2), y, fs, ink))
		y += float64(lineHeight(fs) + 4*scale)
	}
	if msg != "" {
		tw := int(canvas.MeasureText(msg, float64(msgFS)))
		g.AddChild(formText(msg, float64(panelX+(panelW-tw)/2), y, msgFS, ink))
	}

	geo := &alertGeo{panel: image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)}
	sideBySide := len(actions) == 2
	btnTop := panelY + bodyH
	if sideBySide {
		bw := panelW / 2
		for i, act := range actions {
			x := panelX + i*bw
			r := image.Rect(x, btnTop, x+bw, btnTop+btnH)
			geo.actions = append(geo.actions, r)
			geo.idxs = append(geo.idxs, i)
			if i > 0 {
				sep := draw.NewRect()
				sep.NoHit = true
				sep.X, sep.Y = float64(x), float64(btnTop)
				sep.Width, sep.Height = float64(scale), float64(btnH)
				sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
				g.AddChild(sep)
			}
			topLine := draw.NewRect()
			topLine.NoHit = true
			topLine.X, topLine.Y = float64(x), float64(btnTop)
			topLine.Width, topLine.Height = float64(bw), float64(scale)
			topLine.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			g.AddChild(topLine)
			weightFS := fs
			if act.style == "cancel" {
				weightFS = fs // bold not available; same size
			}
			tw := int(canvas.MeasureText(act.label, float64(weightFS)))
			g.AddChild(formText(act.label, float64(x+(bw-tw)/2), float64(btnTop+(btnH-lineHeight(weightFS))/2), weightFS, actionInk(act.style, rt)))
		}
	} else {
		for i, act := range actions {
			y0 := btnTop + i*btnH
			r := image.Rect(panelX, y0, panelX+panelW, y0+btnH)
			geo.actions = append(geo.actions, r)
			geo.idxs = append(geo.idxs, i)
			topLine := draw.NewRect()
			topLine.NoHit = true
			topLine.X, topLine.Y = float64(panelX), float64(y0)
			topLine.Width, topLine.Height = float64(panelW), float64(scale)
			topLine.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			g.AddChild(topLine)
			tw := int(canvas.MeasureText(act.label, float64(fs)))
			g.AddChild(formText(act.label, float64(panelX+(panelW-tw)/2), float64(y0+(btnH-lineHeight(fs))/2), fs, actionInk(act.style, rt)))
		}
	}
	a.mu.Lock()
	a.geoms[ln.Node] = geo
	a.mu.Unlock()
	return g
}

func (a *AlertDialog) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, _ *canvas.Interaction, _ image.Rectangle) bool {
	if p.Type != canvas.PointerPress || !a.OverlayOpen(n, rt) {
		return false
	}
	a.mu.Lock()
	geo := a.geoms[n]
	a.mu.Unlock()
	if geo == nil {
		return false
	}
	pt := image.Pt(int(p.X), int(p.Y))
	actions := parseDlgActions(n, "actions", rt)
	for i, r := range geo.actions {
		if !pt.In(r) {
			continue
		}
		idx := geo.idxs[i]
		if idx >= 0 && idx < len(actions) {
			act := actions[idx]
			if act.inv != nil {
				dispatchInvoke(act.inv, rt)
			} else if act.style == "cancel" {
				if inv := modalDismiss(n); inv != nil {
					dispatchInvoke(inv, rt)
				}
			}
		}
		return true
	}
	// Backdrop tap dismisses when a dismiss handler exists.
	if !pt.In(geo.panel) {
		if inv := modalDismiss(n); inv != nil {
			dispatchInvoke(inv, rt)
		}
		return true
	}
	return true
}

// ---- actionsheet ------------------------------------------------------------

type actionSheetGeo struct {
	panel   image.Rectangle
	actions []image.Rectangle
	cancels []image.Rectangle
}

// ActionSheet is a bottom sheet of action rows + optional cancel.
type ActionSheet struct {
	mu    sync.Mutex
	geoms map[*model.Node]*actionSheetGeo
}

func (s *ActionSheet) Measure(_ *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 320 * scale, 200 * scale
}

func (s *ActionSheet) Record(_ *canvas.LayoutNode, _ *runtime.Runtime, _ int) draw.Node {
	return nil
}

func (s *ActionSheet) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return sheetOpen(n, rt)
}

func (s *ActionSheet) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, _ image.Point) draw.Node {
	if ln == nil || !s.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	panelW := stageW - 16*scale
	if panelW > 400*scale {
		panelW = 400 * scale
	}
	actions := parseDlgActions(ln.Node, "actions", rt)
	cancels := parseDlgActions(ln.Node, "cancel", rt)
	title := formTitle(ln.Node, rt)
	rowH := 52 * scale
	titleH := 0
	if title != "" {
		titleH = 44 * scale
	}
	groupH := titleH + len(actions)*rowH
	cancelGap := 0
	cancelH := 0
	if len(cancels) > 0 {
		cancelGap = 8 * scale
		cancelH = rowH
	}
	totalH := groupH + cancelGap + cancelH + 8*scale
	panelX := (stageW - panelW) / 2
	panelY := stageH - totalH
	if panelY < 0 {
		panelY = 0
	}

	g := draw.NewGroup()
	g.Width, g.Height = float64(stageW), float64(stageH)
	g.Model = ln.Node
	g.Overlay = true
	bg := draw.NewRect()
	bg.Width, bg.Height = float64(stageW), float64(stageH)
	bg.Fill = color.RGBA{0, 0, 0, 115}
	g.AddChild(bg)

	card := draw.NewRect()
	card.X, card.Y = float64(panelX), float64(panelY)
	card.Width, card.Height = float64(panelW), float64(groupH)
	card.BorderRadius = 14 * float64(scale)
	card.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	g.AddChild(card)

	geo := &actionSheetGeo{panel: image.Rect(panelX, panelY, panelX+panelW, stageH)}
	y := panelY
	fsTitle := 13 * scale
	fsBtn := 18 * scale
	if title != "" {
		ink2 := themeColor(rt, "textSecondary", color.RGBA{60, 60, 67, 153})
		tw := int(canvas.MeasureText(title, float64(fsTitle)))
		g.AddChild(formText(title, float64(panelX+(panelW-tw)/2), float64(y+(titleH-lineHeight(fsTitle))/2), fsTitle, ink2))
		y += titleH
	}
	for i, act := range actions {
		r := image.Rect(panelX, y, panelX+panelW, y+rowH)
		geo.actions = append(geo.actions, r)
		if i > 0 || title != "" {
			sep := draw.NewRect()
			sep.NoHit = true
			sep.X, sep.Y = float64(panelX), float64(y)
			sep.Width, sep.Height = float64(panelW), float64(scale)
			sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			g.AddChild(sep)
		}
		tw := int(canvas.MeasureText(act.label, float64(fsBtn)))
		g.AddChild(formText(act.label, float64(panelX+(panelW-tw)/2), float64(y+(rowH-lineHeight(fsBtn))/2), fsBtn, actionInk(act.style, rt)))
		y += rowH
	}
	if len(cancels) > 0 {
		y += cancelGap
		c := cancels[0]
		cancelCard := draw.NewRect()
		cancelCard.X, cancelCard.Y = float64(panelX), float64(y)
		cancelCard.Width, cancelCard.Height = float64(panelW), float64(rowH)
		cancelCard.BorderRadius = 14 * float64(scale)
		cancelCard.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
		g.AddChild(cancelCard)
		r := image.Rect(panelX, y, panelX+panelW, y+rowH)
		geo.cancels = append(geo.cancels, r)
		tw := int(canvas.MeasureText(c.label, float64(fsBtn)))
		g.AddChild(formText(c.label, float64(panelX+(panelW-tw)/2), float64(y+(rowH-lineHeight(fsBtn))/2), fsBtn, formAccent(rt)))
	}
	s.mu.Lock()
	s.geoms[ln.Node] = geo
	s.mu.Unlock()
	return g
}

func (s *ActionSheet) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, _ *canvas.Interaction, _ image.Rectangle) bool {
	if p.Type != canvas.PointerPress || !s.OverlayOpen(n, rt) {
		return false
	}
	s.mu.Lock()
	geo := s.geoms[n]
	s.mu.Unlock()
	if geo == nil {
		return false
	}
	pt := image.Pt(int(p.X), int(p.Y))
	actions := parseDlgActions(n, "actions", rt)
	for i, r := range geo.actions {
		if pt.In(r) && i < len(actions) {
			if actions[i].inv != nil {
				dispatchInvoke(actions[i].inv, rt)
			}
			return true
		}
	}
	cancels := parseDlgActions(n, "cancel", rt)
	for i, r := range geo.cancels {
		if !pt.In(r) {
			continue
		}
		if i < len(cancels) && cancels[i].inv != nil {
			dispatchInvoke(cancels[i].inv, rt)
		} else if inv := sheetDismiss(n); inv != nil {
			dispatchInvoke(inv, rt)
		}
		return true
	}
	if !pt.In(geo.panel) {
		if inv := sheetDismiss(n); inv != nil {
			dispatchInvoke(inv, rt)
		}
		return true
	}
	return true
}

// ---- descriptions -----------------------------------------------------------

// Descriptions is a two-column label/value list from `items`.
type Descriptions struct{}

func (Descriptions) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	items := boundArray(n, rt, "items")
	maxL, maxV := 0, 0
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		lw := int(canvas.MeasureText(fmt.Sprint(m["label"]), float64(fs)))
		vw := int(canvas.MeasureText(fmt.Sprint(m["value"]), float64(fs)))
		if lw > maxL {
			maxL = lw
		}
		if vw > maxV {
			maxV = vw
		}
		h += lineHeight(fs) + 8*scale
	}
	w = maxL + maxV + 16*scale
	if w < 1 {
		w = 120 * scale
	}
	if h < 1 {
		h = lineHeight(fs)
	}
	return w, h
}

func (Descriptions) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	items := boundArray(ln.Node, rt, "items")
	// Label column width = max label.
	colW := 0
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		if tw := int(canvas.MeasureText(fmt.Sprint(m["label"]), float64(fs))); tw > colW {
			colW = tw
		}
	}
	colW += 16 * scale
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	ink := formInk(ln.Node, ln, rt)
	y := 0.0
	lh := float64(lineHeight(fs) + 8*scale)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		g.AddChild(formText(fmt.Sprint(m["label"]), 0, y, fs, ink2))
		g.AddChild(formText(fmt.Sprint(m["value"]), float64(colW), y, fs, ink))
		y += lh
	}
	return g
}

// ---- materialstepper --------------------------------------------------------

// MaterialStepper is a vertical step list; active step shows its child content.
type MaterialStepper struct{}

func (MaterialStepper) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	titles := stepTitles(n, rt)
	fs := formFontSize(n, scale)
	w = 280 * scale
	for i := range titles {
		h += 40 * scale
		if i == activeStep(n, rt) && i < len(n.Children) {
			if cln := canvas.Measure(n.Children[i], rt, nil, scale); cln != nil {
				h += cln.Height + 8*scale
				if cln.Width+40*scale > w {
					w = cln.Width + 40*scale
				}
			}
		}
	}
	if h < 1 {
		h = lineHeight(fs)
	}
	return w, h
}

func (MaterialStepper) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return MaterialStepper{}.record(ln, rt, scale, nil)
}

func (MaterialStepper) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return MaterialStepper{}.record(ln, rt, scale, sinks)
}

func (MaterialStepper) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	titles := stepTitles(ln.Node, rt)
	active := activeStep(ln.Node, rt)
	fs := formFontSizeLN(ln, scale)
	accent := formAccent(rt)
	doneC := color.RGBA{22, 163, 74, 255}
	sep := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	ink := formInk(ln.Node, ln, rt)
	white := color.RGBA{255, 255, 255, 255}

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	y := 0
	circleR := 13 * scale
	for i, title := range titles {
		// circle
		cx, cy := circleR, y+circleR
		c := draw.NewCircle()
		c.X, c.Y = float64(cx-circleR), float64(cy-circleR)
		c.Radius = float64(circleR)
		mark := strconv.Itoa(i + 1)
		if i < active {
			c.Fill = doneC
			mark = "✓"
		} else if i == active {
			c.Fill = accent
		} else {
			c.Fill = sep
		}
		g.AddChild(c)
		small := 12 * scale
		tw := int(canvas.MeasureText(mark, float64(small)))
		g.AddChild(formText(mark, float64(cx-tw/2), float64(cy-lineHeight(small)/2), small, white))
		// title
		weightInk := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
		if i == active {
			weightInk = ink
		}
		g.AddChild(formText(title, float64(2*circleR+12*scale), float64(y+(2*circleR-lineHeight(fs))/2), fs, weightInk))
		rowH := 2 * circleR
		contentH := 0
		if i == active && i < len(ln.Children) {
			child := ln.Children[i]
			ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
			bounds := image.Rect(2*circleR+12*scale, y+rowH+8*scale, ln.Width, y+rowH+8*scale+ch)
			if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
			contentH = ch + 8*scale
		}
		// connector
		if i < len(titles)-1 {
			line := draw.NewRect()
			line.NoHit = true
			line.X = float64(circleR - scale)
			line.Y = float64(y + 2*circleR)
			line.Width = float64(2 * scale)
			line.Height = float64(16*scale + contentH)
			line.Fill = sep
			g.AddChild(line)
		}
		y += rowH + contentH + 16*scale
	}
	ln.Children = nil
	return g
}

func stepTitles(n *model.Node, rt *runtime.Runtime) []string {
	if arr := boundArray(n, rt, "steps"); len(arr) > 0 {
		out := make([]string, 0, len(arr))
		for _, it := range arr {
			out = append(out, fmt.Sprint(it))
		}
		return out
	}
	return stringList(n.Props["steps"])
}

func activeStep(n *model.Node, rt *runtime.Runtime) int {
	raw, ok := n.Prop("active")
	if !ok {
		return 0
	}
	return int(asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt))))
}

// ---- monthview --------------------------------------------------------------

type monthGeo struct {
	cells            []image.Rectangle
	dates            []string
	prev, next       image.Rectangle
	hasPrev, hasNext bool
	year, month      int
}

// MonthView is a calendar month grid with prev/next and day selection.
type MonthView struct {
	mu    sync.Mutex
	geoms map[*model.Node]*monthGeo
}

func (m *MonthView) Measure(_ *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	// header + weekday row + 6 weeks
	return 280 * scale, (36 + 20 + 6*38) * scale
}

func (m *MonthView) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	n := ln.Node
	sel, hasSel := mvNormDay(formPropEval(n, "selected", rt))
	y, mo, ok := mvNormMonth(formPropEval(n, "month", rt))
	if !ok {
		if y, mo, ok = mvNormMonth(sel); !ok {
			y, mo = 2026, 7
		}
	}
	minD, hasMin := mvNormDay(formPropEval(n, "min", rt))
	maxD, hasMax := mvNormDay(formPropEval(n, "max", rt))
	today, hasToday := mvNormDay(formPropEval(n, "today", rt))
	start := 0
	if ws := strings.ToLower(formPropEval(n, "weekStart", rt)); ws == "monday" || ws == "1" {
		start = 1
	}
	showAdj := formPropEval(n, "showAdjacent", rt) != "false"
	head := formPropEval(n, "heading", rt)
	if head == "" {
		head = fmt.Sprintf("%s %d", mvMonthNames[mo-1], y)
	}
	labels := stringList(n.Props["weekdays"])
	if len(labels) != 7 {
		labels = mvWeekdays[:]
	}

	cellW := ln.Width / 7
	cellH := 38 * scale
	headerH := 36 * scale
	weekH := 20 * scale
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	ink := formInk(n, ln, rt)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	accent := formAccent(rt)
	fs := 14 * scale
	headFS := 15 * scale

	geo := &monthGeo{year: y, month: mo}
	// prev / title / next
	navW := 36 * scale
	geo.prev = image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+navW, ln.AbsY+headerH)
	geo.next = image.Rect(ln.AbsX+ln.Width-navW, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+headerH)
	py, pm := mvMonthAdd(y, mo, -1)
	ny, nm := mvMonthAdd(y, mo, 1)
	geo.hasPrev = !hasMin || mvFmtDate(py, pm, mvMonthDays(py, pm)) >= minD
	geo.hasNext = !hasMax || mvFmtDate(ny, nm, 1) <= maxD
	if geo.hasPrev {
		g.AddChild(formText("‹", float64(10*scale), float64((headerH-lineHeight(headFS))/2), headFS, accent))
	}
	tw := int(canvas.MeasureText(head, float64(headFS)))
	g.AddChild(formText(head, float64((ln.Width-tw)/2), float64((headerH-lineHeight(headFS))/2), headFS, ink))
	if geo.hasNext {
		g.AddChild(formText("›", float64(ln.Width-20*scale), float64((headerH-lineHeight(headFS))/2), headFS, accent))
	}

	// weekday headers
	for i := 0; i < 7; i++ {
		lbl := labels[(start+i)%7]
		lw := int(canvas.MeasureText(lbl, float64(11*scale)))
		g.AddChild(formText(lbl, float64(i*cellW+(cellW-lw)/2), float64(headerH), 11*scale, ink2))
	}

	days := mvMonthDays(y, mo)
	lead := (mvMonthWeekday(y, mo, 1) - start + 7) % 7
	total := ((lead + days + 6) / 7) * 7
	events := mvEvents(n, rt)
	for i := 0; i < total; i++ {
		off := i - lead
		cy, cm, cd, adj := y, mo, off+1, false
		switch {
		case off < 0:
			cy, cm = py, pm
			cd, adj = mvMonthDays(py, pm)+off+1, true
		case off >= days:
			cy, cm = ny, nm
			cd, adj = off-days+1, true
		}
		row, col := i/7, i%7
		x := col * cellW
		yy := headerH + weekH + row*cellH
		if adj && !showAdj {
			continue
		}
		date := mvFmtDate(cy, cm, cd)
		blocked := (hasMin && date < minD) || (hasMax && date > maxD)
		isSel := hasSel && date == sel
		cellInk := ink
		if adj {
			cellInk = ink2
		}
		if isSel {
			selR := draw.NewRect()
			selR.NoHit = true
			pad := 2 * scale
			selR.X, selR.Y = float64(x+pad), float64(yy+pad)
			selR.Width, selR.Height = float64(cellW-2*pad), float64(cellH-2*pad)
			selR.BorderRadius = 9 * float64(scale)
			selR.Fill = accent
			g.AddChild(selR)
			cellInk = themeColor(rt, "onAccent", color.RGBA{255, 255, 255, 255})
		} else if hasToday && date == today {
			ring := draw.NewRect()
			ring.NoHit = true
			pad := 2 * scale
			ring.X, ring.Y = float64(x+pad), float64(yy+pad)
			ring.Width, ring.Height = float64(cellW-2*pad), float64(cellH-2*pad)
			ring.BorderRadius = 9 * float64(scale)
			ring.Stroke = accent
			ring.StrokeWidth = 1.5 * float64(scale)
			g.AddChild(ring)
		}
		if blocked {
			cellInk = color.RGBA{cellInk.R, cellInk.G, cellInk.B, 70}
		}
		label := strconv.Itoa(cd)
		lw := int(canvas.MeasureText(label, float64(fs)))
		g.AddChild(formText(label, float64(x+(cellW-lw)/2), float64(yy+(cellH-lineHeight(fs))/2-2*scale), fs, cellInk))
		if colr, ok := events[date]; ok {
			dot := draw.NewCircle()
			dot.NoHit = true
			dr := float64(2 * scale)
			dot.X = float64(x) + float64(cellW)/2 - dr
			dot.Y = float64(yy+cellH) - float64(8*scale)
			dot.Radius = dr
			dot.Fill = colr
			g.AddChild(dot)
		}
		if !blocked {
			geo.cells = append(geo.cells, image.Rect(ln.AbsX+x, ln.AbsY+yy, ln.AbsX+x+cellW, ln.AbsY+yy+cellH))
			geo.dates = append(geo.dates, date)
		}
	}
	m.mu.Lock()
	m.geoms[n] = geo
	m.mu.Unlock()
	return g
}

func (m *MonthView) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, _ *canvas.Interaction, _ image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	m.mu.Lock()
	geo := m.geoms[n]
	m.mu.Unlock()
	if geo == nil {
		return false
	}
	pt := image.Pt(int(p.X), int(p.Y))
	if geo.hasPrev && pt.In(geo.prev) {
		py, pm := mvMonthAdd(geo.year, geo.month, -1)
		if inv := parseInvokeProp(n, "onMonthChange"); inv != nil {
			args := map[string]string{"value": fmt.Sprintf("%04d-%02d", py, pm)}
			for k, v := range inv.Args {
				args[k] = v
			}
			dispatchInvoke(&model.Invoke{Name: inv.Name, Args: args}, rt)
		}
		return true
	}
	if geo.hasNext && pt.In(geo.next) {
		ny, nm := mvMonthAdd(geo.year, geo.month, 1)
		if inv := parseInvokeProp(n, "onMonthChange"); inv != nil {
			args := map[string]string{"value": fmt.Sprintf("%04d-%02d", ny, nm)}
			for k, v := range inv.Args {
				args[k] = v
			}
			dispatchInvoke(&model.Invoke{Name: inv.Name, Args: args}, rt)
		}
		return true
	}
	for i, r := range geo.cells {
		if pt.In(r) {
			date := geo.dates[i]
			if path := formBoundPath(formPropStrRaw(n, "selected")); path != "" {
				rt.SetStatePath(path, date)
			}
			if n.OnChange != nil {
				args := map[string]string{"value": date}
				for k, v := range n.OnChange.Args {
					args[k] = v
				}
				dispatchInvoke(&model.Invoke{Name: n.OnChange.Name, Args: args}, rt)
			}
			return true
		}
	}
	return false
}

func formPropEval(n *model.Node, key string, rt *runtime.Runtime) string {
	raw, ok := n.Prop(key)
	if !ok {
		return ""
	}
	return formEvalStr(fmt.Sprint(raw), rt)
}

func formPropStrRaw(n *model.Node, key string) string {
	raw, ok := n.Prop(key)
	if !ok {
		return ""
	}
	return fmt.Sprint(raw)
}

var mvMonthNames = [12]string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}
var mvWeekdays = [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}

func mvFmtDate(y, m, d int) string { return fmt.Sprintf("%04d-%02d-%02d", y, m, d) }

func mvMonthDays(y, m int) int {
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

func mvMonthWeekday(y, m, d int) int {
	t := [12]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	if m < 3 {
		y--
	}
	w := (y + y/4 - y/100 + y/400 + t[m-1] + d) % 7
	if w < 0 {
		w += 7
	}
	return w
}

func mvMonthAdd(y, m, delta int) (int, int) {
	t := y*12 + (m - 1) + delta
	if t < 0 {
		t = 0
	}
	return t / 12, t%12 + 1
}

func mvNormDay(s string) (string, bool) {
	p := strings.Split(strings.TrimSpace(s), "-")
	if len(p) != 3 {
		return "", false
	}
	y, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	d, e3 := strconv.Atoi(p[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return "", false
	}
	if y < 1 || m < 1 || m > 12 || d < 1 || d > mvMonthDays(y, m) {
		return "", false
	}
	return mvFmtDate(y, m, d), true
}

func mvNormMonth(s string) (int, int, bool) {
	p := strings.Split(strings.TrimSpace(s), "-")
	if len(p) != 2 && len(p) != 3 {
		return 0, 0, false
	}
	y, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || y < 1 || m < 1 || m > 12 {
		return 0, 0, false
	}
	return y, m, true
}

func mvEvents(n *model.Node, rt *runtime.Runtime) map[string]color.RGBA {
	out := map[string]color.RGBA{}
	for _, it := range boundArray(n, rt, "events") {
		switch t := it.(type) {
		case string:
			if d, ok := mvNormDay(t); ok {
				out[d] = formAccent(rt)
			}
		case map[string]any:
			d, ok := mvNormDay(fmt.Sprint(t["date"]))
			if !ok {
				continue
			}
			c := formAccent(rt)
			if raw, ok := t["color"].(string); ok && raw != "" {
				c = parseHexOrTheme(raw, rt)
			}
			out[d] = c
		}
	}
	return out
}

// ---- motion -----------------------------------------------------------------

// Motion is a pass-through container; the engine's entrance system already
// reads the `animation` prop on any node (including this one).
type Motion struct{}

func (Motion) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height
		}
	}
	return w, h
}

func (Motion) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return Motion{}.record(ln, rt, scale, nil)
}

func (Motion) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return Motion{}.record(ln, rt, scale, sinks)
}

func (Motion) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	y := 0
	for _, child := range ln.Children {
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g
}

// ---- transform --------------------------------------------------------------

// Transform applies rotate/scale/translate from props onto its children group.
type Transform struct{}

func (Transform) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height
		}
	}
	return w, h
}

func (Transform) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return Transform{}.record(ln, rt, scale, nil)
}

func (Transform) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return Transform{}.record(ln, rt, scale, sinks)
}

func (Transform) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	// CSS transform-origin: center — offset so rotation/scale pivot the box.
	cx, cy := float64(ln.Width)/2, float64(ln.Height)/2
	rot := numProp(ln.Node, rt, "rotate")
	sc := numProp(ln.Node, rt, "scale")
	sx := numProp(ln.Node, rt, "scaleX")
	sy := numProp(ln.Node, rt, "scaleY")
	tx := numProp(ln.Node, rt, "translateX")
	ty := numProp(ln.Node, rt, "translateY")
	if sc != nil {
		v := *sc
		if sx == nil {
			sx = &v
		}
		if sy == nil {
			sy = &v
		}
	}
	if rot != nil {
		// graph.Rotate is radians; prop is degrees.
		g.Rotation = *rot * math.Pi / 180
	}
	g.ScaleX, g.ScaleY = 1, 1
	if sx != nil {
		g.ScaleX = *sx
	}
	if sy != nil {
		g.ScaleY = *sy
	}
	// CSS skewX/skewY (degrees) → BaseNode Skew (radians).
	if sk := numProp(ln.Node, rt, "skew"); sk != nil {
		g.SkewX = *sk * math.Pi / 180
	}
	if skx := numProp(ln.Node, rt, "skewX"); skx != nil {
		g.SkewX = *skx * math.Pi / 180
	}
	if sky := numProp(ln.Node, rt, "skewY"); sky != nil {
		g.SkewY = *sky * math.Pi / 180
	}
	// Place origin at center, apply transform, then children at -center.
	// BaseNode: Scale → Skew → Rotate → Translate — X/Y is center+translate.
	ox, oy := 0.0, 0.0
	if tx != nil {
		ox = *tx * float64(scale)
	}
	if ty != nil {
		oy = *ty * float64(scale)
	}
	g.X = cx + ox
	g.Y = cy + oy

	inner := draw.NewGroup()
	inner.X, inner.Y = -cx, -cy
	inner.Width, inner.Height = float64(ln.Width), float64(ln.Height)
	y := 0
	for _, child := range ln.Children {
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			inner.AddChild(cn)
		}
		y += ch
	}
	g.AddChild(inner)
	// Optional chrome from style is drawn by the engine around this group.
	ln.Children = nil
	return g
}

func numProp(n *model.Node, rt *runtime.Runtime, key string) *float64 {
	raw, ok := n.Prop(key)
	if !ok {
		return nil
	}
	v := asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt)))
	return &v
}

// ---- picker -----------------------------------------------------------------

type pickerGeo struct {
	box  image.Rectangle
	rowH int
	opts []formOption
}

// Picker is a vertical option wheel; tapping a row selects it.
type Picker struct {
	mu    sync.Mutex
	local map[*model.Node]string
	geoms map[*model.Node]*pickerGeo
}

func (p *Picker) Measure(_ *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 200 * scale, 180 * scale
}

func (p *Picker) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	opts := formOptions(ln.Node.Props["options"])
	if len(opts) == 0 {
		opts = formOptions(boundArray(ln.Node, rt, "options"))
	}
	rowH := 36 * scale
	cur := p.value(ln.Node, rt)
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Clip = true
	// center band
	band := draw.NewRect()
	band.NoHit = true
	band.Y = float64((ln.Height - rowH) / 2)
	band.Width, band.Height = float64(ln.Width), float64(rowH)
	band.BorderRadius = 8 * float64(scale)
	band.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	g.AddChild(band)

	// Find selected index; scroll so it sits in the center.
	sel := 0
	for i, o := range opts {
		if o.value == cur {
			sel = i
			break
		}
	}
	// Draw a window of options around the selection.
	midY := (ln.Height - rowH) / 2
	fs := 18 * scale
	ink := formInk(ln.Node, ln, rt)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	for i, o := range opts {
		y := midY + (i-sel)*rowH
		if y+rowH < 0 || y > ln.Height {
			continue
		}
		lbl := o.label
		if lbl == "" {
			lbl = o.value
		}
		c := ink2
		if i == sel {
			c = ink
		}
		tw := int(canvas.MeasureText(lbl, float64(fs)))
		g.AddChild(formText(lbl, float64((ln.Width-tw)/2), float64(y+(rowH-lineHeight(fs))/2), fs, c))
	}
	p.mu.Lock()
	p.geoms[ln.Node] = &pickerGeo{
		box:  image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height),
		rowH: rowH,
		opts: opts,
	}
	p.mu.Unlock()
	return g
}

func (p *Picker) value(n *model.Node, rt *runtime.Runtime) string {
	if path := formBoundPath(n.Value); path != "" {
		return formEvalStr(n.Value, rt)
	}
	p.mu.Lock()
	v := p.local[n]
	p.mu.Unlock()
	if v != "" {
		return v
	}
	return formEvalStr(n.Value, rt)
}

func (p *Picker) HandlePointer(n *model.Node, rt *runtime.Runtime, pi canvas.PointerInput, _ *canvas.Interaction, frame image.Rectangle) bool {
	if pi.Type != canvas.PointerPress || formDisabled(n, rt) {
		return false
	}
	p.mu.Lock()
	geo := p.geoms[n]
	p.mu.Unlock()
	if geo == nil || len(geo.opts) == 0 {
		return false
	}
	box := geo.box
	if box.Empty() {
		box = frame
	}
	relY := int(pi.Y) - box.Min.Y
	mid := box.Dy() / 2
	// delta rows from center
	delta := (relY - mid) / max(geo.rowH, 1)
	if relY-mid < 0 && (relY-mid)%max(geo.rowH, 1) != 0 {
		delta-- // floor for negatives
	}
	cur := p.value(n, rt)
	sel := 0
	for i, o := range geo.opts {
		if o.value == cur {
			sel = i
			break
		}
	}
	next := sel + delta
	if next < 0 {
		next = 0
	}
	if next >= len(geo.opts) {
		next = len(geo.opts) - 1
	}
	val := geo.opts[next].value
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, val)
	} else {
		p.mu.Lock()
		p.local[n] = val
		p.mu.Unlock()
	}
	commitFormChange(n, rt, val)
	return true
}

// ---- rating -----------------------------------------------------------------

type ratingGeo struct {
	box  image.Rectangle
	size int
	max  int
}

// Rating draws filled/empty stars (or dots) for value/max; tap sets value.
type Rating struct {
	mu    sync.Mutex
	local map[*model.Node]int
	geoms map[*model.Node]*ratingGeo
}

func (r *Rating) Measure(n *model.Node, _ *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	maxV := 5
	if v, ok := n.Prop("max"); ok {
		if m := int(formFloat(v)); m > 0 {
			maxV = m
		}
	}
	sz := 18
	if v, ok := n.Prop("size"); ok {
		if s := int(formFloat(v)); s > 0 {
			sz = s
		}
	}
	return maxV * (sz + 2) * scale, (sz + 4) * scale
}

func (r *Rating) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	val := r.value(ln.Node, rt)
	maxV := 5
	if v, ok := ln.Node.Prop("max"); ok {
		if m := int(formFloat(v)); m > 0 {
			maxV = m
		}
	}
	sz := 18 * scale
	if v, ok := ln.Node.Prop("size"); ok {
		if s := int(formFloat(v)); s > 0 {
			sz = s * scale
		}
	}
	r.mu.Lock()
	r.geoms[ln.Node] = &ratingGeo{
		box:  image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height),
		size: sz + 2*scale,
		max:  maxV,
	}
	r.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	fill := color.RGBA{245, 158, 11, 255}
	empty := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	starRune, hasStar := canvas.LookupIconRune("star")
	for i := 1; i <= maxV; i++ {
		x := float64((i - 1) * (sz + 2*scale))
		c := empty
		if i <= val {
			c = fill
		}
		if hasStar {
			t := draw.NewText()
			t.X, t.Y = x, 0
			t.Content = string(starRune)
			t.FontSize = float64(sz)
			t.Fill = c
			g.AddChild(t)
		} else {
			dot := draw.NewCircle()
			d := float64(sz) * 0.7
			dot.X = x + (float64(sz)-d)/2
			dot.Y = (float64(ln.Height) - d) / 2
			dot.Radius = d / 2
			if i <= val {
				dot.Fill = fill
			} else {
				dot.Stroke = empty
				dot.StrokeWidth = float64(scale)
			}
			g.AddChild(dot)
		}
	}
	return g
}

func (r *Rating) value(n *model.Node, rt *runtime.Runtime) int {
	if raw, ok := n.Prop("value"); ok {
		return int(asFloat64(runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt))))
	}
	r.mu.Lock()
	v := r.local[n]
	r.mu.Unlock()
	return v
}

func (r *Rating) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, _ *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || formDisabled(n, rt) {
		return false
	}
	r.mu.Lock()
	geo := r.geoms[n]
	r.mu.Unlock()
	if geo == nil || geo.size < 1 {
		return false
	}
	box := geo.box
	if box.Empty() {
		box = frame
	}
	idx := (int(p.X)-box.Min.X)/geo.size + 1
	if idx < 1 {
		idx = 1
	}
	if idx > geo.max {
		idx = geo.max
	}
	if path := formBoundPath(formPropStrRaw(n, "value")); path != "" {
		rt.SetStatePath(path, idx)
	} else {
		r.mu.Lock()
		r.local[n] = idx
		r.mu.Unlock()
	}
	commitFormChange(n, rt, idx)
	return true
}

// ---- refreshindicator -------------------------------------------------------

// RefreshIndicator wraps children; a downward drag past threshold fires onRefresh.
type RefreshIndicator struct {
	mu     sync.Mutex
	dragY  map[*model.Node]float64
	startY map[*model.Node]float64
}

func (r *RefreshIndicator) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height
		}
	}
	if h < 1 {
		h = 80 * scale
	}
	return w, h
}

func (r *RefreshIndicator) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return r.record(ln, rt, scale, nil)
}

func (r *RefreshIndicator) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return r.record(ln, rt, scale, sinks)
}

func (r *RefreshIndicator) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Clip = true
	r.mu.Lock()
	dy := r.dragY[ln.Node]
	r.mu.Unlock()
	if dy > 0 {
		// pull affordance
		spinH := int(math.Min(dy, float64(40*scale)))
		if spinH > 0 {
			spin := draw.NewRect()
			spin.NoHit = true
			spin.Width = float64(ln.Width)
			spin.Height = float64(spinH)
			spin.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 80})
			g.AddChild(spin)
			// simple spinner dots
			fs := 12 * scale
			lbl := "…"
			tw := int(canvas.MeasureText(lbl, float64(fs)))
			g.AddChild(formText(lbl, float64((ln.Width-tw)/2), float64((spinH-lineHeight(fs))/2), fs, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		}
	}
	y := int(math.Min(dy, float64(40*scale)))
	for _, child := range ln.Children {
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g
}

func (r *RefreshIndicator) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	const threshold = 56.0
	switch p.Type {
	case canvas.PointerPress:
		inter.Pressed = n
		r.mu.Lock()
		r.startY[n] = p.Y
		r.dragY[n] = 0
		r.mu.Unlock()
		return true
	case canvas.PointerMove:
		if inter.Pressed != n {
			return false
		}
		r.mu.Lock()
		dy := p.Y - r.startY[n]
		if dy < 0 {
			dy = 0
		}
		r.dragY[n] = dy
		r.mu.Unlock()
		return true
	case canvas.PointerRelease:
		if inter.Pressed != n {
			return false
		}
		r.mu.Lock()
		dy := r.dragY[n]
		r.dragY[n] = 0
		delete(r.startY, n)
		r.mu.Unlock()
		if dy >= threshold {
			if inv := parseInvokeProp(n, "onRefresh"); inv != nil {
				dispatchInvoke(inv, rt)
			} else if n.OnPress != nil {
				dispatchInvoke(n.OnPress, rt)
			}
		}
		return true
	}
	return false
}

// ---- selectabletext ---------------------------------------------------------

// SelectableText is Flutter's SelectableText: drag/shift/Cmd+A select, Cmd+C
// copies via the engine clipboard seam. Shares InputState with inputs (read-
// only: mutations are blocked in handleEditKey).
type SelectableText struct {
	mu     sync.Mutex
	inters map[*model.Node]*canvas.Interaction
}

func (s *SelectableText) cacheInter(n *model.Node, inter *canvas.Interaction) {
	if s.inters == nil {
		s.inters = map[*model.Node]*canvas.Interaction{}
	}
	if inter == nil {
		return
	}
	s.mu.Lock()
	s.inters[n] = inter
	s.mu.Unlock()
}

func (s *SelectableText) sessionFor(n *model.Node) *canvas.InputState {
	s.mu.Lock()
	inter := s.inters[n]
	s.mu.Unlock()
	if inter == nil || inter.Input == nil || inter.Input.Node != n {
		return nil
	}
	return inter.Input
}

func (SelectableText) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	txt := formEvalStr(n.Text, rt)
	w = int(canvas.MeasureText(txt, float64(fs)))
	if w < 1 {
		w = scale
	}
	h = lineHeight(fs)
	return w, h
}

func (s *SelectableText) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	txt := formEvalStr(ln.Node.Text, rt)
	if sess := s.sessionFor(ln.Node); sess != nil {
		txt = string(sess.Runes)
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	// Selection highlight behind the text (theme selection color).
	if sess := s.sessionFor(ln.Node); sess != nil && sess.SelStart < sess.SelEnd {
		x0 := int(canvas.MeasureText(string(sess.Runes[:sess.SelStart]), float64(fs)))
		x1 := int(canvas.MeasureText(string(sess.Runes[:sess.SelEnd]), float64(fs)))
		hi := draw.NewRect()
		hi.NoHit = true
		hi.X = float64(x0)
		hi.Y = 0
		hi.Width = float64(x1 - x0)
		hi.Height = float64(lineHeight(fs))
		hi.Fill = themeColor(rt, "selection", color.RGBA{0, 122, 255, 77})
		if c := themeColor(rt, "accent", color.RGBA{0, 122, 255, 255}); c.A > 0 {
			hi.Fill = color.RGBA{c.R, c.G, c.B, 77}
		}
		g.AddChild(hi)
	}
	g.AddChild(formText(txt, 0, 0, fs, ink))
	return g
}

func (s *SelectableText) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	s.cacheInter(n, inter)
	if p.Type == canvas.PointerPress {
		inter.Focused = n
		inter.FocusVisible = false
		// Let the engine open the session and map the caret on the next frame;
		// returning true captures the press for drag-selection (engine path).
		return true
	}
	return false
}

func (s *SelectableText) OnFocused(n *model.Node, inter *canvas.Interaction) {
	s.cacheInter(n, inter)
}
