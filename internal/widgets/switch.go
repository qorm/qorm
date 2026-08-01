package widgets

// The switch is the pill-track boolean control (HTML: the "switch" branch of
// render_input.go:309 — an iOS-style toggle over the same checked binding as
// checkbox).

import (
	"image"
	"fmt"
	"image/color"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("switch", &Switch{local: map[*model.Node]bool{}})
}

// Switch is the pill-track toggle: a 44x26 rounded track (accent while on,
// input gray while off) with a white circular thumb at the on/off end, plus
// an optional label to the right. The thumb SNAPS between ends: a slide tween
// needs a time-driven frame source per toggle (AnimatedWidget only drives
// never-settling animations), which wave-1 deliberately skips — documented
// downgrade.
type Switch struct {
	mu sync.Mutex
	// local holds UNBOUND switches (see Checkbox.local).
	local map[*model.Node]bool
}

const (
	switchTrackW = 44
	switchTrackH = 26
	switchThumbD = 22
)

// Measure reports the track plus label (HTML: label with gap:10px).
func (s *Switch) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	w, h = switchTrackW*scale, max(switchTrackH*scale, lineHeight(fs))
	if label := formLabel(n, rt); label != "" {
		w += 10*scale + int(canvas.MeasureText(label, float64(fs)))
	}
	return
}

// Record draws the track and the thumb at the end matching the state.
func (s *Switch) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	on := s.checked(ln.Node, rt)
	trackW := float64(switchTrackW * scale)
	trackH := float64(switchTrackH * scale)
	trackY := (float64(ln.Height) - trackH) / 2

	g := draw.NewGroup()
	track := draw.NewRect()
	track.Y = trackY
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
	thumb.Y = trackY + inset
	if on {
		thumb.X = trackW - thumbD - inset
	} else {
		thumb.X = inset
	}
	g.AddChild(thumb)

	if label := formLabel(ln.Node, rt); label != "" {
		fs := formFontSizeLN(ln, scale)
		g.AddChild(formText(label, trackW+float64(10*scale), (float64(ln.Height)-float64(lineHeight(fs)))/2, fs, formInk(ln.Node, ln, rt)))
	}
	return g
}

// HandlePointer flips on press — the same write-back + onChange pair as
// Checkbox (HTML renders both over a <input type=checkbox> core).
func (s *Switch) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	next := !s.checked(n, rt)
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

// checked resolves exactly like Checkbox.checked (binding, then the local
// toggle, then the `checked` prop as the initial state).
func (s *Switch) checked(n *model.Node, rt *runtime.Runtime) bool {
	if formBoundPath(n.Value) != "" {
		return formTruthy(runtime.EvalBinding(n.Value, formCtx(rt)))
	}
	s.mu.Lock()
	lv, ok := s.local[n]
	s.mu.Unlock()
	if ok {
		return lv
	}
	if v, ok := n.Prop("checked"); ok {
		return formTruthy(runtime.EvalBinding(fmt.Sprint(v), formCtx(rt)))
	}
	return false
}
