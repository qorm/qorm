package canvas

// Game-style feedback FX (the `fx` prop) — one-shot / short-loop motion
// inspired by 2D engine APIs:
//
//   DOTween:  DOShake / DOPunchScale / DOFade
//   Phaser:   cameras.shake, tweens with yoyo scale
//   Godot:    Tween.shake / AnimationPlayer one-shots
//   Flame:    SequenceEffect-style composable offsets
//
// Unlike entrance `animation` (mount-time), `fx` restarts whenever the bound
// effect name OR `fxToken` changes — so a game can fire damage shake with:
//
//	{ "fx": "hit", "fxToken": "{{ state.hits }}" }
//
// and `state.hits = state.hits + 1` from qscript.

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// fxKey identifies one mounted node's FX clock (list index disambiguates).
type fxKey struct {
	n   *model.Node
	idx int
}

// fxState is one feedback clock. Lives on Interaction.FX so it dies with the
// scene switch (same discipline as Entrance).
type fxState struct {
	start time.Time
	name  string
	token string
}

// fxParams is what one FX contributes this frame (composed onto entrance /
// transform channels in measure → PerformLayout).
type fxParams struct {
	opacity  float64
	dx, dy   float64
	scale    float64 // 1 = identity
	rotation float64 // radians
	running  bool
}

// fxFor evaluates the node's `fx` prop for this frame. now is injectable for
// tests. Empty / "none" is a no-op. Changing name or fxToken restarts the clock.
func fxFor(n *model.Node, idx int, rt *runtime.Runtime, inter *Interaction, now time.Time) fxParams {
	raw, ok := n.Prop("fx")
	if !ok || inter == nil {
		return fxParams{opacity: 1, scale: 1}
	}
	name := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
	if name == "" || name == "none" {
		return fxParams{opacity: 1, scale: 1}
	}

	token := ""
	if tr, ok := n.Prop("fxToken"); ok {
		token = evalPropStr(tr, rt)
	} else if tr, ok := n.Prop("fxKey"); ok {
		// Alias used by some game engines ("key" restarts the clip).
		token = evalPropStr(tr, rt)
	}

	if inter.FX == nil {
		inter.FX = map[fxKey]*fxState{}
	}
	key := fxKey{n, idx}
	st, ok := inter.FX[key]
	if !ok || st.name != name || st.token != token {
		st = &fxState{start: now, name: name, token: token}
		inter.FX[key] = st
	}

	duration := fxNum(n, rt, "fxDuration", 0)
	if duration <= 0 {
		duration = fxNum(n, rt, "duration", fxDefaultDuration(name))
	}
	intensity := fxNum(n, rt, "fxIntensity", 0)
	if intensity <= 0 {
		intensity = fxNum(n, rt, "intensity", fxDefaultIntensity(name))
	}
	delay := fxNum(n, rt, "fxDelay", 0) + staggerMS(n, idx, rt)

	elapsed := float64(now.Sub(st.start).Milliseconds()) - delay
	if elapsed < 0 {
		return fxParams{opacity: 1, scale: 1, running: true}
	}
	var t float64
	if duration <= 0 {
		t = 1
	} else {
		t = elapsed / duration
	}

	loop := fxLoop(n, rt, name)
	running := t < 1
	cycle := t
	if loop {
		cycle = t - math.Floor(t)
		running = true
	} else if t >= 1 {
		cycle = 1
		running = false
	}
	if !running {
		return fxParams{opacity: 1, scale: 1}
	}

	return evalFX(name, cycle, intensity)
}

func fxDefaultDuration(name string) float64 {
	switch name {
	case "float", "bob":
		return 1200
	case "flash", "blink":
		return 400
	case "hit":
		return 320
	case "punch":
		return 280
	case "burst":
		return 360
	case "wobble":
		return 500
	case "knockback":
		return 220
	default: // shake
		return 300
	}
}

func fxDefaultIntensity(name string) float64 {
	switch name {
	case "float", "bob":
		return 6
	case "punch":
		return 0.18
	case "wobble":
		return 12 // degrees
	case "knockback":
		return 16
	case "flash", "blink":
		return 0.55 // opacity dip
	case "hit":
		return 10
	default: // shake
		return 10
	}
}

// fxLoop: float/bob/blink loop until cleared; others are one-shots.
func fxLoop(n *model.Node, rt *runtime.Runtime, name string) bool {
	if raw, ok := n.Prop("fxLoop"); ok {
		s := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
		return s == "true" || s == "1" || s == "yes" || s == "infinite"
	}
	if raw, ok := n.Prop("repeat"); ok {
		s := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
		if s == "infinite" {
			return true
		}
	}
	switch name {
	case "float", "bob", "blink":
		return true
	}
	return false
}

