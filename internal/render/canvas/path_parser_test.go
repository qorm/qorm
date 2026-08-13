package canvas

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

func TestParseSVGPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		fill        bool
		stroke      bool
		strokeWidth float64
		expectedOps int
	}{
		{
			name:        "Empty path",
			path:        "",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 0,
		},
		{
			name:        "Simple line fill",
			path:        "M 0 0 L 10 0 L 10 10 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 4, // Save, Clip, Paint, Restore
		},
		{
			name:        "Simple line stroke",
			path:        "M 0 0 L 10 0",
			fill:        false,
			stroke:      true,
			strokeWidth: 2,
			expectedOps: 12, // 1 segment (4 ops) + 2 joints (8 ops) = 12 ops
		},
		{
			name:        "Cubic Bezier curve fill",
			path:        "M 0 0 C 5 0 5 10 10 10 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 4, // 1 subpath fill
		},
		{
			name:        "Multiple subpaths",
			path:        "M 0 0 L 10 0 Z M 20 20 L 30 20 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 8, // 2 subpaths filled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := ParseSVGPath(tt.path, tt.fill, tt.stroke, tt.strokeWidth)
			if len(ops) != tt.expectedOps {
				t.Errorf("ParseSVGPath() generated %d ops, expected %d", len(ops), tt.expectedOps)
			}
		})
	}
}

// dPt converts a flattened subpath into a comparable "x,y" string per point.
func dPt(p geom.Point) string {
	return fmt.Sprintf("%.0f,%.0f", p.X, p.Y)
}

