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

// UpdateAndGetAnimatedStyle handles the lifecycle of an animated style.
// Duration and easing come from the active theme's motion tokens
// ("normal" duration, "standard" easing); the accessors are nil-safe and
// fall back to 250ms / easeOutCubic when no theme is loaded.
func UpdateAndGetAnimatedStyle(id string, target NodeStyle, rt *runtime.Runtime) (NodeStyle, bool) {
	if id == "" {
		return target, false
	}

	state, ok := globalAnimStates[id]
	if !ok {
		// First time seeing this node. Theme accessors are nil-safe: with no
		// theme loaded they yield the default motion tokens (250ms/easeOutCubic).
		var th *theme.Theme
		if rt != nil {
			th = rt.Theme
		}
		state = &AnimState{
			TargetStyle:  target,
			CurrentStyle: target,
			Controller:   anim.NewController(time.Duration(th.DurationMs("normal"))*time.Millisecond, th.Easing("standard")),
		}
		// Push it immediately to finished
		state.Controller.StartTime = time.Now().Add(-1 * time.Second)
		globalAnimStates[id] = state
		return target, false
	}

	// Did the target change?
	targetChanged := false
	if state.TargetStyle.Background != target.Background ||
		state.TargetStyle.Color != target.Color ||
		state.TargetStyle.Padding != target.Padding ||
		state.TargetStyle.Width != target.Width ||
		state.TargetStyle.Height != target.Height ||
		state.TargetStyle.Opacity != target.Opacity {
		targetChanged = true
	}

	if targetChanged {
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

	state.CurrentStyle = current
	return current, true // Needs redraw
}
