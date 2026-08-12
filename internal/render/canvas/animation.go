package canvas

import (
	"time"

	"github.com/qorm/qorm/internal/anim"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

type AnimState struct {
	TargetStyle  NodeStyle
	CurrentStyle NodeStyle
	Controller   *anim.Controller
}

var globalAnimStates = make(map[string]*AnimState)

// UpdateAndGetAnimatedStyle is the classic form, keyed by the node ID with
// the theme's standard duration.
func UpdateAndGetAnimatedStyle(id string, target NodeStyle, rt *runtime.Runtime) (NodeStyle, bool) {
	return UpdateAndGetAnimatedStyleD(id, target, rt, 0)
}

// UpdateAndGetAnimatedStyleD is the animated-style resolver with a per-node
// duration override (a declarative `transition` — the interaction-effect
// resolver's transition half) and a caller-chosen key: disambiguate repeat
// instances that share a template ID, or their tweens would fight. duration
// <= 0 falls back to the theme's "normal" motion token (250ms / easeOutCubic).
// target.TransitionEasing selects a named curve ("spring", "easeOut", …);
// empty uses the theme standard easing. Returns the interpolated style and
// whether a redraw is still needed.
func UpdateAndGetAnimatedStyleD(key string, target NodeStyle, rt *runtime.Runtime, duration time.Duration) (NodeStyle, bool) {
	if key == "" {
		return target, false
	}

	state, ok := globalAnimStates[key]
	if !ok {
		// First time seeing this node. Theme accessors are nil-safe: with no
		// theme loaded they yield the default motion tokens (250ms/easeOutCubic).
		var th *theme.Theme
		if rt != nil {
			th = rt.Theme
		}
		d := duration
		if d <= 0 {
			d = time.Duration(th.DurationMs("normal")) * time.Millisecond
		}
		state = &AnimState{
			TargetStyle:  target,
			CurrentStyle: target,
			Controller:   anim.NewController(d, resolveTransitionCurve(target.TransitionEasing, th)),
		}
		// Push it immediately to finished
		state.Controller.StartTime = time.Now().Add(-1 * time.Second)
		globalAnimStates[key] = state
		return target, false
	}

	// Did the target change? Compare every animatable field — a margin-only
	// change (e.g. physics.json's moving_block) must re-target the tween, not
	// pin the style to the first frame.
	targetChanged := false
	if state.TargetStyle.Background != target.Background ||
		state.TargetStyle.Color != target.Color ||
		state.TargetStyle.Padding != target.Padding ||
		state.TargetStyle.MarginTop != target.MarginTop ||
		state.TargetStyle.MarginBot != target.MarginBot ||
		state.TargetStyle.MarginLeft != target.MarginLeft ||
		state.TargetStyle.MarginRight != target.MarginRight ||
		state.TargetStyle.Gap != target.Gap ||
		state.TargetStyle.BorderRadius != target.BorderRadius ||
		state.TargetStyle.Width != target.Width ||
		state.TargetStyle.Height != target.Height ||
		state.TargetStyle.Opacity != target.Opacity ||
		state.TargetStyle.EffectiveScale != target.EffectiveScale {
		targetChanged = true
	}

	if targetChanged {
		// Refresh duration/easing so a new transition style (e.g. spring)
		// takes effect on the next press/hover retarget.
		if duration > 0 {
			state.Controller.Duration = duration
		}
		var th *theme.Theme
		if rt != nil {
			th = rt.Theme
		}
		state.Controller.Curve = resolveTransitionCurve(target.TransitionEasing, th)
		state.TargetStyle = target
		state.Controller.Reset()
	}

	// Calculate interpolation using new anim engine
	t, isRunning := state.Controller.Value()

	if !isRunning {
		state.CurrentStyle = state.TargetStyle
		return state.CurrentStyle, false
	}

	// Copy all target fields (including non-animatable ones)
	current := target

	// Interpolate all animatable fields
	current.Background = anim.ColorTween(state.CurrentStyle.Background, state.TargetStyle.Background).Lerp(t)
	current.Color = anim.ColorTween(state.CurrentStyle.Color, state.TargetStyle.Color).Lerp(t)
	current.Padding = anim.IntTween(state.CurrentStyle.Padding, state.TargetStyle.Padding).Lerp(t)
	current.Width = anim.IntTween(state.CurrentStyle.Width, state.TargetStyle.Width).Lerp(t)
	current.Height = anim.IntTween(state.CurrentStyle.Height, state.TargetStyle.Height).Lerp(t)

	current.MarginTop = anim.IntTween(state.CurrentStyle.MarginTop, state.TargetStyle.MarginTop).Lerp(t)
	current.MarginBot = anim.IntTween(state.CurrentStyle.MarginBot, state.TargetStyle.MarginBot).Lerp(t)
	current.MarginLeft = anim.IntTween(state.CurrentStyle.MarginLeft, state.TargetStyle.MarginLeft).Lerp(t)
	current.MarginRight = anim.IntTween(state.CurrentStyle.MarginRight, state.TargetStyle.MarginRight).Lerp(t)
	current.Gap = anim.IntTween(state.CurrentStyle.Gap, state.TargetStyle.Gap).Lerp(t)
	current.BorderRadius = anim.Float64Tween(state.CurrentStyle.BorderRadius, state.TargetStyle.BorderRadius).Lerp(t)
	current.Opacity = anim.Float64Tween(state.CurrentStyle.Opacity, state.TargetStyle.Opacity).Lerp(t)
	current.EffectiveScale = anim.Float64Tween(state.CurrentStyle.EffectiveScale, state.TargetStyle.EffectiveScale).Lerp(t)

	state.CurrentStyle = current
	return current, true // Needs redraw
}

// resolveTransitionCurve picks a named easing for declarative transitions.
// Unknown/empty names fall back to the theme standard curve (easeOutCubic).
func resolveTransitionCurve(name string, th *theme.Theme) anim.Curve {
	if name != "" {
		if c, ok := anim.CurveByName(name); ok {
			return c
		}
	}
	return th.Easing("standard")
}
