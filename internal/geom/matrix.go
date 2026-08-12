package geom

import "math"

// Point represents a 2D float point
type Point struct {
	X, Y float64
}

// Matrix represents a 2D affine transformation matrix:
// [ A C E ]
// [ B D F ]
// [ 0 0 1 ]
type Matrix struct {
	A, B, C, D, E, F float64
}

// Identity returns an identity matrix
func Identity() Matrix {
	return Matrix{
		A: 1, B: 0,
		C: 0, D: 1,
		E: 0, F: 0,
	}
}

// Multiply multiplies this matrix by another matrix (this * other)
func (m Matrix) Multiply(other Matrix) Matrix {
	return Matrix{
		A: m.A*other.A + m.C*other.B,
		B: m.B*other.A + m.D*other.B,
		C: m.A*other.C + m.C*other.D,
		D: m.B*other.C + m.D*other.D,
		E: m.A*other.E + m.C*other.F + m.E,
		F: m.B*other.E + m.D*other.F + m.F,
	}
}

// Translate returns a translated matrix
func (m Matrix) Translate(tx, ty float64) Matrix {
	return m.Multiply(Matrix{
		A: 1, B: 0,
		C: 0, D: 1,
		E: tx, F: ty,
	})
}

// Scale returns a scaled matrix
func (m Matrix) Scale(sx, sy float64) Matrix {
	return m.Multiply(Matrix{
		A: sx, B: 0,
		C: 0, D: sy,
		E: 0, F: 0,
	})
}

// Rotate returns a rotated matrix (angle in radians)
func (m Matrix) Rotate(angle float64) Matrix {
	sin, cos := math.Sincos(angle)
	return m.Multiply(Matrix{
		A: cos, B: sin,
		C: -sin, D: cos,
		E: 0, F: 0,
	})
}

// Skew returns a skewed matrix (angles in radians). ax shears X by Y
// (CSS skewX), ay shears Y by X (CSS skewY).
func (m Matrix) Skew(ax, ay float64) Matrix {
	return m.Multiply(Matrix{
		A: 1, B: math.Tan(ay),
		C: math.Tan(ax), D: 1,
		E: 0, F: 0,
	})
}

// TransformPoint applies the matrix to a point
func (m Matrix) TransformPoint(p Point) Point {
	return Point{
		X: m.A*p.X + m.C*p.Y + m.E,
		Y: m.B*p.X + m.D*p.Y + m.F,
	}
}

// TransformBBox transforms a bounding box and returns the new AABB that encapsulates it
func (m Matrix) TransformBBox(b BBox) BBox {
	p1 := m.TransformPoint(Point{b.MinX, b.MinY})
	p2 := m.TransformPoint(Point{b.MaxX, b.MinY})
	p3 := m.TransformPoint(Point{b.MaxX, b.MaxY})
	p4 := m.TransformPoint(Point{b.MinX, b.MaxY})

	minX := p1.X
	maxX := p1.X
	minY := p1.Y
	maxY := p1.Y

	for _, p := range []Point{p2, p3, p4} {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	return BBox{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

// Invert returns the inverse matrix. Useful for mapping pointer events back to local space.
func (m Matrix) Invert() (Matrix, bool) {
	det := m.A*m.D - m.B*m.C
	if math.Abs(det) < 1e-6 {
		return Identity(), false // Non-invertible
	}
	invDet := 1.0 / det
	return Matrix{
		A: m.D * invDet,
		B: -m.B * invDet,
		C: -m.C * invDet,
		D: m.A * invDet,
		E: (m.C*m.F - m.D*m.E) * invDet,
		F: (m.B*m.E - m.A*m.F) * invDet,
	}, true
}
