package testrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

// Spec error codes — see the package doc for the full list and the spec
// revision notes for which are new to the MVP.
const (
	ErrLoad            = "test_load_error"
	ErrNoneFound       = "test_none_found"
	ErrDocInvalid      = "test_doc_invalid"
	ErrSceneNotFound   = "test_scene_not_found"
	ErrStepUnknown     = "test_step_unknown"
	ErrAssertUnknown   = "test_assert_unknown"
	ErrAssertionFailed = "test_assertion_failed"
	ErrInvalidSelector = "query_invalid_selector"
	ErrQueryAmbiguous  = "test_query_ambiguous"
	ErrTargetNotFound  = "test_target_not_found"
	ErrEventUnknown    = "test_event_unknown"
	ErrEventNotHandled = "test_event_not_handled"
	ErrActionNotFound  = "test_action_not_found"
	ErrRuntime         = "test_runtime_error"
)

// Status values used across Report and TestResult (spec: "passed"; the runner
// reports "failed" for assertion failures and "error" for tests that could
// not complete — a missing scene, an unknown step, a runtime script error).
const (
	StatusPassed = "passed"
	StatusFailed = "failed"
	StatusError  = "error"
)

// Report is the minimal spec report: overall outcome, counts, and one entry
// per executed test. Diagnostics carry runner-level notes (e.g. a deferred
// "mocks" section that was ignored); hostCalls is always empty in the MVP
// (no host-mock registry yet) and kept in the shape for spec compatibility.
type Report struct {
	Status      string       `json:"status"`
	Tests       int          `json:"tests"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	Diagnostics []string     `json:"diagnostics"`
	HostCalls   []string     `json:"hostCalls"`
	DurationMs  int64        `json:"durationMs"`
	Results     []TestResult `json:"results"`
}

// TestResult is one test document's outcome.
type TestResult struct {
	ID       string    `json:"id"`
	File     string    `json:"file"`
	Status   string    `json:"status"`
	Failures []Failure `json:"failures,omitempty"`
	Errors   []string  `json:"errors,omitempty"`
}

// Failure is one failed assertion (or a query/selector error the test
// document itself caused): the spec requires the failed assertion, actual vs
// expected, and the target selector or path.
type Failure struct {
	Code     string `json:"errorCode"`
	Assert   string `json:"assert"`
	Target   string `json:"target"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

// Step is one parsed test step.
type Step struct {
	Type   string
	Scene  string
	Action string
	Args   map[string]any
	Target map[string]any
	Event  string
	Path   string
	Value  any
}

// Assert is one parsed assertion.
type Assert struct {
	Type   string
	Path   string
	Value  any
	Target map[string]any
	Prop   string
}

// Doc is one parsed type:"test" document.
type Doc struct {
	ID      string
	File    string
	Scene   string
	Steps   []Step
	Asserts []Assert
}

// Run loads the app at appDir, discovers or takes the test documents listed
// in files (empty = default discovery: every type:"test" document under
// appDir, canonically tests/*.json), executes each against a fresh runtime,
// and returns the report. The returned error is only the report-level failure
// (app load, discovery walk, no tests found) — per-test failures live in the
// report. Exit semantics are the CLI's: the report's Status is "passed" iff
// every test passed.
func Run(appDir string, files []string) (*Report, error) {
	start := time.Now()
	report := &Report{Status: StatusPassed, Diagnostics: []string{}, HostCalls: []string{}}

	app, err := loader.LoadDir(appDir)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", ErrLoad, err)
	}

	docs, diags, err := collectDocs(appDir, files)
	if err != nil {
		return nil, err
	}
	report.Diagnostics = append(report.Diagnostics, diags...)
	if len(docs) == 0 {
		return nil, fmt.Errorf("%s: no type:\"test\" documents found under %s (canonical location: %s)", ErrNoneFound, appDir, filepath.Join(appDir, "tests"))
	}

	// Lexicographic order (the loader's walk order) so reports are stable.
	sort.Slice(docs, func(i, j int) bool { return docs[i].File < docs[j].File })

	for _, doc := range docs {
		result := runDoc(app, doc)
		report.Tests++
		switch result.Status {
		case StatusPassed:
			report.Passed++
		default:
			report.Failed++
		}
		report.Results = append(report.Results, *result)
	}
	if report.Failed > 0 {
		report.Status = StatusFailed
	}
	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

// collectDocs returns the parsed test documents for the run: the named files
// when given (each is read directly and must be a type:"test" doc), otherwise
// loader.CollectTestDocs default discovery. Parsing notes (deferred features,
// non-test docs) become diagnostics, not failures.
func collectDocs(appDir string, files []string) ([]*Doc, []string, error) {
	var raw []map[string]any
	var diags []string
	if len(files) > 0 {
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %v", ErrLoad, err)
			}
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				return nil, nil, fmt.Errorf("%s: %s: %v", ErrDocInvalid, f, err)
			}
			doc["source"] = filepath.ToSlash(f)
			raw = append(raw, doc)
		}
	} else {
		docs, err := loader.CollectTestDocs(appDir)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", ErrLoad, err)
		}
		raw = append(raw, docs...)
	}
	var out []*Doc
	for _, doc := range raw {
		parsed, diag, err := parseDoc(doc)
		if err != nil {
			return nil, nil, err
		}
		if diag != "" {
			diags = append(diags, diag)
		}
		out = append(out, parsed)
	}
	return out, diags, nil
}

