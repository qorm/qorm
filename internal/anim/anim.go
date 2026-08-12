package anim

import (
	"image/color"
	"math"
	"time"
)

// Curve represents an animation easing function.
type Curve func(t float64) float64

// Linear is a curve that returns the input unchanged.
var Linear Curve = func(t float64) float64 { return t }

// EaseOutCubic is a curve that slows down towards the end.
var EaseOutCubic Curve = func(t float64) float64 {
	t--
	return t*t*t + 1
}

// Spring is an underdamped spring easing: it starts at 0, overshoots past 1
// (a ~18% peak bounce around t≈0.26) and oscillates while decaying, settling
// within ~1% of 1 by t≈0.7. Good for entrance bounces and pop physics where
// a gentle overshoot reads as life.
var Spring Curve = func(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	v := 1 - math.Exp(-6*t)*math.Cos(10*t)
	if v < 0 {
		return 0
	}
	return v
}

// EaseInCubic is a curve that accelerates from rest.
var EaseInCubic Curve = func(t float64) float64 {
	return t * t * t
}

// EaseInOutCubic accelerates then decelerates (symmetric).
var EaseInOutCubic Curve = func(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	t = 2*t - 2
	return 0.5*t*t*t + 1
}

// namedCurves maps theme/spec easing names to curves. The short names
// ("easeOut", "easeInOut") match planning/spec/motion-spec.md vocabulary.
var namedCurves = map[string]Curve{
	"linear":         Linear,
	"easeIn":         EaseInCubic,
	"easeInCubic":    EaseInCubic,
	"easeOut":        EaseOutCubic,
	"easeOutCubic":   EaseOutCubic,
	"easeInOut":      EaseInOutCubic,
	"easeInOutCubic": EaseInOutCubic,
	"spring":         Spring,
	// Token aliases used by theme motion sections.
	"standard":   EaseOutCubic,
	"emphasized": EaseInOutCubic,
}

// CurveByName looks up a named easing curve. The boolean reports whether the
// name is registered; callers should fall back to a default curve when false.
func CurveByName(name string) (Curve, bool) {
	c, ok := namedCurves[name]
	return c, ok
}

// Tween generic type for animating any value
type Tween[T any] struct {
	Begin  T
	End    T
	interp func(a, b T, t float64) T
}

func (tw Tween[T]) Lerp(t float64) T {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return tw.interp(tw.Begin, tw.End, t)
}

func IntTween(begin, end int) Tween[int] {
	return Tween[int]{
		Begin: begin,
		End:   end,
		interp: func(a, b int, t float64) int {
			return int(float64(a) + float64(b-a)*t)
		},
	}
}

func Float64Tween(begin, end float64) Tween[float64] {
	return Tween[float64]{
		Begin: begin,
		End:   end,
		interp: func(a, b float64, t float64) float64 {
			return a + (b-a)*t
		},
	}
}

func ColorTween(begin, end color.RGBA) Tween[color.RGBA] {
	return Tween[color.RGBA]{
		Begin: begin,
		End:   end,
		interp: func(a, b color.RGBA, t float64) color.RGBA {
			return color.RGBA{
				R: uint8(float64(a.R) + float64(b.R-a.R)*t),
				G: uint8(float64(a.G) + float64(b.G-a.G)*t),
				B: uint8(float64(a.B) + float64(b.B-a.B)*t),
				A: uint8(float64(a.A) + float64(b.A-a.A)*t),
			}
		},
	}
}

// Controller manages the state of an animation
type Controller struct {
	Duration  time.Duration
	Curve     Curve
	StartTime time.Time
}

func NewController(duration time.Duration, curve Curve) *Controller {
	if curve == nil {
		curve = Linear
	}
	return &Controller{
		Duration:  duration,
		Curve:     curve,
		StartTime: time.Now(),
	}
}

func (c *Controller) Reset() {
	c.StartTime = time.Now()
}

// Value returns a normalized 0..1 float for the current animation progress
func (c *Controller) Value() (float64, bool) {
	elapsed := time.Since(c.StartTime)
	if elapsed >= c.Duration {
		return c.Curve(1.0), false // finished
	}
	t := float64(elapsed) / float64(c.Duration)
	return c.Curve(t), true // still running
}
