package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/geom"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/render/graph"
)

// SoftwareRenderer must satisfy the Renderer contract.
var _ Renderer = SoftwareRenderer{}

// buildTestOps records a display list exercising the main op kinds:
// filled rect, rounded stroke, opacity and text.
func buildTestOps() *op.Ops {
	ops := &op.Ops{}

	// Filled blue square at (4,4)-(20,20).
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 20, 20)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})

	// Rounded red stroke ring around (30,4)-(50,24), radius 6, width 2.
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.StrokeOp{Width: 2})
	ops.Add(op.ClipOp{Rect: image.Rect(30, 4, 50, 24), Radius: 6})
	ops.Add(op.StrokePaintOp{})
	ops.Add(op.RestoreOp{})

	// Half-transparent green fill at (4,30)-(60,44).
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.OpacityOp{Alpha: 0.5})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 30, 60, 44)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})

	// Text.
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.TextOp{Text: "AB", Pos: image.Pt(4, 46), Scale: 1})

	return ops
}

// The SoftwareRenderer.Render path must produce exactly the same pixels as
// the Rasterize convenience wrapper — the frame loop switches from one to the
// other without any visible change.
func TestSoftwareRendererMatchesRasterize(t *testing.T) {
	size := image.Pt(64, 56)
	want := Rasterize(buildTestOps(), size)

	got := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	SoftwareRenderer{}.Render(buildTestOps(), got)

	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

// Render must start every frame from a clean white buffer — a persistent
// Surface buffer is reused across frames and may never carry residue.
func TestRenderCleansTargetEachFrame(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 32, 32))

	frame1 := &op.Ops{}
	frame1.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	frame1.Add(op.ClipOp{Rect: image.Rect(0, 0, 32, 32)})
	frame1.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(frame1, buf)
	if c := buf.RGBAAt(16, 16); c.R != 255 || c.G != 0 {
		t.Fatalf("frame 1 should paint red, got %v", c)
	}

	// Frame 2 draws nothing: the buffer must come back pure white.
	SoftwareRenderer{}.Render(&op.Ops{}, buf)
	if c := buf.RGBAAt(16, 16); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("frame 2 left residue at (16,16): %v", c)
	}
}

// Rendering into a caller-provided buffer must not allocate a second image:
// the returned target is the same memory that was passed in.
func TestRenderDrawsIntoProvidedBuffer(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 8, 8))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{1, 2, 3, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 8, 8)})
	ops.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(ops, buf)
	if c := buf.RGBAAt(0, 0); c != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("provided buffer not drawn into: %v", c)
	}
}

// A matrix transform must map op geometry into screen space: a translate ×
// scale(2) applied to a 5×5 clip lands at (12,7)-(22,17), doubling the size.
// This is the raster half of the infinite-canvas board's zoom — the graph
// layer already baked the same matrix into GlobalTransform for hit testing.
func TestTransformScalesGeometry(t *testing.T) {
	ops := &op.Ops{}
	ops.Add(op.SaveOp{})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
	ops.Add(op.TransformOp{M: geom.Identity().Translate(10, 5).Scale(2, 2)})
	ops.Add(op.ClipOp{Rect: image.Rect(1, 1, 6, 6)})
	ops.Add(op.PaintOp{})
	ops.Add(op.RestoreOp{})
	img := Rasterize(ops, image.Pt(40, 40))

	blue := color.RGBA{0, 0, 255, 255}
	for _, p := range []image.Point{{12, 7}, {21, 16}, {16, 12}} {
		if c := img.RGBAAt(p.X, p.Y); c != blue {
			t.Errorf("scaled fill missing at %v: got %v, want %v", p, c, blue)
		}
	}
	for _, p := range []image.Point{{11, 6}, {22, 17}, {10, 5}} {
		if c := img.RGBAAt(p.X, p.Y); c == blue {
			t.Errorf("scaled fill overflows at %v (got %v)", p, c)
		}
	}
}

// The graph layer must hand the rasterizer the same matrix it uses for hit
// testing: a Group with ScaleX/ScaleY set (the board's zoom) paints its child
// rect at the scaled screen position. Without this, graph.Draw's old integer
// translate dropped scale at the pixels while HitTest honoured it.
func TestGraphScaleReachesRaster(t *testing.T) {
	ops := &op.Ops{}
	g := graph.NewGroup()
	g.X, g.Y = 10, 5
	g.ScaleX, g.ScaleY = 2, 2
	g.AddChild(func() graph.Node {
		r := graph.NewRect()
		r.Fill = color.RGBA{0, 255, 0, 255}
		r.Width, r.Height = 5, 5
		return r
	}())
	g.Draw(graph.NewContext(ops))
	img := Rasterize(ops, image.Pt(40, 40))

	// The group's local matrix is Translate(10,5)·Scale(2), so the child rect
	// (0,0,5,5) maps to (10,5)-(20,15).
	green := color.RGBA{0, 255, 0, 255}
	for _, p := range []image.Point{{10, 5}, {19, 14}, {15, 10}} {
		if c := img.RGBAAt(p.X, p.Y); c != green {
			t.Errorf("graph-scaled rect missing at %v: got %v, want %v", p, c, green)
		}
	}
	for _, p := range []image.Point{{9, 4}, {20, 15}, {21, 16}} {
		if c := img.RGBAAt(p.X, p.Y); c == green {
			t.Errorf("graph-scaled rect overflows at %v: got %v", p, c)
		}
	}
}