// parseDoc validates a raw test document and extracts the runnable shape.
func parseDoc(doc map[string]any) (*Doc, string, error) {
	d := &Doc{File: sourceOf(doc)}
	if t := loader.DocType(doc); t != "test" {
		return nil, "", fmt.Errorf("%s: %s: document type %q, want \"test\"", ErrDocInvalid, d.File, t)
	}
	if v := loader.DocString(doc, "qorm"); v != "" && v != "0.1" {
		return nil, "", fmt.Errorf("%s: %s: unsupported qorm format version %q (only \"0.1\")", ErrDocInvalid, d.File, v)
	}
	d.ID = loader.DocID(doc)
	if d.ID == "" {
		d.ID = strings.TrimSuffix(filepath.Base(d.File), ".json")
	}
	d.Scene = loader.DocString(doc, "scene")

	// Deferred MVP surfaces are read for diagnostics, never executed: a test
	// that declares "mocks" runs with the host-mock registry absent, so its
	// host capabilities execute UNMOCKED — say so, loudly, in the report.
	diag := ""
	if mocks, ok := doc["mocks"].(map[string]any); ok && len(mocks) > 0 {
		diag = fmt.Sprintf("%s: \"mocks\" (%d entries) ignored: host-mock registry deferred — host capabilities run unmocked for this test", d.File, len(mocks))
	}

	var steps []Step
	if raw, ok := doc["steps"].([]any); ok {
		for i, s := range raw {
			m, ok := s.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("%s: %s: steps[%d] is not an object", ErrDocInvalid, d.File, i)
			}
			step, err := parseStep(d.File, i, m)
			if err != nil {
				return nil, "", err
			}
			steps = append(steps, *step)
		}
	} else if _, present := doc["steps"]; present {
		return nil, "", fmt.Errorf("%s: %s: \"steps\" must be an array", ErrDocInvalid, d.File)
	}
	d.Steps = steps

	var asserts []Assert
	rawAsserts, has := doc["assert"]
	if !has {
		rawAsserts, has = doc["asserts"]
	}
	if has {
		list, ok := rawAsserts.([]any)
		if !ok {
			return nil, "", fmt.Errorf("%s: %s: \"assert\" must be an array", ErrDocInvalid, d.File)
		}
		for i, a := range list {
			m, ok := a.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("%s: %s: assert[%d] is not an object", ErrDocInvalid, d.File, i)
			}
			assert, err := parseAssert(d.File, i, m)
			if err != nil {
				return nil, "", err
			}
			asserts = append(asserts, *assert)
		}
	}
	d.Asserts = asserts
	return d, diag, nil
}

