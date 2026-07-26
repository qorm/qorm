package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// The determinism guard, extended.
//
// TestAllExamplesRenderDeterministically (conformance_test.go) asserts
// INSTANTANEOUS determinism: the same state renders byte-identically twice. It
// never dispatches, and runtime.New installs no host hooks, so intermediate
// frames cannot reach it — it stays exactly as written.
//
// What intermediate frames add is a second property worth guarding, CONFLUENCE:
// publishing frames mid-action must change only the sequence of frames, never
// where the action lands. Once quiescent, an action carrying `render` steps must
// render byte-identically to the same action with those steps deleted. At this
// stage a dispatch is run-to-completion, so "quiescent" is simply "Dispatch
// returned" — the guard needs no sleeps and no polling.

// stripRenderSteps returns a copy of a step tree with every `render` step
// removed, branches included: the same action written the pre-frames way.
func stripRenderSteps(steps []model.Step) []model.Step {
	out := make([]model.Step, 0, len(steps))
	for _, st := range steps {
		if st.Type == "render" {
			continue
		}
		st.Then = stripRenderSteps(st.Then)
		st.Else = stripRenderSteps(st.Else)
		st.OnSuccess = stripRenderSteps(st.OnSuccess)
		st.OnError = stripRenderSteps(st.OnError)
		out = append(out, st)
	}
	return out
}

// countRenders counts `render` steps in a step tree, branches included.
func countRenders(steps []model.Step) int {
	n := 0
	for _, st := range steps {
		if st.Type == "render" {
			n++
		}
		n += countRenders(st.Then) + countRenders(st.Else)
		n += countRenders(st.OnSuccess) + countRenders(st.OnError)
	}
	return n
}

// assertConverges dispatches the named action twice: once on a runtime with a
// host frame sink installed (so every `render` really publishes), once on a
// runtime whose `render` steps have been deleted and which has no host at all.
// Both settled renders must be byte-identical, and the first must have actually
// published something — otherwise the test proves nothing.
func assertConverges(t *testing.T, app *model.App, action string) {
	t.Helper()
	withRT := qrt.New(app)
	frames := 0
	withRT.Commit = func() { frames++ }
	withRT.Dispatch(action, nil)

	bare := *app
	bare.Actions = make(map[string]*model.Action, len(app.Actions))
	for id, act := range app.Actions {
		bare.Actions[id] = &model.Action{ID: act.ID, Steps: stripRenderSteps(act.Steps)}
	}
	bareRT := qrt.New(&bare)
	bareRT.Dispatch(action, nil)

	if frames == 0 {
		t.Fatalf("action %q declares a render step but published no frame", action)
	}
	if got, want := render.Render(withRT).HTML, render.Render(bareRT).HTML; got != want {
		t.Errorf("action %q: intermediate frames changed the settled render\n with: %s\nwithout: %s", action, got, want)
	}
	if withRT.CurrentScene() != bareRT.CurrentScene() {
		t.Errorf("action %q: intermediate frames changed the settled scene: %q vs %q",
			action, withRT.CurrentScene(), bareRT.CurrentScene())
	}
}

// fixtureApp loads a JSON document list the way the playground does, failing on
// any diagnostic.
func fixtureApp(t *testing.T, raw ...string) *model.App {
	t.Helper()
	docs := make([]map[string]any, len(raw))
	for i, s := range raw {
		if err := json.Unmarshal([]byte(s), &docs[i]); err != nil {
			t.Fatalf("fixture doc %d: %v", i, err)
		}
	}
	app := loader.FromDocs(docs)
	if len(app.Diagnostics) != 0 {
		t.Fatalf("fixture must load clean: %v", app.Diagnostics)
	}
	return app
}

