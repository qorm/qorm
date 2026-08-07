package loader

import (
	"strings"
	"testing"
)

// Tests for loading the execution-model additions: `if` / `invoke` steps, the
// http.* onSuccess/onError branches, scene onEnter hooks, and the timer node's
// static checks.

func hasDiag(diags []string, frag string) bool {
	for _, d := range diags {
		if strings.Contains(d, frag) {
			return true
		}
	}
	return false
}

func execDocs(action map[string]any) []map[string]any {
	docs := []map[string]any{
		{"type": "app", "id": "x", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"count": "number"}, "initial": map[string]any{"count": float64(0)}}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	}
	if action != nil {
		docs = append(docs, action)
	}
	return docs
}

func TestLoadIfStep(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "check",
		"steps": []any{map[string]any{
			"type": "if", "condition": "{{ state.count > 0 }}",
			"then": []any{map[string]any{"type": "state.set", "path": "count", "value": "1"}},
			"else": []any{map[string]any{
				"type": "if", "condition": "{{ state.count < 0 }}",
				"then": []any{map[string]any{"type": "state.set", "path": "count", "value": "2"}},
			}},
		}},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("well-formed if steps must load clean: %v", app.Diagnostics)
	}
	st := app.Actions["check"].Steps[0]
	if st.Type != "if" || st.Condition != "{{ state.count > 0 }}" {
		t.Fatalf("if step lost fields: %+v", st)
	}
	if len(st.Then) != 1 || len(st.Else) != 1 || st.Else[0].Type != "if" || len(st.Else[0].Then) != 1 {
		t.Fatalf("nested branches lost: %+v", st)
	}
}

func TestLoadInvokeStep(t *testing.T) {
	docs := execDocs(map[string]any{
		"type": "action", "id": "outer",
		"steps": []any{map[string]any{
			"type": "invoke", "name": "inner",
			"args": map[string]any{"v": "{{ state.count }}"},
		}},
	})
	docs = append(docs, map[string]any{
		"type": "action", "id": "inner",
		"steps": []any{map[string]any{"type": "state.set", "path": "count", "value": "{{ v }}"}},
	})
	app := FromDocs(docs)
	if len(app.Diagnostics) != 0 {
		t.Fatalf("well-formed invoke must load clean: %v", app.Diagnostics)
	}
	st := app.Actions["outer"].Steps[0]
	if st.Name != "inner" || st.Args["v"] != "{{ state.count }}" {
		t.Fatalf("invoke step lost fields: %+v", st)
	}
}

func TestLoadHTTPBranches(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "sync",
		"steps": []any{map[string]any{
			"type": "http.get", "url": "https://api.example.com/items", "result": "items", "error": "err",
			"onSuccess": []any{map[string]any{"type": "state.set", "path": "phase", "value": "loaded"}},
			"onError":   []any{map[string]any{"type": "state.set", "path": "phase", "value": "{{ error }}"}},
		}},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("well-formed http branches must load clean: %v", app.Diagnostics)
	}
	st := app.Actions["sync"].Steps[0]
	if len(st.OnSuccess) != 1 || len(st.OnError) != 1 {
		t.Fatalf("http branches lost: %+v", st)
	}
}

func TestIfStepDiagnostics(t *testing.T) {
	// Missing condition.
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad",
		"steps": []any{map[string]any{"type": "if", "then": []any{}}},
	}))
	if !hasDiag(app.Diagnostics, "'if' 步骤缺少 condition") {
		t.Errorf("missing if condition must be diagnosed: %v", app.Diagnostics)
	}
	// A constant (unbound) condition is almost always a forgotten {{ }}.
	app = FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad2",
		"steps": []any{map[string]any{"type": "if", "condition": "state.count > 0"}},
	}))
	if !hasDiag(app.Diagnostics, "不含 {{...}} 绑定") {
		t.Errorf("constant if condition must warn: %v", app.Diagnostics)
	}
}

func TestInvokeStepDiagnostics(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad",
		"steps": []any{map[string]any{"type": "invoke"}},
	}))
	if !hasDiag(app.Diagnostics, "'invoke' 步骤缺少 name") {
		t.Errorf("nameless invoke must be diagnosed: %v", app.Diagnostics)
	}
	app = FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad2",
		"steps": []any{map[string]any{"type": "invoke", "name": "nowhere"}},
	}))
	if !hasDiag(app.Diagnostics, `引用了不存在的动作 "nowhere"`) {
		t.Errorf("unknown invoke target must be diagnosed: %v", app.Diagnostics)
	}
	// Nested invokes are checked too.
	app = FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad3",
		"steps": []any{map[string]any{
			"type": "if", "condition": "{{ state.count }}",
			"then": []any{map[string]any{"type": "invoke", "name": "ghost"}},
		}},
	}))
	if !hasDiag(app.Diagnostics, `引用了不存在的动作 "ghost"`) {
		t.Errorf("unknown invoke target inside a branch must be diagnosed: %v", app.Diagnostics)
	}
}

