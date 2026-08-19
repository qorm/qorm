package testrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

// fixture is the self-contained app under testdata/: a minimal counter with
// increment/set_status actions, a hidden node, and a tests/ directory with
// one passing, one failing, and two error-case documents.
const fixture = "testdata/app"

func TestRunPassingTest(t *testing.T) {
	// The pass.json document exercises the whole happy surface: set_state,
	// node-event and direct-action simulate_event, every assert type, and the
	// hidden-node visibility rule.
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "pass.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusPassed {
		t.Fatalf("status = %q, want %q (report: %+v)", report.Status, StatusPassed, report)
	}
	if report.Tests != 1 || report.Passed != 1 || report.Failed != 0 {
		t.Fatalf("tests/passed/failed = %d/%d/%d, want 1/1/0", report.Tests, report.Passed, report.Failed)
	}
	if r := report.Results[0]; r.Status != StatusPassed || len(r.Failures) != 0 {
		t.Errorf("result = %+v, want a clean pass", r)
	}
}

func TestRunDiscoverAll(t *testing.T) {
	// Default discovery runs every type:"test" document under the app. The
	// fixture's intentionally broken documents each fail THEIR OWN test
	// (failed asserts, or status error) and the suite is failed — but every
	// test still gets a result, so a suite with one broken doc shows the
	// other results instead of dying at load.
	report, err := Run(fixture, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Tests != 8 {
		t.Fatalf("tests = %d, want 8", report.Tests)
	}
	if report.Passed != 1 || report.Failed != 7 {
		t.Fatalf("passed/failed = %d/%d, want 1/7 (fail.json, bad_selector.json, scene_missing.json, unknown_step.json, onenter_error.json, onenter_chain_error.json, mount_step_error.json)", report.Passed, report.Failed)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", report.Status)
	}
	var errStatus int
	for _, r := range report.Results {
		if r.Status == StatusError {
			errStatus++
		}
	}
	if errStatus != 5 {
		t.Errorf("error-statused tests = %d, want 5 (scene_missing + unknown_step + onenter_error + onenter_chain_error + mount_step_error)", errStatus)
	}
}

func TestRunFailingTestReportsFailures(t *testing.T) {
	// The failing test is where exit-code-1 semantics start: only the fixture
	// fail.json runs, and the report must carry both failed assertions with
	// expected vs actual, mirroring what the CLI reports as exit 1.
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "fail.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want %q (failing asserts must fail the run)", report.Status, StatusFailed)
	}
	if report.Tests != 1 || report.Failed != 1 || report.Passed != 0 {
		t.Fatalf("tests/passed/failed = %d/%d/%d, want 1/0/1", report.Tests, report.Passed, report.Failed)
	}
	r := report.Results[0]
	if r.ID != "fixture_fail" {
		t.Errorf("result id = %q, want fixture_fail", r.ID)
	}
	if len(r.Failures) != 2 {
		t.Fatalf("failures = %d, want 2 (state_equals + text_equals)", len(r.Failures))
	}
	f := r.Failures[0]
	if f.Code != ErrAssertionFailed || f.Assert != "state_equals" || f.Target != "count" {
		t.Errorf("failure 0 = %+v, want test_assertion_failed on state_equals count", f)
	}
	if f.Expected != "99" || f.Actual != "1" {
		t.Errorf("failure 0 expected/actual = %q/%q, want 99/1 (the increment step ran once)", f.Expected, f.Actual)
	}
	f2 := r.Failures[1]
	if f2.Assert != "text_equals" || f2.Expected != "unexpected" || f2.Actual != "1" {
		t.Errorf("failure 1 = %+v, want text_equals on number with actual \"1\"", f2)
	}
}

func TestRunOnEnterScriptErrorFailsTest(t *testing.T) {
	// The boom scene's onEnter fires a script action that raises a qscript
	// runtime error. The error must not be swallowed: the test errors out and
	// the run fails (exit-1 semantics), with the script failure named on the
	// result — a scene whose enter hook crashes must never report green.
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "onenter_error.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (an onEnter script error must fail the run)", report.Status)
	}
	if report.Tests != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("tests/passed/failed = %d/%d/%d, want 1/0/1", report.Tests, report.Passed, report.Failed)
	}
	r := report.Results[0]
	if r.ID != "fixture_onenter_error" {
		t.Errorf("result id = %q, want fixture_onenter_error", r.ID)
	}
	if r.Status != StatusError {
		t.Fatalf("test status = %q, want error (the enter hook crashed)", r.Status)
	}
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], ErrRuntime) {
		t.Fatalf("errors = %v, want one test_runtime_error", r.Errors)
	}
	if !strings.Contains(r.Errors[0], "noSuchFn") || !strings.Contains(r.Errors[0], "onEnter") {
		t.Errorf("errors[0] = %q, want the onEnter script failure named (noSuchFn)", r.Errors[0])
	}
}

