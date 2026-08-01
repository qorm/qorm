package canvas

// Entrance animations: the cross-cutting `animation` prop (api/animation.md)
// — any node may play an entrance effect when it MOUNTS (created, appended to
// a bound list, scene re-entered). The canvas engine replays this with
// opacity + translation (the two channels its raster interpolates for free);
// effects that need scale/rotate degrade to fade with a one-shot warning.

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

// entranceKey identifies one mounted node. List instances share the template's
// model pointer, so the instance index disambiguates (outside lists idx is 0).
type entranceKey struct {
	n   *model.Node
	idx int
}

// entranceState is one entrance clock. It lives in Interaction.Entrance, so
// it dies with the rest of Interaction on a scene switch — which is exactly
// when entrances should replay (a fresh mount).
type entranceState struct {
	start time.Time
}

// entranceParams is what an entrance contributes to the node's group this
// frame: opacity multiplied in, dx/dy added to its position, and whether the
// animation is still running (the frame loop must keep ticking).
type entranceParams struct {
	opacity float64
	dx, dy  float64
	running bool
}

// entranceFx maps an effect name to its start transform: a fade flag and the
// initial translation that eases to zero. Effects needing scale/rotate are
// not representable in the current raster and degrade to fade.
type entranceFx struct {
	fade   bool
	dx, dy float64
}

var entranceEffects = map[string]entranceFx{
	"fade":       {true, 0, 0},
	"fadeup":     {true, 0, 12},
	"fadedown":   {true, 0, -12},
	"slideup":    {false, 0, 24},
	"slidedown":  {false, 0, -24},
	"slideleft":  {false, 24, 0},
	"slideright": {false, -24, 0},
	"pop":        {true, 0, 0}, // scale degraded to fade (raster has no scale yet)
	"scale":      {true, 0, 0}, // ditto
	"zoomout":    {true, 0, 0}, // ditto
	"rotate":     {true, 0, 0}, // ditto
	"flip":       {true, 0, 0}, // ditto
}

// entranceFor evaluates the node's `animation` prop for this frame. now is
// injectable so tests drive the clock explicitly. The prop is bindable
// ("{{state.effect}}"), evaluated with the instance scope like any other.
func entranceFor(n *model.Node, idx int, rt *runtime.Runtime, inter *Interaction, now time.Time) entranceParams {
	raw, ok := n.Prop("animation")
	if !ok || inter == nil {
		return entranceParams{opacity: 1}
	}
	name := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
	if name == "" || name == "none" {
		return entranceParams{opacity: 1}
	}

	if inter.Entrance == nil {
		inter.Entrance = map[entranceKey]*entranceState{}
	}
	key := entranceKey{n, idx}
	st, ok := inter.Entrance[key]
	if !ok {
		// First sight of this node = its mount: the clock starts now.
		st = &entranceState{start: now}
		inter.Entrance[key] = st
	}

	duration := entranceNum(n, rt, "duration", 450)
	delay := entranceNum(n, rt, "delay", 0)
	repeatInf, repeatN := entranceRepeat(n, rt)

	elapsed := float64(now.Sub(st.start).Milliseconds()) - delay
	if elapsed < 0 {
		return entranceParams{opacity: entranceInitial(name), running: true}
	}
	var t float64
	if duration <= 0 {
		t = 1
	} else {
		t = float64(elapsed) / duration
	}
	cycle, running := t, false
	switch {
	case repeatInf:
		cycle = t - math.Floor(t)
		running = true
	case t < 1:
		running = true
	default:
		cycle = 1
	}
	if repeatN > 1 && !repeatInf {
		if t < float64(repeatN) {
			cycle = t - math.Floor(t)
			running = true
		} else {
			cycle = 1
			running = false
		}
	}
	if !running {
		return entranceParams{opacity: 1}
	}

	ease := entranceEase(rt)
	e := ease(cycle)

	switch name {
	case "bounce":
		// A jump: rise above the line and settle back, opacity full.
		return entranceParams{opacity: 1, dy: -14 * math.Sin(math.Pi*cycle), running: true}
	case "shake":
		return entranceParams{opacity: 1, dx: 10 * math.Sin(4*math.Pi*cycle) * (1 - cycle), running: true}
	case "pulse":
		return entranceParams{opacity: 1 - 0.4*math.Sin(math.Pi*cycle), running: true}
	case "spin":
		// Rotation is not representable; degrade to a gentle fade pulse.
		warnEntranceOnce(name, "animation %q needs rotate/scale, which the native raster does not support yet; playing a fade instead", name)
		return entranceParams{opacity: 0.5 + 0.5*e, running: true}
	}

	fx, ok := entranceEffects[name]
	if !ok {
		warnEntranceOnce(name, "unknown animation effect %q; playing fade", name)
		fx = entranceEffects["fade"]
	}
	if name == "pop" || name == "scale" || name == "zoomout" || name == "rotate" || name == "flip" {
		warnEntranceOnce(name, "animation %q needs scale/rotate, which the native raster does not support yet; playing a fade instead", name)
	}
	p := entranceParams{dx: fx.dx * (1 - e), dy: fx.dy * (1 - e), running: true}
	if fx.fade {
		p.opacity = e
	} else {
		p.opacity = 1
	}
	return p
}

// entranceInitial is the frame-zero opacity for the delay window.
func entranceInitial(name string) float64 {
	if fx, ok := entranceEffects[name]; ok && fx.fade {
		return 0
	}
	return 1
}

// entranceNum reads a numeric animation prop (duration/delay) — plain
// numbers pass through directly, strings go through binding evaluation.
func entranceNum(n *model.Node, rt *runtime.Runtime, prop string, def float64) float64 {
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

// entranceRepeat reads the repeat prop: ("infinite", N).
func entranceRepeat(n *model.Node, rt *runtime.Runtime) (inf bool, count int) {
	raw, ok := n.Prop("repeat")
	if !ok {
		return false, 1
	}
	if f, ok := raw.(float64); ok && f > 1 {
		return false, int(f)
	}
	s := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
	if s == "infinite" {
		return true, 0
	}
	if v := runtime.EvalBinding(s, map[string]any{"state": rt.State}); v != nil {
		if f, ok := v.(float64); ok && f > 1 {
			return false, int(f)
		}
	}
	return false, 1
}

// entranceEase returns the theme's standard easing (falling back to
// easeOutCubic), clamped to [0,1] input — the `curve` prop is not parsed yet
// (nearest theme token, documented).
func entranceEase(rt *runtime.Runtime) func(float64) float64 {
	if rt != nil && rt.Theme != nil {
		if c := rt.Theme.Easing("standard"); c != nil {
			return func(t float64) float64 {
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}
				return c(t)
			}
		}
	}
	return func(t float64) float64 {
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		return 1 - (1-t)*(1-t)*(1-t) // easeOutCubic
	}
}

// One-shot warnings for degraded/unknown effects (per effect name per scene,
// same discipline as the style-key warnings).
var (
	entranceWarnMu   sync.Mutex
	entranceWarnRoot *model.Node
	entranceWarnSeen = map[string]bool{}
)

func warnEntranceOnce(key, format, name string) {
	entranceWarnMu.Lock()
	defer entranceWarnMu.Unlock()
	if entranceWarnSeen[key] {
		return
	}
	entranceWarnSeen[key] = true
	fmt.Fprintf(os.Stderr, "[qorm canvas] "+format+"\n", name)
}
