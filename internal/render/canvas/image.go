package canvas

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // GIF decoding for image widget src
	_ "image/jpeg" // JPEG decoding for image widget src
	_ "image/png"  // PNG decoding for image widget src
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// imagePlaceholder is the grey box painted when an image src fails to load
// (or names an unsupported source), matching the HTML side's silent broken
// img instead of crashing the frame.
var imagePlaceholder = color.RGBA{229, 229, 234, 255}

// imageReadFile is the disk-read seam: tests replace it to count reads and
// prove the cache serves repeat loads without touching the disk again. The
// WASM build replaces it with a JS-fetch-based reader so the playground
// and games page can load images from HTTP URLs.
var imageReadFile = os.ReadFile

// SetImageReadFile replaces the image disk-read function (caller-owned).
// The WASM build wires it to syscall/js fetch; tests wire it to count
// reads. Pass nil to restore os.ReadFile.
func SetImageReadFile(fn func(string) ([]byte, error)) {
	if fn == nil {
		imageReadFile = os.ReadFile
		return
	}
	imageReadFile = fn
}

// imageWarnOut mirrors the style-warning convention (style.go): one line per
// problem, to stderr in production, captured by tests.
var imageWarnOut io.Writer = os.Stderr

// imageCache memoises decode results by resolved absolute path; a nil Img is
// a NEGATIVE entry (known-bad src) so a failing image neither re-reads the
// disk nor re-warns on every frame. Failed srcs stay failed for the process
// lifetime — a file appearing later needs an app reload, the same trade the
// theme loader makes (engine.go themeFailed).
var (
	imageCacheMu sync.Mutex
	imageCache   = map[string]*image.RGBA{}
	imageWarned  = map[string]bool{}
)

// warnImageOnce reports each distinct image problem exactly once (keyed), so
// the per-frame measure/layout passes never spam the log.
func warnImageOnce(key, format string, args ...any) {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()
	if imageWarned[key] {
		return
	}
	imageWarned[key] = true
	fmt.Fprintf(imageWarnOut, "[qorm canvas] "+format+"\n", args...)
}

// resolveImageSrc maps an author src to a local absolute path INSIDE the app
// directory. Relative paths resolve against the app's BaseDir and must not
// escape it (an agent-authored scene is untrusted input: ../.. traversal or
// an absolute path would turn the renderer into a local-file read primitive
// whose pixels can leak back through screenshots). Remote/scheme sources are
// not loadable by the native renderer and report ok=false.
func resolveImageSrc(src string, rt *runtime.Runtime) (string, bool) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", false
	}
	if strings.Contains(src, "://") || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "//") {
		warnImageOnce("remote:"+src, "image src %q is not a local file; the native renderer loads app-relative files only", src)
		return "", false
	}
	if filepath.IsAbs(src) {
		warnImageOnce("abs:"+src, "image src %q is an absolute path; the native renderer only loads files inside the app directory", src)
		return "", false
	}
	base := ""
	if rt != nil && rt.App != nil {
		base = rt.App.BaseDir
	}
	// Web/WASM path: BaseDir is a URL prefix (e.g. "/games/raiden/"). Join
	// with src using path semantics (not filepath) and skip the jail check.
	if strings.HasPrefix(base, "/") || strings.HasPrefix(base, "http") {
		return strings.TrimRight(base, "/") + "/" + src, true
	}
	if base == "" {
		// In-memory/bundle app with a relative src: unresolvable here.
		warnImageOnce("nobase:"+src, "image src %q is relative but the app has no BaseDir; cannot resolve it", src)
		return "", false
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		warnImageOnce("base:"+base, "image src %q: cannot resolve app BaseDir %q: %v", src, base, err)
		return "", false
	}
	clean := filepath.Clean(filepath.Join(baseAbs, src))
	if clean != baseAbs && !strings.HasPrefix(clean, baseAbs+string(filepath.Separator)) {
		warnImageOnce("escape:"+src, "image src %q escapes the app directory; refusing to load it", src)
		return "", false
	}
	return clean, true
}

// loadImage returns the decoded, straight-pixel bitmap for src, or nil on any
// failure (missing file, undecodable bytes, unsupported source). Results —
// including failures — are cached, so this is cheap to call every frame.
func loadImage(src string, rt *runtime.Runtime) *image.RGBA {
	path, ok := resolveImageSrc(src, rt)
	if !ok {
		return nil
	}

	imageCacheMu.Lock()
	if img, hit := imageCache[path]; hit {
		imageCacheMu.Unlock()
		return img
	}
	imageCacheMu.Unlock()

	img := decodeImageFile(path, src)

	imageCacheMu.Lock()
	imageCache[path] = img
	imageCacheMu.Unlock()
	return img
}

