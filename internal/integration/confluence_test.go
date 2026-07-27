package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

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

// ---- async: the same guard, now over a real background host -----------------
//
// The determinism guard is INSTANTANEOUS determinism, and TestRenderStepConfluence
// above extends it to confluence under intermediate frames. Async extends it once
// more, to QUIESCENT determinism: an action whose http step runs in the
// background must, once nothing is in flight, render byte-identically to the
// same action written synchronously. Async is allowed to change the SEQUENCE of
// frames a dispatch produces; it is never allowed to change where it lands.
//
// "Quiescent" is no longer "Dispatch returned", so the guard needs a wait — but
// not a sleep: Runtime.Inflight() is an exact count, so waiting on it reaching
// zero is a decision, not a guess.

// bgHost is a minimal, faithful stand-in for a real host's background sink: it
// runs work on its own goroutine and delivers the outcome under the same lock
// that serialises dispatches, exactly as the server's spawn does. Having the
// guard drive its own host (rather than a whole server) keeps it in the
// integration package, where the shipped examples already live.
type bgHost struct {
	mu sync.Mutex
	rt *qrt.Runtime
}

func (h *bgHost) sink(work func() any, resume func(any)) {
	go func() {
		v := work()
		h.mu.Lock()
		defer h.mu.Unlock()
		resume(v)
	}()
}

// dispatch runs an action under the host lock, the way every host does.
func (h *bgHost) dispatch(action string, args map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rt.Dispatch(action, args)
}

// settle waits for every background request to deliver its outcome. Bounded
// poll on an exact counter — no sleep, and a timeout that fails loudly rather
// than letting a hung request masquerade as a passing render.
func (h *bgHost) settle(t *testing.T, timeout time.Duration) {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(timeout)
	for {
		h.mu.Lock()
		n := h.rt.Inflight()
		h.mu.Unlock()
		if n == 0 {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out with %d request(s) still in flight", n)
		}
	}
}

// stripAsync returns a copy of a step tree with every async flag cleared: the
// same action written the blocking way.
func stripAsync(steps []model.Step) []model.Step {
	out := make([]model.Step, 0, len(steps))
	for _, st := range steps {
		st.Async = false
		st.Then = stripAsync(st.Then)
		st.Else = stripAsync(st.Else)
		st.OnSuccess = stripAsync(st.OnSuccess)
		st.OnError = stripAsync(st.OnError)
		out = append(out, st)
	}
	return out
}

// countAsync counts async steps in a step tree, branches included.
func countAsync(steps []model.Step) int {
	n := 0
	for _, st := range steps {
		if st.Async {
			n++
		}
		n += countAsync(st.Then) + countAsync(st.Else)
		n += countAsync(st.OnSuccess) + countAsync(st.OnError)
	}
	return n
}

// assertAsyncConverges dispatches the named action twice: once on a runtime with
// a real background host, settled before rendering, and once with the async
// flags stripped and no host at all. The settled renders must be byte-identical.
func assertAsyncConverges(t *testing.T, app *model.App, action string) {
	t.Helper()
	if countAsync(app.Actions[action].Steps) == 0 {
		t.Fatalf("action %q declares no async step — the guard would prove nothing", action)
	}
	h := &bgHost{rt: qrt.New(app)}
	h.rt.Async = h.sink
	h.dispatch(action, nil)
	h.settle(t, 10*time.Second)

	blocking := *app
	blocking.Actions = make(map[string]*model.Action, len(app.Actions))
	for id, act := range app.Actions {
		blocking.Actions[id] = &model.Action{ID: act.ID, Steps: stripAsync(act.Steps)}
	}
	syncRT := qrt.New(&blocking)
	syncRT.Dispatch(action, nil)

	h.mu.Lock()
	defer h.mu.Unlock()
	if got, want := render.Render(h.rt).HTML, render.Render(syncRT).HTML; got != want {
		t.Errorf("action %q: async changed the settled render\n async: %s\n  sync: %s", action, got, want)
	}
	if h.rt.CurrentScene() != syncRT.CurrentScene() {
		t.Errorf("action %q: async changed the settled scene: %q vs %q",
			action, h.rt.CurrentScene(), syncRT.CurrentScene())
	}
}

