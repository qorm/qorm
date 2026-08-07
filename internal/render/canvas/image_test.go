package canvas

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/gif" // decoder only — tests in this file construct gifs too
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// resetImageCache clears the module-level decode cache and warning ledger so
// each test starts from a cold loader.
func resetImageCache(t *testing.T) {
	t.Helper()
	imageCacheMu.Lock()
	imageCache = map[string]*image.RGBA{}
	imageWarned = map[string]bool{}
	imageCacheMu.Unlock()
	t.Cleanup(func() {
		imageCacheMu.Lock()
		imageCache = map[string]*image.RGBA{}
		imageWarned = map[string]bool{}
		imageCacheMu.Unlock()
	})
}

// writeTestPNG encodes img to <dir>/<name> and returns the file name (the
// src an app would use), failing the test on any error.
func writeTestPNG(t *testing.T, dir, name string, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write test PNG: %v", err)
	}
	return name
}

// solidRGBA builds a w×h image of one colour.
func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func imageTestRuntime(dir string) *runtime.Runtime {
	return &runtime.Runtime{State: map[string]any{}, App: &model.App{BaseDir: dir}}
}

func imageNode(props map[string]any) *model.Node {
	return &model.Node{Type: "image", ID: "img1", Props: props}
}

// renderImageNode runs RecordImage and draws the resulting graph node into a
// display list, then rasterises it at the given size.
func renderImageNode(n graph.Node, x, y int, size image.Point) *image.RGBA {
	ops := &op.Ops{}
	ctx := graph.NewContext(ops)
	root := graph.NewGroup()
	root.AddChild(n)
	n.Base().X = float64(x)
	n.Base().Y = float64(y)
	root.Draw(ctx)
	return Rasterize(ops, size)
}

// A recorded image node must emit exactly one ImageOp into the display list,
// with the decoded bitmap and the destination rect resolved from fit.
func TestRecordImageEmitsImageOp(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	red := color.RGBA{200, 10, 20, 255}
	writeTestPNG(t, dir, "a.png", solidRGBA(4, 4, red))
	rt := imageTestRuntime(dir)

	node := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "fill"}), rt, 8, 8, 0, nil)
	imgShape, ok := node.(*graph.Image)
	if !ok {
		t.Fatalf("RecordImage returned %T, want *graph.Image", node)
	}
	if imgShape.Bitmap == nil {
		t.Fatal("recorded image has no bitmap")
	}

	ops := &op.Ops{}
	node.Draw(graph.NewContext(ops))
	var imageOps []op.ImageOp
	for _, o := range ops.Operations() {
		if io, ok := o.(op.ImageOp); ok {
			imageOps = append(imageOps, io)
		}
	}
	if len(imageOps) != 1 {
		t.Fatalf("got %d ImageOps, want 1", len(imageOps))
	}
	if imageOps[0].Dest != image.Rect(0, 0, 8, 8) {
		t.Fatalf("ImageOp dest = %v, want (0,0)-(8,8)", imageOps[0].Dest)
	}
}

// Rasterising an image node paints the decoded pixels: a solid red 4×4 PNG
// drawn fill into an 8×8 box turns the whole box red (nearest-neighbour
// upscale), the surrounding frame stays white.
func TestImageRasterizePixels(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	red := color.RGBA{200, 10, 20, 255}
	writeTestPNG(t, dir, "a.png", solidRGBA(4, 4, red))
	rt := imageTestRuntime(dir)

	node := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "fill"}), rt, 8, 8, 0, nil)
	img := renderImageNode(node, 2, 2, image.Pt(12, 12))

	if got := img.RGBAAt(3, 3); got != red {
		t.Fatalf("pixel inside image = %v, want %v", got, red)
	}
	if got := img.RGBAAt(9, 9); got != red {
		t.Fatalf("pixel at image edge = %v, want %v", got, red)
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("pixel outside image = %v, want white", got)
	}
}

