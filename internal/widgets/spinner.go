package widgets

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("spinner", Spinner{})
}

// Spinner is the indeterminate loading indicator (HTML render_feedback.go
// spinner()): a size×size ring — 3px var(--sep) band with a var(--accent)
// quarter arc — rotating once per second, clockwise (the qorm-spin CSS
// animation). It is an AnimatedWidget: it never settles, so the engine keeps
// the frame loop alive while one is mounted and calls Record every frame.
type Spinner struct{}

// Animating always reports true: an indeterminate spinner never settles.
func (Spinner) Animating() bool { return true }

// spinPeriod is one revolution; spinSweep is the accent arc's width (the
// border-top quarter of the HTML ring).
const (
	spinPeriod = time.Second
	spinSweep  = math.Pi / 2
)

// spinStarts holds each mounted spinner node's clock (map[node]*state per
// the widget-seam contract — the graph is rebuilt every frame, so per-node
// state lives here, keyed by the stable model pointer). spinNow is the clock
// seam: tests freeze it to make frames deterministic (the same convention as
// canvas's imageReadFile). Entries for unmounted nodes linger; they cost one
// map slot each and never misbehave, so no reaping pass runs.
var (
	spinMu     sync.Mutex
	spinStarts = map[*model.Node]time.Time{}
	spinNow    = time.Now
)

// spinAngle returns the accent arc's CENTRE angle in radians, measured
// clockwise from 12 o'clock, after elapsed — one full turn per spinPeriod.
// Pure: tests drive it instead of sleeping.
func spinAngle(elapsed time.Duration) float64 {
	frac := float64(elapsed%spinPeriod) / float64(spinPeriod)
	if frac < 0 {
		frac += 1
	}
	return frac * 2 * math.Pi
}

// spinStartFor returns (registering on first sight) the node's clock origin.
func spinStartFor(n *model.Node) time.Time {
	spinMu.Lock()
	defer spinMu.Unlock()
	if t, ok := spinStarts[n]; ok {
		return t
	}
	t := spinNow()
	spinStarts[n] = t
	return t
}

// Measure reports the square content size: the `size` prop (logical px,
// default 24 like the HTML propNum(n, "size", 24)) times the device scale.
func (Spinner) Measure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	spinStartFor(n) // start the clock at mount, before the first Record
	sz := int(propNumDefault(n, "size", 24)) * scale
	return sz, sz
}

// Record rasterizes the ring into a bitmap at the current clock angle and
// mounts it as a draw.Image. The display list has no arc op, so the ring is
// computed per pixel — a few hundred pixels at the default size, once per
// frame — which keeps the arc honest (exact band, exact sweep) instead of
// approximating it with rect segments.
func (Spinner) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	sz := ln.Width
	if ln.Height < sz {
		sz = ln.Height
	}
	if sz <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	th := 3 * scale // HTML: border:3px
	if th < 1 {
		th = 1
	}
	if th > sz/2 {
		th = sz / 2
	}

	track := themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	arc := progressFillColor(ln.Node, rt) // color prop wins, accent default

	centre := spinAngle(spinNow().Sub(spinStartFor(ln.Node)))
	bmp := rasterSpin(sz, th, centre, track, arc)

	node := draw.NewImage()
	node.Bitmap = bmp
	node.Width = float64(sz)
	node.Height = float64(sz)
	node.Fit = "fill" // bitmap is already pixel-exact for the box
	return node
}

// rasterSpin paints the ring: every pixel inside the [rOut-th, rOut] band
// gets the track colour, and the sweep-wide arc centred on `centre` (radians,
// clockwise from 12 o'clock) gets the accent colour. Pixels outside the band
// stay transparent (the renderer alpha-blends the bitmap over the scene).
func rasterSpin(sz, th int, centre float64, track, arc color.RGBA) *image.RGBA {
	bmp := image.NewRGBA(image.Rect(0, 0, sz, sz))
	c := float64(sz) / 2
	rOut := c
	rIn := c - float64(th)
	start := centre - spinSweep/2 // the arc is centred on `centre` (HTML border-top)
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			dx := float64(x) + 0.5 - c
			dy := float64(y) + 0.5 - c
			r := math.Hypot(dx, dy)
			if r < rIn || r > rOut {
				continue
			}
			// Pixel angle, clockwise from 12 o'clock, folded into [0, 2π).
			a := math.Atan2(dx, -dy)
			rel := math.Mod(a-start, 2*math.Pi)
			if rel < 0 {
				rel += 2 * math.Pi
			}
			if rel < spinSweep {
				bmp.SetRGBA(x, y, arc)
			} else {
				bmp.SetRGBA(x, y, track)
			}
		}
	}
	return bmp
}

// propNumDefault reads a numeric prop with a default, mirroring the HTML
// propNum (render_style.go:994: float64 only, no binding evaluation).
func propNumDefault(n *model.Node, key string, def float64) float64 {
	if v, ok := n.Prop(key); ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return def
}
