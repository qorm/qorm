package integration

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render"
	qrt "github.com/qorm/platform/internal/runtime"
)

// maxDispatchedActionsPerExample caps the post-dispatch sweep. Every action is
// swept for all but the largest example sets; the cap only bounds the worst
// case (the `components` gallery has dozens), and the ids are sorted, so which
// actions run is stable rather than a map-order accident.
const maxDispatchedActionsPerExample = 12

// determinismRepeats is how many times a single view is rendered before its
// output is called stable. Two was not enough: a map with two keys iterates in
// the same order about half the time, so a genuinely unordered emission could
// clear a two-render check on any given run.
const determinismRepeats = 3

// TestAllExamplesRenderDeterministically walks every example app and asserts it
// loads, renders non-empty, and renders identically every time — guarding
// against non-deterministic output (e.g. map-iteration order) across the whole
// widget set.
//
// Three axes, because a check that only ever renders one view in one state
// cannot see most of what the runtime does:
//
//   - EVERY SCENE, not just the entry one. A widget used exclusively on a
//     secondary scene (a settings sheet, a detail route) was never rendered by
//     this test at all.
//   - REPEATED, not compared once. See determinismRepeats.
//   - AFTER EACH ACTION, not only in the initial state. This is the axis that
//     matters most now: timers, `render`'s intermediate frames, a derived-value
//     refresh, an async completion and `forEach` all produce state that only
//     exists once something has been dispatched, and the initial-state check
//     never reaches any of it. The dispatch is run twice from two independent
//     runtimes, so a non-deterministic ACTION fails here too, not just a
//     non-deterministic render.
//
// Actions that would reach the network are skipped: this suite must stay
// hermetic, and the runtime's http client is not injectable from here.
func TestAllExamplesRenderDeterministically(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err != nil {
			continue // not an app dir
		}
		// A game that seeds its RNG from now() (the one non-deterministic
		// builtin) intentionally plays a different sequence every restart —
		// determinism across runs is not a property of the example, so the
		// suite skips it. (g2048 keeps a fixed seed and stays in the suite.)
		if e.Name() == "tetris" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			app, err := loader.LoadDir(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}

			entryHTML := renderStable(t, "entry scene", func() string {
				return render.Render(qrt.New(app)).HTML
			})
			if len(entryHTML) == 0 {
				t.Fatal("rendered empty")
			}
			if !strings.Contains(entryHTML, "id=") {
				t.Error("rendered output has no elements")
			}

			for _, id := range sortedKeys(app.Scenes) {
				renderStable(t, "scene "+id, func() string {
					return render.RenderScene(qrt.New(app), id).HTML
				})
			}

			for _, id := range dispatchableActions(app) {
				renderStable(t, "after action "+id, func() string {
					rt := qrt.New(app)
					rt.Dispatch(id, nil)
					// The action may have navigated; render where it landed,
					// which is the frame a user would actually be looking at.
					return render.RenderScene(rt, rt.CurrentScene()).HTML
				})
			}
		})
	}
}

// renderStable renders `what` determinismRepeats times and fails if the output
// ever differs, returning the first rendering.
func renderStable(t *testing.T, what string, render func() string) string {
	t.Helper()
	first := render()
	for i := 1; i < determinismRepeats; i++ {
		if got := render(); got != first {
			t.Errorf("%s: render %d of %d differs — output is non-deterministic", what, i+1, determinismRepeats)
			return first
		}
	}
	return first
}

// dispatchableActions returns the action ids the sweep may run: sorted, capped,
// and with anything that would open a socket left out.
func dispatchableActions(app *model.App) []string {
	ids := make([]string, 0, len(app.Actions))
	for _, id := range sortedKeys(app.Actions) {
		if act := app.Actions[id]; act == nil || reachesTheNetwork(act.Steps) {
			continue
		}
		ids = append(ids, id)
		if len(ids) == maxDispatchedActionsPerExample {
			break
		}
	}
	return ids
}

// reachesTheNetwork reports whether a step list contains an http step anywhere
// in it, including inside branches, loop bodies and async continuations.
func reachesTheNetwork(steps []model.Step) bool {
	for _, st := range steps {
		if strings.HasPrefix(st.Type, "http.") {
			return true
		}
		for _, nested := range [][]model.Step{st.Then, st.Else, st.Steps, st.OnSuccess, st.OnError} {
			if reachesTheNetwork(nested) {
				return true
			}
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
