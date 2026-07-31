package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// ---- qorm_window ----

func TestWindowControlUnavailable(t *testing.T) {
	s := newCounterHandler(t)
	requireToolErr(t, toolCallRPC(t, s, "qorm_window", map[string]any{"op": "move"}),
		"window control unavailable")
}

func TestWindowControlOps(t *testing.T) {
	s := newCounterHandler(t)

	type moveCall struct {
		id   string
		x, y int
		w, h int
	}
	var moves []moveCall
	var ops []string
	var opens []string
	var evals []string
	s.SetWindowControl(
		func(id string, x, y, w, h int) { moves = append(moves, moveCall{id, x, y, w, h}) },
		func(id, op string) { ops = append(ops, id+":"+op) },
		func(id, url string, w, h int) { opens = append(opens, fmt.Sprintf("%s|%s|%dx%d", id, url, w, h)) },
		func(id, js string) { evals = append(evals, id+"|"+js) },
	)

	// move with explicit id and geometry.
	text := resultText(t, toolCallRPC(t, s, "qorm_window",
		map[string]any{"op": "move", "id": "aux", "x": 10, "y": 20, "w": 300, "h": 200}))
	if want := "moved window aux to (10,20) 300x200"; text != want {
		t.Errorf("move result = %q, want %q", text, want)
	}
	// Empty op defaults to move; empty id defaults to "main".
	toolCallRPC(t, s, "qorm_window", map[string]any{})
	// open / eval / named op.
	if text := resultText(t, toolCallRPC(t, s, "qorm_window",
		map[string]any{"op": "open", "id": "w2", "url": "https://example.com", "w": 640, "h": 480})); text != "opened window w2" {
		t.Errorf("open result = %q", text)
	}
	if text := resultText(t, toolCallRPC(t, s, "qorm_window",
		map[string]any{"op": "eval", "id": "w2", "js": "console.log(1)"})); text != "eval sent to w2" {
		t.Errorf("eval result = %q", text)
	}
	if text := resultText(t, toolCallRPC(t, s, "qorm_window",
		map[string]any{"op": "focus"})); text != "window main op: focus" {
		t.Errorf("focus result = %q", text)
	}

	if len(moves) != 2 || moves[0] != (moveCall{"aux", 10, 20, 300, 200}) || moves[1] != (moveCall{"main", 0, 0, 0, 0}) {
		t.Errorf("mover calls = %+v", moves)
	}
	if len(opens) != 1 || opens[0] != "w2|https://example.com|640x480" {
		t.Errorf("open calls = %v", opens)
	}
	if len(evals) != 1 || evals[0] != "w2|console.log(1)" {
		t.Errorf("eval calls = %v", evals)
	}
	if len(ops) != 1 || ops[0] != "main:focus" {
		t.Errorf("op calls = %v", ops)
	}
}

// TestWindowControlNilSubproviders covers a mover-only wiring (open/eval/op
// hooks nil): the corresponding ops must degrade to no-ops without panicking.
func TestWindowControlNilSubproviders(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	moved := 0
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{},
		windowMover: func(string, int, int, int, int) { moved++ }}

	for op, want := range map[string]string{
		"open":     "opened window main",
		"eval":     "eval sent to main",
		"minimize": "window main op: minimize",
	} {
		if text := resultText(t, toolCallRPC(t, s, "qorm_window", map[string]any{"op": op})); text != want {
			t.Errorf("op %q with nil hook = %q, want %q", op, text, want)
		}
	}
	if moved != 0 {
		t.Errorf("mover must not fire for open/eval/minimize, got %d calls", moved)
	}
}

