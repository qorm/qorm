package runtime_test

import (
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
)

func TestOnEnterErrorFallsBackToBoundaryScene(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "errorBoundary": map[string]any{"scene": "oops"}},
		{"type": "scene", "id": "main", "onEnter": "boom", "root": map[string]any{"type": "text", "id": "mainText", "text": "main"}},
		{"type": "scene", "id": "oops", "root": map[string]any{"type": "text", "id": "oopsText", "text": "oops"}},
		{"type": "action", "id": "boom", "script": "let xs = [1]\nxs[5] = 9"},
	})
	rt := runtime.New(app)
	rt.RunPendingEnter()
	if got := rt.CurrentScene(); got != "oops" {
		t.Fatalf("CurrentScene() = %q, want oops", got)
	}
	if rt.LastBoundaryError.Level != "scene" || rt.LastBoundaryError.Phase != "onEnter" {
		t.Fatalf("LastBoundaryError = %#v, want scene/onEnter", rt.LastBoundaryError)
	}
	if got := runtime.Stringify(rt.RouteParams["failedScene"]); got != "main" {
		t.Fatalf("failedScene route param = %q, want main", got)
	}
}

func TestActionErrorFallsBackToSceneOverride(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "errorBoundary": map[string]any{"scene": "appOops"}},
		{"type": "scene", "id": "main", "errorBoundary": map[string]any{"scene": "sceneOops"}, "root": map[string]any{"type": "button", "id": "go", "label": "Go"}},
		{"type": "scene", "id": "appOops", "root": map[string]any{"type": "text", "id": "appOopsText", "text": "app oops"}},
		{"type": "scene", "id": "sceneOops", "root": map[string]any{"type": "text", "id": "sceneOopsText", "text": "scene oops"}},
		{"type": "action", "id": "boom", "script": "let xs = [1]\nxs[3] = 8"},
	})
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	rt.Dispatch("boom", nil)
	if got := rt.CurrentScene(); got != "sceneOops" {
		t.Fatalf("CurrentScene() = %q, want sceneOops", got)
	}
	if rt.LastBoundaryError.Level != "scene" || rt.LastBoundaryError.Phase != "action" || rt.LastBoundaryError.Scene != "main" {
		t.Fatalf("LastBoundaryError = %#v, want main/action scene boundary", rt.LastBoundaryError)
	}
}
