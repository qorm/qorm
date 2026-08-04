package widgets

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("contextmenu", &ContextMenu{open: map[*model.Node]bool{}, geoms: map[*model.Node]*ctxGeo{}})
	canvas.RegisterWidget("cupertinocontextmenu", &ContextMenu{open: map[*model.Node]bool{}, geoms: map[*model.Node]*ctxGeo{}})
}

// ContextMenu wraps a child (the trigger) and opens a floating menu at the
// right-click position (HTML render_widgets.go contextMenu, the `items` prop:
// [{title, name, args, separator}]). The menu is an overlay: clicking an item
// dispatches its action and closes; clicking anywhere else closes it. A
// Long-press opener (touch) is a later milestone.
type ContextMenu struct {
	mu    sync.Mutex
	open  map[*model.Node]bool
	geoms map[*model.Node]*ctxGeo
}

// ctxGeo carries the right-click position and the last-drawn panel geometry
// so HandlePointer can map a press to the item under it in the same place the
// panel was painted. The geometry is computed in OverlayRecord (which has the
// device scale); HandlePointer only records the click.
type ctxGeo struct {
	clickPt image.Point // the right-click position (screen px)
	panel   image.Rectangle
	rowH    int
	padTop  int // the panel's top padding, so item mapping matches the draw
}

const ctxMenuRowH = 34

func (c *ContextMenu) isOpen(n *model.Node) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.open[n]
}

func (c *ContextMenu) setOpen(n *model.Node, v bool) {
	c.mu.Lock()
	c.open[n] = v
	c.mu.Unlock()
}

func (c *ContextMenu) geo(n *model.Node) *ctxGeo {
	c.mu.Lock()
	defer c.mu.Unlock()
	g := c.geoms[n]
	if g == nil {
		g = &ctxGeo{}
		c.geoms[n] = g
	}
	return g
}

// ctxItem is one menu item parsed from the `items` prop.
type ctxItem struct {
	title     string
	name      string
	args      map[string]string
	separator bool
}

func parseCtxItems(n *model.Node) []ctxItem {
	var out []ctxItem
	raw, ok := n.Prop("items")
	if !ok {
		return out
	}
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		item := ctxItem{}
		if v, ok := m["title"].(string); ok {
			item.title = v
		}
		if v, ok := m["name"].(string); ok {
			item.name = v
		}
		if v, ok := m["id"].(string); ok && item.name == "" {
			item.name = v // the HTML's id spelling fires the same handler
		}
		if v, ok := m["separator"].(bool); ok {
			item.separator = v
		}
		if args, ok := m["args"].(map[string]any); ok {
			item.args = make(map[string]string, len(args))
			for k, v := range args {
				item.args[k] = fmt.Sprint(v)
			}
		}
		out = append(out, item)
	}
	return out
}

// contentMeasure sizes a ChildLayoutWidget from its children: the generic
// pass does NOT count children toward a widget's size (v1 leaf semantics), so
// without this a content-sized wrapper collapses to 0 in a ROW (flex stretch
// only rescues it in a column). Column layout is unchanged — the same size.
func contentMeasure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int) {
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

// Measure reports the wrapped child's size (children measure through).
func (*ContextMenu) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return contentMeasure(n, rt, scale)
}

func (c *ContextMenu) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return c.record(ln, rt, scale, nil)
}

// RecordWithSinks implements canvas.ChildLayoutWidget: the trigger content
// flows through the generic pass with the frame's sinks.
func (c *ContextMenu) RecordWithSinks(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	return c.record(ln, rt, scale, sinks)
}

