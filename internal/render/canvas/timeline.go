package canvas

// Declarative timeline (DOTween Sequence / Godot Tween chain) — the `timeline`
// prop on any node. Steps Append by default; `"parallel": true` Joins the
// previous step's group. Restarts when `timelineToken` / bound token changes
// (same fire pattern as `fx` + `fxToken`).
//
//	{
//	  "timeline": [
//	    { "scale": 1.3, "duration": 180, "ease": "backOut" },
//	    { "dx": 48, "duration": 200, "ease": "easeOut", "parallel": true },
//	    { "scale": 1, "dx": 0, "duration": 200, "ease": "easeInOut" }
//	  ],
//	  "timelineToken": "{{ state.tlPlay }}"
//	}
//
// Object form also accepted:
//
//	"timeline": { "steps": […], "loop": true, "yoyo": true, "token": "…" }

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/anim"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

type timelineKey struct {
	n   *model.Node
	idx int
}

type timelineState struct {
	seq       *anim.Sequence
	token     string
	sig       string // step signature — rebuild if authoring changes
	wasRun    bool   // saw Running once (for onComplete edge)
	completed bool   // onComplete already fired for this play
}

// timelineParams mirrors fx/entrance contribution channels.
// active means the node has a timeline (apply pose even after finish — hold end).
type timelineParams struct {
	opacity  float64
	dx, dy   float64
	scale    float64
	rotation float64
	running  bool
	active   bool
}

func timelineFor(n *model.Node, idx int, rt *runtime.Runtime, inter *Interaction, now time.Time) timelineParams {
	raw, ok := n.Prop("timeline")
	if !ok || inter == nil || raw == nil {
		return timelineParams{opacity: 1, scale: 1}
	}

	steps, loop, yoyo, repeat, tokenFromObj := parseTimelineProp(raw, rt)
	if len(steps) == 0 {
		return timelineParams{opacity: 1, scale: 1}
	}

	token := tokenFromObj
	if tr, ok := n.Prop("timelineToken"); ok {
		token = evalPropStr(tr, rt)
	} else if tr, ok := n.Prop("timelineKey"); ok {
		token = evalPropStr(tr, rt)
	}
	// Optional loop/yoyo overrides on the node.
	if raw, ok := n.Prop("timelineLoop"); ok {
		loop = truthyProp(raw, rt)
	}
	if raw, ok := n.Prop("timelineYoyo"); ok {
		yoyo = truthyProp(raw, rt)
	}
	if raw, ok := n.Prop("timelineRepeat"); ok {
		if f, ok := asFloat(raw); ok {
			repeat = int(f)
		} else if s := evalPropStr(raw, rt); s != "" {
			if s == "infinite" || s == "-1" {
				repeat = -1
				loop = true
			} else if n, err := strconv.Atoi(s); err == nil {
				repeat = n
			}
		}
	}

	// List stagger: prepend a wait so item i starts i*stagger ms later.
	if stg := staggerMS(n, idx, rt); stg > 0 {
		wait := anim.SeqStep{Duration: time.Duration(stg) * time.Millisecond}
		steps = append([]anim.SeqStep{wait}, steps...)
	}

	sig := timelineSig(steps, loop, yoyo, repeat)
	if inter.Timeline == nil {
		inter.Timeline = map[timelineKey]*timelineState{}
	}
	key := timelineKey{n, idx}
	st, ok := inter.Timeline[key]
	if !ok || st.token != token || st.sig != sig {
		seq := &anim.Sequence{
			Steps:     steps,
			StartTime: now,
			Loop:      loop,
			Yoyo:      yoyo,
			Repeat:    repeat,
		}
		st = &timelineState{seq: seq, token: token, sig: sig}
		inter.Timeline[key] = st
	}

	pose := st.seq.Evaluate(now)
	scale := pose.Scale
	if scale <= 0 {
		scale = 1
	}
	// Opacity: hold authored values including 0 (fade-out end pose). Only
	// treat "unset" as 1 when the sequence never wrote opacity (still 0 and
	// no step ever targeted it — SeqPose default is 1, so 0 is intentional).
	op := pose.Opacity
	if op < 0 {
		op = 0
	}

	// onComplete: fire once on running → finished edge (not for infinite loops).
	infinite := loop || (yoyo && repeat <= 0) || repeat < 0
	if pose.Running {
		st.wasRun = true
	} else if st.wasRun && !st.completed && !infinite {
		st.completed = true
		inv := parseNodeInvoke(n, "timelineOnComplete")
		if inv == nil {
			inv = parseNodeInvoke(n, "onComplete")
		}
		if inv != nil {
			inter.queueComplete(inv, map[string]any{
				"timeline": n.ID,
				"token":    token,
			})
		}
	}

	return timelineParams{
		opacity:  op,
		dx:       pose.DX,
		dy:       pose.DY,
		scale:    scale,
		rotation: pose.Rotation,
		running:  pose.Running,
		active:   true,
	}
}

