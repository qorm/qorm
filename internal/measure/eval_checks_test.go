package measure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// textApp has nodes carrying expressed text/labels so the "text" check's
// intentText side can be exercised alongside the measured "text" field.
func textApp() *qrt.Runtime {
	app := &model.App{Name: "textapp", Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "root", Children: []*model.Node{
			{Type: "text", ID: "greeting", Text: "Hello world"},
			{Type: "button", ID: "cta", Label: "Sign up"},
			{Type: "column", ID: "box"},
		}},
	}}
	return qrt.New(app)
}

// failsOf returns the fails list of the first result, or nil when it passed.
func failsOf(rep map[string]any) []string {
	res := firstResult(rep)
	raw, _ := res["fails"].([]any)
	out := make([]string, 0, len(raw))
	for _, f := range raw {
		out = append(out, f.(string))
	}
	return out
}

// TestEvalVisible covers the visible assertion in both directions.
func TestEvalVisible(t *testing.T) {
	rt := evalApp()
	shown := []map[string]any{{"id": "submit_btn", "visible": true}}
	if rep := run(t, rt, shown, []map[string]any{{"id": "submit_btn", "visible": true}}); rep["ok"] != true {
		t.Errorf("visible component should pass visible:true: %v", failsOf(rep))
	}
	if rep := run(t, rt, shown, []map[string]any{{"id": "submit_btn", "visible": false}}); rep["ok"] != false {
		t.Error("visible component should fail visible:false")
	}
	hidden := []map[string]any{{"id": "submit_btn", "visible": false}}
	if rep := run(t, rt, hidden, []map[string]any{{"id": "submit_btn", "visible": true}}); rep["ok"] != false {
		t.Error("hidden component should fail visible:true")
	}
}

// TestEvalType covers the type assertion (matched via the intent index).
func TestEvalType(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "submit_btn", "visible": true}}
	if rep := run(t, rt, rows, []map[string]any{{"id": "submit_btn", "type": "button"}}); rep["ok"] != true {
		t.Errorf("button should pass type:button: %v", failsOf(rep))
	}
	if rep := run(t, rt, rows, []map[string]any{{"id": "submit_btn", "type": "link"}}); rep["ok"] != false {
		t.Error("button should fail type:link")
	}
}

// TestEvalText covers the substring check against both the expressed
// intentText and the measured text field.
func TestEvalText(t *testing.T) {
	rt := textApp()
	// matches the expressed intent text from the model
	rep := run(t, rt, []map[string]any{{"id": "greeting"}}, []map[string]any{{"id": "greeting", "text": "Hello"}})
	if rep["ok"] != true {
		t.Errorf("intentText %q contains Hello: %v", "Hello world", failsOf(rep))
	}
	// matches only the measured text field (box has no intent text)
	rep = run(t, rt, []map[string]any{{"id": "box", "text": "Dynamic 42"}}, []map[string]any{{"id": "box", "text": "42"}})
	if rep["ok"] != true {
		t.Errorf("measured text contains 42: %v", failsOf(rep))
	}
	// expressed label counts too
	rep = run(t, rt, []map[string]any{{"id": "cta"}}, []map[string]any{{"id": "cta", "text": "Sign"}})
	if rep["ok"] != true {
		t.Errorf("label intentText contains Sign: %v", failsOf(rep))
	}
	// matches neither
	rep = run(t, rt, []map[string]any{{"id": "greeting", "text": "Bye"}}, []map[string]any{{"id": "greeting", "text": "zzz"}})
	if rep["ok"] != false {
		t.Error("no text contains zzz, so the check must fail")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "lacks") {
		t.Errorf("fail message should mention lack; got %v", f)
	}
}

// TestEvalNoOverflow covers the noOverflow assertion.
func TestEvalNoOverflow(t *testing.T) {
	rt := evalApp()
	overflowing := []map[string]any{{"id": "root", "overflowX": true}}
	rep := run(t, rt, overflowing, []map[string]any{{"id": "root", "noOverflow": true}})
	if rep["ok"] != false {
		t.Error("overflowing component must fail noOverflow")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "x-overflow") {
		t.Errorf("fail should cite x-overflow; got %v", f)
	}
	clipped := []map[string]any{{"id": "root", "overflowX": false}}
	if rep := run(t, rt, clipped, []map[string]any{{"id": "root", "noOverflow": true}}); rep["ok"] != true {
		t.Errorf("non-overflowing component should pass: %v", failsOf(rep))
	}
}