func (c *ContextMenu) record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, sinks *canvas.LayoutSinks) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)
	y := 0
	for _, cln := range ln.Children {
		ch := cln.Height + cln.Style.MarginTop + cln.Style.MarginBot
		bounds := image.Rect(0, y, ln.Width, y+ch)
		if cn := canvas.PerformLayoutWithSinks(cln, bounds, image.Pt(ln.AbsX, ln.AbsY), canvas.SinksInter(sinks), rt, scale, sinks); cn != nil {
			g.AddChild(cn)
		}
		y += ch
	}
	ln.Children = nil
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a right press on the
// wrapped child opens the menu; while open, a press inside the panel
// dispatches the item under it and a press elsewhere closes it.
func (c *ContextMenu) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	items := parseCtxItems(n)
	// Closed and left-pressed: the widget owns its subtree's input, so it must
	// forward the press to the trigger child's own handler — a button (or any
	// onPress child) inside a contextmenu stays clickable.
	if !c.isOpen(n) && p.Type == canvas.PointerPress && !p.Right {
		if child := n.Children[0]; child != nil && child.OnPress != nil && !formDisabled(child, rt) {
			argAny := make(map[string]any, len(child.OnPress.Args))
			for k, v := range child.OnPress.Args {
				argAny[k] = v
			}
			rt.Dispatch(child.OnPress.Name, argAny)
			return true
		}
	}
	if p.Type == canvas.PointerPress && p.Right {
		if c.isOpen(n) {
			c.setOpen(n, false) // a right-press elsewhere closes, like a native menu
			return true
		}
		if len(items) == 0 {
			return false
		}
		c.setOpen(n, true)
		// The panel geometry (sized for the device scale) is computed by
		// OverlayRecord on the next frame; only the click position is kept here.
		g := c.geo(n)
		g.clickPt = image.Pt(int(p.X), int(p.Y))
		return true
	}
	if c.isOpen(n) && p.Type == canvas.PointerPress {
		g := c.geo(n)
		c.setOpen(n, false)
		if p.X >= float64(g.panel.Min.X) && p.X < float64(g.panel.Max.X) &&
			p.Y >= float64(g.panel.Min.Y) && p.Y < float64(g.panel.Max.Y) {
			idx := (int(p.Y) - g.panel.Min.Y - g.padTop) / g.rowH
			if idx >= 0 && idx < len(items) && !items[idx].separator && items[idx].name != "" {
				// Item args evaluate like action args (the state root), so a
				// {{state.x}} spelling resolves before dispatch.
				ctx := map[string]any{"state": rt.State}
				argAny := make(map[string]any, len(items[idx].args))
				for k, v := range items[idx].args {
					argAny[k] = runtime.EvalBinding(v, ctx)
				}
				rt.Dispatch(items[idx].name, argAny)
			}
		}
		return true
	}
	return false
}

// OverlayOpen reports whether the menu is mounted above the scene.
func (c *ContextMenu) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	return c.isOpen(n) && len(parseCtxItems(n)) > 0
}

// OverlayRecord draws the floating menu over the scene at the right-click
// position.
func (c *ContextMenu) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !c.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	items := parseCtxItems(ln.Node)
	g := c.geo(ln.Node)
	// Compute the panel geometry at the recorded right-click position, anchored
	// so it stays on screen; HandlePointer maps presses against the same rect.
	panelW := max(200, 24*scale)
	rowH := ctxMenuRowH * scale
	panelH := len(items)*rowH + 12*scale
	stageW, stageH := overlayStageSize(rt, scale, g.clickPt.X, g.clickPt.Y)
	px, py := g.clickPt.X, g.clickPt.Y
	if px+panelW > stageW {
		px = stageW - panelW
	}
	if py+panelH > stageH {
		py = stageH - panelH
	}
	g.panel = image.Rect(px, py, px+panelW, py+panelH)
	g.rowH = rowH
	g.padTop = 6 * scale
	panel := g.panel

	root := draw.NewGroup()
	root.Width = float64(stageW)
	root.Height = float64(stageH)
	root.Model = ln.Node
	root.Overlay = true

	// Backdrop: a tap anywhere outside the panel closes the menu (the widget's
	// HandlePointer handles the press via the Overlay group's model).
	backdrop := draw.NewRect()
	backdrop.Width = float64(stageW)
	backdrop.Height = float64(stageH)
	root.AddChild(backdrop)

	// The panel.
	panelRect := draw.NewRect()
	panelRect.X = float64(panel.Min.X)
	panelRect.Y = float64(panel.Min.Y)
	panelRect.Width = float64(panel.Dx())
	panelRect.Height = float64(panel.Dy())
	panelRect.BorderRadius = 12 * float64(scale)
	panelRect.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
	panelRect.ShadowColor = color.RGBA{0, 0, 0, 90}
	panelRect.ShadowBlur = 20 * float64(scale)
	panelRect.ShadowY = 8 * float64(scale)
	root.AddChild(panelRect)

	// The items, each a full-width row with a centred-left title.
	fs := 14 * scale
	ink := formInk(ln.Node, ln, rt)
	rowY := panel.Min.Y + 6*scale
	for _, it := range items {
		if it.separator {
			sep := draw.NewRect()
			sep.X = float64(panel.Min.X + 8*scale)
			sep.Y = float64(rowY + g.rowH/2)
			sep.Width = float64(panel.Dx() - 16*scale)
			sep.Height = float64(scale)
			sep.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			root.AddChild(sep)
		} else if it.title != "" {
			root.AddChild(formText(it.title, float64(panel.Min.X+12*scale),
				float64(rowY)+(float64(g.rowH)-float64(lineHeight(fs)))/2, fs, ink))
		}
		rowY += g.rowH
	}
	return root
}