func parseStep(file string, i int, m map[string]any) (*Step, error) {
	st := &Step{Type: loader.DocString(m, "type")}
	switch st.Type {
	case "mount_scene":
		st.Scene = loader.DocString(m, "scene")
	case "simulate_event":
		// Direct dispatch spelling: {"action": "...", "args": {...}}.
		if a, ok := m["action"].(string); ok && a != "" {
			st.Action = a
		} else if n, ok := m["name"].(string); ok && n != "" { // lenient alias
			st.Action = n
		}
		if raw, ok := m["args"].(map[string]any); ok {
			st.Args = raw
		}
		// Node-event spelling: {"target": {...}, "event": "press"}.
		if raw, ok := m["target"].(map[string]any); ok {
			st.Target = raw
			st.Event = loader.DocString(m, "event")
		} else if _, present := m["target"]; present {
			return nil, fmt.Errorf("%s: %s: steps[%d]: simulate_event \"target\" must be an object selector", ErrDocInvalid, file, i)
		}
		if st.Action == "" && st.Target == nil {
			return nil, fmt.Errorf("%s: %s: steps[%d]: simulate_event needs either \"action\" or {\"target\", \"event\"}", ErrDocInvalid, file, i)
		}
	case "set_state":
		st.Path = loader.DocString(m, "path")
		if st.Path == "" {
			return nil, fmt.Errorf("%s: %s: steps[%d]: set_state needs a non-empty \"path\"", ErrDocInvalid, file, i)
		}
		st.Value = m["value"]
	}
	// Unknown step types parse fine and fail at EXEC time with
	// test_step_unknown: one deferred step (advance_time, flush_async) must
	// not take down the whole suite at load time — it errors this test, and
	// the other tests still report.
	return st, nil
}

func parseAssert(file string, i int, m map[string]any) (*Assert, error) {
	a := &Assert{Type: loader.DocString(m, "type")}
	switch a.Type {
	case "state_equals":
		a.Path = loader.DocString(m, "path")
		if a.Path == "" {
			return nil, fmt.Errorf("%s: %s: assert[%d]: state_equals needs a non-empty \"path\"", ErrDocInvalid, file, i)
		}
		a.Value = m["value"]
	case "node_exists", "node_not_exists":
		if raw, ok := m["target"].(map[string]any); ok {
			a.Target = raw
		} else {
			return nil, fmt.Errorf("%s: %s: assert[%d]: %s needs a \"target\" object selector", ErrDocInvalid, file, i, a.Type)
		}
	case "text_equals", "prop_equals":
		if raw, ok := m["target"].(map[string]any); ok {
			a.Target = raw
		} else {
			return nil, fmt.Errorf("%s: %s: assert[%d]: %s needs a \"target\" object selector", ErrDocInvalid, file, i, a.Type)
		}
		if a.Type == "prop_equals" {
			a.Prop = loader.DocString(m, "prop")
			if a.Prop == "" {
				a.Prop = loader.DocString(m, "key") // lenient alias
			}
			if a.Prop == "" {
				return nil, fmt.Errorf("%s: %s: assert[%d]: prop_equals needs a \"prop\" key", ErrDocInvalid, file, i)
			}
		}
		a.Value = m["value"]
	}
	// Unknown assert types parse fine and surface at EVAL time as a
	// test_assert_unknown failure (evalAssert's default case), so one
	// deferred assert does not take down the whole suite at load time.
	return a, nil
}

// sourceOf recovers the file a discovered document came from (the loader's
// collected docs carry a "source" provenance field; explicit file args get
// one in collectDocs). Unslashed for CLI display.
func sourceOf(doc map[string]any) string {
	if s := loader.DocString(doc, "source"); s != "" {
		return filepath.FromSlash(s)
	}
	return "(inline)"
}

