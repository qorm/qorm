// Package loader reads a QORM application from a directory (manifest + scenes
// + actions), skipping test fixtures and nested projects, and builds a
// model.App from the JSON scene format.
package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/expr"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/pkg/qormext"
)

// skipDirs are directories that never contain renderable QORM sources.
var skipDirs = map[string]bool{
	"target": true, "qorm_standalone": true, "src": true,
	"assets": true, "node_modules": true, ".git": true,
}

// LoadDir loads an app from a directory.
func LoadDir(dir string) (*model.App, error) {
	docs, err := CollectDocs(dir)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no QORM source documents found under %s", dir)
	}
	app := FromDocs(docs)
	loadLocales(dir, app)
	app.BaseDir = dir
	return app, nil
}

// loadLocales reads <dir>/locales/<lang>.json message catalogs into the app.
func loadLocales(dir string, app *model.App) {
	if locales := LoadLocales(dir); locales != nil {
		app.Locales = locales
	}
}

// LoadLocales reads <dir>/locales/<lang>.json into a lang -> key -> string map
// (nil if there is no locales directory).
func LoadLocales(dir string) map[string]map[string]string {
	entries, err := os.ReadDir(filepath.Join(dir, "locales"))
	if err != nil {
		return nil
	}
	out := map[string]map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "locales", e.Name()))
		if err != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		cat := make(map[string]string, len(raw))
		for k, v := range raw {
			cat[k] = asString(v)
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = cat
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CollectDocs returns the raw (parsed) QORM source documents under dir,
// skipping test fixtures and nested projects. Used by both the loader and the
// bundle builder.
func CollectDocs(dir string) ([]map[string]any, error) {
	return collect(dir)
}

// FromDocs assembles a model.App from a set of raw source documents.
// Manifests are applied first (a scene file may sort before qorm.json) so the
// globalState schema is known when scene/action expressions are type-checked.
func FromDocs(docs []map[string]any) *model.App {
	app := &model.App{
		Scenes:  map[string]*model.Node{},
		Actions: map[string]*model.Action{},
	}
	var diags []string
	for _, doc := range docs {
		if asString(doc["type"]) == "app" {
			applyManifest(app, doc, &diags)
		}
	}
	sceneVars := stateVars(app.GlobalState.Schema, false)
	actionVars := stateVars(app.GlobalState.Schema, true)
	for i, doc := range docs {
		switch asString(doc["type"]) {
		case "app":
			// Applied by the manifest pass above; nothing to do here.
		case "scene":
			if root, ok := doc["root"].(map[string]any); ok {
				sceneID := asString(doc["id"])
				app.Scenes[sceneID] = buildNode(root, &diags, sceneID, sceneVars, nil)
				// Scene lifecycle: an optional onEnter invoke, dispatched once
				// each time navigation enters this scene (incl. first load and
				// deep links). Parsed like onPress (string shorthand or
				// {name,args}); the name is cross-checked against the loaded
				// actions below, once every doc has been assembled.
				if inv := parseInvoke(doc["onEnter"], &diags, sceneID, "", "onEnter"); inv != nil {
					if app.SceneEnter == nil {
						app.SceneEnter = map[string]*model.Invoke{}
					}
					app.SceneEnter[sceneID] = inv
				}
				// Route guard: the precondition for entering this scene. Like
				// onEnter it lives on the scene document (not the node tree);
				// its redirect target is cross-checked against the loaded
				// scenes below, once every doc has been assembled.
				if g := parseGuard(doc["guard"], &diags, sceneID, sceneVars); g != nil {
					if app.SceneGuards == nil {
						app.SceneGuards = map[string]*model.SceneGuard{}
					}
					app.SceneGuards[sceneID] = g
				}
			}
		case "action":
			act := buildAction(doc, &diags, actionVars)
			if act.ID != "" {
				app.Actions[act.ID] = act
			}
		case "component":
			// A standalone component definition document (conventionally
			// components/<name>.json), equivalent to one entry of qorm.json's
			// inline "components" map: `id` is the component name and
			// `template` its root node, with optional props/slots declarations.
			// The manifest pass above ran first, so an inline definition of the
			// same name wins and this one is diagnosed as a redefinition.
			name := asString(doc["id"])
			if name == "" {
				diags = append(diags, fmt.Sprintf("error: document #%d 是 type:\"component\" 组件定义但缺少 \"id\"(组件名),已忽略。", i))
				continue
			}
			if _, ok := doc["template"].(map[string]any); !ok {
				diags = append(diags, fmt.Sprintf("error: 组件文档 %q 缺少 \"template\"(组件模板根节点对象),已忽略。", name))
				continue
			}
			defineComponent(app, name, doc, sceneVars, &diags)
		default:
			// A doc with no recognised type used to be silently dropped, so
			// an app whose only doc is typeless rendered "no scene" with zero
			// feedback. Surface it (id included when present) so authors — and
			// the playground's diagnostics strip — see the ignored document.
			typ, id := asString(doc["type"]), asString(doc["id"])
			if id != "" {
				diags = append(diags, fmt.Sprintf("error: document #%d has unknown or missing \"type\" (got %q, id %q) — ignored", i, typ, id))
			} else {
				diags = append(diags, fmt.Sprintf("error: document #%d has unknown or missing \"type\" (got %q) — ignored", i, typ))
			}
		}
	}
	if app.Entry == "" {
		app.Entry = "main"
	}
	// A dangling entry used to be silently masked by EntryRoot's any-scene
	// fallback; surface it so a misconfigured manifest is diagnosed. (No
	// scenes at all is not an error: a manifest-less or still-empty app.)
	if len(app.Scenes) > 0 {
		if _, ok := app.Scenes[app.Entry]; !ok {
			ids := make([]string, 0, len(app.Scenes))
			for id := range app.Scenes {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			diags = append(diags, fmt.Sprintf("error: entry scene %q does not exist (scenes: %s)", app.Entry, strings.Join(ids, ", ")))
		}
	}
	// Cross-document reference checks: these need the full action set, so they
	// run after every doc has been assembled (a scene file may sort before the
	// actions it references).
	checkActionRefs(app, &diags)
	checkTimers(app, &diags)
	checkComponents(app, &diags)
	checkGuards(app, &diags)
	checkComputed(app, sceneVars, &diags)
	app.Diagnostics = diags
	return app
}

// actionRefKnown reports whether an action name is statically resolvable: a
// loaded action, a runtime builtin (__dismiss/__sort), or a dynamic binding
// ({{...}}) that can only resolve at run time.
func actionRefKnown(app *model.App, name string) bool {
	if name == "" || strings.Contains(name, "{{") {
		return true // empty/dynamic names are diagnosed (or resolved) elsewhere
	}
	if _, ok := app.Actions[name]; ok {
		return true
	}
	return name == "__dismiss" || name == "__sort" // runtime builtins (see internal/runtime)
}

// checkActionRefs diagnoses statically-unknown action names referenced by the
// new dispatch surfaces: scene onEnter hooks and `invoke` steps. (Node event
// handlers may name component callback props, so they are not checked here.)
func checkActionRefs(app *model.App, diags *[]string) {
	sceneIDs := make([]string, 0, len(app.SceneEnter))
	for id := range app.SceneEnter {
		sceneIDs = append(sceneIDs, id)
	}
	sort.Strings(sceneIDs)
	for _, id := range sceneIDs {
		if inv := app.SceneEnter[id]; !actionRefKnown(app, inv.Name) {
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] onEnter 引用了不存在的动作 %q。", id, inv.Name))
		}
	}
	for _, id := range sortedActionIDs(app) {
		walkSteps(app.Actions[id].Steps, func(st model.Step) {
			if st.Type == "invoke" && !actionRefKnown(app, st.Name) {
				*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 'invoke' 步骤引用了不存在的动作 %q。", id, st.Name))
			}
		})
	}
}

// sortedActionIDs returns the app's action ids in a stable order, so every
// whole-app check emits its diagnostics deterministically.
func sortedActionIDs(app *model.App) []string {
	out := make([]string, 0, len(app.Actions))
	for id := range app.Actions {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// walkSteps visits every step in a step tree, descending into EVERY nested step
// list: `if` then/else, `forEach` steps, and the http result branches. One walk
// shared by every whole-tree check, so a new nesting site is taught to all of
// them at once instead of being forgotten by one.
func walkSteps(steps []model.Step, fn func(model.Step)) {
	for _, st := range steps {
		fn(st)
		walkSteps(st.Then, fn)
		walkSteps(st.Else, fn)
		walkSteps(st.Steps, fn)
		walkSteps(st.OnSuccess, fn)
		walkSteps(st.OnError, fn)
	}
}

// checkGuards validates the scene route guards across the app: the redirect
// target must be a real, different scene, and a guard that cannot redirect
// anywhere is called out because it cannot protect the entry scene. A redirect
// CYCLE (login guards to home, home guards to login) is reported too: the
// runtime caps the chain and refuses such a navigation, which reads as "the
// button does nothing" unless the loader says why.
func checkGuards(app *model.App, diags *[]string) {
	ids := make([]string, 0, len(app.SceneGuards))
	for id := range app.SceneGuards {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g := app.SceneGuards[id]
		switch {
		case g.Redirect == "":
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] guard 没有 redirect:条件不满足时只会拒绝导航(停在原场景),且无法保护入口场景(那里没有可退回的场景)。", id))
		case g.Redirect == id:
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] guard 的 redirect 指向自身,条件不满足时无处可去,该导航将被拒绝。", id))
		default:
			if _, ok := app.Scenes[g.Redirect]; !ok && !strings.Contains(g.Redirect, "{{") {
				*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] guard 的 redirect 指向不存在的场景 %q。", id, g.Redirect))
			}
		}
	}
	for _, id := range ids {
		if cycle := guardRedirectCycle(app, id); len(cycle) > 0 {
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] guard 的 redirect 可能构成环:%s。若这些条件同时为假,运行时会拒绝该次导航(停在原场景)。", id, strings.Join(cycle, " -> ")))
		}
	}
}

