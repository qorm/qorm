package loader

// Loading + diagnostics for the three declarative additions:
//
//	computed  — derived, memoised, read-only values in the manifest
//	guard     — a scene's route precondition + redirect
//	forEach   — a bulk step that runs its body once per element
//
// Each block covers the happy path (fields survive the parse, zero
// diagnostics) and the mistakes an author or an agent actually makes, since a
// diagnostic that never fires is indistinguishable from one that does not
// exist.

import (
	"reflect"
	"testing"
)

// ---- computed ---------------------------------------------------------------

// computedDocs builds an app whose manifest carries the given "computed"
// object at the manifest's top level.
func computedDocs(computed map[string]any, extra ...map[string]any) []map[string]any {
	manifest := map[string]any{
		"type": "app", "id": "x", "entry": "main",
		"globalState": map[string]any{
			"schema":  map[string]any{"items": "array", "count": "number"},
			"initial": map[string]any{"items": []any{}, "count": float64(0)},
		},
	}
	if computed != nil {
		manifest["computed"] = computed
	}
	docs := []map[string]any{
		manifest,
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	}
	return append(docs, extra...)
}

func TestLoadComputed(t *testing.T) {
	app := FromDocs(computedDocs(map[string]any{
		"total":   `{{ sum(map(state.items, "it.price * it.qty")) }}`,
		"isEmpty": "{{ len(state.items) == 0 }}",
		"label":   "{{ computed.isEmpty ? 'empty' : 'has items' }}",
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("well-formed computed declarations must load clean: %v", app.Diagnostics)
	}
	if len(app.Computed) != 3 {
		t.Fatalf("Computed = %#v, want three entries", app.Computed)
	}
	if app.Computed["isEmpty"] != "{{ len(state.items) == 0 }}" {
		t.Errorf("expression lost: %q", app.Computed["isEmpty"])
	}
	order, cyclic := app.ComputedOrder()
	if len(cyclic) != 0 {
		t.Errorf("cyclic = %v, want none", cyclic)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if len(pos) != 3 || pos["isEmpty"] > pos["label"] {
		t.Errorf("order = %v, want isEmpty evaluated before the label that reads it", order)
	}
}

func TestLoadComputedNestedUnderGlobalState(t *testing.T) {
	// Declaring derived values beside the state they derive from is accepted
	// too; both spellings fill the same map.
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "globalState": map[string]any{
			"schema":   map[string]any{"count": "number"},
			"initial":  map[string]any{"count": float64(0)},
			"computed": map[string]any{"doubled": "{{ state.count * 2 }}"},
		}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	})
	if len(app.Diagnostics) != 0 {
		t.Fatalf("globalState.computed must load clean: %v", app.Diagnostics)
	}
	if app.Computed["doubled"] != "{{ state.count * 2 }}" {
		t.Errorf("Computed = %#v", app.Computed)
	}
}

func TestComputedDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		docs []map[string]any
		want string
	}{
		{
			name: "cycle",
			docs: computedDocs(map[string]any{
				"a": "{{ computed.b + 1 }}",
				"b": "{{ computed.a + 1 }}",
			}),
			want: "循环依赖",
		},
		{
			name: "self reference is a cycle too",
			docs: computedDocs(map[string]any{"loop": "{{ computed.loop }}"}),
			want: "循环依赖",
		},
		{
			name: "empty expression",
			docs: computedDocs(map[string]any{"nothing": "  "}),
			want: "的表达式为空",
		},
		{
			name: "missing binding delimiters",
			docs: computedDocs(map[string]any{"oops": "state.count * 2"}),
			want: "不含 {{...}} 绑定",
		},
		{
			name: "name is not an identifier",
			docs: computedDocs(map[string]any{"a.b": "{{ 1 }}"}),
			want: "不是普通标识符",
		},
		{
			name: "type mismatch inside the expression",
			docs: computedDocs(map[string]any{"bad": "{{ state.items - 1 }}"}),
			want: "type mismatch",
		},
		{
			name: "declaration is not an object",
			docs: computedDocs(nil, map[string]any{"type": "unused"})[:1],
			want: "",
		},
	}
	for _, tt := range tests {
		if tt.want == "" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			app := FromDocs(tt.docs)
			if !hasDiag(app.Diagnostics, tt.want) {
				t.Errorf("want a diagnostic containing %q, got %v", tt.want, app.Diagnostics)
			}
		})
	}
}

