package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func themeApp(initial map[string]any, manifestTheme string) *runtime.Runtime {
	app := &model.App{
		Entry: "main", Theme: manifestTheme,
		GlobalState: model.GlobalState{Initial: initial},
		Scenes:      map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	}
	return runtime.New(app)
}

func TestThemingAndResponsive(t *testing.T) {
	// default → auto (Apple palette that follows the OS light/dark setting)
	if h := Page(themeApp(nil, ""), "x", 0); !strings.Contains(h, `class="qorm-theme-auto"`) {
		t.Error("default theme should be auto")
	}
	// an explicit apple pins light — no OS tracking
	if h := Page(themeApp(nil, "apple"), "x", 0); !strings.Contains(h, `class="qorm-theme-apple"`) {
		t.Error("explicit apple theme should apply")
	}
	// manifest theme
	if h := Page(themeApp(nil, "material"), "x", 0); !strings.Contains(h, `class="qorm-theme-material"`) {
		t.Error("manifest theme should apply")
	}
	// state.theme overrides manifest (this is how an agent restyles live)
	rt := themeApp(map[string]any{"theme": "dark"}, "material")
	h := Page(rt, "x", 0)
	if !strings.Contains(h, `class="qorm-theme-dark"`) {
		t.Error("state.theme should override the manifest theme")
	}
	// token sets + auto dark rule + responsive rules present
	for _, m := range []string{
		"--accent:#007aff",                           // apple tokens
		"--accent:#2e7df6",                           // material tokens
		"--accent:#0a84ff",                           // dark tokens
		"@media (prefers-color-scheme: dark)",        // auto follows the OS
		"@media (max-width:640px)", "max-width:100%", // responsive: PC frame + mobile full-bleed
	} {
		if !strings.Contains(h, m) {
			t.Errorf("page should define %q", m)
		}
	}
}

func TestDesignTokensRenderAsCSSVars(t *testing.T) {
	app := &model.App{
		Entry: "main",
		DesignTokens: map[string]model.DesignToken{
			"color.primary": {Type: "color", Value: "#0a84ff", Enforce: true},
			"space.card":    {Type: "size", Value: "16px"},
		},
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	}
	h := Page(runtime.New(app), "x", 0)
	// tokens land on the stage, sorted, as --qorm-token-<name with . -> ->
	if !strings.Contains(h, `#qorm-stage { --qorm-token-color-primary:#0a84ff; --qorm-token-space-card:16px; }`) {
		t.Error("designTokens should render as stage-scoped CSS variables")
	}
	// nothing emitted when the app declares no tokens
	if h := Page(themeApp(nil, ""), "x", 0); strings.Contains(h, `#qorm-stage { --qorm-token-`) {
		t.Error("page without designTokens should not emit token variables")
	}
}
