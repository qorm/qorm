package runtime

import (
	"testing"

	"github.com/qorm/platform/internal/model"
)

func TestHandleSceneErrorFalseWithoutBoundary(t *testing.T) {
	rt := New(&model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}},
	})
	rt.ClearPendingEnter()
	if rt.HandleSceneError("main", "action", "") {
		t.Fatal("empty error message must not trigger a boundary")
	}
	if rt.HandleSceneError("main", "action", "boom") {
		t.Fatal("runtime without error boundary must not redirect")
	}
	if rt.LastBoundaryError.Message != "boom" || rt.LastBoundaryError.Level != "scene" {
		t.Fatalf("LastBoundaryError = %#v, want recorded scene error", rt.LastBoundaryError)
	}
}

func TestPendingEnterAndDispatchErrCoverage(t *testing.T) {
	rt := New(&model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": &model.Node{Type: "text", ID: "mainText", Text: "main"},
			"oops": &model.Node{Type: "text", ID: "oopsText", Text: "oops"},
		},
		ErrorBoundary: &model.SceneErrorBoundary{Scene: "oops"},
		Actions: map[string]*model.Action{
			"boom": {ID: "boom", Script: "let xs = [1]\nxs[4] = 1"},
		},
	})
	if !rt.PendingEnter() {
		t.Fatal("new runtime should start with pendingEnter")
	}
	if err := rt.DispatchErr("boom", nil); err == nil {
		t.Fatal("DispatchErr should surface nested script failure")
	}
	if !rt.PendingEnter() {
		t.Fatal("scene fallback should mark the fallback scene pending")
	}
	rt.ClearPendingEnter()
	if rt.PendingEnter() {
		t.Fatal("ClearPendingEnter should clear the mark")
	}
}
