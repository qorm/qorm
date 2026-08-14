package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// A table renders columns over bound data; clicking a header dispatches
// onChange with {column: field} — the app-wired sort contract.
func TestTableHeaderSortDispatch(t *testing.T) {
	tb := &model.Node{Type: "table", ID: "tb",
		Props: map[string]any{
			"columns": []any{
				map[string]any{"title": "Name", "field": "name", "width": 150.0},
				map[string]any{"title": "Score", "field": "score", "width": 100.0},
			},
		},
		Data:     "{{state.rows}}",
		OnChange: &model.Invoke{Name: "sort"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{tb}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"sort": {ID: "sort", Steps: []model.Step{{Type: "state.set", Path: "col", Value: "{{column}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["rows"] = []any{
		map[string]any{"name": "Alice", "score": 42.0},
		map[string]any{"name": "Bob", "score": 37.0},
	}
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Click the second header ("Score", x 150..250, y 0..32).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 200, Y: 16})
	if rt.State["col"] != "score" {
		t.Errorf("clicking the Score header must dispatch onChange {column:score}, col = %v", rt.State["col"])
	}
}