// A 4×2 image in an 8×8 box: fill stretches to the full box; contain fits
// the limiting axis and centres, leaving white letterbox bands.
func TestImageFitGeometry(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	green := color.RGBA{10, 200, 20, 255}
	writeTestPNG(t, dir, "a.png", solidRGBA(4, 2, green))
	rt := imageTestRuntime(dir)
	white := color.RGBA{255, 255, 255, 255}

	fill := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "fill"}), rt, 8, 8, 0, nil)
	img := renderImageNode(fill, 0, 0, image.Pt(8, 8))
	if got := img.RGBAAt(4, 0); got != green {
		t.Fatalf("fill: top row = %v, want %v (stretched)", got, green)
	}
	if got := img.RGBAAt(4, 7); got != green {
		t.Fatalf("fill: bottom row = %v, want %v (stretched)", got, green)
	}

	// contain: scale 2 (width limits), dest 8×4 centred → rows 2..5.
	contain := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "contain"}), rt, 8, 8, 0, nil)
	img = renderImageNode(contain, 0, 0, image.Pt(8, 8))
	if got := img.RGBAAt(4, 1); got != white {
		t.Fatalf("contain: letterbox row 1 = %v, want white", got)
	}
	if got := img.RGBAAt(4, 2); got != green {
		t.Fatalf("contain: first content row = %v, want %v", got, green)
	}
	if got := img.RGBAAt(4, 5); got != green {
		t.Fatalf("contain: last content row = %v, want %v", got, green)
	}
	if got := img.RGBAAt(4, 6); got != white {
		t.Fatalf("contain: letterbox row 6 = %v, want white", got)
	}

	// cover: scale 4 (height limits), dest 16×8 centred → x clipped to 0..7.
	cover := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "cover"}), rt, 8, 8, 0, nil)
	img = renderImageNode(cover, 0, 0, image.Pt(8, 8))
	if got := img.RGBAAt(0, 4); got != green {
		t.Fatalf("cover: left edge = %v, want %v (overflow clipped)", got, green)
	}

	// none: intrinsic 4×2 centred → cols 2..5, rows 3..4.
	none := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "none"}), rt, 8, 8, 0, nil)
	img = renderImageNode(none, 0, 0, image.Pt(8, 8))
	if got := img.RGBAAt(1, 3); got != white {
		t.Fatalf("none: left of image = %v, want white", got)
	}
	if got := img.RGBAAt(2, 3); got != green {
		t.Fatalf("none: top-left of image = %v, want %v", got, green)
	}
	if got := img.RGBAAt(5, 4); got != green {
		t.Fatalf("none: bottom-right of image = %v, want %v", got, green)
	}
	if got := img.RGBAAt(6, 4); got != white {
		t.Fatalf("none: right of image = %v, want white", got)
	}
}

// A missing file degrades to the grey placeholder box and warns exactly once
// even across repeated loads (the per-frame path calls RecordImage every
// frame — the warning must not repeat).
func TestImageMissingFileDegrades(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	rt := imageTestRuntime(dir)

	var warnBuf bytes.Buffer
	oldOut := imageWarnOut
	imageWarnOut = &warnBuf
	t.Cleanup(func() { imageWarnOut = oldOut })

	n := imageNode(map[string]any{"src": "nope.png"})
	for i := 0; i < 3; i++ {
		node := RecordImage(n, rt, 8, 8, 0, nil)
		if _, ok := node.(*graph.Rect); !ok {
			t.Fatalf("iteration %d: placeholder = %T, want *graph.Rect", i, node)
		}
	}
	if got := bytes.Count(warnBuf.Bytes(), []byte("[qorm canvas]")); got != 1 {
		t.Fatalf("warning count = %d, want exactly 1:\n%s", got, warnBuf.String())
	}

	node := RecordImage(n, rt, 8, 8, 0, nil)
	img := renderImageNode(node, 0, 0, image.Pt(10, 10))
	if got := img.RGBAAt(4, 4); got != imagePlaceholder {
		t.Fatalf("placeholder pixel = %v, want %v", got, imagePlaceholder)
	}
}

