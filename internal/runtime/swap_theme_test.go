package runtime

import (
	"testing"

	"github.com/qorm/platform/internal/model"
)

func themeApp(theme string, initial map[string]any) *model.App {
	return &model.App{
		Entry: "main", Theme: theme,
		Scenes:      map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		GlobalState: model.GlobalState{Initial: initial},
	}
}

// New seeds the manifest theme into state.theme so bindings and the canvas
// backend see it; SwapAppPreservingState must re-seed the same way, or a
// manifest theme edit during hot reload stays pinned to the OLD manifest's
// value (the seed is runtime-written, not a user choice).
func TestSwapAppReseedsManifestTheme(t *testing.T) {
	rt := New(themeApp("dark", nil))
	if got := rt.State["theme"]; got != "dark" {
		t.Fatalf("New must seed state.theme = %v, want \"dark\"", got)
	}

	// Hot reload with a different manifest theme: the untouched seed follows.
	rt.SwapAppPreservingState(themeApp("apple-dark", nil))
	if got := rt.State["theme"]; got != "apple-dark" {
		t.Errorf("after swap state.theme = %v, want re-seeded \"apple-dark\"", got)
	}
	if got := rt.CurrentTheme(); got != "apple-dark" {
		t.Errorf("CurrentTheme() = %q, want \"apple-dark\"", got)
	}
}

// A theme the app/user set itself (e.g. a theme-toggle action) is NOT a seed:
// hot reload must preserve it.
func TestSwapAppPreservesUserSetTheme(t *testing.T) {
	rt := New(themeApp("dark", nil))
	rt.State["theme"] = "win11-dark" // as toggle_theme would write it

	rt.SwapAppPreservingState(themeApp("apple-dark", nil))
	if got := rt.State["theme"]; got != "win11-dark" {
		t.Errorf("user-set theme = %v, want preserved \"win11-dark\"", got)
	}
}

// Re-seeding keeps New's precedence: an explicit initial theme in the NEW
// manifest wins over the manifest's theme field.
func TestSwapAppExplicitInitialThemeWins(t *testing.T) {
	rt := New(themeApp("dark", nil))
	rt.SwapAppPreservingState(themeApp("dark", map[string]any{"theme": "apple-light"}))
	if got := rt.State["theme"]; got != "apple-light" {
		t.Errorf("state.theme = %v, want the explicit initial \"apple-light\"", got)
	}
}

// The seed from a manifest that DROPS its theme is removed on swap, so
// CurrentTheme() falls back to "auto" — the pre-seeding behavior a hot reload
// used to have.
func TestSwapAppDropsSeedWhenManifestThemeRemoved(t *testing.T) {
	rt := New(themeApp("dark", nil))
	rt.SwapAppPreservingState(themeApp("", nil))
	if _, exists := rt.State["theme"]; exists {
		t.Errorf("seed must be dropped when the new manifest has no theme, got %v", rt.State["theme"])
	}
	if got := rt.CurrentTheme(); got != "auto" {
		t.Errorf("CurrentTheme() = %q, want \"auto\"", got)
	}
}
