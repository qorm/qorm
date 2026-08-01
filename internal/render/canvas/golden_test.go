package canvas

// Whole-frame golden regression for the canvas engine (the gap called out in
// planning/rendering-engine/reports/wip-and-testing.md §4: nothing pinned the
// rendered APPEARANCE of a full frame). Each scenario renders a real example
// app into a fixed-size HeadlessSurface and the entire frame buffer is
// SHA-256'd against testdata/golden/frames.sha256 (one `<scene> <hex>` line
// per scenario). The engine is deterministic — software rasterizer, fixed
// size, embedded bitmap/sfnt fonts identical on every platform — so any hash
// drift is a real rendering change, intended or not.
//
// Re-baseline after an intentional rendering change:
//
//	QORM_GOLDEN_UPDATE=1 go test -run TestGolden ./internal/render/canvas/
//
// Update mode also drops the frames as PNGs in testdata/golden/png/ for human
// spot-checks (gitignored — see testdata/golden/.gitignore).

import (
	"crypto/sha256"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// goldenSize is the fixed logical size every scenario renders at (the counter
// app's own desktop window size): determinism comes from never deriving it
// from the environment.
var goldenSize = image.Pt(400, 500)

// goldenSceneOrder fixes both the scenario set and their order in
// frames.sha256, so the baseline file is byte-stable across update runs.
var goldenSceneOrder = []string{"counter_light", "counter_dark", "counter_physics", "gallery_if"}

// goldenFrame is one captured frame: the hash that goes into the baseline and
// a private pixel copy (surfaces reuse their buffer across frames, so the PNG
// dump cannot hold the surface's *image.RGBA).
type goldenFrame struct {
	sum [32]byte
	img *image.RGBA
}

// requireGoldenFontStack skips the whole group when the sfnt text engine is
// absent. The hashes pin the font stack: under the qorm_nocjk build tag (or a
// checkout missing the embedded subset font, the other case activeTTFEngine
// is nil) the bitmap fallback measures and rasterizes text differently, and
// every hash legitimately differs. Comparing there would fail on valid
// frames, so the group skips instead — nocjk coverage stays on the pixel
// tests, which use tag-gated expectations.
func requireGoldenFontStack(t *testing.T) {
	t.Helper()
	if activeTTFEngine() == nil {
		t.Skip("golden frames pin the sfnt font stack; qorm_nocjk / missing subset font uses the bitmap stack with different (valid) hashes")
	}
}

// settleGoldenFrames drives DrawFrame until the engine is fully idle: no
// pending redraw, no tween in flight. Tweens are wall-clock based
// (anim.Controller stamps time.Now at Reset), so a MID-flight frame is
// timing-dependent and unhashable — but a finished controller snaps the style
// to its exact target (UpdateAndGetAnimatedStyle returns TargetStyle once
// isRunning is false), making the settled frame deterministic. The loop is
// bounded so a regression that animates forever fails loudly instead of
// hanging the suite. This replaces the e2e test's fixed 300ms sleep with an
// actual completion condition (Engine.Animating).
func settleGoldenFrames(t *testing.T, e *Engine, surf *HeadlessSurface) {
	t.Helper()
	// 240 × 25ms = 6s worst case; a theme tween settles in ~300ms.
	for i := 0; i < 240; i++ {
		e.DrawFrame(surf)
		if !e.Dirty() && !e.Animating() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("engine did not settle: still dirty or animating after 6s")
}

// captureGoldenFrame settles the engine, forces one full repaint of the
// settled state (a mid-frame physics dispatch writes state after layout, so
// the settle loop's own last frame can predate it — the forced repaint is the
// frame that provably carries every settled write) and hashes the whole
// buffer. The pixels are copied out before later frames reuse the buffer.
func captureGoldenFrame(t *testing.T, e *Engine, surf *HeadlessSurface) goldenFrame {
	t.Helper()
	settleGoldenFrames(t, e, surf)
	e.MarkDirty()
	e.DrawFrame(surf)
	src := surf.Frame()
	img := image.NewRGBA(src.Rect)
	copy(img.Pix, src.Pix)
	return goldenFrame{sum: sha256.Sum256(img.Pix), img: img}
}

// newGoldenEngine loads the example at dir (relative to the repo root — the
// test chdirs there) and builds the headless engine the same way the existing
// end-to-end tests do.
func newGoldenEngine(t *testing.T, dir string) (*Engine, *HeadlessSurface) {
	t.Helper()
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load %s: %v", dir, err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(goldenSize)
}

func TestGoldenFrames(t *testing.T) {
	requireGoldenFontStack(t)

	// Resolve the baseline dir BEFORE chdir: themes/<name>.json and the
	// examples must load from the repo root (counter ships no own themes/, so
	// its apple-light/apple-dark skins resolve from the root themes/ dir —
	// the disk JSON being authoritative over the built-in palette is exactly
	// what TestThemeFileWinsOverBuiltinAdopt pins).
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join("..", "..", ".."))

	frames := map[string]goldenFrame{}

	// counter_light / counter_dark: ONE engine, the main scene's first frame,
	// then a single toggle_theme dispatch (state.theme apple-light ->
	// apple-dark, both loaded from the root themes/ dir on the next frame).
	// The dispatch alone does not dirty the engine — state.set has no frame
	// sink while rt.Commit is unwired (mirroring the e2e tests) — so the
	// driver requests the frame itself.
	e, surf := newGoldenEngine(t, filepath.Join("examples", "counter"))
	frames["counter_light"] = captureGoldenFrame(t, e, surf)
	e.RT.Dispatch("toggle_theme", nil)
	e.MarkDirty()
	frames["counter_dark"] = captureGoldenFrame(t, e, surf)

	// counter_physics: drive increment until the moving block collides, then
	// hash the frame showing "COLLISION DETECTED!". The e2e convergence test
	// (TestCounterPhysicsCollisionEndToEnd) dispatches in a tight loop and
	// lets wall-clock pacing decide WHICH increment collides — the physics
	// pass judges overlap on the ANIMATED box, so the count at impact (and
	// with it the block's resting margin, -20*count) drifts run to run. Here
	// every increment is fully settled before the next, so the pass only ever
	// sees exact target positions and the collision lands on a deterministic
	// count.
	pe, psurf := newGoldenEngine(t, filepath.Join("examples", "counter"))
	pe.RT.Navigate("physics", nil)
	for i := 0; i < 40 && pe.RT.State["status"] != "COLLISION DETECTED!"; i++ {
		pe.RT.Dispatch("increment", nil)
		pe.MarkDirty() // same no-frame-sink reasoning as toggle_theme above
		settleGoldenFrames(t, pe, psurf)
	}
	if got := pe.RT.State["status"]; got != "COLLISION DETECTED!" {
		t.Fatalf("physics status = %v, want %q (the collision must converge for the golden to exist)", got, "COLLISION DETECTED!")
	}
	frames["counter_physics"] = captureGoldenFrame(t, pe, psurf)

	// gallery_if: the gallery main scene carries a conditional subtree
	// ("if": "{{state.agree}}", agree=false initially, so the node is hidden)
	// — the golden pins both the conditional path and the form widgets the
	// counter never renders. No animated_container in this scene, so the
	// first settled frame is the steady state.
	ge, gsurf := newGoldenEngine(t, filepath.Join("examples", "gallery"))
	frames["gallery_if"] = captureGoldenFrame(t, ge, gsurf)

	if os.Getenv("QORM_GOLDEN_UPDATE") == "1" {
		writeGoldenBaseline(t, goldenDir, frames)
		return
	}
	compareGoldenBaseline(t, goldenDir, frames)
}

// writeGoldenBaseline rewrites frames.sha256 in goldenSceneOrder and drops
// each frame as a PNG under png/ for human spot-checks. The PNGs are
// gitignored (testdata/golden/.gitignore): the hash file is the baseline, the
// frames are reproducible from it plus the code, and committing derived
// binaries would only bloat the repo.
func writeGoldenBaseline(t *testing.T, dir string, frames map[string]goldenFrame) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "png"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, name := range goldenSceneOrder {
		f, ok := frames[name]
		if !ok {
			t.Fatalf("scenario %q produced no frame", name)
		}
		b.WriteString(name + " " + hexSum(f.sum) + "\n")
		if err := writeGoldenPNG(filepath.Join(dir, "png", name+".png"), f.img); err != nil {
			t.Fatalf("dump %s PNG: %v", name, err)
		}
	}
	path := filepath.Join(dir, "frames.sha256")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("golden baseline rewritten: %s (PNGs in %s)", path, filepath.Join(dir, "png"))
}