func TestComputedNonObjectDeclaration(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "computed": "总额"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	})
	if !hasDiag(app.Diagnostics, "应为对象") {
		t.Errorf("a non-object computed declaration must be diagnosed: %v", app.Diagnostics)
	}
	if len(app.Computed) != 0 {
		t.Errorf("Computed = %#v, want nothing kept", app.Computed)
	}
}

func TestComputedDuplicateAcrossSpellings(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main",
			"globalState": map[string]any{"computed": map[string]any{"n": "{{ 1 }}"}},
			"computed":    map[string]any{"n": "{{ 2 }}"}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	})
	if !hasDiag(app.Diagnostics, "重复声明") {
		t.Errorf("a name declared in both spellings must be diagnosed: %v", app.Diagnostics)
	}
	if app.Computed["n"] != "{{ 1 }}" {
		t.Errorf("the first declaration must win, got %q", app.Computed["n"])
	}
}

func TestComputedNamespaceCollidesWithAStateKey(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main",
			"globalState": map[string]any{
				"schema":  map[string]any{"computed": "string"},
				"initial": map[string]any{"computed": "mine"},
			},
			"computed": map[string]any{"n": "{{ 1 }}"}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
	})
	if !hasDiag(app.Diagnostics, "globalState.schema 声明了") {
		t.Errorf("a schema key colliding with the reserved namespace must be diagnosed: %v", app.Diagnostics)
	}
	if !hasDiag(app.Diagnostics, "globalState.initial 提供了") {
		t.Errorf("an initial value colliding with the reserved namespace must be diagnosed: %v", app.Diagnostics)
	}
}

func TestComputedWritesAreDiagnosed(t *testing.T) {
	app := FromDocs(computedDocs(
		map[string]any{"total": "{{ state.count * 2 }}"},
		map[string]any{"type": "action", "id": "cheat", "steps": []any{
			map[string]any{"type": "state.set", "path": "computed.total", "value": "{{ 1 }}"},
			// Nested inside a branch AND a loop body: the check must descend
			// into every nested step list, not just the top level.
			map[string]any{"type": "if", "condition": "{{ true }}", "then": []any{
				map[string]any{"type": "forEach", "in": "{{ state.items }}", "steps": []any{
					map[string]any{"type": "http.get", "url": "https://x.test", "result": "computed.total"},
				}},
			}},
			map[string]any{"type": "http.get", "url": "https://x.test", "path": "resp", "error": "computed.err"},
		}}))
	n := 0
	for _, d := range app.Diagnostics {
		if contains(d, "写入了派生值路径") {
			n++
		}
	}
	if n != 3 {
		t.Errorf("want 3 read-only-write diagnostics (path, nested result, error), got %d in %v", n, app.Diagnostics)
	}
}

func TestComputedWriteIsOnlyDiagnosedWhenDeclared(t *testing.T) {
	// An app that declares nothing may legitimately own a state key called
	// "computed" — writing it must stay silent.
	app := FromDocs(computedDocs(nil,
		map[string]any{"type": "action", "id": "w", "steps": []any{
			map[string]any{"type": "state.set", "path": "computed.mine", "value": "{{ 1 }}"},
		}}))
	if hasDiag(app.Diagnostics, "写入了派生值路径") {
		t.Errorf("no declarations: writing 'computed' must not be diagnosed: %v", app.Diagnostics)
	}
}

// ---- guard ------------------------------------------------------------------

// guardDocs builds a two-scene app whose "secure" scene carries the given guard.
func guardDocs(guard any) []map[string]any {
	secure := map[string]any{"type": "scene", "id": "secure",
		"root": map[string]any{"type": "view", "id": "secure_root"}}
	if guard != nil {
		secure["guard"] = guard
	}
	return []map[string]any{
		{"type": "app", "id": "x", "entry": "main",
			"globalState": map[string]any{
				"schema":  map[string]any{"user": "string", "count": "number"},
				"initial": map[string]any{"user": "", "count": float64(0)},
			}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "view", "id": "root"}},
		secure,
	}
}