// guardRedirectCycle follows the redirect chain out of scene id and returns it
// (as a scene path) when it comes back to a scene already on the path. It walks
// the STATIC graph, so it reports a cycle that is possible, not one that is
// certain — the conditions decide at run time. Self-redirects are reported by
// checkGuards as an error instead, so they are not repeated here.
func guardRedirectCycle(app *model.App, id string) []string {
	path := []string{id}
	seen := map[string]bool{id: true}
	for cur := id; ; {
		g := app.SceneGuards[cur]
		if g == nil || g.Redirect == "" || g.Redirect == cur {
			return nil
		}
		cur = g.Redirect
		path = append(path, cur)
		if seen[cur] {
			if cur != id {
				return nil // the cycle does not include id; reported from its own entry
			}
			return path
		}
		seen[cur] = true
	}
}

// checkComputed validates the app's derived-value declarations: each name must
// be a plain identifier (the namespace is read through dotted paths), each
// expression must actually be a binding and type-check, the reserved namespace
// must not collide with a real state key, no declaration may take part in a
// dependency cycle, and no action step may write into the namespace.
func checkComputed(app *model.App, vars map[string]string, diags *[]string) {
	if len(app.Computed) == 0 {
		return
	}
	ns := model.ComputedNamespace
	if _, ok := app.GlobalState.Schema[ns]; ok {
		*diags = append(*diags, fmt.Sprintf("error: globalState.schema 声明了 %q,但该名字是派生值(computed)的保留命名空间,每帧都会被覆盖。请给状态键换个名字。", ns))
	}
	if _, ok := app.GlobalState.Initial[ns]; ok {
		*diags = append(*diags, fmt.Sprintf("error: globalState.initial 提供了 %q 的初始值,但该名字是派生值(computed)的保留命名空间,每帧都会被覆盖。请给状态键换个名字。", ns))
	}
	names := make([]string, 0, len(app.Computed))
	for n := range app.Computed {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		e := app.Computed[name]
		if !isPlainIdent(name) {
			*diags = append(*diags, fmt.Sprintf("error: computed 派生值的名字 %q 不是普通标识符,无法通过 {{ %s.%s }} 读取。", name, ns, name))
		}
		switch {
		case strings.TrimSpace(e) == "":
			*diags = append(*diags, fmt.Sprintf("error: computed 派生值 %q 的表达式为空,读取它只会得到空值。", name))
		case !strings.Contains(e, "{{"):
			*diags = append(*diags, fmt.Sprintf("warning: computed 派生值 %q 的表达式 %q 不含 {{...}} 绑定:它将恒等于这段字面文本。请写成表达式绑定,如 \"{{ %s }}\"。", name, e, e))
		}
		forEachExpr(e, func(src string) {
			for _, mm := range expr.Check(src, vars) {
				*diags = append(*diags, fmt.Sprintf("error: computed 派生值 %q type mismatch: %s in {{ %s }}", name, mm.Detail, mm.Expr))
			}
		})
	}
	if _, cyclic := app.ComputedOrder(); len(cyclic) > 0 {
		*diags = append(*diags, fmt.Sprintf("error: computed 派生值存在循环依赖(或依赖了处于循环中的值):%s。这些值不会被求值,读取它们只会得到空值。", strings.Join(cyclic, ", ")))
	}
	for _, id := range sortedActionIDs(app) {
		walkSteps(app.Actions[id].Steps, func(st model.Step) {
			for _, p := range []string{st.Path, st.Result, st.Error} {
				if model.IsComputedPath(p) {
					*diags = append(*diags, fmt.Sprintf("error: [Action: %s] %q 步骤写入了派生值路径 %q:computed 是只读的(每帧由声明式重新求值),该步骤会被忽略。", id, st.Type, p))
				}
			}
		})
	}
}

// isPlainIdent reports whether s is a bare identifier
// ([A-Za-z_][A-Za-z0-9_]*) — the only shape the expression language can
// reference through a dotted path.
func isPlainIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || (i > 0 && '0' <= c && c <= '9') {
			continue
		}
		return false
	}
	return s != ""
}

// parseGuard reads a scene document's "guard" object:
//
//	"guard": {"condition": "{{ state.user != null }}", "redirect": "login",
//	          "params": {"next": "'dashboard'"}}
//
// Returns nil for an absent or unusable declaration (diagnosed), so a scene
// without a usable guard stays exactly as public as it was before.
func parseGuard(raw any, diags *[]string, sceneID string, vars map[string]string) *model.SceneGuard {
	if raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] guard 应为对象(如 {\"condition\": \"{{ state.user != null }}\", \"redirect\": \"login\"}),已忽略。", sceneID))
		}
		return nil
	}
	g := &model.SceneGuard{Condition: asString(m["condition"]), Redirect: asString(m["redirect"])}
	if params, ok := m["params"].(map[string]any); ok {
		g.Params = map[string]string{}
		for k, v := range params {
			g.Params[k] = asString(v)
		}
	}
	// A guard with no condition guards nothing; keeping it would only add a
	// per-navigation lookup that always passes.
	if g.Condition == "" {
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] guard 缺少 condition(进入该场景所需满足的表达式),已忽略该守卫。", sceneID))
		}
		return nil
	}
	if diags != nil {
		if !strings.Contains(g.Condition, "{{") {
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] guard 的 condition %q 不含 {{...}} 绑定:非空字符串恒为真,该守卫永远不会拦截。请写成表达式绑定,如 \"{{ %s }}\"。", sceneID, g.Condition, g.Condition))
		}
		checkGuardExprTypes(g, diags, sceneID, vars)
	}
	return g
}

