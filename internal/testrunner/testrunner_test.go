package testrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if report.Tests != 5 {
		t.Fatalf("tests = %d, want 5", report.Tests)
	}
	if report.Passed != 1 || report.Failed != 4 {
		t.Fatalf("passed/failed = %d/%d, want 1/4 (fail.json, bad_selector.json, scene_missing.json, unknown_step.json)", report.Passed, report.Failed)
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
	if errStatus != 2 {
		t.Errorf("error-statused tests = %d, want 2 (scene_missing + unknown_step)", errStatus)
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
