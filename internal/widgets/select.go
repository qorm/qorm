package widgets

// The select (HTML: render_input.go:294 — the native <select> / dropdown):
// a current-value box with a dropdown indicator and a floating option list
// over the options prop. Clicking opens the list; clicking an option writes
// the value back and dispatches onChange.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	w := &Select{
		local:    map[*model.Node]string{},
		open:     map[*model.Node]bool{},
		geoms:    map[*model.Node]selectGeom{},
		hoverRow: map[*model.Node]int{},
	}
	canvas.RegisterWidget("select", w)
	canvas.RegisterWidget("dropdown", w)
	// Flutter DropdownButton: same control with a Material-flavoured name.
	canvas.RegisterWidget("dropdownbutton", w)
}

// Select is the single-value picker box: the selected option's label (or the
// raw value, or — while empty — the first option, the browser's default
// display), a chevron indicator at the right edge, and click-to-expand
// selection.
type Select struct {
	mu sync.Mutex
	// local holds UNBOUND selects' values (see Checkbox.local).
	local map[*model.Node]string
	// open marks the node whose dropdown menu is currently expanded.
	open map[*model.Node]bool
	// geoms is the last laid-out geometry per node (absolute physical px),
	// stashed every Record so HandlePointer can map a press to a row index.
	geoms map[*model.Node]selectGeom
	// hoverRow is the menu row under the pointer (-1 = none), tracked from
	// PointerMove events so the open menu highlights like a native one.
	hoverRow map[*model.Node]int
}

const (
	selectPadX    = 12
	selectPadY    = 8
	selectMinW    = 120
	selectGlyph   = 18 // indicator column width
	selectMenuGap = 4
	selectMenuPad = 4
)

type selectGeom struct {
	box     image.Rectangle
	rowH    int
	menuGap int
	menuPad int
	// panel is the open menu's exact rect, written by OverlayRecord every
	// frame the menu is up — optionIndexAt reads it so hit-testing always
	// matches the drawn placement (below the box, or flipped above it).
	panel image.Rectangle
}

// Measure sizes the box to the widest option label plus padding and the
// indicator column (min 120px wide, one text line plus padding tall).
func (s *Select) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	boxH := selectBoxHeight(n, scale)
	lw := 0
	opts := formOptions(n.Props["options"])
	for _, o := range opts {
		if w := int(canvas.MeasureText(o.label, float64(fs))); w > lw {
			lw = w
		}
	}
	if w := int(canvas.MeasureText(s.display(n, nil, rt), float64(fs))); w > lw {
		lw = w
	}
	w = lw + (2*selectPadX+selectGlyph)*scale
	if min := selectMinW * scale; w < min {
		w = min
	}
	return w, boxH
}

// Record draws the input-style chrome, the current label and the indicator.
func (s *Select) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	boxH := selectBoxHeight(ln.Node, scale)
	// The laid-out box wins over the content height: an author height (scene
	// style height:40 > content ~33) makes the generic bg rect TALLER than
	// the chrome, and the bg's bottom border peeks out below the box (the
	// "extra strip under the select"). Chrome, text, chevron and the hit
	// geometry all key off the laid-out height.
	if ln.Height > 0 {
		boxH = ln.Height
	}

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	chrome := draw.NewRect()
	chrome.Width = float64(ln.Width)
	chrome.Height = float64(boxH)
	// The generic pass already paints the node's bg rect from the resolved
	// style (performLayout's hasBg/hasStroke path); the chrome must match it
	// EXACTLY or the two borders peek past each other (the "double edge"
	// under the box when an author sets borderRadius/borderWidth). Author
	// values win; the widget defaults only fill what the author left unset.
	chrome.BorderRadius = 10 * float64(scale)
	if ln.Style.BorderRadius > 0 {
		chrome.BorderRadius = float64(ln.Style.BorderRadius)
	}
	// Browser parity, same rule as style.go's input background: a bare
	// select is white (the native picker chrome). Any resolved background
	// wins — the author's, and also the interactive overlay's hover/press
	// feedback (applyInteractiveOverlay rewrites s.Background when the theme
	// component defines hoveredBackgroundColor/pressedBackgroundColor).
	if ln.Style.Background.A > 0 {
		chrome.Fill = ln.Style.Background
	} else {
		chrome.Fill = color.RGBA{255, 255, 255, 255}
	}
	chrome.Stroke = themeColor(rt, "inputBorder", color.RGBA{198, 198, 200, 255})
	chrome.StrokeWidth = float64(scale)
	if ln.Style.StrokeWidth > 0 {
		chrome.Stroke = ln.Style.StrokeColor
		chrome.StrokeWidth = ln.Style.StrokeWidth
	}
	g.AddChild(chrome)

	fs := formFontSizeLN(ln, scale)
	g.AddChild(formText(s.display(ln.Node, ln, rt), float64(selectPadX*scale),
		(float64(boxH)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))

	// The dropdown indicator uses the shared SVG chevron so it stays smooth
	// instead of degenerating into three stepped bars.
	ink := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	size := 12 * scale
	if size > boxH-4*scale {
		size = boxH - 4*scale
	}
	if size < scale {
		size = scale
	}
	cx := float64(ln.Width - (selectPadX+selectGlyph/2)*scale)
	cy := float64(boxH) / 2
	chev := draw.NewImage()
	chev.NoHit = true
	chev.Bitmap = rasterIcon(iconSet["chevron-down"], size, size, ink)
	chev.Width = float64(size)
	chev.Height = float64(size)
	chev.X = cx - float64(size)/2
	chev.Y = cy - float64(size)/2
	chev.Fit = "fill"
	g.AddChild(chev)

	s.mu.Lock()
	s.geoms[ln.Node] = selectGeom{
		box:     image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+boxH),
		rowH:    selectRowHeight(ln.Node, scale),
		menuGap: selectMenuGap * scale,
		menuPad: selectMenuPad * scale,
	}
	s.mu.Unlock()
	return g
}

