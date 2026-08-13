package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/testrunner"
)

// TestCLITestRunner drives `qorm test` end to end through the real binary:
// the counter app's tests/*.json documents are discovered, executed headless,
// and the spec's minimal JSON report is printed to stdout with exit 0. The
// stdout payload must be pure JSON (nothing else), so `qorm test | jq` style
// consumers stay valid.
func TestCLITestRunner(t *testing.T) {
	bin := buildQORMBinary(t)
	counter := filepath.Join("..", "..", "examples", "counter")

	out, errOut, code := runQORM(t, bin, nil, "test", counter)
	if code != 0 {
		t.Fatalf("qorm test examples/counter: exit = %d, want 0\nstderr: %s", code, errOut)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected the JSON report on stdout, got nothing")
	}
	var rep testrunner.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("stdout is not the spec's JSON report: %v\n%s", err, out)
	}
	if rep.Status != testrunner.StatusPassed {
		t.Errorf("status = %q, want %q", rep.Status, testrunner.StatusPassed)
	}
	if rep.Tests == 0 || rep.Passed != rep.Tests || rep.Failed != 0 {
		t.Errorf("tests/passed/failed = %d/%d/%d, want all passing", rep.Tests, rep.Passed, rep.Failed)
	}
	// The counter app ships four test documents; each result names the file it
	// came from (the loader's provenance stamp), so reports are auditable.
	for _, r := range rep.Results {
		if r.Status != testrunner.StatusPassed {
			t.Errorf("test %s: status = %q, want passed", r.ID, r.Status)
		}
		if !strings.HasPrefix(r.File, "tests/") {
			t.Errorf("test %s: file = %q, want a tests/ provenance path", r.ID, r.File)
		}
	}
}

// TestCLITestRunnerExplicitFile runs a single designated test document and
// checks the runner honours the explicit-file spelling (`qorm test path.json`).
func TestCLITestRunnerExplicitFile(t *testing.T) {
	bin := buildQORMBinary(t)
	file := filepath.Join("..", "..", "examples", "counter", "tests", "increment.json")

	out, _, code := runQORM(t, bin, nil, "test", file)
	if code != 0 {
		t.Fatalf("qorm test <file>: exit = %d, want 0", code)
	}
	var rep testrunner.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if rep.Tests != 1 || rep.Results[0].ID != "counter_increment" {
		t.Errorf("report = %+v, want the single counter_increment test", rep)
	}
}
