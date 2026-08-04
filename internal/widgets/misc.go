package widgets

import (
	"image"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("backbutton", &BackButton{})
	canvas.RegisterWidget("closebutton", &CloseButton{})
	canvas.RegisterWidget("empty", &Empty{})
	canvas.RegisterWidget("segmented", &Segmented{geoms: map[*model.Node]*segmentedGeo{}})
	canvas.RegisterWidget("slidingsegmentedcontrol", &Segmented{geoms: map[*model.Node]*segmentedGeo{}})
	canvas.RegisterWidget("cupertinoslidingsegmentedcontrol", &Segmented{geoms: map[*model.Node]*segmentedGeo{}})
}

// backButton / closeButton
type BackButton struct{}
type CloseButton struct{}

func (*BackButton) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 { scale = 1 }
	return 44 * scale, 44 * scale
}
func (*CloseButton) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 { scale = 1 }
	return 44 * scale, 44 * scale
}

func (*BackButton) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return navBtnRecord(ln, rt, scale, "←")
}
func (*CloseButton) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return navBtnRecord(ln, rt, scale, "✕")
}

func navBtnRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, glyph string) draw.Node {
	if scale < 1 { scale = 1 }
	if ln.Width <= 0 || ln.Height <= 0 { return nil }
	fs := 22 * scale
	ink := formInk(ln.Node, ln, rt)
	g := draw.NewGroup(); g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	tw := int(canvas.MeasureText(glyph, float64(fs)))
	g.AddChild(formText(glyph, (float64(ln.Width)-float64(tw))/2, (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, ink))
	return g
}

// HandlePointer: dispatch onPress, else navigate back (no back yet — just a no-op).
func (*BackButton) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if p.Type != canvas.PointerPress { return false }
	if n.OnPress != nil { dispatchInvoke(n.OnPress, rt); return true }
	return false
}
func (*CloseButton) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if p.Type != canvas.PointerPress { return false }
	if n.OnPress != nil { dispatchInvoke(n.OnPress, rt); return true }
	return false
}

// ---------------------------------------------------------------------------
// empty
// ---------------------------------------------------------------------------
type Empty struct{}

func (*Empty) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 { scale = 1 }
	label := formLabel(n, rt)
	var title string
	if v, ok := n.Prop("title"); ok { if s, ok := v.(string); ok { title = s } }
	fs := formFontSize(n, scale)
	w = int(canvas.MeasureText(label, float64(fs))) + 40*scale
	if t := int(canvas.MeasureText(title, float64(fs))); t > w { w = t }
	h = 80*scale + lineHeight(fs)*2 + 32*scale
	return
}

func (*Empty) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 { scale = 1 }
	if ln.Width <= 0 || ln.Height <= 0 { return nil }
	label := formLabel(ln.Node, rt)
	var title string
	if v, ok := ln.Node.Prop("title"); ok { if s, ok := v.(string); ok { title = s } }
	fs := formFontSizeLN(ln, scale)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})

	g := draw.NewGroup(); g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	// Icon placeholder (a circle with a line, simple "empty" glyph).
	iconX := (ln.Width - 40*scale) / 2
	iconRect := draw.NewRect()
	iconRect.X, iconRect.Y = float64(iconX), float64(16*scale)
	iconRect.Width, iconRect.Height = float64(40*scale), float64(40*scale)
	iconRect.BorderRadius = 20 * float64(scale)
	iconRect.Stroke, iconRect.StrokeWidth = ink2, float64(scale)
	g.AddChild(iconRect)
	tt := int(canvas.MeasureText("—", float64(20*scale)))
	g.AddChild(formText("—", float64(iconX)+(40*float64(scale)-float64(tt))/2, float64(16*scale)+10*float64(scale), 20*scale, ink2))

	y := 64 * scale
	if title != "" {
		tw := int(canvas.MeasureText(title, float64(fs)))
		g.AddChild(formText(title, float64((ln.Width-tw)/2), float64(y), fs, ink2))
		y += lineHeight(fs) + 6*scale
	}
	if label != "" {
		lw := int(canvas.MeasureText(label, float64(fs-2*scale)))
		g.AddChild(formText(label, float64((ln.Width-lw)/2), float64(y), fs-2*scale, ink2))
	}
	return g
}

