package mcp

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// TestCloneAppPreservesAllFields guards against cloneApp dropping App fields:
// apply_patch swaps the clone in as the live app, so anything not carried over
// would vanish from every later render (components, i18n, theme, baseDir, ...).
func TestCloneAppPreservesAllFields(t *testing.T) {
	app := &model.App{
		ID: "x", Name: "X", Entry: "main",
		Scenes:        map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		Theme:         "material",
		DefaultLocale: "en",
		Locales:       map[string]map[string]string{"en": {"hi": "Hi"}},
		BaseDir:       "/some/dir",
		Branding:      true,
		Components:    map[string]*model.Node{"card": {Type: "card", ID: "c"}},
	}
	c := cloneApp(app)

	if c.Theme != "material" || c.DefaultLocale != "en" || c.BaseDir != "/some/dir" || !c.Branding {
		t.Errorf("cloneApp dropped a scalar field: theme=%q locale=%q baseDir=%q branding=%v",
			c.Theme, c.DefaultLocale, c.BaseDir, c.Branding)
	}
	if c.Locales["en"]["hi"] != "Hi" {
		t.Error("cloneApp dropped Locales")
	}
	if c.Components["card"] == nil {
		t.Error("cloneApp dropped Components")
	}
	// Scenes must be a deep copy, so patching the clone can't mutate the original.
	if c.Scenes["main"] == app.Scenes["main"] {
		t.Error("cloneApp should deep-copy Scenes, not share the same *Node")
	}
}
