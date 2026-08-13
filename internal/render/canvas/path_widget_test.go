package canvas

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
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

// TestPathMeasureBBox checks Measure reports the path's own bounding box as
// content size (the demos' explicit style width/height override it later).
func TestPathMeasureBBox(t *testing.T) {
	n := &model.Node{Type: "path", ID: "p",
		Props: map[string]any{
			"d": "M 50 150 Q 100 50 150 150 T 250 150",
		},
	}
	rt := runtime.New(&model.App{Entry: "main"})
	wid, _ := LookupWidget("path")
	w, h := wid.Measure(n, rt, nil, 1)
	if w != 200 || h != 100 {
		t.Errorf("Measure = %dx%d, want 200x100", w, h)
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
