package anim

import (
	"math"
	"testing"
)

func TestSamplePolylineEndpoints(t *testing.T) {
	pts := []Point{{0, 0}, {100, 0}, {100, 50}}
	p0, _ := SamplePolyline(pts, 0)
	if p0.X != 0 || p0.Y != 0 {
		t.Fatalf("u=0 got %+v", p0)
	}
	p1, _ := SamplePolyline(pts, 1)
	if math.Abs(p1.X-100) > 1e-6 || math.Abs(p1.Y-50) > 1e-6 {
		t.Fatalf("u=1 got %+v", p1)
	}
	// Mid first segment
	pm, tan := SamplePolyline(pts, 0.25) // total len 150; 0.25*150=37.5 on first seg
	if math.Abs(pm.X-37.5) > 0.5 {
		t.Fatalf("mid x=%v want ~37.5", pm.X)
	}
	if math.Abs(tan.X-1) > 0.1 {
		t.Fatalf("tangent should be +X, got %+v", tan)
	}
}

func TestSampleCubicBezierMid(t *testing.T) {
	p0 := Point{0, 0}
	p1 := Point{0, 100}
	p2 := Point{100, 100}
	p3 := Point{100, 0}
	pos, _ := SampleCubicBezier(p0, p1, p2, p3, 0.5)
	// Symmetric curve: mid should be near (50, 75)
	if pos.X < 40 || pos.X > 60 || pos.Y < 60 {
		t.Fatalf("cubic mid = %+v", pos)
	}
}
