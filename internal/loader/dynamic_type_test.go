package loader

import (
	"reflect"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Dynamic (bound) node types: {"type":"{{item.kind}}"} names a widget the
// renderer picks per item at render time. The loader cannot know that widget,
// so it must (a) keep the binding verbatim through build + serialize, (b) stand
// down on the checks that key off the widget kind instead of judging the
// literal "{{item.kind}}", and (c) keep running every kind-independent check.

// dynDocs wraps a scene root node in a minimal app doc set.
func dynDocs(root map[string]any) []map[string]any {
	return []map[string]any{
		{"type": "app", "id": "dyn", "name": "Dyn", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"feed": "array", "kind": "string"}}},
		{"type": "scene", "id": "main", "root": root},
	}
}

// polymorphicRoot is the canonical shape: a list whose ONE template node types
// itself from the item's data.
func polymorphicRoot() map[string]any {
	return map[string]any{
		"type": "column", "id": "root", "children": []any{
			map[string]any{
				"type": "list", "id": "feed", "data": "{{ state.feed }}",
				"renderItem": map[string]any{
					"type": "{{ item.kind }}", "id": "cell", "text": "{{ item.body }}",
				},
			},
		},
	}
}

// TestDynamicTypeLoadsClean is the core loader guard: a polymorphic template
// must not be diagnosed at all — the widget name is not knowable here.
func TestDynamicTypeLoadsClean(t *testing.T) {
	app := FromDocs(dynDocs(polymorphicRoot()))
	for _, d := range app.Diagnostics {
		t.Errorf("a bound type must load without diagnostics, got: %s", d)
	}
	tpl := app.Scenes["main"].Children[0].Template
	if tpl == nil || tpl.Type != "{{ item.kind }}" {
		t.Fatalf("the binding must be stored verbatim as the node type, got %+v", tpl)
	}
}

// TestDynamicTypeSkipsValueCheck guards the one static check that keys off the
// widget kind: `value` is only legitimate on the widgets in valueWidgets, and a
// bound type may well resolve to one of them — judging the literal
// "{{item.kind}}" would be a false positive on valid input.
func TestDynamicTypeSkipsValueCheck(t *testing.T) {
	bound := FromDocs(dynDocs(map[string]any{
		"type": "column", "id": "root", "children": []any{
			map[string]any{"type": "{{ state.kind }}", "id": "c", "value": "{{ state.kind }}"},
		},
	}))
	for _, d := range bound.Diagnostics {
		if strings.Contains(d, "'value'") {
			t.Errorf("bound type must not be judged against valueWidgets: %s", d)
		}
	}
	// The check itself is intact for a statically-known non-value widget.
	static := FromDocs(dynDocs(map[string]any{
		"type": "column", "id": "root", "children": []any{
			map[string]any{"type": "text", "id": "c", "value": "oops"},
		},
	}))
	found := false
	for _, d := range static.Diagnostics {
		found = found || strings.Contains(d, "'value'")
	}
	if !found {
		t.Errorf("the misconfigured-'value' warning must still fire on a static type: %v", static.Diagnostics)
	}
}

// TestDynamicTypeKeepsOtherChecks guards the boundary of the exemption: only
// kind-dependent checks stand down. Style keys, the deprecated `on` property
// and expression type checks are all kind-INDEPENDENT and must still fire.
func TestDynamicTypeKeepsOtherChecks(t *testing.T) {
	app := FromDocs(dynDocs(map[string]any{
		"type": "column", "id": "root", "children": []any{
			map[string]any{
				"type":  "{{ state.kind }}",
				"id":    "c",
				"style": map[string]any{"bogusKey": "1"},
				"on":    map[string]any{"press": "x"},
				"text":  "{{ state.kind * 2 }}", // string used numerically
			},
		},
	}))
	want := []string{"未知键", "'on'", "type mismatch"}
	for _, w := range want {
		found := false
		for _, d := range app.Diagnostics {
			found = found || strings.Contains(d, w)
		}
		if !found {
			t.Errorf("a bound type must not suppress the %q diagnostic: %v", w, app.Diagnostics)
		}
	}
}

// TestDynamicTypeRoundTrip guards the serialization identity: the binding is the
// author's source text, so build -> serialize -> build must return it byte for
// byte (a bundle rebuild or an MCP patch must not freeze the type to whatever
// it happened to resolve to).
func TestDynamicTypeRoundTrip(t *testing.T) {
	docs := dynDocs(polymorphicRoot())
	app := FromDocs(docs)
	again := FromDocs(AppToDocs(app))
	tpl := again.Scenes["main"].Children[0].Template
	if tpl == nil || tpl.Type != "{{ item.kind }}" {
		t.Fatalf("bound type lost in the round trip: %+v", tpl)
	}
	for _, d := range again.Diagnostics {
		t.Errorf("the round trip must stay diagnostic-free, got: %s", d)
	}
	// Docs form is the wire/patch form: it must be a fixed point.
	if a, b := AppToDocs(app), AppToDocs(again); !reflect.DeepEqual(nodeOf(t, a), nodeOf(t, b)) {
		t.Errorf("docs round trip is not the identity:\n%v\n%v", nodeOf(t, a), nodeOf(t, b))
	}
	// And the node serializer alone keeps it, for the patch surface.
	n := &model.Node{Type: "{{ item.kind }}", ID: "cell"}
	if got := NodeToJSON(n)["type"]; got != "{{ item.kind }}" {
		t.Errorf("NodeToJSON rewrote the bound type: %v", got)
	}
}

// nodeOf extracts the scene document's root node from a docs list.
func nodeOf(t *testing.T, docs []map[string]any) map[string]any {
	t.Helper()
	for _, d := range docs {
		if asString(d["type"]) == "scene" {
			root, _ := d["root"].(map[string]any)
			return root
		}
	}
	t.Fatalf("no scene doc in %v", docs)
	return nil
}
