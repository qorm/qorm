package anim

import (
	"image/color"
	"math"
	"strings"
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

// ---- Game-engine easings (Phaser / Godot / DOTween / GSAP vocabulary) ----
// Formulas match the common "Penner" set used by those engines so authors can
// port timing curves without inventing QORM-only names.

// EaseInQuad / EaseOutQuad / EaseInOutQuad — quadratic family.
var EaseInQuad Curve = func(t float64) float64 { return t * t }
var EaseOutQuad Curve = func(t float64) float64 { return t * (2 - t) }
var EaseInOutQuad Curve = func(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return -1 + (4-2*t)*t
}

// EaseInSine / EaseOutSine / EaseInOutSine — smooth sinusoidal family.
var EaseInSine Curve = func(t float64) float64 {
	return 1 - math.Cos(t*math.Pi/2)
}
var EaseOutSine Curve = func(t float64) float64 {
	return math.Sin(t * math.Pi / 2)
}
var EaseInOutSine Curve = func(t float64) float64 {
	return -(math.Cos(math.Pi*t) - 1) / 2
}

// EaseOutExpo — fast attack, long settle (UI emphasis / game power-ups).
var EaseOutExpo Curve = func(t float64) float64 {
	if t >= 1 {
		return 1
	}
	if t <= 0 {
		return 0
	}
	return 1 - math.Pow(2, -10*t)
}

// EaseInExpo — inverse of EaseOutExpo.
var EaseInExpo Curve = func(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return math.Pow(2, 10*t-10)
}

// EaseOutBack — slight overshoot then settle (DOTween OutBack / Godot BACK).
var EaseOutBack Curve = func(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	t--
	return 1 + c3*t*t*t + c1*t*t
}

// EaseInBack — pull back then accelerate into the target.
var EaseInBack Curve = func(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	return c3*t*t*t - c1*t*t
}

// EaseInOutBack — pull-back both ends.
var EaseInOutBack Curve = func(t float64) float64 {
	const c1 = 1.70158
	const c2 = c1 * 1.525
	if t < 0.5 {
		return (math.Pow(2*t, 2) * ((c2+1)*2*t - c2)) / 2
	}
	return (math.Pow(2*t-2, 2)*((c2+1)*(t*2-2)+c2) + 2) / 2
}

// EaseOutElastic — springy overshoot with oscillation (Phaser Elastic.Out).
var EaseOutElastic Curve = func(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	const c4 = (2 * math.Pi) / 3
	return math.Pow(2, -10*t)*math.Sin((t*10-0.75)*c4) + 1
}

// EaseOutBounce — ball bounce settle (Phaser Bounce.Out / Godot BOUNCE).
var EaseOutBounce Curve = func(t float64) float64 {
	const n1 = 7.5625
	const d1 = 2.75
	switch {
	case t < 1/d1:
		return n1 * t * t
	case t < 2/d1:
		t -= 1.5 / d1
		return n1*t*t + 0.75
	case t < 2.5/d1:
		t -= 2.25 / d1
		return n1*t*t + 0.9375
	default:
		t -= 2.625 / d1
		return n1*t*t + 0.984375
	}
}

// namedCurves maps theme/spec + game-engine easing names to curves.
// Short names ("easeOut", "easeInOut") match planning/spec/motion-spec.md;
// back/elastic/bounce/quad/sine/expo match Phaser, Godot, DOTween, GSAP.
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
	// Game-engine vocabulary.
	"easeInQuad":     EaseInQuad,
	"easeOutQuad":    EaseOutQuad,
	"easeInOutQuad":  EaseInOutQuad,
	"quadIn":         EaseInQuad,
	"quadOut":        EaseOutQuad,
	"quadInOut":      EaseInOutQuad,
	"easeInSine":     EaseInSine,
	"easeOutSine":    EaseOutSine,
	"easeInOutSine":  EaseInOutSine,
	"sineIn":         EaseInSine,
	"sineOut":        EaseOutSine,
	"sineInOut":      EaseInOutSine,
	"easeInExpo":     EaseInExpo,
	"easeOutExpo":    EaseOutExpo,
	"expoIn":         EaseInExpo,
	"expoOut":        EaseOutExpo,
	"easeInBack":     EaseInBack,
	"easeOutBack":    EaseOutBack,
	"easeInOutBack":  EaseInOutBack,
	"backIn":         EaseInBack,
	"backOut":        EaseOutBack,
	"backInOut":      EaseInOutBack,
	"easeOutElastic": EaseOutElastic,
	"elasticOut":     EaseOutElastic,
	"easeOutBounce":  EaseOutBounce,
	"bounceOut":      EaseOutBounce,
	// Bare family names default to the Out form (most common in UI/games).
	"back":    EaseOutBack,
	"elastic": EaseOutElastic,
	"bounce":  EaseOutBounce,
	"quad":    EaseOutQuad,
	"sine":    EaseOutSine,
	"expo":    EaseOutExpo,
}

