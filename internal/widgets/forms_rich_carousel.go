package widgets

// Canvas ports: field, textformfield, richtext, carousel.

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("field", Field{})
	canvas.RegisterWidget("formfield", Field{})
	canvas.RegisterWidget("textformfield", &TextFormField{inters: map[*model.Node]*canvas.Interaction{}})
	canvas.RegisterWidget("richtext", RichText{})
	canvas.RegisterWidget("carousel", &Carousel{index: map[*model.Node]int{}, geoms: map[*model.Node]image.Rectangle{}})
}

// ---- field ------------------------------------------------------------------

// Field is a labelled column: optional label*, children, then error or help.
type Field struct{}

func (Field) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	w = 200 * scale
	h = 0
	if formPropStr(n, "label", rt) != "" {
		h += lineHeight(fs-2*scale) + 5*scale
	}
	for _, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			h += cln.Height + 5*scale
		}
	}
	if formPropStr(n, "error", rt) != "" || formPropStr(n, "help", rt) != "" {
		h += lineHeight(fs - 2*scale)
	}
	if h < 1 {
		h = scale
	}
	return w, h
}

func (f Field) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return f.record(ln, rt, scale, nil)
}

func (f Field) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return f.record(ln, rt, scale, sinks)
}

func (Field) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	fs := formFontSizeLN(ln, scale)
	y := 0.0
	if label := formPropStr(ln.Node, "label", rt); label != "" {
		if formPropStr(ln.Node, "required", rt) == "true" || propTruthy(ln.Node, "required") {
			label += " *"
		}
		g.AddChild(formText(label, 0, y, fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		y += float64(lineHeight(fs-2*scale) + 5*scale)
	}
	for _, child := range ln.Children {
		cw := child.Width + child.Style.MarginLeft + child.Style.MarginRight
		ch := child.Height + child.Style.MarginTop + child.Style.MarginBot
		bounds := image.Rect(0, int(y), cw, int(y)+ch)
		if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += float64(ch + 5*scale)
	}
	ln.Children = nil
	if err := formPropStr(ln.Node, "error", rt); err != "" {
		g.AddChild(formText(err, 0, y, fs-2*scale, color.RGBA{239, 68, 68, 255}))
	} else if help := formPropStr(ln.Node, "help", rt); help != "" {
		g.AddChild(formText(help, 0, y, fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}
	return g
}

func propTruthy(n *model.Node, key string) bool {
	v, ok := n.Prop(key)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	default:
		return false
	}
}

// ---- textformfield ----------------------------------------------------------

// TextFormField is a labelled single-line editor with prefix/suffix, error and
// helper text (shares the canvas edit session via editableType).
type TextFormField struct {
	mu     sync.Mutex
	inters map[*model.Node]*canvas.Interaction
}

func (t *TextFormField) cacheInter(n *model.Node, inter *canvas.Interaction) {
	if inter == nil {
		return
	}
	t.mu.Lock()
	t.inters[n] = inter
	t.mu.Unlock()
}

func (t *TextFormField) sessionFor(n *model.Node) *canvas.InputState {
	t.mu.Lock()
	inter := t.inters[n]
	t.mu.Unlock()
	if inter == nil || inter.Input == nil || inter.Input.Node != n {
		return nil
	}
	return inter.Input
}

func (*TextFormField) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	w = 220 * scale
	h = 40 * scale
	if formPropStr(n, "label", rt) != "" {
		h += lineHeight(fs-2*scale) + 4*scale
	}
	if formPropStr(n, "error", rt) != "" || formPropStr(n, "helper", rt) != "" {
		h += lineHeight(fs-2*scale) + 4*scale
	}
	return w, h
}

func (t *TextFormField) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	fs := formFontSizeLN(ln, scale)
	y := 0.0
	if label := formPropStr(ln.Node, "label", rt); label != "" {
		g.AddChild(formText(label, 0, y, fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		y += float64(lineHeight(fs-2*scale) + 4*scale)
	}
	errText := formPropStr(ln.Node, "error", rt)
	fieldH := float64(40 * scale)
	chrome := draw.NewRect()
	chrome.Y = y
	chrome.Width = float64(ln.Width)
	chrome.Height = fieldH
	chrome.BorderRadius = 8 * float64(scale)
	chrome.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	if errText != "" {
		chrome.Stroke = color.RGBA{239, 68, 68, 255}
	} else {
		chrome.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	}
	chrome.StrokeWidth = float64(scale)
	g.AddChild(chrome)

	pad := float64(10 * scale)
	x := pad
	if pre := formPropStr(ln.Node, "prefix", rt); pre != "" {
		g.AddChild(formText(pre, x, y+(fieldH-float64(lineHeight(fs)))/2, fs, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
		x += float64(int(canvas.MeasureText(pre, float64(fs)))) + float64(8*scale)
	}
	text, ph := t.display(ln.Node, rt)
	ink := formInk(ln.Node, ln, rt)
	if ph {
		ink = themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	}
	if text != "" {
		g.AddChild(formText(text, x, y+(fieldH-float64(lineHeight(fs)))/2, fs, ink))
	}
	if sess := t.sessionFor(ln.Node); sess != nil {
		if int(time.Since(sess.BlinkStart)/(500*time.Millisecond))%2 == 0 {
			cx := x + float64(int(canvas.MeasureText(string(sess.Runes[:min(sess.Cursor, len(sess.Runes))]), float64(fs))))
			caret := draw.NewRect()
			caret.NoHit = true
			caret.X = cx
			caret.Y = y + float64(8*scale)
			caret.Width = float64(scale)
			if caret.Width < 1 {
				caret.Width = 1
			}
			caret.Height = fieldH - float64(16*scale)
			caret.Fill = formInk(ln.Node, ln, rt)
			g.AddChild(caret)
		}
	}
	y += fieldH + float64(4*scale)
	if errText != "" {
		g.AddChild(formText(errText, 0, y, fs-2*scale, color.RGBA{239, 68, 68, 255}))
	} else if help := formPropStr(ln.Node, "helper", rt); help != "" {
		g.AddChild(formText(help, 0, y, fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}
	return g
}

func (t *TextFormField) display(n *model.Node, rt *runtime.Runtime) (string, bool) {
	if sess := t.sessionFor(n); sess != nil {
		return string(sess.Runes), len(sess.Runes) == 0
	}
	v := formEvalStr(n.Value, rt)
	if v != "" {
		return v, false
	}
	return n.Placeholder, true
}

func (t *TextFormField) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	t.cacheInter(n, inter)
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	return true
}

func (t *TextFormField) OnFocused(n *model.Node, inter *canvas.Interaction) {
	t.cacheInter(n, inter)
}

// ---- richtext ---------------------------------------------------------------

// RichText lays out styled spans with multi-line wrap when the box width is
// constrained (style.width or laid-out width). Spans keep their own font size,
// color, and underline; wrapping is greedy by words, then by runes.
type RichText struct{}

func (RichText) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	spans := richSpans(n, rt, scale)
	avail := 0
	if n != nil && n.Style != nil {
		if f, ok := n.Style["width"].(float64); ok && f > 0 {
			avail = int(f) * scale
		}
	}
	if avail > 0 {
		lines, maxW, totalH := layoutRichLines(spans, avail)
		if len(lines) == 0 {
			return scale, lineHeight(14 * scale)
		}
		return maxW, totalH
	}
	for _, sp := range spans {
		w += sp.w
		if sp.h > h {
			h = sp.h
		}
	}
	if h < 1 {
		h = lineHeight(14 * scale)
	}
	if w < 1 {
		w = scale
	}
	return w, h
}

func (RichText) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	spans := richSpans(ln.Node, rt, scale)
	lines, _, _ := layoutRichLines(spans, ln.Width)
	y := 0.0
	for _, line := range lines {
		x := 0.0
		lineH := 0
		for _, sp := range line {
			if sp.h > lineH {
				lineH = sp.h
			}
		}
		if lineH < 1 {
			lineH = lineHeight(14 * scale)
		}
		for _, sp := range line {
			if sp.text == "" {
				continue
			}
			g.AddChild(formText(sp.text, x, y, sp.fs, sp.ink))
			if sp.underline {
				ul := draw.NewRect()
				ul.NoHit = true
				ul.X = x
				ul.Y = y + float64(lineH) - float64(scale)
				ul.Width = float64(sp.w)
				ul.Height = float64(scale)
				if ul.Height < 1 {
					ul.Height = 1
				}
				ul.Fill = sp.ink
				g.AddChild(ul)
			}
			x += float64(sp.w)
		}
		y += float64(lineH)
	}
	return g
}

type richSpan struct {
	text      string
	fs, w, h  int
	ink       color.RGBA
	underline bool
}

func richSpans(n *model.Node, rt *runtime.Runtime, scale int) []richSpan {
	if n == nil {
		return nil
	}
	raw, ok := n.Prop("spans")
	if !ok {
		return nil
	}
	arr, _ := raw.([]any)
	if s, ok := raw.(string); ok && strings.Contains(s, "{{") {
		arr, _ = runtime.EvalBinding(s, formCtx(rt)).([]any)
	}
	var out []richSpan
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		text := formEvalStr(fmt.Sprint(m["text"]), rt)
		fs := 14 * scale
		if v, ok := m["fontSize"]; ok {
			if f := formFloat(v); f > 0 {
				fs = int(f) * scale
			}
		}
		ink := themeColor(rt, "text", color.RGBA{29, 29, 31, 255})
		if c, ok := m["color"].(string); ok && c != "" {
			ink = parseHexOrTheme(c, rt)
		}
		ul, _ := m["underline"].(bool)
		out = append(out, richSpan{
			text: text, fs: fs, w: int(canvas.MeasureText(text, float64(fs))),
			h: lineHeight(fs), ink: ink, underline: ul,
		})
	}
	return out
}

// layoutRichLines folds spans into lines that fit availW. Empty availW keeps
// a single line (unconstrained measure).
func layoutRichLines(spans []richSpan, availW int) (lines [][]richSpan, maxW, totalH int) {
	if len(spans) == 0 {
		return nil, 0, 0
	}
	if availW <= 0 {
		w, h := 0, 0
		for _, sp := range spans {
			w += sp.w
			if sp.h > h {
				h = sp.h
			}
		}
		return [][]richSpan{spans}, w, h
	}
	var cur []richSpan
	curW := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		lineH := 0
		lw := 0
		for _, sp := range cur {
			lw += sp.w
			if sp.h > lineH {
				lineH = sp.h
			}
		}
		if lw > maxW {
			maxW = lw
		}
		totalH += lineH
		lines = append(lines, cur)
		cur = nil
		curW = 0
	}
	for _, sp := range spans {
		if sp.text == "" {
			continue
		}
		// Whole span fits on the current line.
		if curW+sp.w <= availW || (curW == 0 && sp.w <= availW) {
			cur = append(cur, sp)
			curW += sp.w
			continue
		}
		// Need to break the span's text across lines.
		rest := sp.text
		for rest != "" {
			room := availW - curW
			if room <= 0 {
				flush()
				room = availW
			}
			chunk, next := takeRichChunk(rest, sp.fs, room)
			if chunk == "" {
				// Single rune wider than room: force one rune so we progress.
				r, size := utf8.DecodeRuneInString(rest)
				if r == utf8.RuneError && size == 0 {
					break
				}
				chunk = rest[:size]
				next = rest[size:]
			}
			frag := richSpan{
				text: chunk, fs: sp.fs,
				w: int(canvas.MeasureText(chunk, float64(sp.fs))),
				h: sp.h, ink: sp.ink, underline: sp.underline,
			}
			cur = append(cur, frag)
			curW += frag.w
			rest = next
			if rest != "" {
				flush()
			}
		}
	}
	flush()
	if maxW < 1 {
		maxW = availW
	}
	if totalH < 1 {
		totalH = lineHeight(14)
	}
	return lines, maxW, totalH
}

// takeRichChunk returns the longest prefix of text that fits in room px,
// breaking on spaces when possible, else between runes. The remainder is next.
func takeRichChunk(text string, fs, room int) (chunk, next string) {
	if room <= 0 || text == "" {
		return "", text
	}
	if int(canvas.MeasureText(text, float64(fs))) <= room {
		return text, ""
	}
	// Prefer word boundaries (space after a word).
	best := 0
	lastSpace := -1
	for i, r := range text {
		end := i + utf8.RuneLen(r)
		if int(canvas.MeasureText(text[:end], float64(fs))) > room {
			break
		}
		best = end
		if r == ' ' {
			lastSpace = end
		}
	}
	if best == 0 {
		return "", text
	}
	if lastSpace > 0 && lastSpace < len(text) {
		return text[:lastSpace], text[lastSpace:]
	}
	return text[:best], text[best:]
}

func parseHexOrTheme(c string, rt *runtime.Runtime) color.RGBA {
	c = strings.TrimSpace(c)
	if rt != nil && rt.Theme != nil {
		if col, ok := rt.Theme.GetColor(c); ok {
			return col
		}
		if strings.HasPrefix(c, "var(--") && strings.HasSuffix(c, ")") {
			name := strings.TrimSuffix(strings.TrimPrefix(c, "var(--"), ")")
			if col, ok := rt.Theme.GetColor(name); ok {
				return col
			}
		}
	}
	if strings.HasPrefix(c, "#") {
		hex := c[1:]
		parse := func(s string) uint8 {
			var v uint64
			fmt.Sscanf(s, "%x", &v)
			return uint8(v)
		}
		switch len(hex) {
		case 3:
			return color.RGBA{parse(hex[0:1] + hex[0:1]), parse(hex[1:2] + hex[1:2]), parse(hex[2:3] + hex[2:3]), 255}
		case 6:
			return color.RGBA{parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6]), 255}
		case 8:
			return color.RGBA{parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6]), parse(hex[6:8])}
		}
	}
	return color.RGBA{29, 29, 31, 255}
}