// A translucent fill must alpha-composite over what is already there (the old
// rasterizer used draw.Src and replaced the destination — no blend).
func TestAlphaBlendOver(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 40, 40))
	ops := &op.Ops{}
	// Opaque blue base.
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 255, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 40, 40)})
	ops.Add(op.PaintOp{})
	// Translucent red (alpha 128) over the inner quadrant — pushes a second
	// clip onto the stack, so this also exercises clip nesting.
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 128}})
	ops.Add(op.ClipOp{Rect: image.Rect(10, 10, 30, 30)})
	ops.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(ops, buf)

	// Straight red(128) over straight blue(255) → (128,0,127,255).
	if got := buf.RGBAAt(20, 20); got != (color.RGBA{128, 0, 127, 255}) {
		t.Errorf("blended pixel = %v, want (128,0,127,255)", got)
	}
	// Outside the red clip the blue base is untouched.
	if got := buf.RGBAAt(5, 5); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("base pixel = %v, want blue", got)
	}
}

// Nested clips must intersect: a paint issued under two clips may only land in
// their intersection. (The old rasterizer replaced the clip, so the outer
// clip was ignored and paint leaked outside it.)
func TestNestedClipIntersection(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 160, 160))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 255, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 100, 100)})   // outer
	ops.Add(op.ClipOp{Rect: image.Rect(50, 50, 150, 150)}) // inner
	ops.Add(op.PaintOp{})
	SoftwareRenderer{}.Render(ops, buf)

	green := color.RGBA{0, 255, 0, 255}
	white := color.RGBA{255, 255, 255, 255}
	if got := buf.RGBAAt(75, 75); got != green {
		t.Errorf("intersection pixel = %v, want green", got)
	}
	if got := buf.RGBAAt(25, 25); got != white {
		t.Errorf("outer-only pixel = %v, want white (excluded by inner clip)", got)
	}
	if got := buf.RGBAAt(120, 120); got != white {
		t.Errorf("inner-only pixel = %v, want white (excluded by outer clip)", got)
	}
}

// A rounded-rect stroke must follow the rounded path on both its outer and
// inner edges — the square-corner fast path (four edge rects) may only fire
// for Radius <= 0.
func TestRoundedStrokeFollowsCorner(t *testing.T) {
	buf := image.NewRGBA(image.Rect(0, 0, 48, 48))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{255, 0, 0, 255}})
	ops.Add(op.StrokeOp{Width: 2})
	ops.Add(op.ClipOp{Rect: image.Rect(4, 4, 44, 44), Radius: 8})
	ops.Add(op.StrokePaintOp{})
	SoftwareRenderer{}.Render(ops, buf)

	red := color.RGBA{255, 0, 0, 255}
	white := color.RGBA{255, 255, 255, 255}
	// Square corner point outside the rounded path: a right-angle stroke
	// would paint it (top/left edge bands); the rounded stroke must not.
	if got := buf.RGBAAt(5, 5); got != white {
		t.Errorf("square corner (5,5) = %v, want white — stroke must follow the rounded path", got)
	}
	// On the rounded ring (distance 6.36 from the corner centre) the pixel
	// sits in the inner edge's 1px antialias band [5.5, 6.5]: it must be red
	// at high but PARTIAL coverage — full red was the old binary ring.
	if got := buf.RGBAAt(7, 7); got.R != 255 || got.G == 0 || got.G > 128 || got.B != got.G {
		t.Errorf("ring pixel (7,7) = %v, want high-coverage red (antialiased inner edge)", got)
	}
	// Inside the ring's inner edge (distance² = 32 < 6²): hollow.
	if got := buf.RGBAAt(8, 8); got != white {
		t.Errorf("inner pixel (8,8) = %v, want white (ring is 2px wide)", got)
	}
	// Straight-edge midpoint still paints.
	if got := buf.RGBAAt(24, 5); got != red {
		t.Errorf("edge pixel (24,5) = %v, want red", got)
	}
}

