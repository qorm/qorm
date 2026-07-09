package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// navApp builds a two-scene app with an openProfile action that navigates with
// route params and a back action that pops the stack.
func navApp() *model.App {
	return &model.App{
		Entry: "home",
		Scenes: map[string]*model.Node{
			"home":    {Type: "scaffold"},
			"profile": {Type: "scaffold"},
		},
		Actions: map[string]*model.Action{
			"openProfile": {ID: "openProfile", Steps: []model.Step{{
				Type: "navigate", To: "profile",
				Params: map[string]string{"userId": "{{ userId }}", "name": "{{ name }}"},
			}}},
			"back": {ID: "back", Steps: []model.Step{{Type: "navigate", Back: true}}},
		},
	}
}

func TestNavigateWithParams(t *testing.T) {
	rt := New(navApp())
	rt.Scene = "home"

	// route.* is an empty map at the entry scene, not nil.
	if rt.RouteParams == nil || len(rt.RouteParams) != 0 {
		t.Fatalf("initial RouteParams should be empty map, got %#v", rt.RouteParams)
	}

	rt.Dispatch("openProfile", map[string]any{"userId": "u-101", "name": "Aurora"})
	if rt.Scene != "profile" {
		t.Fatalf("expected scene profile, got %q", rt.Scene)
	}
	if rt.RouteParams["userId"] != "u-101" || rt.RouteParams["name"] != "Aurora" {
		t.Fatalf("RouteParams not set: %#v", rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "push" {
		t.Fatalf("expected push dir, got %q", dir)
	}

	// route.* must be readable from scene bindings.
	ctx := rt.sceneCtx()
	route, ok := ctx["route"].(map[string]any)
	if !ok || route["userId"] != "u-101" {
		t.Fatalf("sceneCtx route missing/wrong: %#v", ctx["route"])
	}
	if got := EvalBinding("{{ route.userId }}", ctx); got != "u-101" {
		t.Fatalf("binding route.userId = %v, want u-101", got)
	}

	// back restores the previous frame's scene AND its (empty) params.
	rt.Dispatch("back", nil)
	if rt.Scene != "home" {
		t.Fatalf("back: expected home, got %q", rt.Scene)
	}
	if len(rt.RouteParams) != 0 {
		t.Fatalf("back: expected empty RouteParams, got %#v", rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "pop" {
		t.Fatalf("expected pop dir, got %q", dir)
	}
}

// TestNavigateBackRestoresParams verifies params travel with the stack across a
// three-scene push chain: popping restores each intermediate frame's params.
func TestNavigateBackRestoresParams(t *testing.T) {
	app := navApp()
	app.Scenes["settings"] = &model.Node{Type: "scaffold"}
	rt := New(app)
	rt.Scene = "home"

	rt.Navigate("profile", map[string]any{"userId": "u-1"})
	rt.Navigate("settings", map[string]any{"tab": "privacy"})
	if rt.Scene != "settings" || rt.RouteParams["tab"] != "privacy" {
		t.Fatalf("after 2 pushes: scene=%q params=%#v", rt.Scene, rt.RouteParams)
	}

	rt.NavigateBack()
	if rt.Scene != "profile" || rt.RouteParams["userId"] != "u-1" {
		t.Fatalf("pop 1: expected profile/u-1, got %q %#v", rt.Scene, rt.RouteParams)
	}
	rt.NavigateBack()
	if rt.Scene != "home" || len(rt.RouteParams) != 0 {
		t.Fatalf("pop 2: expected home/empty, got %q %#v", rt.Scene, rt.RouteParams)
	}
	// Popping an empty stack is a no-op.
	rt.NavigateBack()
	if rt.Scene != "home" {
		t.Fatalf("pop empty: scene changed to %q", rt.Scene)
	}
}

// TestNavigateNilParams ensures a param-less navigate yields an empty (non-nil)
// route map so `{{ route.x }}` reads nil cleanly instead of panicking.
func TestNavigateNilParams(t *testing.T) {
	rt := New(navApp())
	rt.Scene = "home"
	rt.Navigate("profile", nil)
	if rt.RouteParams == nil {
		t.Fatal("RouteParams should be empty map after nil-param navigate")
	}
	// A single-binding expression returns the typed value; a missing route key
	// resolves to nil (and stringifies to "").
	if got := EvalBinding("{{ route.missing }}", rt.sceneCtx()); got != nil {
		t.Fatalf("route.missing should be nil, got %v", got)
	}
	if got := Stringify(EvalBinding("{{ route.missing }}", rt.sceneCtx())); got != "" {
		t.Fatalf("route.missing should stringify to empty, got %q", got)
	}
}
