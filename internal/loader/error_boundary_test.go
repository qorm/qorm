package loader

import (
	"strings"
	"testing"
)

func TestErrorBoundaryParsesAndRoundTrips(t *testing.T) {
	app := FromDocs([]map[string]any{
		{
			"type":          "app",
			"id":            "x",
			"entry":         "main",
			"errorBoundary": map[string]any{"scene": "oops"},
		},
		{
			"type":          "scene",
			"id":            "main",
			"errorBoundary": map[string]any{"scene": "sceneOops"},
			"root": map[string]any{
				"type": "column", "id": "root",
				"errorBoundary": map[string]any{
					"fallback": map[string]any{"type": "text", "id": "fb", "text": "fallback"},
				},
				"children": []any{map[string]any{"type": "text", "id": "body", "text": "body"}},
			},
		},
		{"type": "scene", "id": "oops", "root": map[string]any{"type": "text", "id": "oopsText", "text": "oops"}},
		{"type": "scene", "id": "sceneOops", "root": map[string]any{"type": "text", "id": "sceneOopsText", "text": "scene oops"}},
	})
	if app.ErrorBoundary == nil || app.ErrorBoundary.Scene != "oops" {
		t.Fatalf("app error boundary = %#v, want scene oops", app.ErrorBoundary)
	}
	if got := app.SceneErrorBoundaries["main"]; got == nil || got.Scene != "sceneOops" {
		t.Fatalf("scene override = %#v, want sceneOops", got)
	}
	if got := app.Scenes["main"].ErrorBoundary; got == nil || got.Fallback == nil || got.Fallback.Text != "fallback" {
		t.Fatalf("node boundary = %#v, want fallback subtree", got)
	}
	rt := FromDocs(AppToDocs(app))
	if rt.ErrorBoundary == nil || rt.ErrorBoundary.Scene != "oops" {
		t.Fatalf("round-trip app boundary lost: %#v", rt.ErrorBoundary)
	}
	if got := rt.SceneErrorBoundaries["main"]; got == nil || got.Scene != "sceneOops" {
		t.Fatalf("round-trip scene boundary lost: %#v", got)
	}
}

func TestErrorBoundaryDiagnostics(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "errorBoundary": map[string]any{"scene": "{{ state.dest }}"}},
		{
			"type": "scene", "id": "main",
			"errorBoundary": map[string]any{"scene": "main"},
			"root": map[string]any{
				"type": "column", "id": "root",
				"errorBoundary": map[string]any{},
			},
		},
	})
	joined := strings.Join(app.Diagnostics, "\n")
	for _, want := range []string{
		`errorBoundary 的 scene "{{ state.dest }}" 是 {{...}} 绑定`,
		`errorBoundary 的 scene 指向自身 "main"`,
		`errorBoundary 缺少 fallback 根节点对象`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}

func TestErrorBoundaryRejectsBadShapesAndMissingScenes(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "errorBoundary": "oops"},
		{
			"type": "scene", "id": "main",
			"errorBoundary": map[string]any{"scene": "missing"},
			"root": map[string]any{
				"type": "column", "id": "root",
				"errorBoundary": true,
			},
		},
	})
	joined := strings.Join(app.Diagnostics, "\n")
	for _, want := range []string{
		`[App: x] errorBoundary 应为对象`,
		`[Scene: main] errorBoundary 的 scene 指向不存在的场景 "missing"`,
		`节点 (id: "root", type: "column") 的 errorBoundary 应为对象`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}