// checkGuardExprTypes type-checks the guard's condition and redirect params
// against the scene binding scope, exactly like a node's expressions.
func checkGuardExprTypes(g *model.SceneGuard, diags *[]string, sceneID string, vars map[string]string) {
	check := func(what, src string) {
		forEachExpr(src, func(e string) {
			for _, mm := range expr.Check(e, vars) {
				*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] guard %s type mismatch: %s in {{ %s }}", sceneID, what, mm.Detail, mm.Expr))
			}
		})
	}
	check("condition", g.Condition)
	for _, k := range sortedStrMapKeys(g.Params) {
		check("params."+k, g.Params[k])
	}
}

// sortedStrMapKeys returns a string map's keys in a stable order.
func sortedStrMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- components: definitions, declared schemas, instance checks ----

// componentTemplate splits a component definition object into its template node
// and whether it used the declaration form. Two spellings mean the same thing:
//
//	{"type":"card","children":[...]}                    the definition IS the template
//	{"props":{...},"slots":{...},"template":{...}}      the declaration form
//
// A `template` object is the discriminator — no node type reads a `template`
// key (a list's item template is `renderItem`), so an app written before
// declarations existed can never be mistaken for one.
func componentTemplate(def map[string]any) (tmpl map[string]any, declaring bool) {
	if t, ok := def["template"].(map[string]any); ok {
		return t, true
	}
	return def, false
}

// defineComponent registers one component (from qorm.json's inline "components"
// map or from its own type:"component" document) together with its optional
// declared schema. Redefining a name is diagnosed and ignored, so whichever
// definition is seen first — the manifest is applied before any document — is
// the one that renders.
func defineComponent(app *model.App, name string, def map[string]any, vars map[string]string, diags *[]string) {
	if _, dup := app.Components[name]; dup {
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("error: 组件 %q 被重复定义(qorm.json 内联 components 与 type:\"component\" 组件文档,或多个组件文档同名),仅保留最先出现的定义。", name))
		}
		return
	}
	tmpl, declaring := componentTemplate(def)
	if declaring {
		if schema := parseComponentSchema(name, def, diags); schema != nil {
			if app.ComponentSchemas == nil {
				app.ComponentSchemas = map[string]*model.ComponentSchema{}
			}
			app.ComponentSchemas[name] = schema
			// Declared prop types join the template's type-check scope, so
			// {{ prop.count + 1 }} is checked exactly like {{ state.count + 1 }}.
			vars = componentVars(vars, schema)
		}
	}
	if app.Components == nil {
		app.Components = map[string]*model.Node{}
	}
	app.Components[name] = buildNode(tmpl, diags, "component:"+name, vars, nil)
}

// parseComponentSchema reads the "props" / "slots" declarations off a component
// definition, returning nil when it declares neither (the component then keeps
// the historical anything-goes contract).
func parseComponentSchema(comp string, def map[string]any, diags *[]string) *model.ComponentSchema {
	rawProps, hasProps := def["props"].(map[string]any)
	rawSlots, hasSlots := def["slots"].(map[string]any)
	if !hasProps && !hasSlots {
		return nil
	}
	sc := &model.ComponentSchema{}
	if hasProps {
		sc.Props = make(map[string]model.PropSpec, len(rawProps))
		for _, name := range sortedKeys(rawProps) {
			sc.Props[name] = parsePropSpec(comp, name, rawProps[name], diags)
		}
	}
	if hasSlots {
		sc.Slots = make(map[string]model.SlotSpec, len(rawSlots))
		for _, name := range sortedKeys(rawSlots) {
			spec := model.SlotSpec{}
			switch t := rawSlots[name].(type) {
			case map[string]any:
				spec.Required = asBool(t["required"])
			case nil:
			default:
				if diags != nil {
					*diags = append(*diags, fmt.Sprintf("warning: 组件 %q 的 slot %q 声明格式无法识别(应为 {\"required\": true|false}),按可选 slot 处理。", comp, name))
				}
			}
			sc.Slots[name] = spec
		}
	}
	return sc
}

// parsePropSpec reads one prop declaration: the shorthand `"title": "string"`
// or the long form `{"type":"number","default":0,"required":false}`.
func parsePropSpec(comp, prop string, raw any, diags *[]string) model.PropSpec {
	switch t := raw.(type) {
	case string:
		return model.PropSpec{Type: normalizePropType(comp, prop, t, diags)}
	case map[string]any:
		spec := model.PropSpec{
			Type:     normalizePropType(comp, prop, asString(t["type"]), diags),
			Required: asBool(t["required"]),
		}
		if d, ok := t["default"]; ok && d != nil {
			spec.Default = d
			if diags != nil && !propValueMatches(spec.Type, d) {
				*diags = append(*diags, fmt.Sprintf("warning: 组件 %q 的 prop %q 声明类型为 %q,但其 default 是 %s。", comp, prop, spec.Type, propValueKind(d)))
			}
			if diags != nil && spec.Required {
				*diags = append(*diags, fmt.Sprintf("warning: 组件 %q 的 prop %q 同时声明了 required 与 default:必填项永远由实例提供,default 不会生效。", comp, prop))
			}
		}
		return spec
	case nil:
		return model.PropSpec{}
	default:
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("warning: 组件 %q 的 prop %q 声明格式无法识别(应为类型字符串如 \"string\",或 {\"type\":…,\"default\":…,\"required\":…}),该 prop 将不做校验。", comp, prop))
		}
		return model.PropSpec{}
	}
}

// normalizePropType maps a declared prop type onto the canonical names the
// checks use, mirroring the expression checker's own aliases. "any", an empty
// declaration and (with a warning) an unknown name all mean "unconstrained".
func normalizePropType(comp, prop, t string, diags *[]string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "any":
		return ""
	case "number", "num", "int", "integer", "float", "double":
		return "number"
	case "string", "str", "text":
		return "string"
	case "bool", "boolean":
		return "boolean"
	case "array", "list":
		return "array"
	case "object", "map":
		return "object"
	}
	if diags != nil {
		*diags = append(*diags, fmt.Sprintf("warning: 组件 %q 的 prop %q 声明了未知类型 %q(可用:string/number/boolean/array/object/any),该 prop 将不做类型校验。", comp, prop, t))
	}
	return ""
}

// componentVars extends a component template's type-check scope with its
// declared prop types (prop.title -> "string", …). Undeclared props stay
// unknown to the checker, which never reports what it cannot prove.
func componentVars(base map[string]string, sc *model.ComponentSchema) map[string]string {
	if sc == nil || len(sc.Props) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(sc.Props))
	for k, v := range base {
		out[k] = v
	}
	for name, spec := range sc.Props {
		if spec.Type != "" {
			out["prop."+name] = spec.Type
		}
	}
	return out
}

// componentInstanceName resolves the component a node instantiates: either its
// `type` names one directly ({"type":"panel"} — the form the runtime has always
// used), or it is the spec's explicit instance form
// ({"type":"component","ref":"panel"}, ref optionally "component://panel").
// Returns "" for a node that instantiates nothing.
func componentInstanceName(app *model.App, n *model.Node) string {
	if _, ok := app.Components[n.Type]; ok {
		return n.Type
	}
	if n.Type != "component" {
		return ""
	}
	ref, _ := n.Props["ref"].(string)
	name := model.ComponentRefName(ref)
	if _, ok := app.Components[name]; ok {
		return name
	}
	return ""
}

