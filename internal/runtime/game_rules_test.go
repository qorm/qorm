package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// state.setAt writes one array element at an evaluated index (game boards).
func TestStateSetAt(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root"},
		},
		Actions: map[string]*model.Action{
			"mark": {ID: "mark", Steps: []model.Step{
				{Type: "state.setAt", Path: "board", Index: "{{state.y * 10 + state.x}}", Value: "{{state.v}}"},
			}},
		},
	}
	rt := New(app)
	rt.State["board"] = []any{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0,
		0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	rt.State["x"] = 3.0
	rt.State["y"] = 1.0
	rt.State["v"] = 7.0
	rt.Dispatch("mark", nil)
	arr := rt.State["board"].([]any)
	if arr[13] != 7.0 {
		t.Errorf("board[13] = %v, want 7 after setAt(y*10+x)", arr[13])
	}
	// Out-of-range indices are quiet no-ops (games hit board edges).
	rt.State["x"] = 99.0
	rt.Dispatch("mark", nil)
	if len(arr) != 20 {
		t.Errorf("board grew/shrank on out-of-range setAt: %d", len(arr))
	}
}

// Scene key bindings parse (loader) and resolve for the current scene.
func TestSceneKeyBindings(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root"},
			"game": {Type: "column", ID: "groot"},
		},
		SceneKeys: map[string]map[string]string{
			"game": {"left": "moveLeft", "space": "drop"},
		},
	}
	rt := New(app)
	if _, ok := rt.KeyAction("left"); ok {
		t.Error("keys must not resolve outside their scene")
	}
	rt.Navigate("game", nil)
	if a, ok := rt.KeyAction("LEFT"); !ok || a != "moveLeft" {
		t.Errorf("KeyAction(LEFT) = %q,%v want moveLeft,true (case-insensitive)", a, ok)
	}
	if _, ok := rt.KeyAction("down"); ok {
		t.Error("unbound key resolved")
	}
}
