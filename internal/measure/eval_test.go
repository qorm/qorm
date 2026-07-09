package measure

import (
	"encoding/json"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// evalApp is a minimal runtime whose scene has the ids referenced below, so
// joinRows can attach intent (not strictly needed for the a11y kinds, which
// read measured fields directly, but keeps the join path exercised).
func evalApp() *qrt.Runtime {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "root", Children: []*model.Node{
			{Type: "button", ID: "submit_btn"},
			{Type: "input", ID: "email_field"},
			{Type: "text", ID: "faint"},
		}},
	}}
	return qrt.New(app)
}

// run marshals measured rows + checks and returns the parsed Eval report.
func run(t *testing.T, rt *qrt.Runtime, rows []map[string]any, checks []map[string]any) map[string]any {
	t.Helper()
	mb, _ := json.Marshal(rows)
	cb, _ := json.Marshal(checks)
	out, err := Eval(rt, mb, cb)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
	return rep
}

func firstResult(rep map[string]any) map[string]any {
	results := rep["results"].([]any)
	return results[0].(map[string]any)
}

// TestEvalA11yRole covers the rendered-role assertion (P1.5).
func TestEvalA11yRole(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "submit_btn", "role": "button", "ariaLabel": "Submit", "contrast": 7.1}}

	if rep := run(t, rt, rows, []map[string]any{{"id": "submit_btn", "role": "button"}}); rep["ok"] != true {
		t.Errorf("matching role should pass: %v", firstResult(rep)["fails"])
	}
	if rep := run(t, rt, rows, []map[string]any{{"id": "submit_btn", "role": "link"}}); rep["ok"] != false {
		t.Error("mismatched role should fail")
	}
}

// TestEvalA11yAriaLabel covers hasAriaLabel (P1.5).
func TestEvalA11yAriaLabel(t *testing.T) {
	rt := evalApp()
	labeled := []map[string]any{{"id": "email_field", "ariaLabel": "Email address"}}
	if rep := run(t, rt, labeled, []map[string]any{{"id": "email_field", "hasAriaLabel": true}}); rep["ok"] != true {
		t.Error("labeled field should pass hasAriaLabel:true")
	}
	unlabeled := []map[string]any{{"id": "email_field", "ariaLabel": ""}}
	if rep := run(t, rt, unlabeled, []map[string]any{{"id": "email_field", "hasAriaLabel": true}}); rep["ok"] != false {
		t.Error("unlabeled field should fail hasAriaLabel:true")
	}
}

// TestEvalA11yContrast covers contrastRatio, including the unavailable case (P1.5).
func TestEvalA11yContrast(t *testing.T) {
	rt := evalApp()
	if rep := run(t, rt, []map[string]any{{"id": "submit_btn", "contrast": 7.1}},
		[]map[string]any{{"id": "submit_btn", "contrastRatio": 4.5}}); rep["ok"] != true {
		t.Error("7.1 should satisfy min 4.5")
	}
	if rep := run(t, rt, []map[string]any{{"id": "faint", "contrast": 2.3}},
		[]map[string]any{{"id": "faint", "contrastRatio": 4.5}}); rep["ok"] != false {
		t.Error("2.3 should fail min 4.5")
	}
	// contrast 0 = client couldn't compute it — must fail, not silently pass.
	if rep := run(t, rt, []map[string]any{{"id": "faint", "contrast": 0.0}},
		[]map[string]any{{"id": "faint", "contrastRatio": 4.5}}); rep["ok"] != false {
		t.Error("unavailable contrast must not pass")
	}
}

// TestEvalFocusTrapRejected ensures an unsupported assertion never silently
// passes — a verification tool must not vouch for a check it cannot make.
func TestEvalFocusTrapRejected(t *testing.T) {
	rt := evalApp()
	rep := run(t, rt, []map[string]any{{"id": "root", "role": "dialog"}},
		[]map[string]any{{"id": "root", "focusTrap": true}})
	if rep["ok"] != false {
		t.Fatal("focusTrap must be rejected, not silently passed")
	}
}