// parseNodeInvoke reads a string action id or {name, args} map like loader.parseInvoke.
func parseNodeInvoke(n *model.Node, key string) *model.Invoke {
	if n == nil {
		return nil
	}
	raw, ok := n.Prop(key)
	if !ok || raw == nil {
		return nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		return &model.Invoke{Name: s, Args: map[string]string{}}
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	name, _ := m["name"].(string)
	if name == "" {
		return nil
	}
	inv := &model.Invoke{Name: name, Args: map[string]string{}}
	if args, ok := m["args"].(map[string]any); ok {
		for k, v := range args {
			switch t := v.(type) {
			case string:
				inv.Args[k] = t
			default:
				inv.Args[k] = fmt.Sprint(t)
			}
		}
	}
	return inv
}

// staggerMS returns index * stagger (ms). Prop `stagger` is per-item delay for
// list instances (DOTween SetDelay(i * step) / GSAP stagger).
func staggerMS(n *model.Node, idx int, rt *runtime.Runtime) float64 {
	if n == nil || idx <= 0 {
		return 0
	}
	raw, ok := n.Prop("stagger")
	if !ok {
		return 0
	}
	per := 0.0
	if f, ok := asFloat(raw); ok {
		per = f
	} else if s, ok := raw.(string); ok {
		s = strings.TrimSpace(evalPropStr(s, rt))
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			per = f
		}
	}
	if per <= 0 {
		return 0
	}
	return per * float64(idx)
}

func truthyProp(raw any, rt *runtime.Runtime) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(evalPropStr(v, rt)))
		return s == "true" || s == "1" || s == "yes" || s == "infinite"
	}
	s := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
	return s == "true" || s == "1" || s == "yes"
}

func asFloat(v any) (float64, bool) {
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

// parseTimelineProp accepts []any steps or {steps, loop, yoyo, token, repeat}.
func parseTimelineProp(raw any, rt *runtime.Runtime) (steps []anim.SeqStep, loop, yoyo bool, repeat int, token string) {
	switch v := raw.(type) {
	case []any:
		steps = parseTimelineSteps(v, rt)
		return steps, false, false, 0, ""
	case map[string]any:
		if s, ok := v["steps"].([]any); ok {
			steps = parseTimelineSteps(s, rt)
		} else if s, ok := v["timeline"].([]any); ok {
			steps = parseTimelineSteps(s, rt)
		}
		loop = truthyMap(v, "loop") || truthyMap(v, "repeatInfinite")
		yoyo = truthyMap(v, "yoyo")
		if f, ok := asFloat(v["repeat"]); ok {
			repeat = int(f)
		}
		if s, ok := v["token"].(string); ok {
			token = evalPropStr(s, rt)
		}
		return steps, loop, yoyo, repeat, token
	}
	return nil, false, false, 0, ""
}

func truthyMap(m map[string]any, k string) bool {
	v, ok := m[k]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes"
	}
	return false
}

func parseTimelineSteps(arr []any, rt *runtime.Runtime) []anim.SeqStep {
	var out []anim.SeqStep
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		st, ok := parseOneTimelineStep(m, rt)
		if ok {
			out = append(out, st)
		}
	}
	return out
}

func parseOneTimelineStep(m map[string]any, rt *runtime.Runtime) (anim.SeqStep, bool) {
	// wait-only step: { "wait": 100 } or { "delay": 100 } with no channels
	waitOnly := false
	var st anim.SeqStep

	if w, ok := stepDuration(m, "wait"); ok && w > 0 {
		st.Duration = w
		waitOnly = true
	}
	if d, ok := stepDuration(m, "delay"); ok {
		st.Delay = d
	}
	if d, ok := stepDuration(m, "duration"); ok {
		st.Duration = d
		waitOnly = false
	} else if d, ok := stepDuration(m, "ms"); ok {
		st.Duration = d
		waitOnly = false
	}
	if st.Duration <= 0 && st.Delay <= 0 {
		// default short step if channels present
		st.Duration = 200 * time.Millisecond
	}

	if e, ok := m["ease"].(string); ok && e != "" {
		if c, ok := anim.CurveByName(evalPropStr(e, rt)); ok {
			st.Curve = c
		}
	} else if e, ok := m["curve"].(string); ok && e != "" {
		if c, ok := anim.CurveByName(evalPropStr(e, rt)); ok {
			st.Curve = c
		}
	}
	if st.Curve == nil {
		st.Curve = anim.EaseOutCubic
	}

	st.Parallel = truthyMap(m, "parallel") || truthyMap(m, "join")

	// Path follow (DOTween DOPath / Godot Path2D): path: [[x,y],…] or cubic.
	if pts, cubic := parsePathPoints(m["path"]); len(pts) >= 2 {
		st.Path = pts
		st.Cubic = cubic || truthyMap(m, "cubic")
		st.Orient = truthyMap(m, "orient") || truthyMap(m, "orientToPath")
		waitOnly = false
	}

	if waitOnly && !hasTimelineChannels(m) {
		// Pure wait: hold pose (no channel ends).
		return st, true
	}

	if v, ok := stepFloat(m, "opacity", rt); ok {
		st.Opacity = anim.F64(v)
	}
	if v, ok := stepFloat(m, "scale", rt); ok {
		st.Scale = anim.F64(v)
	}
	if v, ok := stepFloat(m, "dx", rt); ok {
		st.DX = anim.F64(v)
	} else if v, ok := stepFloat(m, "x", rt); ok {
		// Relative offset alias used by some authors.
		st.DX = anim.F64(v)
	}
	if v, ok := stepFloat(m, "dy", rt); ok {
		st.DY = anim.F64(v)
	} else if v, ok := stepFloat(m, "y", rt); ok {
		st.DY = anim.F64(v)
	}
	if v, ok := stepFloat(m, "rotation", rt); ok {
		st.Rotation = anim.F64(v)
	} else if v, ok := stepFloat(m, "rotate", rt); ok {
		st.Rotation = anim.F64(v)
	}

	if waitOnly && !hasTimelineChannels(m) {
		return st, true
	}
	if !hasTimelineChannels(m) && st.Duration > 0 && st.Delay == 0 && waitOnly {
		return st, true
	}
	// Skip empty junk objects.
	if !hasTimelineChannels(m) && !waitOnly {
		// duration-only without channels still acts as wait
		return st, true
	}
	return st, true
}