// TestMalformedToolArgsAreErrorsNotZeroValues pins the strict-decode contract:
// arguments that are present but unparseable (wrong JSON shape or wrong field
// types) are a tool error, never a silent decay into zero values. Before this,
// a malformed qorm_window call moved the window to (0,0) 0x0, a malformed
// qorm_dispatch reported `unknown action ""`, and a malformed qorm_assert
// passed trivially on an empty check list.
func TestMalformedToolArgsAreErrorsNotZeroValues(t *testing.T) {
	s := newCounterHandler(t)
	moved := 0
	s.SetWindowControl(func(string, int, int, int, int) { moved++ }, nil, nil, nil)

	rawCall := func(name, arguments string) map[string]any {
		t.Helper()
		return rpc(t, s, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
			name, arguments))
	}

	for name, arguments := range map[string]string{
		"qorm_window":          `{"op":"move","x":"NaN"}`, // wrong field type
		"qorm_dispatch":        `"increment"`,             // not an object
		"qorm_simulate_action": `[1,2]`,
		"qorm_assert":          `{"checks":"stateEquals"}`,
		"qorm_check_layout":    `{"viewportW":"wide"}`,
		"qorm_query":           `{"type":42}`,
		"qorm_get_node":        `{"id":{}}`,
		"qorm_source_location": `{"id":[]}`,
	} {
		requireToolErr(t, rawCall(name, arguments), "invalid arguments")
	}
	if moved != 0 {
		t.Errorf("malformed qorm_window args must not move the window, got %d moves", moved)
	}

	// Absent and null arguments stay accepted as all-defaults: the empty
	// window call still means "move main to (0,0)" by contract.
	if text := resultText(t, rawCall("qorm_window", "null")); !strings.Contains(text, "moved window main") {
		t.Errorf("null arguments should keep the documented default, got %q", text)
	}
	if moved != 1 {
		t.Errorf("null-args window call should move once, got %d", moved)
	}
}

// ---- inspect family ----

func TestRenderHTML(t *testing.T) {
	s := newCounterHandler(t)
	html := resultText(t, toolCallRPC(t, s, "qorm_render_html", map[string]any{}))
	if !strings.Contains(html, "COUNTER") {
		t.Errorf("render_html should show the title text, got %.200s", html)
	}
}

func TestA11yTreeAuditsMissingNames(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Children: []*model.Node{
			{Type: "button", ID: "nameless_btn"},
			{Type: "button", ID: "ok_btn", Label: "OK"},
		},
	}}}
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{}}

	tree := resultObj(t, toolCallRPC(t, s, "qorm_a11y_tree", map[string]any{}))
	counts := tree["counts"].(map[string]any)
	if counts["interactive"].(float64) != 2 {
		t.Errorf("interactive count = %v, want 2", counts["interactive"])
	}
	issues := tree["issues"].([]any)
	if len(issues) != 1 {
		t.Fatalf("exactly one a11y issue expected, got %v", issues)
	}
	issue := issues[0].(map[string]any)
	if issue["nodeId"] != "nameless_btn" || issue["type"] != "missing-name" {
		t.Errorf("issue = %v, want missing-name on nameless_btn", issue)
	}
}

// TestInspectSurfacesDesignTokens asserts inspect exposes the design-token
// system (so the agent can learn the allowed values) only when declared.
func TestInspectSurfacesDesignTokens(t *testing.T) {
	s := &Server{rt: qrt.New(tokenApp()), mu: &sync.Mutex{}}
	insp := resultObj(t, toolCallRPC(t, s, "qorm_inspect", map[string]any{}))
	tokens, ok := insp["designTokens"].(map[string]any)
	if !ok {
		t.Fatalf("inspect must surface declared designTokens, got %v", insp)
	}
	primary, ok := tokens["color.primary"].(map[string]any)
	if !ok || primary["value"] != "#0a84ff" || primary["enforce"] != true {
		t.Errorf("color.primary token = %v", tokens["color.primary"])
	}

	// An app without tokens must not carry the field at all.
	s2 := newCounterHandler(t)
	insp2 := resultObj(t, toolCallRPC(t, s2, "qorm_inspect", map[string]any{}))
	if _, has := insp2["designTokens"]; has {
		t.Error("inspect must omit designTokens when the app declares none")
	}
}

func TestCapabilities(t *testing.T) {
	s := newCounterHandler(t)
	caps := resultArr(t, toolCallRPC(t, s, "qorm_capabilities", map[string]any{}))
	if len(caps) == 0 {
		t.Fatal("capabilities must not be empty")
	}
	var camera map[string]any
	for _, c := range caps {
		if cm := c.(map[string]any); cm["Stem"] == "camera" {
			camera = cm
		}
	}
	if camera == nil {
		t.Fatal("camera capability missing from registry listing")
	}
	var hasWeb bool
	for _, p := range camera["Platforms"].([]any) {
		if p == "web" {
			hasWeb = true
		}
	}
	if !hasWeb {
		t.Errorf("camera should list the web platform, got %v", camera["Platforms"])
	}
}

