package widgets

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("icon", Icon{})
}

// Icon renders a named glyph from the built-in 24×24 stroke SVG set — the
// framework's emoji replacement (HTML render_media.go:102 → icons.go). The
// SVG body is flattened and rasterized in pure Go (no new dependencies):
//
//   - path data: M/L/H/V/C/S/Q/A/Z (curves and arcs — endpoint-to-center
//     conversion, SVG spec F.6.5 — are sampled into line segments);
//   - elements: <path>, <circle>, <ellipse>, <rect> (rx rounded);
//   - paint: stroke (width 2, round caps/joins via disk stamping) unless an
//     element sets stroke="none"; fill (nonzero winding scanline) only when
//     an element sets an explicit fill other than "none" — the set's wrapper
//     default (icons.go:72).
//
// Unsupported and documented (none used by the built-in set): T (smooth
// quadratic), per-element stroke-width, line/polyline/polygon elements,
// dash arrays, gradients. Rendering is 4x supersampled then box-downsampled,
// so edges are antialiased. Unknown names keep their measured box, draw a
// grey placeholder (the broken-img convention, canvas/image.go:24) and warn
// exactly once per name.
type Icon struct{}

// iconName mirrors the HTML resolution order (render_media.go:103): icon
// prop, glyph prop, then the node text — bindings evaluated against state.
func iconName(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	for _, k := range []string{"icon", "glyph"} {
		if raw, ok := n.Prop(k); ok {
			if s := strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln)))); s != "" {
				return s
			}
		}
	}
	return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(n.Text, formCtxLn(rt, ln))))
}

// maxIconSide bounds an icon's logical edge. The 4x-supersampled coverage
// map and the final bitmap both grow with the square of the size, and scene
// JSON feeds this directly — an unclamped size is an OOM/DoS on the render
// thread (R7-C).
const maxIconSide = 512

// iconSide resolves the square edge in logical px: the HTML default is the
// `size` prop (propNum 22, render_media.go:107); canvas additionally honors
// style fontSize so the icon follows text sizing, then 22.
func iconSide(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) float64 {
	side := 0.0
	if raw, ok := n.Prop("size"); ok {
		switch v := raw.(type) {
		case float64:
			if v > 0 {
				side = v
			}
		case int:
			if v > 0 {
				side = float64(v)
			}
		case string:
			r := runtime.EvalBinding(v, formCtxLn(rt, ln))
			if f, err := strconv.ParseFloat(fmt.Sprint(r), 64); err == nil && f > 0 {
				side = f
			}
		}
	}
	if side <= 0 {
		if s, ok := styleNumber(n, "fontSize"); ok && s > 0 {
			side = float64(s)
		} else {
			side = 22
		}
	}
	if side > maxIconSide {
		side = maxIconSide
	}
	return side
}

// iconInk resolves the glyph color: author style color (currentColor
// inheritance in HTML), else the theme text color.
func iconInk(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) color.RGBA {
	// Style color wins (author styled the icon), then prop color (the HTML
	// path's propStrOr("color") spelling), then the theme text default.
	if ln != nil && ln.Style.Color.A > 0 {
		return ln.Style.Color
	}
	if raw, ok := n.Prop("color"); ok {
		if s, ok := raw.(string); ok && s != "" {
			return canvas.ResolveColor(s, rt)
		}
	}
	return themeColor(rt, "text", color.RGBA{29, 29, 31, 255})
}

// Measure reports the square side × scale, regardless of whether the name
// resolves (the box survives an unknown name, like a broken img keeps its
// dimensions).
func (Icon) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	side := int(iconSide(n, nil, rt)) * scale
	return side, side
}