// decodeImageFile reads and decodes one image file into a STRAIGHT
// (non-premultiplied) RGBA — the renderer's buffer convention. Failures warn
// once and return nil (the caller negative-caches).
func decodeImageFile(path, src string) *image.RGBA {
	b, err := imageReadFile(path)
	if err != nil {
		warnImageOnce("load:"+path, "image src %q (%s) failed to load: %v; drawing a placeholder", src, path, err)
		return nil
	}
	// Decompression-bomb guard (R6-E): check the declared dimensions BEFORE
	// decoding — a 70KB PNG can legally inflate to gigapixels, and the
	// allocation happens on the render thread.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		warnImageOnce("decode:"+path, "image src %q (%s) is not a decodable PNG/JPEG: %v; drawing a placeholder", src, path, err)
		return nil
	}
	if px := int64(cfg.Width) * int64(cfg.Height); cfg.Width <= 0 || cfg.Height <= 0 || px > maxImagePixels {
		warnImageOnce("toobig:"+path, "image src %q (%s) declares %dx%d pixels — over the native cap (%d); drawing a placeholder", src, path, cfg.Width, cfg.Height, maxImagePixels)
		return nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		warnImageOnce("decode:"+path, "image src %q (%s) is not a decodable PNG/JPEG: %v; drawing a placeholder", src, path, err)
		return nil
	}
	return toStraightRGBA(decoded)
}

// maxImagePixels caps a decoded image (~64MB of RGBA) so a hostile or
// accidental giant bitmap cannot OOM the render thread.
const maxImagePixels = 16 << 20

// toStraightRGBA converts any decoded image to *image.RGBA holding straight
// (non-premultiplied) bytes. At().RGBA() yields alpha-premultiplied 16-bit
// channels, so the conversion un-premultiplies; an NRGBA source is already
// straight and copies directly.
func toStraightRGBA(src image.Image) *image.RGBA {
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	if nrgba, ok := src.(*image.NRGBA); ok {
		for y := 0; y < sb.Dy(); y++ {
			for x := 0; x < sb.Dx(); x++ {
				c := nrgba.NRGBAAt(sb.Min.X+x, sb.Min.Y+y)
				dst.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, c.A})
			}
		}
		return dst
	}
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			r, g, b, a := src.At(sb.Min.X+x, sb.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			if a != 0xffff {
				// Un-premultiply: straight = premul * 0xffff / a.
				r = r * 0xffff / a
				g = g * 0xffff / a
				b = b * 0xffff / a
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// imageFit normalises the author `fit` prop to a mode graph.Image
// understands. HTML defaults to cover (render_media.go:19); fill/contain/
// cover/none are supported, anything else warns once and degrades to cover.
func imageFit(n *model.Node, rt *runtime.Runtime) string {
	raw, ok := n.Prop("fit")
	if !ok {
		return "cover"
	}
	fit := strings.ToLower(strings.TrimSpace(evalPropStr(raw, rt)))
	switch fit {
	case "", "cover":
		return "cover"
	case "fill", "contain", "none":
		return fit
	default:
		warnImageOnce("fit:"+fit, "image fit %q is not supported by the native renderer (fill/contain/cover/none); using cover", fit)
		return "cover"
	}
}

// imageSrc evaluates the node's interpolated `src` prop ({{state.x}} etc.
// resolve, mirroring the HTML path's interp, render_media.go:15). vars
// carries the repeat-instance scope (item/index) when the image sits inside
// a gridview or list renderItem template.
func imageSrc(n *model.Node, rt *runtime.Runtime, vars map[string]any) string {
	raw, ok := n.Prop("src")
	if !ok {
		return ""
	}
	if vars != nil {
		return strings.TrimSpace(evalPropStrWithVars(raw, rt, vars))
	}
	return strings.TrimSpace(evalPropStr(raw, rt))
}

// MeasureImage is the measure-pass entry point for the image widget: it
// returns the node's content size in physical pixels. The intrinsic image
// size (× scale, like style dims) is used; explicit style width/height still
// win — the generic override in measure() applies them on top of this result.
// Unloaded/failed images report 0×0 so the node collapses like an empty img.
// vars carries the repeat-instance scope when the image is inside a list/gridview
// renderItem (nil outside lists).
func MeasureImage(n *model.Node, rt *runtime.Runtime, scale int, vars map[string]any) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	img := loadImage(imageSrc(n, rt, vars), rt)
	if img == nil {
		return 0, 0
	}
	b := img.Bounds()
	return b.Dx() * scale, b.Dy() * scale
}

// RecordImage is the layout-pass entry point for the image widget: it builds
// the scene-graph shape for the node's content box (width×height physical
// pixels, already resolved by PerformLayout). borderRadius comes from the
// node's parsed style and clips the bitmap like a Rect body. A failed or
// empty src yields the grey placeholder box (failed srcs also warned once at
// load time); empty src adds nothing but the box, mirroring a broken img.
// vars carries the repeat-instance scope when the image is inside a
// list/gridview renderItem (nil outside lists).
func RecordImage(n *model.Node, rt *runtime.Runtime, width, height int, borderRadius float64, vars map[string]any) graph.Node {
	if width <= 0 || height <= 0 {
		return nil
	}
	src := imageSrc(n, rt, vars)
	img := loadImage(src, rt)
	if img == nil || src == "" {
		ph := graph.NewRect()
		ph.Width = float64(width)
		ph.Height = float64(height)
		ph.Fill = imagePlaceholder
		ph.BorderRadius = borderRadius
		return ph
	}
	node := graph.NewImage()
	node.Width = float64(width)
	node.Height = float64(height)
	node.Bitmap = img
	node.Fit = imageFit(n, rt)
	node.BorderRadius = borderRadius
	return node
}