// Text must honour the clip stack and the opacity carried in its color.
func TestTextRespectsClipAndOpacity(t *testing.T) {
	// render "A" twice: once fully opaque, once with zero alpha (the engine
	// pre-multiplies opacity into the color before calling DrawText).
	render := func(alpha uint8) *image.RGBA {
		buf := image.NewRGBA(image.Rect(0, 0, 40, 20))
		ops := &op.Ops{}
		ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, alpha}})
		ops.Add(op.TextOp{Text: "A", Pos: image.Pt(4, 4), Scale: 2})
		SoftwareRenderer{}.Render(ops, buf)
		return buf
	}
	inked := func(buf *image.RGBA) bool {
		for y := 0; y < 20; y++ {
			for x := 0; x < 40; x++ {
				if buf.RGBAAt(x, y) != (color.RGBA{255, 255, 255, 255}) {
					return true
				}
			}
		}
		return false
	}
	if !inked(render(255)) {
		t.Fatal("opaque text should ink pixels")
	}
	if inked(render(0)) {
		t.Error("zero-alpha text must not ink any pixel (opacity wired into text)")
	}

	// A clip that does not cover the text must erase it entirely.
	buf := image.NewRGBA(image.Rect(0, 0, 40, 20))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(30, 0, 40, 20)}) // away from the glyph at x=4
	ops.Add(op.TextOp{Text: "A", Pos: image.Pt(4, 4), Scale: 2})
	SoftwareRenderer{}.Render(ops, buf)
	if inked(buf) {
		t.Error("text outside the active clip must be clipped away")
	}
}

func TestRoundedClipHasCoverageBand(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			src.SetRGBA(x, y, color.RGBA{220, 30, 40, 255})
		}
	}
	ops := &op.Ops{}
	ops.Add(op.ClipOp{Rect: image.Rect(2, 2, 14, 14), Radius: 4})
	ops.Add(op.ImageOp{Src: src, Dest: image.Rect(2, 2, 14, 14)})
	img := Rasterize(ops, image.Pt(16, 16))
	if got := img.RGBAAt(8, 8); got != (color.RGBA{220, 30, 40, 255}) {
		t.Fatalf("rounded image body = %v, want source colour", got)
	}
	// The corner is neither a hard red pixel nor untouched white: coverage
	// must be represented in the edge pixel.
	corner := img.RGBAAt(3, 3)
	if corner == (color.RGBA{255, 255, 255, 255}) || corner == (color.RGBA{220, 30, 40, 255}) {
		t.Fatalf("rounded corner = %v, want intermediate coverage", corner)
	}
}

func TestImageFractionalResizeUsesBilinearSampling(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{0, 0, 0, 255})
	src.SetRGBA(1, 0, color.RGBA{255, 255, 255, 255})
	ops := &op.Ops{}
	ops.Add(op.ImageOp{Src: src, Dest: image.Rect(0, 0, 3, 1)})
	img := Rasterize(ops, image.Pt(3, 1))
	mid := img.RGBAAt(1, 0)
	if mid.R < 80 || mid.R > 180 || mid.G != mid.R || mid.B != mid.R {
		t.Fatalf("fractional resize midpoint = %v, want a blended grey", mid)
	}
}

func TestRRectFastFillOpaque(t *testing.T) {
	ops := &op.Ops{}
	fill := color.RGBA{10, 20, 200, 255}
	ops.Add(op.RRectOp{Rect: image.Rect(8, 6, 40, 30), Fill: fill})
	img := Rasterize(ops, image.Pt(48, 36))
	if got := img.RGBAAt(20, 16); got != fill {
		t.Fatalf("fast fill interior = %v, want %v", got, fill)
	}
	if got := img.RGBAAt(0, 0); got == fill {
		t.Fatalf("outside fast fill = %v, leaked fill", got)
	}
}

func TestImageNearestFastBlitMatchesSlow(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 60), uint8(y * 60), 180, 255})
		}
	}
	// Pixelated 2x scale: dest 8×8. Fast path and the coverage loop must
	// agree on a texel centre.
	ops := &op.Ops{}
	ops.Add(op.ImageOp{Src: src, Dest: image.Rect(0, 0, 8, 8), Pixelated: true})
	img := Rasterize(ops, image.Pt(8, 8))
	want := src.RGBAAt(1, 2)
	if got := img.RGBAAt(3, 5); got != want {
		t.Fatalf("nearest blit (3,5) = %v, want src(1,2)=%v", got, want)
	}
}

func TestCircleUsesAntialiasedRRectPath(t *testing.T) {
	c := graph.NewCircle()
	c.X, c.Y, c.Radius = 4, 4, 8
	c.Fill = color.RGBA{20, 100, 220, 255}
	ops := &op.Ops{}
	c.Draw(graph.NewContext(ops))
	img := Rasterize(ops, image.Pt(20, 20))
	if got := img.RGBAAt(6, 6); got == (color.RGBA{255, 255, 255, 255}) || got == c.Fill {
		t.Fatalf("circle corner = %v, want antialiased intermediate", got)
	}
}