func TestListActions(t *testing.T) {
	s := newCounterHandler(t)
	actions := resultArr(t, toolCallRPC(t, s, "qorm_list_actions", map[string]any{}))
	if len(actions) != 4 {
		t.Fatalf("counter has 4 actions, got %d: %v", len(actions), actions)
	}
	first := actions[0].(map[string]any)
	if first["id"] != "decrement" {
		t.Errorf("actions must be sorted; first = %v", first["id"])
	}
	steps := first["steps"].([]any)
	if len(steps) != 1 || steps[0] != "state.set count = {{ count - 1 }}" {
		t.Errorf("decrement step summary = %v", steps)
	}
}

func TestActivityProvider(t *testing.T) {
	s := newCounterHandler(t)

	requireToolErr(t, toolCallRPC(t, s, "qorm_activity", map[string]any{}),
		"activity log unavailable")

	const log = `{"events":[{"who":"human","did":"tap"}]}`
	s.SetActivityProvider(func() string { return log })
	got := resultText(t, toolCallRPC(t, s, "qorm_activity", map[string]any{}))
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("activity payload is not an object: %q (%v)", got, err)
	}
	if len(payload["events"].([]any)) != 1 {
		t.Errorf("the host's own payload must survive: %q", got)
	}
	// The quiescence signal: an agent reads one tool to learn both what
	// happened and whether the app has finished reacting to it.
	if payload["inflight"] != float64(0) {
		t.Errorf("a settled runtime must report inflight 0: %q", got)
	}
}

// TestActivityInflightSurfacesOpenWork: the count is the runtime's, not a
// constant — an agent that dispatched an async action must be able to see that
// the reply has not landed yet, instead of reasoning about a loading frame as
// if it were the final state.
func TestActivityInflightSurfacesOpenWork(t *testing.T) {
	s := newCounterHandler(t)
	s.SetActivityProvider(func() string { return `{"events":[]}` })

	// A host sink that never resumes: the request stays open, which is exactly
	// the state an agent needs to be able to observe.
	s.rt.Async = func(func() any, func(any)) {}
	s.rt.App.Actions["fetch"] = &model.Action{ID: "fetch", Steps: []model.Step{
		{Type: "http.get", URL: "http://127.0.0.1:1/never", Async: true, Result: "resp"},
	}}
	s.rt.Dispatch("fetch", nil)

	got := resultText(t, toolCallRPC(t, s, "qorm_activity", map[string]any{}))
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("activity payload: %q (%v)", got, err)
	}
	if payload["inflight"] != float64(1) {
		t.Errorf("an open async request must show as inflight: %q", got)
	}
}

// TestActivityNonObjectPayloadPassesThrough: a host whose activity log is not
// a JSON object gets its payload back verbatim rather than mangled — losing the
// log to bolt a counter onto it would be the wrong trade.
func TestActivityNonObjectPayloadPassesThrough(t *testing.T) {
	s := newCounterHandler(t)
	for _, payload := range []string{`["not","an","object"]`, `not json at all`, `null`} {
		s.SetActivityProvider(func() string { return payload })
		if got := resultText(t, toolCallRPC(t, s, "qorm_activity", map[string]any{})); got != payload {
			t.Errorf("payload %q came back as %q", payload, got)
		}
	}
}

func TestExportSceneAndBundle(t *testing.T) {
	s := newCounterHandler(t)

	scene := resultObj(t, toolCallRPC(t, s, "qorm_export_scene", map[string]any{}))
	if scene["id"] != "main" || scene["type"] != "scene" {
		t.Errorf("export_scene header = id:%v type:%v", scene["id"], scene["type"])
	}
	root := scene["root"].(map[string]any)
	if root["type"] != "column" || root["id"] != "root" {
		t.Errorf("export_scene root = %v", root)
	}

	bun := resultObj(t, toolCallRPC(t, s, "qorm_export_bundle", map[string]any{}))
	if h, _ := bun["contentHash"].(string); h == "" {
		t.Error("export_bundle must carry a content hash")
	}
	content := bun["content"].(map[string]any)
	if id := content["app"].(map[string]any)["id"]; id != "qorm_counter" {
		t.Errorf("bundle app id = %v, want qorm_counter", id)
	}
	if _, signed := bun["signature"]; signed {
		t.Error("agent-exported bundles must be unsigned (a human signs them)")
	}
}

// TestExportBundleSurfacesSerializationError asserts an app whose node props
// cannot be JSON-encoded reports a tool error instead of crashing.
func TestExportBundleSurfacesSerializationError(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Props: map[string]any{"bad": make(chan int)},
	}}}
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{}}
	requireToolErr(t, toolCallRPC(t, s, "qorm_export_bundle", map[string]any{}), "unsupported type")
}