func hasTimelineChannels(m map[string]any) bool {
	for _, k := range []string{"opacity", "scale", "dx", "dy", "x", "y", "rotation", "rotate", "path"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// parsePathPoints accepts [[x,y],…] or [{x,y},…] arrays. Returns cubic=true
// when the map used key "cubic" path of exactly 4 points is left to caller.
func parsePathPoints(raw any) ([]anim.Point, bool) {
	arr, ok := raw.([]any)
	if !ok || len(arr) < 2 {
		return nil, false
	}
	pts := make([]anim.Point, 0, len(arr))
	for _, item := range arr {
		switch v := item.(type) {
		case []any:
			if len(v) >= 2 {
				x, xok := asFloat(v[0])
				y, yok := asFloat(v[1])
				if xok && yok {
					pts = append(pts, anim.Point{X: x, Y: y})
				}
			}
		case map[string]any:
			x, xok := asFloat(v["x"])
			y, yok := asFloat(v["y"])
			if !xok {
				x, xok = asFloat(v["X"])
			}
			if !yok {
				y, yok = asFloat(v["Y"])
			}
			if xok && yok {
				pts = append(pts, anim.Point{X: x, Y: y})
			}
		}
	}
	if len(pts) < 2 {
		return nil, false
	}
	return pts, false
}

func stepDuration(m map[string]any, key string) (time.Duration, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return 0, false
		}
		return time.Duration(t) * time.Millisecond, true
	case int:
		if t <= 0 {
			return 0, false
		}
		return time.Duration(t) * time.Millisecond, true
	case string:
		if d, err := parseCSSDuration(t); err == nil && d > 0 {
			return d, true
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil && f > 0 {
			return time.Duration(f) * time.Millisecond, true
		}
	}
	return 0, false
}

func stepFloat(m map[string]any, key string, rt *runtime.Runtime) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	if f, ok := asFloat(v); ok {
		return f, true
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(evalPropStr(s, rt))
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
		if ev := runtime.EvalBinding(s, map[string]any{"state": rt.State}); ev != nil {
			if f, ok := asFloat(ev); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func timelineSig(steps []anim.SeqStep, loop, yoyo bool, repeat int) string {
	// Coarse signature so authoring edits rebuild the sequence.
	var b strings.Builder
	fmt.Fprintf(&b, "L%vY%vR%dN%d", loop, yoyo, repeat, len(steps))
	for _, st := range steps {
		fmt.Fprintf(&b, "|%d:%d:p%v", st.Duration.Milliseconds(), st.Delay.Milliseconds(), st.Parallel)
		if st.Scale != nil {
			fmt.Fprintf(&b, "s%.3f", *st.Scale)
		}
		if st.DX != nil {
			fmt.Fprintf(&b, "x%.1f", *st.DX)
		}
		if st.DY != nil {
			fmt.Fprintf(&b, "y%.1f", *st.DY)
		}
		if st.Opacity != nil {
			fmt.Fprintf(&b, "o%.2f", *st.Opacity)
		}
		if st.Rotation != nil {
			fmt.Fprintf(&b, "r%.1f", *st.Rotation)
		}
		if len(st.Path) > 0 {
			fmt.Fprintf(&b, "P%d", len(st.Path))
			for _, p := range st.Path {
				fmt.Fprintf(&b, ",%.0f:%.0f", p.X, p.Y)
			}
			if st.Cubic {
				b.WriteByte('C')
			}
			if st.Orient {
				b.WriteByte('O')
			}
		}
	}
	return b.String()
}

var (
	tlWarnMu   sync.Mutex
	tlWarnSeen = map[string]bool{}
)

func warnTimelineOnce(msg string) {
	tlWarnMu.Lock()
	defer tlWarnMu.Unlock()
	if tlWarnSeen[msg] {
		return
	}
	tlWarnSeen[msg] = true
	fmt.Fprintf(os.Stderr, "[qorm canvas] %s\n", msg)
}
