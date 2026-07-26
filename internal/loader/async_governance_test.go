package loader

import (
	"strings"
	"testing"
)

// Load-time tests for the async governance vocabulary: `key`, `timeout`,
// `pending` on an http step, the `delay` step, and the diagnostic that catches
// a loading state which can never be seen.
//
// The pattern throughout: a field that WORKS must reach the model, and a field
// that CANNOT work where it was written must produce a diagnostic rather than
// sit there looking effective. A silently inert `key` is the worst outcome —
// the app looks deduplicated and is not.

// TestLoadAsyncGovernanceFields: all four new keys parse onto the step, with
// the numeric ones read as whole milliseconds.
func TestLoadAsyncGovernanceFields(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "search",
		"steps": []any{
			map[string]any{"type": "http.get", "url": "https://example.com",
				"async": true, "key": "search", "timeout": float64(2500),
				"pending": "searching", "result": "hits"},
			map[string]any{"type": "delay", "ms": float64(400)},
			map[string]any{"type": "state.set", "path": "count", "value": "1"},
		},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("the governance fields must load clean: %v", app.Diagnostics)
	}
	steps := app.Actions["search"].Steps
	if steps[0].Key != "search" {
		t.Errorf("key: %q", steps[0].Key)
	}
	if steps[0].TimeoutMS != 2500 {
		t.Errorf("timeout: %d", steps[0].TimeoutMS)
	}
	if steps[0].Pending != "searching" {
		t.Errorf("pending: %q", steps[0].Pending)
	}
	if steps[1].DelayMS != 400 {
		t.Errorf("delay ms: %d", steps[1].DelayMS)
	}
	// The defaults must stay zero, so an app written before these existed keeps
	// the client ceiling and no slot.
	if steps[2].Key != "" || steps[2].TimeoutMS != 0 || steps[2].Pending != "" || steps[2].DelayMS != 0 {
		t.Errorf("an ordinary step must carry no governance fields: %+v", steps[2])
	}
}

// TestGovernanceFieldsOnNonHTTPStepsWarn: each of the three http-only fields is
// inert anywhere else. An author who wrote one there believes their request is
// cancellable / bounded / observable; nothing is listening, so say so.
func TestGovernanceFieldsOnNonHTTPStepsWarn(t *testing.T) {
	for _, tc := range []struct {
		field string
		step  map[string]any
		frag  string
	}{
		{"key", map[string]any{"type": "state.set", "path": "count", "value": "1", "key": "k"}, `支持 "key"`},
		{"pending", map[string]any{"type": "state.set", "path": "count", "value": "1", "pending": "busy"}, `支持 "pending"`},
		{"timeout", map[string]any{"type": "state.set", "path": "count", "value": "1", "timeout": float64(500)}, `支持 "timeout"`},
		{"ms", map[string]any{"type": "state.set", "path": "count", "value": "1", "ms": float64(500)}, `支持 "ms"`},
	} {
		t.Run(tc.field, func(t *testing.T) {
			app := FromDocs(execDocs(map[string]any{
				"type": "action", "id": "bad", "steps": []any{tc.step},
			}))
			if !hasDiag(app.Diagnostics, tc.frag) {
				t.Errorf("%q on a non-http step must warn: %v", tc.field, app.Diagnostics)
			}
			for _, d := range app.Diagnostics {
				if strings.HasPrefix(d, "error:") {
					t.Errorf("it is a warning, not an error — the step still runs: %v", d)
				}
			}
		})
	}
}

// TestNonPositiveTimeoutWarns: a timeout of 0 (or a quoted "5000", which reads
// as 0) does not shorten anything — it silently leaves the request on the 20s
// ceiling the author was trying to escape, which is the opposite of what they
// wrote.
func TestNonPositiveTimeoutWarns(t *testing.T) {
	for _, v := range []any{float64(0), float64(-1), "5000", true} {
		app := FromDocs(execDocs(map[string]any{
			"type": "action", "id": "slow",
			"steps": []any{map[string]any{"type": "http.get", "url": "https://example.com", "timeout": v}},
		}))
		if !hasDiag(app.Diagnostics, "timeout 必须是正数毫秒") {
			t.Errorf("timeout %v must warn: %v", v, app.Diagnostics)
		}
	}
}

// TestDelayWithoutMsIsAnError: a delay that declares no wait is not a shorter
// pause, it is none — the steps after it run in the same dispatch, so the JSON
// promises pacing it will never deliver.
func TestDelayWithoutMsIsAnError(t *testing.T) {
	missing := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "pace", "steps": []any{map[string]any{"type": "delay"}},
	}))
	if !hasDiag(missing.Diagnostics, "'delay' 步骤缺少 ms") {
		t.Errorf("a delay with no ms must be reported: %v", missing.Diagnostics)
	}
	for _, v := range []any{float64(0), float64(-5), "200"} {
		app := FromDocs(execDocs(map[string]any{
			"type": "action", "id": "pace",
			"steps": []any{map[string]any{"type": "delay", "ms": v}},
		}))
		if !hasDiag(app.Diagnostics, "ms 必须是正数") {
			t.Errorf("delay ms %v must be reported: %v", v, app.Diagnostics)
		}
	}
}

// TestPendingIntoComputedIsAnError: `pending` is a state write, so it obeys the
// rule every other write obeys — the derived namespace is republished each
// frame and writing into it is dropped at dispatch.
func TestPendingIntoComputedIsAnError(t *testing.T) {
	docs := execDocs(map[string]any{
		"type": "action", "id": "fetch",
		"steps": []any{map[string]any{"type": "http.get", "url": "https://example.com", "pending": "computed.busy"}},
	})
	docs[0]["computed"] = map[string]any{"busy": "{{ false }}"}
	app := FromDocs(docs)
	if !hasDiag(app.Diagnostics, "写入了派生值路径") {
		t.Errorf("a pending path into the computed namespace must be an error: %v", app.Diagnostics)
	}
}

