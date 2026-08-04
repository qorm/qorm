package widgets

import (
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("expansiontile", &ExpansionTile{opens: map[*model.Node]bool{}, geo: map[*model.Node]image.Rectangle{}, contents: map[*model.Node][]contentHit{}})
}

// ExpansionTile is a self-contained expand/collapse row (HTML render_data.go
// expansionTile): a summary (title / leading + chevron) whose children show
// below when expanded. `initiallyExpanded` seeds the open state; clicking the
// summary toggles it.
type ExpansionTile struct {
	mu       sync.Mutex
	opens    map[*model.Node]bool
	geo      map[*model.Node]image.Rectangle // summary rect (screen px), for press mapping
	contents map[*model.Node][]contentHit    // expanded content rects, for handler forwarding
}

const expansionSummaryH = 44

func (t *ExpansionTile) isOpen(n *model.Node) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v, ok := t.opens[n]; ok {
		return v
	}
	if raw, ok := n.Prop("initiallyExpanded"); ok {
		return formTruthy(raw)
	}
	return false
}

func (t *ExpansionTile) setOpen(n *model.Node, v bool) {
	t.mu.Lock()
	t.opens[n] = v
	t.mu.Unlock()
}

// Measure reports the summary plus the children when expanded.
func (t *ExpansionTile) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	label := formLabel(n, rt)
	if label == "" {
		label = "Expand"
	}
	w = int(canvas.MeasureText(label, float64(fs))) + 40*scale
	h = expansionSummaryH * scale
	if t.isOpen(n) {
		for _, c := range n.Children {
			if cln := canvas.Measure(c, rt, nil, scale); cln != nil {
				if cln.Width > w {
					w = cln.Width
				}
				h += cln.Height
			}
		}
	}
	return w, h
}

func (t *ExpansionTile) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return t.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget (the children flow
// through with the frame's sinks).
func (t *ExpansionTile) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return t.record(ln, rt, scale, sinks)
}

func (t *ExpansionTile) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	open := t.isOpen(ln.Node)
	summaryH := expansionSummaryH * scale
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	label := formLabel(ln.Node, rt)
	if label == "" {
		label = "Expand"
	}

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	sep := draw.NewRect()
	sep.Width = float64(ln.Width)
	sep.Height = float64(scale)
	sep.Y = float64(ln.Height - scale)
	sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	g.AddChild(sep)

	chevron := "▾"
	if open {
		chevron = "▴"
	}
	if lead, ok := ln.Node.Prop("leading"); ok {
		if s, ok := lead.(string); ok {
			g.AddChild(formText(s, float64(14*scale), (float64(summaryH)-float64(lineHeight(fs)))/2, fs, ink))
			g.AddChild(formText(label, float64(14*scale+int(canvas.MeasureText(s, float64(fs)))+8*scale),
				(float64(summaryH)-float64(lineHeight(fs)))/2, fs, ink))
		}
	} else {
		g.AddChild(formText(label, float64(14*scale), (float64(summaryH)-float64(lineHeight(fs)))/2, fs, ink))
	}
	g.AddChild(formText(chevron, float64(ln.Width-24*scale), (float64(summaryH)-float64(lineHeight(fs)))/2, fs, ink))

	contents := make([]contentHit, 0, len(ln.Children))
	y := summaryH
	if open {
		for _, cln := range ln.Children {
			ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
			bounds := image.Rect(14*scale, y, ln.Width-14*scale, y+ch)
			if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX+14*scale, ln.AbsY+y), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
				g.AddChild(cn)
			}
			contents = append(contents, contentHit{
				rect: image.Rect(ln.AbsX+14*scale, ln.AbsY+y, ln.AbsX+ln.Width-14*scale, ln.AbsY+y+ch),
				node: cln.Node,
			})
			y += ch
		}
	}
	ln.Children = nil

	t.mu.Lock()
	t.geo[ln.Node] = image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+summaryH)
	t.contents[ln.Node] = contents
	t.mu.Unlock()
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on the summary
// toggles the expansion; a press on the expanded content forwards to the
// pressed child's own handler.
func (t *ExpansionTile) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	t.mu.Lock()
	summary := t.geo[n]
	contents := t.contents[n]
	t.mu.Unlock()
	if p.X >= float64(summary.Min.X) && p.X <= float64(summary.Max.X) &&
		p.Y >= float64(summary.Min.Y) && p.Y <= float64(summary.Max.Y) {
		t.setOpen(n, !t.isOpen(n))
		return true
	}
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