func TestGetNode(t *testing.T) {
	s := newCounterHandler(t)

	// A button exposes label, onPress and style.
	btn := resultObj(t, toolCallRPC(t, s, "qorm_get_node", map[string]any{"id": "btn_increment"}))
	if btn["type"] != "button" || btn["label"] != "Plus (+)" {
		t.Errorf("btn_increment = type:%v label:%v", btn["type"], btn["label"])
	}
	onPress := btn["onPress"].(map[string]any)
	if onPress["action"] != "increment" {
		t.Errorf("btn_increment onPress = %v", onPress)
	}

	// A container lists its child ids and exposes its style.
	root := resultObj(t, toolCallRPC(t, s, "qorm_get_node", map[string]any{"id": "root"}))
	kids := root["children"].([]any)
	if len(kids) != 4 || kids[0] != "title" || kids[2] != "controls" || kids[3] != "btn_theme" {
		t.Errorf("root children = %v", kids)
	}
	if bg := root["style"].(map[string]any)["background"]; bg != "var(--background)" {
		t.Errorf("root style.background = %v", bg)
	}

	// A text node exposes text and has no onPress.
	title := resultObj(t, toolCallRPC(t, s, "qorm_get_node", map[string]any{"id": "title"}))
	if title["text"] != "COUNTER" {
		t.Errorf("title text = %v", title["text"])
	}
	if _, has := title["onPress"]; has {
		t.Error("title must not report an onPress")
	}

	// Unknown id is an error.
	requireToolErr(t, toolCallRPC(t, s, "qorm_get_node", map[string]any{"id": "ghost"}), "not found")
}

func TestSourceLocation(t *testing.T) {
	s := newCounterHandler(t)

	loc := resultObj(t, toolCallRPC(t, s, "qorm_source_location", map[string]any{"id": "title"}))
	if loc["file"] != "scenes/main.json" {
		t.Errorf("source file = %v, want scenes/main.json", loc["file"])
	}
	if line, _ := loc["line"].(float64); line < 1 {
		t.Errorf("source line = %v, want >= 1", loc["line"])
	}
	if snip, _ := loc["snippet"].(string); !strings.Contains(snip, "title") {
		t.Errorf("snippet %q should show the declaration line", snip)
	}

	requireToolErr(t, toolCallRPC(t, s, "qorm_source_location", map[string]any{}), "id is required")
	requireToolErr(t, toolCallRPC(t, s, "qorm_source_location", map[string]any{"id": "definitely_not_here"}),
		"not declared literally")
}

func TestSourceLocationNoSourceTree(t *testing.T) {
	// An app without a base dir (a bundle) has no source tree to search.
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}}
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{}}
	requireToolErr(t, toolCallRPC(t, s, "qorm_source_location", map[string]any{"id": "root"}),
		"no source tree")
}

func TestFindNodeReachesTemplateAndWhenBranches(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Children: []*model.Node{
			{Type: "list", ID: "my_list", Template: &model.Node{Type: "text", ID: "tpl_item"}},
			{Type: "when", ID: "cond",
				Then: &model.Node{Type: "text", ID: "then_item"},
				Else: &model.Node{Type: "text", ID: "else_item"}},
		},
	}}}
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{}}
	for _, id := range []string{"tpl_item", "then_item", "else_item"} {
		n := resultObj(t, toolCallRPC(t, s, "qorm_get_node", map[string]any{"id": id}))
		if n["id"] != id {
			t.Errorf("get_node(%s) = %v; template/when branches must be reachable", id, n)
		}
	}
}

func TestQueryTool(t *testing.T) {
	s := newCounterHandler(t)

	q := resultObj(t, toolCallRPC(t, s, "qorm_query", map[string]any{"type": "button"}))
	if q["count"].(float64) != 3 {
		t.Errorf("button query count = %v, want 3", q["count"])
	}
	var sawIncrement bool
	for _, m := range q["matches"].([]any) {
		if m.(map[string]any)["id"] == "btn_increment" {
			sawIncrement = true
		}
	}
	if !sawIncrement {
		t.Errorf("button query should match btn_increment: %v", q["matches"])
	}

	// textContains is case-insensitive against the title text.
	q = resultObj(t, toolCallRPC(t, s, "qorm_query", map[string]any{"textContains": "counter"}))
	if q["count"].(float64) != 1 {
		t.Errorf("text query count = %v, want 1 (the title)", q["count"])
	}

	// idContains matches all three buttons and rejects everything else.
	q = resultObj(t, toolCallRPC(t, s, "qorm_query", map[string]any{"idContains": "btn"}))
	if q["count"].(float64) != 3 {
		t.Errorf("idContains query count = %v, want 3", q["count"])
	}
}

