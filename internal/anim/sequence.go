package anim

import (
	"math"
	"time"
)

// Sequence is a DOTween Sequence / Godot Tween chain: ordered steps, optional
// parallel groups, delay, loop, and yoyo of the whole timeline.
//
// Steps with Parallel=true join the current group (start together; group
// duration is the max of members). A non-parallel step closes the group and
// starts a new sequential segment — same model as DOTween Append/Join and
// Godot Tween.parallel().

// SeqPose is the evaluated multi-channel pose at one instant.
type SeqPose struct {
	Opacity  float64 // 0..1, default 1
	Scale    float64 // 1 = identity
	DX, DY   float64 // px translation
	Rotation float64 // radians
	Running  bool
	// Progress is 0..1 over the whole sequence (ignoring loop/yoyo fold).
	Progress float64
}

// SeqStep is one tween segment (property bundle → duration/ease).
// Zero/nil channel pointers mean "hold previous value" for that channel.
type SeqStep struct {
	Duration time.Duration
	Delay    time.Duration // pre-step wait inside the group timeline
	Curve    Curve
	Parallel bool // join previous group (Join); false = Append new group
	// Absolute end values (nil = keep previous pose's value).
	Opacity  *float64
	Scale    *float64
	DX, DY   *float64
	Rotation *float64 // degrees in authoring; stored/applied as radians in pose
	// Path is a DOTween DOPath / Godot Path2D follow: when set (≥2 points),
	// DX/DY (and optional rotation if Orient) are driven by sampling the path
	// at the eased local t instead of lerping scalar ends.
	Path   []Point
	Cubic  bool // true + len(Path)==4 → cubic Bezier; else polyline
	Orient bool // rotate to path tangent (degrees written into Rotation channel)
}

// Sequence plays steps from StartTime.
type Sequence struct {
	Steps     []SeqStep
	StartTime time.Time
	Loop      bool // whole-sequence loop
	Yoyo      bool // whole-sequence yoyo (implies ping-pong of timeline progress)
	// Repeat whole-sequence count when Loop/Yoyo: <=0 infinite if Loop|Yoyo,
	// >0 finite. Ignored when both Loop and Yoyo are false.
	Repeat int
}

// NewSequence builds a sequence that starts now.
func NewSequence(steps ...SeqStep) *Sequence {
	return &Sequence{Steps: steps, StartTime: time.Now()}
}

func (s *Sequence) Reset() {
	if s != nil {
		s.StartTime = time.Now()
	}
}

// totalDuration is one forward pass (sum of group durations, each group =
// max(delay+duration) of members).
func (s *Sequence) totalDuration() time.Duration {
	if s == nil || len(s.Steps) == 0 {
		return 0
	}
	var total time.Duration
	var groupMax time.Duration
	flush := func() {
		total += groupMax
		groupMax = 0
	}
	for i, st := range s.Steps {
		if i > 0 && !st.Parallel {
			flush()
		}
		d := st.Delay + st.Duration
		if d > groupMax {
			groupMax = d
		}
	}
	flush()
	return total
}

// Evaluate returns the pose at now. Empty steps → identity, not running.
//
// Loop / Yoyo / Repeat (DOTween SetLoops):
//   - default: play forward once
//   - Yoyo: ping-pong; Repeat<=0 and Loop → infinite; Repeat==N → N yoyo cycles;
//     Yoyo alone (no Loop, Repeat==0) → one yoyo cycle then stop at start
//   - Loop without Yoyo: restart forward; Repeat<=0 infinite, Repeat==N → N plays
func (s *Sequence) Evaluate(now time.Time) SeqPose {
	id := SeqPose{Opacity: 1, Scale: 1}
	if s == nil || len(s.Steps) == 0 {
		return id
	}
	total := s.totalDuration()
	if total <= 0 {
		p := s.poseAtLinear(1)
		p.Running = false
		p.Progress = 1
		return p
	}
	elapsed := now.Sub(s.StartTime)
	if elapsed < 0 {
		return id
	}
	raw := float64(elapsed) / float64(total)

	fold := func(u float64, running bool) SeqPose {
		if u < 0 {
			u = 0
		}
		if u > 1 {
			u = 1
		}
		p := s.poseAtLinear(u)
		p.Running = running
		p.Progress = u
		return p
	}

	if s.Yoyo {
		infinite := s.Loop || s.Repeat < 0
		limit := 2.0 // one yoyo cycle default
		if s.Repeat > 0 {
			limit = float64(s.Repeat) * 2
			infinite = false
		}
		if !infinite && raw >= limit {
			return fold(0, false)
		}
		half := math.Mod(raw, 2)
		if half < 0 {
			half += 2
		}
		u := half
		if half > 1 {
			u = 2 - half
		}
		return fold(u, true)
	}

	if s.Loop || s.Repeat < 0 {
		if s.Repeat > 0 && raw >= float64(s.Repeat) {
			return fold(1, false)
		}
		u := raw - math.Floor(raw)
		return fold(u, true)
	}
	if s.Repeat > 1 {
		// Finite forward repeats without Loop flag.
		if raw >= float64(s.Repeat) {
			return fold(1, false)
		}
		u := raw - math.Floor(raw)
		return fold(u, true)
	}

	// Once
	if raw >= 1 {
		return fold(1, false)
	}
	return fold(raw, true)
}