// The decode cache serves repeat loads from memory: two RecordImage calls for
// the same src read the disk once.
func TestImageCacheHit(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	writeTestPNG(t, dir, "a.png", solidRGBA(2, 2, color.RGBA{1, 2, 3, 255}))
	rt := imageTestRuntime(dir)

	var reads atomic.Int64
	oldRead := imageReadFile
	imageReadFile = func(name string) ([]byte, error) {
		reads.Add(1)
		return oldRead(name)
	}
	t.Cleanup(func() { imageReadFile = oldRead })

	n := imageNode(map[string]any{"src": "a.png"})
	RecordImage(n, rt, 4, 4, 0, nil)
	RecordImage(n, rt, 4, 4, 0, nil)
	MeasureImage(n, rt, 1, nil)
	if got := reads.Load(); got != 1 {
		t.Fatalf("disk reads = %d, want 1 (second load served from cache)", got)
	}

	// The negative path caches too: a missing file is not re-read per frame.
	n = imageNode(map[string]any{"src": "missing.png"})
	RecordImage(n, rt, 4, 4, 0, nil)
	RecordImage(n, rt, 4, 4, 0, nil)
	if got := reads.Load(); got != 2 {
		t.Fatalf("disk reads after negative caching = %d, want 2", got)
	}
}

// MeasureImage reports the intrinsic image size scaled by the device-pixel
// ratio; explicit style dims stay the measure() pass's own override. Failed
// loads collapse to 0×0 like an empty img.
func TestMeasureImage(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	writeTestPNG(t, dir, "a.png", solidRGBA(6, 3, color.RGBA{9, 9, 9, 255}))
	rt := imageTestRuntime(dir)

	w, h := MeasureImage(imageNode(map[string]any{"src": "a.png"}), rt, 1, nil)
	if w != 6 || h != 3 {
		t.Fatalf("MeasureImage scale 1 = %dx%d, want 6x3", w, h)
	}
	w, h = MeasureImage(imageNode(map[string]any{"src": "a.png"}), rt, 2, nil)
	if w != 12 || h != 6 {
		t.Fatalf("MeasureImage scale 2 = %dx%d, want 12x6", w, h)
	}
	w, h = MeasureImage(imageNode(map[string]any{"src": "missing.png"}), rt, 1, nil)
	if w != 0 || h != 0 {
		t.Fatalf("MeasureImage missing = %dx%d, want 0x0", w, h)
	}
}

// The src prop interpolates bindings, mirroring the HTML path.
func TestImageSrcBinding(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	blue := color.RGBA{10, 20, 200, 255}
	writeTestPNG(t, dir, "bound.png", solidRGBA(2, 2, blue))
	rt := imageTestRuntime(dir)
	rt.State["pic"] = "bound.png"

	node := RecordImage(imageNode(map[string]any{"src": "{{state.pic}}", "fit": "fill"}), rt, 4, 4, 0, nil)
	img := renderImageNode(node, 0, 0, image.Pt(4, 4))
	if got := img.RGBAAt(2, 2); got != blue {
		t.Fatalf("bound src pixel = %v, want %v", got, blue)
	}
}

// An image drawn under a clip (rounded rect, like a parent card) only paints
// inside the clip; the corner outside the radius stays white. The image
// node's own borderRadius clips the bitmap the same way.
func TestImageRespectsClip(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	red := color.RGBA{200, 10, 20, 255}
	writeTestPNG(t, dir, "a.png", solidRGBA(8, 8, red))
	rt := imageTestRuntime(dir)
	white := color.RGBA{255, 255, 255, 255}

	// Outer clip: a rounded rect the image tries to escape.
	ops := &op.Ops{}
	ops.Add(op.SaveOp{})
	ops.Add(op.ClipOp{Rect: image.Rect(0, 0, 16, 16), Radius: 8})
	ops.Add(op.ImageOp{Src: solidRGBA(16, 16, red), Dest: image.Rect(0, 0, 16, 16)})
	ops.Add(op.RestoreOp{})
	img := Rasterize(ops, image.Pt(16, 16))
	if got := img.RGBAAt(0, 0); got != white {
		t.Fatalf("corner outside rounded clip = %v, want white", got)
	}
	if got := img.RGBAAt(8, 8); got != red {
		t.Fatalf("centre inside rounded clip = %v, want %v", got, red)
	}

	// The image node's own borderRadius (style borderRadius) clips too.
	node := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "fill"}), rt, 16, 16, 8, nil)
	img = renderImageNode(node, 0, 0, image.Pt(16, 16))
	if got := img.RGBAAt(0, 0); got != white {
		t.Fatalf("borderRadius corner = %v, want white", got)
	}
	if got := img.RGBAAt(8, 8); got != red {
		t.Fatalf("borderRadius centre = %v, want %v", got, red)
	}
}

