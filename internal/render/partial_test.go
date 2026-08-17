package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func counterApp() *model.App {
	return &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{
			Schema:  map[string]string{"count": "number", "status": "string"},
			Initial: map[string]any{"count": 0, "status": "idle"},
		},
		Scenes: map[string]*model.Node{"main": {
			Type: "column", ID: "root",
			Children: []*model.Node{
				{Type: "text", ID: "title", Text: "COUNTER"},
				{Type: "text", ID: "number", Text: "{{ state.count }}"},
				{Type: "text", ID: "status_text", Text: "{{ state.status }}"},
			},
		}},
		Actions: map[string]*model.Action{"increment": {Steps: []model.Step{{Type: "state.increment", Path: "count"}}}},
	}
}

func TestPatchSceneCounterIncrement(t *testing.T) {
	rt := runtime.New(counterApp())
	full := RenderScene(rt, "main")
	idx := BuildDepIndex(rt.App.Scenes["main"])

	rt.Dispatch("increment", nil)
	dirty := rt.TakeDirtyPaths()
	if len(dirty) != 1 || dirty[0] != "count" {
		t.Fatalf("dirty paths: %#v", dirty)
	}
	patched, ok := PatchScene(rt, "main", full, dirty, idx)
	if !ok {
		t.Fatal("expected partial patch")
	}
	if !strings.Contains(patched.HTML, ">1<") {
		t.Fatalf("patched html should show count 1:\n%s", patched.HTML)
	}
	if strings.Contains(patched.HTML, "COUNTER") && strings.Count(patched.HTML, "COUNTER") == 1 {
		// title unchanged — good
	}
	// status_text should still say idle (unchanged fragment)
	if !strings.Contains(patched.HTML, "idle") {
		t.Fatalf("status should remain idle:\n%s", patched.HTML)
	}
}

func TestPatchSceneSkipsListData(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{
			Schema:  map[string]string{"items": "array", "title": "string"},
			Initial: map[string]any{"items": []any{"a", "b"}, "title": "List"},
		},
		Scenes: map[string]*model.Node{"main": {
			Type: "column", ID: "root",
			Children: []*model.Node{
				{Type: "text", ID: "title", Text: "{{ state.title }}"},
				{Type: "list", ID: "list", Data: "{{ state.items }}", Template: &model.Node{
					Type: "text", ID: "row", Text: "{{ item }}",
				}},
			},
		}},
	}
	rt := runtime.New(app)
	full := RenderScene(rt, "main")
	idx := BuildDepIndex(rt.App.Scenes["main"])

	rt.State["title"] = "Updated"
	dirty := []string{"title"}
	patched, ok := PatchScene(rt, "main", full, dirty, idx)
	if !ok {
		t.Fatal("title-only change should partial patch")
	}
	if !strings.Contains(patched.HTML, "Updated") {
		t.Fatalf("patched title missing:\n%s", patched.HTML)
	}

	rt.State["items"] = []any{"x"}
	_, ok = PatchScene(rt, "main", full, []string{"items"}, idx)
	if ok {
		t.Fatal("list data change should not partial patch")
	}
}