// instanceProps collects everything a component instance passes into the
// template's prop.* scope — the exact union renderComponent builds: every node
// key except the nested `props` object, then the text/label/value shorthands,
// then the nested `props` object (which wins on conflict).
func instanceProps(n *model.Node) map[string]any {
	out := make(map[string]any, len(n.Props)+3)
	for k, v := range n.Props {
		if k == "props" {
			continue
		}
		out[k] = v
	}
	if n.Text != "" {
		out["text"] = n.Text
	}
	if n.Label != "" {
		out["label"] = n.Label
	}
	if n.Value != "" {
		out["value"] = n.Value
	}
	if pm, ok := n.Props["props"].(map[string]any); ok {
		for k, v := range pm {
			out[k] = v
		}
	}
	return out
}

// checkComponents validates every component instance in every scene (and inside
// component templates, which may nest components) against the declared schema
// of the component it instantiates. Components without a declaration are not
// checked at all, so an app that declares nothing is diagnosed exactly as before.
func checkComponents(app *model.App, diags *[]string) {
	if len(app.ComponentSchemas) == 0 {
		return
	}
	scopes, roots := componentScopes(app)
	for _, scope := range scopes {
		walkSceneNodes(roots[scope], func(n *model.Node) {
			name := componentInstanceName(app, n)
			if name == "" {
				return
			}
			sc := app.ComponentSchemas[name]
			if sc == nil {
				return
			}
			checkInstanceProps(scope, name, n, sc, diags)
			checkInstanceSlots(scope, name, n, sc, diags)
		})
	}
}

// checkInstanceProps diagnoses a missing required prop, a literal value that
// cannot satisfy its declared type, and an undeclared key in the instance's
// nested props object.
func checkInstanceProps(scope, name string, n *model.Node, sc *model.ComponentSchema, diags *[]string) {
	if sc.Props == nil {
		return
	}
	props := instanceProps(n)
	for _, pn := range sortedPropNames(sc.Props) {
		spec := sc.Props[pn]
		v, ok := props[pn]
		if !ok {
			if spec.Required {
				*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] 组件 %q 的实例 (id: %q) 缺少必填 prop %q。", scope, name, n.ID, pn))
			}
			continue
		}
		if !propValueMatches(spec.Type, v) {
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] 组件 %q 的实例 (id: %q) 的 prop %q 声明类型为 %q,但传入了 %s。", scope, name, n.ID, pn, spec.Type, propValueKind(v)))
		}
	}
	// Undeclared props are only reported inside the explicit nested `props`
	// object: a node's TOP-LEVEL keys are indistinguishable from structural
	// fields (type/id/style/children/slot/if/…), every one of which the
	// renderer also exposes as prop.*, so flagging those would be noise.
	pm, ok := n.Props["props"].(map[string]any)
	if !ok {
		return
	}
	for _, k := range sortedKeys(pm) {
		if _, declared := sc.Props[k]; !declared {
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] 组件 %q 的实例 (id: %q) 在 props 中传入了未声明的 prop %q(组件已声明 props schema,该 prop 仍会透传到 prop.%s)。", scope, name, n.ID, k, k))
		}
	}
}

// checkInstanceSlots diagnoses a missing required slot and a child attributed
// to a slot the component never declared.
func checkInstanceSlots(scope, name string, n *model.Node, sc *model.ComponentSchema, diags *[]string) {
	if len(sc.Slots) == 0 {
		return
	}
	filled := make(map[string]bool, len(n.Children))
	for _, c := range n.Children {
		s, _ := c.Props["slot"].(string)
		if s == "" {
			continue
		}
		filled[s] = true
		if _, declared := sc.Slots[s]; !declared {
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] 组件 %q 的实例 (id: %q) 的子节点 (id: %q) 填充了未声明的 slot %q。", scope, name, n.ID, c.ID, s))
		}
	}
	for _, sn := range sortedSlotNames(sc.Slots) {
		if sc.Slots[sn].Required && !filled[sn] {
			*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] 组件 %q 的实例 (id: %q) 缺少必填 slot %q(用一个带 \"slot\": %q 的子节点填充)。", scope, name, n.ID, sn, sn))
		}
	}
}

// propValueMatches reports whether a literal instance value can satisfy a
// declared prop type. An unconstrained declaration, a null and any {{binding}}
// (whose value exists only at render time) always match, so the check can only
// fire on something statically wrong. Numbers and booleans spelled as strings
// are accepted: the text/label/value shorthands are strings by construction
// (model.Node stores them so), and requiring a JSON number there would flag
// correct apps.
func propValueMatches(declared string, v any) bool {
	if declared == "" || v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		if strings.Contains(t, "{{") {
			return true
		}
		switch declared {
		case "string":
			return true
		case "number":
			_, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			return err == nil
		case "boolean":
			s := strings.TrimSpace(t)
			return s == "true" || s == "false"
		}
		return false
	case float64, int, int64:
		return declared == "number" || declared == "string"
	case bool:
		return declared == "boolean" || declared == "string"
	case []any:
		return declared == "array"
	case map[string]any:
		return declared == "object"
	}
	return true
}

// propValueKind names the JSON kind of a value, for the type-mismatch message.
func propValueKind(v any) string {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, "{{") {
			return "绑定表达式"
		}
		return "string"
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", t)
	}
}

// sortedKeys returns a raw JSON object's keys in a stable order, so every
// diagnostic this file emits is deterministic.
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedPropNames(m map[string]model.PropSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSlotNames(m map[string]model.SlotSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// componentScopes returns every checkable node tree — the scenes plus the
// component templates (keyed "component:<name>") — with a sorted scope list so
// the checks that walk them emit diagnostics in a stable order.
func componentScopes(app *model.App) ([]string, map[string]*model.Node) {
	scopes := make([]string, 0, len(app.Scenes)+len(app.Components))
	roots := make(map[string]*model.Node, len(app.Scenes)+len(app.Components))
	for id, root := range app.Scenes {
		roots[id] = root
		scopes = append(scopes, id)
	}
	for name, root := range app.Components {
		roots["component:"+name] = root
		scopes = append(scopes, "component:"+name)
	}
	sort.Strings(scopes)
	return scopes, roots
}

// walkSceneNodes visits every node reachable from n, including renderItem
// templates and `when` branches.
func walkSceneNodes(n *model.Node, fn func(*model.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		walkSceneNodes(c, fn)
	}
	walkSceneNodes(n.Template, fn)
	walkSceneNodes(n.Then, fn)
	walkSceneNodes(n.Else, fn)
}

// checkTimers statically validates timer nodes across all scenes and
// components: a timer needs an id (the client's idempotency key), a schedule
// (`every` for repetition or `after` for a one-shot, in ms — `every` below
// render.TimerMinEveryMS is clamped at render time), and an onTick invoke
// naming a loaded action.
func checkTimers(app *model.App, diags *[]string) {
	scopes, roots := componentScopes(app)
	for _, scope := range scopes {
		walkSceneNodes(roots[scope], func(n *model.Node) {
			if n.Type != "timer" {
				return
			}
			if n.ID == "" {
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] timer 节点缺少 id:id 是重渲/morph 后去重的幂等键,缺失时该 timer 不会被调度。", scope))
			}
			every, everyLit := timerMS(n.Props["every"])
			after, afterLit := timerMS(n.Props["after"])
			switch {
			case everyLit && afterLit && every > 0 && after > 0:
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] timer (id: %q) 同时声明了 every 和 after,every(周期触发)优先,after 将被忽略。", scope, n.ID))
			case (!everyLit || every <= 0) && (!afterLit || after <= 0):
				if !isDynamicProp(n.Props["every"]) && !isDynamicProp(n.Props["after"]) {
					*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] timer (id: %q) 需要 every(毫秒周期)或 after(毫秒一次性延时)之一,否则不会触发。", scope, n.ID))
				}
			}
			if everyLit && every > 0 && every < render.TimerMinEveryMS {
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] timer (id: %q) 的 every=%dms 低于下限 %dms(防自我拒绝服务),渲染时将被钳制为 %dms。", scope, n.ID, every, render.TimerMinEveryMS, render.TimerMinEveryMS))
			}
			tick, hasTick := n.Props["onTick"]
			if !hasTick {
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] timer (id: %q) 缺少 onTick,触发时将无事可做。", scope, n.ID))
				return
			}
			name := ""
			switch t := tick.(type) {
			case string:
				name = t
			case map[string]any:
				name = asString(t["name"])
			}
			if !actionRefKnown(app, name) {
				*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] timer (id: %q) 的 onTick 引用了不存在的动作 %q。", scope, n.ID, name))
			}
		})
	}
}