func TestParseSVGPathHVCmds(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "horizontal then vertical absolute",
			path: "M 0 0 H 10 V 10 L 0 10 Z",
			want: []string{"0,0", "10,0", "10,10", "0,10", "0,0"},
		},
		{
			name: "horizontal then vertical relative",
			path: "m 10 10 h 20 v 20 Z",
			want: []string{"10,10", "30,10", "30,30", "10,10"},
		},
		{
			name: "implicit repeats after H/V",
			path: "M 0 0 H 10 20 V 5 15",
			want: []string{"0,0", "10,0", "20,0", "20,5", "20,15"},
		},
		{
			name: "relative h/v after absolute start",
			path: "M 100 100 h 10 v -20",
			want: []string{"100,100", "110,100", "110,80"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := parsePathToSubpaths(tt.path)
			if len(sub) != 1 {
				t.Fatalf("expected 1 subpath, got %d", len(sub))
			}
			got := flattenPathString(sub[0])
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d points, got %d: %v", len(tt.want), len(got), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("point %d: expected %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func flattenPathString(pts []geom.Point) []string {
	out := make([]string, 0, len(pts))
	for _, p := range pts {
		out = append(out, dPt(p))
	}
	return out
}

func TestParseSVGPathQuadCmds(t *testing.T) {
	// quadAt is the exact point of a quadratic at parameter t (0 <= t <= 1):
	// (1-t)^2*p0 + 2(1-t)t*c + t^2*p2.
	quadAt := func(p0, c, p2 geom.Point, t float64) geom.Point {
		u := 1 - t
		return geom.Point{
			X: u*u*p0.X + 2*u*t*c.X + t*t*p2.X,
			Y: u*u*p0.Y + 2*u*t*c.Y + t*t*p2.Y,
		}
	}
	contains := func(pts []geom.Point, want geom.Point) bool {
		for _, p := range pts {
			if math.Abs(p.X-want.X) < 0.01 && math.Abs(p.Y-want.Y) < 0.01 {
				return true
			}
		}
		return false
	}
	type curve struct{ start, ctrl, end geom.Point }
	tests := []struct {
		name   string
		path   string
		curves []curve
	}{
		{
			name: "quadratic bezier",
			path: "M 0 0 Q 10 20 20 0",
			curves: []curve{
				{start: geom.Point{}, ctrl: geom.Point{X: 10, Y: 20}, end: geom.Point{X: 20}},
			},
		},
		{
			name: "quadratic relative",
			path: "M 10 10 q 10 20 20 0",
			curves: []curve{
				{start: geom.Point{X: 10, Y: 10}, ctrl: geom.Point{X: 20, Y: 30}, end: geom.Point{X: 30, Y: 10}},
			},
		},
		{
			// T reflects Q's control point about the curve end: control = (200,250).
			name: "smooth quadratic continues the reflect",
			path: "M 0 100 Q 50 0 100 100 T 200 100",
			curves: []curve{
				{start: geom.Point{Y: 100}, ctrl: geom.Point{X: 50}, end: geom.Point{X: 100, Y: 100}},
				{start: geom.Point{X: 100, Y: 100}, ctrl: geom.Point{X: 150, Y: 200}, end: geom.Point{X: 200, Y: 100}},
			},
		},
		{
			// A T with no preceding Q has its control coincident with the
			// start (a straight segment).
			name: "smooth quadratic alone decays to a line",
			path: "M 0 0 T 50 50",
			curves: []curve{
				{start: geom.Point{}, ctrl: geom.Point{}, end: geom.Point{X: 50, Y: 50}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := parsePathToSubpaths(tt.path)
			if len(sub) != 1 {
				t.Fatalf("expected 1 subpath, got %d", len(sub))
			}
			pts := sub[0]
			// Last point of every curve lands exactly (within the flattening
			// grid — the endpoint is a grid node).
			if end := pts[len(pts)-1]; end.X != tt.curves[len(tt.curves)-1].end.X || end.Y != tt.curves[len(tt.curves)-1].end.Y {
				t.Fatalf("final endpoint = (%g,%g), want (%g,%g)", end.X, end.Y, tt.curves[len(tt.curves)-1].end.X, tt.curves[len(tt.curves)-1].end.Y)
			}
			// Flattening uses a 10-segment grid, so only grid-aligned samples
			// coincide exactly with the flattened points.
			for _, c := range tt.curves {
				for _, tq := range []float64{0.2, 0.5, 0.8} {
					if !contains(pts, quadAt(c.start, c.ctrl, c.end, tq)) {
						t.Errorf("missing curve point at t=%.2f for %+v in %v", tq, c, flattenPathString(pts))
					}
				}
			}
		})
	}
}

func TestParseSVGPathSmoothCubic(t *testing.T) {
	// S after C reflects the previous second control about the current point:
	// C 0 10 10 20 20 10 followed by S 40 20 40 0 has control1 = 2*(20,10)-(10,20)
	// = (30,0). The first interior point (t=0.1) shows the reflection's pull.
	path := "M 0 10 C 0 10 10 20 20 10 S 40 20 40 0"
	sub := parsePathToSubpaths(path)
	if len(sub) != 1 {
		t.Fatalf("expected 1 subpath, got %d", len(sub))
	}
	pts := sub[0]
	if len(pts) < 11 {
		t.Fatalf("expected >= 11 points (2 flattened curves), got %d", len(pts))
	}
	// S with no preceding cubic uses the current point as control1: a pure
	// line out of the first curve.
	sub2 := parsePathToSubpaths("M 0 0 S 20 0 20 20")
	if len(sub2) != 1 {
		t.Fatalf("expected 1 subpath for bare S, got %d", len(sub2))
	}
	pts2 := sub2[0]
	if pts2[len(pts2)-1].X != 20 || pts2[len(pts2)-1].Y != 20 {
		t.Errorf("S endpoint wrong: got %s", dPt(pts2[len(pts2)-1]))
	}
	// Relative S: end offset from the current point.
	sub3 := parsePathToSubpaths("M 10 10 s 10 0 20 20")
	if len(sub3) != 1 {
		t.Fatalf("expected 1 subpath for relative S, got %d", len(sub3))
	}
	end := sub3[0][len(sub3[0])-1]
	if end.X != 30 || end.Y != 30 {
		t.Errorf("relative S endpoint wrong: got %s", dPt(end))
	}
}

func TestParseSVGPathEmptyOrMalformed(t *testing.T) {
	// Commands before the opening moveto are dropped; a curve command with
	// too few coordinates contributes nothing.
	if sub := parsePathToSubpaths(""); len(sub) != 0 {
		t.Errorf("expected no subpaths for empty d, got %v", sub)
	}
	if sub := parsePathToSubpaths("L 5 5"); len(sub) != 0 {
		t.Errorf("expected no subpaths before any moveto, got %v", sub)
	}
	if sub := parsePathToSubpaths("M 1 1 C 2 2"); len(sub) != 1 || len(sub[0]) != 1 {
		t.Errorf("expected single moveto point for truncated C, got %v", sub)
	}
	if sub := parsePathToSubpaths("M 0 0 Q 5 5"); len(sub) != 1 || len(sub[0]) != 1 {
		t.Errorf("expected single moveto point for truncated Q, got %v", sub)
	}
}

// TestPathOpsColors guards the coloured op path the path widget rasterizes:
// fill segments are painted with the fill color and stroke segments with the
// stroke color, each group isolated by its own Save/Restore.
func TestPathOpsColors(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	var paintColors []color.RGBA
	ops := pathOps("M 0 0 L 10 0 L 10 10 Z", red, blue, 2)
	for _, o := range ops {
		if c, ok := o.(op.ColorOp); ok {
			paintColors = append(paintColors, c.Color)
		}
	}
	if len(paintColors) == 0 {
		t.Fatal("expected ColorOps in path ops")
	}
	if paintColors[0] != red {
		t.Errorf("first fill ColorOp = %v, want red", paintColors[0])
	}
	if ndx := countPaintOps(ops); ndx == 0 {
		t.Error("expected PaintOps in path ops")
	}
	// A transparent stroke (alpha 0) adds no stroke ColorOps.
	ops2 := pathOps("M 0 0 L 10 0 L 10 10 Z", red, color.RGBA{}, 2)
	colors := 0
	for _, o := range ops2 {
		if _, ok := o.(op.ColorOp); ok {
			colors++
		}
	}
	if colors != 1 {
		t.Errorf("expected 1 ColorOp (fill only), got %d", colors)
	}
	// A transparent fill leaves only stroke colors.
	ops3 := pathOps("M 0 0 L 10 0 Z", color.RGBA{}, blue, 2)
	colors = 0
	for _, o := range ops3 {
		if _, ok := o.(op.ColorOp); ok {
			colors++
		}
	}
	if colors == 0 {
		t.Error("expected stroke ColorOps for transparent fill")
	}
}

func countPaintOps(ops []op.Op) int {
	n := 0
	for _, o := range ops {
		if _, ok := o.(op.PaintOp); ok {
			n++
		}
	}
	return n
}

func TestPathBBox(t *testing.T) {
	// The demos on the canvas-advanced scene. Control points are NOT on the
	// curve, so the bbox tracks the flattened samples (y dips to 100 at the
	// quad midpoint and to 200 at the reflected T midpoint).
	bb := pathBBox("M 50 150 Q 100 50 150 150 T 250 150")
	want := image.Rect(50, 100, 250, 200)
	if bb != want {
		t.Errorf("bbox = %v, want %v", bb, want)
	}
	if got := pathBBox(""); got != (image.Rectangle{}) {
		t.Errorf("empty bbox = %v, want zero", got)
	}
}

func TestTokenizeSVGPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"M10,20", []string{"M", "10", "20"}},
		{"M 10 20", []string{"M", "10", "20"}},
		{"M-10-20", []string{"M", "-10", "-20"}},
		{"M 10.5, -20.5", []string{"M", "10.5", "-20.5"}},
		{"c1,2 3,4", []string{"c", "1", "2", "3", "4"}},
		{"m 1e-5 2", []string{"m", "1e-5", "2"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := tokenizeSVGPath(tt.input)
			if len(tokens) != len(tt.expected) {
				t.Fatalf("Expected %d tokens, got %d: %v", len(tt.expected), len(tokens), tokens)
			}
			for i, tok := range tokens {
				if tok != tt.expected[i] {
					t.Errorf("Token %d: expected %q, got %q", i, tt.expected[i], tok)
				}
			}
		})
	}
}
