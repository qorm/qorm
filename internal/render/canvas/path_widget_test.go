package canvas

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// pathExists is an assertion helper: any pixel whose green channel dominates
// (the stroke color is #00ff7f, green-heavy) counts.
func countGreenPixels(img *image.RGBA) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.G > 150 && c.G > c.R+50 && c.G > c.B+50 {
				n++
			}
		}
	}
	return n
}

// TestPathWidgetRegistered locks the engine seam: LookupWidget finds "path"
// (canvas engine plus HTML renderer both accept the type, so examples that
// use it compile warning-free).
func TestPathWidgetRegistered(t *testing.T) {
	if _, ok := LookupWidget("path"); !ok {
		t.Fatal("path widget not registered")
	}
}

// TestPathMeasureBBox checks Measure reports the path's extent as a
// 0,0-anchored box (bb.Max.X x bb.Max.Y, the viewBox="0 0 w h" mirror), so an
// offset path keeps its author coordinates and paints whole. The demos'
// explicit style width/height override this later.
func TestPathMeasureBBox(t *testing.T) {
	n := &model.Node{Type: "path", ID: "p",
		Props: map[string]any{
			"d": "M 50 150 Q 100 50 150 150 T 250 150",
		},
	}
	rt := runtime.New(&model.App{Entry: "main"})
	wid, _ := LookupWidget("path")
	w, h := wid.Measure(n, rt, nil, 1)
	if w != 250 || h != 200 {
		t.Errorf("Measure = %dx%d, want 250x200", w, h)
	}
	// An offset (bbox.Min > 0) path sizes the box from its max extent, not
	// its size — the Defect-2 regression (corner sliver). The old size
	// (bb.Dx x bb.Dy) gave 100x100 here and cropped everything past (100,100).
	off := &model.Node{Type: "path", ID: "p2",
		Props: map[string]any{"d": "M 50 50 L 150 150 L 50 150 Z"},
	}
	w2, h2 := wid.Measure(off, rt, nil, 1)
	if w2 != 150 || h2 != 150 {
		t.Errorf("offset-path Measure = %dx%d, want 150x150", w2, h2)
	}
}

// TestPathRecordRasterNotEmpty drives the demo's path node (stroke only,
// transparent fill) through layout and asserts the rasterised image carries
// the green stroke — a fully transparent bitmap would mean Record dropped the
// path.
func TestPathRecordRasterNotEmpty(t *testing.T) {
	n := &model.Node{
		Type: "path",
		ID:   "morph_path",
		Props: map[string]any{
			"d": "M 50 150 Q 100 50 150 150 T 250 150",
		},
		Style: map[string]any{
			"x":           50.0,
			"y":           200.0,
			"width":       300.0,
			"height":      300.0,
			"background":  "transparent",
			"strokeColor": "#00ff7f",
			"strokeWidth": 6.0,
		},
	}
	root := &model.Node{Type: "board", ID: "world",
		Style:    map[string]any{"width": 400.0, "height": 500.0, "background": "#1a1a24"},
		Children: []*model.Node{n},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	s := NewHeadlessSurface(image.Pt(400, 500))
	e.DrawFrame(s)

	imgs, _, _ := GraphImageCount(e.Graph())
	if imgs != 1 {
		t.Fatalf("path graph images = %d, want 1", imgs)
	}
	if green := countGreenPixels(s.Frame()); green < 100 {
		t.Errorf("stroke pixels = %d, want >= 100 (morph path must paint)", green)
	}
}

// TestPathBindingSnap re-evaluates the bound `d` on every frame: flipping the
// state must change the measured shape with no morph interpolation in
// between — a state swap snaps, which is the decided MVP scope.
func TestPathBindingSnap(t *testing.T) {
	n := &model.Node{Type: "path", ID: "p",
		Props: map[string]any{
			"d": "{{ state.mode ? 'M 0 0 L 100 100' : 'M 0 100 L 100 0' }}",
		},
	}
	app := &model.App{Entry: "main", GlobalState: model.GlobalState{Initial: map[string]any{"mode": false}}}
	rt := runtime.New(app)
	w1 := parsePathToSubpaths(svgPathD(n, rt))
	if len(w1[0]) != 2 {
		t.Fatalf("unexpected subpath 1: %v", w1[0])
	}
	if w1[0][1].X != 100 || w1[0][1].Y != 0 {
		t.Errorf("false-branch end = (%g,%g), want (100,0)", w1[0][1].X, w1[0][1].Y)
	}
	rt.State["mode"] = true
	w2 := parsePathToSubpaths(svgPathD(n, rt))
	if w2[0][1].X != 100 || w2[0][1].Y != 100 {
		t.Errorf("true-branch end = (%g,%g), want (100,100)", w2[0][1].X, w2[0][1].Y)
	}
}

// graphImage walks the scene graph and returns the first *graph.Image (the
// path widget's raster is emitted as one image node).
func graphImage(root graph.Node) *graph.Image {
	var found *graph.Image
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if found != nil || n == nil {
			return
		}
		if gi, ok := n.(*graph.Image); ok {
			found = gi
			return
		}
		for _, c := range n.Base().Children {
			walk(c)
		}
	}
	walk(root)
	return found
}