// Record rasterizes the named glyph into a bitmap node sized to the resolved
// box, or the grey placeholder for unknown names.
func (Icon) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	w, h := ln.Width, ln.Height
	if max := maxIconSide * scale; w > max || h > max {
		w, h = max, max
	}
	name := iconName(ln.Node, ln, rt)
	// When the icon font has a glyph for this name, render it as a text
	// rune — the bitmap font path draws the icon glyph at the same scale
	// and with the same color as any other text, with no per-frame SVG
	// raster pass.
	if r, ok := canvas.LookupIconRune(name); ok {
		node := draw.NewText()
		node.X = 0
		node.Y = 0
		node.Content = string(r)
		node.FontSize = float64(w)
		node.Fill = iconInk(ln.Node, ln, rt)
		return node
	}
	// Fallback: rasterize the SVG body into a bitmap, same as before.
	body, ok := iconSet[name]
	if !ok {
		warnIconOnce(name)
		ph := draw.NewRect()
		ph.Width = float64(w)
		ph.Height = float64(h)
		ph.BorderRadius = 4 * float64(scale)
		ph.Fill = color.RGBA{229, 229, 234, 255}
		return ph
	}
	node := draw.NewImage()
	node.Width = float64(w)
	node.Height = float64(h)
	node.Bitmap = rasterIcon(body, w, h, iconInk(ln.Node, ln, rt))
	node.Fit = "fill"
	return node
}

// iconWarn* mirror the one-shot warning convention (canvas/style.go:499):
// one line per unknown name for the process lifetime, to stderr in
// production, captured by tests.
var (
	iconWarnMu  sync.Mutex
	iconWarned            = map[string]bool{}
	iconWarnOut io.Writer = os.Stderr
)

func warnIconOnce(name string) {
	iconWarnMu.Lock()
	defer iconWarnMu.Unlock()
	if iconWarned[name] {
		return
	}
	iconWarned[name] = true
	fmt.Fprintf(iconWarnOut, "[qorm widgets] icon %q is not in the built-in icon set; drawing a placeholder\n", name)
}

// ---------------------------------------------------------------------------
// SVG flattening + rasterization (pure Go, zero dependencies)
// ---------------------------------------------------------------------------

type fpoint struct{ x, y float64 }

// subpath is one flattened polyline; closed marks Z-terminated paths.
type subpath struct {
	pts    []fpoint
	closed bool
}

// RasterIcon renders an icon-set SVG body into a w×h straight-alpha bitmap
// at 4x supersampling. Exported for go:generate (tools that batch-convert
// the SVG icon set into bitmap font glyphs).
func RasterIcon(body string, w, h int, ink color.RGBA) *image.RGBA {
	return rasterIcon(body, w, h, ink)
}

// IconSet returns the built-in SVG icon set (name → SVG path body). Exported
// for go:generate so the font-glyph tool can enumerate it.
func IconSet() map[string]string { return iconSet }

// rasterIcon renders an icon-set SVG body into a w×h straight-alpha bitmap
// at 4x supersampling. The 24×24 viewBox maps onto the target box.
func rasterIcon(body string, w, h int, ink color.RGBA) *image.RGBA {
	const ss = 4
	W, H := w*ss, h*ss
	cov := make([]uint8, W*H)
	fx, fy := float64(W)/24, float64(H)/24
	strokes, fills := buildIconGeometry(body, fx, fy)
	fillSubpaths(cov, W, H, fills)
	strokeSubpaths(cov, W, H, strokes, 2*(fx+fy)/2) // set-wide stroke-width 2
	return downsampleIcon(cov, W, H, ss, ink)
}

