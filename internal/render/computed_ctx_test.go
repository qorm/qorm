package render_test

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// Computed values are published into the renderer's binding context under their
// own root, so a scene can bind the short {{ computed.x }} spelling instead of
// reaching through state.
func TestComputedVisibleToSceneBindings(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"price": 4.0, "qty": 2.0},
				map[string]any{"price": 1.5, "qty": 2.0},
			},
		}},
		Computed: map[string]string{
			"total": `{{ sum(map(state.items, "it.price * it.qty")) }}`,
		},
		Scenes: map[string]*model.Node{"main": {Type: "column", Children: []*model.Node{
			{Type: "text", ID: "short", Text: "{{computed.total}}"},
			{Type: "text", ID: "long", Text: "{{state.computed.total}}"},
		}}},
	}
	html := render.Render(qrt.New(app)).HTML

	if strings.Count(html, "11") != 2 {
		t.Errorf("both spellings should render 11, got:\n%s", html)
	}
	if strings.Contains(html, "{{") {
		t.Errorf("an unresolved binding leaked:\n%s", html)
	}
}