// ---------------------------------------------------------------------------
// segmented
// ---------------------------------------------------------------------------
type Segmented struct{ geoms map[*model.Node]*segmentedGeo }
type segmentedGeo struct{ rects []image.Rectangle }

func (s *Segmented) geo(n *model.Node) *segmentedGeo {
	g, ok := s.geoms[n]
	if !ok { g = &segmentedGeo{}; s.geoms[n] = g }
	return g
}

func (*Segmented) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 { scale = 1 }
	opts := optionList(n.Props["options"]); fs := formFontSize(n, scale)
	for _, o := range opts {
		w += int(canvas.MeasureText(o.label, float64(fs))) + 30*scale
	}
	w += (len(opts) - 1) * 2 * scale + 6*scale
	h = lineHeight(fs) + 20*scale; return
}

func (s *Segmented) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 { scale = 1 }
	if ln.Width <= 0 || ln.Height <= 0 { return nil }
	opts := optionList(ln.Node.Props["options"])
	cur := ln.Node.Value
	fs := formFontSizeLN(ln, scale)
	accent := formAccent(rt); white := color.RGBA{255, 255, 255, 255}
	bg := themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})

	g := draw.NewGroup(); g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	track := draw.NewRect()
	track.Width, track.Height = float64(ln.Width), float64(ln.Height)
	track.BorderRadius = 8 * float64(scale); track.Fill = bg; g.AddChild(track)

	geo := s.geo(ln.Node); geo.rects = geo.rects[:0]
	x := 3 * scale
	for _, opt := range opts {
		lw := int(canvas.MeasureText(opt.label, float64(fs))) + 28*scale
		sel := opt.value == cur
		if sel {
			hl := draw.NewRect()
			hl.X, hl.Y = float64(x), float64(3*scale)
			hl.Width, hl.Height = float64(lw), float64(ln.Height-6*scale)
			hl.BorderRadius = 6 * float64(scale); hl.Fill = accent; g.AddChild(hl)
		}
		tc := ink2; if sel { tc = white }
		g.AddChild(formText(opt.label, float64(x+14*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, tc))
		geo.rects = append(geo.rects, image.Rect(ln.AbsX+x, ln.AbsY, ln.AbsX+x+lw, ln.AbsY+ln.Height))
		x += lw + 2*scale
	}
	return g
}

func (s *Segmented) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress { return false }
	opts := optionList(n.Props["options"])
	geo := s.geo(n)
	for i, r := range geo.rects {
		if p.X >= float64(r.Min.X) && p.X <= float64(r.Max.X) && p.Y >= float64(r.Min.Y) && p.Y <= float64(r.Max.Y) {
			if i < len(opts) {
				if path := formBoundPath(n.Value); path != "" {
					rt.SetStatePath(path, opts[i].value)
				}
				if n.OnChange != nil {
					args := map[string]any{"value": opts[i].value}
					for k, v := range n.OnChange.Args { args[k] = v }
					rt.Dispatch(n.OnChange.Name, args)
				}
				return true
			}
		}
	}
	return false
}

type option struct{ value, label string }

func optionList(v any) []option {
	var out []option
	arr, ok := v.([]any)
	if !ok { return out }
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			val := stringifyAny(m["value"]); lbl := stringifyAny(m["label"])
			if val == "" { val = lbl }; out = append(out, option{val, lbl})
		} else if s, ok := it.(string); ok {
			out = append(out, option{s, s})
		}
	}
	return out
}

func stringifyAny(v any) string {
	if s, ok := v.(string); ok { return s }
	return runtime.Stringify(v)
}

