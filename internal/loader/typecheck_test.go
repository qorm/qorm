package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// typeDocs builds a minimal app (manifest + scene + action) with the given
// state schema. The scene doc deliberately sorts before the app doc to prove
// FromDocs applies manifests first (two-pass).
func typeDocs(schema map[string]any) []map[string]any {
	return []map[string]any{
		{
			"type": "scene",
			"id":   "main",
			"root": map[string]any{
				"type": "column",
				"id":   "root",
				"children": []any{
					map[string]any{"type": "text", "id": "lbl", "text": "{{ state.count * 2 }}"},
				},
			},
		},
		{
			"type": "action",
			"id":   "increment",
			"steps": []any{
				map[string]any{"type": "state.set", "path": "count", "value": "{{ count + 1 }}"},
				map[string]any{"type": "state.set", "path": "count", "value": "{{ count - 1 }}"},
			},
		},
		{
			"type": "app",
			"id":   "typecheck",
			"name": "TypeCheck",
			"globalState": map[string]any{
				"schema": schema,
			},
		},
	}
}

func typeErrors(diags []string) []string {
	var out []string
	for _, d := range diags {
		if strings.Contains(d, "type mismatch") {
			out = append(out, d)
		}
	}
	return out
}

func TestLoaderTypeCheck(t *testing.T) {
	t.Run("string schema used numerically is reported", func(t *testing.T) {
		app := FromDocs(typeDocs(map[string]any{"count": "string"}))
		errs := typeErrors(app.Diagnostics)
		if len(errs) != 2 { // scene `state.count * 2` + action `count - 1` (`+` is concat)
			t.Fatalf("want 2 type errors, got %d: %v", len(errs), app.Diagnostics)
		}
		sceneErr := errs[0]
		if !strings.HasPrefix(sceneErr, "error: ") {
			t.Errorf("type error must carry the error: prefix, got %q", sceneErr)
		}
		for _, want := range []string{`[Scene: main]`, `(id: "lbl")`, "state.count is string, used as number", "{{ state.count * 2 }}"} {
			if !strings.Contains(sceneErr, want) {
				t.Errorf("scene diagnostic %q missing %q", sceneErr, want)
			}
		}
		actionErr := errs[1]
		for _, want := range []string{"error: ", "[Action: increment]", "count is string, used as number", "{{ count - 1 }}"} {
			if !strings.Contains(actionErr, want) {
				t.Errorf("action diagnostic %q missing %q", actionErr, want)
			}
		}
	})

	t.Run("number schema produces no type errors", func(t *testing.T) {
		app := FromDocs(typeDocs(map[string]any{"count": "number"}))
		if errs := typeErrors(app.Diagnostics); len(errs) != 0 {
			t.Fatalf("want no type errors, got %v", errs)
		}
	})

	t.Run("missing schema entry passes as unknown", func(t *testing.T) {
		app := FromDocs(typeDocs(map[string]any{"other": "string"}))
		if errs := typeErrors(app.Diagnostics); len(errs) != 0 {
			t.Fatalf("want no type errors for undeclared keys, got %v", errs)
		}
	})
}

// TestExamplesNoTypeErrors is the no-false-positive gate: every shipped
// example must load without a single static type-error diagnostic.
func TestExamplesNoTypeErrors(t *testing.T) {
	examples := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(examples)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		app, err := LoadDir(filepath.Join(examples, e.Name()))
		if err != nil {
			t.Errorf("%s: load: %v", e.Name(), err)
			continue
		}
		checked++
		if errs := typeErrors(app.Diagnostics); len(errs) != 0 {
			t.Errorf("%s: unexpected type errors: %v", e.Name(), errs)
		}
	}
	if checked == 0 {
		t.Fatal("no examples were checked")
	}
}
