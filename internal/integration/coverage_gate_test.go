// Coverage gate (GUARDRAIL 2): key packages must never silently lose test
// coverage. TestCoverageGate is an IN-REPO floor — because it runs under a
// plain `go test ./...`, it gates ci.yml / release.yml / the release.sh
// preflight and verify.sh for free, with no workflow file to drift and no
// third-party action to trust. The single source of truth is the
// coverageFloors table below.
//
// THE INVARIANT: for every package in coverageFloors, the statement coverage
// reported by `go test -cover <pkg>` stays at or above its floor. Floors are
// whole numbers set 2-3 points below the coverage observed when the floor was
// written (coverage wobbles by about a line between runs), so ordinary churn
// does not trip the gate, but a real regression — a new untested code path,
// a deleted test, a build tag silently excluding a test file — does.
//
// HOW IT WORKS: the gate never runs `go test ./...` and never globs patterns;
// it iterates the EXPLICIT package list in coverageFloors and, for each,
// spawns a cached `go test -cover -timeout 300s <pkg>` child with
// QORM_COVERAGE_GATE_INNER=1 in its environment, then parses the
// `coverage: X.Y% of statements` line with a single regexp. Output with no
// such line — `[no test files]`, `[no statements]`, a failing child, an empty
// run — is an ERROR, never a silent pass: a package can only leave the gate
// by being removed from the table with a reason, not by ceasing to report a
// number. The build cache keeps the inner runs fast; each child is bounded by
// its own -timeout and a context deadline, so a wedged child cannot hang the
// gate, and the recursion guard stops the gate ever measuring itself.
//
// EXEMPT — packages the gate deliberately does NOT measure (reason mandatory,
// enforced disjoint from coverageFloors):
//
//	github.com/qorm/qorm/cmd/qorm              server main loop + native packaging
//	                                           (codesign/notarize/gradle); exercised
//	                                           by release tooling, hard to unit-test
//	github.com/qorm/qorm/internal/integration  no statements: test-only package
//	                                           (every file is *_test.go), so
//	                                           `go test -cover` reports no percentage
//	github.com/qorm/qorm/internal/webview/...  vendored cgo bindings, excluded
//	                                           from the pure-Go default build
//	github.com/qorm/qorm/cmd/qorm-wasm         build-constrained to js/wasm;
//	                                           excluded from the default
//	                                           GOOS/GOARCH build
//
// HOW TO SATISFY / RATCHET RULE:
//
//  1. If the gate fails on a package you touched: add tests for the new code
//     until coverage is back above the floor. That is the expected fix.
//  2. RATCHET: when coverage rises MATERIALLY and durably (new tests merged),
//     RAISE the floor in coverageFloors in the SAME PR, keeping 2-3 points of
//     headroom, and refresh its trailing "observed" comment. The floor only
//     ever goes up.
//  3. A floor may be lowered (or an entry removed) ONLY with a reason comment
//     explaining why the covered code is gone for good — never to make a
//     failing build green.
//  4. Onboarding a new key package: add it to coverageFloors with its current
//     coverage minus 2-3 points as the starting floor.
//
// SELF-PROOF: TestCoverageGateFailsOnImpossibleFloor drives the SAME
// parseCoverage + checkFloor helpers the real gate uses against a synthetic
// 50.0% line and a floor of 99 and asserts the comparison ERRORS — proving
// the gate can actually block a regression, not merely run. A gate whose
// comparison can never fire would pass every real run and guard nothing.
package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// coverageGateInnerEnv marks the child `go test` runs the gate itself spawns.
// When it is set, TestCoverageGate skips, so the gate can never recurse into
// itself — even if internal/integration were ever added to coverageFloors.
const coverageGateInnerEnv = "QORM_COVERAGE_GATE_INNER"