// ---------------------------------------------------------------------------
// navigationrail
// ---------------------------------------------------------------------------
type NavigationRail struct{ geoms map[*model.Node][]image.Rectangle }

func init() {
	canvas.RegisterWidget("navigationrail", &NavigationRail{geoms: map[*model.Node][]image.Rectangle{}})
}

func (*NavigationRail) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 { scale = 1 }
	items := boundArray(n, rt, "items"); fs := 11 * scale
	w = 72 * scale
	for _, it := range items {
		obj, _ := it.(map[string]any)
		if obj == nil { continue }
		lbl := stringifyAny(obj["label"])
		if lw := int(canvas.MeasureText(lbl, float64(fs))) + 16*scale; lw > w { w = lw }
		h += 12*scale + 20*scale + lineHeight(fs) + 4*scale + 12*scale
	}
	return w, h + 12*scale
}

func (r *NavigationRail) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 { scale = 1 }
	if ln.Width <= 0 || ln.Height <= 0 { return nil }
	items := boundArray(ln.Node, rt, "items")
	cur := ln.Node.Value
	fs := 11 * scale; iconFS := 20 * scale
	accent := formAccent(rt)
	ink2 := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	surface := themeColor(rt, "surface", color.RGBA{255, 255, 255, 255})

	g := draw.NewGroup(); g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	bg := draw.NewRect(); bg.Width, bg.Height = float64(ln.Width), float64(ln.Height)
	bg.Fill = surface; g.AddChild(bg)

	geo := &[]image.Rectangle{}
	r.geoms[ln.Node] = *geo
	y := 12 * scale
	for _, it := range items {
		obj, _ := it.(map[string]any)
		if obj == nil { continue }
		val := stringifyAny(obj["value"]); active := val == cur
		lbl := stringifyAny(obj["label"])
		icon := stringifyAny(obj["icon"])

		bgC := surface
		if active {
			bgC = accent; c := bgC; c.A = 30; bgC = c
		}
		btn := draw.NewRect()
		btn.X = float64(8 * scale); btn.Y = float64(y)
		btn.Width = float64(ln.Width - 16*scale); btn.Height = float64(lineHeight(fs) + 20*scale + 16*scale)
		btn.BorderRadius = 10 * float64(scale); btn.Fill = bgC; g.AddChild(btn)

		ic := ink2; if active { ic = accent }
		if icon != "" {
			tw := int(canvas.MeasureText(icon, float64(iconFS)))
			g.AddChild(formText(icon, float64((ln.Width-tw)/2), float64(y+12*scale), iconFS, ic))
		}
		lw := int(canvas.MeasureText(lbl, float64(fs)))
		tc := ink2; if active { tc = accent }
		g.AddChild(formText(lbl, float64((ln.Width-lw)/2), float64(y+12*scale+20*scale+4*scale), fs, tc))

		rowH := 12*scale + 20*scale + 4*scale + lineHeight(fs) + 12*scale
		r.geoms[ln.Node] = append(r.geoms[ln.Node], image.Rect(ln.AbsX, ln.AbsY+y, ln.AbsX+ln.Width, ln.AbsY+y+rowH))
		y += rowH
	}
	return g
}

func (r *NavigationRail) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || n.OnChange == nil { return false }
	items := boundArray(n, rt, "items")
	rects := r.geoms[n]
	var vi int
	for i, rect := range rects {
		if p.X >= float64(rect.Min.X) && p.X <= float64(rect.Max.X) && p.Y >= float64(rect.Min.Y) && p.Y <= float64(rect.Max.Y) {
			vi = i; break
		}
	}
	if vi < len(items) {
		obj, _ := items[vi].(map[string]any)
		if obj != nil {
			val := stringifyAny(obj["value"])
			if path := formBoundPath(n.Value); path != "" { rt.SetStatePath(path, val) }
			args := map[string]any{"value": val}
			for k, v := range n.OnChange.Args { args[k] = v }
			rt.Dispatch(n.OnChange.Name, args); return true
		}
	}
	return false
}
