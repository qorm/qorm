package widgets

// The switch is the pill-track boolean control (HTML: the "switch" branch of
// render_input.go:309 — an iOS-style toggle over the same checked binding as
// checkbox).

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/qorm/platform/internal/anim"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("switch", &Switch{local: map[*model.Node]bool{}, slides: map[*model.Node]*switchSlide{}})
}

// Switch is the pill-track toggle: a 44x26 rounded track (accent while on,
// input gray while off) with a white circular thumb at the on/off end, plus
// an optional label to the right. The thumb SLIDES between ends on toggle (a
// 150ms ease-out tween); the switch is an AnimatedWidget so the engine keeps
// ticking until the slide settles.
type Switch struct {
	mu sync.Mutex
	// local holds UNBOUND switches (see Checkbox.local).
	local map[*model.Node]bool
	// slides is the per-node thumb tween (progress 0..1 along the track).
	slides map[*model.Node]*switchSlide
}

// switchSlide is one switch's thumb tween: progress is the CURRENT position in
// [0,1], target the destination, and ctrl the running controller (nil when
// settled — the thumb is pinned to the state).
type switchSlide struct {
	progress float64
	target   float64
	running  bool
	ctrl     *anim.Controller
}

// switchSlideDuration is how long the thumb takes to travel between ends.
const switchSlideDuration = 150 * time.Millisecond

func (s *Switch) slide(n *model.Node) *switchSlide {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl := s.slides[n]
	if sl == nil {
		sl = &switchSlide{}
		s.slides[n] = sl
	}
	return sl
}

// Animating reports true while any switch's thumb tween is mid-flight — the
// AnimatedWidget seam that keeps the engine's frame loop ticking.
func (s *Switch) Animating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sl := range s.slides {
		if sl.running {
			return true
		}
	}
	return false
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
	on := s.checked(ln.Node, ln, rt)
	trackW := float64(switchTrackW * scale)
	trackH := float64(switchTrackH * scale)
	trackY := (float64(ln.Height) - trackH) / 2

	// Advance the thumb tween: on first sight the thumb pins to the state (no
	// slide-in on mount); a toggle armed a controller in HandlePointer, whose
	// progress is eased toward its target each frame until it settles.
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
	// The thumb travels between inset and the on-end across the slide progress.
	thumb.X = inset + (trackW-thumbD-2*inset)*sl.progress
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
	next := !s.checked(n, nil, rt)
	// Arm the thumb slide from the current position toward the new state.
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

// checked resolves exactly like Checkbox.checked (binding, then the local
// toggle, then the `checked` prop as the initial state).
func (s *Switch) checked(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) bool {
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

// Inline marks Switch as inline-level (canvas.InlineWidget): flex containers keep its content size.
func (*Switch) Inline() {}