// runDoc executes one test document against a fresh runtime: the document's
// scene is mounted (firing its onEnter), the steps run in order, then the
// asserts. A failing step aborts the test with StatusError (the runtime is in
// a poisoned state and later asserts would be noise); failing asserts are all
// collected.
func runDoc(app *model.App, doc *Doc) *TestResult {
	rt := qrt.New(app)
	res := &TestResult{ID: doc.ID, File: doc.File, Status: StatusPassed}

	scene := doc.Scene
	if scene == "" {
		scene = app.Entry
	}
	if err := mountScene(rt, scene); err != nil {
		res.Status = StatusError
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	for i, step := range doc.Steps {
		if err := execStep(rt, step); err != nil {
			res.Status = StatusError
			res.Errors = append(res.Errors, fmt.Sprintf("step %d (%s): %v", i+1, step.Type, err))
			return res
		}
	}

	for _, a := range doc.Asserts {
		if f := evalAssert(rt, a); f != nil {
			res.Failures = append(res.Failures, *f)
		}
	}
	if len(res.Failures) > 0 {
		res.Status = StatusFailed
	}
	return res
}

// sortedKeys is a small helper kept for stable selector iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mountScene points the runtime at a scene and drains its onEnter exactly like
// a host mount would. The scene field accepts the spec's "scene://" prefix.
// A guard that diverts the mount is fine (the runtime lands where the app
// would land — a test can assert the redirect); a guard that refuses entry
// outright parks the runtime on GuardBlocked, which is reported. Any onEnter
// whose action raises a qscript runtime error fails the mount — read from
// EnterScriptError, which survives the whole chain: each link's dispatch
// clears LastScriptError at its boundary, so a crash anywhere in a
// navigate-then-enter chain must not be hidden by a later clean scene (a
// green run must never come out of a crash during an enter hook).
func mountScene(rt *qrt.Runtime, scene string) error {
	scene = strings.TrimPrefix(scene, "scene://")
	if rt.App.Scenes[scene] == nil {
		return fmt.Errorf("%s: scene %q does not exist (known: %s)", ErrSceneNotFound, scene, strings.Join(sortedKeys(rt.App.Scenes), ", "))
	}
	rt.Scene = scene
	if rt.RouteParams == nil {
		rt.RouteParams = map[string]any{}
	}
	rt.NavDir = "push"
	// Mark the mount explicitly: only the very first drain rides the flag New
	// raised for the entry scene — a mount_scene STEP must fire the target's
	// onEnter too, and the drain loop only runs while pendingEnter is raised.
	rt.MarkPendingEnter()
	rt.RunPendingEnter()
	if rt.Blocked() {
		return fmt.Errorf("%s: scene %q was refused by its route guard (runtime parked on GuardBlocked)", ErrSceneNotFound, scene)
	}
	if se := rt.EnterScriptError; se != "" {
		return fmt.Errorf("%s: onEnter of scene %q failed: %s", ErrRuntime, scene, se)
	}
	return nil
}

// execStep runs one test step, returning an error carrying a spec error code.
func execStep(rt *qrt.Runtime, step Step) error {
	switch step.Type {
	case "mount_scene":
		return mountScene(rt, step.Scene)
	case "set_state":
		if !rt.SetStatePath(step.Path, step.Value) {
			return fmt.Errorf("%s: set_state refused (empty path or write into the read-only computed namespace)", ErrRuntime)
		}
		return nil
	case "simulate_event":
		return simulateEvent(rt, step)
	default:
		return fmt.Errorf("%s: unknown step type %q", ErrStepUnknown, step.Type)
	}
}

// simulateEvent runs the node-event or direct-action spelling. Both evaluate
// handler args against CURRENT state (EvalArgs), exactly like the server's
// press path, so a dispatched action sees the state the previous steps left.
func simulateEvent(rt *qrt.Runtime, step Step) error {
	if step.Action != "" {
		if rt.App.Actions[step.Action] == nil {
			return fmt.Errorf("%s: action %q is not defined by the app", ErrActionNotFound, step.Action)
		}
		if err := rt.DispatchErr(step.Action, evalArgs(rt, step.Args)); err != nil {
			return fmt.Errorf("%s: dispatch %q: %v", ErrRuntime, step.Action, err)
		}
		return scriptErr(rt)
	}
	inv, event, err := targetHandler(rt, step.Target, step.Event)
	if err != nil {
		return err
	}
	if err := rt.DispatchErr(inv.Name, rt.EvalArgs(inv.Args)); err != nil {
		return fmt.Errorf("%s: dispatch %q: %v", ErrRuntime, event, err)
	}
	return scriptErr(rt)
}

// scriptErr reports a script action's runtime failure, which surfaces on
// LastScriptError rather than through the dispatch return.
func scriptErr(rt *qrt.Runtime) error {
	if rt.LastScriptError != "" {
		return fmt.Errorf("%s: script failed: %s", ErrRuntime, rt.LastScriptError)
	}
	return nil
}

// evalArgs interpolates string arg values before dispatch (a {"msg":
// "{{state.x}}"} arg is resolved like any scene binding); non-strings pass
// through unchanged.
func evalArgs(rt *qrt.Runtime, args map[string]any) map[string]any {
	if len(args) == 0 {
		return args
	}
	ctx := bindingCtx(rt)
	out := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok {
			out[k] = qrt.EvalBinding(s, ctx)
		} else {
			out[k] = v
		}
	}
	return out
}