func TestLoadSceneGuard(t *testing.T) {
	app := FromDocs(guardDocs(map[string]any{
		"condition": "{{ state.user != '' }}",
		"redirect":  "main",
		"params":    map[string]any{"next": "{{ 'secure' }}"},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a well-formed guard must load clean: %v", app.Diagnostics)
	}
	g := app.SceneGuards["secure"]
	if g == nil {
		t.Fatal("guard not loaded")
	}
	if g.Condition != "{{ state.user != '' }}" || g.Redirect != "main" {
		t.Errorf("guard fields lost: %+v", g)
	}
	if !reflect.DeepEqual(g.Params, map[string]string{"next": "{{ 'secure' }}"}) {
		t.Errorf("guard params = %#v", g.Params)
	}
	if len(app.SceneGuards) != 1 {
		t.Errorf("an unguarded scene must not gain a guard: %#v", app.SceneGuards)
	}
}

func TestGuardDiagnostics(t *testing.T) {
	tests := []struct {
		name  string
		guard any
		want  string
	}{
		{"not an object", "state.user", "guard 应为对象"},
		{"missing condition", map[string]any{"redirect": "main"}, "guard 缺少 condition"},
		{"constant condition", map[string]any{"condition": "state.user", "redirect": "main"}, "不含 {{...}} 绑定"},
		{"unknown redirect", map[string]any{"condition": "{{ state.user }}", "redirect": "nowhere"}, "指向不存在的场景"},
		{"self redirect", map[string]any{"condition": "{{ state.user }}", "redirect": "secure"}, "指向自身"},
		{"no redirect", map[string]any{"condition": "{{ state.user }}"}, "guard 没有 redirect"},
		{"condition type mismatch", map[string]any{"condition": "{{ state.user - 1 }}", "redirect": "main"}, "guard condition type mismatch"},
		{"params type mismatch", map[string]any{"condition": "{{ state.user }}", "redirect": "main",
			"params": map[string]any{"n": "{{ state.user - 1 }}"}}, "guard params.n type mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := FromDocs(guardDocs(tt.guard))
			if !hasDiag(app.Diagnostics, tt.want) {
				t.Errorf("want a diagnostic containing %q, got %v", tt.want, app.Diagnostics)
			}
		})
	}
}

func TestGuardWithoutConditionIsDropped(t *testing.T) {
	app := FromDocs(guardDocs(map[string]any{"redirect": "main"}))
	if _, ok := app.SceneGuards["secure"]; ok {
		t.Error("a guard with no condition must be dropped, not kept as an always-open one")
	}
}

func TestGuardRedirectCycleIsWarned(t *testing.T) {
	docs := guardDocs(map[string]any{"condition": "{{ state.user != '' }}", "redirect": "main"})
	// Close the loop: main redirects back to secure.
	docs[1]["guard"] = map[string]any{"condition": "{{ state.count > 0 }}", "redirect": "secure"}
	app := FromDocs(docs)
	if !hasDiag(app.Diagnostics, "可能构成环") {
		t.Errorf("a mutual redirect must be warned about: %v", app.Diagnostics)
	}
	// One warning per scene on the cycle, not one per traversal.
	n := 0
	for _, d := range app.Diagnostics {
		if contains(d, "可能构成环") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want one cycle warning per scene on the cycle, got %d: %v", n, app.Diagnostics)
	}
}

func TestGuardRedirectCycleNotIncludingTheStartIsReportedOnce(t *testing.T) {
	// a -> b -> c -> b: the cycle is {b, c}. Walking from a reaches it but must
	// not claim a is on it — the warning belongs to b and c, from their own
	// entries, so the report names the scenes actually looping.
	docs := guardDocs(map[string]any{"condition": "{{ state.user != '' }}", "redirect": "third"})
	docs = append(docs,
		map[string]any{"type": "scene", "id": "third",
			"root":  map[string]any{"type": "view", "id": "third_root"},
			"guard": map[string]any{"condition": "{{ state.count > 0 }}", "redirect": "fourth"}},
		map[string]any{"type": "scene", "id": "fourth",
			"root":  map[string]any{"type": "view", "id": "fourth_root"},
			"guard": map[string]any{"condition": "{{ state.count > 1 }}", "redirect": "third"}})
	app := FromDocs(docs)
	for _, d := range app.Diagnostics {
		if contains(d, "可能构成环") && contains(d, "[Scene: secure]") {
			t.Errorf("the scene that merely LEADS to a cycle must not be reported as on it: %s", d)
		}
	}
	n := 0
	for _, d := range app.Diagnostics {
		if contains(d, "可能构成环") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want one warning per scene on the cycle (third, fourth), got %d: %v", n, app.Diagnostics)
	}
}

func TestGuardRedirectChainWithoutACycleIsClean(t *testing.T) {
	docs := guardDocs(map[string]any{"condition": "{{ state.user != '' }}", "redirect": "main"})
	docs = append(docs, map[string]any{"type": "scene", "id": "third",
		"root":  map[string]any{"type": "view", "id": "third_root"},
		"guard": map[string]any{"condition": "{{ state.count > 0 }}", "redirect": "secure"}})
	app := FromDocs(docs)
	if hasDiag(app.Diagnostics, "可能构成环") {
		t.Errorf("a finite redirect chain must not be reported as a cycle: %v", app.Diagnostics)
	}
}

// ---- forEach ----------------------------------------------------------------

func TestLoadForEachStep(t *testing.T) {
	app := FromDocs(computedDocs(nil, map[string]any{
		"type": "action", "id": "markAll",
		"steps": []any{map[string]any{
			"type": "forEach", "in": "{{ state.items }}", "as": "msg",
			"steps": []any{
				map[string]any{"type": "state.updateWhere", "path": "items",
					"matchKey": "id", "match": "{{ msg.id }}",
					"item": map[string]any{"read": "{{ true }}"}},
			},
		}},
	}))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a well-formed forEach must load clean: %v", app.Diagnostics)
	}
	st := app.Actions["markAll"].Steps[0]
	if st.Type != "forEach" || st.In != "{{ state.items }}" || st.As != "msg" {
		t.Fatalf("forEach step lost fields: %+v", st)
	}
	if len(st.Steps) != 1 || st.Steps[0].Type != "state.updateWhere" {
		t.Fatalf("forEach body lost: %+v", st.Steps)
	}
}

func TestForEachDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		step map[string]any
		want string
	}{
		{
			name: "missing in",
			step: map[string]any{"type": "forEach", "steps": []any{
				map[string]any{"type": "state.increment", "path": "count"}}},
			want: "'forEach' 步骤缺少 in",
		},
		{
			name: "constant in",
			step: map[string]any{"type": "forEach", "in": "state.items", "steps": []any{
				map[string]any{"type": "state.increment", "path": "count"}}},
			want: "不含 {{...}} 绑定",
		},
		{
			name: "empty body",
			step: map[string]any{"type": "forEach", "in": "{{ state.items }}"},
			want: "没有 steps",
		},
		{
			name: "unusable alias",
			step: map[string]any{"type": "forEach", "in": "{{ state.items }}", "as": "state",
				"steps": []any{map[string]any{"type": "state.increment", "path": "count"}}},
			want: "别名 as: \"state\" 不可用",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := FromDocs(computedDocs(nil, map[string]any{
				"type": "action", "id": "a", "steps": []any{tt.step}}))
			if !hasDiag(app.Diagnostics, tt.want) {
				t.Errorf("want a diagnostic containing %q, got %v", tt.want, app.Diagnostics)
			}
		})
	}
}

func TestForEachBodyParticipatesInWholeTreeChecks(t *testing.T) {
	// The loop body is a nested step list like `then` / `onSuccess`, so every
	// whole-tree check must descend into it: an unknown `invoke` target inside
	// a loop is as broken as one at the top level.
	app := FromDocs(computedDocs(nil, map[string]any{
		"type": "action", "id": "a", "steps": []any{
			map[string]any{"type": "forEach", "in": "{{ state.items }}", "steps": []any{
				map[string]any{"type": "invoke", "name": "doesNotExist"},
			}},
		}}))
	if !hasDiag(app.Diagnostics, "引用了不存在的动作") {
		t.Errorf("an unknown invoke inside a loop body must be diagnosed: %v", app.Diagnostics)
	}
}

func TestForEachRenderStepsCountTowardTheAdvisoryCeiling(t *testing.T) {
	body := []any{}
	for i := 0; i < maxRenderSteps+1; i++ {
		body = append(body, map[string]any{"type": "render"})
	}
	app := FromDocs(computedDocs(nil, map[string]any{
		"type": "action", "id": "a", "steps": []any{
			map[string]any{"type": "forEach", "in": "{{ state.items }}", "steps": body},
		}}))
	if !hasDiag(app.Diagnostics, "个 'render' 步骤") {
		t.Errorf("render steps inside a loop body must count: %v", app.Diagnostics)
	}
}

func TestForEachNestingDepthIsCapped(t *testing.T) {
	inner := map[string]any{"type": "state.increment", "path": "count"}
	for i := 0; i < maxStepNesting+2; i++ {
		inner = map[string]any{"type": "forEach", "in": "{{ state.items }}", "steps": []any{inner}}
	}
	app := FromDocs(computedDocs(nil, map[string]any{
		"type": "action", "id": "a", "steps": []any{inner}}))
	if !hasDiag(app.Diagnostics, "步骤嵌套超过") {
		t.Errorf("an over-deep loop nest must be diagnosed: %v", app.Diagnostics)
	}
}