// TestEvalMinMaxSize covers minW/maxW/minH/maxH including exact-boundary
// equality (which satisfies both min and max).
func TestEvalMinMaxSize(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "box", "w": 100.0, "h": 50.0}}
	cases := []struct {
		check map[string]any
		ok    bool
	}{
		{map[string]any{"minW": 90.0}, true},
		{map[string]any{"minW": 100.0}, true}, // equality satisfies min
		{map[string]any{"minW": 101.0}, false},
		{map[string]any{"maxW": 110.0}, true},
		{map[string]any{"maxW": 100.0}, true}, // equality satisfies max
		{map[string]any{"maxW": 99.0}, false},
		{map[string]any{"minH": 50.0}, true},
		{map[string]any{"minH": 51.0}, false},
		{map[string]any{"maxH": 50.0}, true},
		{map[string]any{"maxH": 49.0}, false},
	}
	for _, c := range cases {
		c.check["id"] = "box"
		rep := run(t, rt, rows, []map[string]any{c.check})
		if rep["ok"] != c.ok {
			t.Errorf("check %v against 100x50: ok=%v, want %v (fails %v)", c.check, rep["ok"], c.ok, failsOf(rep))
		}
	}
}

// TestEvalXYTolerance covers the x/y position assertions with their +/-3 px
// tolerance.
func TestEvalXYTolerance(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "box", "x": 100.0, "y": 200.0}}
	cases := []struct {
		check map[string]any
		ok    bool
	}{
		{map[string]any{"x": 100.0}, true},
		{map[string]any{"x": 103.0}, true}, // +3 tolerated
		{map[string]any{"x": 104.0}, false},
		{map[string]any{"x": 97.0}, true}, // -3 tolerated
		{map[string]any{"x": 96.0}, false},
		{map[string]any{"y": 203.0}, true},
		{map[string]any{"y": 204.0}, false},
		{map[string]any{"y": 197.0}, true},
		{map[string]any{"y": 196.0}, false},
	}
	for _, c := range cases {
		c.check["id"] = "box"
		rep := run(t, rt, rows, []map[string]any{c.check})
		if rep["ok"] != c.ok {
			t.Errorf("check %v against x=100,y=200: ok=%v, want %v", c.check, rep["ok"], c.ok)
		}
	}
}

// TestEvalWithin covers the containment assertion, its +/-2 px slack, and the
// missing-parent failure.
func TestEvalWithin(t *testing.T) {
	rt := evalApp()
	parent := map[string]any{"id": "root", "x": 0.0, "y": 0.0, "w": 200.0, "h": 200.0}

	inside := []map[string]any{parent, {"id": "box", "x": 10.0, "y": 10.0, "w": 50.0, "h": 50.0}}
	if rep := run(t, rt, inside, []map[string]any{{"id": "box", "within": "root"}}); rep["ok"] != true {
		t.Errorf("contained child should pass: %v", failsOf(rep))
	}

	// touching the edge with exactly 2px of slack still passes
	slack := []map[string]any{parent, {"id": "box", "x": -2.0, "y": -2.0, "w": 204.0, "h": 204.0}}
	if rep := run(t, rt, slack, []map[string]any{{"id": "box", "within": "root"}}); rep["ok"] != true {
		t.Errorf("2px slack should pass: %v", failsOf(rep))
	}

	outside := []map[string]any{parent, {"id": "box", "x": 300.0, "y": 10.0, "w": 50.0, "h": 50.0}}
	rep := run(t, rt, outside, []map[string]any{{"id": "box", "within": "root"}})
	if rep["ok"] != false {
		t.Error("child outside the parent must fail within")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "not within") {
		t.Errorf("fail should say not within; got %v", f)
	}

	// parent id not measured -> the check cannot be verified -> must fail
	orphan := []map[string]any{{"id": "box", "x": 10.0, "y": 10.0, "w": 50.0, "h": 50.0}}
	rep = run(t, rt, orphan, []map[string]any{{"id": "box", "within": "root"}})
	if rep["ok"] != false {
		t.Error("within against an unmeasured parent must fail")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "not found") {
		t.Errorf("fail should say not found; got %v", f)
	}
}

