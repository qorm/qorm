package loader

import (
	"strings"
	"testing"
)

// TestUnknownStyleKeyWarns checks that a style key the renderer does not
// understand produces a non-fatal "warning:" diagnostic naming the node and
// the key, while keys from render.KnownStyleKeys stay silent.
func TestUnknownStyleKeyWarns(t *testing.T) {
	var diags []string
	n := buildNode(map[string]any{
		"type": "box",
		"id":   "b1",
		"style": map[string]any{
			"border":  "1px solid red",
			"padding": 8,
		},
	}, &diags, "main", nil)
	if n == nil || n.Style["border"] != "1px solid red" {
		t.Fatalf("unknown style key must be non-fatal (node keeps its style), got %+v", n)
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d, "padding") {
			t.Errorf("known style key must not warn, got: %s", d)
		}
		if strings.HasPrefix(d, "warning:") && strings.Contains(d, `"b1"`) && strings.Contains(d, `"border"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown style key should warn (node id + key in message), diags=%v", diags)
	}
}
