package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Tests for the `render` step — the frame primitive. It asks the HOST to
// publish the state written so far as an intermediate frame, so a loading flag
// set before a slow step is actually painted. The whole feature hangs off one
// nil-able hook (Runtime.Commit) that runtime.New never installs, which is what
// keeps a bare runtime — and every simulation Clone — byte-for-byte as
// synchronous and deterministic as it was before the step existed.

// renderApp builds an app whose action writes a flag, renders, does more work
// and clears the flag: the canonical "submit spinner" shape.
func renderApp(steps ...model.Step) *model.App {
	return &model.App{
		Entry:   "main",
		Scenes:  map[string]*model.Node{"main": {Type: "view", ID: "root"}, "detail": {Type: "view", ID: "droot"}},
		Actions: map[string]*model.Action{"go": {ID: "go", Steps: steps}},
	}
}

// TestRenderStepCallsCommit: the step calls the host's frame sink at exactly
// the point it appears in the list, and the sink observes the state written by
// the steps BEFORE it (not the end state) — that is the whole point.
func TestRenderStepCallsCommit(t *testing.T) {
	rt := New(renderApp(
		model.Step{Type: "state.set", Path: "saving", Value: "{{ true }}"},
		model.Step{Type: "render"},
		model.Step{Type: "state.set", Path: "saving", Value: "{{ false }}"},
	))
	var frames []any
	rt.Commit = func() { frames = append(frames, rt.State["saving"]) }
	rt.Dispatch("go", nil)
	if len(frames) != 1 {
		t.Fatalf("one render step must commit exactly one frame: got %d", len(frames))
	}
	if frames[0] != true {
		t.Errorf("the intermediate frame must see the loading state: got %v", frames[0])
	}
	if rt.State["saving"] != false {
		t.Errorf("the steps after render still run: saving=%v", rt.State["saving"])
	}
}

// TestRenderStepNoHostIsNoOp: with no host sink the step is inert — no panic,
// and every sibling step still runs. This is the fallback that lets the same
// JSON run unchanged on a host that has not opted in (and is why the WASM build,
// `qorm render`, miniapp export and MCP simulate need no changes at all).
func TestRenderStepNoHostIsNoOp(t *testing.T) {
	rt := New(renderApp(
		model.Step{Type: "render"},
		model.Step{Type: "state.increment", Path: "n"},
		model.Step{Type: "render"},
		model.Step{Type: "state.increment", Path: "n"},
	))
	rt.Dispatch("go", nil)
	if rt.State["n"] != float64(2) {
		t.Errorf("a hookless render must not interrupt the dispatch: n=%v", rt.State["n"])
	}
}

// TestRenderStepFrameCap: a looping action cannot flood live-sync — frames are
// capped per top-level interaction, and the steps keep running past the cap.
func TestRenderStepFrameCap(t *testing.T) {
	steps := make([]model.Step, 0, MaxFrames+1)
	for i := 0; i < MaxFrames+1; i++ {
		steps = append(steps, model.Step{Type: "render"})
	}
	steps = append(steps, model.Step{Type: "state.set", Path: "done", Value: "{{ true }}"})
	rt := New(renderApp(steps...))
	n := 0
	rt.Commit = func() { n++ }
	rt.Dispatch("go", nil)
	if n != MaxFrames {
		t.Errorf("frames must be capped at %d per dispatch: got %d", MaxFrames, n)
	}
	if rt.State["done"] != true {
		t.Error("steps after the cap is reached must still run")
	}
}

// TestRenderStepBudgetIsPerTopLevelDispatch: nested invokes share the outer
// dispatch's allowance (so recursion cannot reset it), and a NEW top-level
// dispatch gets a fresh one.
func TestRenderStepBudgetIsPerTopLevelDispatch(t *testing.T) {
	app := renderApp(
		model.Step{Type: "render"},
		model.Step{Type: "invoke", Name: "inner"},
	)
	inner := make([]model.Step, 0, MaxFrames)
	for i := 0; i < MaxFrames; i++ {
		inner = append(inner, model.Step{Type: "render"})
	}
	app.Actions["inner"] = &model.Action{ID: "inner", Steps: inner}
	rt := New(app)
	n := 0
	rt.Commit = func() { n++ }
	rt.Dispatch("go", nil)
	if n != MaxFrames {
		t.Errorf("a nested invoke shares the caller's frame budget: got %d, want %d", n, MaxFrames)
	}
	rt.Dispatch("go", nil)
	if n != 2*MaxFrames {
		t.Errorf("a new top-level dispatch resets the budget: got %d, want %d", n, 2*MaxFrames)
	}
}

// TestRenderStepInsideBranches: `render` is a normal step, so it works wherever
// steps do — inside an `if` branch and inside an http result branch.
func TestRenderStepInsideBranches(t *testing.T) {
	app := renderApp(model.Step{
		Type:      "if",
		Condition: "{{ true }}",
		Then:      []model.Step{{Type: "render"}},
		Else:      []model.Step{{Type: "render"}, {Type: "render"}},
	})
	rt := New(app)
	n := 0
	rt.Commit = func() { n++ }
	rt.Dispatch("go", nil)
	if n != 1 {
		t.Errorf("only the taken branch's render commits: got %d", n)
	}
}

// TestNewRuntimeHasNoHostHooks pins the invariant the determinism guard rests
// on: runtime.New installs NO host hooks. TestAllExamplesRenderDeterministically
// renders New(app) twice and compares bytes; if New ever wired a frame sink, a
// `render` step could publish (and mutate host state) from a pure render path.
func TestNewRuntimeHasNoHostHooks(t *testing.T) {
	rt := New(renderApp(model.Step{Type: "render"}))
	if rt.Commit != nil {
		t.Error("runtime.New must not install a host frame sink — hosts install it explicitly")
	}
	if rt.budget != nil {
		t.Errorf("a fresh runtime starts with no frame budget: got %+v", rt.budget)
	}
}

// TestCloneHasNoHostHooks pins the no-side-effect promise of qorm_simulate:
// a clone runs actions for preview, so it must never be able to push frames
// into the live session it was cloned from.
func TestCloneHasNoHostHooks(t *testing.T) {
	rt := New(renderApp(model.Step{Type: "render"}))
	live := 0
	rt.Commit = func() { live++ }
	c := rt.Clone()
	if c.Commit != nil {
		t.Fatal("Clone must not carry the host frame sink into a simulation")
	}
	c.Dispatch("go", nil)
	if live != 0 {
		t.Errorf("a simulated dispatch must not publish frames to the live host: got %d", live)
	}
}