// coverageFloors is the gate: package import path -> minimum statement
// coverage, in whole percent. Each floor sits 2-3 points below the coverage
// observed when it was written (trailing comment). See the file-top doc for
// the ratchet rule.
var coverageFloors = map[string]float64{
	"github.com/qorm/qorm/internal/keys":       97, // observed 100.0%
	"github.com/qorm/qorm/internal/expr":       97, // observed 100.0%
	"github.com/qorm/qorm/internal/loader":     97, // observed 100.0%
	"github.com/qorm/qorm/internal/bundle":     97, // observed 100.0%
	"github.com/qorm/qorm/internal/measure":    97, // observed 100.0%
	"github.com/qorm/qorm/internal/render":     96, // observed 99.7%
	"github.com/qorm/qorm/internal/runtime":    96, // observed 99.5%
	"github.com/qorm/qorm/internal/mcp":        96, // observed 99.4%
	"github.com/qorm/qorm/internal/server":     95, // observed 98.4%
	"github.com/qorm/qorm/internal/miniapp":    93, // observed 96.6%
	"github.com/qorm/qorm/internal/support":    90, // observed 94.0%
	"github.com/qorm/qorm/internal/ota":        90, // observed 93.1%
	"github.com/qorm/qorm/internal/model":      97, // observed 100.0%
	"github.com/qorm/qorm/internal/mdsite":     89, // observed 92.3%
	"github.com/qorm/qorm/pkg/qormext":         95, // observed 100.0%
	"github.com/qorm/qorm/internal/updates":    84, // observed 87.8%
	"github.com/qorm/qorm/internal/sourcemap":  84, // observed 87.7%
	"github.com/qorm/qorm/internal/capability": 82, // observed 84.9%
	"github.com/qorm/qorm/internal/a11y":       70, // observed 73.3%
}

// coverageExempt lists packages the gate deliberately does NOT measure. Every
// entry carries a mandatory reason, and TestCoverageGate asserts the set is
// disjoint from coverageFloors — a package is floored or exempt, never both.
var coverageExempt = map[string]string{
	"github.com/qorm/qorm/cmd/qorm":             "server main loop + native packaging (codesign/notarize/gradle); exercised by release tooling, hard to unit-test",
	"github.com/qorm/qorm/internal/integration": "no statements: test-only package (every file is *_test.go), so `go test -cover` reports no percentage",
	"github.com/qorm/qorm/internal/webview/...": "vendored cgo bindings, excluded from the pure-Go default build",
	"github.com/qorm/qorm/cmd/qorm-wasm":        "build-constrained to js/wasm; excluded from the default GOOS/GOARCH build",
}

// covLineRe extracts the percentage from the `go test -cover` summary line
// `coverage: 99.7% of statements`. Only the decimal part is optional.
var covLineRe = regexp.MustCompile(`coverage: ([0-9]+(?:\.[0-9]+)?)% of statements`)

// parseCoverage reads the statement-coverage percentage out of a child
// `go test -cover` run's combined output. ok=false (with a reason) whenever
// there is no trustworthy number — `[no test files]`, `[no statements]`, a
// failing child, a percentage outside [0,100] — and callers must fail the gate
// there: absence of a number is never a pass. Factored out so the real gate
// and the self-proof exercise identical code.
func parseCoverage(output string) (pct float64, ok bool, reason string) {
	m := covLineRe.FindStringSubmatch(output)
	if m == nil {
		switch {
		case strings.Contains(output, "[no test files]"):
			return 0, false, "package has no test files"
		case strings.Contains(output, "[no statements]"):
			return 0, false, "package has no statements"
		default:
			return 0, false, "no `coverage: X.Y% of statements` line (child test failure, or parser drift)"
		}
	}
	pct, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false, "unparseable coverage percentage " + strconv.Quote(m[1])
	}
	if pct < 0 || pct > 100 {
		return 0, false, fmt.Sprintf("implausible coverage %.1f%% outside [0,100] — parser drift?", pct)
	}
	return pct, true, ""
}

// checkFloor is the whole gate reduced to one comparison: parsed coverage must
// be at or above the floor. Factored out so the real gate and the self-proof
// in TestCoverageGateFailsOnImpossibleFloor exercise identical code.
func checkFloor(pkg string, pct, floor float64) error {
	if pct < floor {
		return fmt.Errorf("%s: coverage %.1f%% is below the floor %.0f%% — add tests for the new code; "+
			"never lower a floor without a reason comment (see coverage_gate_test.go header)", pkg, pct, floor)
	}
	return nil
}