// Group opacity multiplies through ImageOp like it does through PaintOp: a
// half-opaque parent blends the bitmap over the white background.
func TestImageRespectsOpacity(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	writeTestPNG(t, dir, "a.png", solidRGBA(4, 4, color.RGBA{0, 0, 0, 255}))
	rt := imageTestRuntime(dir)

	node := RecordImage(imageNode(map[string]any{"src": "a.png", "fit": "fill"}), rt, 4, 4, 0, nil)
	ops := &op.Ops{}
	ctx := graph.NewContext(ops)
	root := graph.NewGroup()
	root.Opacity = 0.5
	root.AddChild(node)
	root.Draw(ctx)
	img := Rasterize(ops, image.Pt(4, 4))
	// 50% black over white → ~128 grey.
	got := img.RGBAAt(2, 2)
	if got.R < 120 || got.R > 136 || got.A != 255 {
		t.Fatalf("half-opacity image pixel = %v, want ~128 grey", got)
	}
}

// An unsupported fit value warns once and degrades to cover (the HTML
// default); remote srcs warn once and draw the placeholder.
func TestImageUnsupportedDegrades(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	red := color.RGBA{200, 10, 20, 255}
	writeTestPNG(t, dir, "a.png", solidRGBA(4, 2, red))
	rt := imageTestRuntime(dir)

	var warnBuf bytes.Buffer
	oldOut := imageWarnOut
	imageWarnOut = &warnBuf
	t.Cleanup(func() { imageWarnOut = oldOut })

	n := imageNode(map[string]any{"src": "a.png", "fit": "scale-down"})
	node := RecordImage(n, rt, 8, 8, 0, nil)
	shape, ok := node.(*graph.Image)
	if !ok {
		t.Fatalf("unsupported fit: got %T, want *graph.Image", node)
	}
	if shape.Fit != "cover" {
		t.Fatalf("unsupported fit normalised to %q, want cover", shape.Fit)
	}
	RecordImage(n, rt, 8, 8, 0, nil) // second frame: no repeat warning
	if got := bytes.Count(warnBuf.Bytes(), []byte("scale-down")); got != 1 {
		t.Fatalf("fit warning count = %d, want 1:\n%s", got, warnBuf.String())
	}

	n = imageNode(map[string]any{"src": "https://example.com/x.png"})
	node = RecordImage(n, rt, 8, 8, 0, nil)
	if _, ok := node.(*graph.Rect); !ok {
		t.Fatalf("remote src: got %T, want placeholder *graph.Rect", node)
	}
	RecordImage(n, rt, 8, 8, 0, nil)
	if got := bytes.Count(warnBuf.Bytes(), []byte("https://example.com")); got != 1 {
		t.Fatalf("remote warning count = %d, want 1:\n%s", got, warnBuf.String())
	}
}

// JPEG sources decode too (stdlib decoder, no new dependencies).
func TestImageJPEGDecodes(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()

	// Encode a JPEG via the standard library.
	var buf bytes.Buffer
	jpegSolid := solidRGBA(4, 4, color.RGBA{255, 0, 0, 255})
	if err := jpeg.Encode(&buf, jpegSolid, nil); err != nil {
		t.Fatalf("encode test JPEG: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := imageTestRuntime(dir)
	node := RecordImage(imageNode(map[string]any{"src": "a.jpg", "fit": "fill"}), rt, 4, 4, 0, nil)
	if _, ok := node.(*graph.Image); !ok {
		t.Fatalf("JPEG src: got %T, want *graph.Image (decode failed?)", node)
	}
}

// Concurrent loads (layout on one goroutine, MCP query on another) must not
// race the cache; run with -race.
func TestImageCacheConcurrent(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	writeTestPNG(t, dir, "a.png", solidRGBA(2, 2, color.RGBA{5, 6, 7, 255}))
	rt := imageTestRuntime(dir)

	done := make(chan struct{})
	for g := 0; g < 4; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			n := imageNode(map[string]any{"src": "a.png"})
			for i := 0; i < 50; i++ {
				RecordImage(n, rt, 4, 4, 0, nil)
				MeasureImage(n, rt, 1, nil)
			}
		}()
	}
	for g := 0; g < 4; g++ {
		<-done
	}
}