// buildIconGeometry flattens every supported element into device-space
// subpaths, split by paint mode (the set's wrapper defaults: fill none,
// stroke currentColor).
func buildIconGeometry(body string, fx, fy float64) (strokes, fills []subpath) {
	for _, el := range parseElements(body) {
		var subs []subpath
		switch el.name {
		case "path":
			subs = flattenPath(el.attrs["d"])
		case "circle":
			r := attrNum(el.attrs, "r", 0)
			subs = []subpath{ellipsePath(attrNum(el.attrs, "cx", 0), attrNum(el.attrs, "cy", 0), r, r)}
		case "ellipse":
			subs = []subpath{ellipsePath(attrNum(el.attrs, "cx", 0), attrNum(el.attrs, "cy", 0),
				attrNum(el.attrs, "rx", 0), attrNum(el.attrs, "ry", 0))}
		case "rect":
			subs = []subpath{rectPath(attrNum(el.attrs, "x", 0), attrNum(el.attrs, "y", 0),
				attrNum(el.attrs, "width", 0), attrNum(el.attrs, "height", 0), attrNum(el.attrs, "rx", 0))}
		default:
			continue // line/polyline/polygon: unused by the built-in set
		}
		for i := range subs {
			for j := range subs[i].pts {
				subs[i].pts[j].x *= fx
				subs[i].pts[j].y *= fy
			}
		}
		if f, ok := el.attrs["fill"]; ok && f != "none" {
			fills = append(fills, subs...)
		}
		if el.attrs["stroke"] != "none" {
			strokes = append(strokes, subs...)
		}
	}
	return strokes, fills
}

// svgElement is one parsed self-closing tag (<path d="…"/>).
type svgElement struct {
	name  string
	attrs map[string]string
}

// parseElements scans the icon body for <tag attr="v" …/> elements. The set
// is machine-authored (icons.go), so a forgiving scanner suffices.
func parseElements(s string) []svgElement {
	var out []svgElement
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			return out
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '>')
		if j < 0 {
			return out
		}
		tag := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[:j]), "/"))
		s = s[j+1:]
		name, rest := tag, ""
		if k := strings.IndexAny(tag, " \t"); k >= 0 {
			name, rest = tag[:k], tag[k+1:]
		}
		out = append(out, svgElement{name: name, attrs: parseAttrs(rest)})
	}
}

// parseAttrs reads k="v" pairs (the only attr form the icon set uses).
func parseAttrs(s string) map[string]string {
	m := map[string]string{}
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return m
		}
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			return m
		}
		k := strings.TrimSpace(s[:eq])
		s = strings.TrimSpace(s[eq+1:])
		if !strings.HasPrefix(s, `"`) {
			return m
		}
		end := strings.IndexByte(s[1:], '"')
		if end < 0 {
			return m
		}
		m[k] = s[1 : 1+end]
		s = s[1+end+1:]
	}
}

func attrNum(attrs map[string]string, k string, def float64) float64 {
	if s, ok := attrs[k]; ok {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return v
		}
	}
	return def
}

// ellipsePath samples a full ellipse as a closed subpath.
func ellipsePath(cx, cy, rx, ry float64) subpath {
	const n = 40
	pts := make([]fpoint, 0, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		pts = append(pts, fpoint{cx + rx*math.Cos(a), cy + ry*math.Sin(a)})
	}
	return subpath{pts: pts, closed: true}
}

// rectPath samples a (rounded) rect as a closed subpath; rx clamps to half
// the short side like SVG.
func rectPath(x, y, w, h, rx float64) subpath {
	if w <= 0 || h <= 0 {
		return subpath{}
	}
	if rx <= 0 {
		return subpath{pts: []fpoint{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}, {x, y}}, closed: true}
	}
	if rx > math.Min(w, h)/2 {
		rx = math.Min(w, h) / 2
	}
	arc := func(cx, cy float64, a0, a1 float64) []fpoint {
		const n = 8
		pts := make([]fpoint, 0, n+1)
		for i := 0; i <= n; i++ {
			a := a0 + (a1-a0)*float64(i)/n
			pts = append(pts, fpoint{cx + rx*math.Cos(a), cy + rx*math.Sin(a)})
		}
		return pts
	}
	var pts []fpoint
	pts = append(pts, fpoint{x + rx, y}, fpoint{x + w - rx, y})
	pts = append(pts, arc(x+w-rx, y+rx, -math.Pi/2, 0)[1:]...)
	pts = append(pts, fpoint{x + w, y + h - rx})
	pts = append(pts, arc(x+w-rx, y+h-rx, 0, math.Pi/2)[1:]...)
	pts = append(pts, fpoint{x + rx, y + h})
	pts = append(pts, arc(x+rx, y+h-rx, math.Pi/2, math.Pi)[1:]...)
	pts = append(pts, fpoint{x, y + rx})
	pts = append(pts, arc(x+rx, y+rx, math.Pi, 3*math.Pi/2)[1:]...)
	pts = append(pts, pts[0])
	return subpath{pts: pts, closed: true}
}