// timerMS reads a literal millisecond prop value; lit is false for absent or
// non-numeric (e.g. a {{...}} binding) values.
func timerMS(v any) (ms int, lit bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case string:
		if t == "" || strings.Contains(t, "{{") {
			return 0, false
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return int(f), true
	}
	return 0, false
}

// isDynamicProp reports whether a prop value is a {{...}} binding (statically
// unknowable, so numeric checks must not fire on it).
func isDynamicProp(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "{{")
}

// stateVars maps the manifest's globalState schema onto the identifier names
// visible to expressions, for expr.Check. Scene bindings see "state.count";
// action expressions additionally see bare "count" (Runtime.Dispatch copies
// top-level state keys into the context), enabled via bare.
func stateVars(schema map[string]string, bare bool) map[string]string {
	vars := make(map[string]string, len(schema)*2+3)
	// The responsive viewport variables (see the `when` node) are always in
	// scope, so `{{ viewport.width >= 768 }}` type-checks like state does.
	vars["viewport.width"] = "number"
	vars["viewport.height"] = "number"
	vars["viewport.orientation"] = "string"
	for k, t := range schema {
		vars["state."+k] = t
		if bare {
			vars[k] = t
		}
	}
	return vars
}

// LoadFile loads a single scene file (no app-level state binding).
func LoadFile(path string) (*model.App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	app := &model.App{Scenes: map[string]*model.Node{}, Actions: map[string]*model.Action{}, Entry: "main"}
	if asString(doc["type"]) == "scene" {
		if root, ok := doc["root"].(map[string]any); ok {
			sceneID := asString(doc["id"])
			app.Scenes[sceneID] = buildNode(root, &app.Diagnostics, sceneID, nil, nil)
			app.Entry = sceneID
		}
	}
	return app, nil
}

func collect(dir string) ([]map[string]any, error) {
	var out []map[string]any
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			return nil // ignore malformed / non-object json
		}
		if asString(doc["type"]) == "test" {
			return nil
		}
		out = append(out, doc)
		return nil
	})
	return out, err
}

// parseMenuItems reads a JSON array of menu items.
func parseMenuItems(raw any) []model.MenuItem {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []model.MenuItem
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, model.MenuItem{
			ID: asString(m["id"]), Title: asString(m["title"]), Icon: asString(m["icon"]),
			Shortcut: asString(m["shortcut"]), Role: asString(m["role"]), Separator: asBool(m["separator"]), Items: parseMenuItems(m["items"]),
		})
	}
	return out
}

// parseMenuGroups reads the menu-bar groups.
func parseMenuGroups(raw any) []model.MenuGroup {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []model.MenuGroup
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, model.MenuGroup{Title: asString(m["title"]), Items: parseMenuItems(m["items"])})
	}
	return out
}

func applyManifest(app *model.App, doc map[string]any, diags *[]string) {
	app.ID = asString(doc["id"])
	app.Name = asString(doc["name"])
	app.Entry = asString(doc["entry"])
	app.DefaultLocale = asString(doc["defaultLocale"])
	app.Theme = asString(doc["theme"])
	app.Branding = true // default on; qorm.json "branding":false removes the metadata note
	if v, ok := doc["branding"]; ok {
		app.Branding = asBool(v)
	}
	// Plugin (middle-layer) ABI compatibility: warn if the app was authored
	// against an incompatible qormext contract major. Non-fatal — the app still
	// loads; the custom native ops just may not behave.
	if app.PluginABI = asString(doc["pluginABI"]); app.PluginABI != "" && !qormext.CompatibleABI(app.PluginABI) {
		*diags = append(*diags, fmt.Sprintf("error: pluginABI %q is incompatible with this runtime's plugin ABI v%d — the app's custom native ops may not work", app.PluginABI, qormext.ABIVersion))
	}
	if gs, ok := doc["globalState"].(map[string]any); ok {
		app.GlobalState.Schema = map[string]string{}
		if sch, ok := gs["schema"].(map[string]any); ok {
			for k, v := range sch {
				app.GlobalState.Schema[k] = asString(v)
			}
		}
		if init, ok := gs["initial"].(map[string]any); ok {
			app.GlobalState.Initial = init
		}
		// Derived values may be declared beside the state they derive from…
		applyComputed(app, gs["computed"], "globalState.computed", diags)
	}
	// …or at the manifest's top level, which is the canonical spelling
	// ManifestToJSON writes back. Both fill the same map; a name declared twice
	// is diagnosed and the first declaration wins.
	applyComputed(app, doc["computed"], "computed", diags)
	if ws, ok := doc["widgets"].([]any); ok {
		for _, it := range ws {
			if m, ok := it.(map[string]any); ok {
				w := model.Widget{ID: asString(m["id"]), Name: asString(m["name"]), Title: asString(m["title"])}
				if ls, ok := m["lines"].([]any); ok {
					for _, it := range ls {
						if lm, ok := it.(map[string]any); ok {
							w.Lines = append(w.Lines, model.WidgetLine{Label: asString(lm["label"]), Value: asString(lm["value"])})
						}
					}
				}
				app.Widgets = append(app.Widgets, w)
			}
		}
	}
	if comps, ok := doc["components"].(map[string]any); ok {
		if app.Components == nil {
			app.Components = map[string]*model.Node{}
		}
		// Schema was parsed above (globalState precedes components in this
		// function), so component expressions are type-checked too.
		compVars := stateVars(app.GlobalState.Schema, false)
		names := make([]string, 0, len(comps))
		for name := range comps {
			names = append(names, name)
		}
		sort.Strings(names) // stable diagnostics order
		for _, name := range names {
			if m, ok := comps[name].(map[string]any); ok {
				defineComponent(app, name, m, compVars, diags)
			}
		}
	}
	if dt, ok := doc["designTokens"].(map[string]any); ok {
		app.DesignTokens = map[string]model.DesignToken{}
		for name, def := range dt {
			m, ok := def.(map[string]any)
			if !ok {
				continue
			}
			app.DesignTokens[name] = model.DesignToken{
				Type:    asString(m["type"]),
				Value:   asString(m["value"]),
				Enforce: asBool(m["enforce"]),
			}
		}
	}
	if scs, ok := doc["shortcuts"].([]any); ok {
		for _, it := range scs {
			if m, ok := it.(map[string]any); ok {
				app.Shortcuts = append(app.Shortcuts, model.Shortcut{
					ID:       asString(m["id"]),
					Title:    asString(m["title"]),
					Subtitle: asString(m["subtitle"]),
					Icon:     asString(m["icon"]),
				})
			}
		}
	}
	if plats, ok := doc["platforms"].(map[string]any); ok {
		if desk, ok := plats["desktop"].(map[string]any); ok {
			app.DesktopMenu = parseMenuGroups(desk["menu"])
			if tr, ok := desk["tray"].(map[string]any); ok {
				app.Tray = model.TrayConfig{Icon: asString(tr["icon"]), Tip: asString(tr["tip"]), Items: parseMenuItems(tr["items"])}
			}
			if w, ok := desk["window"].(map[string]any); ok {
				app.Window = model.Window{
					Width:       int(asFloat(w["width"])),
					Height:      int(asFloat(w["height"])),
					Title:       asString(w["title"]),
					Resizable:   asBool(w["resizable"]),
					Chromeless:  asBool(w["chromeless"]),
					Transparent: asBool(w["transparent"]),
					HideLog:     asBool(w["hideLog"]),
					HideTray:    asBool(w["hideTray"]),
				}
			}
		}
	}
}

