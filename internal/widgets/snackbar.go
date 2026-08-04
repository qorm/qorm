package widgets

// The snackbar (HTML: render_feedback.go:14) — a transient message bar fixed
// at the bottom center: the label (the `label`/`text` prop) and an optional
// action label (`action` prop). The `open` prop (a binding) controls
// visibility; tapping the bar dispatches the node's onPress. Auto-dismiss
// after a duration is a later milestone — the author flips `open` off.

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
	canvas.RegisterWidget("snackbar", &Snackbar{geoms: map[*model.Node]*snackGeo{}})
}

// Snackbar is the bottom message bar overlay.
type Snackbar struct {
	mu    sync.Mutex
	geoms map[*model.Node]*snackGeo
}

type snackGeo struct {
	bar image.Rectangle
}

func (s *Snackbar) geo(n *model.Node) *snackGeo {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.geoms[n]
	if g == nil {
		g = &snackGeo{}
		s.geoms[n] = g
	}
	return g
}

func (*Snackbar) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	return 0, 0 // an overlay: no in-flow size
}

func (s *Snackbar) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	return nil // the overlay IS the snackbar
}

// OverlayOpen reports whether the bar is mounted (the `open` prop).
func (s *Snackbar) OverlayOpen(n *model.Node, rt *runtime.Runtime) bool {
	raw, ok := n.Prop("open")
	if !ok {
		return false
	}
	return formTruthy(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, nil)))
}

// OverlayRecord draws the bottom bar with the label and the action.
func (s *Snackbar) OverlayRecord(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) draw.Node {
	if ln == nil || !s.OverlayOpen(ln.Node, rt) {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	stageW, stageH := overlayStageSize(rt, scale, 0, 0)
	barW := ln.Width
	if barW < 200*scale {
		barW = 200 * scale
	}
	if barW > stageW-20*scale {
		barW = stageW - 20 * scale
	}
	barH := 48 * scale
	barX := (stageW - barW) / 2
	barY := stageH - barH - 20*scale
	s.mu.Lock()
	s.geoms[ln.Node] = &snackGeo{bar: image.Rect(barX, barY, barX+barW, barY+barH)}
	s.mu.Unlock()

	g := draw.NewGroup()
	g.Width = float64(stageW)
	g.Height = float64(stageH)
	g.Model = ln.Node
	g.Overlay = true

	bar := draw.NewRect()
	bar.X = float64(barX)
	bar.Y = float64(barY)
	bar.Width = float64(barW)
	bar.Height = float64(barH)
	bar.BorderRadius = 8 * float64(scale)
	bar.Fill = color.RGBA{50, 50, 50, 255}
	bar.ShadowColor = color.RGBA{0, 0, 0, 70}
	bar.ShadowBlur = 12 * float64(scale)
	bar.ShadowY = 4 * float64(scale)
	g.AddChild(bar)

	fs := 14 * scale
	label := formLabel(ln.Node, rt)
	if label == "" {
		label = "..."
	}
	tx := barX + 16*scale
	act := ""
	if raw, ok := ln.Node.Prop("action"); ok {
		act = formEvalStr(fmt.Sprint(raw), rt)
	}
	if act != "" {
		aw := int(canvas.MeasureText(act, float64(fs))) + 12*scale
		g.AddChild(formText(act, float64(barX+barW-16*scale-aw), float64(barY)+(float64(barH)-float64(lineHeight(fs)))/2, fs, color.RGBA{124, 192, 255, 255}))
	}
	g.AddChild(formText(label, float64(tx), float64(barY)+(float64(barH)-float64(lineHeight(fs)))/2, fs, color.RGBA{255, 255, 255, 255}))
	return g
}

// HandlePointer dispatches the node's onPress on a bar tap (the action or the
// body — v1 treats the whole bar as the action surface).
func (s *Snackbar) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress || n.OnPress == nil {
		return false
	}
	g := s.geo(n)
	if p.X < float64(g.bar.Min.X) || p.X >= float64(g.bar.Max.X) ||
		p.Y < float64(g.bar.Min.Y) || p.Y >= float64(g.bar.Max.Y) {
		return false
	}
	argAny := make(map[string]any, len(n.OnPress.Args))
	for k, v := range n.OnPress.Args {
		argAny[k] = v
	}
	rt.Dispatch(n.OnPress.Name, argAny)
	return true
}