// ---- carousel ---------------------------------------------------------------

// Carousel shows one child at a time; press left/right third to step, dots
// under the stage when indicators is true.
type Carousel struct {
	mu    sync.Mutex
	index map[*model.Node]int
	geoms map[*model.Node]image.Rectangle
}

func (c *Carousel) idx(n *model.Node) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.index[n]
}

func (c *Carousel) setIdx(n *model.Node, i, nMax int) {
	if nMax <= 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= nMax {
		i = nMax - 1
	}
	c.mu.Lock()
	c.index[n] = i
	c.mu.Unlock()
}

func (c *Carousel) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	w, h = 280*scale, 160*scale
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
	if propTruthy(n, "indicators") {
		h += 20 * scale
	}
	return w, h
}

func (c *Carousel) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return c.record(ln, rt, scale, nil)
}

func (c *Carousel) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return c.record(ln, rt, scale, sinks)
}

func (c *Carousel) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if ln == nil || ln.Node == nil {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	kids := ln.Children
	n := len(kids)
	if n == 0 {
		return nil
	}
	i := c.idx(ln.Node)
	if i >= n {
		i = n - 1
		c.setIdx(ln.Node, i, n)
	}
	c.mu.Lock()
	c.geoms[ln.Node] = image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)
	c.mu.Unlock()

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	g.Clip = true
	stageH := ln.Height
	if propTruthy(ln.Node, "indicators") {
		stageH -= 20 * scale
	}
	child := kids[i]
	bounds := image.Rect(0, 0, ln.Width, stageH)
	if cn := canvas.PerformLayoutWithSinks(child, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
		g.AddChild(cn)
	}
	ln.Children = nil
	if propTruthy(ln.Node, "indicators") {
		dotY := float64(stageH + 6*scale)
		totalW := float64(n*(7*scale) + (n-1)*(6*scale))
		x := (float64(ln.Width) - totalW) / 2
		for di := 0; di < n; di++ {
			d := draw.NewCircle()
			d.Radius = float64(3*scale) + 0.5
			if di == i {
				d.Fill = formAccent(rt)
			} else {
				d.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			}
			dg := draw.NewGroup()
			dg.X, dg.Y = x, dotY
			dg.AddChild(d)
			g.AddChild(dg)
			x += float64(13 * scale)
		}
	}
	return g
}

func (c *Carousel) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	count := len(n.Children)
	if count == 0 {
		return false
	}
	c.mu.Lock()
	box := c.geoms[n]
	c.mu.Unlock()
	if box.Empty() {
		box = frame
	}
	// Tap left third → prev, right third → next, dots zone uses x proportion.
	relX := int(p.X) - box.Min.X
	w := box.Dx()
	if w < 1 {
		w = 1
	}
	cur := c.idx(n)
	if relX < w/3 {
		c.setIdx(n, cur-1, count)
		return true
	}
	if relX > 2*w/3 {
		c.setIdx(n, cur+1, count)
		return true
	}
	// Middle: advance next (tap to cycle).
	c.setIdx(n, (cur+1)%count, count)
	return true
}
