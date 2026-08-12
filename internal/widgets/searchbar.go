package widgets

// SearchBar: single-line search field with a floating filtered-results panel
// (HTML searchbar). Typing uses the shared canvas edit session once
// editableType("searchbar") is true; picking a row fills the value and
// dispatches onSelect with {label}.

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("searchbar", &SearchBar{
		open:     map[*model.Node]bool{},
		geoms:    map[*model.Node]searchGeom{},
		hoverRow: map[*model.Node]int{},
		inters:   map[*model.Node]*canvas.Interaction{},
	})
}

// SearchBar is the search field + results panel.
type SearchBar struct {
	mu       sync.Mutex
	open     map[*model.Node]bool
	geoms    map[*model.Node]searchGeom
	hoverRow map[*model.Node]int
	inters   map[*model.Node]*canvas.Interaction
}

type searchGeom struct {
	box   image.Rectangle
	rowH  int
	panel image.Rectangle
	items []searchItem
}

type searchItem struct {
	label, detail string
}

const (
	searchMinW    = 220
	searchH       = 40
	searchPad     = 12
	searchRowH    = 36
	searchMenuPad = 4
	searchMaxRows = 6
)

func (s *SearchBar) sessionFor(n *model.Node) *canvas.InputState {
	s.mu.Lock()
	inter := s.inters[n]
	s.mu.Unlock()
	if inter == nil || inter.Input == nil || inter.Input.Node != n {
		return nil
	}
	return inter.Input
}

func (s *SearchBar) cacheInter(n *model.Node, inter *canvas.Interaction) {
	if inter == nil {
		return
	}
	s.mu.Lock()
	s.inters[n] = inter
	s.mu.Unlock()
}

func (s *SearchBar) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w = searchMinW * scale
	if n != nil && n.Style != nil {
		if f, ok := n.Style["width"].(float64); ok && f > 0 {
			w = int(f) * scale
		}
	}
	return w, searchH * scale
}

