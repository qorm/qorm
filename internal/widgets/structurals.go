package widgets

import (
	"image"
	"image/color"
	"strconv"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("breadcrumb", &Breadcrumb{})
	canvas.RegisterWidget("steps", &Steps{})
	canvas.RegisterWidget("pagination", &Pagination{geoms: map[*model.Node]*paginationGeo{}})
	canvas.RegisterWidget("timeline", &Timeline{})
}

func stringList(v any) []string {
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, it := range arr {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func boundArray(n *model.Node, rt *runtime.Runtime, key string) []any {
	raw, ok := n.Prop(key)
	if !ok {
		return nil
	}
	if s, ok := raw.(string); ok {
		if a, _ := runtime.EvalBinding(s, formCtxLn(rt, nil)).([]any); a != nil {
			return a
		}
	}
	if a, ok := raw.([]any); ok {
		return a
	}
	return nil
}

// ---------------------------------------------------------------------------
// breadcrumb
// ---------------------------------------------------------------------------
type Breadcrumb struct{}

func (*Breadcrumb) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	items := stringList(n.Props["items"])
	sep := "/"
	if v, ok := n.Prop("separator"); ok {
		if s, ok := v.(string); ok {
			sep = s
		}
	}
	sw := int(canvas.MeasureText(sep, float64(fs)))
	for _, it := range items {
		w += int(canvas.MeasureText(it, float64(fs))) + sw + 8*scale
	}
	h = lineHeight(fs)
	return
}

func (*Breadcrumb) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	items := stringList(ln.Node.Props["items"])
	sep := "/"
	if v, ok := ln.Node.Prop("separator"); ok {
		if s, ok := v.(string); ok {
			sep = s
		}
	}
	fs := formFontSizeLN(ln, scale)
	sw := int(canvas.MeasureText(sep, float64(fs)))
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	x := 0
	for i, it := range items {
		c := formInk(ln.Node, ln, rt)
		if i == len(items)-1 {
			c = themeColor(rt, "text", color.RGBA{17, 24, 39, 255})
		}
		g.AddChild(formText(it, float64(x), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, c))
		x += int(canvas.MeasureText(it, float64(fs)))
		if i < len(items)-1 {
			sepC := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
			g.AddChild(formText(sep, float64(x+4*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, sepC))
			x += sw + 8*scale
		}
	}
	return g
}

// ---------------------------------------------------------------------------
// steps
// ---------------------------------------------------------------------------
type Steps struct{}

func (*Steps) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	labels := stringList(n.Props["steps"])
	fs := formFontSize(n, scale)
	for _, l := range labels {
		w += int(canvas.MeasureText(l, float64(fs))) + 32*scale
	}
	if len(labels) > 1 {
		w += (len(labels) - 1) * 30 * scale
	}
	h = lineHeight(fs) + 8*scale
	return
}

func (*Steps) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	labels := stringList(ln.Node.Props["steps"])
	active := 0
	if v, ok := ln.Node.Prop("active"); ok {
		if s, ok := v.(string); ok {
			if vv := runtime.EvalBinding(s, formCtxLn(rt, ln)); vv != nil {
				active = int(asFloat64(vv))
			}
		} else {
			active = int(asFloat64(v))
		}
	}
	fs := formFontSizeLN(ln, scale)
	smallFS := 12 * scale
	accent := formAccent(rt)
	white := color.RGBA{255, 255, 255, 255}
	sepG := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	ink := formInk(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	x := 0
	for i, lbl := range labels {
		circleG := draw.NewCircle()
		circleG.Radius = 12 * float64(scale)
		circleG.X = float64(x + 12*scale)
		circleG.Y = float64(ln.Height)/2 - 12*float64(scale)
		num := strconv.Itoa(i + 1)
		tw := int(canvas.MeasureText(num, float64(smallFS)))
		if i <= active {
			circleG.Fill = accent
			g.AddChild(circleG)
			g.AddChild(formText(num, float64(x+12*scale-tw/2), float64(ln.Height)/2-float64(lineHeight(smallFS))/2, smallFS, white))
		} else {
			circleG.Fill = sepG
			g.AddChild(circleG)
			g.AddChild(formText(num, float64(x+12*scale-tw/2), float64(ln.Height)/2-float64(lineHeight(smallFS))/2, smallFS, ink2))
		}
		lblC := ink2
		if i <= active {
			lblC = ink
		}
		g.AddChild(formText(lbl, float64(x+32*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, lblC))
		lw := int(canvas.MeasureText(lbl, float64(fs))) + 32*scale
		x += lw
		if i < len(labels)-1 {
			conn := draw.NewRect()
			conn.X = float64(x)
			conn.Y = float64(ln.Height)/2 - float64(scale)/2
			conn.Width, conn.Height = float64(30*scale), float64(scale)
			conn.Fill = sepG
			g.AddChild(conn)
			x += 30 * scale
		}
	}
	return g
}

// ---------------------------------------------------------------------------
// pagination
// ---------------------------------------------------------------------------
type Pagination struct {
	geoms map[*model.Node]*paginationGeo
}
type paginationGeo struct {
	buttons []image.Rectangle
	pages   []int
}

func (p *Pagination) geo(n *model.Node) *paginationGeo {
	g, ok := p.geoms[n]
	if !ok {
		g = &paginationGeo{}
		p.geoms[n] = g
	}
	return g
}

func (*Pagination) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	total := 1
	if v, ok := n.Prop("total"); ok {
		total = int(asFloat64(v))
	}
	fs := formFontSize(n, scale)
	for i := 0; i < total+2; i++ {
		w += 32 * scale
	}
	w += (total + 1) * (6 * scale)
	h = lineHeight(fs) + 16*scale
	return
}