// contains is strings.Contains under a shorter name, for the counting loops
// above.
func contains(s, sub string) bool { return hasDiag([]string{s}, sub) }

// ---- state-rooted step paths ---------------------------------------------------

// TestStateRootedStepPathIsWarned: a step path is already relative to the state
// root, so `state.count` is a typo that creates a top-level key named "state".
// Nothing used to say so — and the mistake is the natural one to make, because
// the binding two lines up in the scene really is spelled `{{ state.count }}`.
func TestStateRootedStepPathIsWarned(t *testing.T) {
	app := FromDocs(computedDocs(nil,
		map[string]any{"type": "action", "id": "typo", "steps": []any{
			map[string]any{"type": "state.set", "path": "state.count", "value": "{{ 1 }}"},
			// Nested inside a branch and a loop body, and on the async write
			// fields: the check must descend and cover every target field.
			map[string]any{"type": "if", "condition": "{{ true }}", "then": []any{
				map[string]any{"type": "forEach", "in": "{{ state.items }}", "steps": []any{
					map[string]any{"type": "http.get", "url": "https://x.test",
						"result": "state.resp", "error": "state.err", "pending": "state.busy"},
				}},
			}},
		}}))
	n := 0
	for _, d := range app.Diagnostics {
		if contains(d, "步骤路径本来就相对状态根") {
			n++
		}
	}
	if n != 4 {
		t.Errorf("want 4 state-rooted-path warnings (path, result, error, pending), got %d in %v", n, app.Diagnostics)
	}
	for _, d := range app.Diagnostics {
		if contains(d, "步骤路径本来就相对状态根") && !contains(d, "warning:") {
			t.Errorf("the typo must stay a warning — \"state\" is a legal key name: %q", d)
		}
	}
}

// TestPlainStepPathsAreNotWarned guards the false-positive side: an ordinary
// relative path, a nested one, and a key whose name merely STARTS with "state"
// are all correct and must stay silent.
func TestPlainStepPathsAreNotWarned(t *testing.T) {
	app := FromDocs(computedDocs(nil,
		map[string]any{"type": "action", "id": "fine", "steps": []any{
			map[string]any{"type": "state.set", "path": "count", "value": "{{ 1 }}"},
			map[string]any{"type": "state.set", "path": "user.name", "value": "{{ 'ada' }}"},
			// "stateful" starts with the root's name but is not rooted through
			// it: the dot boundary is what decides.
			map[string]any{"type": "state.set", "path": "stateful", "value": "{{ 1 }}"},
			// The root alone is a legal (if odd) key name, not a mis-rooted path.
			map[string]any{"type": "state.set", "path": "state", "value": "{{ 1 }}"},
		}}))
	if hasDiag(app.Diagnostics, "步骤路径本来就相对状态根") {
		t.Errorf("a correct relative path must not be warned: %v", app.Diagnostics)
	}
}

// TestStateRootedComputedPathIsOneDiagnosticNotTwo: `state.computed.total` is
// both a state-rooted typo and a write into the read-only namespace. It earns
// the specific (error) diagnostic, not both.
func TestStateRootedComputedPathIsOneDiagnosticNotTwo(t *testing.T) {
	app := FromDocs(computedDocs(
		map[string]any{"total": "{{ state.count * 2 }}"},
		map[string]any{"type": "action", "id": "cheat", "steps": []any{
			map[string]any{"type": "state.set", "path": "state.computed.total", "value": "{{ 1 }}"},
		}}))
	if !hasDiag(app.Diagnostics, "写入了派生值路径") {
		t.Errorf("the binding spelling must be caught as a read-only write: %v", app.Diagnostics)
	}
	if hasDiag(app.Diagnostics, "步骤路径本来就相对状态根") {
		t.Errorf("one mistake, one diagnostic: %v", app.Diagnostics)
	}
	// Without declarations `computed` is an ordinary key, so the same path is
	// just the state-rooted typo — and must still be reported as one.
	plain := FromDocs(computedDocs(nil,
		map[string]any{"type": "action", "id": "cheat", "steps": []any{
			map[string]any{"type": "state.set", "path": "state.computed.total", "value": "{{ 1 }}"},
		}}))
	if !hasDiag(plain.Diagnostics, "步骤路径本来就相对状态根") {
		t.Errorf("without declarations it is still a mis-rooted path: %v", plain.Diagnostics)
	}
}