// applyComputed merges one "computed" declaration object (name -> expression)
// into the app. where names the spelling it came from, for the duplicate
// diagnostic; a non-object declaration is reported and ignored.
func applyComputed(app *model.App, raw any, where string, diags *[]string) {
	if raw == nil {
		return
	}
	m, ok := raw.(map[string]any)
	if !ok {
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("error: %q 应为对象(派生值名 -> 表达式,如 {\"total\": \"{{ sum(state.prices) }}\"}),已忽略。", where))
		}
		return
	}
	for _, name := range sortedKeys(m) {
		if app.Computed == nil {
			app.Computed = map[string]string{}
		}
		if _, dup := app.Computed[name]; dup {
			if diags != nil {
				*diags = append(*diags, fmt.Sprintf("error: computed 派生值 %q 在 globalState.computed 与顶层 computed 中被重复声明,仅保留最先出现的声明。", name))
			}
			continue
		}
		app.Computed[name] = asString(m[name])
	}
}

// BuildNode builds a node tree from a raw JSON object (exported for patch ops).
func BuildNode(m map[string]any) *model.Node { return buildNode(m, nil, "", nil, nil) }

// valueWidgets are the node types whose renderer legitimately consumes the
// `value` attribute (two-way state binding or a display value) — see the
// corresponding renderer funcs in internal/render. Only types OUTSIDE this set
// get the "misconfigured 'value'" diagnostic: for anything else (text, view,
// button, ...) `value` is silently ignored by the renderer and is almost
// always a mistaken stand-in for `text` / `{{state.x}}`. Keep this in sync
// with the render dispatch's value-reading cases (aliases included).
var valueWidgets = map[string]bool{
	// text inputs and pickers (render_input.go / render_widgets.go)
	"input": true, "textarea": true, "select": true, "dropdown": true,
	"textformfield": true, "autocomplete": true, "searchbar": true,
	"dropdownbutton": true,
	"picker":         true, "cupertinopicker": true,
	"datepicker": true, "cupertinodatepicker": true,
	"timepicker": true, "cupertinotimepicker": true,
	// toggles and choices
	"checkbox": true, "switch": true, "radio": true, "slider": true,
	"segmented": true, "slidingsegmentedcontrol": true, "cupertinoslidingsegmentedcontrol": true,
	"switchlisttile": true, "checkboxlisttile": true, "radiolisttile": true,
	// navigation selection state
	"bottomnav": true, "bottomnavigationbar": true, "navigationbar": true,
	"navigationrail": true, "navigationdrawer": true,
	// display widgets whose value IS the datum
	"progress": true, "circularprogress": true, "circularprogressindicator": true,
	"rating": true, "stat": true, "metric": true,
	// hardware capture widgets bind their result via value (render_gesture.go)
	"camera": true, "location": true, "geolocation": true,
	"recorder": true, "audiorecorder": true,
	"biometric": true, "faceid": true, "fingerprint": true,
}

// buildNode builds one node. vars is the identifier -> declared-type map for
// static expression type checking (nil disables it, e.g. for patch ops).
// scope is the set of bare names bound by enclosing renderItem templates
// (item/index/first/last or their `as`-derived forms, accumulated across
// nesting) — legal dot-less bindings there, so the "add a state./prop. prefix"
// suggestion must not fire on them. nil outside any template.
func buildNode(m map[string]any, diags *[]string, sceneID string, vars map[string]string, scope map[string]bool) *model.Node {
	nodeID := asString(m["id"])
	nodeType := asString(m["type"])
	// A `type` carrying a {{binding}} — {"type":"{{item.kind}}"} — is a
	// polymorphic node: the renderer evaluates it against the live scope and
	// dispatches on the RESULT (see resolveType in internal/render), so the
	// widget kind is unknowable here. The binding is stored verbatim (round-trip
	// safe: NodeToJSON writes n.Type back out unchanged), and every static check
	// that keys off the widget kind must stand down for it rather than judge the
	// literal "{{item.kind}}" — checks that do not depend on the kind still run.
	boundType := strings.Contains(nodeType, "{{")

	if diags != nil {
		// 校验 on 属性
		if _, hasOn := m["on"]; hasOn {
			*diags = append(*diags, fmt.Sprintf("[Scene: %s] 节点 (id: %q, type: %q) 使用了已弃用的 'on' 属性（如 on: {press: ...}）。请直接使用 'onPress' 或 'onChange'。", sceneID, nodeID, nodeType))
		}

		// 校验 value 属性：仅对渲染器不消费 value 的节点类型告警
		// （消费 value 的控件见 valueWidgets——那里 value 是双向绑定的正规 API）。
		// type 为绑定时跳过：节点类型渲染期才定，可能正好落在 valueWidgets 内，
		// 此时按字面量 "{{...}}" 判定只会误报。
		if val, hasVal := m["value"]; hasVal && !boundType {
			valStr := asString(val)
			if valStr != "" && !valueWidgets[nodeType] {
				*diags = append(*diags, fmt.Sprintf("[Scene: %s] 节点 (id: %q, type: %q) 错误地配置了 'value': %q。普通文本节点请使用 'text' 属性，状态绑定请使用 '{{state.xxx}}'。", sceneID, nodeID, nodeType, valStr))
			}
		}

		// 校验 style 中的未知键：渲染器只识别固定的键集合（render.KnownStyleKeys），
		// 未知键会被静默忽略，这里给出非致命告警。排序保证诊断顺序稳定。
		if style, hasStyle := m["style"].(map[string]any); hasStyle {
			var unknown []string
			for k := range style {
				if !render.KnownStyleKeys[k] {
					unknown = append(unknown, k)
				}
			}
			sort.Strings(unknown)
			for _, k := range unknown {
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] 节点 (id: %q, type: %q) 的 style 包含未知键 %q，渲染器将忽略该键。", sceneID, nodeID, nodeType, k))
			}
		}

		// 校验表达式格式（如非 state. 或 prop. 的绑定）与静态类型
		checkExpressions(m, diags, sceneID, nodeID, vars, scope)
	}

	n := &model.Node{
		Type:        nodeType,
		ID:          nodeID,
		Text:        asString(m["text"]),
		Label:       asString(m["label"]),
		Placeholder: asString(m["placeholder"]),
		Value:       asString(m["value"]),
		Props:       m,
	}
	if s, ok := m["style"].(map[string]any); ok {
		n.Style = s
	}
	if l, ok := m["layout"].(map[string]any); ok {
		n.Layout = l
	}
	n.OnPress = parseInvoke(m["onPress"], diags, sceneID, nodeID, "onPress")
	n.OnChange = parseInvoke(m["onChange"], diags, sceneID, nodeID, "onChange")
	if ri, ok := m["renderItem"].(map[string]any); ok {
		// A renderItem template runs with the item bound into the expression
		// scope under `as` (default "item") plus index/first/last. Resolve the
		// names through the renderer's own ListAliasNames so the static checks
		// can never drift from what actually gets bound at render time, and
		// warn when an explicit alias is unusable (reserved root like "state",
		// or not a plain identifier) — the renderer silently falls back to
		// "item" for those.
		alias, idxKey, firstKey, lastKey := render.ListAliasNames(asString(m["as"]))
		if diags != nil {
			if as := asString(m["as"]); as != "" && as != "item" && alias == "item" {
				*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] 节点 (id: %q) 的 renderItem 别名 as: %q 不可用（保留名或非法标识符），渲染器将回退为默认的 \"item\"。", sceneID, nodeID, as))
			}
		}
		tScope := make(map[string]bool, len(scope)+4)
		for k := range scope {
			tScope[k] = true
		}
		tScope[alias], tScope[idxKey], tScope[firstKey], tScope[lastKey] = true, true, true, true
		n.Template = buildNode(ri, diags, sceneID, vars, tScope)
	}
	n.Data = asString(m["data"])
	// "when" node: responsive conditional — condition picks then/else subtree.
	if nodeType == "when" {
		n.Condition = asString(m["condition"])
		// A non-empty condition without a {{...}} binding is a constant: the
		// renderer treats any non-empty string as truthy, so the then branch
		// renders unconditionally and the else branch is dead. Warn at load
		// time — this is almost always a forgotten {{ }} around the expression.
		if diags != nil && n.Condition != "" && !strings.Contains(n.Condition, "{{") {
			*diags = append(*diags, fmt.Sprintf("warning: [Scene: %s] 节点 (id: %q) 的 when condition %q 不含 {{...}} 绑定：非空字符串恒为真，将永远渲染 then 分支。请写成表达式绑定，如 \"{{ %s }}\"。", sceneID, nodeID, n.Condition, n.Condition))
		}
		if tm, ok := m["then"].(map[string]any); ok {
			n.Then = buildNode(tm, diags, sceneID, vars, scope)
		}
		if em, ok := m["else"].(map[string]any); ok {
			n.Else = buildNode(em, diags, sceneID, vars, scope)
		}
	}
	if kids, ok := m["children"].([]any); ok {
		for _, k := range kids {
			if km, ok := k.(map[string]any); ok {
				n.Children = append(n.Children, buildNode(km, diags, sceneID, vars, scope))
			}
		}
	}
	return n
}