// TestRenderStepConfluence covers the shapes a `render` step can appear in —
// before a backend call, inside both http result branches, inside an `if`
// branch, and around a navigation — against a local backend, so the guard is
// hermetic and fast.
func TestRenderStepConfluence(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"from backend"}`))
	}))
	defer ok.Close()
	boom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer boom.Close()

	manifest := `{"type":"app","id":"cf","entry":"main","globalState":{
		"schema":{"saving":"bool","phase":"string","title":"string","err":"string","n":"number"},
		"initial":{"saving":false,"phase":"","title":"","err":"","n":0}}}`
	scenes := []string{
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"busy","if":"{{ state.saving }}","text":"busy"},
			{"type":"text","id":"p","text":"{{ state.phase }} / {{ state.title }} / {{ state.err }} / {{ state.n }}"}]}}`,
		`{"type":"scene","id":"detail","root":{"type":"text","id":"d","text":"detail {{ state.n }}"}}`,
	}

	cases := []struct {
		name   string
		action string
	}{
		{"loading-then-success", `{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"render"},
			{"type":"http.get","url":"` + ok.URL + `","result":"resp","error":"err",
			 "onSuccess":[{"type":"state.set","path":"title","value":"{{ response.title }}"},
			              {"type":"render"},
			              {"type":"state.set","path":"phase","value":"done"}]},
			{"type":"state.set","path":"saving","value":"{{ false }}"}]}`},
		{"loading-then-error", `{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"render"},
			{"type":"http.get","url":"` + boom.URL + `","result":"resp","error":"err",
			 "onError":[{"type":"render"},
			            {"type":"state.set","path":"phase","value":"failed"}]},
			{"type":"state.set","path":"saving","value":"{{ false }}"}]}`},
		{"inside-if-branch", `{"type":"action","id":"go","steps":[
			{"type":"if","condition":"{{ state.n == 0 }}",
			 "then":[{"type":"state.increment","path":"n"},
			         {"type":"render"},
			         {"type":"state.increment","path":"n"}],
			 "else":[{"type":"render"}]}]}`},
		{"around-navigate", `{"type":"action","id":"go","steps":[
			{"type":"state.increment","path":"n"},
			{"type":"render"},
			{"type":"navigate","to":"detail"},
			{"type":"render"},
			{"type":"state.increment","path":"n"}]}`},
		{"through-invoke", `{"type":"action","id":"go","steps":[
			{"type":"render"},
			{"type":"invoke","name":"inner"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			docs := append([]string{manifest}, scenes...)
			docs = append(docs, c.action,
				`{"type":"action","id":"inner","steps":[{"type":"render"},{"type":"state.increment","path":"n"}]}`)
			assertConverges(t, fixtureApp(t, docs...), "go")
		})
	}
}

// TestExampleRenderStepsConverge runs the same guard over the shipped examples,
// so an example that starts using `render` is covered the moment it lands.
// Actions that call a backend are skipped here — reaching the public internet
// from a unit test would be slow and flaky; the http shapes are covered
// hermetically by TestRenderStepConfluence above.
func TestExampleRenderStepsConverge(t *testing.T) {
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
			continue
		}
		app, err := loader.LoadDir(dir)
		if err != nil {
			continue // load failures are the loader gates' business
		}
		var names []string
		for name, act := range app.Actions {
			if countRenders(act.Steps) > 0 && !callsBackend(app, name, 0) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			t.Run(e.Name()+"/"+name, func(t *testing.T) { assertConverges(t, app, name) })
		}
	}
}

// callsBackend reports whether an action reaches the network, directly or
// through an invoke chain (depth-guarded against mutual recursion).
func callsBackend(app *model.App, name string, depth int) bool {
	if depth > 8 {
		return true // give up safely: treat an unbounded chain as networked
	}
	act, ok := app.Actions[name]
	if !ok {
		return false
	}
	var walk func([]model.Step) bool
	walk = func(steps []model.Step) bool {
		for _, st := range steps {
			if len(st.Type) > 5 && st.Type[:5] == "http." {
				return true
			}
			if st.Type == "invoke" && callsBackend(app, st.Name, depth+1) {
				return true
			}
			if walk(st.Then) || walk(st.Else) || walk(st.OnSuccess) || walk(st.OnError) {
				return true
			}
		}
		return false
	}
	return walk(act.Steps)
}
