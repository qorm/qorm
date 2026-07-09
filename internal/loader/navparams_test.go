package loader

import "testing"

// TestNavigateParamsParsed verifies the loader reads a navigate step's `params`
// object into Step.Params (parameter name -> value expression) without emitting
// an error diagnostic.
func TestNavigateParamsParsed(t *testing.T) {
	docs := []map[string]any{
		{
			"type": "action",
			"id":   "openProfile",
			"steps": []any{
				map[string]any{
					"type": "navigate",
					"to":   "profile",
					"params": map[string]any{
						"userId": "{{ userId }}",
						"name":   "{{ name }}",
					},
				},
			},
		},
	}
	app := FromDocs(docs)
	act, ok := app.Actions["openProfile"]
	if !ok || len(act.Steps) != 1 {
		t.Fatalf("action not parsed: %+v", app.Actions)
	}
	step := act.Steps[0]
	if step.Type != "navigate" || step.To != "profile" {
		t.Fatalf("navigate step mis-parsed: %+v", step)
	}
	if step.Params["userId"] != "{{ userId }}" || step.Params["name"] != "{{ name }}" {
		t.Fatalf("params not parsed: %#v", step.Params)
	}
	for _, d := range app.Diagnostics {
		if len(d) >= 6 && d[:6] == "error:" {
			t.Fatalf("unexpected error diagnostic: %s", d)
		}
	}
}

// TestNavigateWithoutParams confirms a params-less navigate step leaves
// Step.Params nil (backward compatible).
func TestNavigateWithoutParams(t *testing.T) {
	docs := []map[string]any{
		{
			"type":  "action",
			"id":    "go",
			"steps": []any{map[string]any{"type": "navigate", "to": "profile"}},
		},
	}
	app := FromDocs(docs)
	if p := app.Actions["go"].Steps[0].Params; p != nil {
		t.Fatalf("expected nil Params, got %#v", p)
	}
}