// TestQueryWalksTemplates asserts the query descends into a list's item
// template, so templated nodes are findable before patching.
func TestQueryWalksTemplates(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Children: []*model.Node{
			{Type: "list", ID: "my_list", Template: &model.Node{Type: "text", ID: "tpl_item"}},
		},
	}}}
	s := &Server{rt: qrt.New(app), mu: &sync.Mutex{}}
	q := resultObj(t, toolCallRPC(t, s, "qorm_query", map[string]any{"idContains": "tpl_item"}))
	if q["count"].(float64) != 1 {
		t.Errorf("query must find templated nodes, got %v", q)
	}
}

// ---- operate / test error paths ----

func TestOperateAndSimulateErrorPaths(t *testing.T) {
	s := newCounterHandler(t)

	// simulate with an unknown action reports the error inside the result.
	sim := resultObj(t, toolCallRPC(t, s, "qorm_simulate_action", map[string]any{"action": "nope"}))
	if e, _ := sim["error"].(string); !strings.Contains(e, "unknown action") {
		t.Errorf("simulate unknown action = %v", sim)
	}
	// simulate must not have mutated live state.
	if got := fmt.Sprint(s.rt.State["count"]); got != "0" {
		t.Errorf("simulate must be side-effect-free; count=%v", got)
	}

	// dispatch with an unknown action is a tool error.
	requireToolErr(t, toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "nope"}),
		`unknown action "nope"`)

	// set_state without a path is rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_set_state", map[string]any{"value": 3}),
		"set_state requires path and value")

	// An unknown tool name is reported, not crashed on.
	requireToolErr(t, toolCallRPC(t, s, "qorm_frobnicate", map[string]any{}), "unknown tool")
}

func TestAssertKinds(t *testing.T) {
	s := newCounterHandler(t)

	passing := []any{
		map[string]any{"kind": "stateEquals", "path": "count", "value": 0},
		map[string]any{"kind": "htmlContains", "text": "COUNTER"},
		map[string]any{"kind": "nodeExists", "id": "btn_increment"},
	}
	res := resultObj(t, toolCallRPC(t, s, "qorm_assert", map[string]any{"checks": passing}))
	if res["pass"] != true {
		t.Errorf("all checks should pass: %v", res["checks"])
	}

	failing := []any{
		map[string]any{"kind": "stateEquals", "path": "count", "value": 99},
		map[string]any{"kind": "htmlContains", "text": "ZZZ_NEVER_RENDERED"},
		map[string]any{"kind": "nodeExists", "id": "ghost"},
		map[string]any{"kind": "bogusKind"},
	}
	res = resultObj(t, toolCallRPC(t, s, "qorm_assert", map[string]any{"checks": failing}))
	if res["pass"] != false {
		t.Error("mixed failing checks must report pass=false")
	}
	checks := res["checks"].([]any)
	if len(checks) != 4 {
		t.Fatalf("expected 4 check results, got %d", len(checks))
	}
	for i, c := range checks {
		if c.(map[string]any)["pass"] != false {
			t.Errorf("check %d should fail: %v", i, c)
		}
	}
	if detail := checks[3].(map[string]any)["detail"].(string); !strings.Contains(detail, "unknown check kind") {
		t.Errorf("unknown-kind detail = %q", detail)
	}
}

// ---- measure / check_layout ----

