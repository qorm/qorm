package widgets

// The slider (HTML: render_input.go:363 — <input type=range>): a track with
// an accent filled portion and a draggable thumb over min/max/step.
//
// Interaction mirrors the browser's gesture: press takes the pointer capture
// (inter.Pressed = n, so the whole stream routes here until release), move
// follows the pointer while captured, release drops the capture; a click on
// the track seeks immediately, while a click on the thumb preserves the
// grab offset until movement begins. The state write-back is CONTINUOUS
// (every update, like the browser's input event feeding the binding) while
// onChange dispatches once per gesture at release with the final value — the
// HTML change-event semantics (render_input.go changeWiring is the native
// onchange).

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	canvas.RegisterWidget("slider", &Slider{
		local:       map[*model.Node]float64{},
		geoms:       map[*model.Node]sliderGeom{},
		dragged:     map[*model.Node]bool{},
		grabOffsets: map[*model.Node]float64{},
		grabbed:     map[*model.Node]bool{},
	})
}

// Slider is the continuous value control. The thumb position is the value
// fraction between min and max; the value snaps to step and clamps into
// [min, max] before every write-back.
type Slider struct {
	mu sync.Mutex
	// local holds UNBOUND sliders' values (see Checkbox.local).
	local map[*model.Node]float64
	// geoms is the last laid-out geometry per node (absolute physical px),
	// stashed every Record so HandlePointer can map an X to a value fraction.
	geoms map[*model.Node]sliderGeom
	// dragged marks nodes whose current press-move-release gesture actually
	// changed the value — onChange fires at release only for those (the
	// native range control fires change only when the value moved).
	dragged map[*model.Node]bool
	// grabOffsets preserves the point where the pointer grabbed the thumb. A
	// press on the thumb must not jump its value to the pointer's exact X; the
	// thumb follows subsequent movement while keeping that offset.
	grabOffsets map[*model.Node]float64
	// grabbed distinguishes a thumb drag from a track click. Track presses
	// retain the browser behavior of jumping immediately to the clicked value.
	grabbed map[*model.Node]bool
}

// sliderGeom is one slider's on-screen box plus its physical thumb radius
// (the thumb center travels [thumbR, W-thumbR] of the box width).
type sliderGeom struct {
	box    image.Rectangle
	thumbR float64
}

const (
	sliderMinW   = 160
	sliderBoxH   = 24
	sliderTrackH = 4
	sliderThumbD = 16
)

// Measure reports a 160x24 default box (an explicit style width/height
// overrides through the generic sizing, as with every widget).
func (s *Slider) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return sliderMinW * scale, sliderBoxH * scale
}

// Record draws the track (filled portion in accent up to the thumb) and the
// thumb at the value fraction, and stashes the box for pointer mapping.
func (s *Slider) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	thumbR := float64(sliderThumbD * scale / 2)
	s.mu.Lock()
	s.geoms[ln.Node] = sliderGeom{
		box:    image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height),
		thumbR: thumbR,
	}
	s.mu.Unlock()
	trackH := float64(sliderTrackH * scale)
	trackY := (float64(ln.Height) - trackH) / 2
	cx := s.thumbCenter(ln.Node, rt, float64(ln.Width), thumbR)

	g := draw.NewGroup()
	track := draw.NewRect()
	track.Y = trackY
	track.Width = float64(ln.Width)
	track.Height = trackH
	track.BorderRadius = trackH / 2
	track.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	g.AddChild(track)

	if cx > 0 {
		fill := draw.NewRect()
		fill.NoHit = true
		fill.Y = trackY
		fill.Width = cx
		fill.Height = trackH
		fill.BorderRadius = trackH / 2
		fill.Fill = formAccent(rt)
		g.AddChild(fill)
	}

	thumb := draw.NewCircle()
	thumb.X = cx - thumbR
	thumb.Y = float64(ln.Height)/2 - thumbR
	thumb.Radius = thumbR
	thumb.Fill = color.RGBA{255, 255, 255, 255}
	thumb.Stroke = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	thumb.StrokeWidth = float64(scale)
	g.AddChild(thumb)
	return g
}