// OverlayOpen reports whether the popup should be mounted above the scene.
func (s *Select) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	if formDisabled(n, rt) {
		return false
	}
	return s.isOpen(n) && len(formOptions(n.Props["options"])) > 0
}

// OverlayRecord draws the floating option list over the select box.
func (s *Select) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !s.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	opts := formOptions(ln.Node.Props["options"])
	if len(opts) == 0 {
		return nil
	}

	boxH := selectBoxHeight(ln.Node, scale)
	if ln.Height > 0 {
		// Same rule as Record: the panel hangs off the LAID-OUT box, not the
		// content height, or it floats over the taller author-sized box.
		boxH = ln.Height
	}
	rowH := selectRowHeight(ln.Node, scale)
	menuGap := selectMenuGap * scale
	menuPad := selectMenuPad * scale
	cur := s.value(ln.Node, ln, rt)

	boxX := ln.AbsX
	boxY := ln.AbsY
	panelX := boxX
	panelW := ln.Width
	panelH := selectMenuHeight(ln.Node, opts, scale)

	// Placement: below the box, flipping ABOVE it when the menu would leave
	// the viewport (the old unconditional-below painted the lower rows
	// off-window — invisible and unclickable). geoms.panel carries the exact
	// rect so optionIndexAt maps rows in the same place the panel was drawn.
	stageW, stageH := overlayStageSize(rt, scale, panelX+panelW, 0)
	panelY := boxY + boxH + menuGap
	if panelY+panelH > stageH && boxY-menuGap-panelH >= 0 {
		panelY = boxY - menuGap - panelH
	}
	if panelY+panelH > stageH {
		stageH = panelY + panelH
	}
	s.mu.Lock()
	geo := s.geoms[ln.Node]
	geo.rowH = rowH
	geo.menuGap = menuGap
	geo.menuPad = menuPad
	geo.panel = image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)
	s.geoms[ln.Node] = geo
	s.mu.Unlock()

	g := draw.NewGroup()
	g.Width = float64(stageW)
	g.Height = float64(stageH)
	g.Model = ln.Node
	g.Overlay = true

	backdrop := draw.NewRect()
	backdrop.Width = float64(stageW)
	backdrop.Height = float64(stageH)
	g.AddChild(backdrop)

	panel := draw.NewRect()
	panel.X = float64(panelX)
	panel.Y = float64(panelY)
	panel.Width = float64(panelW)
	panel.Height = float64(panelH)
	panel.BorderRadius = 10 * float64(scale)
	panel.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panel.Fill.A = 255 // modal menu: a frosted fill without a real backdrop-blur pass just ghosts the content beneath through it
	panel.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	panel.StrokeWidth = float64(scale)
	panel.ShadowColor = color.RGBA{0, 0, 0, 32}
	panel.ShadowBlur = 14 * float64(scale)
	panel.ShadowY = 4 * float64(scale)
	g.AddChild(panel)

	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	hover := s.hoverRowOf(ln.Node)
	for i, o := range opts {
		y := float64(panelY + menuPad + i*rowH)
		hovered := i == hover
		if hovered || o.value == cur {
			sel := draw.NewRect()
			sel.NoHit = true
			sel.X = float64(panelX + 6*scale)
			sel.Y = y
			sel.Width = float64(panelW - 12*scale)
			sel.Height = float64(rowH)
			sel.BorderRadius = 6 * float64(scale)
			sel.Fill = color.RGBA{0, 122, 255, 18}
			if hovered {
				// Native-menu hover: solid accent row, white label.
				sel.Fill = themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
			}
			g.AddChild(sel)
		}
		rowInk := ink
		if hovered {
			rowInk = color.RGBA{255, 255, 255, 255}
		}
		txt := formText(o.label, float64(panelX+selectPadX*scale),
			y+(float64(rowH)-float64(lineHeight(fs)))/2,
			fs, rowInk)
		if o.value == cur {
			txt.FontWeight = 600
		}
		g.AddChild(txt)
	}

	return g
}

