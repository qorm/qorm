package canvas

import (
	"encoding/json"
	"image"
	"testing"

	"github.com/qorm/platform/internal/geom"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
)

func TestParseZIndex(t *testing.T) {
	rt := testRuntime(nil)
	n := &model.Node{Type: "box", Style: map[string]any{"zIndex": 3.0}}
	s := parseStyle(n, rt)
	if s.ZIndex != 3 {
		t.Errorf("zIndex 3 = %d, want 3", s.ZIndex)
	}
	n2 := &model.Node{Type: "box", Style: map[string]any{"zIndex": "-1"}}
	s2 := parseStyle(n2, rt)
	if s2.ZIndex != -1 {
		t.Errorf("zIndex \"-1\" = %d, want -1", s2.ZIndex)
	}
	if !canvasStyleKeys["zIndex"] {
		t.Error("zIndex missing from canvasStyleKeys")
	}
}

func overlapStack(zA, zB any) *model.Node {
	a := &model.Node{Type: "box", ID: "A", Style: map[string]any{
		"width": 40.0, "height": 40.0, "x": 0.0, "y": 0.0,
		"background": "#ff0000", "zIndex": zA,
	}}
	b := &model.Node{Type: "box", ID: "B", Style: map[string]any{
		"width": 40.0, "height": 40.0, "x": 10.0, "y": 10.0,
		"background": "#0000ff", "zIndex": zB,
	}}
	return &model.Node{Type: "stack", ID: "root", Style: map[string]any{
		"width": 80.0, "height": 80.0,
	}, Children: []*model.Node{a, b}}
}

func rasterScene(root *model.Node, size image.Point) (graph.Node, *image.RGBA) {
	rt := testRuntime(nil)
	ops := &op.Ops{}
	g, _ := Layout(ops, root, size, rt, nil, 1)
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	SoftwareRenderer{}.Render(ops, img)
	return g, img
}

func hitModelID(g graph.Node, x, y float64) string {
	hit := g.HitTest(geom.Point{X: x, Y: y})
	for n := hit; n != nil; {
		if m := n.Base().Model; m != nil && m.ID != "" {
			return m.ID
		}
		p := n.Base().Parent
		if p == nil {
			break
		}
		n = p
	}
	return ""
}

func siblingModelIDs(g graph.Node, parentID string) []string {
	p := walkModel(g, parentID)
	if p == nil {
		return nil
	}
	var ids []string
	for _, c := range p.Base().Children {
		if m := c.Base().Model; m != nil && m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func TestZIndexPaintAndHit(t *testing.T) {
	g, img := rasterScene(overlapStack(1.0, 2.0), image.Pt(80, 80))
	c := img.RGBAAt(20, 20)
	if c.B < 200 || c.R > 40 {
		t.Fatalf("overlap (20,20) = %v, want blue (B on top)", c)
	}
	if id := hitModelID(g, 20, 20); id != "B" {
		t.Fatalf("hit (20,20) = %q, want B", id)
	}
	ids := siblingModelIDs(g, "root")
	if len(ids) < 2 || ids[0] != "A" || ids[len(ids)-1] != "B" {
		t.Fatalf("sibling order = %v, want A then B", ids)
	}
}

func TestZIndexSwap(t *testing.T) {
	g, img := rasterScene(overlapStack(5.0, 1.0), image.Pt(80, 80))
	c := img.RGBAAt(20, 20)
	if c.R < 200 || c.B > 40 {
		t.Fatalf("overlap (20,20) = %v, want red (A on top)", c)
	}
	if id := hitModelID(g, 20, 20); id != "A" {
		t.Fatalf("hit (20,20) = %q, want A", id)
	}
	ids := siblingModelIDs(g, "root")
	if len(ids) < 2 || ids[0] != "B" || ids[len(ids)-1] != "A" {
		t.Fatalf("sibling order = %v, want B then A", ids)
	}
}

func TestZIndexStableDocumentOrder(t *testing.T) {
	g, img := rasterScene(overlapStack(2.0, 2.0), image.Pt(80, 80))
	c := img.RGBAAt(20, 20)
	if c.B < 200 || c.R > 40 {
		t.Fatalf("equal zIndex overlap = %v, want blue (later sibling)", c)
	}
	if id := hitModelID(g, 20, 20); id != "B" {
		t.Fatalf("equal zIndex hit = %q, want B (document order)", id)
	}
	ids := siblingModelIDs(g, "root")
	if len(ids) < 2 || ids[0] != "A" || ids[len(ids)-1] != "B" {
		t.Fatalf("equal zIndex order = %v, want stable A then B", ids)
	}
}

func TestMeasureReportZIndex(t *testing.T) {
	a := &model.Node{Type: "box", ID: "layered", Style: map[string]any{
		"width": 20.0, "height": 20.0, "zIndex": 4.0,
	}}
	b := &model.Node{Type: "box", ID: "plain", Style: map[string]any{
		"width": 20.0, "height": 20.0,
	}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{a, b}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	e.DrawFrame(NewHeadlessSurface(image.Pt(80, 80)))
	var rows []map[string]any
	if err := json.Unmarshal(e.CollectMeasure(), &rows); err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		got[id] = r["zIndex"]
	}
	if got["layered"] != float64(4) {
		t.Errorf("layered zIndex = %v (%T), want 4", got["layered"], got["layered"])
	}
	if got["plain"] != "auto" {
		t.Errorf("plain zIndex = %v, want auto", got["plain"])
	}
}

func TestZIndexNegativePaintsBehind(t *testing.T) {
	g, img := rasterScene(overlapStack(-1.0, 0.0), image.Pt(80, 80))
	c := img.RGBAAt(20, 20)
	if c.B < 200 || c.R > 40 {
		t.Fatalf("negative-z overlap = %v, want blue (B auto above A)", c)
	}
	if id := hitModelID(g, 20, 20); id != "B" {
		t.Fatalf("negative-z hit = %q, want B", id)
	}
}