func (s *SearchBar) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	s.mu.Lock()
	s.geoms[ln.Node] = searchGeom{box: image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)}
	s.mu.Unlock()

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
	pad := float64(searchPad * scale)
	text, placeholder := s.display(ln.Node, rt)
	ink := formInk(ln.Node, ln, rt)
	if placeholder {
		ink = themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	}
	if text != "" {
		g.AddChild(formText(text, pad, (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, ink))
	}

	// Caret while editing (blink half-period matches canvas input).
	if sess := s.sessionFor(ln.Node); sess != nil {
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

func (s *SearchBar) display(n *model.Node, rt *runtime.Runtime) (text string, placeholder bool) {
	if sess := s.sessionFor(n); sess != nil {
		return string(sess.Runes), len(sess.Runes) == 0
	}
	v := formEvalStr(n.Value, rt)
	if v != "" {
		return v, false
	}
	hint := formPropStr(n, "hint", rt)
	if hint == "" {
		hint = n.Placeholder
	}
	return hint, true
}

func (s *SearchBar) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	if formDisabled(n, rt) {
		return false
	}
	s.mu.Lock()
	open := s.open[n]
	s.mu.Unlock()
	if !open {
		return false
	}
	return len(s.filtered(n, rt)) > 0
}

func (s *SearchBar) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !s.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	items := s.filtered(ln.Node, rt)
	if len(items) == 0 {
		return nil
	}
	if len(items) > searchMaxRows {
		items = items[:searchMaxRows]
	}
	rowH := searchRowH * scale
	menuPad := searchMenuPad * scale
	panelH := 2*menuPad + len(items)*rowH
	panelW := ln.Width
	panelX := ln.AbsX
	panelY := ln.AbsY + ln.Height + 4*scale
	stageW := panelX + panelW + 8
	stageH := panelY + panelH + 8
	if stageW < 1 {
		stageW = panelW
	}

	s.mu.Lock()
	geo := s.geoms[ln.Node]
	geo.rowH = rowH
	geo.panel = image.Rect(panelX, panelY, panelX+panelW, panelY+panelH)
	geo.items = items
	s.geoms[ln.Node] = geo
	hover := s.hoverRow[ln.Node]
	s.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(stageW), float64(stageH)
	g.Model = ln.Node
	g.Overlay = true

	// Transparent backdrop to catch outside clicks (handled by press outside panel).
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
	panel.ShadowBlur = 14 * float64(scale)
	panel.ShadowY = 4 * float64(scale)
	g.AddChild(panel)

	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	for i, it := range items {
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
		g.AddChild(formText(it.label, float64(panelX+menuPad+8*scale), y+float64(8*scale), fs, ink))
		if it.detail != "" {
			g.AddChild(formText(it.detail, float64(panelX+panelW-menuPad-8*scale)-float64(int(canvas.MeasureText(it.detail, float64(fs-2*scale)))), y+float64(10*scale), fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		}
	}
	return g
}

func (s *SearchBar) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	s.cacheInter(n, inter)

	s.mu.Lock()
	geo := s.geoms[n]
	s.mu.Unlock()

	// Press on a result row.
	if p.Type == canvas.PointerPress && !geo.panel.Empty() && image.Pt(int(p.X), int(p.Y)).In(geo.panel) {
		idx := (int(p.Y) - geo.panel.Min.Y - searchMenuPad) / max(geo.rowH, 1)
		items := s.filtered(n, rt)
		if idx >= 0 && idx < len(items) && idx < searchMaxRows {
			label := items[idx].label
			if path := formBoundPath(n.Value); path != "" {
				rt.SetStatePath(path, label)
			}
			// Close panel and blur selection UX.
			s.mu.Lock()
			s.open[n] = false
			s.mu.Unlock()
			if inv := parseInvokeProp(n, "onSelect"); inv != nil {
				args := map[string]string{"label": label}
				for k, v := range inv.Args {
					args[k] = v
				}
				dispatchInvoke(&model.Invoke{Name: inv.Name, Args: args}, rt)
			}
			return true
		}
	}

	if p.Type == canvas.PointerMove && !geo.panel.Empty() && image.Pt(int(p.X), int(p.Y)).In(geo.panel) {
		idx := (int(p.Y) - geo.panel.Min.Y - searchMenuPad) / max(geo.rowH, 1)
		s.mu.Lock()
		s.hoverRow[n] = idx
		s.mu.Unlock()
		return true
	}

	if p.Type == canvas.PointerPress {
		// Focus the field so the edit session opens (editableType includes searchbar).
		inter.Focused = n
		inter.FocusVisible = false
		s.mu.Lock()
		s.open[n] = true
		s.mu.Unlock()
		return true
	}
	return false
}

func (s *SearchBar) OnFocused(n *model.Node, inter *canvas.Interaction) {
	s.cacheInter(n, inter)
	s.mu.Lock()
	s.open[n] = true
	s.mu.Unlock()
}

func (s *SearchBar) filtered(n *model.Node, rt *runtime.Runtime) []searchItem {
	q, _ := s.display(n, rt)
	q = strings.ToLower(strings.TrimSpace(q))
	// While placeholder is showing, treat query as empty → no panel.
	if sess := s.sessionFor(n); sess == nil {
		v := formEvalStr(n.Value, rt)
		if v == "" {
			return nil
		}
		q = strings.ToLower(strings.TrimSpace(v))
	} else if len(sess.Runes) == 0 {
		return nil
	} else {
		q = strings.ToLower(string(sess.Runes))
	}
	if q == "" {
		return nil
	}
	var out []searchItem
	for _, it := range s.items(n, rt) {
		if strings.Contains(strings.ToLower(it.label), q) {
			out = append(out, it)
		}
	}
	return out
}

func (s *SearchBar) items(n *model.Node, rt *runtime.Runtime) []searchItem {
	raw, ok := n.Prop("items")
	if !ok {
		return nil
	}
	val := runtime.EvalBinding(fmt.Sprint(raw), formCtx(rt))
	// EvalBinding on non-template returns raw; also accept direct array props.
	if arr, ok := raw.([]any); ok && !strings.Contains(fmt.Sprint(raw), "{{") {
		val = arr
	}
	if s, ok := raw.(string); ok && strings.Contains(s, "{{") {
		val = runtime.EvalBinding(s, formCtx(rt))
	}
	arr, _ := val.([]any)
	var out []searchItem
	for _, it := range arr {
		switch t := it.(type) {
		case string:
			out = append(out, searchItem{label: t})
		case map[string]any:
			label := fmt.Sprint(t["label"])
			if label == "" || label == "<nil>" {
				label = fmt.Sprint(t)
			}
			detail := ""
			if d, ok := t["detail"]; ok {
				detail = fmt.Sprint(d)
			}
			out = append(out, searchItem{label: label, detail: detail})
		default:
			out = append(out, searchItem{label: fmt.Sprint(it)})
		}
	}
	return out
}

// parseInvokeProp is defined in gesturedetector helpers — use local copy if missing.
func parseInvokeProp(n *model.Node, key string) *model.Invoke {
	if n == nil {
		return nil
	}
	raw, ok := n.Prop(key)
	if !ok || raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case string:
		if t == "" {
			return nil
		}
		return &model.Invoke{Name: t}
	case map[string]any:
		name, _ := t["name"].(string)
		if name == "" {
			name, _ = t["action"].(string)
		}
		if name == "" {
			return nil
		}
		args := map[string]string{}
		if raw, ok := t["args"].(map[string]any); ok {
			for k, v := range raw {
				args[k] = fmt.Sprint(v)
			}
		}
		return &model.Invoke{Name: name, Args: args}
	default:
		return nil
	}
}
