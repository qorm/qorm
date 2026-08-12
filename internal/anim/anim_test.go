package anim

import (
	"math"
	"testing"
)

func TestCurveByNameRegistry(t *testing.T) {
	names := []string{
		"linear",
		"easeIn", "easeInCubic",
		"easeOut", "easeOutCubic",
		"easeInOut", "easeInOutCubic",
		"standard", "emphasized",
		// Game-engine vocabulary (Phaser / Godot / DOTween / GSAP).
		"spring", "back", "elastic", "bounce",
		"easeOutBack", "easeOutElastic", "easeOutBounce",
		"easeOutQuad", "easeInOutSine", "easeOutExpo",
		"backOut", "elasticOut", "bounceOut", "quadOut", "sineOut", "expoOut",
	}
	for _, name := range names {
		if _, ok := CurveByName(name); !ok {
			t.Errorf("CurveByName(%q) not registered", name)
		}
	}
	if _, ok := CurveByName("not-a-real-ease"); ok {
		t.Error("CurveByName(not-a-real-ease) should be unknown")
	}
	if _, ok := CurveByName(""); ok {
		t.Error("CurveByName(\"\") should be unknown")
	}
}

func TestGameEngineEasingsEndpoints(t *testing.T) {
	for name, c := range map[string]Curve{
		"EaseOutBack":    EaseOutBack,
		"EaseInBack":     EaseInBack,
		"EaseOutElastic": EaseOutElastic,
		"EaseOutBounce":  EaseOutBounce,
		"EaseOutQuad":    EaseOutQuad,
		"EaseOutSine":    EaseOutSine,
		"EaseOutExpo":    EaseOutExpo,
	} {
		if got := c(0); math.Abs(got) > 1e-9 {
			t.Errorf("%s(0) = %v, want ~0", name, got)
		}
		if got := c(1); math.Abs(got-1) > 1e-9 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
	// Back/elastic overshoot past 1 mid-flight (game-engine signature).
	if EaseOutBack(0.7) <= 1 {
		t.Errorf("EaseOutBack should overshoot past 1 mid-curve, got %v", EaseOutBack(0.7))
	}
}

func TestCurveAliases(t *testing.T) {
	// The short spec names must resolve to the same curves as the long ones.
	for _, alias := range []string{"easeOut", "standard"} {
		c, _ := CurveByName(alias)
		for _, x := range []float64{0, 0.25, 0.5, 0.75, 1} {
			if c(x) != EaseOutCubic(x) {
				t.Errorf("%s(%v) != EaseOutCubic(%v)", alias, x, x)
			}
		}
	}
	c, _ := CurveByName("emphasized")
	if c(0.3) != EaseInOutCubic(0.3) {
		t.Error("emphasized alias must equal EaseInOutCubic")
	}
}

func TestCurveEndpoints(t *testing.T) {
	for name, c := range map[string]Curve{
		"Linear":         Linear,
		"EaseOutCubic":   EaseOutCubic,
		"EaseInCubic":    EaseInCubic,
		"EaseInOutCubic": EaseInOutCubic,
		"Spring":         Spring,
	} {
		if got := c(0); got != 0 {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := c(1); got != 1 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
	if got := EaseInOutCubic(0.5); got != 0.5 {
		t.Errorf("EaseInOutCubic(0.5) = %v, want 0.5 (symmetry)", got)
	}
}

// Spring is underdamped: it overshoots past 1 (the bounce) then settles back —
// unlike the monotone easings, it must NOT be added to the monotone test.
func TestSpringOvershootsAndSettles(t *testing.T) {
	overshot := false
	for i := 1; i <= 100; i++ {
		x := float64(i) / 100
		v := Spring(x)
		if v > 1.05 {
			overshot = true
		}
		// It must decay back toward 1 by the end.
		if x > 0.7 && math.Abs(v-1) > 0.05 {
			t.Errorf("Spring(%v) = %v, want to settle near 1 after x=0.7", x, v)
		}
	}
	if !overshot {
		t.Error("Spring must overshoot past 1 (a spring bounces, it does not ease)")
	}
	if _, got := CurveByName("spring"); !got {
		t.Error("spring must be registered in the named-curve map")
	}
}

func TestCurvesMonotone(t *testing.T) {
	for name, c := range map[string]Curve{
		"EaseOutCubic":   EaseOutCubic,
		"EaseInCubic":    EaseInCubic,
		"EaseInOutCubic": EaseInOutCubic,
	} {
		prev := c(0)
		for i := 1; i <= 100; i++ {
			x := float64(i) / 100
			if v := c(x); v < prev {
				t.Fatalf("%s decreased at x=%v: %v < %v", name, x, v, prev)
			} else {
				prev = v
			}
		}
	}
}

func TestTweenClamps(t *testing.T) {
	tw := IntTween(10, 20)
	if got := tw.Lerp(-1); got != 10 {
		t.Errorf("Lerp(-1) = %d, want 10 (clamped)", got)
	}
	if got := tw.Lerp(2); got != 20 {
		t.Errorf("Lerp(2) = %d, want 20 (clamped)", got)
	}
	if got := tw.Lerp(0.5); got != 15 {
		t.Errorf("Lerp(0.5) = %d, want 15", got)
	}
}