func TestRunOnEnterChainScriptErrorFailsTest(t *testing.T) {
	// Regression: the crash happens in the FIRST link of an enter chain
	// (chain's onEnter navigates to chainboom, then invokes a script that
	// writes state and crashes). The chain drains on: chainboom's CLEAN
	// onEnter dispatches afterwards and clears LastScriptError at its
	// boundary — without the runtime's EnterScriptError accumulator the
	// crash vanished and the run reported green. The mount must fail with
	// the script error named, and the pre-crash state write (count = 1)
	// proves the crashing script really executed.
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "onenter_chain_error.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (an enter-chain crash must fail the run)", report.Status)
	}
	if report.Tests != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("tests/passed/failed = %d/%d/%d, want 1/0/1", report.Tests, report.Passed, report.Failed)
	}
	r := report.Results[0]
	if r.Status != StatusError {
		t.Fatalf("test status = %q, want error (a chain link crashed)", r.Status)
	}
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], ErrRuntime) {
		t.Fatalf("errors = %v, want one test_runtime_error", r.Errors)
	}
	if !strings.Contains(r.Errors[0], "noSuchFn") || !strings.Contains(r.Errors[0], "onEnter") {
		t.Errorf("errors[0] = %q, want the chain crash named (noSuchFn)", r.Errors[0])
	}
}

func TestRunMountSceneStepFiresOnEnter(t *testing.T) {
	// Regression: a mount_scene STEP must fire the target's onEnter. Only the
	// document-level mount rode the pending flag New raised; later steps
	// drained nothing, so a step-mount into a scene whose onEnter crashes
	// reported green. The boom scene's onEnter crashes — the step must error
	// the test.
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "mount_step_error.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (the step-mounted scene's onEnter crashed)", report.Status)
	}
	r := report.Results[0]
	if r.Status != StatusError {
		t.Fatalf("test status = %q, want error", r.Status)
	}
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], ErrRuntime) || !strings.Contains(r.Errors[0], "noSuchFn") {
		t.Fatalf("errors = %v, want one test_runtime_error naming the crash (noSuchFn)", r.Errors)
	}
}

func TestRunErrorOnMissingScene(t *testing.T) {
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "scene_missing.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (errored tests fail the run)", report.Status)
	}
	r := report.Results[0]
	if r.Status != StatusError {
		t.Fatalf("test status = %q, want error (the scene never mounted)", r.Status)
	}
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], ErrSceneNotFound) {
		t.Errorf("errors = %v, want one test_scene_not_found error", r.Errors)
	}
}

func TestRunInvalidSelectorReported(t *testing.T) {
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "bad_selector.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if r.Status != StatusFailed {
		t.Fatalf("test status = %q, want failed", r.Status)
	}
	if len(r.Failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(r.Failures))
	}
	if f := r.Failures[0]; f.Code != ErrInvalidSelector {
		t.Errorf("failure = %+v, want query_invalid_selector for a selector with path", f)
	}
}

func TestRunUnknownStepErrors(t *testing.T) {
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "unknown_step.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if r.Status != StatusError {
		t.Fatalf("test status = %q, want error for advance_time (deferred from the MVP)", r.Status)
	}
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], ErrStepUnknown) {
		t.Errorf("errors = %v, want one test_step_unknown error", r.Errors)
	}
}

func TestRunNoTestsFound(t *testing.T) {
	// An app with no test documents must not report green: a run that tests
	// nothing silently passing would break CI expectations.
	dir := copyFixtureWithoutTests(t)
	_, err := Run(dir, nil)
	if err == nil {
		t.Fatalf("Run: want error, got nil (no tests must not pass)")
	}
	if !strings.Contains(err.Error(), ErrNoneFound) {
		t.Errorf("error = %v, want test_none_found", err)
	}
}

func TestRunNonTestDocRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "not_a_test.json")
	if err := os.WriteFile(p, []byte(`{"type": "scene", "id": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(fixture, []string{p})
	if err == nil || !strings.Contains(err.Error(), ErrDocInvalid) {
		t.Errorf("error = %v, want test_doc_invalid for a non-test document", err)
	}
}

func TestReportMatchesSpecJSONShape(t *testing.T) {
	report, err := Run(fixture, []string{filepath.Join(fixture, "tests", "pass.json")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The spec's minimal report keys must be present verbatim: status, tests,
	// passed, failed, diagnostics, hostCalls, durationMs.
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"status", "tests", "passed", "failed", "diagnostics", "hostCalls", "durationMs"} {
		if _, ok := m[key]; !ok {
			t.Errorf("report missing spec key %q: %s", key, data)
		}
	}
	if m["status"] != "passed" || m["failed"] != float64(0) {
		t.Errorf("report = %s, want passed/0", data)
	}
	if host := m["hostCalls"].([]any); len(host) != 0 {
		t.Errorf("hostCalls = %v, want empty in the MVP (no host-mock registry)", host)
	}
}

func TestMatchSelectorIdIsCaseInsensitive(t *testing.T) {
	nodes := []*matNode{{id: "scoreVal", typ: "text", text: "0"}}
	if got := matchSelector(nodes, map[string]any{"id": "scoreVal"}); len(got) != 1 {
		t.Fatalf("mixed-case selector: got %d matches, want 1", len(got))
	}
	if got := matchSelector(nodes, map[string]any{"id": "scoreval"}); len(got) != 1 {
		t.Fatalf("lower selector: got %d matches, want 1", len(got))
	}
}

func TestMaterializeExpandsListRenderItem(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"text": "Alpha"},
				map[string]any{"text": "Beta"},
			},
		}},
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root", Children: []*model.Node{
				{Type: "list", ID: "todo_list", Data: "{{state.items}}",
					Template: &model.Node{Type: "text", Text: "{{item.text}}"}},
			}},
		},
	}
	rt := qrt.New(app)
	var texts []string
	for _, n := range materialize(rt) {
		if n.text != "" {
			texts = append(texts, n.text)
		}
	}
	joined := strings.Join(texts, ",")
	if !strings.Contains(joined, "Alpha") || !strings.Contains(joined, "Beta") {
		t.Fatalf("list rows not materialized, texts=%v", texts)
	}
}

func TestMaterializeExpandsJSONComponents(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Components: map[string]*model.Node{
			"metric": {Type: "column", Children: []*model.Node{
				{Type: "text", Text: "{{prop.label}}"},
				{Type: "text", Text: "{{prop.value}}"},
			}},
			"panel": {Type: "column", Children: []*model.Node{
				{Type: "text", Text: "{{prop.title}}"},
				{Type: "slot"},
			}},
		},
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root", Children: []*model.Node{
				{Type: "metric", ID: "m1", Label: "Revenue", Value: "$12.4k"},
				{Type: "panel", ID: "acct", Props: map[string]any{"title": "Account"}, Children: []*model.Node{
					{Type: "text", Text: "Pro"},
				}},
			}},
		},
	}
	rt := qrt.New(app)
	var texts []string
	var ids []string
	for _, n := range materialize(rt) {
		if n.id != "" {
			ids = append(ids, n.id)
		}
		if n.text != "" {
			texts = append(texts, n.text)
		}
	}
	joined := strings.Join(texts, ",")
	for _, want := range []string{"Revenue", "$12.4k", "Account", "Pro"} {
		if !strings.Contains(joined, want) {
			t.Errorf("component template missing %q, texts=%v", want, texts)
		}
	}
	idJoin := strings.Join(ids, ",")
	if !strings.Contains(idJoin, "m1") || !strings.Contains(idJoin, "acct") {
		t.Errorf("instance ids lost, ids=%v", ids)
	}
}

func TestSimulateEventUsesListItemScope(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"id": "alpha"},
				map[string]any{"id": "beta"},
			},
			"picked": "",
		}},
		Actions: map[string]*model.Action{
			"mark": {ID: "mark", Steps: []model.Step{
				{Type: "state.set", Path: "picked", Value: "{{ id }}"},
			}},
		},
		Scenes: map[string]*model.Node{
			"main": {Type: "list", ID: "lst", Data: "{{state.items}}",
				Template: &model.Node{
					Type:    "button",
					Text:    "{{item.id}}",
					OnPress: &model.Invoke{Name: "mark", Args: map[string]string{"id": "{{item.id}}"}},
				}},
		},
	}
	rt := qrt.New(app)
	if err := simulateEvent(rt, Step{Target: map[string]any{"text": "beta"}, Event: "press"}); err != nil {
		t.Fatalf("press beta: %v", err)
	}
	if got := qrt.Stringify(rt.State["picked"]); got != "beta" {
		t.Fatalf("picked = %q, want beta (item scope must evaluate {{item.id}})", got)
	}
}

func TestSimulateEventUsesListAlias(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"id": "alpha"},
				map[string]any{"id": "beta"},
			},
			"picked": "",
		}},
		Actions: map[string]*model.Action{
			"mark": {ID: "mark", Steps: []model.Step{
				{Type: "state.set", Path: "picked", Value: "{{ id }}"},
			}},
		},
		Scenes: map[string]*model.Node{
			"main": {Type: "list", ID: "lst", Data: "{{state.items}}",
				Props: map[string]any{"as": "line"},
				Template: &model.Node{
					Type:    "button",
					ID:      "gift_{{line.id}}",
					Text:    "{{line.id}}",
					OnPress: &model.Invoke{Name: "mark", Args: map[string]string{"id": "{{line.id}}"}},
				}},
		},
	}
	rt := qrt.New(app)
	if err := simulateEvent(rt, Step{Target: map[string]any{"id": "gift_beta"}, Event: "press"}); err != nil {
		t.Fatalf("press gift_beta: %v", err)
	}
	if got := qrt.Stringify(rt.State["picked"]); got != "beta" {
		t.Fatalf("picked = %q, want beta (as:line must evaluate {{line.id}})", got)
	}
}

func TestRunRefusesErrorDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(`{"type":"app","id":"broken","entry":"main"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	scene := `{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"hi"},"onEnter":"missing"}`
	if err := os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(dir, nil)
	if err == nil {
		t.Fatal("Run must refuse an app with error-level loader diagnostics")
	}
	if !strings.Contains(err.Error(), ErrLoad) {
		t.Errorf("error = %v, want %s", err, ErrLoad)
	}
	if !strings.Contains(err.Error(), "onEnter") {
		t.Errorf("error = %v, want to name the dangling onEnter", err)
	}
}

func TestStateEqualsComputedPath(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"done": true},
				map[string]any{"done": false},
			},
		}},
		Computed: map[string]string{
			"openCount": `{{ len(filter(state.items, "it.done == false")) }}`,
		},
		Actions: map[string]*model.Action{
			"noop": {ID: "noop"},
		},
		Scenes: map[string]*model.Node{
			"main": {Type: "text", ID: "t", Text: "hi"},
		},
	}
	rt := qrt.New(app)
	if f := evalAssert(rt, Assert{Type: "state_equals", Path: "computed.openCount", Value: 1}); f != nil {
		t.Fatalf("initial openCount: %+v", f)
	}
	rt.SetStatePath("items", []any{
		map[string]any{"done": true},
		map[string]any{"done": true},
	})
	rt.Dispatch("noop", nil) // republish computed at the dispatch boundary
	if f := evalAssert(rt, Assert{Type: "state_equals", Path: "computed.openCount", Value: 0}); f != nil {
		t.Fatalf("after all done: %+v", f)
	}
}

func TestRunExampleAppsWithTests(t *testing.T) {
	for _, dir := range []string{
		"../../examples/counter",
		"../../examples/todo",
		"../../examples/derived",
		"../../examples/uikit",
		"../../examples/tetris",
	} {
		report, err := Run(dir, nil)
		if err != nil {
			t.Errorf("%s: Run: %v", dir, err)
			continue
		}
		if report.Status != StatusPassed {
			t.Errorf("%s: status=%s passed=%d failed=%d results=%+v", dir, report.Status, report.Passed, report.Failed, report.Results)
		}
	}
}

// copyFixtureWithoutTests builds a standalone copy of the fixture app with its
// tests/ directory removed, so discovery yields no documents.
func copyFixtureWithoutTests(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := copyTree(fixture, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "tests")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			if err := copyTree(from, to); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
