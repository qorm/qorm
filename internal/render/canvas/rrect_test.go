package canvas

// Pixel tests for op.RRectOp: per-pixel SDF coverage — antialiased rounded
// corners and smooth (not stepped) shadow falloff.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
)

func renderRRect(t *testing.T, size image.Point, rr op.RRectOp) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops := &op.Ops{}
	ops.Add(rr)
	SoftwareRenderer{}.Render(ops, img)
	return img
}

func TestRRectCornerAntialiasing(t *testing.T) {
	img := renderRRect(t, image.Pt(40, 40), op.RRectOp{
		Rect: image.Rect(4, 4, 36, 36), Radius: 8,
		Fill: color.RGBA{255, 0, 0, 255},
	})

	// The exact corner must be background (outside the rounded shape).
	if got := img.RGBAAt(4, 4); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("outer corner = %v, want background", got)
	}
	// Along the corner arc there must be intermediate-coverage pixels — the
	// antialiasing band the binary clip path never produced.
	mid := 0
	for y := 4; y < 14; y++ {
		for x := 4; x < 14; x++ {
			// Red fill over the white scene: R stays 255, so intermediate
			// coverage shows up in the green channel.
			if c := img.RGBAAt(x, y); c.G > 0 && c.G < 255 {
				mid++
			}
		}
	}
	if mid == 0 {
		t.Error("no intermediate-coverage pixels on the corner arc — edges are still binary")
	}
	// The body is fully opaque.
	if got := img.RGBAAt(20, 20); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("body = %v, want opaque fill", got)
	}
}

func TestRRectRotatedKeepsCorners(t *testing.T) {
	// A square with large radius, rotated 45° about its center, must not paint
	// a filled axis-aligned diamond AABB — the centre stays fill, far AABB
	// corners stay background.
	const size = 80
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	ops := &op.Ops{}
	// Local 40×40 rrect, then rotate 45° about its centre (20,20), then
	// translate to canvas centre (40,40).
	// Matrix: T(40,40) · R(π/4) · T(-20,-20) applied as local→screen.
	// Ops apply as Scale then Rotate then Translate when recorded via graph;
	// here we post-multiply: start Identity, Translate, Rotate, Translate.
	// Software: currentMatrix = currentMatrix.Multiply(o.M) so later ops apply first
	// to the point. Graph: Translate(X,Y).Rotate.Scale.Scale so point: Scale → Rotate → Translate.
	// Emulate: T(cx,cy) * R * T(-hw,-hh) * localRect(0,0,40,40)
	cx, cy, half := 40.0, 40.0, 20.0
	ang := math.Pi / 4
	// Build matrix as Identity.Translate(cx,cy).Rotate(ang).Translate(-half,-half)
	// Multiply order left-to-right means apply rightmost first to points.
	m := geom.Identity().Translate(cx, cy).Rotate(ang).Translate(-half, -half)
	ops.Add(op.TransformOp{M: m})
	ops.Add(op.RRectOp{
		Rect: image.Rect(0, 0, 40, 40), Radius: 12,
		Fill: color.RGBA{255, 0, 0, 255},
	})
	SoftwareRenderer{}.Render(ops, img)

	// Center of canvas should be red (inside rotated square).
	if got := img.RGBAAt(40, 40); got.R < 200 {
		t.Errorf("center = %v, want red fill", got)
	}
	// Far corner of AABB (top-left of canvas) must stay background white —
	// axis-aligned SDF on the AABB would incorrectly fill toward corners.
	if got := img.RGBAAt(2, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("far AABB corner = %v, want background (rotated rrect leak)", got)
	}
	// Mid-top of canvas is outside a 40×40 square rotated 45° about centre
	// (half-diagonal ~28px; distance to mid-top is 35px). Must stay white —
	// axis-aligned SDF on the AABB would paint a diamond that reaches here.
	if got := img.RGBAAt(40, 5); got.R > 200 && got.G < 50 {
		t.Errorf("mid-top = %v, should be outside rotated square (background)", got)
	}
	if got := img.RGBAAt(40, 5); got != (color.RGBA{255, 255, 255, 255}) {
		// Soft AA near edge is OK; pure red fill is not.
		if got.R > 240 && got.G < 30 {
			t.Errorf("mid-top filled solid red = %v", got)
		}
	}
}

