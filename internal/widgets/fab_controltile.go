package widgets

// FAB and SwitchListTile canvas ports (HTML fab / controlTile).

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/anim"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("fab", &Fab{})
	canvas.RegisterWidget("floatingactionbutton", &Fab{})
	canvas.RegisterWidget("switchlisttile", &SwitchListTile{
		local:  map[*model.Node]bool{},
		slides: map[*model.Node]*switchSlide{},
	})
}

// ---- fab --------------------------------------------------------------------

// Fab is a circular (or extended pill) primary action button.
type Fab struct{}

func (Fab) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	extended := formPropStr(n, "extended", rt) == "true"
	if extended {
		label := formLabel(n, rt)
		if label == "" {
			label = "+"
		}
		fs := 15 * scale
		return int(canvas.MeasureText(label, float64(fs))) + 40*scale, 48 * scale
	}
	return 56 * scale, 56 * scale
}

func (Fab) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	label := formLabel(ln.Node, rt)
	if label == "" {
		label = "+"
	}
	extended := formPropStr(ln.Node, "extended", rt) == "true"
	accent := formAccent(rt)
	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)
	bg := draw.NewRect()
	bg.Width, bg.Height = float64(ln.Width), float64(ln.Height)
	if extended {
		bg.BorderRadius = float64(ln.Height) / 2
	} else {
		bg.BorderRadius = float64(ln.Width) / 2
	}
	bg.Fill = accent
	bg.ShadowColor = color.RGBA{0, 0, 0, 46}
	bg.ShadowBlur = float64(16 * scale)
	bg.ShadowY = float64(6 * scale)
	g.AddChild(bg)
	fs := 24 * scale
	if extended {
		fs = 15 * scale
	}
	tw := int(canvas.MeasureText(label, float64(fs)))
	tx := (float64(ln.Width) - float64(tw)) / 2
	ty := (float64(ln.Height) - float64(lineHeight(fs))) / 2
	g.AddChild(formText(label, tx, ty, fs, color.RGBA{255, 255, 255, 255}))
	return g
}

func (Fab) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	if n.OnPress != nil {
		dispatchInvoke(n.OnPress, rt)
		return true
	}
	return false
}

func (Fab) Inline() {}

// ---- switchlisttile ---------------------------------------------------------

// SwitchListTile is a full-width list row with title/subtitle and a trailing
// switch (HTML controlTile switch branch). Toggle state reuses the same
// binding conventions as Switch.
type SwitchListTile struct {
	mu     sync.Mutex
	local  map[*model.Node]bool
	slides map[*model.Node]*switchSlide
}

func (s *SwitchListTile) slide(n *model.Node) *switchSlide {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl := s.slides[n]
	if sl == nil {
		sl = &switchSlide{}
		s.slides[n] = sl
	}
	return sl
}

func (s *SwitchListTile) Animating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sl := range s.slides {
		if sl.running {
			return true
		}
	}
	return false
}

func (s *SwitchListTile) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	h = 16*scale + lineHeight(fs)
	if formPropStr(n, "subtitle", rt) != "" {
		h += lineHeight(fs - 2*scale)
	}
	if h < switchTrackH*scale+16*scale {
		h = switchTrackH*scale + 16*scale
	}
	return 300 * scale, h
}

func (s *SwitchListTile) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	fs := formFontSizeLN(ln, scale)
	ink := formInk(ln.Node, ln, rt)
	title := formLabel(ln.Node, rt)
	if title == "" {
		title = "..."
	}
	sub := formPropStr(ln.Node, "subtitle", rt)
	on := s.checked(ln.Node, ln, rt)

	g := draw.NewGroup()
	g.Width, g.Height = float64(ln.Width), float64(ln.Height)

	// Title + subtitle on the left.
	x := float64(14 * scale)
	ty := float64(10 * scale)
	g.AddChild(formText(title, x, ty, fs, ink))
	if sub != "" {
		g.AddChild(formText(sub, x, ty+float64(lineHeight(fs)), fs-2*scale, themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})))
	}

	// Trailing switch track (mirrors Switch.Record).
	trackW := float64(switchTrackW * scale)
	trackH := float64(switchTrackH * scale)
	trackX := float64(ln.Width) - trackW - float64(14*scale)
	trackY := (float64(ln.Height) - trackH) / 2
	sl := s.slide(ln.Node)
	if sl.ctrl == nil {
		if on {
			sl.progress = 1
		} else {
			sl.progress = 0
		}
	} else {
		if v, running := sl.ctrl.Value(); running {
			sl.progress = sl.progress + (sl.target-sl.progress)*v
			sl.running = true
		} else {
			sl.progress = sl.target
			sl.running = false
			sl.ctrl = nil
		}
	}
	track := draw.NewRect()
	track.X, track.Y = trackX, trackY
	track.Width, track.Height = trackW, trackH
	track.BorderRadius = trackH / 2
	if on {
		track.Fill = formAccent(rt)
	} else {
		track.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	}
	g.AddChild(track)
	thumbD := float64(switchThumbD * scale)
	inset := float64(2 * scale)
	thumb := draw.NewCircle()
	thumb.Radius = thumbD / 2
	thumb.Fill = color.RGBA{255, 255, 255, 255}
	thumb.X = trackX + inset + (trackW-thumbD-2*inset)*sl.progress
	thumb.Y = trackY + inset
	g.AddChild(thumb)
	return g
}

func (s *SwitchListTile) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) || p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	next := !s.checked(n, nil, rt)
	target := 0.0
	if next {
		target = 1
	}
	sl := s.slide(n)
	sl.target = target
	sl.ctrl = anim.NewController(switchSlideDuration, anim.EaseOutCubic)
	sl.running = true
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, next)
	} else {
		s.mu.Lock()
		s.local[n] = next
		s.mu.Unlock()
	}
	commitFormChange(n, rt, next)
	return true
}

func (s *SwitchListTile) checked(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) bool {
	if formBoundPath(n.Value) != "" {
		return formTruthy(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	}
	s.mu.Lock()
	lv, ok := s.local[n]
	s.mu.Unlock()
	if ok {
		return lv
	}
	if v, ok := n.Prop("checked"); ok {
		return formTruthy(runtime.EvalBinding(fmt.Sprint(v), formCtxLn(rt, ln)))
	}
	return false
}
