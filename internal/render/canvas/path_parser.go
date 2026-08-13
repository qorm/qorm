package canvas

import (
	"image"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/op"
)

// ParseSVGPath parses an SVG path 'd' string (M, L, C, Z commands) and converts
// it into a sequence of drawing operations compatible with the software rasterizer.
func ParseSVGPath(d string, fill bool, stroke bool, strokeWidth float64) []op.Op {
	var ops []op.Op
	subpaths := parsePathToSubpaths(d)

	if fill {
		for _, path := range subpaths {
			if len(path) < 3 {
				continue
			}
			bbox := computeBBox(path)
			ops = append(ops, op.SaveOp{})
			ops = append(ops, op.ClipOp{
				Rect:    bbox,
				Poly:    path,
				EvenOdd: false,
			})
			ops = append(ops, op.PaintOp{})
			ops = append(ops, op.RestoreOp{})
		}
	}

	if stroke && strokeWidth > 0 {
		hw := strokeWidth / 2.0
		for _, path := range subpaths {
			if len(path) < 2 {
				continue
			}
			// Draw segments
			for i := 0; i < len(path)-1; i++ {
				p1, p2 := path[i], path[i+1]
				if p1 == p2 {
					continue
				}
				dx, dy := p2.X-p1.X, p2.Y-p1.Y
				length := math.Hypot(dx, dy)
				nx, ny := -dy/length*hw, dx/length*hw

				quad := []geom.Point{
					{X: p1.X + nx, Y: p1.Y + ny},
					{X: p2.X + nx, Y: p2.Y + ny},
					{X: p2.X - nx, Y: p2.Y - ny},
					{X: p1.X - nx, Y: p1.Y - ny},
				}
				bbox := computeBBox(quad)
				ops = append(ops, op.SaveOp{})
				ops = append(ops, op.ClipOp{Rect: bbox, Poly: quad})
				ops = append(ops, op.PaintOp{})
				ops = append(ops, op.RestoreOp{})
			}
			// Draw round joints
			for i := 0; i < len(path); i++ {
				p := path[i]
				bbox := image.Rect(
					int(math.Floor(p.X-hw)), int(math.Floor(p.Y-hw)),
					int(math.Ceil(p.X+hw)), int(math.Ceil(p.Y+hw)),
				)
				ops = append(ops, op.SaveOp{})
				ops = append(ops, op.ClipOp{
					Rect:      bbox,
					EllipseRX: hw,
					EllipseRY: hw,
				})
				ops = append(ops, op.PaintOp{})
				ops = append(ops, op.RestoreOp{})
			}
		}
	}

	return ops
}

func computeBBox(path []geom.Point) image.Rectangle {
	if len(path) == 0 {
		return image.Rectangle{}
	}
	minX, minY := path[0].X, path[0].Y
	maxX, maxY := path[0].X, path[0].Y
	for _, p := range path[1:] {
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
	return image.Rect(
		int(math.Floor(minX)), int(math.Floor(minY)),
		int(math.Ceil(maxX)), int(math.Ceil(maxY)),
	)
}

func parsePathToSubpaths(d string) [][]geom.Point {
	var subpaths [][]geom.Point
	var currentPath []geom.Point

	tokens := tokenizeSVGPath(d)
	idx := 0

	var lastCmd byte
	var currentPt geom.Point
	var startPt geom.Point

	nextFloat := func() (float64, bool) {
		if idx >= len(tokens) {
			return 0, false
		}
		val, err := strconv.ParseFloat(tokens[idx], 64)
		if err != nil {
			return 0, false
		}
		idx++
		return val, true
	}

	for idx < len(tokens) {
		token := tokens[idx]
		isCmd := len(token) == 1 && unicode.IsLetter(rune(token[0]))

		var cmd byte
		if isCmd {
			cmd = token[0]
			lastCmd = cmd
			idx++
		} else {
			cmd = lastCmd
			if cmd == 'M' {
				cmd = 'L'
			} else if cmd == 'm' {
				cmd = 'l'
			}
		}

		if cmd == 0 {
			idx++
			continue
		}

		switch cmd {
		case 'M', 'm':
			x, ok1 := nextFloat()
			y, ok2 := nextFloat()
			if !ok1 || !ok2 {
				continue
			}
			if cmd == 'm' {
				x += currentPt.X
				y += currentPt.Y
			}
			if len(currentPath) > 0 {
				subpaths = append(subpaths, currentPath)
			}
			currentPt = geom.Point{X: x, Y: y}
			startPt = currentPt
			currentPath = []geom.Point{currentPt}

		case 'L', 'l':
			x, ok1 := nextFloat()
			y, ok2 := nextFloat()
			if !ok1 || !ok2 {
				continue
			}
			if cmd == 'l' {
				x += currentPt.X
				y += currentPt.Y
			}
			currentPt = geom.Point{X: x, Y: y}
			currentPath = append(currentPath, currentPt)

		case 'C', 'c':
			x1, ok1 := nextFloat()
			y1, ok2 := nextFloat()
			x2, ok3 := nextFloat()
			y2, ok4 := nextFloat()
			x, ok5 := nextFloat()
			y, ok6 := nextFloat()
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
				continue
			}
			if cmd == 'c' {
				x1 += currentPt.X
				y1 += currentPt.Y
				x2 += currentPt.X
				y2 += currentPt.Y
				x += currentPt.X
				y += currentPt.Y
			}
			// Flatten cubic bezier
			endPt := geom.Point{X: x, Y: y}
			pts := flattenCubicBezier(currentPt, geom.Point{X: x1, Y: y1}, geom.Point{X: x2, Y: y2}, endPt, 10)
			for i := 1; i < len(pts); i++ {
				currentPath = append(currentPath, pts[i])
			}
			currentPt = endPt

		case 'Z', 'z':
			if len(currentPath) > 0 {
				currentPath = append(currentPath, startPt)
				subpaths = append(subpaths, currentPath)
				currentPath = nil
				currentPt = startPt
			}
		}
	}
	if len(currentPath) > 0 {
		subpaths = append(subpaths, currentPath)
	}

	return subpaths
}

func flattenCubicBezier(p0, p1, p2, p3 geom.Point, segments int) []geom.Point {
	var pts []geom.Point
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		u := 1.0 - t
		tt := t * t
		uu := u * u
		uuu := uu * u
		ttt := tt * t

		x := uuu*p0.X + 3*uu*t*p1.X + 3*u*tt*p2.X + ttt*p3.X
		y := uuu*p0.Y + 3*uu*t*p1.Y + 3*u*tt*p2.Y + ttt*p3.Y
		pts = append(pts, geom.Point{X: x, Y: y})
	}
	return pts
}

func tokenizeSVGPath(d string) []string {
	var tokens []string
	var current strings.Builder

	for i := 0; i < len(d); i++ {
		c := d[i]
		if unicode.IsSpace(rune(c)) || c == ',' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else if unicode.IsLetter(rune(c)) && c != 'e' && c != 'E' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(c))
		} else if c == '-' {
			// A minus sign can start a new number if current has something
			if current.Len() > 0 && current.String()[current.Len()-1] != 'e' && current.String()[current.Len()-1] != 'E' {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			current.WriteByte(c)
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
