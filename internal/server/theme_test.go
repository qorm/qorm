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
	// default → apple
	if h := Page(themeApp(nil, ""), "x", 0); !strings.Contains(h, `class="qorm-theme-apple"`) {
		t.Error("default theme should be apple")
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
	// token sets + responsive rules present
	for _, m := range []string{
		"--accent:#007aff",                           // apple tokens
		"--accent:#2e7df6",                           // material tokens
		"--accent:#0a84ff",                           // dark tokens
		"@media (max-width:640px)", "max-width:100%", // responsive: PC frame + mobile full-bleed
	} {
		if !strings.Contains(h, m) {
			t.Errorf("page should define %q", m)
		}
	}
}
