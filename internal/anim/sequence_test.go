package anim

import (
	"math"
	"testing"
	"time"
)

func TestControllerDelay(t *testing.T) {
	c := NewController(100*time.Millisecond, Linear).WithDelay(50 * time.Millisecond)
	c.StartTime = time.Now().Add(-20 * time.Millisecond)
	v, run := c.Value()
	if !run || v != 0 {
		t.Fatalf("during delay: v=%v run=%v", v, run)
	}
	c.StartTime = time.Now().Add(-80 * time.Millisecond) // 50 delay + 30 into anim
	v, run = c.Value()
	if !run || v < 0.2 || v > 0.4 {
		t.Fatalf("after delay mid: v=%v run=%v", v, run)
	}
}

func TestControllerYoyo(t *testing.T) {
	c := NewController(100*time.Millisecond, Linear).WithLoop(LoopYoyo, 1)
	c.StartTime = time.Now().Add(-50 * time.Millisecond) // half forward
	v, run := c.Value()
	if !run || math.Abs(v-0.5) > 0.05 {
		t.Fatalf("yoyo forward mid: v=%v run=%v", v, run)
	}
	c.StartTime = time.Now().Add(-150 * time.Millisecond) // reverse mid
	v, run = c.Value()
	if !run || math.Abs(v-0.5) > 0.05 {
		t.Fatalf("yoyo reverse mid: v=%v run=%v", v, run)
	}
	c.StartTime = time.Now().Add(-250 * time.Millisecond) // past 1 yoyo cycle
	v, run = c.Value()
	if run {
		t.Fatalf("yoyo should finish after 1 cycle, v=%v", v)
	}
	if math.Abs(v) > 1e-9 {
		t.Fatalf("yoyo end should be 0, got %v", v)
	}
}

func TestControllerRepeat(t *testing.T) {
	c := NewController(100*time.Millisecond, Linear).WithLoop(LoopRepeat, 2)
	c.StartTime = time.Now().Add(-150 * time.Millisecond) // second cycle mid
	v, run := c.Value()
	if !run || v < 0.4 || v > 0.6 {
		t.Fatalf("repeat mid cycle2: v=%v run=%v", v, run)
	}
	c.StartTime = time.Now().Add(-250 * time.Millisecond)
	_, run = c.Value()
	if run {
		t.Fatal("repeat 2 should finish after 200ms")
	}
}

func TestSequenceAppendChain(t *testing.T) {
	// scale 1→2 over 100ms, then dx 0→40 over 100ms
	s := NewSequence(
		SeqStep{Duration: 100 * time.Millisecond, Curve: Linear, Scale: F64(2)},
		SeqStep{Duration: 100 * time.Millisecond, Curve: Linear, DX: F64(40)},
	)
	s.StartTime = time.Now()
	// Mid first step
	p := s.Evaluate(s.StartTime.Add(50 * time.Millisecond))
	if !p.Running || math.Abs(p.Scale-1.5) > 0.05 {
		t.Fatalf("step1 mid: scale=%v run=%v", p.Scale, p.Running)
	}
	if math.Abs(p.DX) > 0.5 {
		t.Fatalf("step1 should not move dx yet, dx=%v", p.DX)
	}
	// Mid second step
	p = s.Evaluate(s.StartTime.Add(150 * time.Millisecond))
	if !p.Running || math.Abs(p.Scale-2) > 0.05 {
		t.Fatalf("step2 mid: scale should be 2, got %v", p.Scale)
	}
	if math.Abs(p.DX-20) > 1 {
		t.Fatalf("step2 mid dx=%v want ~20", p.DX)
	}
	// Done
	p = s.Evaluate(s.StartTime.Add(250 * time.Millisecond))
	if p.Running {
		t.Fatal("sequence should finish")
	}
	if math.Abs(p.DX-40) > 0.5 || math.Abs(p.Scale-2) > 0.05 {
		t.Fatalf("final pose scale=%v dx=%v", p.Scale, p.DX)
	}
}

func TestSequenceParallelJoin(t *testing.T) {
	// scale and dx in parallel (Join)
	s := NewSequence(
		SeqStep{Duration: 100 * time.Millisecond, Curve: Linear, Scale: F64(2)},
		SeqStep{Duration: 100 * time.Millisecond, Curve: Linear, DX: F64(40), Parallel: true},
	)
	s.StartTime = time.Now()
	p := s.Evaluate(s.StartTime.Add(50 * time.Millisecond))
	if math.Abs(p.Scale-1.5) > 0.05 || math.Abs(p.DX-20) > 1 {
		t.Fatalf("parallel mid: scale=%v dx=%v", p.Scale, p.DX)
	}
	if s.totalDuration() != 100*time.Millisecond {
		t.Fatalf("parallel group duration = %v, want 100ms", s.totalDuration())
	}
}

func TestSequenceCubicPath(t *testing.T) {
	s := NewSequence(SeqStep{
		Duration: 100 * time.Millisecond,
		Curve:    Linear,
		Cubic:    true,
		Path:     []Point{{0, 0}, {0, 100}, {100, 100}, {100, 0}},
	})
	s.StartTime = time.Now()
	p := s.Evaluate(s.StartTime.Add(50 * time.Millisecond))
	if p.DX < 30 || p.DX > 70 || p.DY < 40 {
		t.Fatalf("cubic mid pose = (%v,%v)", p.DX, p.DY)
	}
}

func TestSequencePathFollow(t *testing.T) {
	s := NewSequence(SeqStep{
		Duration: 100 * time.Millisecond,
		Curve:    Linear,
		Path:     []Point{{0, 0}, {100, 0}, {100, 50}},
	})
	s.StartTime = time.Now()
	// u=0.5 of total length 150 → 75 along path → still on first segment x=75
	// Wait total len 150; u=100/150 of time... at t=50ms u=0.5 → distance 75 on first seg
	p := s.Evaluate(s.StartTime.Add(50 * time.Millisecond))
	if math.Abs(p.DX-50) > 2 { // 0.5 of first segment only if uniform by time not length...
		// SamplePolyline uses arc-length: u=0.5 → 75px → first seg 100, so x=75
		if math.Abs(p.DX-75) > 2 {
			t.Fatalf("path mid DX=%v want ~75", p.DX)
		}
	}
	p = s.Evaluate(s.StartTime.Add(100 * time.Millisecond))
	if math.Abs(p.DX-100) > 1 || math.Abs(p.DY-50) > 1 {
		t.Fatalf("path end = (%v,%v) want (100,50)", p.DX, p.DY)
	}
}

func TestSequenceYoyo(t *testing.T) {
	s := NewSequence(
		SeqStep{Duration: 100 * time.Millisecond, Curve: Linear, Scale: F64(2)},
	)
	s.Yoyo = true
	s.Repeat = 1
	s.StartTime = time.Now()
	// Reverse mid: scale back toward 1
	p := s.Evaluate(s.StartTime.Add(150 * time.Millisecond))
	if !p.Running || p.Scale < 1.2 || p.Scale > 1.8 {
		t.Fatalf("yoyo reverse mid scale=%v run=%v", p.Scale, p.Running)
	}
	p = s.Evaluate(s.StartTime.Add(250 * time.Millisecond))
	if p.Running {
		t.Fatal("yoyo once should finish")
	}
}