func parseInvoke(v any, diags *[]string, sceneID, nodeID, eventName string) *model.Invoke {
	// String shorthand: "onPress": "increment" invokes that action with no args.
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		if diags != nil && strings.HasPrefix(s, "scene://") {
			*diags = append(*diags, fmt.Sprintf("[Scene: %s] 节点 (id: %q) 的 %s 动作使用了已弃用的 'scene://' 协议前缀: %q。请直接指定目标场景 ID (如 'main')。", sceneID, nodeID, eventName, s))
			s = strings.TrimPrefix(s, "scene://")
		}
		return &model.Invoke{Name: s, Args: map[string]string{}}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	name := asString(m["name"])
	if diags != nil && strings.HasPrefix(name, "scene://") {
		*diags = append(*diags, fmt.Sprintf("[Scene: %s] 节点 (id: %q) 的 %s 动作使用了已弃用的 'scene://' 协议前缀: %q。请直接指定目标场景 ID (如 'main')。", sceneID, nodeID, eventName, name))
		name = strings.TrimPrefix(name, "scene://")
	}
	inv := &model.Invoke{Name: name, Args: map[string]string{}}
	if args, ok := m["args"].(map[string]any); ok {
		for k, v := range args {
			inv.Args[k] = asString(v)
		}
	}
	return inv
}

// maxStepNesting is the loader-side cap on nested step lists (`if` then/else,
// http onSuccess/onError); the runtime enforces the same limit at dispatch
// time. Deeper nests are dropped with an error diagnostic.
const maxStepNesting = 32

// maxRenderSteps is the advisory ceiling on `render` steps in one action. Each
// one publishes a live-sync frame, and a subscriber whose buffer is full simply
// drops frames — so past a handful the extra frames cost bandwidth without ever
// being seen. The runtime's own hard cap (maxFrames) is far higher; this is the
// authoring hint, not the safety net.
const maxRenderSteps = 8

func buildAction(doc map[string]any, diags *[]string, vars map[string]string) *model.Action {
	actID := asString(doc["id"])
	act := &model.Action{ID: actID}
	act.Steps = buildSteps(doc["steps"], diags, actID, vars, 0)
	if diags != nil {
		if n := countRenderSteps(act.Steps); n > maxRenderSteps {
			*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 有 %d 个 'render' 步骤(建议不超过 %d 个):每个 render 都会推送一帧实时同步,过多的中间帧会被订阅者丢弃。", actID, n, maxRenderSteps))
		}
	}
	return act
}

// countRenderSteps counts `render` steps in a step tree, every nested step list
// included (branches, loop bodies, http result branches).
func countRenderSteps(steps []model.Step) int {
	n := 0
	walkSteps(steps, func(st model.Step) {
		if st.Type == "render" {
			n++
		}
	})
	return n
}

// buildSteps parses a raw JSON step array (recursively: `if` then/else and
// http onSuccess/onError nest step arrays), guarding nesting depth.
func buildSteps(raw any, diags *[]string, actID string, vars map[string]string, depth int) []model.Step {
	steps, ok := raw.([]any)
	if !ok {
		return nil
	}
	if depth > maxStepNesting {
		if diags != nil {
			*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 步骤嵌套超过 %d 层,更深的分支已被丢弃。", actID, maxStepNesting))
		}
		return nil
	}
	var out []model.Step
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, buildStep(sm, diags, actID, vars, depth))
	}
	return out
}