func TestRRectShadowXOffset(t *testing.T) {
	// Shadow offset to the right should darken pixels right of the box, not left.
	img := renderRRect(t, image.Pt(80, 60), op.RRectOp{
		Rect: image.Rect(20, 10, 50, 40), Radius: 2,
		Fill:       color.RGBA{255, 255, 255, 255},
		Shadow:     color.RGBA{0, 0, 0, 180},
		ShadowBlur: 6,
		ShadowX:    10,
		ShadowY:    0,
	})
	// Right of shape (x=58): shadow present (darker than white).
	rRight, _, _, _ := img.At(58, 25).RGBA()
	// Left of shape (x=12): little/no shadow.
	rLeft, _, _, _ := img.At(12, 25).RGBA()
	if rRight>>8 >= 250 {
		t.Errorf("expected shadow to the right of the box, R=%d", rRight>>8)
	}
	if rLeft>>8 < rRight>>8 {
		t.Errorf("shadow should be stronger on the right (left R=%d right R=%d)", rLeft>>8, rRight>>8)
	}
}

func TestRRectRadialGradient(t *testing.T) {
	img := renderRRect(t, image.Pt(40, 40), op.RRectOp{
		Rect: image.Rect(4, 4, 36, 36), Radius: 0,
		Fill:           color.RGBA{0, 0, 0, 255},
		GradientStops:  []color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}},
		GradientRadial: true,
	})
	// Center should be closer to red; near corner closer to blue.
	c := img.RGBAAt(20, 20)
	e := img.RGBAAt(6, 6)
	if c.R <= e.R {
		t.Errorf("radial center should be redder than edge: center=%v edge=%v", c, e)
	}
}

func TestRRectShadowIsSmoothNotStepped(t *testing.T) {
	img := renderRRect(t, image.Pt(60, 60), op.RRectOp{
		Rect: image.Rect(10, 10, 40, 40), Radius: 4,
		Fill:       color.RGBA{255, 255, 255, 255},
		Shadow:     color.RGBA{0, 0, 0, 128},
		ShadowBlur: 8,
		ShadowY:    2,
	})

	// The buffer is opaque, so shadow strength shows as DARKNESS: marching
	// outward below the bottom edge, luminance must INCREASE monotonically
	// (the shadow melts out) and take more than the old fake's 3 steps.
	var lum []uint32
	for y := 41; y < 55; y++ {
		r, _, _, _ := img.At(25, y).RGBA()
		lum = append(lum, r>>8)
	}
	distinct := map[uint32]bool{}
	for i, v := range lum {
		if v < 255 {
			distinct[v] = true
		}
		if i > 0 && v < lum[i-1] {
			t.Errorf("shadow gets darker outward at y=%d: %v", 41+i, lum)
		}
	}
	if len(distinct) < 5 {
		t.Errorf("shadow has only %d distinct levels (old fake had 3) — not smooth: %v", len(distinct), lum)
	}
	// The shadow hangs below the shape (ShadowY=2): nothing above the top.
	if r, _, _, _ := img.At(25, 2).RGBA(); r>>8 < 250 {
		t.Errorf("unexpected shadow above the shape (R=%d)", r>>8)
	}
}

func TestRRectStrokeRingWithAA(t *testing.T) {
	img := renderRRect(t, image.Pt(40, 40), op.RRectOp{
		Rect: image.Rect(5, 5, 35, 35), Radius: 6,
		Fill:        color.RGBA{255, 255, 255, 255},
		Stroke:      color.RGBA{0, 0, 255, 255},
		StrokeWidth: 2,
	})
	// Mid-edge: the ring is blue.
	if got := img.RGBAAt(20, 6); got.B < 200 {
		t.Errorf("stroke ring pixel = %v, want blue ring", got)
	}
	// Deep inside: white fill.
	if got := img.RGBAAt(20, 20); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("inner = %v, want fill", got)
	}
	// Outside: background.
	if got := img.RGBAAt(20, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("outer = %v, want background", got)
	}
}