// TestPathOffsetNoExplicitSizeRasterFullShape is the Defect-2 regression: an
// offset path (bbox.Min > 0) with NO explicit style width/height must paint
// the FULL shape, not the corner sliver the old size-box crop left visible.
// Driven through the engine's DrawFrame like the other raster tests: the
// path box comes from Measure, Record rasters into it, and pixel probes
// across the shape extent prove the whole triangle shows.
func TestPathOffsetNoExplicitSizeRasterFullShape(t *testing.T) {
	// Right triangle: legs x=50 and y=150, hypotenuse (50,50)->(150,150).
	// bbox = (50,50)-(150,150); the layout box must be 150x150, NOT 100x100.
	n := &model.Node{
		Type:  "path",
		ID:    "offset_path",
		Props: map[string]any{"d": "M 50 50 L 150 150 L 50 150 Z"},
		Style: map[string]any{
			"x":          10.0,
			"y":          10.0,
			"background": "#ff3333",
		},
	}
	root := &model.Node{Type: "board", ID: "world",
		Style:    map[string]any{"width": 400.0, "height": 500.0, "background": "#101018"},
		Children: []*model.Node{n},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	s := NewHeadlessSurface(image.Pt(400, 500))
	e.DrawFrame(s)

	gi := graphImage(e.Graph())
	if gi == nil {
		t.Fatal("path produced no graph image")
	}
	b := gi.Bitmap.Bounds()
	if b.Dx() != 150 || b.Dy() != 150 {
		t.Fatalf("path raster = %dx%d, want 150x150 (anchored-at-origin box)", b.Dx(), b.Dy())
	}
	red := func(x, y int) bool {
		c := gi.Bitmap.RGBAAt(x, y)
		return c.R > 200 && c.G < 80 && c.B < 80
	}
	// Inside the triangle: any pixel at least 10px from every edge.
	for _, p := range [][2]int{{60, 140}, {80, 120}, {100, 140}, {60, 100}} {
		if !red(p[0], p[1]) {
			t.Errorf("expected shape pixel at (%d,%d), got %v", p[0], p[1], gi.Bitmap.RGBAAt(p[0], p[1]))
		}
	}
	// Outside the triangle: corners and clear margins stay background.
	for _, p := range [][2]int{{0, 0}, {30, 30}, {100, 20}, {10, 140}} {
		if red(p[0], p[1]) {
			t.Errorf("background pixel at (%d,%d) painted as shape", p[0], p[1])
		}
	}
	// The full ~5000px triangle must be visible; the old corner sliver kept
	// only ~1250px of it.
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if red(x, y) {
				total++
			}
		}
	}
	if total < 3000 {
		t.Errorf("shape pixels = %d, want >= 3000 (full triangle visible)", total)
	}
	// End-to-end: the projected shape also shows on the composed frame.
	fc := s.Frame().RGBAAt(100, 140)
	if !(fc.R > 200 && fc.G < 80 && fc.B < 80) {
		t.Errorf("frame pixel at (100,140) = %v, want the shape's red", fc)
	}
	if fc2 := s.Frame().RGBAAt(0, 0); fc2.R > 100 {
		t.Errorf("frame pixel at (0,0) = %v, want background", fc2)
	}
}

