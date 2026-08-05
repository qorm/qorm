// One-shot headless renderer: dumps an example app's settled first frame as a
// PNG at 2x scale for AA-quality inspection. Usage: go run ./tmp_aashot <app-dir> <out.png> [theme]
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
	_ "github.com/qorm/qorm/internal/widgets" // register the library widgets (icon/tabs/switch/…)
)

func main() {
	dir, out := os.Args[1], os.Args[2]
	app, err := loader.LoadDir(dir)
	if err != nil {
		panic(err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	if len(os.Args) > 3 {
		if th, err := theme.LoadTheme(os.Args[3]); err == nil {
			rt.Theme = th
		}
	}
	eng := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	sc := 2
	if os.Getenv("AASHOT_SCALE") == "1" {
		sc = 1
	}
	surf := canvas.NewHeadlessSurface(image.Pt(460*sc, 820*sc))
	surf.ScaleFactor = sc
	for i := 0; i < 240; i++ {
		eng.DrawFrame(surf)
		if !eng.Dirty() && !eng.Animating() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Optional tap: go run ./tmp_aashot <dir> <out> [theme] [tapX tapY]
	// (logical coords — scale up to physical before injecting).
	if len(os.Args) > 5 {
		var tx, ty int
		fmt.Sscan(os.Args[4], &tx)
		fmt.Sscan(os.Args[5], &ty)
		px, py := float64(tx*sc), float64(ty*sc)
		hit := eng.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: px, Y: py, Buttons: 1})
		fmt.Fprintf(os.Stderr, "[aashot] press at (%v,%v) -> %v, select frames before settle: %d\n", px, py, hit, len(eng.WidgetFrames("select")))
		var walkM func(n *model.Node)
		walkM = func(n *model.Node) {
			if n == nil {
				return
			}
			if w, ok := canvas.LookupWidget(n.Type); ok {
				if ow, yes := w.(canvas.OverlayWidget); yes {
					fmt.Fprintf(os.Stderr, "[aashot] widget %q id=%q OverlayOpen=%v\n", n.Type, n.ID, ow.OverlayOpen(n, eng.RT))
				}
			}
			for _, c := range n.Children {
				walkM(c)
			}
		}
		walkM(eng.RT.App.EntryRoot())
		// Re-run the layout pipeline directly (post-press state) and render it
		// independently: isolates "engine frame path" vs "layout/overlay mount".
		directOps := &op.Ops{}
		g2, _ := canvas.Layout(directOps, eng.RT.App.EntryRoot(), image.Pt(920, 1640), eng.RT, nil, 2)
		_ = g2
		img2 := canvas.Rasterize(directOps, image.Pt(920, 1640))
		f2, err := os.Create("/tmp/aa_select_direct.png")
		if err == nil {
			png.Encode(f2, img2)
			f2.Close()
		}
		nOps := 0
		if directOps != nil {
			nOps = len(directOps.Operations())
		}
		fmt.Fprintf(os.Stderr, "[aashot] direct layout ops=%d -> /tmp/aa_select_direct.png\n", nOps)
		// Probe the overlay geometry without touching widget internals.
		frames := eng.WidgetFrames("select")
		for m, fr := range frames {
			if w, ok := canvas.LookupWidget("select"); ok {
				if ow, yes := w.(canvas.OverlayWidget); yes && ow.OverlayOpen(m, eng.RT) {
					ln := &canvas.LayoutNode{Node: m, Width: fr.Dx(), Height: fr.Dy(), AbsX: fr.Min.X, AbsY: fr.Min.Y}
					ov := ow.OverlayRecord(ln, eng.RT, 2, fr.Min)
					if ov == nil {
						fmt.Fprintf(os.Stderr, "[aashot] OverlayRecord -> nil\n")
					} else {
						b := ov.Base()
						fmt.Fprintf(os.Stderr, "[aashot] overlay group %.0f,%.0f %.0fx%.0f children=%d\n",
							b.X, b.Y, b.Width, b.Height, len(ov.(*graph.Group).Children))
					}
				}
			}
		}
		eng.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: px, Y: py})
		for i := 0; i < 240; i++ {
			eng.DrawFrame(surf)
			if !eng.Dirty() && !eng.Animating() {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		for m, fr := range eng.WidgetFrames("select") {
			fmt.Fprintf(os.Stderr, "[aashot] select-frame node=%p rect=%v overlay=%v\n", m, fr, fr.Dx() > 900)
		}
	}
	// Optional hover: AASHOT_MOVE="x,y" (logical) injects a pointer move after
	// any tap, settles, then dumps — for hover-state screenshots.
	if mv := os.Getenv("AASHOT_MOVE"); mv != "" {
		var mx, my int
		fmt.Sscanf(mv, "%d,%d", &mx, &my)
		eng.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: float64(mx * sc), Y: float64(my * sc)})
		for i := 0; i < 240; i++ {
			eng.DrawFrame(surf)
			if !eng.Dirty() && !eng.Animating() {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprintf(os.Stderr, "[aashot] moved to %d,%d\n", mx, my)
	}
	eng.MarkDirty()
	eng.DrawFrame(surf)
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, surf.Frame()); err != nil {
		panic(err)
	}
	fmt.Println("wrote", out)
}
