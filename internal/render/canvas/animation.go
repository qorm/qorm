package canvas

import (
	"time"

	"github.com/qorm/platform/internal/anim"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

type AnimState struct {
	BeginStyle   NodeStyle // origin at last retarget (yoyo begin)
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
// empty uses the theme standard easing. TransitionYoyo / TransitionLoop /
// TransitionRepeat map to DOTween SetLoops. Returns the interpolated style
// and whether a redraw is still needed.
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
		ctrl := anim.NewController(d, resolveTransitionCurve(target.TransitionEasing, th))
		applyTransitionLoop(ctrl, target)
		state = &AnimState{
			BeginStyle:   target,
			TargetStyle:  target,
			CurrentStyle: target,
			Controller:   ctrl,
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
		state.TargetStyle.EffectiveScale != target.EffectiveScale ||
		state.TargetStyle.PosX != target.PosX ||
		state.TargetStyle.PosY != target.PosY {
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
		applyTransitionLoop(state.Controller, target)
		// Yoyo/lerp origin is the live visual, not the previous target.
		state.BeginStyle = state.CurrentStyle
		state.TargetStyle = target
		state.Controller.Reset()
	} else {
		// Loop mode flags may change without field retarget (rare).
		applyTransitionLoop(state.Controller, target)
	}

	// Calculate interpolation using new anim engine
	t, isRunning := state.Controller.Value()

	if !isRunning {
		// Yoyo settles at begin; once/repeat settle at target.
		if state.Controller.Mode == anim.LoopYoyo {
			state.CurrentStyle = state.BeginStyle
		} else {
			state.CurrentStyle = state.TargetStyle
		}
		return state.CurrentStyle, false
	}

	// Copy all target fields (including non-animatable ones)
	current := target

	// Interpolate all animatable fields from Begin → Target (yoyo-safe).
	begin := state.BeginStyle
	end := state.TargetStyle
	current.Background = anim.ColorTween(begin.Background, end.Background).Lerp(t)
	current.Color = anim.ColorTween(begin.Color, end.Color).Lerp(t)
	current.Padding = anim.IntTween(begin.Padding, end.Padding).Lerp(t)
	current.Width = anim.IntTween(begin.Width, end.Width).Lerp(t)
	current.Height = anim.IntTween(begin.Height, end.Height).Lerp(t)

	current.MarginTop = anim.IntTween(begin.MarginTop, end.MarginTop).Lerp(t)
	current.MarginBot = anim.IntTween(begin.MarginBot, end.MarginBot).Lerp(t)
	current.MarginLeft = anim.IntTween(begin.MarginLeft, end.MarginLeft).Lerp(t)
	current.MarginRight = anim.IntTween(begin.MarginRight, end.MarginRight).Lerp(t)
	current.Gap = anim.IntTween(begin.Gap, end.Gap).Lerp(t)
	current.BorderRadius = anim.Float64Tween(begin.BorderRadius, end.BorderRadius).Lerp(t)
	current.Opacity = anim.Float64Tween(begin.Opacity, end.Opacity).Lerp(t)
	current.EffectiveScale = anim.Float64Tween(begin.EffectiveScale, end.EffectiveScale).Lerp(t)
	// Absolute position (board / left-top): tween so x/y changes with
	// transition animate instead of snapping (layout motion).
	current.PosX = anim.Float64Tween(begin.PosX, end.PosX).Lerp(t)
	current.PosY = anim.Float64Tween(begin.PosY, end.PosY).Lerp(t)

	state.CurrentStyle = current
	return current, true // Needs redraw
}

// applyTransitionLoop maps NodeStyle loop flags onto Controller (DOTween SetLoops).
// LoopRepeat/LoopYoyo treat Repeat<=0 as infinite.
func applyTransitionLoop(c *anim.Controller, s NodeStyle) {
	if c == nil {
		return
	}
	switch {
	case s.TransitionYoyo:
		c.Mode = anim.LoopYoyo
		if s.TransitionRepeat != 0 {
			c.Repeat = s.TransitionRepeat // <0 infinite, >0 count
		} else if s.TransitionLoop {
			c.Repeat = 0 // infinite yoyo
		} else {
			c.Repeat = 1 // one yoyo cycle then settle at begin
		}
	case s.TransitionLoop || s.TransitionRepeat < 0:
		c.Mode = anim.LoopRepeat
		c.Repeat = 0 // infinite
		if s.TransitionRepeat > 0 {
			c.Repeat = s.TransitionRepeat
		}
	case s.TransitionRepeat > 1:
		c.Mode = anim.LoopRepeat
		c.Repeat = s.TransitionRepeat
	default:
		c.Mode = anim.LoopOnce
		c.Repeat = 0
	}
}

// resolveTransitionCurve picks a named easing for declarative transitions.
// Unknown/empty names fall back to the theme standard curve (easeOutCubic).
func resolveTransitionCurve(name string, th *theme.Theme) anim.Curve {
	if name != "" {
		// Case-insensitive game-engine names (backOut, Elastic.Out → elasticout).
		if c, ok := anim.CurveByName(name); ok {
			return c
		}
	}
	return th.Easing("standard")
}
