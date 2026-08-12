package canvas

import (
	"math"
	"time"

	"github.com/qorm/qorm/internal/anim"
	"github.com/qorm/qorm/internal/theme"
)

// flipState tracks one node's previous layout box for FLIP layout motion.
type flipState struct {
	// Settled (or last committed) absolute box.
	x, y, w, h float64
	// In-flight animation.
	animating                            bool
	fromX, fromY, fromW, fromH           float64
	toX, toY, toW, toH                   float64
	ctrl                                 *anim.Controller
}

var globalFLIP = map[string]*flipState{}

// applyLayoutFLIP implements First-Last-Invert-Play layout animation for a
// node with a stable key (id / id@index). The node is already laid out at
// (x,y,w,h) in absolute pixels; the return offsets/scales paint it at the
// interpolated visual box so layout jumps ease instead of snap.
//
// dx,dy are added to the group's local X/Y; sx,sy multiply ScaleX/ScaleY
// (about the group's origin = top-left). running keeps the engine animating.
func applyLayoutFLIP(key string, x, y, w, h float64, dur time.Duration, easing string) (dx, dy, sx, sy float64, running bool) {
	sx, sy = 1, 1
	if key == "" || dur <= 0 {
		return 0, 0, 1, 1, false
	}
	st, ok := globalFLIP[key]
	if !ok {
		globalFLIP[key] = &flipState{x: x, y: y, w: w, h: h}
		return 0, 0, 1, 1, false
	}

	const eps = 0.5
	moved := math.Abs(st.x-x) > eps || math.Abs(st.y-y) > eps ||
		math.Abs(st.w-w) > eps || math.Abs(st.h-h) > eps

	if !st.animating {
		if !moved {
			return 0, 0, 1, 1, false
		}
		// Start FLIP: previous settled box → new layout box.
		st.fromX, st.fromY, st.fromW, st.fromH = st.x, st.y, st.w, st.h
		st.toX, st.toY, st.toW, st.toH = x, y, w, h
		curve := resolveTransitionCurve(easing, (*theme.Theme)(nil))
		st.ctrl = anim.NewController(dur, curve)
		st.animating = true
	} else {
		// Mid-flight retarget if layout moved again.
		if math.Abs(st.toX-x) > eps || math.Abs(st.toY-y) > eps ||
			math.Abs(st.toW-w) > eps || math.Abs(st.toH-h) > eps {
			t, _ := st.ctrl.Value()
			st.fromX = st.fromX + (st.toX-st.fromX)*t
			st.fromY = st.fromY + (st.toY-st.fromY)*t
			st.fromW = st.fromW + (st.toW-st.fromW)*t
			st.fromH = st.fromH + (st.toH-st.fromH)*t
			st.toX, st.toY, st.toW, st.toH = x, y, w, h
			st.ctrl.Reset()
		}
	}

	t, run := st.ctrl.Value()
	if !run {
		st.animating = false
		st.x, st.y, st.w, st.h = x, y, w, h
		return 0, 0, 1, 1, false
	}
	visX := st.fromX + (st.toX-st.fromX)*t
	visY := st.fromY + (st.toY-st.fromY)*t
	visW := st.fromW + (st.toW-st.fromW)*t
	visH := st.fromH + (st.toH-st.fromH)*t
	dx = visX - x
	dy = visY - y
	if w > 1e-6 {
		sx = visW / w
	}
	if h > 1e-6 {
		sy = visH / h
	}
	return dx, dy, sx, sy, true
}

// flipStillRunning reports whether any FLIP animation is in flight (engine loop).
func flipStillRunning() bool {
	for _, st := range globalFLIP {
		if st != nil && st.animating {
			if _, run := st.ctrl.Value(); run {
				return true
			}
			st.animating = false
		}
	}
	return false
}
