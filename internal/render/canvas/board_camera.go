package canvas

import (
	"image"
	"math"
	"strconv"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// applyBoardCamera implements the `cameraTarget` / `cameraCenter` props on a
// `board` root. A side-scroller camera (mario, metroid, sonic, ...) is just
// "follow this object"; the previous workflow forced the app to write
// canvas-internal inter.Board.PanX from a qscript action and re-enter the
// engine each frame, which is fragile and out-of-band. These props let a
// JSON author declare the follow target once and have the engine resolve it
// every layout pass.
//
// Engine-level defaults are derived from the physical screen size, NOT
// from any particular game's tile size — a metroid with 16-px cells, a
// mario with 32-px cells, and a sonic with 48-px cells all get a working
// camera by passing `cameraCell: <px>` and (optionally) `cameraCenter: true`.
//
// Props:
//   - cameraTarget    : {{state.player}}    // an object {x, y} in PHYSICAL px
//                                            // (the same coordinate space the
//                                            // rest of the app uses — the
//                                            // physics step is px-based, and
//                                            // so is everything a tile list
//                                            // outputs).
//   - cameraCenter    : true                // center the target on the
//                                            // viewport (default: align target
//                                            // with the top-left, PanX = -x)
//   - cameraCell      : (deprecated, no-op) // kept for back-compat. The
//                                            // camera contract is px-in,
//                                            // px-out — apps whose state is
//                                            // in world units should
//                                            // pre-multiply in qscript.
//   - cameraViewport  : 16                  // viewport width in PHYSICAL px
//                                            // (used by cameraCenter only);
//                                            // the engine still renders
//                                            // whatever fits in the
//                                            // surface bounds either way.
//
// The user's drag-to-pan still wins while a pan is in flight: a manual pan
// is the user taking control, and the engine stops following until the
// drag ends. Outside of an active pan, the camera re-engages every frame.
//
// The target is read each layout pass because the props may bind state, the
// same model the timer schedule uses. Evaluation is forgiving: a missing or
// stale target (player already dead) leaves the camera at its last
// position instead of snapping to (0, 0).
func applyBoardCamera(n *model.Node, rt *runtime.Runtime, inter *Interaction, screen image.Point, scale int) {
	if n == nil || n.Type != "board" || inter == nil || !inter.Board.Active {
		return
	}
	if inter.Board.Panning {
		return // a manual drag is in flight — follow mode yields
	}
	raw, ok := n.Prop("cameraTarget")
	if !ok {
		return
	}
	target := resolveCameraTarget(raw, rt)
	if target == nil {
		return
	}
	tx, okt := toFloatAny(target["x"])
	ty, oky := toFloatAny(target["y"])
	if !okt || !oky || math.IsNaN(tx) || math.IsNaN(ty) {
		return
	}

	cell := 1.0
	if raw, ok := n.Prop("cameraCell"); ok {
		if v, ok := toFloatAny(raw); ok && v > 0 {
			cell = v
		}
	}

	// Viewport in physical px: an explicit cameraViewport prop wins, else
	// derive from the surface bounds. We do NOT multiply the cameraTarget
	// by cell anymore — the camera contract is px-in / px-out (see the doc
	// comment) — but cameraCell is still applied to cameraViewport so the
	// natural authoring form `cameraViewport: 16` + `cameraCell: 32` (a
	// 16-cell-wide viewport in a 32-px game) keeps landing at 512 px. The
	// pre-existing board tests / scene examples read on unchanged. When no
	// cameraViewport is given, the surface bounds are already in px and the
	// cell multiplier must NOT apply (it would turn a 512-px screen into a
	// 16 384-px "viewport", hiding the player off the left edge).
	viewW, viewH := boardViewportFromScreen(n, screen, cell)
	if _, hasViewport := n.Prop("cameraViewport"); hasViewport {
		viewW *= cell
		viewH *= cell
	}

	cx := tx
	cy := ty

	center := false
	if raw, ok := n.Prop("cameraCenter"); ok {
		if b, ok := raw.(bool); ok {
			center = b
		}
	}
	if center {
		inter.Board.PanX = -cx + viewW/2
		inter.Board.PanY = -cy + viewH/2
	} else {
		inter.Board.PanX = -cx
		inter.Board.PanY = -cy
	}
}

// boardViewportFromScreen computes the camera's PHYSICAL-px width/height for
// the cameraCenter math. An explicit cameraViewport prop wins (the height
// is derived from the screen aspect so a `16` viewport at 1024x480 means
// 16 cells × 32 px = 512 px wide and a proportional 240 px tall); otherwise
// both axes come straight from the surface. The cell parameter is accepted
// for back-compat with the previous world-unit contract but is now a no-op —
// the result is always in physical px.
func boardViewportFromScreen(n *model.Node, screen image.Point, cell float64) (float64, float64) {
	_ = cell // cell is now a no-op; px-in / px-out
	screenW := float64(screen.X)
	screenH := float64(screen.Y)
	if screenW <= 0 || screenH <= 0 {
		// Headless / pre-layout: fall back to a 4:3 16-px viewport, the
		// common side-scroller default. Authors can override with
		// cameraViewport: <px>.
		if raw, ok := n.Prop("cameraViewport"); ok {
			if v, ok := toFloatAny(raw); ok && v > 0 {
				return v, v * 3 / 4
			}
		}
		return 16, 12
	}
	if raw, ok := n.Prop("cameraViewport"); ok {
		if v, ok := toFloatAny(raw); ok && v > 0 {
			// cameraViewport is "logical viewport width in px". Translate to
			// the same height ratio as the actual surface so the target sits
			// at the geometric center of the window regardless of aspect.
			return v, v * screenH / screenW
		}
	}
	return screenW, screenH
}

// resolveCameraTarget turns a raw prop value into a {x, y} map. Strings
// are bound against state (a `{{state.player}}` shorthand); already-typed
// objects pass through. Anything else (numbers, nil) means "no target".
func resolveCameraTarget(raw any, rt *runtime.Runtime) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		if v == "" {
			return nil
		}
		out := runtime.EvalBinding(v, runtimeEvalCtx(rt))
		if m, ok := out.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// toFloatAny coerces a JSON value (number, string-number, or already-float)
// into a float64. Missing keys and unparseable values return ok=false; the
// caller decides whether that's "use 0" or "no target".
func toFloatAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		if x == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// runtimeEvalCtx builds the map runtime.EvalBinding expects. The camera
// target only needs `state`, so the cheaper path is enough here.
func runtimeEvalCtx(rt *runtime.Runtime) map[string]any {
	if rt == nil {
		return nil
	}
	return map[string]any{"state": rt.State}
}
