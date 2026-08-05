package loader

import (
	"strings"
	"testing"
)

// The pre-`type` action-step format (`op: "set"` + `target`) parses to an
// empty step Type, which the runtime's step switch silently no-ops — a dead
// action with zero feedback. The loader must name it, like the scene://
// deprecation, without breaking the load.
func TestOldFormatActionStepProducesDiagnostic(t *testing.T) {
	docs := []map[string]any{
		{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "t"}},
		{"type": "action", "id": "old_set", "steps": []any{
			map[string]any{"op": "set", "target": "status", "value": "{{ 'hit' }}"},
		}},
		{"type": "action", "id": "new_set", "steps": []any{
			map[string]any{"type": "state.set", "path": "status", "value": "{{ 'hit' }}"},
		}},
	}
	app := FromDocs(docs)

	// Loading itself must not break: both actions are stored.
	if app.Actions["old_set"] == nil || app.Actions["new_set"] == nil {
		t.Fatalf("actions must still load, got %v", app.Actions)
	}

	var opWarn, targetWarn bool
	for _, d := range app.Diagnostics {
		if strings.Contains(d, `"op"`) && strings.Contains(d, "old_set") {
			opWarn = true
		}
		if strings.Contains(d, `"target"`) && strings.Contains(d, "old_set") {
			targetWarn = true
		}
	}
	if !opWarn || !targetWarn {
		t.Errorf("want deprecation warnings for both \"op\" and \"target\", got %v", app.Diagnostics)
	}

	// The new format must not be diagnosed (no false positive).
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "new_set") && (strings.Contains(d, `"op"`) || strings.Contains(d, `"target"`)) {
			t.Errorf("new-format step must not be diagnosed, got %v", app.Diagnostics)
		}
	}
}

// The real-world case that motivated the diagnostic: the counter example's
// set_status action was left in the old format and silently no-ops the
// physics scene's onCollide. After migration it must parse to a live
// state.set step with no old-format diagnostic.
func TestCounterExampleSetStatusMigrated(t *testing.T) {
	app, err := LoadDir("../../examples/counter")
	if err != nil {
		t.Fatalf("load counter example: %v", err)
	}
	act := app.Actions["set_status"]
	if act == nil {
		t.Fatal("counter example must define set_status")
	}
	if len(act.Steps) != 1 || act.Steps[0].Type != "state.set" || act.Steps[0].Path != "status" {
		t.Fatalf("set_status step = %+v, want one state.set step on path \"status\"", act.Steps)
	}
	for _, d := range app.Diagnostics {
		if strings.Contains(d, `"op"`) || strings.Contains(d, `"target"`) {
			t.Errorf("migrated counter example must produce no old-format diagnostic, got %v", app.Diagnostics)
		}
	}
}
