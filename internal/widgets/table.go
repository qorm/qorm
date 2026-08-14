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
	canvas.RegisterWidget("table", &Table{geoms: map[*model.Node]*tableGeo{}})
	canvas.RegisterWidget("datatable", &Table{geoms: map[*model.Node]*tableGeo{}})
}

// Table renders a data table (HTML render_data.go table): a `columns` prop
// ([{title, field/value, width?}]) over the `data` binding (an array of
// objects), with a header row and one row per object. Clicking a header
// dispatches the node's OnChange with {column: field} so the author can sort
// their data (the HTML's app-wired sort contract).
type Table struct {
	mu    sync.Mutex
	geoms map[*model.Node]*tableGeo
}

type tableGeo struct {
	headers []image.Rectangle // header cell rects (screen px), for column mapping
}

const (
	tableHeaderH = 32
	tableRowH    = 32
)

// tableCol is one column parsed from the `columns` prop.
type tableCol struct {
	title string
	field string
	width int
}

func parseTableCols(n *model.Node) []tableCol {
	var out []tableCol
	raw, ok := n.Prop("columns")
	if !ok {
		return out
	}
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, c := range arr {
		col := tableCol{width: 120}
		if m, ok := c.(map[string]any); ok {
			if v, ok := m["title"].(string); ok {
				col.title = v
			}
			if v, ok := m["label"].(string); ok {
				col.title = v
			}
			if v, ok := m["field"].(string); ok {
				col.field = v
			}
			if v, ok := m["value"].(string); ok {
				col.field = v
			}
			if v, ok := m["width"].(float64); ok {
				col.width = int(v)
			}
		}
		if col.title == "" && col.field != "" {
			col.title = col.field
		}
		if col.field == "" {
			col.field = col.title
		}
		out = append(out, col)
	}
	return out
}

// tableData evaluates the `data` binding to the row objects.
func tableData(n *model.Node, rt *runtime.Runtime) []map[string]any {
	items, _ := runtime.EvalBinding(n.Data, map[string]any{"state": rt.State}).([]any)
	var out []map[string]any
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Measure reports the header + one row per data object, across the columns.
func (t *Table) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	cols := parseTableCols(n)
	for _, c := range cols {
		w += c.width * scale
	}
	h = (tableHeaderH + len(tableData(n, rt))*tableRowH) * scale
	return w, h
}

func (t *Table) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	cols := parseTableCols(ln.Node)
	rows := tableData(ln.Node, rt)
	fs := 13 * scale
	ink := formInk(ln.Node, ln, rt)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	sep := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})

	g := draw.NewGroup()
	g.Width = float64(ln.Width)
	g.Height = float64(ln.Height)

	headers := make([]image.Rectangle, 0, len(cols))
	x := 0
	for _, c := range cols {
		// Header cell.
		cell := draw.NewRect()
		cell.X = float64(x)
		cell.Width = float64(c.width * scale)
		cell.Height = float64(tableHeaderH * scale)
		cell.Fill = themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})
		g.AddChild(cell)
		if c.title != "" {
			g.AddChild(formText(c.title, float64(x+10*scale), (float64(tableHeaderH*scale)-float64(lineHeight(fs)))/2, fs, ink2))
		}
		headers = append(headers, image.Rect(ln.AbsX+x, ln.AbsY, ln.AbsX+x+c.width*scale, ln.AbsY+tableHeaderH*scale))
		x += c.width * scale

		// Column separator.
		vsep := draw.NewRect()
		vsep.X = float64(x - scale)
		vsep.Width = float64(scale)
		vsep.Height = float64(tableHeaderH * scale)
		vsep.Fill = sep
		g.AddChild(vsep)
	}

	// Data rows.
	y := tableHeaderH * scale
	for _, row := range rows {
		cx := 0
		for _, c := range cols {
			var val string
			if v, ok := row[c.field]; ok {
				val = runtime.Stringify(v)
			}
			if val != "" {
				g.AddChild(formText(val, float64(cx+10*scale), float64(y)+(float64(tableRowH*scale)-float64(lineHeight(fs)))/2, fs, ink))
			}
			cx += c.width * scale
		}
		hline := draw.NewRect()
		hline.Width = float64(ln.Width)
		hline.Height = float64(scale)
		hline.Y = float64(y + tableRowH*scale)
		hline.Fill = sep
		g.AddChild(hline)
		y += tableRowH * scale
	}

	t.mu.Lock()
	t.geoms[ln.Node] = &tableGeo{headers: headers}
	t.mu.Unlock()
	return g
}

// HandlePointer implements canvas.InteractiveWidget: a press on a header cell
// dispatches the node's OnChange with {column: field} — the sort contract the
// author wires to re-order their data.
func (t *Table) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || n.OnChange == nil {
		return false
	}
	t.mu.Lock()
	headers := t.geoms[n].headers
	t.mu.Unlock()
	cols := parseTableCols(n)
	for i, h := range headers {
		if p.X >= float64(h.Min.X) && p.X <= float64(h.Max.X) &&
			p.Y >= float64(h.Min.Y) && p.Y <= float64(h.Max.Y) {
			if i < len(cols) {
				args := map[string]any{"column": cols[i].field}
				for k, v := range n.OnChange.Args {
					args[k] = v
				}
				rt.Dispatch(n.OnChange.Name, args)
				return true
			}
		}
	}
	return false
}