func TestStepNestingDepthGuard(t *testing.T) {
	inner := map[string]any{"type": "state.set", "path": "count", "value": "1"}
	step := inner
	for i := 0; i < maxStepNesting+4; i++ {
		step = map[string]any{"type": "if", "condition": "{{ state.count }}", "then": []any{step}}
	}
	app := FromDocs(execDocs(map[string]any{"type": "action", "id": "deep", "steps": []any{step}}))
	if !hasDiag(app.Diagnostics, "步骤嵌套超过") {
		t.Errorf("over-deep nesting must be diagnosed: %v", app.Diagnostics)
	}
}

func TestSceneOnEnterParses(t *testing.T) {
	// Map form.
	docs := []map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"},
			"onEnter": map[string]any{"name": "load", "args": map[string]any{"v": "1"}}},
		{"type": "action", "id": "load", "steps": []any{map[string]any{"type": "state.set", "path": "x", "value": "{{ v }}"}}},
	}
	app := FromDocs(docs)
	if len(app.Diagnostics) != 0 {
		t.Fatalf("well-formed onEnter must load clean: %v", app.Diagnostics)
	}
	inv := app.SceneEnter["main"]
	if inv == nil || inv.Name != "load" || inv.Args["v"] != "1" {
		t.Fatalf("onEnter invoke lost: %+v", inv)
	}
	// String shorthand.
	docs[1] = map[string]any{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}, "onEnter": "load"}
	app = FromDocs(docs)
	if inv := app.SceneEnter["main"]; inv == nil || inv.Name != "load" {
		t.Fatalf("string-shorthand onEnter lost: %+v", app.SceneEnter)
	}
}

func TestSceneOnEnterUnknownActionDiagnosed(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}, "onEnter": "missing"},
	}
	app := FromDocs(docs)
	if !hasDiag(app.Diagnostics, `onEnter 引用了不存在的动作 "missing"`) {
		t.Errorf("unknown onEnter action must be diagnosed: %v", app.Diagnostics)
	}
}

// ---- timer static checks ----------------------------------------------------

func timerDocs(timer map[string]any) []map[string]any {
	timer["type"] = "timer"
	return []map[string]any{
		{"type": "app", "id": "x", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"rate": "number"}, "initial": map[string]any{"rate": float64(1000)}}},
		{"type": "scene", "id": "main", "root": map[string]any{
			"type": "column", "id": "root", "children": []any{timer}}},
		{"type": "action", "id": "tick", "steps": []any{map[string]any{"type": "state.set", "path": "rate", "value": "{{ rate }}"}}},
	}
}

func TestTimerClean(t *testing.T) {
	app := FromDocs(timerDocs(map[string]any{"id": "poll", "every": float64(5000), "onTick": "tick"}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a well-formed timer must load clean: %v", app.Diagnostics)
	}
}

func TestTimerDiagnostics(t *testing.T) {
	cases := []struct {
		name  string
		timer map[string]any
		frag  string
	}{
		{"missing id", map[string]any{"every": float64(1000), "onTick": "tick"}, "timer 节点缺少 id"},
		{"missing schedule", map[string]any{"id": "t1", "onTick": "tick"}, "需要 every"},
		{"every too small", map[string]any{"id": "t2", "every": float64(8), "onTick": "tick"}, "低于 16ms"},
		{"missing onTick", map[string]any{"id": "t3", "every": float64(1000)}, "缺少 onTick"},
		{"unknown onTick action", map[string]any{"id": "t4", "every": float64(1000), "onTick": "ghost"}, `引用了不存在的动作 "ghost"`},
		{"unknown onTick invoke form", map[string]any{"id": "t5", "every": float64(1000), "onTick": map[string]any{"name": "ghost"}}, `引用了不存在的动作 "ghost"`},
		{"both every and after", map[string]any{"id": "t6", "every": float64(1000), "after": float64(500), "onTick": "tick"}, "every(周期触发)优先"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := FromDocs(timerDocs(c.timer))
			if !hasDiag(app.Diagnostics, c.frag) {
				t.Errorf("want a diagnostic containing %q, got: %v", c.frag, app.Diagnostics)
			}
		})
	}
}

