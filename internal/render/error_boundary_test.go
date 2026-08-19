package render_test

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/render"
	"github.com/qorm/platform/internal/runtime"
)

func TestNodeErrorBoundaryFallsBackAndKeepsSiblings(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{
				"type": "column", "id": "root",
				"children": []any{
					map[string]any{"type": "text", "id": "before", "text": "BEFORE"},
					map[string]any{
						"type": "column", "id": "boundary",
						"errorBoundary": map[string]any{
							"fallback": map[string]any{"type": "text", "id": "fallback", "text": "FALLBACK"},
						},
						"children": []any{
							map[string]any{"type": "colunm", "id": "bad", "children": []any{map[string]any{"type": "text", "id": "nested", "text": "BAD"}}},
						},
					},
					map[string]any{"type": "text", "id": "after", "text": "AFTER"},
				},
			},
		},
	})
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	res := render.RenderScene(rt, rt.CurrentScene())
	for _, want := range []string{"BEFORE", "FALLBACK", "AFTER"} {
		if !strings.Contains(res.HTML, want) {
			t.Fatalf("render missing %q:\n%s", want, res.HTML)
		}
	}
	if strings.Contains(res.HTML, `data-qorm-unknown="colunm"`) {
		t.Fatalf("unknown widget should have been swallowed by boundary:\n%s", res.HTML)
	}
	if rt.LastBoundaryError.Level != "node" || rt.LastBoundaryError.NodeID != "boundary" {
		t.Fatalf("LastBoundaryError = %#v, want node boundary hit", rt.LastBoundaryError)
	}
}

func TestSceneBoundaryRenderUsesFallbackScene(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "errorBoundary": map[string]any{"scene": "oops"}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "mainText", "text": "MAIN"}},
		{"type": "scene", "id": "oops", "root": map[string]any{"type": "text", "id": "oopsText", "text": "OOPS"}},
	})
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	if !rt.HandleSceneError("main", "render", "boom") {
		t.Fatal("HandleSceneError should redirect to fallback scene")
	}
	res := render.RenderScene(rt, rt.CurrentScene())
	if !strings.Contains(res.HTML, "OOPS") {
		t.Fatalf("render should show fallback scene:\n%s", res.HTML)
	}
}
