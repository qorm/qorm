package canvas

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/qorm/platform/internal/geom"
	"github.com/qorm/platform/internal/op"
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

// pathOps is the coloured cousin of ParseSVGPath: each fill subpath paints
// with fillColor (skipped when transparent), each stroked outline segment
// and round joint with strokeColor at strokeWidth (skipped when transparent
// or width <= 0). The op.ColorOp before every group keeps fill and stroke
// independent inside one display list. Geometry is identical to
// ParseSVGPath — the legacy entry stays unchanged because its callers count
// its op groups.
func pathOps(d string, fillColor, strokeColor color.RGBA, strokeWidth float64) []op.Op {
	var ops []op.Op
	subpaths := parsePathToSubpaths(d)

	if fillColor.A > 0 {
		for _, path := range subpaths {
			if len(path) < 3 {
				continue
			}
			ops = append(ops,
				op.SaveOp{}, op.ColorOp{Color: fillColor},
				op.ClipOp{Rect: computeBBox(path), Poly: path, EvenOdd: false},
				op.PaintOp{}, op.RestoreOp{})
		}
	}

	if strokeColor.A > 0 && strokeWidth > 0 {
		hw := strokeWidth / 2.0
		for _, path := range subpaths {
			if len(path) < 2 {
				continue
			}
			// Stroked segments: the same quad strip ParseSVGPath strokes,
			// painted in strokeColor.
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
				ops = append(ops,
					op.SaveOp{}, op.ColorOp{Color: strokeColor},
					op.ClipOp{Rect: computeBBox(quad), Poly: quad},
					op.PaintOp{}, op.RestoreOp{})
			}
			// Round joints (stroke-linecap/linejoin round).
			for i := 0; i < len(path); i++ {
				p := path[i]
				bbox := image.Rect(
					int(math.Floor(p.X-hw)), int(math.Floor(p.Y-hw)),
					int(math.Ceil(p.X+hw)), int(math.Ceil(p.Y+hw)),
				)
				ops = append(ops,
					op.SaveOp{}, op.ColorOp{Color: strokeColor},
					op.ClipOp{Rect: bbox, EllipseRX: hw, EllipseRY: hw},
					op.PaintOp{}, op.RestoreOp{})
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
	// Smooth-command reflection state (SVG spec): for T only the previous
	// Q/q/T/t command's control point reflects; for S only a previous
	// C/c/S/s's second control point does. A moveto, lineto, closepath or a
	// curve of the other family resets the reflector.
	var lastControl geom.Point
	var prevQuad, prevCubic bool
	started := false // a path must begin with a moveto; commands before it are skipped

	// nextFloat consumes the token at idx: on success it returns the parsed
	// value, on failure (malformed number, or strconv.ErrRange for a value
	// overflowing float64, e.g. "1e309") it returns false — but the token is
	// ALWAYS consumed. A failed parse must never leave the token at idx, or
	// the caller's next loop pass would re-run nextFloat on the same token
	// and spin forever (Defect-1 reproduction "M 0 0 L 1e309 0").
	nextFloat := func() (float64, bool) {
		if idx >= len(tokens) {
			return 0, false
		}
		val, err := strconv.ParseFloat(tokens[idx], 64)
		idx++
		if err != nil {
			return 0, false
		}
		return val, true
	}
	// emit appends a flattened curve to the current subpath, skipping the
	// duplicated start point.
	emit := func(pts []geom.Point) {
		for i := 1; i < len(pts); i++ {
			currentPath = append(currentPath, pts[i])
		}
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
		if cmd != 'M' && cmd != 'm' && !started {
			// An SVG path must open with a moveto; ignore stray segments by
			// draining the pending token (the command letter, if any, was
			// already consumed above).
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
			started = true
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
			prevQuad, prevCubic = false, false

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
			prevQuad, prevCubic = false, false

		case 'H', 'h':
			x, ok := nextFloat()
			if !ok {
				continue
			}
			if cmd == 'h' {
				x += currentPt.X
			}
			currentPt = geom.Point{X: x, Y: currentPt.Y}
			currentPath = append(currentPath, currentPt)
			prevQuad, prevCubic = false, false

		case 'V', 'v':
			y, ok := nextFloat()
			if !ok {
				continue
			}
			if cmd == 'v' {
				y += currentPt.Y
			}
			currentPt = geom.Point{X: currentPt.X, Y: y}
			currentPath = append(currentPath, currentPt)
			prevQuad, prevCubic = false, false

		case 'Q', 'q':
			x1, ok1 := nextFloat()
			y1, ok2 := nextFloat()
			x, ok3 := nextFloat()
			y, ok4 := nextFloat()
			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue
			}
			if cmd == 'q' {
				x1 += currentPt.X
				y1 += currentPt.Y
				x += currentPt.X
				y += currentPt.Y
			}
			ctrl := geom.Point{X: x1, Y: y1}
			endPt := geom.Point{X: x, Y: y}
			emit(flattenQuadBezier(currentPt, ctrl, endPt, 10))
			currentPt = endPt
			lastControl = ctrl
			prevQuad, prevCubic = true, false

		case 'T', 't':
			x, ok1 := nextFloat()
			y, ok2 := nextFloat()
			if !ok1 || !ok2 {
				continue
			}
			// Smooth quadratic: reflect the previous quad control point about
			// the current point; without a Q/q/T/t predecessor the control is
			// coincident with the current point (the curve is a line).
			ctrl := currentPt
			if prevQuad {
				ctrl = geom.Point{X: 2*currentPt.X - lastControl.X, Y: 2*currentPt.Y - lastControl.Y}
			}
			if cmd == 't' {
				x += currentPt.X
				y += currentPt.Y
			}
			endPt := geom.Point{X: x, Y: y}
			emit(flattenQuadBezier(currentPt, ctrl, endPt, 10))
			currentPt = endPt
			lastControl = ctrl
			prevQuad, prevCubic = true, false

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
			ctrl2 := geom.Point{X: x2, Y: y2}
			emit(flattenCubicBezier(currentPt, geom.Point{X: x1, Y: y1}, ctrl2, endPt, 10))
			currentPt = endPt
			lastControl = ctrl2
			prevQuad, prevCubic = false, true

		case 'S', 's':
			x2, ok1 := nextFloat()
			y2, ok2 := nextFloat()
			x, ok3 := nextFloat()
			y, ok4 := nextFloat()
			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue
			}
			// Smooth cubic: first control is the reflection of the previous
			// curve's second control about the current point; without a
			// C/c/S/s predecessor it is the current point itself.
			ctrl1 := currentPt
			if prevCubic {
				ctrl1 = geom.Point{X: 2*currentPt.X - lastControl.X, Y: 2*currentPt.Y - lastControl.Y}
			}
			if cmd == 's' {
				x2 += currentPt.X
				y2 += currentPt.Y
				x += currentPt.X
				y += currentPt.Y
			}
			ctrl2 := geom.Point{X: x2, Y: y2}
			endPt := geom.Point{X: x, Y: y}
			emit(flattenCubicBezier(currentPt, ctrl1, ctrl2, endPt, 10))
			currentPt = endPt
			lastControl = ctrl2
			prevQuad, prevCubic = false, true

		case 'Z', 'z':
			if len(currentPath) > 0 {
				currentPath = append(currentPath, startPt)
				subpaths = append(subpaths, currentPath)
				currentPath = nil
				currentPt = startPt
			}
			prevQuad, prevCubic = false, false
			if !isCmd {
				// Stray numbers after a closepath (an implicit-repeat of Z)
				// are invalid SVG. Consume the token so the loop always
				// advances — "M 0 0 L 5 5 Z 10 10" would otherwise spin on
				// the first bare number.
				idx++
			}

		case 'A', 'a':
			// Elliptical arc: rx, ry, x-axis-rotation, large-arc-flag,
			// sweep-flag, then the endpoint x,y. The five leading
			// parameters are accepted and ignored — arc-to-bezier
			// flattening is deferred MVP scope. The arc is approximated
			// with a straight line (a chord) to its endpoint (the last two
			// parameters, relative to the current point for 'a'), which
			// keeps the subpath connected and subsequent commands aligned.
			// Implicit repeats (more coordinate groups after the first
			// arc) draw further chords from the new current point.
			rx, ok1 := nextFloat()
			ry, ok2 := nextFloat()
			rot, ok3 := nextFloat()
			large, ok4 := nextFloat()
			sweep, ok5 := nextFloat()
			x, ok6 := nextFloat()
			y, ok7 := nextFloat()
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
				continue
			}
			_ = rx
			_ = ry
			_ = rot
			_ = large
			_ = sweep
			if cmd == 'a' {
				x += currentPt.X
				y += currentPt.Y
			}
			currentPt = geom.Point{X: x, Y: y}
			currentPath = append(currentPath, currentPt)
			prevQuad, prevCubic = false, false

		default:
			// Unknown/stray token (an unsupported command letter or the
			// numbers that follow it): skip it. The letter itself was
			// already consumed above; a number token would otherwise
			// re-enter the loop with the same unknown command and never
			// advance. Every iteration consumes at least one token, so no
			// input can hang the parser.
			if !isCmd {
				idx++
			}
		}
	}
	if len(currentPath) > 0 {
		subpaths = append(subpaths, currentPath)
	}

	return subpaths
}

func flattenQuadBezier(p0, p1, p2 geom.Point, segments int) []geom.Point {
	var pts []geom.Point
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		u := 1.0 - t
		x := u*u*p0.X + 2*u*t*p1.X + t*t*p2.X
		y := u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y
		pts = append(pts, geom.Point{X: x, Y: y})
	}
	return pts
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
