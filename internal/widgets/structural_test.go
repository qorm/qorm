package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func TestPaginationPageDispatch(t *testing.T) {
	pg := &model.Node{Type: "pagination", ID: "pg",
		Props:   map[string]any{"page": 1, "total": 3.0},
		OnPress: &model.Invoke{Name: "nav"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{pg}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"nav": {ID: "nav", Steps: []model.Step{{Type: "state.set", Path: "pg", Value: "{{page}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// The third button is page 2 (prev ←, then 1, then 2, then 3, then →).
	// Button 0: ← at x 0..32, button 1: 1 at x 38..70, button 2: 2 at x 76..108.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 90, Y: 20})
	if rt.State["pg"] != 2 {
		t.Errorf("clicking page 2 must dispatch {page:2}, pg = %v", rt.State["pg"])
	}
}