func TestMeasureAndCheckLayout(t *testing.T) {
	s := newCounterHandler(t)

	// Unwired server: both tools report measurement unavailable.
	requireToolErr(t, toolCallRPC(t, s, "qorm_measure", map[string]any{}), "measurement unavailable")
	requireToolErr(t, toolCallRPC(t, s, "qorm_check_layout", map[string]any{"checks": []any{}}),
		"measurement unavailable")

	// Wired but the client has not measured yet (empty payload).
	s.SetMeasureProvider(func() []byte { return []byte("[]") })
	requireToolErr(t, toolCallRPC(t, s, "qorm_measure", map[string]any{}), "no measurement yet")
	requireToolErr(t, toolCallRPC(t, s, "qorm_check_layout", map[string]any{"checks": []any{}}),
		"no measurement yet")

	// A real self-measurement payload.
	payload, _ := json.Marshal([]map[string]any{
		{"id": "btn_increment", "x": 100.0, "y": 200.0, "w": 60.0, "h": 60.0, "visible": true},
	})
	s.SetMeasureProvider(func() []byte { return payload })

	rep := resultObj(t, toolCallRPC(t, s, "qorm_measure", map[string]any{}))
	if rep["components"].(float64) != 1 {
		t.Errorf("measured components = %v, want 1", rep["components"])
	}
	row := rep["measured"].([]any)[0].(map[string]any)
	if row["type"] != "button" {
		t.Errorf("report should join the node's expressed type, got %v", row["type"])
	}

	// check_layout: passing assertions, plus viewport simulation.
	checks := []any{map[string]any{"id": "btn_increment", "visible": true, "minW": 50.0, "maxW": 100.0}}
	res := resultObj(t, toolCallRPC(t, s, "qorm_check_layout",
		map[string]any{"checks": checks, "viewportW": 320, "viewportH": 568}))
	if res["ok"] != true {
		t.Errorf("passing layout checks should report ok=true: %v", res)
	}
	if s.rt.Viewport.W != 320 || s.rt.Viewport.H != 568 {
		t.Errorf("check_layout should simulate the requested viewport, got %+v", s.rt.Viewport)
	}

	// A violated bound fails.
	res = resultObj(t, toolCallRPC(t, s, "qorm_check_layout",
		map[string]any{"checks": []any{map[string]any{"id": "btn_increment", "maxW": 10.0}}}))
	if res["ok"] != false {
		t.Errorf("violated maxW must fail: %v", res)
	}

	// An id that was never measured fails loud, not silent.
	res = resultObj(t, toolCallRPC(t, s, "qorm_check_layout",
		map[string]any{"checks": []any{map[string]any{"id": "ghost_node", "visible": true}}}))
	if res["ok"] != false {
		t.Errorf("unmeasured id must fail: %v", res)
	}

	// Malformed checks JSON is a tool error.
	requireToolErr(t, toolCallRPC(t, s, "qorm_check_layout", map[string]any{"checks": "not-an-array"}),
		"bad checks")
}

// ---- design: diff / preview / apply / undo over the wire ----

func TestDiffTool(t *testing.T) {
	s := newCounterHandler(t)

	ops := []any{map[string]any{"op": "setProp", "target": "title", "key": "text", "value": "NEW"}}
	d := resultObj(t, toolCallRPC(t, s, "qorm_diff", map[string]any{"ops": ops}))
	if d["summary"] != "0 added, 0 removed, 1 changed" {
		t.Errorf("diff summary = %v", d["summary"])
	}
	changed := d["changed"].([]any)
	if changed[0].(map[string]any)["id"] != "title" {
		t.Errorf("diff changed = %v", changed)
	}
	// diff must not touch the live app.
	if got := findInScenes(s.rt.App, "title").Text; got != "COUNTER" {
		t.Errorf("diff must be side-effect-free; title=%q", got)
	}

	// Invalid ops JSON.
	requireToolErr(t, toolCallRPC(t, s, "qorm_diff", map[string]any{"ops": "junk"}), "invalid ops")
	// A patch that cannot apply surfaces its error.
	requireToolErr(t, toolCallRPC(t, s, "qorm_diff",
		map[string]any{"ops": []any{map[string]any{"op": "remove", "target": "ghost"}}}), "not found")
}

