package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// A list tile renders its title and dispatches onPress through the generic
// engine path when pressed (no widget-side input logic).
func TestListTileRendersAndPresses(t *testing.T) {
	lt := &model.Node{Type: "listtile", ID: "lt",
		Props:   map[string]any{"subtitle": "sub"},
		Label:   "Title",
		OnPress: &model.Invoke{Name: "go"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{lt}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"go": {ID: "go", Steps: []model.Step{{Type: "state.set", Path: "pressed", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 200))
	e.DrawFrame(surf)

	// The title text renders (dark ink on the row).
	txt := 0
	b := surf.Frame().Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := surf.Frame().RGBAAt(x, y); c.R < 100 && c.G < 100 && c.B < 100 {
				txt++
			}
		}
	}
	if txt == 0 {
		t.Fatal("the list tile must render its title text")
	}
	// Press the tile: the generic path dispatches onPress.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 100, Y: 30})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 100, Y: 30})
	if rt.State["pressed"] != "yes" {
		t.Error("pressing the list tile must dispatch its onPress")
	}
}

// A selected chip paints its accent pill; the delete "×" dispatches onChange.
func TestChipSelectedAndDelete(t *testing.T) {
	ch := &model.Node{Type: "chip", ID: "ch",
		Props:    map[string]any{"selected": "true", "label": "Filter"},
		OnChange: &model.Invoke{Name: "remove"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{ch}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"remove": {ID: "remove", Steps: []model.Step{{Type: "state.set", Path: "removed", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 200))
	e.DrawFrame(surf)

	// The selected pill paints the accent (blue) fill.
	accent := 0
	b := surf.Frame().Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := surf.Frame().RGBAAt(x, y); c.R < 40 && c.G > 100 && c.B > 200 {
				accent++
			}
		}
	}
	if accent == 0 {
		t.Fatal("a selected chip must paint its accent pill")
	}
	// Press the delete "×" (the right edge of the pill).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 45, Y: 20})
	if rt.State["removed"] != "yes" {
		t.Error("pressing the delete affordance must dispatch onChange")
	}
}
