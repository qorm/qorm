package loader

import (
	"strings"
	"testing"
)

// TestBuildNodeWhen verifies the loader parses a responsive `when` node:
// condition string plus then/else built as full nodes (recursively).
func TestBuildNodeWhen(t *testing.T) {
	doc := map[string]any{
		"type": "scene", "id": "main",
		"root": map[string]any{
			"type": "column", "id": "root",
			"children": []any{
				map[string]any{
					"type":      "when",
					"id":        "sw",
					"condition": "{{ viewport.width >= 768 }}",
					"then": map[string]any{
						"type": "row", "id": "wide",
						"children": []any{map[string]any{"type": "text", "id": "t1", "text": "wide"}},
					},
					"else": map[string]any{"type": "column", "id": "narrow"},
				},
			},
		},
	}
	app := FromDocs([]map[string]any{doc})
	root := app.Scenes["main"]
	if root == nil || len(root.Children) != 1 {
		t.Fatalf("scene not built: %+v", root)
	}
	w := root.Children[0]
	if w.Type != "when" || w.Condition != "{{ viewport.width >= 768 }}" {
		t.Fatalf("when node not parsed: type=%q condition=%q", w.Type, w.Condition)
	}
	if w.Then == nil || w.Then.Type != "row" || w.Then.ID != "wide" {
		t.Fatalf("then branch not built: %+v", w.Then)
	}
	if len(w.Then.Children) != 1 || w.Then.Children[0].Text != "wide" {
		t.Fatalf("then branch children not built recursively: %+v", w.Then.Children)
	}
	if w.Else == nil || w.Else.Type != "column" || w.Else.ID != "narrow" {
		t.Fatalf("else branch not built: %+v", w.Else)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("unexpected diagnostic: %s", d)
	}
}

// TestWhenSerializeRoundTrip checks NodeToJSON emits condition/then/else from
// the struct fields, and that re-loading the emitted JSON reproduces the node.
func TestWhenSerializeRoundTrip(t *testing.T) {
	src := map[string]any{
		"type":      "when",
		"id":        "sw",
		"condition": "{{ viewport.orientation == 'portrait' }}",
		"then":      map[string]any{"type": "text", "id": "p", "text": "portrait"},
		"else":      map[string]any{"type": "text", "id": "l", "text": "landscape"},
	}
	n := BuildNode(src)
	out := NodeToJSON(n)
	if out["condition"] != "{{ viewport.orientation == 'portrait' }}" {
		t.Fatalf("condition not serialised: %v", out["condition"])
	}
	thenOut, ok := out["then"].(map[string]any)
	if !ok || thenOut["id"] != "p" || thenOut["text"] != "portrait" {
		t.Fatalf("then not serialised from struct: %v", out["then"])
	}
	elseOut, ok := out["else"].(map[string]any)
	if !ok || elseOut["id"] != "l" {
		t.Fatalf("else not serialised from struct: %v", out["else"])
	}
	// round-trip: rebuild from the emitted JSON
	again := BuildNode(out)
	if again.Condition != n.Condition || again.Then == nil || again.Then.Text != "portrait" || again.Else == nil {
		t.Fatalf("round-trip lost when fields: %+v", again)
	}
}

// TestWhenTypeCheckCondition ensures viewport vars are typed for the static
// checker: misusing viewport.orientation (a string) as a number is diagnosed.
func TestWhenTypeCheckCondition(t *testing.T) {
	doc := map[string]any{
		"type": "scene", "id": "main",
		"root": map[string]any{
			"type": "when", "id": "sw",
			"condition": "{{ viewport.orientation * 2 }}",
			"then":      map[string]any{"type": "text", "id": "a", "text": "x"},
		},
	}
	app := FromDocs([]map[string]any{doc})
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "viewport.orientation") && strings.Contains(d, "type mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a type-mismatch diagnostic for viewport.orientation, got %v", app.Diagnostics)
	}
}
