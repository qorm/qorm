package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

func twoSceneApp(mainText, otherText string, initial map[string]any) *model.App {
	return &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main":  {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "text", ID: "t", Text: mainText}}},
			"other": {Type: "scaffold", ID: "o", Children: []*model.Node{{Type: "text", ID: "ot", Text: otherText}}},
		},
		GlobalState: model.GlobalState{Initial: initial},
	}
}

// TestReloadPreservesSession covers dev hot-reload: swapping in an edited app
// keeps the live session (in-progress state, current scene, viewport) while
// serving the new structure; new state keys get their initials; and a scene the
// edit removed falls back to the entry instead of rendering nothing.
func TestReloadPreservesSession(t *testing.T) {
	s := New(runtime.New(twoSceneApp("v1", "other", map[string]any{"count": float64(0)})))
	// a live session: the user changed state, navigated away, has a viewport
	s.rt.State["count"] = float64(7)
	s.rt.Navigate("other", nil)
	s.rt.Viewport = runtime.Viewport{W: 390, H: 844}

	// the edit: text v1->v2 and other->other-edited, plus a new state key
	s.Reload(runtime.New(twoSceneApp("v2", "other-edited", map[string]any{"count": float64(0), "added": "x"})))

	if got := s.rt.State["count"]; got != float64(7) {
		t.Errorf("in-progress state not preserved: count=%v, want 7", got)
	}
	if got := s.rt.State["added"]; got != "x" {
		t.Errorf("state key introduced by the edit missing: %v", got)
	}
	if s.rt.CurrentScene() != "other" {
		t.Errorf("current scene not preserved: %q, want other", s.rt.CurrentScene())
	}
	if (s.rt.Viewport != runtime.Viewport{W: 390, H: 844}) {
		t.Errorf("viewport not preserved: %+v", s.rt.Viewport)
	}
	if html := render.RenderScene(s.rt, s.rt.CurrentScene()).HTML; !strings.Contains(html, "other-edited") {
		t.Errorf("reloaded structure not served (missing edited text)")
	}

	// a further edit that deletes the current scene → fall back to the entry
	s.Reload(runtime.New(&model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "text", ID: "t", Text: "v3"}}}},
		GlobalState: model.GlobalState{Initial: map[string]any{}},
	}))
	if s.rt.CurrentScene() != "" {
		t.Errorf("scene removed by the edit should fall back to entry, got %q", s.rt.CurrentScene())
	}
	if html := render.RenderScene(s.rt, s.rt.CurrentScene()).HTML; !strings.Contains(html, "v3") {
		t.Errorf("entry fallback should render the reloaded entry scene")
	}
}