// ---- the invisible-loading-state lint -----------------------------------------

// loadingDocs is the classic mistake: raise a flag, call the backend
// synchronously, lower the flag — all inside one dispatch, which renders one
// frame at its boundary, so the flag is never on screen.
func loadingDocs(httpStep map[string]any, before ...map[string]any) []map[string]any {
	steps := []any{map[string]any{"type": "state.set", "path": "busy", "value": "{{ true }}"}}
	for _, b := range before {
		steps = append(steps, b)
	}
	steps = append(steps, httpStep,
		map[string]any{"type": "state.set", "path": "busy", "value": "{{ false }}"})
	return execDocs(map[string]any{"type": "action", "id": "save", "steps": steps})
}

const invisibleLoadingFrag = "该状态永远不会被渲染出来"

// TestInvisibleLoadingStateWarns is the lint's reason to exist: the JSON reads
// perfectly and does nothing, and no test of the app can catch it because the
// FINAL state is correct — only the frames in between are missing.
func TestInvisibleLoadingStateWarns(t *testing.T) {
	app := FromDocs(loadingDocs(map[string]any{"type": "http.put", "url": "https://example.com"}))
	if !hasDiag(app.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("a loading flag around a synchronous request must warn: %v", app.Diagnostics)
	}
	// It is advice, not a refusal: the action still runs.
	for _, d := range app.Diagnostics {
		if strings.HasPrefix(d, "error:") {
			t.Errorf("the lint must be a warning: %v", d)
		}
	}
}

// TestInvisibleLoadingCuresAreAccepted pins the three ways out the message
// names. Each must silence the lint, because each genuinely fixes it.
func TestInvisibleLoadingCuresAreAccepted(t *testing.T) {
	cures := []struct {
		name string
		docs []map[string]any
	}{
		{"render step", loadingDocs(
			map[string]any{"type": "http.put", "url": "https://example.com"},
			map[string]any{"type": "render"})},
		{"async", loadingDocs(
			map[string]any{"type": "http.put", "url": "https://example.com", "async": true})},
		{"pending", loadingDocs(
			map[string]any{"type": "http.put", "url": "https://example.com", "pending": "busy"})},
	}
	for _, c := range cures {
		t.Run(c.name, func(t *testing.T) {
			app := FromDocs(c.docs)
			if hasDiag(app.Diagnostics, invisibleLoadingFrag) {
				t.Errorf("%s must silence the lint: %v", c.name, app.Diagnostics)
			}
		})
	}
}

// TestInvisibleLoadingLintIsNarrow: the diagnostic fires on the full shape —
// flag raised, request, flag lowered — and nothing looser. A boolean that is
// never reset is not a loading flag (it is a "has loaded once" marker), and a
// value copied from state is data, not a flag; reporting either would train
// authors to ignore the warning.
func TestInvisibleLoadingLintIsNarrow(t *testing.T) {
	neverReset := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save", "steps": []any{
			map[string]any{"type": "state.set", "path": "touched", "value": "{{ true }}"},
			map[string]any{"type": "http.put", "url": "https://example.com"},
		},
	}))
	if hasDiag(neverReset.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("a flag that is never lowered is not a loading flag: %v", neverReset.Diagnostics)
	}

	notALiteral := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save", "steps": []any{
			map[string]any{"type": "state.set", "path": "busy", "value": "{{ state.count > 0 }}"},
			map[string]any{"type": "http.put", "url": "https://example.com"},
			map[string]any{"type": "state.set", "path": "busy", "value": "{{ false }}"},
		},
	}))
	if hasDiag(notALiteral.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("a computed value is data being copied, not a flag being raised: %v", notALiteral.Diagnostics)
	}

	noRequest := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save", "steps": []any{
			map[string]any{"type": "state.set", "path": "busy", "value": "{{ true }}"},
			map[string]any{"type": "state.set", "path": "busy", "value": "{{ false }}"},
		},
	}))
	if hasDiag(noRequest.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("without a request there is nothing slow to hide: %v", noRequest.Diagnostics)
	}
}

// TestInvisibleLoadingLintSeesNestedLists: a branch is its own step list, and
// the mistake is just as invisible inside one — an action that fetches only
// when a cache is cold is exactly where it hides.
func TestInvisibleLoadingLintSeesNestedLists(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save", "steps": []any{
			map[string]any{"type": "if", "condition": "{{ state.count == 0 }}", "then": []any{
				map[string]any{"type": "state.set", "path": "busy", "value": "{{ true }}"},
				map[string]any{"type": "http.get", "url": "https://example.com"},
				map[string]any{"type": "state.set", "path": "busy", "value": "{{ false }}"},
			}},
		},
	}))
	if !hasDiag(app.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("the lint must descend into branches: %v", app.Diagnostics)
	}
}

// TestInvisibleLoadingAcceptsAResetInTheResultBranch: lowering the flag in
// onSuccess/onError is the other half of the same shape, so it must be
// recognised — otherwise the lint would miss the very apps most likely to have
// the bug.
func TestInvisibleLoadingAcceptsAResetInTheResultBranch(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save", "steps": []any{
			map[string]any{"type": "state.set", "path": "busy", "value": "{{ true }}"},
			map[string]any{"type": "http.get", "url": "https://example.com",
				"onSuccess": []any{map[string]any{"type": "state.clear", "path": "busy"}}},
		},
	}))
	if !hasDiag(app.Diagnostics, invisibleLoadingFrag) {
		t.Errorf("a reset inside the result branch completes the shape: %v", app.Diagnostics)
	}
}
