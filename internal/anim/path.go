package anim

import "math"

// Point is a 2D sample used by path follow (DOTween DOPath / Godot Path2D).
type Point struct{ X, Y float64 }

// SamplePolyline returns the point at progress u ∈ [0,1] along a polyline,
// plus the unit tangent (for optional orientation). Degenerate paths return
// the origin and tangent (1,0).
func SamplePolyline(pts []Point, u float64) (pos, tan Point) {
	tan = Point{X: 1}
	n := len(pts)
	if n == 0 {
		return
	}
	if n == 1 {
		return pts[0], tan
	}
	if u < 0 {
		u = 0
	}
	if u > 1 {
		u = 1
	}
	// Cumulative lengths.
	seg := make([]float64, n-1)
	total := 0.0
	for i := 0; i < n-1; i++ {
		dx := pts[i+1].X - pts[i].X
		dy := pts[i+1].Y - pts[i].Y
		seg[i] = math.Hypot(dx, dy)
		total += seg[i]
	}
	if total <= 1e-9 {
		return pts[n-1], tan
	}
	target := u * total
	acc := 0.0
	for i := 0; i < n-1; i++ {
		if acc+seg[i] >= target || i == n-2 {
			local := 0.0
			if seg[i] > 1e-9 {
				local = (target - acc) / seg[i]
			}
			if local < 0 {
				local = 0
			}
			if local > 1 {
				local = 1
			}
			a, b := pts[i], pts[i+1]
			pos = Point{X: a.X + (b.X-a.X)*local, Y: a.Y + (b.Y-a.Y)*local}
			tx, ty := b.X-a.X, b.Y-a.Y
			if len := math.Hypot(tx, ty); len > 1e-9 {
				tan = Point{X: tx / len, Y: ty / len}
			}
			return pos, tan
		}
		acc += seg[i]
	}
	return pts[n-1], tan
}

// SampleCubicBezier evaluates a cubic Bezier (P0,P1,P2,P3) at u ∈ [0,1]
// and returns position + approximate unit tangent.
func SampleCubicBezier(p0, p1, p2, p3 Point, u float64) (pos, tan Point) {
	if u < 0 {
		u = 0
	}
	if u > 1 {
		u = 1
	}
	ou := 1 - u
	// B(u) = (1-u)³P0 + 3(1-u)²u P1 + 3(1-u)u² P2 + u³ P3
	pos = Point{
		X: ou*ou*ou*p0.X + 3*ou*ou*u*p1.X + 3*ou*u*u*p2.X + u*u*u*p3.X,
		Y: ou*ou*ou*p0.Y + 3*ou*ou*u*p1.Y + 3*ou*u*u*p2.Y + u*u*u*p3.Y,
	}
	// B'(u) = 3(1-u)²(P1-P0) + 6(1-u)u(P2-P1) + 3u²(P3-P2)
	tx := 3*ou*ou*(p1.X-p0.X) + 6*ou*u*(p2.X-p1.X) + 3*u*u*(p3.X-p2.X)
	ty := 3*ou*ou*(p1.Y-p0.Y) + 6*ou*u*(p2.Y-p1.Y) + 3*u*u*(p3.Y-p2.Y)
	if len := math.Hypot(tx, ty); len > 1e-9 {
		tan = Point{X: tx / len, Y: ty / len}
	} else {
		tan = Point{X: 1}
	}
	return pos, tan
}

// SamplePath picks polyline (≥2 pts that are not a single cubic quartet) or
// cubic Bezier when exactly 4 points and cubic=true.
func SamplePath(pts []Point, u float64, cubic bool) (pos, tan Point) {
	if cubic && len(pts) == 4 {
		return SampleCubicBezier(pts[0], pts[1], pts[2], pts[3], u)
	}
	return SamplePolyline(pts, u)
}