func TestSDRoundBox(t *testing.T) {
	// 10x10 box at origin, radius 2.
	if d := sdRoundBox(0, 0, 0, 0, 5, 5, 2); d >= 0 {
		t.Errorf("center d=%v, want < 0", d)
	}
	if d := sdRoundBox(5, 0, 0, 0, 5, 5, 2); d != 0 {
		t.Errorf("edge midpoint d=%v, want 0", d)
	}
	if d := sdRoundBox(9, 9, 0, 0, 5, 5, 2); d <= 0 {
		t.Errorf("far corner d=%v, want > 0", d)
	}
	// Radius clamped to 0 behaves as a sharp box.
	if d := sdRoundBox(5.5, 5.5, 0, 0, 5, 5, 0); d <= 0 {
		t.Errorf("sharp corner point d=%v, want > 0 (outside sharp corner)", d)
	}
}

// R6-D: an agent-authored scene is untrusted input — image src must stay
// inside the app directory (no ../ escape, no absolute paths).
func TestImageSrcConfinedToBaseDir(t *testing.T) {
	dir := t.TempDir()
	app := &model.App{Entry: "main", BaseDir: dir, Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	rt := runtime.New(app)

	if p, ok := resolveImageSrc("assets/pic.png", rt); !ok || !strings.HasPrefix(p, filepath.Clean(dir)) {
		t.Errorf("inside-path src = %q, ok=%v; want resolvable inside BaseDir", p, ok)
	}
	if _, ok := resolveImageSrc("../../etc/passwd", rt); ok {
		t.Error("../ escape src must be refused")
	}
	if _, ok := resolveImageSrc("/etc/passwd", rt); ok {
		t.Error("absolute-path src must be refused")
	}
	if _, ok := resolveImageSrc("..", rt); ok {
		t.Error("bare .. src must be refused")
	}
}

// R6-E: check declared dimensions before decoding — a tiny PNG can legally
// inflate to gigapixels (decompression bomb on the render thread).
func TestImageDecodeRejectsHugeDimensions(t *testing.T) {
	// Hand-build a minimal PNG whose IHDR declares 100000x100000 (config-only;
	// no IDAT needed for DecodeConfig).
	iw := image.NewRGBA(image.Rect(0, 0, 1, 1))
	_ = iw
	var hdr bytes.Buffer
	w := func(b ...byte) { hdr.Write(b) }
	w(137, 80, 78, 71, 13, 10, 26, 10) // signature
	ihdr := []byte("IHDR")
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], 100000)
	binary.BigEndian.PutUint32(data[4:8], 100000)
	data[8] = 8 // bit depth
	data[9] = 6 // color type RGBA
	chunk := append(append([]byte{}, ihdr...), data...)
	binary.Write(&hdr, binary.BigEndian, uint32(len(data)))
	hdr.Write(chunk)
	binary.Write(&hdr, binary.BigEndian, crc32.ChecksumIEEE(chunk))

	path := filepath.Join(t.TempDir(), "bomb.png")
	if err := os.WriteFile(path, hdr.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := decodeImageFile(path, "bomb.png"); got != nil {
		t.Error("a PNG declaring 1e10 pixels must be refused before decoding")
	}
}

// R6-C: a NaN scroll delta must not pass the clamps and poison the persisted
// viewport offset.
func TestScrollNaNDeltaDropped(t *testing.T) {
	e, surf, sv := scrollFixture(t, tallChildren(10, 50))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	e.HandleScroll(ScrollInput{DY: math.NaN()})
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 0 {
		t.Errorf("NaN delta wrote offset %v", off.Y)
	}
	e.HandleScroll(ScrollInput{DY: math.Inf(1)})
	if off := e.Inter.ScrollOffsets[sv]; off.Y != 0 {
		t.Errorf("+Inf delta wrote offset %v", off.Y)
	}
}
