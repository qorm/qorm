//go:build ignore

// Custom canvas widget: "rating" — a five-dot score display registered into
// the native engine by THIS app's Go middle layer. `qorm package` compiles
// this file into the desktop binary (it is injected as userops_gen.go, see
// cmd/qorm/package_native.go), so init() runs before the window opens and
// the scene's {"type": "rating"} resolves through the engine's registry.
//
// The widget composes ONLY the public draw layer (internal/render/draw) —
// the engine internals are never touched.
package main

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("rating", ratingWidget{})
}

// ratingWidget draws five dots; the first `value` are filled with the theme
// accent, the rest are outlined. `value` may be a number or a {{state.x}}
// binding (evaluated per frame, like every other prop in the engine).
type ratingWidget struct{}

func (ratingWidget) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	return 5 * 24 * scale, 24 * scale
}

func (ratingWidget) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	value := 0.0
	if raw, ok := ln.Node.Prop("value"); ok {
		if f, ok := runtime.EvalBinding(
			strings.TrimSpace(fmt.Sprint(raw)),
			map[string]any{"state": rt.State},
		).(float64); ok {
			value = f
		}
	}
	accent := color.RGBA{0x00, 0x7A, 0xFF, 255}
	if rt.Theme != nil {
		if c, ok := rt.Theme.GetColor("primary"); ok {
			accent = c
		}
	}

	size := 24 * scale
	d := float64(16 * scale)
	g := draw.NewGroup()
	for i := 0; i < 5; i++ {
		dot := draw.NewCircle()
		dot.X = float64(i*size + (size-int(d))/2)
		dot.Y = float64((24*scale - int(d)) / 2)
		dot.Radius = d / 2
		if float64(i) < value {
			dot.Fill = accent
		} else {
			dot.Stroke = accent
			dot.StrokeWidth = float64(scale)
		}
		g.AddChild(dot)
	}
	return g
}