func (p *Pagination) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	page := 1
	if v, ok := ln.Node.Prop("page"); ok {
		if s, ok := v.(string); ok {
			if vv := runtime.EvalBinding(s, formCtxLn(rt, ln)); vv != nil {
				page = int(asFloat64(vv))
			}
		} else {
			page = int(asFloat64(v))
		}
	}
	total := 1
	if v, ok := ln.Node.Prop("total"); ok {
		total = int(asFloat64(v))
	}
	if total < 1 {
		total = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	accent := formAccent(rt)
	white := color.RGBA{255, 255, 255, 255}
	sepG := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)

	type pb struct {
		label  string
		target int
	}
	var btns []pb
	btns = append(btns, pb{"←", page - 1})
	for p := 1; p <= total; p++ {
		btns = append(btns, pb{strconv.Itoa(p), p})
	}
	btns = append(btns, pb{"→", page + 1})

	geo := p.geo(ln.Node)
	geo.buttons = geo.buttons[:0]
	geo.pages = geo.pages[:0]
	x, btnW, btnH := 0, 32*scale, 32*scale
	btnY := (ln.Height - btnH) / 2
	for _, b := range btns {
		btn := draw.NewRect()
		btn.X, btn.Y = float64(x), float64(btnY)
		btn.Width, btn.Height = float64(btnW), float64(btnH)
		btn.BorderRadius = 6 * float64(scale)
		btn.Stroke, btn.StrokeWidth = sepG, float64(scale)
		btn.Fill = color.RGBA{255, 255, 255, 255}
		if b.target == page {
			btn.Fill = accent
		}
		g.AddChild(btn)

		tc := ink
		if b.target == page {
			tc = white
		}
		lw := int(canvas.MeasureText(b.label, float64(fs)))
		g.AddChild(formText(b.label, float64(x)+(float64(btnW)-float64(lw))/2, float64(btnY)+(float64(btnH)-float64(lineHeight(fs)))/2, fs, tc))
		geo.buttons = append(geo.buttons, image.Rect(ln.AbsX+x, ln.AbsY+btnY, ln.AbsX+x+btnW, ln.AbsY+btnY+btnH))
		geo.pages = append(geo.pages, b.target)
		x += btnW + 6*scale
	}
	return g
}

func (p *Pagination) HandlePointer(n *model.Node, rt *runtime.Runtime, ev canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if ev.Type != canvas.PointerPress || n.OnPress == nil {
		return false
	}
	geo := p.geo(n)
	for i, b := range geo.buttons {
		if ev.X >= float64(b.Min.X) && ev.X <= float64(b.Max.X) && ev.Y >= float64(b.Min.Y) && ev.Y <= float64(b.Max.Y) {
			if i < len(geo.pages) {
				args := map[string]any{"page": geo.pages[i]}
				for k, v := range n.OnPress.Args {
					args[k] = v
				}
				rt.Dispatch(n.OnPress.Name, args)
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// timeline
// ---------------------------------------------------------------------------
type Timeline struct{}

func (*Timeline) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	items := boundArray(n, rt, "items")
	fs := formFontSize(n, scale)
	for _, it := range items {
		t := runtime.Stringify(it)
		if m, ok := it.(map[string]any); ok {
			if s, ok := m["title"].(string); ok {
				t = s
			}
		}
		if lw := int(canvas.MeasureText(t, float64(fs))) + 40*scale; lw > w {
			w = lw
		}
		h += lineHeight(fs)*2 + 12*scale
	}
	return
}

func (*Timeline) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	items := boundArray(ln.Node, rt, "items")
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	trackC := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	accent := formAccent(rt)

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	y, trackX, dotR := 0, 20*scale, 6*scale
	step := lineHeight(fs)*2 + 12*scale
	for i, it := range items {
		var title, textv, dotColor string
		if m, ok := it.(map[string]any); ok {
			title, _ = m["title"].(string)
			textv, _ = m["text"].(string)
			dotColor, _ = m["color"].(string)
		}
		if title == "" {
			title = runtime.Stringify(it)
		}

		dot := draw.NewCircle()
		dot.X = float64(trackX - dotR)
		dot.Y = float64(y + lineHeight(fs)/2 - dotR)
		dot.Radius = float64(dotR)
		dot.Fill = accent
		if dotColor != "" {
			if c := canvas.ResolveColor(dotColor, rt); c.A > 0 {
				dot.Fill = c
			}
		}
		g.AddChild(dot)
		g.AddChild(formText(title, float64(trackX+16*scale), float64(y), fs, ink))
		subY := y + lineHeight(fs) + 2*scale
		if textv != "" {
			g.AddChild(formText(textv, float64(trackX+16*scale), float64(subY), fs-2*scale, ink2))
		}

		if i < len(items)-1 {
			track := draw.NewRect()
			track.X = float64(trackX - scale)
			track.Y = float64(y + lineHeight(fs)/2 + dotR)
			track.Width, track.Height = float64(2*scale), float64(step-lineHeight(fs)/2-dotR)
			track.Fill = trackC
			g.AddChild(track)
		}
		y += step
	}
	return g
}
