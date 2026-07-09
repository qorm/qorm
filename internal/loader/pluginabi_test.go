package loader

import (
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/pkg/qormext"
)

// loadManifest runs applyManifest on a manifest doc and returns the app + diags.
func loadManifest(doc map[string]any) (*model.App, []string) {
	app := &model.App{}
	var diags []string
	applyManifest(app, doc, &diags)
	return app, diags
}

func TestPluginABICompatible(t *testing.T) {
	cur := strconv.Itoa(qormext.ABIVersion)
	app, diags := loadManifest(map[string]any{"id": "x", "name": "X", "pluginABI": cur})
	if app.PluginABI != cur {
		t.Fatalf("PluginABI = %q, want %q", app.PluginABI, cur)
	}
	for _, d := range diags {
		if strings.Contains(d, "pluginABI") {
			t.Errorf("compatible ABI should not warn, got: %s", d)
		}
	}
}

func TestPluginABIIncompatibleWarns(t *testing.T) {
	bad := strconv.Itoa(qormext.ABIVersion + 1)
	_, diags := loadManifest(map[string]any{"id": "x", "name": "X", "pluginABI": bad})
	found := false
	for _, d := range diags {
		if strings.Contains(d, "pluginABI") && strings.Contains(d, "incompatible") {
			found = true
		}
	}
	if !found {
		t.Fatalf("incompatible pluginABI %q should warn, diags=%v", bad, diags)
	}
}

func TestPluginABIUnsetSilent(t *testing.T) {
	app, diags := loadManifest(map[string]any{"id": "x", "name": "X"})
	if app.PluginABI != "" {
		t.Errorf("unset PluginABI should be empty, got %q", app.PluginABI)
	}
	for _, d := range diags {
		if strings.Contains(d, "pluginABI") {
			t.Errorf("no pluginABI should not warn, got: %s", d)
		}
	}
}