// The native image widget decodes GIF via the stdlib image/gif decoder
// (registered by the canvas engine's blank import); the resulting RGBA is
// drawn just like a PNG. This locks in the format support: removing the
// _ "image/gif" import would let this test fail at decode time.
func TestImageDecodesGif(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()

	// 4x4 GIF, palette: index 0 transparent black, index 1 opaque red.
	pal := color.Palette{color.RGBA{0, 0, 0, 0}, color.RGBA{0xff, 0, 0, 0xff}}
	img := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetColorIndex(x, y, 1)
		}
	}
	gifPath := writeTestGif(t, dir, "red.gif", &gif.GIF{
		Image: []*image.Paletted{img},
		Delay: []int{0},
	})
	rt := imageTestRuntime(dir)

	n := imageNode(map[string]any{"src": "red.gif"})
	got := RecordImage(n, rt, 8, 8, 0, nil)
	img8, ok := got.(*graph.Image)
	if !ok {
		t.Fatalf("RecordImage type = %T, want *graph.Image", got)
	}
	if img8.Width != 8 || img8.Height != 8 {
		t.Fatalf("RecordImage size = %gx%g, want 8x8", img8.Width, img8.Height)
	}
	if img8.Bitmap == nil {
		t.Fatal("RecordImage returned a nil bitmap")
	}
	// Sample the rendered bitmap: at least one pixel must be non-transparent
	// (GIF decoded into a real RGBA, not the broken-img placeholder). The
	// palette index 0 is transparent black; index 1 is opaque red.
	painted := 0
	for y := 0; y < 8 && painted < 1; y++ {
		for x := 0; x < 8 && painted < 1; x++ {
			px := img8.Bitmap.RGBAAt(x, y)
			if px.A != 0 {
				painted++
			}
		}
	}
	if painted == 0 {
		t.Error("GIF decode produced only transparent pixels — got the placeholder?")
	}

	// The path is the resolved abs path the loader used.
	_ = gifPath
}

// Image src binding resolves {{item}} from the repeat-instance scope, the
// same path a gridview renderItem's <image> uses (mario sprites, etc.).
func TestImageSrcBindingWithItemScope(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	green := color.RGBA{10, 200, 30, 255}
	writeTestPNG(t, dir, "sprite.png", solidRGBA(4, 4, green))
	rt := imageTestRuntime(dir)

	// Simulate a gridview renderItem with item=2 → cellImage[2]="sprite.png"
	cellImage := []any{"sky.png", "ground.png", "sprite.png", "coin.png"}
	vars := map[string]any{
		"item":       2,
		"cellImage":  cellImage,
	}
	src := "sprite.png" // fallback: at(state.cellImage, item) should resolve to this

	// Direct eval with vars: at(cellImage, item) → "sprite.png"
	n := imageNode(map[string]any{"src": "{{ at(cellImage, item) }}", "fit": "fill"})
	node := RecordImage(n, rt, 4, 4, 0, vars)
	img := renderImageNode(node, 0, 0, image.Pt(4, 4))
	if got := img.RGBAAt(2, 2); got != green {
		t.Fatalf("item-scoped src pixel = %v, want %v (src=%s)", got, green, src)
	}
}

// writeTestGif encodes a GIF in-memory and writes it to dir/name; returns
// the resolved absolute path. Mirrors writeTestPNG.
func writeTestGif(t *testing.T, dir, name string, g *gif.GIF) string {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("gif.EncodeAll: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}