// runCoverageGateChild runs one cached `go test -cover <pkg>` with the
// recursion-guard env set and returns its combined output. The context puts a
// hard wall-clock bound on top of the inner -timeout, so a wedged child can
// never hang the gate.
func runCoverageGateChild(ctx context.Context, pkg string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-cover", "-timeout", "300s", pkg)
	cmd.Env = append(os.Environ(), coverageGateInnerEnv+"=1")
	return cmd.CombinedOutput()
}

// tailOf returns the last n bytes of b, prefixed by a truncation marker, so a
// failing child's verbose output does not flood the test log.
func tailOf(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append([]byte("...(truncated)...\n"), b[len(b)-n:]...)
}

// TestCoverageGate enforces the coverageFloors table: every listed package
// must report `go test -cover` statement coverage at or above its floor. It
// measures only the explicit package list (never ./... or a glob), and output
// with no parseable percentage fails loudly rather than passing silently.
func TestCoverageGate(t *testing.T) {
	if testing.Short() {
		t.Skip("the coverage gate spawns real `go test -cover` child processes; skipped in -short mode")
	}
	if os.Getenv(coverageGateInnerEnv) != "" {
		t.Skip("recursion guard: this run is a child of the coverage gate itself")
	}

	// A package is floored or exempt-with-reason, never both.
	for pkg := range coverageFloors {
		if reason, ok := coverageExempt[pkg]; ok {
			t.Errorf("%s is both floored and exempt (%s) — pick one", pkg, reason)
		}
	}

	pkgs := make([]string, 0, len(coverageFloors))
	for p := range coverageFloors {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		floor := coverageFloors[pkg]
		ctx, cancel := context.WithTimeout(context.Background(), 330*time.Second)
		out, runErr := runCoverageGateChild(ctx, pkg)
		cancel()

		pct, ok, why := parseCoverage(string(out))
		if !ok {
			t.Errorf("%s: cannot gate on coverage: %s (run error: %v)\n--- child output ---\n%s",
				pkg, why, runErr, tailOf(out, 4000))
			continue
		}
		if err := checkFloor(pkg, pct, floor); err != nil {
			t.Error(err)
		} else {
			t.Logf("%s: %.1f%% (floor %.0f%%)", pkg, pct, floor)
		}
	}
}

// TestCoverageGateFailsOnImpossibleFloor is the gate's self-proof: it drives
// the SAME parseCoverage + checkFloor pair the real gate uses on a synthetic
// 50.0% line against an impossible floor of 99 and asserts the comparison
// ERRORS — with no `go test` child involved — proving the gate genuinely
// blocks a below-floor package. (Same pattern as the planted snippets in
// TestAttrInjectionLint.)
func TestCoverageGateFailsOnImpossibleFloor(t *testing.T) {
	const synthetic = "ok  github.com/qorm/qorm/internal/synthetic  0.001s  coverage: 50.0% of statements"

	pct, ok, why := parseCoverage(synthetic)
	if !ok {
		t.Fatalf("parseCoverage rejected a well-formed coverage line: %s", why)
	}
	if pct != 50.0 {
		t.Fatalf("parseCoverage read %.1f, want 50.0", pct)
	}

	if err := checkFloor("github.com/qorm/qorm/internal/synthetic", pct, 99); err == nil {
		t.Errorf("self-proof failed: 50.0%% against a floor of 99%% did NOT error — the coverage gate can never block a regression")
	}
	// The comparison must be exact in the other direction too: at-floor and
	// above-floor pass, so the gate is not merely always-failing.
	if err := checkFloor("github.com/qorm/qorm/internal/synthetic", pct, 50); err != nil {
		t.Errorf("self-proof failed: coverage exactly at the floor errored: %v", err)
	}
	if err := checkFloor("github.com/qorm/qorm/internal/synthetic", pct, 49); err != nil {
		t.Errorf("self-proof failed: coverage above the floor errored: %v", err)
	}

	// Output with no measurable number must be a parse failure, never a silent
	// pass — the gate must not go quiet when a package stops reporting.
	for _, silent := range []string{
		"ok  github.com/qorm/qorm/internal/synthetic  [no test files]",
		"ok  github.com/qorm/qorm/internal/synthetic  coverage: [no statements]",
		"--- FAIL: TestX\nFAIL",
		"",
	} {
		if _, ok, _ := parseCoverage(silent); ok {
			t.Errorf("self-proof failed: parseCoverage accepted output with no coverage percentage: %q", silent)
		}
	}
}
