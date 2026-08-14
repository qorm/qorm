package widgets

import (
	"image"
	"image/color"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("accordion", &Accordion{actives: map[*model.Node]int{}, headers: map[*model.Node][]image.Rectangle{}, contents: map[*model.Node][]contentHit{}})
}

// contentHit pairs a laid-out content child's screen rect with its model node
// so a press inside an open panel can be forwarded to the child's own handler.
type contentHit struct {
	rect image.Rectangle
	node *model.Node
}

// Accordion is a stacked expand/collapse (HTML render_data.go accordion): each
// child carries a `title`, and the `active` index (a binding, default 0 — the
// first panel open) names the one expanded panel whose content shows below its
// header. Clicking a header expands that panel (and collapses it again when it
// is already the active one — a single-open accordion, the common form).
type Accordion struct {
	mu       sync.Mutex
	actives  map[*model.Node]int
	headers  map[*model.Node][]image.Rectangle // header rects (screen px), for press mapping
	contents map[*model.Node][]contentHit      // open panel content rects, for handler forwarding
}

const accordionHeaderH = 44

// active resolves the open panel index: the bound `active` prop when it is a
// whole-string binding, else the local toggle state (default 0, matching the
// HTML's activeIndex default), clamped to the child count.
func (a *Accordion) active(n *model.Node, rt *runtime.Runtime, count int) int {
	idx := 0
	if raw, ok := n.Prop("active"); ok {
		if s, ok := raw.(string); ok {
			if path := formBoundPath(s); path != "" {
				// StatePath resolves dotted paths the same way SetStatePath
				// writes them ({{state.ui.open}} nests).
				if v := rt.StatePath(path); v != nil {
					idx = int(asFloat64(v))
				}
			}
		}
	}
	a.mu.Lock()
	if v, ok := a.actives[n]; ok {
		idx = v
	}
	a.mu.Unlock()
	if idx < 0 {
		return -1
	}
	if count <= 0 {
		return -1
	}
	if idx >= count {
		return count - 1
	}
	return idx
}

// setActive writes the new active index: through the bound state when the
// `active` prop is a binding, else the local toggle state.
func (a *Accordion) setActive(n *model.Node, rt *runtime.Runtime, idx int) {
	if raw, ok := n.Prop("active"); ok {
		if s, ok := raw.(string); ok {
			if path := formBoundPath(s); path != "" {
				rt.SetStatePath(path, idx)
				return
			}
		}
	}
	a.mu.Lock()
	a.actives[n] = idx
	a.mu.Unlock()
}

// Measure reports the stacked size: every header plus the active child's
// content.
func (a *Accordion) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	active := a.active(n, rt, len(n.Children))
	for i, c := range n.Children {
		if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
			if cln.Width > w {
				w = cln.Width
			}
			if i == active {
				h += 16*scale + cln.Height // record pads the panel 8px top + 8px bottom
			}
		}
		h += accordionHeaderH * scale
	}
	return w, h
}

func (a *Accordion) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return a.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget (the panels' content
// flows through with the frame's sinks).
func (a *Accordion) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return a.record(ln, rt, scale, sinks)
}

func (a *Accordion) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	active := a.active(ln.Node, rt, len(ln.Children))
	headerH := accordionHeaderH * scale
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	// The outer chrome: a bordered rounded column.
	chrome := draw.NewRect()
	chrome.Width = float64(ln.Width)
	chrome.Height = float64(ln.Height)
	chrome.BorderRadius = 10 * float64(scale)
	chrome.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	chrome.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	chrome.StrokeWidth = float64(scale)
	g.AddChild(chrome)

	headers := make([]image.Rectangle, 0, len(ln.Children))
	contents := make([]contentHit, 0, len(ln.Children))
	y := 0
	for i, cln := range ln.Children {
		title := "Item"
		if raw, ok := cln.Node.Prop("title"); ok {
			if s, ok := raw.(string); ok {
				title = s
			}
		}
		// Header: a full-width row with the title. Header rects are recorded
		// in SCREEN coordinates — HandlePointer maps presses in the same space.
		header := draw.NewRect()
		header.X = float64(scale)
		header.Y = float64(y)
		header.Width = float64(ln.Width - 2*scale)
		header.Height = float64(headerH)
		header.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
		g.AddChild(header)
		if i > 0 {
			sep := draw.NewRect()
			sep.X = float64(14 * scale)
			sep.Y = float64(y)
			sep.Width = float64(ln.Width - 14*scale)
			sep.Height = float64(scale)
			sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			g.AddChild(sep)
		}
		chevron := "▾"
		if i == active {
			chevron = "▴"
		}
		g.AddChild(formText(title, float64(14*scale), float64(y)+(float64(headerH)-float64(lineHeight(fs)))/2, fs, ink))
		g.AddChild(formText(chevron, float64(ln.Width-24*scale), float64(y)+(float64(headerH)-float64(lineHeight(fs)))/2, fs, ink))
		headers = append(headers, image.Rect(ln.AbsX, ln.AbsY+y, ln.AbsX+ln.Width, ln.AbsY+y+headerH))
		y += headerH

		// The active panel's content below its header; its rect is recorded
		// so a press inside can be forwarded to the child's own handler.
		if i == active && cln.Node != nil {
			ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
			bounds := image.Rect(14*scale, y+8*scale, ln.Width-14*scale, y+8*scale+ch)
			if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX+14*scale, ln.AbsY+y+8*scale), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
			contents = append(contents, contentHit{
				rect: image.Rect(ln.AbsX+14*scale, ln.AbsY+y+8*scale, ln.AbsX+ln.Width-14*scale, ln.AbsY+y+8*scale+ch),
				node: cln.Node,
			})
			y += 16*scale + ch
		}
	}
	ln.Children = nil

	a.mu.Lock()
	a.headers[ln.Node] = headers
	a.contents[ln.Node] = contents
	a.mu.Unlock()
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on a header
// toggles that panel — expanding it, or collapsing it when it is already the
// active one.
func (a *Accordion) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	a.mu.Lock()
	headers := a.headers[n]
	contents := a.contents[n]
	a.mu.Unlock()
	active := a.active(n, rt, len(n.Children))
	for i, h := range headers {
		if p.X >= float64(h.Min.X) && p.X <= float64(h.Max.X) &&
			p.Y >= float64(h.Min.Y) && p.Y <= float64(h.Max.Y) {
			if i == active {
				a.setActive(n, rt, -1) // collapse the open panel
			} else {
				a.setActive(n, rt, i)
			}
			return true
		}
	}
	// A press on an open panel's content forwards to the content child's own
	// handler (the widget owns its subtree's input, so it passes the press
	// along rather than eating it).
	for _, ch := range contents {
		if p.X >= float64(ch.rect.Min.X) && p.X <= float64(ch.rect.Max.X) &&
			p.Y >= float64(ch.rect.Min.Y) && p.Y <= float64(ch.rect.Max.Y) {
			if ch.node.OnPress != nil && !formDisabled(ch.node, rt) {
				dispatchInvoke(ch.node.OnPress, rt)
				return true
			}
		}
	}
	return false
}
