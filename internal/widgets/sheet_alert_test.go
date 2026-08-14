package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// The alert banner renders in-flow with its variant tint; a danger alert
// paints a red-tinted background.
func TestAlertVariantRenders(t *testing.T) {
	al := &model.Node{Type: "alert", ID: "al",
		Props:    map[string]any{"variant": "danger", "title": "Error", "text": "boom"},
		Children: nil}
	e, surf := formEngine(t, al)
	e.DrawFrame(surf)
	// The tinted banner reads as a pink band over the white surface (red
	// dominant, the isRed predicate requires a strong pure red — the 60-alpha
	// tint blends).
	tinted := 0
	b := surf.Frame().Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := surf.Frame().RGBAAt(x, y); c.R > 230 && c.R-c.G > 30 {
				tinted++
			}
		}
	}
	if tinted == 0 {
		t.Error("a danger alert must paint its red tint")
	}
}

// The sheet mounts when its `open` binding is truthy and dismisses on a
// backdrop tap (the panel is bottom-anchored).
func TestSheetOpenBackdropDismiss(t *testing.T) {
	sh := &model.Node{Type: "sheet", ID: "sh",
		Props:    map[string]any{"open": "{{state.show}}"},
		OnPress:  &model.Invoke{Name: "close"},
		Children: []*model.Node{{Type: "text", ID: "body", Props: map[string]any{"text": "sheet body"}}}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sh}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"close": {ID: "close", Steps: []model.Step{{Type: "state.set", Path: "show", Value: "false"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	rt.State["show"] = true
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("sheet")
	sw := w.(*Sheet)
	if !sw.OverlayOpen(sh, rt) {
		t.Fatal("the sheet must mount while open=true")
	}
	// A backdrop tap (a corner, above the bottom panel) dispatches the dismiss.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 5, Y: 5})
	if got := rt.State["show"]; got != "false" {
		t.Errorf("a backdrop tap must dispatch the dismiss handler, show = %v", got)
	}
}