// HandlePointer opens the menu on press, or selects one option row while open
// and closes the menu again.
func (s *Select) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	opts := formOptions(n.Props["options"])
	if len(opts) == 0 {
		return false
	}
	// Pointer motion over the open menu tracks the hovered row (native-menu
	// highlight); motion over the closed box costs no redraw.
	if p.Type == canvas.PointerMove && s.isOpen(n) {
		if geo, ok := s.geometry(n); ok {
			idx := s.optionIndexAt(geo, opts, p.X, p.Y)
			s.mu.Lock()
			changed := s.hoverRow[n] != idx
			s.hoverRow[n] = idx
			s.mu.Unlock()
			return changed
		}
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	if s.isOpen(n) {
		if geo, ok := s.geometry(n); ok {
			if idx := s.optionIndexAt(geo, opts, p.X, p.Y); idx >= 0 && idx < len(opts) {
				next := opts[idx].value
				if next != s.value(n, nil, rt) {
					s.setValue(n, rt, next)
					commitFormChange(n, rt, next)
				}
			}
		}
		s.setOpen(n, false)
		return true
	}
	s.setOpen(n, true)
	return true
}

// value resolves the current value: the binding, else the uncontrolled store,
// else the literal value (may be empty — the display falls back then).
func (s *Select) value(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	if formBoundPath(n.Value) != "" {
		return fmt.Sprint(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
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
func (s *Select) display(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	cur := s.value(n, ln, rt)
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

func selectBoxHeight(n *model.Node, scale int) int {
	return lineHeight(formFontSize(n, scale)) + 2*selectPadY*scale
}

func selectRowHeight(n *model.Node, scale int) int {
	return selectBoxHeight(n, scale)
}

func selectMenuHeight(n *model.Node, opts []formOption, scale int) int {
	if len(opts) == 0 {
		return 0
	}
	return selectMenuPad*2*scale + len(opts)*selectRowHeight(n, scale)
}

func overlayStageSize(rt *runtime.Runtime, scale, minW, minH int) (w, h int) {
	w, h = minW, minH
	if rt != nil && rt.Viewport.W > 0 && rt.Viewport.H > 0 {
		// rt.Viewport is already in physical px on the canvas engine (the
		// layout pass feeds it the surface size) — scaling it again doubled
		// the backdrop/stage past the window.
		if vw := rt.Viewport.W; vw > w {
			w = vw
		}
		if vh := rt.Viewport.H; vh > h {
			h = vh
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return
}

func (s *Select) geometry(n *model.Node) (selectGeom, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	geo, ok := s.geoms[n]
	return geo, ok
}

func (s *Select) isOpen(n *model.Node) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open[n]
}

func (s *Select) setOpen(n *model.Node, open bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if open {
		for k := range s.open {
			delete(s.open, k)
		}
		s.open[n] = true
		s.hoverRow[n] = -1
		return
	}
	delete(s.open, n)
	delete(s.hoverRow, n)
}

// hoverRowOf reads the pointer-hovered menu row (-1 = none).
func (s *Select) hoverRowOf(n *model.Node) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.hoverRow[n]; ok {
		return v
	}
	return -1
}

func (s *Select) setValue(n *model.Node, rt *runtime.Runtime, next string) {
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, next)
		return
	}
	s.mu.Lock()
	s.local[n] = next
	s.mu.Unlock()
}

func (s *Select) optionIndexAt(geo selectGeom, opts []formOption, x, y float64) int {
	if len(opts) == 0 || geo.panel.Empty() {
		return -1
	}
	if x < float64(geo.panel.Min.X) || x > float64(geo.panel.Max.X) {
		return -1
	}
	yy := y - float64(geo.panel.Min.Y) - float64(geo.menuPad)
	if yy < 0 {
		return -1
	}
	idx := int(yy / float64(geo.rowH))
	if idx < 0 || idx >= len(opts) {
		return -1
	}
	return idx
}
