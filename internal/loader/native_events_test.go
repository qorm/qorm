package loader

import "testing"

// The native-canvas event keys (onCollide / onHoverIn / onHoverOut /
// onTouchStart / onTouchMove / onTouchEnd) must parse into the model's typed
// fields. They were silently dropped before, which left every JSON-loaded
// app with dead collision/hover/touch handlers in the canvas engine — the
// deepest layer of the counter physics demo failure (F7 / R5).
func TestNativeCanvasEventKeysParse(t *testing.T) {
	keys := []string{"onCollide", "onHoverIn", "onHoverOut", "onTouchStart", "onTouchMove", "onTouchEnd"}
	node := map[string]any{"type": "button", "id": "b"}
	for _, k := range keys {
		node[k] = map[string]any{"name": "act", "args": map[string]any{"msg": "hi"}}
	}
	app := FromDocs([]map[string]any{
		{"type": "scene", "id": "main", "root": node},
		{"type": "action", "id": "act", "steps": []any{}},
	})

	n := app.Scenes["main"]
	if n == nil {
		t.Fatal("scene did not load")
	}
	got := map[string]bool{
		"onCollide":    n.OnCollide != nil,
		"onHoverIn":    n.OnHoverIn != nil,
		"onHoverOut":   n.OnHoverOut != nil,
		"onTouchStart": n.OnTouchStart != nil,
		"onTouchMove":  n.OnTouchMove != nil,
		"onTouchEnd":   n.OnTouchEnd != nil,
	}
	for _, k := range keys {
		if !got[k] {
			t.Errorf("%s did not parse into the model's typed field", k)
		}
	}
	if n.OnCollide == nil || n.OnCollide.Name != "act" || n.OnCollide.Args["msg"] != "hi" {
		t.Errorf("onCollide parsed incorrectly: %+v", n.OnCollide)
	}

	// String shorthand parses too (same as onPress).
	app2 := FromDocs([]map[string]any{
		{"type": "scene", "id": "main", "root": map[string]any{"type": "button", "id": "b", "onCollide": "act"}},
		{"type": "action", "id": "act", "steps": []any{}},
	})
	if n2 := app2.Scenes["main"]; n2.OnCollide == nil || n2.OnCollide.Name != "act" {
		t.Errorf("string-shorthand onCollide parsed incorrectly: %+v", n2.OnCollide)
	}
}