// HandlePointer implements the press-drag-release gesture with capture.
func (s *Slider) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	if formDisabled(n, rt) {
		return false
	}
	switch p.Type {
	case canvas.PointerPress:
		inter.Pressed = n // take the capture: the drag streams here until release
		inter.Focused = n
		inter.FocusVisible = false
		// A fresh gesture starts with no stale capture metadata (for example,
		// when a host drops a release while the window is being deactivated).
		s.mu.Lock()
		delete(s.grabOffsets, n)
		delete(s.grabbed, n)
		delete(s.dragged, n)
		s.mu.Unlock()
		if s.grabThumb(n, rt, p.X) {
			// Keep the current value on thumb press. The first move applies the
			// pointer delta, avoiding the visible jump caused by an off-center
			// click on the thumb.
			return true
		}
		// A press on the track is a direct seek, matching native range inputs.
		s.updateFromX(n, rt, p.X)
		return true
	case canvas.PointerMove:
		if inter.Pressed == n {
			s.updateFromPointer(n, rt, p.X)
			return true
		}
		return false
	case canvas.PointerRelease:
		if inter.Pressed != n {
			return false
		}
		s.updateFromPointer(n, rt, p.X)
		s.mu.Lock()
		moved := s.dragged[n]
		delete(s.dragged, n)
		delete(s.grabOffsets, n)
		delete(s.grabbed, n)
		s.mu.Unlock()
		if moved {
			// Native range parity: change fires once per gesture, at release.
			commitFormChange(n, rt, s.value(n, nil, rt))
		}
		return true
	}
	return false
}

// grabThumb starts a thumb drag when the press lands on the thumb. It records
// the pointer's signed offset from the thumb center so a click on the thumb's
// edge does not snap the value before the pointer has moved.
func (s *Slider) grabThumb(n *model.Node, rt *runtime.Runtime, x float64) bool {
	s.mu.Lock()
	geo, ok := s.geoms[n]
	s.mu.Unlock()
	if !ok {
		return false
	}
	center := float64(geo.box.Min.X) + s.thumbCenter(n, rt, float64(geo.box.Dx()), geo.thumbR)
	if math.Abs(x-center) > geo.thumbR {
		return false
	}
	s.mu.Lock()
	s.grabOffsets[n] = x - center
	s.grabbed[n] = true
	s.mu.Unlock()
	return true
}

// updateFromPointer applies a captured pointer event. Thumb drags preserve the
// original grab offset; track presses use the pointer X directly.
func (s *Slider) updateFromPointer(n *model.Node, rt *runtime.Runtime, x float64) {
	s.mu.Lock()
	offset, grabbed := s.grabOffsets[n], s.grabbed[n]
	s.mu.Unlock()
	if grabbed {
		x -= offset
	}
	s.updateFromX(n, rt, x)
}

// updateFromX maps an absolute pointer X onto the value range and commits the
// write-back (bound state path or the uncontrolled store), snapped to step
// and clamped into [min, max]. The thumb center travels [thumbR, W-thumbR] of
// the stashed box, the same mapping Record draws with.
func (s *Slider) updateFromX(n *model.Node, rt *runtime.Runtime, x float64) {
	s.mu.Lock()
	geo, ok := s.geoms[n]
	s.mu.Unlock()
	if !ok {
		return
	}
	min, max, step := sliderRange(n)
	travel := float64(geo.box.Dx()) - 2*geo.thumbR
	if travel <= 0 {
		travel = float64(geo.box.Dx())
	}
	frac := (x - float64(geo.box.Min.X) - geo.thumbR) / travel
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	v := min + frac*(max-min)
	if step > 0 {
		v = min + math.Round((v-min)/step)*step
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	if v == s.value(n, nil, rt) {
		return
	}
	if path := formBoundPath(n.Value); path != "" {
		rt.SetStatePath(path, v)
	} else {
		s.mu.Lock()
		s.local[n] = v
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.dragged[n] = true
	s.mu.Unlock()
}

// thumbCenter converts the value fraction to the thumb's center X within a
// box of width w (the thumb stays fully inside the track, as in browsers).
func (s *Slider) thumbCenter(n *model.Node, rt *runtime.Runtime, w, thumbR float64) float64 {
	min, max, _ := sliderRange(n)
	frac := 0.0
	if max > min {
		frac = (s.value(n, nil, rt) - min) / (max - min)
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	travel := w - 2*thumbR
	if travel < 0 {
		travel = 0
	}
	return thumbR + frac*travel
}

// value resolves the current value: the binding, else the uncontrolled store,
// else the literal value prop (HTML evaluates the value unconditionally),
// clamped into [min, max] for display like the browser does.
func (s *Slider) value(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) float64 {
	min, max, _ := sliderRange(n)
	v := min
	if formBoundPath(n.Value) != "" {
		v = formFloat(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
	} else {
		s.mu.Lock()
		lv, ok := s.local[n]
		s.mu.Unlock()
		if ok {
			v = lv
		} else if n.Value != "" {
			v = formFloat(runtime.EvalBinding(n.Value, formCtxLn(rt, ln)))
		}
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v
}

// sliderRange reads min/max/step (HTML: propNum defaults 0/100/1).
func sliderRange(n *model.Node) (min, max, step float64) {
	min, max, step = 0, 100, 1
	if v, ok := n.Prop("min"); ok {
		min = formFloat(v)
	}
	if v, ok := n.Prop("max"); ok {
		max = formFloat(v)
	}
	if v, ok := n.Prop("step"); ok {
		step = formFloat(v)
	}
	return
}