// TestPathToxicDDoesNotHangDrawFrame is the Defect-1 repro at the level it
// was originally found: a hostile 'd' (author/state-controlled) drives the
// full engine DrawFrame — the old parser spun forever in it, freezing the
// canvas host. Termination is by construction (the test completing within the
// suite timeout); the frame must still paint normally.
func TestPathToxicDDoesNotHangDrawFrame(t *testing.T) {
	// wantImage: an overflowed coordinate ("1e309") is consumed and dropped,
	// leaving only the moveto — a single point has nothing to stroke, so the
	// widget legitimately paints nothing. Every other hostile d must still
	// produce a drawable stroke.
	toxic := []struct {
		d         string
		wantImage bool
	}{
		{"M 0 0 A 50 50 0 0 1 100 100", true}, // arc chord
		{"M 0 0 L 5 5 Z 10 10", true},         // stray numbers after Z
		{"M 0 0 L 1e309 0", false},            // consumed overflow
		{"M 0 0 L -1e309 -1e309", false},
		{"M 0 0 R 5 5 ??? L 1 2", true}, // unknown command + garbage
	}
	root := &model.Node{Type: "board", ID: "world",
		Style:    map[string]any{"width": 300.0, "height": 200.0, "background": "#101018"},
		Children: []*model.Node{},
	}
	for _, tc := range toxic {
		p := &model.Node{Type: "path", ID: "toxic_path",
			Props: map[string]any{"d": tc.d},
			Style: map[string]any{"x": 10.0, "y": 10.0, "width": 100.0, "height": 100.0, "strokeColor": "#00ff7f", "strokeWidth": 2.0},
		}
		root.Children = []*model.Node{p}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
		rt := runtime.New(app)
		rt.Theme = theme.GetDefault()
		e := NewEngine(rt, SoftwareRenderer{})
		s := NewHeadlessSurface(image.Pt(300, 200))
		e.DrawFrame(s)
		if got := graphImage(e.Graph()) != nil; got != tc.wantImage {
			t.Errorf("d=%q: graph image present = %v, want %v", tc.d, got, tc.wantImage)
		}
	}
}

// TestPathRasterBothColors verifies Record honors both the fill (background)
// and the outline (strokeColor): the fill area paints the fill color and the
// outline the stroke color — a colour-blind render would show only one.
func TestPathRasterBothColors(t *testing.T) {
	n := &model.Node{Type: "path", ID: "p",
		Props: map[string]any{"d": "M 10 10 L 90 10 L 50 90 Z"},
		Style: map[string]any{"background": "#ff0000", "strokeColor": "#00ff00", "strokeWidth": 3.0},
	}
	rt := runtime.New(&model.App{Entry: "main"})
	ln := &LayoutNode{Node: n, Width: 100, Height: 100, Style: parseStyle(n, rt)}
	wid, _ := LookupWidget("path")
	node := wid.Record(ln, rt, 1)
	if node == nil {
		t.Fatal("Record returned nil")
	}
	gi, ok := node.(*graph.Image)
	if !ok {
		t.Fatalf("Record returned %T, want *graph.Image", node)
	}
	red, green := 0, 0
	b := gi.Bitmap.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch c := gi.Bitmap.RGBAAt(x, y); {
			case c.R > 200 && c.G < 60 && c.B < 60:
				red++
			case c.G > 200 && c.R < 60 && c.B < 60:
				green++
			}
		}
	}
	if red < 100 {
		t.Errorf("fill pixels = %d, want >= 100", red)
	}
	if green < 50 {
		t.Errorf("stroke pixels = %d, want >= 50", green)
	}
}
