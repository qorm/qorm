package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// deepLinkApp adds a third scene to the shared navApp for stack-depth tests.
func deepLinkApp() *model.App {
	app := navApp()
	app.Scenes["settings"] = &model.Node{Type: "scaffold"}
	return app
}

func TestNavigateToEntrySpellings(t *testing.T) {
	// The entry scene can be addressed as "" or by its id; both behave the same.
	rt := New(deepLinkApp())
	rt.Scene = "home"

	rt.NavigateTo("profile", map[string]any{"userId": "u-7"})
	if rt.Scene != "profile" || rt.RouteParams["userId"] != "u-7" {
		t.Fatalf("NavigateTo profile: scene=%q params=%v", rt.Scene, rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "push" {
		t.Fatalf("NavigateTo forward should push, got %q", dir)
	}

	// NavigateTo the frame directly below the top unwinds via NavigateBack: the
	// entry id "home" spells the same frame as "".
	rt.NavigateTo("home", nil)
	if rt.Scene != "home" || len(rt.RouteParams) != 0 {
		t.Fatalf("NavigateTo(home) should pop: scene=%q params=%v", rt.Scene, rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "pop" {
		t.Fatalf("NavigateTo previous frame should pop, got %q", dir)
	}

	// No-op: navigating to the scene already shown (either spelling) changes nothing.
	rt.NavigateTo("home", nil)
	rt.NavigateTo("", nil)
	if rt.Scene != "home" || len(rt.NavStack) != 0 {
		t.Fatalf("no-op NavigateTo changed state: scene=%q stack=%v", rt.Scene, rt.NavStack)
	}
	if dir := rt.TakeNavDir(); dir != "" {
		t.Fatalf("no-op NavigateTo should not set a direction, got %q", dir)
	}

	// Unknown non-entry scenes are ignored.
	rt.NavigateTo("ghost", nil)
	if rt.Scene != "home" || len(rt.NavStack) != 0 {
		t.Fatalf("unknown scene should be ignored: scene=%q stack=%v", rt.Scene, rt.NavStack)
	}
}

func TestNavigateToPushBeyondStack(t *testing.T) {
	// When the target is NOT the frame below the top, NavigateTo pushes a new
	// frame (browser jump-forward to an unrelated page).
	rt := New(deepLinkApp())
	rt.Scene = "home"
	rt.NavigateTo("profile", nil)  // stack: [home]
	rt.NavigateTo("settings", nil) // stack: [home, profile]
	rt.NavigateTo("profile", map[string]any{"userId": "fresh"})
	// "profile" is below the top ("settings") but not directly below? Actually it
	// IS directly below the top — so this unwinds via Back, restoring the OLD
	// frame params (nil -> empty), not the fresh params.
	if rt.Scene != "profile" {
		t.Fatalf("expected profile, got %q", rt.Scene)
	}
	if len(rt.RouteParams) != 0 {
		t.Fatalf("back-unwind restores the frame's own (empty) params, got %v", rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "pop" {
		t.Fatalf("below-top target should pop, got %q", dir)
	}

	// Now stack: [home, profile]. Jump to settings again: directly below top? Top
	// is profile, below-top frame is home; settings is neither current nor
	// below-top, so this is a push.
	rt.NavigateTo("settings", map[string]any{"tab": "general"})
	if rt.Scene != "settings" || rt.RouteParams["tab"] != "general" {
		t.Fatalf("unrelated target should push: scene=%q params=%v", rt.Scene, rt.RouteParams)
	}
	if dir := rt.TakeNavDir(); dir != "push" {
		t.Fatalf("unrelated target should push, got %q", dir)
	}
	if len(rt.NavStack) != 2 {
		t.Fatalf("stack depth = %d, want 2 ([home, profile])", len(rt.NavStack))
	}
}

func TestNavigateBackNilFrameParams(t *testing.T) {
	// A frame recorded with nil params (via NavigateTo) restores an empty map on
	// pop, never nil.
	rt := New(deepLinkApp())
	rt.Scene = "home"
	rt.NavStack = append(rt.NavStack, navFrame{Scene: "home", Params: nil})
	rt.Scene = "profile"
	rt.NavigateBack()
	if rt.RouteParams == nil {
		t.Fatal("NavigateBack must restore an empty map, not nil")
	}
}

func TestRoutePathVariants(t *testing.T) {
	rt := New(deepLinkApp())

	// Entry scene with params: addressed without a scene key, params only.
	rt.Scene = "home"
	rt.RouteParams = map[string]any{"promo": "spring"}
	if got := rt.RoutePath(); got != "/?promo=spring" {
		t.Errorf("entry with params: got %q", got)
	}

	// Non-entry scene without params: scene key only.
	rt.Scene = "profile"
	rt.RouteParams = map[string]any{}
	if got := rt.RoutePath(); got != "/?scene=profile" {
		t.Errorf("scene only: got %q", got)
	}

	// Param values are stringified (float -> integer text, bool -> true).
	rt.RouteParams = map[string]any{"page": float64(3), "admin": true}
	if got := rt.RoutePath(); got != "/?admin=true&page=3&scene=profile" {
		t.Errorf("stringified params: got %q", got)
	}

	// Special characters survive encoding (url.Values.Encode escapes them).
	rt.RouteParams = map[string]any{"q": "a b&c=d"}
	if got := rt.RoutePath(); got != "/?q=a+b%26c%3Dd&scene=profile" {
		t.Errorf("escaped params: got %q", got)
	}
}

func TestRoutePathNavigateToPathRoundTrip(t *testing.T) {
	rt := New(deepLinkApp())
	rt.Scene = "home"

	// Push a scene with params, capture its URL, then drive a fresh runtime from
	// that URL: scene and (string-coerced) params must match.
	rt.Navigate("profile", map[string]any{"userId": "u-42", "name": "A b&c"})
	path := rt.RoutePath()

	rt2 := New(deepLinkApp())
	rt2.Scene = "home"
	// Strip the leading "/?" to get the raw query NavigateToPath expects.
	query := path[len("/?"):]
	rt2.NavigateToPath(query)
	if rt2.Scene != "profile" {
		t.Fatalf("round-trip scene: got %q", rt2.Scene)
	}
	if rt2.RouteParams["userId"] != "u-42" || rt2.RouteParams["name"] != "A b&c" {
		t.Fatalf("round-trip params (special chars must decode): %v", rt2.RouteParams)
	}
	// Route params from a URL are always strings.
	if _, ok := rt2.RouteParams["userId"].(string); !ok {
		t.Fatalf("URL route params should be strings, got %T", rt2.RouteParams["userId"])
	}
}

func TestNavigateToPathEdgeCases(t *testing.T) {
	rt := New(deepLinkApp())
	rt.Scene = "home"

	// Duplicate keys keep the first value; a bare key becomes an empty string.
	rt.NavigateToPath("scene=profile&tag=one&tag=two&flag")
	if rt.Scene != "profile" {
		t.Fatalf("scene = %q", rt.Scene)
	}
	if rt.RouteParams["tag"] != "one" {
		t.Errorf("duplicate key should keep first value, got %v", rt.RouteParams["tag"])
	}
	if got := rt.RouteParams["flag"]; got != "" {
		t.Errorf("bare key should map to an empty string, got %v", got)
	}

	// A garbage query (invalid escaping) parses to nothing and navigates to the
	// entry — a no-op here since home is the entry.
	rt2 := New(deepLinkApp())
	rt2.Scene = "home"
	rt2.NavigateToPath("%zz=bad")
	if rt2.Scene != "home" || len(rt2.RouteParams) != 0 {
		t.Errorf("invalid query should be a no-op at entry: scene=%q params=%v", rt2.Scene, rt2.RouteParams)
	}

	// A scene-only query from a non-entry scene pops back to the entry.
	rt3 := New(deepLinkApp())
	rt3.Scene = "home"
	rt3.NavigateTo("profile", nil)
	rt3.NavigateToPath("scene=") // empty scene value -> entry, which is below-top
	if rt3.Scene != "home" {
		t.Errorf("scene= (empty) should return to entry, got %q", rt3.Scene)
	}
	if dir := rt3.TakeNavDir(); dir != "pop" {
		t.Errorf("return to entry should pop, got %q", dir)
	}
}

func TestNavigatePreservesNavDirLifecycle(t *testing.T) {
	// TakeNavDir clears the direction after it ships (the client consumes it once).
	rt := New(deepLinkApp())
	rt.Scene = "home"
	rt.Navigate("profile", nil)
	if got := rt.TakeNavDir(); got != "push" {
		t.Fatalf("first take = %q", got)
	}
	if got := rt.TakeNavDir(); got != "" {
		t.Fatalf("second take should be empty, got %q", got)
	}
}
