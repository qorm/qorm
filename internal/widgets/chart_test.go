package widgets

// Chart widget tests: registration, measure defaults, and the rasterized
// output — a bar chart paints its bars in the resolved series colour (design
// tokens through var(--qorm-token-*) included), an area chart paints the 15%
// fill plus the full-ink edge.

import (
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func chartRT() *runtime.Runtime {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}})
	rt.Theme = theme.GetDefault()
	return rt
}

func TestChartBarPaintsSeriesColor(t *testing.T) {
	n := &model.Node{Type: "chart", ID: "c",
		Props: map[string]any{"data": []any{1.0, 3.0, 2.0}, "color": "#10b981"},
		Style: map[string]any{"width": 120.0, "height": 40.0}}
	rt := chartRT()
	wi, ok := canvas.LookupWidget("chart")
	if !ok {
		t.Fatal("chart not registered")
	}
	w, h := wi.Measure(n, rt, nil, 1)
	if w != 120 || h != 40 {
		t.Fatalf("measure = %dx%d, want 120x40", w, h)
	}
	ln := &canvas.LayoutNode{Node: n, Width: w, Height: h}
	g := wi.Record(ln, rt, 1).(*draw.Group)
	if len(g.Children) != 1 {
		t.Fatalf("bar chart children = %d, want 1 bitmap", len(g.Children))
	}
	bmp := g.Children[0].(*draw.Image).Bitmap
	green := color.RGBA{16, 185, 129, 255}
	found := false
	for y := 0; y < bmp.Bounds().Dy(); y++ {
		for x := 0; x < bmp.Bounds().Dx(); x++ {
			if bmp.RGBAAt(x, y) == green {
				found = true
			}
		}
	}
	if !found {
		t.Error("no pixel in the author series colour #10b981")
	}
}

func TestChartAreaPaintsFillAndEdge(t *testing.T) {
	n := &model.Node{Type: "chart", ID: "a",
		Props: map[string]any{"chartType": "area", "data": []any{1.0, 2.0, 3.0}},
		Style: map[string]any{"width": 100.0, "height": 40.0}}
	rt := chartRT()
	wi, _ := canvas.LookupWidget("chart")
	w, h := wi.Measure(n, rt, nil, 1)
	ln := &canvas.LayoutNode{Node: n, Width: w, Height: h}
	g := wi.Record(ln, rt, 1).(*draw.Group)
	if len(g.Children) != 2 {
		t.Fatalf("area chart children = %d, want fill+edge bitmaps", len(g.Children))
	}
	fillA := g.Children[0].(*draw.Image).Bitmap.RGBAAt(50, 38).A
	edgeA := g.Children[1].(*draw.Image).Bitmap
	var maxA uint8
	for y := 0; y < edgeA.Bounds().Dy(); y++ {
		for x := 0; x < edgeA.Bounds().Dx(); x++ {
			if a := edgeA.RGBAAt(x, y).A; a > maxA {
				maxA = a
			}
		}
	}
	if fillA == 0 {
		t.Error("area fill is transparent at the bottom band")
	}
	if maxA != 255 {
		t.Errorf("edge max alpha = %d, want the full-ink 255 line", maxA)
	}
}

func TestChartDesignTokenColour(t *testing.T) {
	rt := chartRT()
	rt.App.DesignTokens = map[string]model.DesignToken{
		"color.success": {Type: "color", Value: "#10b981"},
	}
	n := &model.Node{Type: "chart", ID: "t",
		Props: map[string]any{"data": []any{1.0}, "color": "var(--qorm-token-color-success)"},
		Style: map[string]any{"width": 20.0, "height": 10.0}}
	ln := &canvas.LayoutNode{Node: n, Width: 20, Height: 10}
	if ink := chartInk(n, ln, rt); ink != (color.RGBA{16, 185, 129, 255}) {
		t.Errorf("design-token ink = %v, want #10b981", ink)
	}
}
