package canvas

import (
	"math"
	"time"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
)

// ClickDetector turns a stream of presses into a click count: two presses
// within DoubleClickMaxGap at nearly the same position are a double-click,
// a third a triple-click. It is deliberately minimal (time + position only)
// so a later gesture round can build double-tap / long-press detection on the
// same primitive. Lives in Interaction (reset with it on a scene switch).
type ClickDetector struct {
	node     *model.Node // the editable the clicks belong to (a field change resets)
	lastTime time.Time
	lastPos  geom.Point
	count    int
}

const (
	// DoubleClickMaxGap is the max time between presses that still counts as
	// the same click (the platform standard is ~500ms).
	DoubleClickMaxGap = 500 * time.Millisecond
	// DoubleClickRadius is the max pointer travel between presses (physical
	// px) that still counts as the same click — enough to absorb sub-pixel
	// jitter, small enough that presses on different fields never merge.
	DoubleClickRadius = 4.0
)

// Register records a press at p now for the editable n and returns the click
// count (1..3, capped at 3 so a rapid fourth press cannot escalate to a
// quadruple-click). A press on a DIFFERENT field is always a fresh single
// click, so fast clicks on two adjacent inputs never merge into a double.
func (d *ClickDetector) Register(n *model.Node, p geom.Point, now time.Time) int {
	if d.node != n {
		d.node, d.count, d.lastTime, d.lastPos = n, 1, now, p
		return 1
	}
	if !d.lastTime.IsZero() && now.Sub(d.lastTime) <= DoubleClickMaxGap &&
		math.Hypot(p.X-d.lastPos.X, p.Y-d.lastPos.Y) <= DoubleClickRadius {
		d.count++
	} else {
		d.count = 1
	}
	if d.count > 3 {
		d.count = 3
	}
	d.lastTime, d.lastPos = now, p
	return d.count
}

// LongPressDetector distinguishes a press held beyond LongPressMinDuration
// (without drifting past LongPressSlop) from a plain tap. It is the long-press
// building block for a later gestureDetector round: call Press on pointer-down,
// Release on pointer-up. (Firing mid-hold needs a timer source; release-time
// detection is the primitive every consumer can build on.)
type LongPressDetector struct {
	startTime time.Time
	startPos  geom.Point
	down      bool
}

const (
	// LongPressMinDuration is how long a hold must last to count as a
	// long-press (the platform convention for context-menu long-press).
	LongPressMinDuration = 500 * time.Millisecond
	// LongPressSlop is how far the pointer may drift before the press stops
	// counting as a long-press (a drag is not a long-press).
	LongPressSlop = 10.0
)

// Press arms the detector at p now.
func (d *LongPressDetector) Press(p geom.Point, now time.Time) {
	d.startTime, d.startPos, d.down = now, p, true
}

// Release reports whether the press qualified as a long-press and disarms.
func (d *LongPressDetector) Release(p geom.Point, now time.Time) bool {
	ok := d.down && now.Sub(d.startTime) >= LongPressMinDuration &&
		math.Hypot(p.X-d.startPos.X, p.Y-d.startPos.Y) <= LongPressSlop
	d.down = false
	return ok
}
