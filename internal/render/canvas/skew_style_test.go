package canvas

import (
	"image"
	"math"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
)

func TestParseSkew(t *testing.T) {
	rt := testRuntime(nil)
	n := &model.Node{Type: "box", Style: map[string]any{"skewX": 15.0}}
	s := parseStyle(n, rt)
	if s.SkewX != 15 {
		t.Errorf("skewX 15 = %v, want 15", s.SkewX)
	}
	n2 := &model.Node{Type: "box", Style: map[string]any{"skewY": "20deg"}}
	s2 := parseStyle(n2, rt)
	if s2.SkewY != 20 {
		t.Errorf("skewY \"20deg\" = %v, want 20", s2.SkewY)
	}
	if !canvasStyleKeys["skewX"] || !canvasStyleKeys["skewY"] {
		t.Error("skewX/skewY missing from canvasStyleKeys")
	}
}

func TestSkewDoesNotChangeLayoutBox(t *testing.T) {
	box := &model.Node{Type: "box", ID: "skewed",
		Style: map[string]any{
			"width": 40.0, "height": 40.0,
			"background": "#00aa00",
			"skewX":      20.0,
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	g := layoutScene(root, testRuntime(nil), image.Pt(200, 200))
	n := walkModel(g, "skewed")
	if n == nil {
		t.Fatal("skewed node missing from graph")
	}
	grp, ok := n.(*graph.Group)
	if !ok {
		t.Fatalf("graph node %T, want *graph.Group", n)
	}
	if grp.Width != 40 || grp.Height != 40 {
		t.Fatalf("layout box = %gx%g, want 40x40 (skew must not change layout)", grp.Width, grp.Height)
	}
	want := 20 * math.Pi / 180
	if math.Abs(grp.SkewX-want) > 1e-9 {
		t.Fatalf("SkewX = %v, want %v (20deg in radians)", grp.SkewX, want)
	}
	if grp.SkewY != 0 {
		t.Fatalf("SkewY = %v, want 0", grp.SkewY)
	}
}

func TestSkewYAppliesRadians(t *testing.T) {
	box := &model.Node{Type: "box", ID: "sky",
		Style: map[string]any{
			"width": 40.0, "height": 40.0,
			"skewY": "15deg",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	g := layoutScene(root, testRuntime(nil), image.Pt(120, 120))
	grp := walkModel(g, "sky").(*graph.Group)
	want := 15 * math.Pi / 180
	if math.Abs(grp.SkewY-want) > 1e-9 {
		t.Fatalf("SkewY = %v, want %v", grp.SkewY, want)
	}
}
