package canvas

import (
	"image"
	"math"
	"testing"

	"github.com/qorm/platform/internal/geom"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/runtime"
)

func TestParseClipPathPolygon(t *testing.T) {
	kind, _, _, _, _, poly, evenOdd, ok := parseClipPath("polygon(50% 0%, 100% 100%, 0% 100%)", 80, 80)
	if !ok || kind != "polygon" || evenOdd || len(poly) != 3 {
		t.Fatalf("triangle: kind=%s ok=%v evenOdd=%v n=%d", kind, ok, evenOdd, len(poly))
	}
	want := [][2]float64{{40, 0}, {80, 80}, {0, 80}}
	for i, p := range want {
		if math.Abs(poly[i][0]-p[0]) > 1e-9 || math.Abs(poly[i][1]-p[1]) > 1e-9 {
			t.Errorf("pt[%d]=%v want %v", i, poly[i], p)
		}
	}

	kind, _, _, _, _, poly, evenOdd, ok = parseClipPath("polygon(evenodd, 0 0, 100% 0, 50% 100%)", 100, 80)
	if !ok || kind != "polygon" || !evenOdd || len(poly) != 3 {
		t.Fatalf("evenodd: kind=%s ok=%v evenOdd=%v n=%d", kind, ok, evenOdd, len(poly))
	}
	if poly[0] != [2]float64{0, 0} || poly[1] != [2]float64{100, 0} || poly[2] != [2]float64{50, 80} {
		t.Errorf("evenodd pts = %v", poly)
	}
}

func TestClipPathPolygonRastersTriangle(t *testing.T) {
	box := &model.Node{Type: "box", ID: "tri",
		Style: map[string]any{
			"width": 80.0, "height": 80.0, "x": 0.0, "y": 0.0,
			"background": "#ff0000",
			"clipPath":   "polygon(50% 0%, 100% 100%, 0% 100%)",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	ops := &op.Ops{}
	g, _ := Layout(ops, root, image.Pt(80, 80), testRuntime(nil), nil, 1)
	if g == nil {
		t.Fatal("layout failed")
	}
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	SoftwareRenderer{}.Render(ops, img)
	// Center-top interior of the triangle (apex at 40,0).
	if c := img.RGBAAt(40, 20); c.R < 200 || c.G > 40 {
		t.Errorf("center-top interior should be red, got %v", c)
	}
	// Top-left corner is outside the triangle.
	if c := img.RGBAAt(0, 0); c.R > 200 && c.G < 50 {
		t.Errorf("top-left corner should be empty, got %v", c)
	}
}

func TestClipOpPolygonFingerprintHashesPoints(t *testing.T) {
	a := &op.Ops{}
	a.Add(op.ClipOp{Poly: []geom.Point{{X: 40, Y: 0}, {X: 80, Y: 80}, {X: 0, Y: 80}}})
	b := &op.Ops{}
	b.Add(op.ClipOp{Poly: []geom.Point{{X: 40, Y: 0}, {X: 80, Y: 80}, {X: 0, Y: 80}}})
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("identical polygon clips must share fingerprint")
	}
	c := &op.Ops{}
	c.Add(op.ClipOp{Poly: []geom.Point{{X: 0, Y: 0}, {X: 80, Y: 80}, {X: 0, Y: 80}}})
	if a.Fingerprint() == c.Fingerprint() {
		t.Fatal("different polygon points must change fingerprint")
	}
}

func TestClipPathPolygonStyleEndToEnd(t *testing.T) {
	box := &model.Node{Type: "box", ID: "tri",
		Style: map[string]any{
			"width": 80.0, "height": 80.0, "x": 0.0, "y": 0.0,
			"background": "#ff0000",
			"clipPath":   "polygon(50% 0%, 100% 100%, 0% 100%)",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)
	frame := surf.Frame()
	if c := frame.RGBAAt(40, 20); c.R < 150 {
		t.Errorf("engine raster center-top should be red-ish, got %v", c)
	}
	if c := frame.RGBAAt(0, 0); c.R > 200 && c.G < 50 {
		t.Errorf("engine raster top-left should not be solid red, got %v", c)
	}
}
