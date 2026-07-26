package loader

import (
	"strings"
	"testing"
)

// scopeDocs assembles a minimal app around one scene root node.
func scopeDocs(root map[string]any) []map[string]any {
	return []map[string]any{
		{"type": "app", "id": "t", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"rows": "array"}, "initial": map[string]any{"rows": []any{}}}},
		{"type": "scene", "id": "main", "root": root},
	}
}

// TestTemplateScopeNamesNotFlagged: the bare names a renderItem template binds
// (item/index/first/last and their `as`-derived forms, across nesting) are
// legitimate dot-less bindings — the "add a state./prop. prefix" suggestion
// must not fire on them, in text, `if`, or a nested when condition.
func TestTemplateScopeNamesNotFlagged(t *testing.T) {
	app := FromDocs(scopeDocs(map[string]any{
		"type": "list", "id": "l", "data": "{{ state.rows }}", "as": "section",
		"renderItem": map[string]any{
			"type": "column", "id": "sec", "children": []any{
				map[string]any{"type": "text", "id": "h", "text": "{{ sectionIndex + 1 }}. {{ section.title }}"},
				map[string]any{"type": "when", "id": "w", "condition": "{{ sectionFirst }}",
					"then": map[string]any{"type": "text", "id": "wt", "text": "top"}},
				map[string]any{
					"type": "list", "id": "inner", "data": "{{ section.rows }}",
					"renderItem": map[string]any{
						"type": "text", "id": "r", "text": "{{ index }}:{{ item }}",
						"if": "{{ !last }}",
					},
				},
			},
		},
	}))
	for _, d := range app.Diagnostics {
		t.Errorf("template scope names must load clean, got diagnostic: %s", d)
	}
}

// TestTemplateUnknownBareNameStillFlagged: the whitelist is scoped to the
// names actually bound — an unrelated bare identifier inside a template keeps
// the missing-prefix suggestion, and so does one outside any template.
func TestTemplateUnknownBareNameStillFlagged(t *testing.T) {
	app := FromDocs(scopeDocs(map[string]any{
		"type": "column", "id": "c", "children": []any{
			map[string]any{"type": "text", "id": "bare", "text": "{{ counter }}"},
			map[string]any{"type": "list", "id": "l", "data": "{{ state.rows }}",
				"renderItem": map[string]any{"type": "text", "id": "r", "text": "{{ indexes }}"}},
		},
	}))
	var hits int
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "counter") || strings.Contains(d, "indexes") {
			hits++
		}
	}
	if hits != 2 {
		t.Errorf("bare non-scope identifiers should still be flagged (want 2 hits, got %d): %v", hits, app.Diagnostics)
	}
}

// TestTemplateAliasScopedToItsList: an alias's names are only in scope inside
// that list's own template, not in sibling templates.
func TestTemplateAliasScopedToItsList(t *testing.T) {
	app := FromDocs(scopeDocs(map[string]any{
		"type": "column", "id": "c", "children": []any{
			map[string]any{"type": "list", "id": "a", "data": "{{ state.rows }}", "as": "row",
				"renderItem": map[string]any{"type": "text", "id": "ra", "text": "{{ rowIndex }}"}},
			map[string]any{"type": "list", "id": "b", "data": "{{ state.rows }}",
				"renderItem": map[string]any{"type": "text", "id": "rb", "text": "{{ rowIndex }}"}},
		},
	}))
	var hits []string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "rowIndex") {
			hits = append(hits, d)
		}
	}
	if len(hits) != 1 || !strings.Contains(hits[0], `"rb"`) {
		t.Errorf("rowIndex should only be legal inside the as:\"row\" list's template, got %v", hits)
	}
}

// TestBadAliasWarns: an alias the renderer would ignore (reserved context
// root, or not a plain identifier) gets a load-time warning naming the node.
func TestBadAliasWarns(t *testing.T) {
	for _, as := range []string{"state", "my row"} {
		app := FromDocs(scopeDocs(map[string]any{
			"type": "list", "id": "l", "data": "{{ state.rows }}", "as": as,
			"renderItem": map[string]any{"type": "text", "id": "r", "text": "{{ item }}"},
		}))
		var found bool
		for _, d := range app.Diagnostics {
			if strings.Contains(d, "as:") && strings.Contains(d, `"l"`) {
				found = true
			}
		}
		if !found {
			t.Errorf("as=%q: expected an unusable-alias warning, got %v", as, app.Diagnostics)
		}
	}
	// A good alias warns about nothing.
	app := FromDocs(scopeDocs(map[string]any{
		"type": "list", "id": "l", "data": "{{ state.rows }}", "as": "row",
		"renderItem": map[string]any{"type": "text", "id": "r", "text": "{{ row.name }}"},
	}))
	for _, d := range app.Diagnostics {
		t.Errorf("valid alias must load clean, got: %s", d)
	}
}

// TestAsAliasRoundTrips: `as` is an untyped prop, so NodeToJSON carries it
// back out and a rebuilt node keeps the alias.
func TestAsAliasRoundTrips(t *testing.T) {
	n := BuildNode(map[string]any{
		"type": "list", "id": "l", "data": "{{ state.rows }}", "as": "row",
		"renderItem": map[string]any{"type": "text", "id": "r", "text": "{{ row.name }}"},
	})
	out := NodeToJSON(n)
	if out["as"] != "row" {
		t.Fatalf("as alias lost in serialization: %v", out)
	}
	if again := BuildNode(out); again.Props["as"] != "row" || again.Template == nil {
		t.Fatalf("as alias lost on rebuild: %+v", again)
	}
}