// TestEvalBelow covers the vertical-order assertion and its tolerance.
func TestEvalBelow(t *testing.T) {
	rt := evalApp()
	header := map[string]any{"id": "root", "x": 0.0, "y": 0.0, "w": 200.0, "h": 50.0}

	// cy=47 == py+ph-3: tolerated
	atTolerance := []map[string]any{header, {"id": "box", "x": 0.0, "y": 47.0, "w": 10.0, "h": 10.0}}
	if rep := run(t, rt, atTolerance, []map[string]any{{"id": "box", "below": "root"}}); rep["ok"] != true {
		t.Errorf("y at header bottom - 3 should pass below: %v", failsOf(rep))
	}

	overlapping := []map[string]any{header, {"id": "box", "x": 0.0, "y": 46.0, "w": 10.0, "h": 10.0}}
	rep := run(t, rt, overlapping, []map[string]any{{"id": "box", "below": "root"}})
	if rep["ok"] != false {
		t.Error("box overlapping the header must fail below")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "not below") {
		t.Errorf("fail should say not below; got %v", f)
	}

	// target id not measured -> the check cannot be verified -> must fail, not
	// silently pass (symmetric with 'within'; a verification tool must never
	// report a check it cannot make).
	orphan := []map[string]any{{"id": "box", "x": 0.0, "y": 100.0, "w": 10.0, "h": 10.0}}
	rep = run(t, rt, orphan, []map[string]any{{"id": "box", "below": "missing"}})
	if rep["ok"] != false {
		t.Error("below against an unmeasured target must fail, not silently pass")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "not found") {
		t.Errorf("fail should say not found; got %v", f)
	}
}

// TestEvalUnknownKey verifies an unrecognised assertion key is rejected rather
// than silently ignored — a typo like {'minWidth':100} or {'visble':true} must
// not produce a vacuous pass.
func TestEvalUnknownKey(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "box", "w": 100.0, "visible": true}}
	for _, key := range []string{"minWidth", "visble", "heigth"} {
		rep := run(t, rt, rows, []map[string]any{{"id": "box", key: 100.0}})
		if rep["ok"] != false {
			t.Errorf("unknown key %q must fail, not vacuously pass", key)
			continue
		}
		if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "unknown check key") || !strings.Contains(f[0], key) {
			t.Errorf("fail for %q should name the unknown check key; got %v", key, f)
		}
	}
	// a recognised key alongside an unknown one still surfaces the unknown key
	rep := run(t, rt, rows, []map[string]any{{"id": "box", "visible": true, "visble": true}})
	if rep["ok"] != false {
		t.Error("a batch containing an unknown key must fail")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "unknown check key") {
		t.Errorf("fail should cite the unknown key; got %v", f)
	}
}

// TestEvalBackgroundNotAndColorNot cover the negative color assertions.
func TestEvalBackgroundNotAndColorNot(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "box", "background": "rgb(255, 0, 0)", "color": "rgb(10, 10, 10)"}}

	if rep := run(t, rt, rows, []map[string]any{{"id": "box", "backgroundNot": "255, 0, 0"}}); rep["ok"] != false {
		t.Error("background containing the forbidden value must fail")
	}
	if rep := run(t, rt, rows, []map[string]any{{"id": "box", "backgroundNot": "0, 255, 0"}}); rep["ok"] != true {
		t.Errorf("background without the forbidden value should pass: %v", failsOf(rep))
	}
	if rep := run(t, rt, rows, []map[string]any{{"id": "box", "colorNot": "10, 10, 10"}}); rep["ok"] != false {
		t.Error("color containing the forbidden value must fail")
	}
	if rep := run(t, rt, rows, []map[string]any{{"id": "box", "colorNot": "255"}}); rep["ok"] != true {
		t.Errorf("color without the forbidden value should pass: %v", failsOf(rep))
	}
}