// evalFX maps a game-engine-style effect name to transform offsets at cycle
// progress t ∈ [0,1]. Intensity scales amplitude (px, scale delta, or deg).
func evalFX(name string, t, intensity float64) fxParams {
	// Decay envelope: full strength early, settles to 0 at t=1 (one-shots).
	env := 1 - t
	if env < 0 {
		env = 0
	}
	switch name {
	case "shake":
		// DOTween DOShakePosition / Phaser cameras.shake: multi-frequency jitter.
		amp := intensity * env
		return fxParams{
			opacity: 1, scale: 1, running: true,
			dx: amp * math.Sin(t*math.Pi*10),
			dy: amp * 0.6 * math.Sin(t*math.Pi*13+0.7),
		}
	case "punch":
		// DOTween DOPunchScale: scale out then settle to 1.
		// intensity is the peak extra scale (0.18 → 1.18).
		peak := intensity
		if peak <= 0 {
			peak = 0.18
		}
		// Single half-sine bump.
		s := 1 + peak*math.Sin(math.Pi*t)*env
		return fxParams{opacity: 1, scale: s, running: true}
	case "flash", "blink":
		// Brief opacity dip (invuln flash / UI attention).
		dip := intensity
		if dip <= 0 {
			dip = 0.55
		}
		// Two quick blinks over the cycle.
		wave := math.Abs(math.Sin(t * math.Pi * 4))
		op := 1 - dip*wave
		if op < 0.15 {
			op = 0.15
		}
		return fxParams{opacity: op, scale: 1, running: true}
	case "hit":
		// Combo: shake + punch + slight flash (common damage feedback pack).
		amp := intensity * env
		if amp <= 0 {
			amp = 10 * env
		}
		s := 1 + 0.14*math.Sin(math.Pi*t)*env
		op := 1 - 0.25*math.Abs(math.Sin(t*math.Pi*3))*env
		return fxParams{
			opacity: op, scale: s, running: true,
			dx: amp * math.Sin(t*math.Pi*11),
			dy: amp * 0.5 * math.Cos(t*math.Pi*9),
		}
	case "float", "bob":
		// Idle bob for pickups / floating labels (looping sine).
		amp := intensity
		if amp <= 0 {
			amp = 6
		}
		return fxParams{
			opacity: 1, scale: 1, running: true,
			dy: -amp * math.Sin(t*2*math.Pi),
		}
	case "wobble":
		// Rotation wobble (degrees → radians), decaying.
		deg := intensity
		if deg <= 0 {
			deg = 12
		}
		rad := deg * math.Pi / 180 * math.Sin(t*math.Pi*6) * env
		return fxParams{opacity: 1, scale: 1, rotation: rad, running: true}
	case "knockback":
		// Single horizontal shove that springs back (platformer hit).
		amp := intensity
		if amp <= 0 {
			amp = 16
		}
		// Out then back: sin(π t) * env would decay twice — use (1-t)*sin(π t).
		return fxParams{
			opacity: 1, scale: 1, running: true,
			dx: amp * math.Sin(math.Pi*t) * env,
		}
	case "burst":
		// Lightweight particle-burst stand-in (no multi-sprite emitter):
		// radial knock + scale punch + flash — DOTween DOPunch + explosion pack.
		amp := intensity
		if amp <= 0 {
			amp = 14
		}
		ang := t * math.Pi * 6
		return fxParams{
			opacity: 1 - 0.35*env*math.Abs(math.Sin(t*math.Pi*4)),
			scale:   1 + 0.35*math.Sin(math.Pi*t)*env,
			running: true,
			dx:      amp * math.Cos(ang) * env,
			dy:      amp * math.Sin(ang) * env,
		}
	default:
		warnFXOnce(name)
		// Unknown → mild shake so authors still see motion.
		amp := 6 * env
		return fxParams{
			opacity: 1, scale: 1, running: true,
			dx: amp * math.Sin(t*math.Pi*8),
		}
	}
}

func fxNum(n *model.Node, rt *runtime.Runtime, prop string, def float64) float64 {
	raw, ok := n.Prop(prop)
	if !ok {
		return def
	}
	num := func(v any) (float64, bool) {
		switch t := v.(type) {
		case float64:
			return t, true
		case int:
			return float64(t), true
		case int64:
			return float64(t), true
		}
		return 0, false
	}
	if f, ok := num(raw); ok {
		return f
	}
	if s, ok := raw.(string); ok {
		if f, ok := num(runtime.EvalBinding(strings.TrimSpace(s), map[string]any{"state": rt.State})); ok {
			return f
		}
	}
	return def
}

var (
	fxWarnMu   sync.Mutex
	fxWarnSeen = map[string]bool{}
)

func warnFXOnce(name string) {
	fxWarnMu.Lock()
	defer fxWarnMu.Unlock()
	if fxWarnSeen[name] {
		return
	}
	fxWarnSeen[name] = true
	fmt.Fprintf(os.Stderr, "[qorm canvas] unknown fx %q; playing mild shake (try shake/punch/flash/hit/float/wobble/knockback/burst)\n", name)
}