// buildStep parses one step object (see buildSteps for the nesting contract).
func buildStep(sm map[string]any, diags *[]string, actID string, vars map[string]string, depth int) model.Step {
	if diags != nil {
		checkStepExprTypes(sm, diags, actID, vars)
	}
	toVal := asString(sm["to"])
	if diags != nil && strings.HasPrefix(toVal, "scene://") {
		*diags = append(*diags, fmt.Sprintf("[Action: %s] 导航目标使用了已弃用的 'scene://' 协议前缀: %q。请直接指定目标场景 ID (如 'main')。", actID, toVal))
		toVal = strings.TrimPrefix(toVal, "scene://")
	}
	step := model.Step{
		Type:      asString(sm["type"]),
		Path:      asString(sm["path"]),
		Value:     asString(sm["value"]),
		MatchKey:  asString(sm["matchKey"]),
		Match:     asString(sm["match"]),
		Field:     asString(sm["field"]),
		URL:       asString(sm["url"]),
		Method:    asString(sm["method"]),
		Body:      asString(sm["body"]),
		Result:    asString(sm["result"]),
		Error:     asString(sm["error"]),
		Async:     sm["async"] == true,
		To:        toVal,
		Back:      sm["back"] == true,
		From:      asString(sm["from"]),
		Condition: asString(sm["condition"]),
		Name:      asString(sm["name"]),
		In:        asString(sm["in"]),
		As:        asString(sm["as"]),
	}
	if item, ok := sm["item"].(map[string]any); ok {
		step.Object = map[string]string{}
		for k, v := range item {
			step.Object[k] = asString(v)
		}
	}
	if hdr, ok := sm["headers"].(map[string]any); ok {
		step.Headers = map[string]string{}
		for k, v := range hdr {
			step.Headers[k] = asString(v)
		}
	}
	if params, ok := sm["params"].(map[string]any); ok {
		step.Params = map[string]string{}
		for k, v := range params {
			step.Params[k] = asString(v)
		}
	}
	if args, ok := sm["args"].(map[string]any); ok {
		step.Args = map[string]string{}
		for k, v := range args {
			step.Args[k] = asString(v)
		}
	}
	// Nested branch step lists. checkStepExprTypes above only descends into
	// sub-MAPS, so expressions inside these ARRAYS are checked here, by the
	// recursive buildSteps walk.
	step.Then = buildSteps(sm["then"], diags, actID, vars, depth+1)
	step.Else = buildSteps(sm["else"], diags, actID, vars, depth+1)
	step.Steps = buildSteps(sm["steps"], diags, actID, vars, depth+1)
	step.OnSuccess = buildSteps(sm["onSuccess"], diags, actID, vars, depth+1)
	step.OnError = buildSteps(sm["onError"], diags, actID, vars, depth+1)
	if diags != nil {
		switch step.Type {
		case "if":
			if step.Condition == "" {
				*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 'if' 步骤缺少 condition,将永远执行 else 分支。", actID))
			} else if !strings.Contains(step.Condition, "{{") {
				*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 'if' 步骤的 condition %q 不含 {{...}} 绑定:非空字符串恒为真,将永远执行 then 分支。请写成表达式绑定,如 \"{{ %s }}\"。", actID, step.Condition, step.Condition))
			}
		case "invoke":
			if step.Name == "" {
				*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 'invoke' 步骤缺少 name(目标动作 id),将被忽略。", actID))
			}
		case "forEach":
			if step.In == "" {
				*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 'forEach' 步骤缺少 in(要遍历的集合表达式,如 \"{{ state.items }}\"),循环体不会执行。", actID))
			} else if !strings.Contains(step.In, "{{") {
				*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 'forEach' 步骤的 in %q 不含 {{...}} 绑定:字面字符串不是数组,循环体不会执行。请写成表达式绑定,如 \"{{ %s }}\"。", actID, step.In, step.In))
			}
			if len(step.Steps) == 0 {
				*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 'forEach' 步骤没有 steps(循环体),遍历不会产生任何效果。", actID))
			}
			if alias, _, _, _ := render.ListAliasNames(step.As); step.As != "" && step.As != "item" && alias == "item" {
				*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 'forEach' 步骤的别名 as: %q 不可用(保留名或非法标识符),运行时将回退为默认的 \"item\"。", actID, step.As))
			}
		}
		// "async" only means something on a backend call: it is the request
		// round trip that moves to the background. Anywhere else the field is
		// silently inert, which reads like an unfulfilled promise — say so.
		if step.Async && !strings.HasPrefix(step.Type, "http.") {
			*diags = append(*diags, fmt.Sprintf("warning: [Action: %s] 只有 'http.*' 步骤支持 \"async\": true,%q 步骤上的该字段会被忽略。", actID, step.Type))
		}
	}
	return step
}

// checkStepExprTypes statically type-checks every `{{expr}}` in an action
// step's string fields (value/match/body/...) against the state schema.
func checkStepExprTypes(sm map[string]any, diags *[]string, actID string, vars map[string]string) {
	for _, v := range sm {
		strVal, ok := v.(string)
		if !ok {
			if subMap, ok := v.(map[string]any); ok {
				checkStepExprTypes(subMap, diags, actID, vars)
			}
			continue
		}
		forEachExpr(strVal, func(src string) {
			for _, mm := range expr.Check(src, vars) {
				*diags = append(*diags, fmt.Sprintf("error: [Action: %s] type mismatch: %s in {{ %s }}", actID, mm.Detail, mm.Expr))
			}
		})
	}
}

func checkExpressions(m map[string]any, diags *[]string, sceneID, nodeID string, vars map[string]string, scope map[string]bool) {
	isWhen := asString(m["type"]) == "when"
	for k, v := range m {
		if k == "children" || k == "renderItem" {
			continue
		}
		// A when node's branches are built as nodes themselves (buildNode
		// recurses), so their expressions are checked there — skip the raw maps
		// to avoid duplicate diagnostics attributed to the when node's id.
		if isWhen && (k == "then" || k == "else") {
			continue
		}
		strVal, ok := v.(string)
		if !ok {
			if subMap, ok := v.(map[string]any); ok {
				checkExpressions(subMap, diags, sceneID, nodeID, vars, scope)
			}
			continue
		}
		forEachExpr(strVal, func(src string) {
			// A string-literal expression (e.g. {{ '}}' }} or {{ "x" }}) is a
			// constant, not a bare state/prop binding, so the "add a prefix"
			// suggestion does not apply. Neither does it apply when the
			// expression references a name a renderItem template binds
			// ({{index}}, {{item}}, {{rowIndex + 1}}, ...) — those are the
			// list scope's own bare identifiers, not a forgotten prefix.
			isStrLit := len(src) > 0 && (src[0] == '\'' || src[0] == '"')
			if len(src) > 0 && !isStrLit && !strings.Contains(src, ".") &&
				!strings.Contains(src, "(") &&
				src != "true" && src != "false" && !usesScopeName(src, scope) {
				*diags = append(*diags, fmt.Sprintf("[Scene: %s] 节点 (id: %q) 表达式 %q 使用了非标准的绑定，属性值绑定建议加上前缀，如 'state.%s' 或 'prop.%s'。", sceneID, nodeID, "{{"+src+"}}", src, src))
			}
			for _, mm := range expr.Check(src, vars) {
				*diags = append(*diags, fmt.Sprintf("error: [Scene: %s] 节点 (id: %q) type mismatch: %s in {{ %s }}", sceneID, nodeID, mm.Detail, mm.Expr))
			}
		})
	}
}

// usesScopeName reports whether src references any in-scope renderItem
// template name (the item alias, index/first/last, or their `as`-derived
// forms) as a standalone identifier token. It tokenizes rather than substring-
// matches so a scope name "row" does not falsely claim "rows" or "arrow".
func usesScopeName(src string, scope map[string]bool) bool {
	if len(scope) == 0 {
		return false
	}
	start := -1
	for i := 0; i <= len(src); i++ {
		var c byte
		if i < len(src) {
			c = src[i]
		}
		isIdentChar := c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' ||
			(start >= 0 && '0' <= c && c <= '9')
		if isIdentChar {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if scope[src[start:i]] {
				return true
			}
			start = -1
		}
	}
	return false
}

// forEachExpr calls fn with each trimmed `{{ ... }}` expression inside s.
func forEachExpr(s string, fn func(src string)) {
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			return
		}
		end := expr.CloseIndex(s[start+2:])
		if end == -1 {
			return
		}
		fn(strings.TrimSpace(s[start+2 : start+2+end]))
		s = s[start+end+4:]
	}
}

// ---- coercion helpers ----

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return formatNumber(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// asFloat coerces a JSON number to float64. Integer Go types are accepted as
// well, so ManifestToJSON's output — which writes window dimensions as Go
// ints — can be re-read by applyManifest without an intermediate JSON
// encode/decode (JSON decoding yields float64, which also keeps working).
func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return 0
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
