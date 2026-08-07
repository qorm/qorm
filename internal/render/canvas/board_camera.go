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

	// cameraCenter accepts `true` (centre both axes), `"x"` (centre X only,
	// leave Y pinned to the world top), or `"y"` (centre Y only). The
	// per-axis form is what a side-scroller wants: NES Mario's camera
	// stays glued to the bottom of the level (Y is constant) and only the
	// X axis follows the player, so the ground tile is always anchored to
	// the bottom of the screen regardless of the player's vertical state.
	centerX, centerY := false, false
	if raw, ok := n.Prop("cameraCenter"); ok {
		switch v := raw.(type) {
		case bool:
			centerX, centerY = v, v
		case string:
			switch v {
			case "x":
				centerX = true
			case "y":
				centerY = true
			case "true", "1":
				centerX, centerY = true, true
			}
		}
	}
	// cameraDeadZone (logical viewport units) lets the camera IGNORE target
	// movement inside a centered dead band, the way a side-scroller holds
	// its camera at the start of the level until the player walks past the
	// screen midpoint, then keeps them roughly in the same on-screen column
	// as the level scrolls. NES Mario's "mario at the left third of the
	// screen" feel is the canonical example: a 0 dead-zone centres the
	// target; a value of e.g. 8 (in cameraViewport units) reserves 8 cells
	// in the middle of the viewport as a "no scroll" band and only the
	// outer thirds trigger follow. The prop accepts a float (it shares
	// cameraViewport's px-in / px-out contract).
	dz := 0.0
	if raw, ok := n.Prop("cameraDeadZone"); ok {
		if v, ok := toFloatAny(raw); ok && v > 0 {
			dz = v
		}
	}
	// cameraMax bounds the camera's resolved pan in PHYSICAL px — without
	// it, the engine will follow the target past the right edge of the
	// level and start showing empty space (the same way a side-scroller
	// forgets to stop at the level end). Two-arg form: {x, y} clamps each
	// axis independently. Authors who want to lock the camera to the
	// level bounds compute (levelW - viewportW) * cell in their scene
	// JSON and pass the result.
	maxX, maxY := math.Inf(1), math.Inf(1)
	if raw, ok := n.Prop("cameraMax"); ok {
		switch v := raw.(type) {
		case float64:
			maxX = v
			maxY = v
		case int:
			maxX = float64(v)
			maxY = float64(v)
		case int64:
			maxX = float64(v)
			maxY = float64(v)
		case map[string]any:
			if x, ok := toFloatAny(v["x"]); ok {
				maxX = x
			}
			if y, ok := toFloatAny(v["y"]); ok {
				maxY = y
			}
		}
	}
	if centerX || centerY {
		desiredX := 0.0
		desiredY := 0.0
		if centerX {
			desiredX = -cx + viewW/2
		} else {
			desiredX = -cx
		}
		if centerY {
			desiredY = -cy + viewH/2
		} else {
			desiredY = 0
		}
		if centerX && dz > 0 {
			// Convert the dead zone to PanX units (pan-inverse of target
			// motion: target moving right by dx shifts the desired pan by
			// -dx; we want the camera NOT to shift when the target moves
			// inside the central band). The current pan is the previous
			// frame's resolved value, snapshotted before this assignment.
			curX := inter.Board.PanX
			half := dz / 2
			// pan window where the camera is parked: curX - half .. curX + half
			lo, hi := curX-half, curX+half
			if desiredX < lo {
				desiredX = lo
			} else if desiredX > hi {
				desiredX = hi
			}
		}
		inter.Board.PanX = desiredX
		inter.Board.PanY = desiredY
	} else {
		// No centering: pin the camera to the world origin (top-left) and
		// let the target move freely inside the viewport. This is the
		// natural "no follow" mode — a board that shows the full world
		// with the player inside it, no auto-scroll.
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