func TestPreviewErrorPaths(t *testing.T) {
	s := newCounterHandler(t)

	requireToolErr(t, toolCallRPC(t, s, "qorm_preview_patch", map[string]any{"ops": 7}), "invalid ops")

	// Failing ops report ok:false + error, leaving the live app untouched.
	p := resultObj(t, toolCallRPC(t, s, "qorm_preview_patch",
		map[string]any{"ops": []any{map[string]any{"op": "remove", "target": "ghost"}}}))
	if p["ok"] != false {
		t.Fatalf("preview of a bad op must fail, got %v", p)
	}
	if e, _ := p["error"].(string); !strings.Contains(e, "not found") {
		t.Errorf("preview error = %q", e)
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "COUNTER" {
		t.Errorf("failed preview must not touch the live app; title=%q", got)
	}
}

func TestApplyPatchTokenBinding(t *testing.T) {
	s := newCounterHandler(t)
	opsA := []any{map[string]any{"op": "setProp", "target": "title", "key": "text", "value": "A"}}
	opsB := []any{map[string]any{"op": "setProp", "target": "title", "key": "text", "value": "B"}}
	tokA := patchToken([]PatchOp{{Op: "setProp", Target: "title", Key: "text", Value: "A"}})

	// Preview A establishes the binding and returns the deterministic token.
	prev := resultObj(t, toolCallRPC(t, s, "qorm_preview_patch", map[string]any{"ops": opsA}))
	if prev["previewToken"] != tokA {
		t.Fatalf("previewToken = %v, want %v", prev["previewToken"], tokA)
	}

	// Wrong token for the right ops is rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_apply_patch",
		map[string]any{"ops": opsA, "previewToken": "deadbeefdeadbeef"}), "previewToken")
	if got := findInScenes(s.rt.App, "title").Text; got != "COUNTER" {
		t.Errorf("rejected apply must not touch the live app; title=%q", got)
	}

	// Right token but different ops than previewed is rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_apply_patch",
		map[string]any{"ops": opsB, "previewToken": tokA}), "previewToken")

	// Missing token is rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_apply_patch", map[string]any{"ops": opsA}), "previewToken")

	// Malformed ops JSON is rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_apply_patch",
		map[string]any{"ops": 7, "previewToken": tokA}), "invalid ops")

	// The bound apply commits and reports undo depth.
	res := resultObj(t, toolCallRPC(t, s, "qorm_apply_patch",
		map[string]any{"ops": opsA, "previewToken": tokA}))
	if res["ok"] != true || res["undoDepth"].(float64) != 1 {
		t.Fatalf("bound apply = %v", res)
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "A" {
		t.Errorf("apply should commit; title=%q", got)
	}

	// The token is single-use: once consumed, the same ops+token are rejected.
	requireToolErr(t, toolCallRPC(t, s, "qorm_apply_patch",
		map[string]any{"ops": opsA, "previewToken": tokA}), "previewToken")
}

