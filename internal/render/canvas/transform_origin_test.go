package canvas

import (
	"image"
	"math"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render"
	"github.com/qorm/platform/internal/render/graph"
)

func TestParseTransformOriginKeywordsAndPercent(t *testing.T) {
	ox, oy := parseTransformOrigin("left top", 40, 40)
	if ox != 0 || oy != 0 {
		t.Fatalf("left top = (%v,%v), want (0,0)", ox, oy)
	}
	ox, oy = parseTransformOrigin("0 0", 40, 40)
	if ox != 0 || oy != 0 {
		t.Fatalf("0 0 = (%v,%v), want (0,0)", ox, oy)
	}
	ox, oy = parseTransformOrigin("50% 100%", 40, 40)
	if ox != 20 || oy != 40 {
		t.Fatalf("50%% 100%% = (%v,%v), want (20,40)", ox, oy)
	}
	ox, oy = parseTransformOrigin("", 40, 40)
	if ox != 20 || oy != 20 {
		t.Fatalf("empty = (%v,%v), want center (20,20)", ox, oy)
	}
	ox, oy = parseTransformOrigin("center", 40, 40)
	if ox != 20 || oy != 20 {
		t.Fatalf("center = (%v,%v), want (20,20)", ox, oy)
	}
	ox, oy = parseTransformOrigin("top", 40, 40)
	if ox != 20 || oy != 0 {
		t.Fatalf("top = (%v,%v), want (20,0)", ox, oy)
	}
	ox, oy = parseTransformOrigin("right bottom", 40, 40)
	if ox != 40 || oy != 40 {
		t.Fatalf("right bottom = (%v,%v), want (40,40)", ox, oy)
	}
	ox, oy = parseTransformOrigin("12px 8px", 40, 40)
	if ox != 12 || oy != 8 {
		t.Fatalf("12px 8px = (%v,%v), want (12,8)", ox, oy)
	}
}

func TestTransformOriginStyleKey(t *testing.T) {
	if !render.KnownStyleKeys["transformOrigin"] {
		t.Error("transformOrigin missing from KnownStyleKeys")
	}
	if !canvasStyleKeys["transformOrigin"] {
		t.Error("transformOrigin missing from canvasStyleKeys")
	}
	n := &model.Node{Type: "box", Style: map[string]any{"transformOrigin": "0 0"}}
	s := parseStyle(n, testRuntime(nil))
	if s.TransformOrigin != "0 0" {
		t.Fatalf("parseStyle TransformOrigin = %q, want %q", s.TransformOrigin, "0 0")
	}
}

func TestRotate90TransformOriginPinsTopLeft(t *testing.T) {
	mk := func(origin string) *model.Node {
		st := map[string]any{
			"width": 40.0, "height": 40.0, "x": 0.0, "y": 0.0,
			"rotate": 90.0,
		}
		if origin != "" {
			st["transformOrigin"] = origin
		}
		return &model.Node{Type: "box", ID: "spin", Style: st}
	}
	rootOf := func(box *model.Node) *model.Node {
		return &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	}

	corner := layoutScene(rootOf(mk("0 0")), testRuntime(nil), image.Pt(80, 80))
	center := layoutScene(rootOf(mk("")), testRuntime(nil), image.Pt(80, 80))
	cg := walkModel(corner, "spin").(*graph.Group)
	ng := walkModel(center, "spin").(*graph.Group)

	if cg.Width != 40 || cg.Height != 40 || ng.Width != 40 || ng.Height != 40 {
		t.Fatalf("layout box changed: corner %gx%g center %gx%g", cg.Width, cg.Height, ng.Width, ng.Height)
	}
	if math.Abs(cg.X) > 1e-6 || math.Abs(cg.Y) > 1e-6 {
		t.Fatalf("origin 0 0 group XY = (%v,%v), want (0,0) (top-left pinned)", cg.X, cg.Y)
	}
	if math.Abs(ng.X) < 1e-6 && math.Abs(ng.Y) < 1e-6 {
		t.Fatal("default center pivot must shift the group origin, got (0,0)")
	}
	if math.Abs(ng.X-40) > 1e-6 || math.Abs(ng.Y) > 1e-6 {
		t.Fatalf("center pivot group XY = (%v,%v), want (40,0)", ng.X, ng.Y)
	}
}