// TestAsyncConvergesToSyncResult covers the shapes an async request can take —
// success, failure, a result read only through the branch, several requests in
// one action, and async combined with an intermediate frame — against local
// backends, so the guard is hermetic and fast.
func TestAsyncConvergesToSyncResult(t *testing.T) {
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
	scene := `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
		{"type":"text","id":"busy","if":"{{ state.saving }}","text":"busy"},
		{"type":"text","id":"p","text":"{{ state.phase }} / {{ state.title }} / {{ state.err }} / {{ state.n }}"}]}}`

	cases := []struct {
		name   string
		action string
	}{
		{"success-branch", `{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"http.get","url":"` + ok.URL + `","async":true,"result":"resp","error":"err",
			 "onSuccess":[{"type":"state.set","path":"title","value":"{{ response.title }}"},
			              {"type":"state.set","path":"saving","value":"{{ false }}"},
			              {"type":"state.set","path":"phase","value":"done"}]}]}`},
		{"error-branch", `{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"http.get","url":"` + boom.URL + `","async":true,"result":"resp","error":"err",
			 "onError":[{"type":"state.set","path":"saving","value":"{{ false }}"},
			            {"type":"state.set","path":"phase","value":"failed"}]}]}`},
		{"result-path-only", `{"type":"action","id":"go","steps":[
			{"type":"http.get","url":"` + ok.URL + `","async":true,"result":"resp"}]}`},
		{"with-intermediate-frame", `{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"render"},
			{"type":"http.get","url":"` + ok.URL + `","async":true,"result":"resp",
			 "onSuccess":[{"type":"render"},
			              {"type":"state.set","path":"saving","value":"{{ false }}"}]}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertAsyncConverges(t, fixtureApp(t, manifest, scene, c.action), "go")
		})
	}

	// Several requests in one action converge too, but only because each
	// continuation writes a DIFFERENT key: concurrent replies land in whatever
	// order the network delivers them, so two continuations writing the same key
	// are last-writer-wins and have no settled value to converge on. Asserting
	// that here is what keeps the property honest rather than accidental.
	t.Run("several-requests", func(t *testing.T) {
		action := `{"type":"action","id":"go","steps":[
			{"type":"http.get","url":"` + ok.URL + `","async":true,
			 "onSuccess":[{"type":"state.set","path":"title","value":"{{ response.title }}"}]},
			{"type":"http.get","url":"` + ok.URL + `","async":true,
			 "onSuccess":[{"type":"state.increment","path":"n"}]},
			{"type":"http.get","url":"` + boom.URL + `","async":true,
			 "onError":[{"type":"state.set","path":"phase","value":"failed"}]}]}`
		assertAsyncConverges(t, fixtureApp(t, manifest, scene, action), "go")
	})
}

// TestExampleAsyncActionsAreDeclaredWell keeps the shipped examples honest
// without reaching the public internet: an async step whose result is read by a
// SIBLING step rather than by its result branch would silently read the value
// from before the request. The runtime cannot detect that (the sibling is a
// perfectly valid expression), so it is pinned here, over whatever the examples
// actually ship.
func TestExampleAsyncActionsAreDeclaredWell(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}
	found := 0
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
			if countAsync(act.Steps) > 0 {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			found++
			t.Run(e.Name()+"/"+name, func(t *testing.T) {
				for _, st := range app.Actions[name].Steps {
					if !st.Async {
						continue
					}
					if len(st.OnSuccess) == 0 && len(st.OnError) == 0 && st.Result == "" && st.Path == "" {
						t.Errorf("async step in %q writes nothing: neither a result path nor a result branch", name)
					}
					if st.Error != "" && len(st.OnError) == 0 && len(st.OnSuccess) > 0 {
						t.Errorf("async step in %q handles success but not failure: a loading state cleared only in onSuccess is stuck forever on an error", name)
					}
				}
			})
		}
	}
	if found == 0 {
		t.Error("no example demonstrates an async request — the feature ships undemonstrated")
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

// ---- AsyncAll: the shipped examples under the packaged-app host --------------
//
// TestAsyncConvergesToSyncResult pins confluence for actions that OPTED INTO
// async. Packaged hosts (the standalone WASM runtime, the offline bundle, the
// playground) set Runtime.AsyncAll, which forces EVERY http step — declared
// sync or not — onto the background sink. An action whose post-request logic
// sits in SIBLING steps then reads the state from before the request: the
// loading flag is lowered the instant the request leaves, and a failure
// rollback silently never runs. That was P0-5, and the shipped examples below
// are where it bit.
//
// The cure is declaring the request async with its follow-up in the result
// branches (or a governance field), and this guard keeps the examples on that
// side of the line: each action must settle to the same render on a
// synchronous host (async stripped, no sink, the round trip blocking the
// dispatch) and on an AsyncAll host (sink installed, drained to quiescence).

// retargetHTTP points every http step of a step tree at a hermetic backend, so
// the shipped example JSON can be exercised without the public internet.
func retargetHTTP(steps []model.Step, url string) {
	for i := range steps {
		if len(steps[i].Type) > 5 && steps[i].Type[:5] == "http." {
			steps[i].URL = url
		}
		retargetHTTP(steps[i].Then, url)
		retargetHTTP(steps[i].Else, url)
		retargetHTTP(steps[i].OnSuccess, url)
		retargetHTTP(steps[i].OnError, url)
	}
}

// TestExampleActionsConvergeUnderAsyncAll runs the examples that call a
// backend through both host readings of the same JSON. A gated backend makes
// the in-flight window deterministic: the request stays open until the test
// releases it, so "the dispatch returned while the request was still open" is
// an assertion, not a race.
func TestExampleActionsConvergeUnderAsyncAll(t *testing.T) {
	newBackend := func(status int) (*httptest.Server, chan struct{}) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			<-release
			w.Header().Set("Content-Type", "application/json")
			if status != http.StatusOK {
				http.Error(w, "nope", status)
				return
			}
			_, _ = w.Write([]byte(`{"fact":"from backend"}`))
		}))
		return srv, release
	}

	cases := []struct {
		name   string
		dir    string
		action string
		args   map[string]any
		status int
	}{
		{"tasks/saveTask success", "tasks", "saveTask", map[string]any{"id": "a"}, http.StatusOK},
		{"tasks/saveTask failure rolls back", "tasks", "saveTask", map[string]any{"id": "a"}, http.StatusInternalServerError},
		{"netdemo/getFact success", "netdemo", "getFact", nil, http.StatusOK},
		{"netdemo/getFact failure", "netdemo", "getFact", nil, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, release := newBackend(c.status)
			defer srv.Close()
			app, err := loader.LoadDir(filepath.Join("..", "..", "examples", c.dir))
			if err != nil {
				t.Fatalf("load example %s: %v", c.dir, err)
			}
			for _, act := range app.Actions {
				retargetHTTP(act.Steps, srv.URL)
			}

			// The packaged-app host: every http step forced onto the
			// background sink, drained to quiescence before rendering.
			h := &bgHost{rt: qrt.New(app)}
			h.rt.AsyncAll = true
			h.rt.Async = h.sink
			h.dispatch(c.action, c.args)
			h.mu.Lock()
			n := h.rt.Inflight()
			h.mu.Unlock()
			if n != 1 {
				t.Fatalf("AsyncAll host: dispatch returned with %d request(s) in flight, want 1 — the request did not go background", n)
			}
			close(release)
			h.settle(t, 10*time.Second)

			// The synchronous reading of the same JSON: async stripped, no
			// sink, the round trip blocking the dispatch.
			syncApp := *app
			syncApp.Actions = make(map[string]*model.Action, len(app.Actions))
			for id, act := range app.Actions {
				syncApp.Actions[id] = &model.Action{ID: act.ID, Steps: stripAsync(act.Steps)}
			}
			syncRT := qrt.New(&syncApp)
			syncRT.Dispatch(c.action, c.args)

			h.mu.Lock()
			defer h.mu.Unlock()
			if got, want := render.Render(h.rt).HTML, render.Render(syncRT).HTML; got != want {
				t.Errorf("AsyncAll changed the settled render\n asyncAll: %s\n     sync: %s", got, want)
			}
			if h.rt.CurrentScene() != syncRT.CurrentScene() {
				t.Errorf("AsyncAll changed the settled scene: %q vs %q",
					h.rt.CurrentScene(), syncRT.CurrentScene())
			}
		})
	}
}