func TestUndoOverRPC(t *testing.T) {
	s := newCounterHandler(t)
	ops := []PatchOp{{Op: "setProp", Target: "title", Key: "text", Value: "X"}}
	s.previewPatch(ops)
	if _, err := s.applyPatchTool(ops, patchToken(ops)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	res := resultObj(t, toolCallRPC(t, s, "qorm_undo", map[string]any{}))
	if res["ok"] != true || res["undoDepth"].(float64) != 0 {
		t.Fatalf("undo = %v", res)
	}
	if !strings.Contains(res["html"].(string), "COUNTER") {
		t.Error("undo should render the reverted app")
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "COUNTER" {
		t.Errorf("undo should restore the pre-image; title=%q", got)
	}

	// Nothing left to undo.
	requireToolErr(t, toolCallRPC(t, s, "qorm_undo", map[string]any{}), "nothing to undo")
}

func TestUndoHistoryCappedAt50(t *testing.T) {
	s := newCounterHandler(t)
	for i := 0; i < 55; i++ {
		ops := []PatchOp{{Op: "setProp", Target: "title", Key: "text", Value: fmt.Sprintf("v%d", i)}}
		s.previewPatch(ops)
		res, err := s.applyPatchTool(ops, patchToken(ops))
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		want := i + 1
		if want > maxHistory {
			want = maxHistory
		}
		if got := res["undoDepth"].(int); got != want {
			t.Fatalf("apply %d: undoDepth=%d, want %d", i, got, want)
		}
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "v54" {
		t.Fatalf("latest apply should win; title=%q", got)
	}
	if len(s.history) != maxHistory {
		t.Fatalf("history len = %d, want the %d cap", len(s.history), maxHistory)
	}

	// Exactly maxHistory undos succeed; the oldest retained pre-image is v4
	// (the first five, COUNTER..v3, were evicted by the cap).
	for i := 0; i < maxHistory; i++ {
		if _, err := s.undo(); err != nil {
			t.Fatalf("undo %d: %v", i, err)
		}
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "v4" {
		t.Errorf("after exhausting the capped history title should be v4 (oldest retained pre-image), got %q", got)
	}
	if _, err := s.undo(); err == nil {
		t.Error("undo past the exhausted history must fail")
	}
}

// ---- graceful degradation ----

func TestInspectDegradesOnUnserializableState(t *testing.T) {
	s := newCounterHandler(t)
	s.rt.State["bad"] = make(chan int) // json.Marshal cannot encode a channel
	m := toolCallRPC(t, s, "qorm_inspect", map[string]any{})
	if resultIsErr(m) {
		t.Fatal("inspect should degrade gracefully, not error")
	}
	text := resultText(t, m)
	if json.Valid([]byte(text)) {
		t.Fatalf("with an unserializable state value inspect should fall back to the %%v dump, got JSON: %.120s", text)
	}
	if !strings.HasPrefix(text, "map[") || !strings.Contains(text, "bad:") {
		t.Errorf("expected the Go %%v map dump, got %.120s", text)
	}
}

// TestDispatchReturnsTheSettledStateNotALoadingFrame pins the agent-side
// contract for async http steps. An agent reads qorm_dispatch's answer and acts
// on it, so answering with the loading frame the request was launched from
// would have it reason about a state the app is about to leave. The tool
// therefore detaches the host's background sink for the duration of the call,
// which makes the async step take its synchronous path — the settled result by
// definition, and without ever dropping the lock mid-call.
func TestDispatchReturnsTheSettledStateNotALoadingFrame(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fact":"cats"}`))
	}))
	defer backend.Close()

	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "as", "entry": "main", "globalState": map[string]any{
			"schema":  map[string]any{"loading": "bool", "resp": "object"},
			"initial": map[string]any{"loading": false, "resp": map[string]any{}}}},
		{"type": "scene", "id": "main", "root": map[string]any{
			"type": "text", "id": "t", "text": "{{ state.resp.fact }}"}},
		{"type": "action", "id": "go", "steps": []any{
			map[string]any{"type": "state.set", "path": "loading", "value": "{{ true }}"},
			map[string]any{"type": "http.get", "url": backend.URL, "async": true, "result": "resp",
				"onSuccess": []any{map[string]any{"type": "state.set", "path": "loading", "value": "{{ false }}"}}},
		}},
	})
	if len(app.Diagnostics) != 0 {
		t.Fatalf("fixture must load clean: %v", app.Diagnostics)
	}

	rt := qrt.New(app)
	// A host that would defer the work forever: if the tool used it, the reply
	// could only ever be the loading frame.
	spawned := 0
	rt.Async = func(func() any, func(any)) { spawned++ }
	s := &Server{rt: rt, mu: &sync.Mutex{}}

	text := resultText(t, toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "go"}))
	if spawned != 0 {
		t.Errorf("an agent dispatch must not hand work to the background sink: %d", spawned)
	}
	if !strings.Contains(text, "cats") {
		t.Errorf("the agent must receive the settled render, not the loading frame: %s", text)
	}
	if rt.State["loading"] != false {
		t.Errorf("the result branch must have run before the tool returned: %v", rt.State["loading"])
	}
	if rt.Inflight() != 0 {
		t.Errorf("nothing may be left in flight after the call: %d", rt.Inflight())
	}
	if rt.Async == nil {
		t.Error("the sink must be restored after the call — detaching it is scoped to the dispatch")
	}
}

// TestDispatchRunsAPacedActionToCompletion extends the same contract to the
// `delay` step. Detaching the background sink makes a paced action collapse to
// its final state instead of leaving the agent holding stage one and no way to
// know more is coming — and, just as importantly, without the tool call
// sleeping through the pauses while it holds the host lock.
func TestDispatchRunsAPacedActionToCompletion(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "pace", "entry": "main", "globalState": map[string]any{
			"schema": map[string]any{"phase": "string"}, "initial": map[string]any{"phase": "idle"}}},
		{"type": "scene", "id": "main", "root": map[string]any{
			"type": "text", "id": "t", "text": "phase={{ state.phase }}"}},
		{"type": "action", "id": "go", "steps": []any{
			map[string]any{"type": "state.set", "path": "phase", "value": "{{ 'one' }}"},
			map[string]any{"type": "delay", "ms": float64(60000)},
			map[string]any{"type": "state.set", "path": "phase", "value": "{{ 'two' }}"},
		}},
	})
	if len(app.Diagnostics) != 0 {
		t.Fatalf("fixture must load clean: %v", app.Diagnostics)
	}
	rt := qrt.New(app)
	spawned := 0
	rt.Async = func(func() any, func(any)) { spawned++ }
	s := &Server{rt: rt, mu: &sync.Mutex{}}

	text := resultText(t, toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "go"}))
	if spawned != 0 {
		t.Errorf("a minute-long pause must not be handed to the sink during an agent call: %d", spawned)
	}
	if !strings.Contains(text, "phase=two") {
		t.Errorf("the agent must receive the finished action: %s", text)
	}
	if rt.Inflight() != 0 {
		t.Errorf("nothing may be left waiting: %d", rt.Inflight())
	}
}
