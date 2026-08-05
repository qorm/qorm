package widgets

// Placement tests for the select dropdown's viewport edge behaviour: the
// panel flips above the box when opening downward would leave the viewport
// (previously the lower rows painted off-window — invisible and
// unclickable), and geoms.panel always mirrors the drawn rect so
// optionIndexAt maps rows in the same coordinate space.

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func openSelect(t *testing.T, n *model.Node) (*Select, canvas.OverlayWidget) {
	t.Helper()
	wi, ok := canvas.LookupWidget("select")
	if !ok {
		t.Fatal("select not registered")
	}
	sel := wi.(*Select)
	sel.setOpen(n, true)
	return sel, wi.(canvas.OverlayWidget)
}

func selectRT() *runtime.Runtime {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}})
	rt.Theme = theme.GetDefault()
	return rt
}

func TestSelectOverlayFlipsAboveNearViewportBottom(t *testing.T) {
	n := &model.Node{Type: "select", ID: "flip", Props: map[string]any{"options": []any{
		map[string]any{"value": "a", "label": "Alpha"},
		map[string]any{"value": "b", "label": "Beta"},
		map[string]any{"value": "c", "label": "Gamma"},
	}}}
	rt := selectRT()
	rt.Viewport = runtime.Viewport{W: 400, H: 200}
	sel, ow := openSelect(t, n)

	ln := &canvas.LayoutNode{Node: n, Width: 200, Height: 36, AbsX: 20, AbsY: 150}
	ov := ow.OverlayRecord(ln, rt, 1, image.Point{})
	if ov == nil {
		t.Fatal("OverlayRecord nil")
	}
	panel := ov.(*draw.Group).Children[1].(*draw.Rect)
	if panel.Y >= float64(ln.AbsY) {
		t.Errorf("panel.Y = %.0f, want flipped ABOVE the box (AbsY %d) — opening downward leaves the 200px viewport", panel.Y, ln.AbsY)
	}
	// geoms.panel must mirror the drawn rect (hit-testing reads it).
	geo, ok := sel.geometry(n)
	if !ok {
		t.Fatal("no geometry recorded")
	}
	if geo.panel.Min.Y != int(panel.Y) || geo.panel.Max.Y != int(panel.Y+panel.Height) {
		t.Errorf("geoms.panel = %v, want the drawn panel rect (y %.0f..%.0f)", geo.panel, panel.Y, panel.Y+panel.Height)
	}
	// A press on the flipped panel's first row must map to option 0.
	opts := formOptions(n.Props["options"])
	if idx := sel.optionIndexAt(geo, opts, 30, panel.Y+float64(geo.menuPad)+2); idx != 0 {
		t.Errorf("optionIndexAt on flipped first row = %d, want 0", idx)
	}
	sel.setOpen(n, false)
}

func TestSelectOverlayStaysBelowWhenRoomAllows(t *testing.T) {
	n := &model.Node{Type: "select", ID: "down", Props: map[string]any{"options": []any{
		map[string]any{"value": "a", "label": "Alpha"},
	}}}
	rt := selectRT()
	rt.Viewport = runtime.Viewport{W: 400, H: 800}
	_, ow := openSelect(t, n)

	ln := &canvas.LayoutNode{Node: n, Width: 200, Height: 36, AbsX: 20, AbsY: 150}
	ov := ow.OverlayRecord(ln, rt, 1, image.Point{})
	panel := ov.(*draw.Group).Children[1].(*draw.Rect)
	if panel.Y <= float64(ln.AbsY) {
		t.Errorf("panel.Y = %.0f, want BELOW the box (AbsY %d) when the viewport has room", panel.Y, ln.AbsY)
	}
	sel, _ := canvas.LookupWidget("select")
	sel.(*Select).setOpen(n, false)
}


// Moving the pointer over the open menu tracks the hovered row: the widget
// asks for a redraw on change, and OverlayRecord paints the native-menu
// accent highlight with a white label on that row.
func TestSelectMenuRowHoverHighlight(t *testing.T) {
	n := &model.Node{Type: "select", ID: "hover", Props: map[string]any{"options": []any{
		map[string]any{"value": "a", "label": "Alpha"},
		map[string]any{"value": "b", "label": "Beta"},
	}}}
	rt := selectRT()
	rt.Viewport = runtime.Viewport{W: 400, H: 800}
	sel, ow := openSelect(t, n)

	ln := &canvas.LayoutNode{Node: n, Width: 200, Height: 36, AbsX: 20, AbsY: 150}
	ov := ow.OverlayRecord(ln, rt, 1, image.Point{}) // writes geoms.panel
	if ov == nil {
		t.Fatal("OverlayRecord nil")
	}

	wi, _ := canvas.LookupWidget("select")
	iw := wi.(canvas.InteractiveWidget)
	inter := &canvas.Interaction{}
	// Move over row 1 (Beta): panel.Min.Y + menuPad + rowH + a bit.
	geo, _ := sel.geometry(n)
	y := float64(geo.panel.Min.Y+geo.menuPad+geo.rowH) + 2
	redraw := iw.HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerMove, X: 30, Y: y}, inter, image.Rectangle{})
	if !redraw {
		t.Fatal("hovering a different row must request a redraw")
	}
	if got := sel.hoverRowOf(n); got != 1 {
		t.Fatalf("hoverRow = %d, want 1 (Beta)", got)
	}
	// Moving within the same row: no redraw.
	if iw.HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerMove, X: 31, Y: y}, inter, image.Rectangle{}) {
		t.Fatal("motion within one row must not redraw")
	}
	// The next OverlayRecord highlights row 1 with the accent fill.
	ov2 := ow.OverlayRecord(ln, rt, 1, image.Point{})
	g := ov2.(*draw.Group)
	accent := themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	found := false
	for _, c := range g.Children {
		if r, ok := c.(*draw.Rect); ok && r.Fill == accent {
			wantY := float64(geo.panel.Min.Y + geo.menuPad + geo.rowH)
			if r.Y == wantY {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no accent highlight rect on the hovered row; children=%v", g.Children)
	}
	sel.setOpen(n, false)
}