// poseAtLinear evaluates a single forward pass at u ∈ [0,1].
func (s *Sequence) poseAtLinear(u float64) SeqPose {
	if u < 0 {
		u = 0
	}
	if u > 1 {
		u = 1
	}
	total := s.totalDuration()
	cur := SeqPose{Opacity: 1, Scale: 1}
	if total <= 0 {
		// Snap through all steps' end values in order.
		for _, st := range s.Steps {
			cur = applyStepEnd(cur, st)
		}
		return cur
	}
	targetElapsed := time.Duration(u * float64(total))

	// Rebuild groups.
	type group struct {
		steps []SeqStep
		dur   time.Duration
	}
	var groups []group
	var g group
	flush := func() {
		if len(g.steps) == 0 {
			return
		}
		groups = append(groups, g)
		g = group{}
	}
	for i, st := range s.Steps {
		if i > 0 && !st.Parallel {
			flush()
		}
		g.steps = append(g.steps, st)
		d := st.Delay + st.Duration
		if d > g.dur {
			g.dur = d
		}
	}
	flush()

	var acc time.Duration
	for _, gr := range groups {
		startPose := cur
		applyGroup := func(local time.Duration) SeqPose {
			out := startPose
			for _, st := range gr.steps {
				t := stepLocalT(st, local)
				part := blendStep(startPose, st, t)
				out = mergePose(out, startPose, part, st)
			}
			return out
		}
		if targetElapsed >= acc+gr.dur {
			cur = applyGroup(gr.dur) // completed group
			acc += gr.dur
			continue
		}
		return applyGroup(targetElapsed - acc)
	}
	return cur
}

func stepLocalT(st SeqStep, local time.Duration) float64 {
	if local <= st.Delay {
		return 0
	}
	if st.Duration <= 0 {
		return 1
	}
	t := float64(local-st.Delay) / float64(st.Duration)
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	curve := st.Curve
	if curve == nil {
		curve = EaseOutCubic
	}
	return curve(t)
}

func applyStepEnd(cur SeqPose, st SeqStep) SeqPose {
	return blendStep(cur, st, 1)
}

func blendStep(from SeqPose, st SeqStep, t float64) SeqPose {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	out := from
	if st.Opacity != nil {
		out.Opacity = from.Opacity + (*st.Opacity-from.Opacity)*t
	}
	if st.Scale != nil {
		out.Scale = from.Scale + (*st.Scale-from.Scale)*t
	}
	if len(st.Path) >= 2 {
		pos, tan := SamplePath(st.Path, t, st.Cubic)
		out.DX = pos.X
		out.DY = pos.Y
		if st.Orient {
			out.Rotation = math.Atan2(tan.Y, tan.X)
		}
	} else {
		if st.DX != nil {
			out.DX = from.DX + (*st.DX-from.DX)*t
		}
		if st.DY != nil {
			out.DY = from.DY + (*st.DY-from.DY)*t
		}
		if st.Rotation != nil {
			// Author degrees → radians.
			end := *st.Rotation * math.Pi / 180
			out.Rotation = from.Rotation + (end-from.Rotation)*t
		}
	}
	return out
}

// mergePose copies channels that st authored from part onto out (others keep out).
func mergePose(out, start, part SeqPose, st SeqStep) SeqPose {
	if st.Opacity != nil {
		out.Opacity = part.Opacity
	}
	if st.Scale != nil {
		out.Scale = part.Scale
	}
	if len(st.Path) >= 2 || st.DX != nil {
		out.DX = part.DX
	}
	if len(st.Path) >= 2 || st.DY != nil {
		out.DY = part.DY
	}
	if st.Rotation != nil || (len(st.Path) >= 2 && st.Orient) {
		out.Rotation = part.Rotation
	}
	return out
}

// F64 returns a *float64 for SeqStep field literals.
func F64(v float64) *float64 { return &v }