// pathParser tokenizes SVG path data on demand.
type pathParser struct {
	s string
	i int
}

func (p *pathParser) skip() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == ',' || p.s[p.i] == '\t' || p.s[p.i] == '\n' || p.s[p.i] == '\r') {
		p.i++
	}
}

// num reads one SVG number: [+-]?(digits[.digits]|.digits)(e[+-]?digits)?
// Packed forms split exactly like the spec: "9-1" at the sign, "5.9.9" at
// the SECOND dot (a number holds at most one decimal point).
func (p *pathParser) num() float64 {
	p.skip()
	start := p.i
	if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
		p.i++
	}
	seenDot := false
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '.' {
			if seenDot {
				break
			}
			seenDot = true
			p.i++
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		p.i++
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
	}
	v, _ := strconv.ParseFloat(p.s[start:p.i], 64)
	return v
}

func isPathCmd(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

// flattenPath parses SVG path data into line-segment subpaths, sampling
// cubics (C/S), quadratics (Q) and arcs (A, via svgArc). Unsupported
// commands (T) terminate parsing of the path, like a broken d in a browser
// (which renders nothing past the error).
func flattenPath(d string) (subs []subpath) {
	p := &pathParser{s: d}
	var cur subpath
	var cx, cy, sx, sy, c2x, c2y float64
	var prevCmd byte
	var haveCtrl bool

	flush := func(closed bool) {
		cur.closed = closed
		if len(cur.pts) > 0 {
			subs = append(subs, cur)
		}
		cur = subpath{}
	}
	lineTo := func(x, y float64) {
		cur.pts = append(cur.pts, fpoint{x, y})
		cx, cy = x, y
	}

	for p.skip(); p.i < len(p.s); p.skip() {
		cmd := prevCmd
		if isPathCmd(p.s[p.i]) {
			cmd = p.s[p.i]
			p.i++
		} else if cmd == 'M' {
			cmd = 'L' // implicit lineto after moveto (SVG 2.1)
		} else if cmd == 'm' {
			cmd = 'l'
		}
		rel := cmd >= 'a' && cmd <= 'z'
		up := cmd
		if rel {
			up = cmd - ('a' - 'A')
		}
		switch up {
		case 'M':
			x, y := p.num(), p.num()
			if rel {
				x += cx
				y += cy
			}
			flush(false)
			cur.pts = append(cur.pts, fpoint{x, y})
			cx, cy, sx, sy = x, y, x, y
		case 'L':
			x, y := p.num(), p.num()
			if rel {
				x += cx
				y += cy
			}
			lineTo(x, y)
		case 'H':
			x := p.num()
			if rel {
				x += cx
			}
			lineTo(x, cy)
		case 'V':
			y := p.num()
			if rel {
				y += cy
			}
			lineTo(cx, y)
		case 'C', 'S':
			var x1, y1, x2, y2, x, y float64
			if up == 'C' {
				x1, y1 = p.num(), p.num()
				if rel {
					x1 += cx
					y1 += cy
				}
			} else if haveCtrl { // S: reflect the previous second control point
				x1, y1 = 2*cx-c2x, 2*cy-c2y
			} else {
				x1, y1 = cx, cy
			}
			x2, y2, x, y = p.num(), p.num(), p.num(), p.num()
			if rel {
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			for _, pt := range sampleCubic(fpoint{cx, cy}, fpoint{x1, y1}, fpoint{x2, y2}, fpoint{x, y}) {
				cur.pts = append(cur.pts, pt)
			}
			cx, cy, c2x, c2y, haveCtrl = x, y, x2, y2, true
			prevCmd = cmd
			continue
		case 'Q':
			x1, y1, x, y := p.num(), p.num(), p.num(), p.num()
			if rel {
				x1 += cx
				y1 += cy
				x += cx
				y += cy
			}
			for _, pt := range sampleQuad(fpoint{cx, cy}, fpoint{x1, y1}, fpoint{x, y}) {
				cur.pts = append(cur.pts, pt)
			}
			cx, cy = x, y
		case 'A':
			rx, ry, rot := p.num(), p.num(), p.num()
			large, sweep := p.num(), p.num()
			x, y := p.num(), p.num()
			if rel {
				x += cx
				y += cy
			}
			for _, pt := range svgArc(cx, cy, rx, ry, rot, large, sweep, x, y) {
				cur.pts = append(cur.pts, pt)
			}
			cx, cy = x, y
		case 'Z':
			lineTo(sx, sy)
			flush(true)
		default:
			return // unsupported command: stop, keep what flattened so far
		}
		haveCtrl = false
		prevCmd = cmd
	}
	flush(false)
	return subs
}

// sampleCubic flattens a cubic Bézier, excluding p0, including p3.
func sampleCubic(p0, p1, p2, p3 fpoint) []fpoint {
	approx := dist(p0, p1) + dist(p1, p2) + dist(p2, p3)
	n := int(approx*2) + 4
	if n > 40 {
		n = 40
	}
	pts := make([]fpoint, 0, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		mt := 1 - t
		pts = append(pts, fpoint{
			mt*mt*mt*p0.x + 3*mt*mt*t*p1.x + 3*mt*t*t*p2.x + t*t*t*p3.x,
			mt*mt*mt*p0.y + 3*mt*mt*t*p1.y + 3*mt*t*t*p2.y + t*t*t*p3.y,
		})
	}
	return pts
}

// sampleQuad flattens a quadratic Bézier, excluding p0, including p2.
func sampleQuad(p0, p1, p2 fpoint) []fpoint {
	approx := dist(p0, p1) + dist(p1, p2)
	n := int(approx*2) + 4
	if n > 32 {
		n = 32
	}
	pts := make([]fpoint, 0, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		mt := 1 - t
		pts = append(pts, fpoint{
			mt*mt*p0.x + 2*mt*t*p1.x + t*t*p2.x,
			mt*mt*p0.y + 2*mt*t*p1.y + t*t*p2.y,
		})
	}
	return pts
}

func dist(a, b fpoint) float64 { return math.Hypot(b.x-a.x, b.y-a.y) }

// svgArc flattens an endpoint-parameterization elliptical arc (SVG spec
// F.6.5) from (x1,y1) to (x2,y2). Returned points EXCLUDE the start.
func svgArc(x1, y1, rx, ry, rot, large, sweep, x2, y2 float64) []fpoint {
	if rx == 0 || ry == 0 || (x1 == x2 && y1 == y2) {
		return []fpoint{{x2, y2}}
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := rot * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy
	if lam := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	co := 0.0
	if den > 0 {
		co = math.Sqrt(math.Max(0, num/den))
	}
	if (large == 1) == (sweep == 1) {
		co = -co
	}
	cxp := co * rx * y1p / ry
	cyp := -co * ry * x1p / rx
	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2
	angle := func(ux, uy, vx, vy float64) float64 {
		d := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		c := (ux*vx + uy*vy) / d
		c = math.Max(-1, math.Min(1, c))
		a := math.Acos(c)
		if ux*vy-uy*vx < 0 {
			a = -a
		}
		return a
	}
	th1 := angle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dth := angle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if sweep == 0 && dth > 0 {
		dth -= 2 * math.Pi
	}
	if sweep == 1 && dth < 0 {
		dth += 2 * math.Pi
	}
	n := int(math.Abs(dth)/0.12) + 1
	if n > 64 {
		n = 64
	}
	pts := make([]fpoint, 0, n)
	for i := 1; i <= n; i++ {
		th := th1 + dth*float64(i)/float64(n)
		pts = append(pts, fpoint{
			cx + rx*math.Cos(th)*cosP - ry*math.Sin(th)*sinP,
			cy + rx*math.Cos(th)*sinP + ry*math.Sin(th)*cosP,
		})
	}
	return pts
}

// fillSubpaths paints the nonzero-winding interior of closed subpaths into
// the coverage buffer (device pixels; sample at pixel centers).
func fillSubpaths(cov []uint8, W, H int, subs []subpath) {
	type xing struct {
		x   float64
		dir int
	}
	for y := 0; y < H; y++ {
		yc := float64(y) + 0.5
		var xs []xing
		for _, sp := range subs {
			pts := sp.pts
			if len(pts) < 2 {
				continue
			}
			// Fill implicitly closes open subpaths (SVG fill rule).
			if pts[0] != pts[len(pts)-1] {
				pts = append(append([]fpoint{}, pts...), pts[0])
			}
			for i := 0; i+1 < len(pts); i++ {
				a, b := pts[i], pts[i+1]
				if a.y == b.y {
					continue
				}
				if (a.y <= yc && b.y > yc) || (b.y <= yc && a.y > yc) {
					x := a.x + (yc-a.y)*(b.x-a.x)/(b.y-a.y)
					d := 1
					if b.y < a.y {
						d = -1
					}
					xs = append(xs, xing{x, d})
				}
			}
		}
		if len(xs) < 2 {
			continue
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
		wind := 0
		for i := 0; i+1 < len(xs); i++ {
			wind += xs[i].dir
			if wind == 0 {
				continue
			}
			for x := int(math.Ceil(xs[i].x - 0.5)); float64(x)+0.5 < xs[i+1].x; x++ {
				if x >= 0 && x < W {
					cov[y*W+x] = 1
				}
			}
		}
	}
}

// strokeSubpaths stamps disks along every polyline — round caps and joins
// fall out of the disk shape, matching the set's stroke-linecap/linejoin.
func strokeSubpaths(cov []uint8, W, H int, subs []subpath, width float64) {
	r := width / 2
	if r < 0.75 {
		r = 0.75
	}
	stamp := func(cx, cy float64) {
		for y := int(math.Floor(cy - r)); y <= int(math.Ceil(cy+r)); y++ {
			if y < 0 || y >= H {
				continue
			}
			for x := int(math.Floor(cx - r)); x <= int(math.Ceil(cx+r)); x++ {
				if x < 0 || x >= W {
					continue
				}
				dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
				if dx*dx+dy*dy <= r*r {
					cov[y*W+x] = 1
				}
			}
		}
	}
	for _, sp := range subs {
		for i := 0; i+1 < len(sp.pts); i++ {
			a, b := sp.pts[i], sp.pts[i+1]
			steps := int(dist(a, b)/0.5) + 1
			for k := 0; k <= steps; k++ {
				t := float64(k) / float64(steps)
				stamp(a.x+(b.x-a.x)*t, a.y+(b.y-a.y)*t)
			}
		}
	}
}

// downsampleIcon box-averages the ss×ss coverage blocks into a straight
// (non-premultiplied) RGBA bitmap — the renderer's buffer convention
// (graph/shapes.go Image).
func downsampleIcon(cov []uint8, W, H, ss int, ink color.RGBA) *image.RGBA {
	w, h := W/ss, H/ss
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum := 0
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					sum += int(cov[(y*ss+dy)*W+x*ss+dx])
				}
			}
			if sum == 0 {
				continue
			}
			a := uint8(uint32(sum) * uint32(ink.A) / uint32(ss*ss))
			img.SetRGBA(x, y, color.RGBA{ink.R, ink.G, ink.B, a})
		}
	}
	return img
}