func TestTimerDynamicIntervalNotWarned(t *testing.T) {
	// A bound interval is statically unknowable — no schedule/floor warnings.
	app := FromDocs(timerDocs(map[string]any{"id": "dyn", "every": "{{ state.rate }}", "onTick": "tick"}))
	if len(app.Diagnostics) != 0 {
		t.Errorf("a dynamic interval must not warn: %v", app.Diagnostics)
	}
}

func TestTimerInComponentChecked(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "x", "entry": "main", "components": map[string]any{
			"Poller": map[string]any{"type": "timer", "id": "cpoll", "every": float64(1000), "onTick": "ghost"},
		}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	}
	app := FromDocs(docs)
	if !hasDiag(app.Diagnostics, `引用了不存在的动作 "ghost"`) {
		t.Errorf("timers inside components must be checked: %v", app.Diagnostics)
	}
}

// ---- render step -------------------------------------------------------------

// TestLoadRenderStep: `render` carries no fields of its own, so it must survive
// the load (and a serialise round-trip) as a bare typed step, warning-free.
func TestLoadRenderStep(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "save",
		"steps": []any{
			map[string]any{"type": "state.set", "path": "count", "value": "1"},
			map[string]any{"type": "render"},
			map[string]any{"type": "state.set", "path": "count", "value": "2"},
		},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a render step must load clean: %v", app.Diagnostics)
	}
	steps := app.Actions["save"].Steps
	if len(steps) != 3 || steps[1].Type != "render" {
		t.Fatalf("render step lost on load: %+v", steps)
	}
}

// TestRenderStepFloodWarning: every `render` publishes a live-sync frame, and a
// subscriber whose buffer fills simply drops them — so an action packed with
// renders is an authoring mistake worth flagging. The count walks branches too,
// since that is where a loop-shaped action hides them.
func TestRenderStepFloodWarning(t *testing.T) {
	branch := make([]any, 0, maxRenderSteps)
	for i := 0; i < maxRenderSteps; i++ {
		branch = append(branch, map[string]any{"type": "render"})
	}
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "flood",
		"steps": []any{
			map[string]any{"type": "render"},
			map[string]any{
				"type": "if", "condition": "{{ state.count > 0 }}",
				"then": branch,
			},
		},
	}))
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "'render'") && strings.HasPrefix(d, "warning:") {
			found = true
		}
	}
	if !found {
		t.Errorf("more than %d render steps must warn: %v", maxRenderSteps, app.Diagnostics)
	}

	// Exactly at the ceiling is fine — the warning is for going past it.
	ok := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "fine", "steps": branch,
	}))
	if len(ok.Diagnostics) != 0 {
		t.Errorf("%d render steps must load clean: %v", maxRenderSteps, ok.Diagnostics)
	}
}

// ---- async http step ---------------------------------------------------------

// TestLoadAsyncHTTPStep: `"async": true` is a plain boolean on an http step —
// it must survive the load, and its absence must stay false (the default that
// keeps every existing app blocking exactly as it did).
func TestLoadAsyncHTTPStep(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "fetch",
		"steps": []any{
			map[string]any{"type": "http.get", "url": "https://example.com", "async": true, "result": "resp",
				"onSuccess": []any{map[string]any{"type": "state.set", "path": "count", "value": "1"}}},
			map[string]any{"type": "http.get", "url": "https://example.com", "result": "resp2"},
		},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("an async http step must load clean: %v", app.Diagnostics)
	}
	steps := app.Actions["fetch"].Steps
	if !steps[0].Async {
		t.Error("\"async\": true must reach the step")
	}
	if steps[1].Async {
		t.Error("a step without \"async\" must stay synchronous — that is the default the whole design rests on")
	}
}

// TestAsyncOnNonHTTPStepWarns: only a backend call has a round trip to move to
// the background. Anywhere else the field is inert, which reads like a promise
// the runtime never keeps — so the loader says so rather than ignoring it.
func TestAsyncOnNonHTTPStepWarns(t *testing.T) {
	app := FromDocs(execDocs(map[string]any{
		"type": "action", "id": "bad",
		"steps": []any{map[string]any{"type": "state.set", "path": "count", "value": "1", "async": true}},
	}))
	if !hasDiag(app.Diagnostics, `只有 'http.*' 步骤支持`) {
		t.Errorf("async on a non-http step must warn: %v", app.Diagnostics)
	}
	for _, d := range app.Diagnostics {
		if strings.HasPrefix(d, "error:") {
			t.Errorf("it is a warning, not an error — the step still runs: %v", d)
		}
	}
}
