package canvas

import (
	"math"
	"time"

	"github.com/qorm/qorm/internal/geom"
)

// ClickDetector turns a stream of presses into a click count: two presses
// within DoubleClickMaxGap at nearly the same position are a double-click,
// a third a triple-click. It is deliberately minimal (time + position only)
// so a later gesture round can build double-tap / long-press detection on the
// same primitive. Lives in Interaction (reset with it on a scene switch).
type ClickDetector struct {
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

// Register records a press at p now and returns the click count (1..3, capped
// at 3 so a rapid fourth press cannot escalate to a quadruple-click).
func (d *ClickDetector) Register(p geom.Point, now time.Time) int {
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