// events maps a simulate_event event name to the model handler it exercises.
var events = map[string]string{
	"press":      "OnPress",
	"submit":     "OnPress",
	"tap":        "OnPress",
	"change":     "OnChange",
	"input":      "OnChange",
	"keydown":    "OnKeyDown",
	"keyup":      "OnKeyUp",
	"hoverin":    "OnHoverIn",
	"hoverout":   "OnHoverOut",
	"touchstart": "OnTouchStart",
	"touchmove":  "OnTouchMove",
	"touchend":   "OnTouchEnd",
}

// targetHandler finds the node a target selector matches in the materialized
// tree and returns the handler the named event dispatches. A selector that
// matches no node is test_target_not_found; more than one is
// test_query_ambiguous; unbound events and unhandled nodes get their own
// codes, so a typo never masquerades as a green no-op.
func targetHandler(rt *qrt.Runtime, sel map[string]any, event string) (*model.Invoke, string, error) {
	nodes := materialize(rt)
	matches := matchSelector(nodes, sel)
	if len(matches) == 0 {
		return nil, event, fmt.Errorf("%s: no node matches selector {%s} in scene %q", ErrTargetNotFound, selectorString(sel), sceneName(rt))
	}
	if len(matches) > 1 {
		return nil, event, fmt.Errorf("%s: selector {%s} matches %d nodes; simulate_event needs exactly one", ErrQueryAmbiguous, selectorString(sel), len(matches))
	}
	field, ok := events[strings.ToLower(event)]
	if !ok {
		return nil, event, fmt.Errorf("%s: event %q (supported: press, change, keydown, keyup, hoverin, hoverout, touchstart, touchmove, touchend)", ErrEventUnknown, event)
	}
	inv := handlerFor(matches[0].n, field)
	if inv == nil {
		return nil, event, fmt.Errorf("%s: node %q has no %s handler", ErrEventNotHandled, matches[0].id, field)
	}
	if strings.Contains(inv.Name, "{{") {
		inv = &model.Invoke{Name: qrt.Stringify(qrt.EvalBinding(inv.Name, bindingCtx(rt))), Args: inv.Args}
	}
	return inv, event, nil
}

// handlerFor reads a handler field off a model node by name, so the event map
// above stays the single place names are bound.
func handlerFor(n *model.Node, field string) *model.Invoke {
	switch field {
	case "OnPress":
		return n.OnPress
	case "OnChange":
		return n.OnChange
	case "OnKeyDown":
		return n.OnKeyDown
	case "OnKeyUp":
		return n.OnKeyUp
	case "OnHoverIn":
		return n.OnHoverIn
	case "OnHoverOut":
		return n.OnHoverOut
	case "OnTouchStart":
		return n.OnTouchStart
	case "OnTouchMove":
		return n.OnTouchMove
	case "OnTouchEnd":
		return n.OnTouchEnd
	}
	return nil
}
