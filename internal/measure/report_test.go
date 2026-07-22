package measure

import (
	"encoding/json"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestNodeIndexFieldsAndBranches verifies the intent index captures each
// node's expressed fields and walks children, list templates, and both when
// branches; nodes without an id are not indexed.
func TestNodeIndexFieldsAndBranches(t *testing.T) {
	m := map[string]map[string]any{}
	NodeIndex(nil, m) // nil-safe no-op
	if len(m) != 0 {
		t.Fatalf("nil node must index nothing; got %v", m)
	}

	n := &model.Node{Type: "column", Children: []*model.Node{
		{Type: "text", ID: "t1", Text: "Hello"},
		{Type: "button", ID: "b1", Label: "Go", Value: "{{state.x}}"},
		{Type: "input", ID: "i1", Value: "{{state.email}}"},
		{Type: "text", Text: "no id - must not be indexed"},
	}, Template: &model.Node{Type: "row", ID: "tpl"},
		Then: &model.Node{Type: "text", ID: "then1"},
		Else: &model.Node{Type: "text", ID: "else1"},
	}
	NodeIndex(n, m)

	if len(m) != 6 {
		t.Fatalf("indexed %d entries, want 6: %v", len(m), m)
	}
	if m["t1"]["type"] != "text" || m["t1"]["text"] != "Hello" {
		t.Errorf("t1 = %v, want type+text", m["t1"])
	}
	if _, has := m["t1"]["label"]; has {
		t.Error("empty label must be omitted")
	}
	if m["b1"]["label"] != "Go" || m["b1"]["binding"] != "{{state.x}}" {
		t.Errorf("b1 = %v, want label+binding", m["b1"])
	}
	if m["i1"]["binding"] != "{{state.email}}" {
		t.Errorf("i1 = %v, want binding", m["i1"])
	}
	if _, has := m["i1"]["text"]; has {
		t.Error("empty text must be omitted")
	}
	for _, id := range []string{"tpl", "then1", "else1"} {
		if _, ok := m[id]; !ok {
			t.Errorf("template/then/else branch node %q must be indexed", id)
		}
	}
}

// TestIntentTextPrecedence verifies the text > label > binding preference and
// that empty or non-string values are skipped.
func TestIntentTextPrecedence(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"text": "T", "label": "L", "binding": "B"}, "T"},
		{map[string]any{"label": "L", "binding": "B"}, "L"},
		{map[string]any{"binding": "B"}, "B"},
		{map[string]any{"type": "text"}, ""},
		{map[string]any{"text": "", "label": "L"}, "L"}, // empty text skipped
		{map[string]any{"text": 42, "label": "L"}, "L"}, // non-string skipped
		{map[string]any{"text": "", "label": "", "binding": ""}, ""},
	}
	for _, c := range cases {
		if got := intentText(c.in); got != c.want {
			t.Errorf("intentText(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func reportApp() *qrt.Runtime {
	app := &model.App{Name: "reporter", Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "root", Children: []*model.Node{
			{Type: "text", ID: "msg", Text: "Hello"},
			{Type: "button", ID: "ok_btn", Label: "OK"},
		}},
	}}
	return qrt.New(app)
}

// TestReportEnrichesMeasured verifies Report joins each measured row with its
// expressed intent (type, intent entry, intentText) and reports app + counts.
func TestReportEnrichesMeasured(t *testing.T) {
	rt := reportApp()
	rows := []map[string]any{
		{"id": "msg", "x": 1.0, "y": 2.0, "w": 30.0, "h": 12.0, "text": "measured glyph run"},
		{"id": "ok_btn"},
		{"id": "foreign"}, // no intent for this id
	}
	mb, _ := json.Marshal(rows)
	out, err := Report(rt, mb)
	if err != nil {
		t.Fatal(err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep["app"] != "reporter" {
		t.Errorf("app = %v, want reporter", rep["app"])
	}
	if rep["components"] != float64(3) {
		t.Errorf("components = %v, want 3", rep["components"])
	}
	measured := rep["measured"].([]any)
	msg := measured[0].(map[string]any)
	if msg["type"] != "text" {
		t.Errorf("msg type = %v, want text (from intent)", msg["type"])
	}
	if msg["intentText"] != "Hello" {
		t.Errorf("msg intentText = %v, want Hello (expressed text wins over measured)", msg["intentText"])
	}
	intent, ok := msg["intent"].(map[string]any)
	if !ok || intent["text"] != "Hello" {
		t.Errorf("msg intent = %v, want map with text Hello", msg["intent"])
	}
	btn := measured[1].(map[string]any)
	if btn["type"] != "button" || btn["intentText"] != "OK" {
		t.Errorf("ok_btn = type:%v intentText:%v, want button/OK", btn["type"], btn["intentText"])
	}
	foreign := measured[2].(map[string]any)
	if _, has := foreign["intent"]; has {
		t.Errorf("row without intent must not gain an intent key; got %v", foreign)
	}
	if _, has := foreign["type"]; has {
		t.Error("row without intent must keep no injected type")
	}
}

// TestReportEmptyAndInvalidMeasured verifies Report tolerates absent or
// malformed measurement payloads with a zero-component report, never an error.
func TestReportEmptyAndInvalidMeasured(t *testing.T) {
	rt := reportApp()
	for _, mb := range [][]byte{nil, {}, []byte("{garbage")} {
		out, err := Report(rt, mb)
		if err != nil {
			t.Fatalf("Report(%q): %v", mb, err)
		}
		var rep map[string]any
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("report JSON for input %q: %v", mb, err)
		}
		if rep["components"] != float64(0) {
			t.Errorf("components for input %q = %v, want 0", mb, rep["components"])
		}
	}
}

// TestJoinRowsSkipsEmptyID verifies rows without an id are excluded from the
// id->row join: they stay in the Report's measured listing but gain no intent
// and can never satisfy a check (an "" id check is "not rendered", not joined
// to them).
func TestJoinRowsSkipsEmptyID(t *testing.T) {
	rt := reportApp()
	rows := []map[string]any{
		{"x": 1.0},           // no id key at all
		{"id": "", "y": 2.0}, // explicit empty id
		{"id": "msg"},
	}
	out, err := Report(rt, mustMarshal(t, rows))
	if err != nil {
		t.Fatal(err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	measured := rep["measured"].([]any)
	if len(measured) != 3 {
		t.Fatalf("measured lists all rows; got %d, want 3", len(measured))
	}
	for i := 0; i < 2; i++ {
		row := measured[i].(map[string]any)
		if _, has := row["intent"]; has {
			t.Errorf("id-less row %d must not be joined with intent; got %v", i, row)
		}
	}
	if rep["components"] != float64(3) {
		t.Errorf("components = %v, want 3", rep["components"])
	}
	// a check for "" must be "not rendered", not accidentally joined to the
	// id-less rows
	checkRep := run(t, rt, rows, []map[string]any{{"id": "", "visible": true}})
	if checkRep["ok"] != false {
		t.Error("check for an empty id must fail as not rendered")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestReportNoScene verifies Report works when the app has no renderable
// scene: no intent is attached but the report is still well-formed.
func TestReportNoScene(t *testing.T) {
	rt := qrt.New(&model.App{Name: "bare", Entry: "nope", Scenes: map[string]*model.Node{}})
	mb, _ := json.Marshal([]map[string]any{{"id": "x"}})
	out, err := Report(rt, mb)
	if err != nil {
		t.Fatal(err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep["components"] != float64(1) || rep["app"] != "bare" {
		t.Errorf("rep = %v, want 1 component for app bare", rep)
	}
}