// compareGoldenBaseline checks every captured frame against the baseline. On
// a mismatch the ACTUAL frame is written to t.TempDir and the error names the
// path plus both hashes, so a rendering diff can be eyeballed immediately.
func compareGoldenBaseline(t *testing.T, dir string, frames map[string]goldenFrame) {
	t.Helper()
	path := filepath.Join(dir, "frames.sha256")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden baseline: %v (first run? re-baseline with: QORM_GOLDEN_UPDATE=1 go test -run TestGolden ./internal/render/canvas/)", err)
	}
	want := map[string]string{}
	for ln, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("%s:%d: malformed line %q, want `<scene> <hex>`", path, ln+1, line)
		}
		want[fields[0]] = fields[1]
	}
	if len(want) != len(goldenSceneOrder) {
		t.Fatalf("%s lists %d scenes, the test renders %d — re-baseline with QORM_GOLDEN_UPDATE=1", path, len(want), len(goldenSceneOrder))
	}
	for _, name := range goldenSceneOrder {
		expected, ok := want[name]
		if !ok {
			t.Errorf("%s: missing from %s — re-baseline with QORM_GOLDEN_UPDATE=1", name, path)
			continue
		}
		f := frames[name]
		if got := hexSum(f.sum); got != expected {
			pngPath := filepath.Join(t.TempDir(), name+".actual.png")
			if err := writeGoldenPNG(pngPath, f.img); err != nil {
				t.Errorf("%s: dump actual frame: %v", name, err)
			}
			t.Errorf("golden frame %q changed:\n  actual   %s\n  expected %s\n  actual frame PNG: %s (t.TempDir is removed when the test finishes; QORM_GOLDEN_UPDATE=1 keeps persistent PNGs in testdata/golden/png/)",
				name, got, expected, pngPath)
		}
	}
}

func hexSum(sum [32]byte) string {
	const hexdigits = "0123456789abcdef"
	var b [64]byte
	for i, v := range sum {
		b[2*i], b[2*i+1] = hexdigits[v>>4], hexdigits[v&0xF]
	}
	return string(b[:])
}

func writeGoldenPNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