// TestEvalNotRendered covers the check against an id with no measured element.
func TestEvalNotRendered(t *testing.T) {
	rt := evalApp()
	rep := run(t, rt, nil, []map[string]any{{"id": "ghost", "visible": true}})
	if rep["ok"] != false {
		t.Fatal("check for an unrendered id must fail")
	}
	if f := failsOf(rep); len(f) != 1 || !strings.Contains(f[0], "not rendered") {
		t.Errorf("fail should say not rendered; got %v", f)
	}
	// an unrendered element carries no actual values
	if res := firstResult(rep); res["actual"] != nil {
		t.Errorf("not-rendered result must have no actual; got %v", res["actual"])
	}
}

// TestEvalBadChecksJSON verifies malformed check input yields an error rather
// than a vacuous pass.
func TestEvalBadChecksJSON(t *testing.T) {
	rt := evalApp()
	_, err := Eval(rt, nil, []byte("{not json"))
	if err == nil || !strings.Contains(err.Error(), "bad checks JSON") {
		t.Errorf("malformed checks must error; got %v", err)
	}
}

// TestEvalTally verifies the report-level pass/fail accounting and the
// per-check pass flags across a mixed batch.
func TestEvalTally(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{
		{"id": "submit_btn", "visible": true, "role": "button"},
		{"id": "email_field", "visible": true},
	}
	checks := []map[string]any{
		{"id": "submit_btn", "visible": true},
		{"id": "submit_btn", "role": "button"},
		{"id": "email_field", "visible": false}, // fails
	}
	rep := run(t, rt, rows, checks)
	if rep["checks"] != float64(3) || rep["passed"] != float64(2) || rep["failed"] != float64(1) {
		t.Errorf("tally = checks:%v passed:%v failed:%v, want 3/2/1", rep["checks"], rep["passed"], rep["failed"])
	}
	if rep["ok"] != false {
		t.Error("any failure must make ok=false")
	}
	results := rep["results"].([]any)
	passFlags := []bool{}
	for _, r := range results {
		passFlags = append(passFlags, r.(map[string]any)["pass"] == true)
	}
	if passFlags[0] != true || passFlags[1] != true || passFlags[2] != false {
		t.Errorf("per-check pass = %v, want [true true false]", passFlags)
	}
}

// TestEvalActualValues verifies a rendered result echoes the measured values
// the agent needs to diagnose a failure.
func TestEvalActualValues(t *testing.T) {
	rt := evalApp()
	rows := []map[string]any{{"id": "submit_btn", "x": 5.0, "y": 6.0, "w": 100.0, "h": 40.0,
		"visible": true, "role": "button", "ariaLabel": "Submit", "contrast": 7.1,
		"color": "rgb(1,2,3)", "background": "rgb(4,5,6)"}}
	rep := run(t, rt, rows, []map[string]any{{"id": "submit_btn", "visible": true}})
	actual, ok := firstResult(rep)["actual"].(map[string]any)
	if !ok {
		t.Fatalf("result should carry actual values; got %v", firstResult(rep))
	}
	want := map[string]any{"x": 5.0, "y": 6.0, "w": 100.0, "h": 40.0, "visible": true,
		"role": "button", "ariaLabel": "Submit", "contrast": 7.1,
		"color": "rgb(1,2,3)", "background": "rgb(4,5,6)"}
	for k, v := range want {
		if actual[k] != v {
			t.Errorf("actual[%s] = %v, want %v", k, actual[k], v)
		}
	}
}

// TestEvalEmptyChecks verifies an empty check set is a vacuous pass with zero
// counts (the contract the CLI relies on for empty check input).
func TestEvalEmptyChecks(t *testing.T) {
	rt := evalApp()
	out, err := Eval(rt, nil, []byte("[]"))
	if err != nil {
		t.Fatal(err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep["ok"] != true || rep["checks"] != float64(0) || rep["passed"] != float64(0) || rep["failed"] != float64(0) {
		t.Errorf("empty checks should be a clean vacuous pass; got %v", rep)
	}
}