// iconSet is the built-in 24×24 stroke icon set — a verbatim copy of
// internal/render/icons.go (the HTML renderer's map is unexported; keep the
// two in sync when glyphs change). Same paint contract: fill=none,
// stroke=currentColor, stroke-width=2, round caps/joins.
var iconSet = map[string]string{
	// hardware capabilities
	"camera":      `<path d="M3 8a2 2 0 0 1 2-2h2l1.5-2h7L17 4h2a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><circle cx="12" cy="12.5" r="3.5"/>`,
	"image":       `<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9" r="1.8"/><path d="M4 18l5-5 4 4 3-3 4 4"/>`,
	"video":       `<rect x="2" y="6" width="13" height="12" rx="2"/><path d="M15 10l6-3v10l-6-3z"/>`,
	"mic":         `<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M6 11a6 6 0 0 0 12 0M12 17v4M8 21h8"/>`,
	"location":    `<path d="M12 21s-6-5.2-6-10a6 6 0 0 1 12 0c0 4.8-6 10-6 10z"/><circle cx="12" cy="11" r="2.2"/>`,
	"compass":     `<circle cx="12" cy="12" r="9"/><path d="M15.5 8.5l-2 5-5 2 2-5z"/>`,
	"bluetooth":   `<path d="M7 7l10 10-5 4V3l5 4L7 17"/>`,
	"wifi":        `<path d="M2 8.5a15 15 0 0 1 20 0M5 12a10 10 0 0 1 14 0M8 15.5a5 5 0 0 1 8 0"/><circle cx="12" cy="19" r="1"/>`,
	"nfc":         `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 15V9l5 6V9M17 9v6"/>`,
	"battery":     `<rect x="2" y="7" width="18" height="10" rx="2"/><path d="M22 11v2"/><rect x="4" y="9" width="10" height="6" rx="1" fill="currentColor" stroke="none"/>`,
	"bell":        `<path d="M6 9a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6z"/><path d="M10 19a2 2 0 0 0 4 0"/>`,
	"flashlight":  `<path d="M8 3h8v3l-1.5 3v9a1 1 0 0 1-1 1h-3a1 1 0 0 1-1-1V9L8 6z"/><path d="M11 12h2"/>`,
	"volume":      `<path d="M4 9v6h4l5 4V5L8 9z"/><path d="M16 9a4 4 0 0 1 0 6"/>`,
	"brightness":  `<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2"/>`,
	"share":       `<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="M8.6 10.5l6.8-4M8.6 13.5l6.8 4"/>`,
	"clipboard":   `<rect x="5" y="4" width="14" height="17" rx="2"/><rect x="9" y="2.5" width="6" height="4" rx="1"/><path d="M8 11h8M8 15h6"/>`,
	"copy":        `<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/>`,
	"device":      `<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M11 18h2"/>`,
	"globe":       `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3.5 3 14 0 18M12 3c-3 3.5-3 14 0 18"/>`,
	"sun":         `<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2"/>`,
	"zap":         `<path d="M13 2L4 14h7l-1 8 9-12h-7z"/>`,
	"database":    `<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>`,
	"lock":        `<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`,
	"fingerprint": `<path d="M12 5a7 7 0 0 0-7 7v3M19 12a7 7 0 0 0-3.5-6M8.5 20a10 10 0 0 1-1-4v-4a4.5 4.5 0 0 1 9 0v4M12 12v4a4 4 0 0 0 .5 2"/>`,
	"screenshot":  `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 5V3M17 5V3M7 19v2M17 19v2M3 9h2M19 9h2M3 15h2M19 15h2"/>`,
	// common UI
	"check":         `<path d="M4 12l5 5L20 6"/>`,
	"x":             `<path d="M6 6l12 12M18 6L6 18"/>`,
	"alert":         `<path d="M12 3L2 20h20z"/><path d="M12 10v4"/><path d="M12 17h.01"/>`,
	"info":          `<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.5h.01"/>`,
	"inbox":         `<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.5 5.1L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1z"/>`,
	"trash":         `<path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><path d="M6 6v14a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V6"/><path d="M10 11v6M14 11v6"/>`,
	"mail":          `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 7l9 6 9-6"/>`,
	"folder":        `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`,
	"book":          `<path d="M4 5a2 2 0 0 1 2-2h12v16H6a2 2 0 0 0-2 2z"/><path d="M4 19h14"/>`,
	"menu":          `<path d="M4 6h16M4 12h16M4 18h16"/>`,
	"plus":          `<path d="M12 5v14M5 12h14"/>`,
	"minus":         `<path d="M5 12h14"/>`,
	"search":        `<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>`,
	"settings":      `<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>`,
	"home":          `<path d="M4 11l8-7 8 7M6 10v10h12V10"/>`,
	"user":          `<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`,
	"heart":         `<path d="M12 21C7 17 3 13.5 3 9a4.5 4.5 0 0 1 9-1 4.5 4.5 0 0 1 9 1c0 4.5-4 8-9 12z"/>`,
	"star":          `<path d="M12 3l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 9.2l5.9-.9z"/>`,
	"chevron-right": `<path d="M9 5l7 7-7 7"/>`,
	"chevron-down":  `<path d="M5 9l7 7 7-7"/>`,
	"download":      `<path d="M12 3v12M7 11l5 5 5-5M4 21h16"/>`,
	"upload":        `<path d="M12 21V9M7 13l5-5 5 5M4 4h16"/>`,

	// Game-sprite icons — examples/mario uses these for the board cells.
	// 24x24 silhouettes, drawn with one ink color over the cell's bg, so the
	// mario theme resolves through the framework (no hardcoded colors in
	// user config).
	"mario":  `<rect x="5" y="3" width="14" height="6"/><rect x="4" y="8" width="16" height="2"/><circle cx="12" cy="13" r="5"/><rect x="9" y="14" width="6" height="1.5"/><rect x="6" y="17" width="12" height="5"/><rect x="7" y="22" width="3" height="2"/><rect x="14" y="22" width="3" height="2"/>`,
	"goomba": `<ellipse cx="12" cy="13" rx="7" ry="6"/><ellipse cx="9" cy="12" rx="1.5" ry="2" fill="none" stroke-width="1.4"/><ellipse cx="15" cy="12" rx="1.5" ry="2" fill="none" stroke-width="1.4"/><circle cx="9.5" cy="12.5" r="0.7" fill="none" stroke-width="1.2"/><circle cx="14.5" cy="12.5" r="0.7" fill="none" stroke-width="1.2"/><path d="M8 18q4 -2.5 8 0" fill="none"/><rect x="7" y="20" width="2.5" height="2.5"/><rect x="14.5" y="20" width="2.5" height="2.5"/>`,
	"coin":   `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3" fill="none" stroke-width="2"/>`,
	"brick":  `<rect x="2" y="6" width="20" height="14"/><path d="M2 13h20M8 6v7M16 6v7M4 13v7M12 13v7M20 13v7" fill="none" stroke-width="1.2"/>`,
	"ground": `<rect x="2" y="6" width="20" height="14"/><path d="M2 9h20M5 6v3M14 6v3M3 13v7M9 13v7M17 13v7" fill="none" stroke-width="1.2"/><rect x="2" y="6" width="20" height="2"/>`,
	"flag":   `<rect x="4" y="2" width="2" height="20"/><path d="M6 3h12l-3 4 3 4H6z"/>`,
}

// Inline marks Icon as inline-level (canvas.InlineWidget): flex containers keep its content size.
func (Icon) Inline() {}