// CurveByName looks up a named easing curve (case-insensitive). The boolean
// reports whether the name is registered; callers should fall back to a
// default curve when false.
func CurveByName(name string) (Curve, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if c, ok := namedCurves[key]; ok {
		return c, true
	}
	// CamelCase registry keys (easeOutBack) after lowercasing become
	// easeoutback — also try stripping nothing further; rebuild from
	// known aliases if the map still holds mixed-case keys.
	if c, ok := namedCurves[name]; ok {
		return c, true
	}
	// Match mixed-case map entries under lowercase equality.
	for k, c := range namedCurves {
		if strings.ToLower(k) == key {
			return c, true
		}
	}
	return nil, false
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

// LoopMode selects how Controller.Value advances after one duration span.
// Matches DOTween SetLoops / Godot Tween set_loops vocabulary.
type LoopMode int

const (
	// LoopOnce plays 0→1 once then finishes (default, prior Controller behavior).
	LoopOnce LoopMode = iota
	// LoopRepeat restarts 0→1. Repeat < 0 is infinite; Repeat > 0 is that many
	// full cycles after the first (total plays = Repeat when Repeat >= 1… see Value).
	// Convention: Repeat <= 0 with LoopRepeat means infinite; Repeat == N > 0
	// means play N times total.
	LoopRepeat
	// LoopYoyo plays 0→1→0 (one yoyo cycle = 2×Duration). Repeat <= 0 infinite;
	// Repeat == N means N full yoyo cycles then finish at the start value.
	LoopYoyo
)

// Controller manages the state of an animation (DOTween / Godot Tween core).
type Controller struct {
	Duration  time.Duration
	Delay     time.Duration // time before progress leaves 0 (engine-style set_delay)
	Curve     Curve
	StartTime time.Time
	Mode      LoopMode
	// Repeat: LoopOnce ignores it. LoopRepeat/LoopYoyo: <=0 infinite, >0 count.
	Repeat int
}

func NewController(duration time.Duration, curve Curve) *Controller {
	if curve == nil {
		curve = Linear
	}
	return &Controller{
		Duration:  duration,
		Curve:     curve,
		StartTime: time.Now(),
		Mode:      LoopOnce,
	}
}

// WithLoop returns c after setting mode/repeat (fluent, DOTween-style).
func (c *Controller) WithLoop(mode LoopMode, repeat int) *Controller {
	if c != nil {
		c.Mode = mode
		c.Repeat = repeat
	}
	return c
}

// WithDelay returns c after setting start delay.
func (c *Controller) WithDelay(d time.Duration) *Controller {
	if c != nil {
		c.Delay = d
	}
	return c
}

func (c *Controller) Reset() {
	c.StartTime = time.Now()
}

// Value returns a normalized 0..1 float for the current animation progress
// and whether the animation is still running. Delay holds at Curve(0).
// LoopYoyo returns the ping-pong progress (eased per half-cycle).
func (c *Controller) Value() (float64, bool) {
	if c == nil {
		return 1, false
	}
	curve := c.Curve
	if curve == nil {
		curve = Linear
	}
	elapsed := time.Since(c.StartTime)
	if c.Delay > 0 {
		if elapsed < c.Delay {
			return curve(0), true
		}
		elapsed -= c.Delay
	}
	dur := c.Duration
	if dur <= 0 {
		return curve(1), false
	}
	raw := float64(elapsed) / float64(dur)

	switch c.Mode {
	case LoopRepeat:
		if c.Repeat > 0 && raw >= float64(c.Repeat) {
			return curve(1), false
		}
		t := raw - math.Floor(raw)
		return curve(t), true
	case LoopYoyo:
		// One yoyo cycle = forward + reverse = 2 half-spans.
		cycles := raw / 2
		if c.Repeat > 0 && cycles >= float64(c.Repeat) {
			return curve(0), false // settled at start after N yoyos
		}
		half := raw - math.Floor(raw/2)*2 // in [0, 2)
		if half <= 1 {
			return curve(half), true
		}
		return curve(2 - half), true
	default: // LoopOnce
		if elapsed >= dur {
			return curve(1), false
		}
		t := float64(elapsed) / float64(dur)
		return curve(t), true
	}
}

// Finished reports whether Value would return running=false (convenience).
func (c *Controller) Finished() bool {
	_, run := c.Value()
	return !run
}
