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
			}
		case "action":
			act := buildAction(doc, &diags, actionVars)
			if act.ID != "" {
				app.Actions[act.ID] = act
			}
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
	actIDs := make([]string, 0, len(app.Actions))
	for id := range app.Actions {
		actIDs = append(actIDs, id)
	}
	sort.Strings(actIDs)
	for _, id := range actIDs {
		var walk func(steps []model.Step)
		walk = func(steps []model.Step) {
			for _, st := range steps {
				if st.Type == "invoke" && !actionRefKnown(app, st.Name) {
					*diags = append(*diags, fmt.Sprintf("error: [Action: %s] 'invoke' 步骤引用了不存在的动作 %q。", id, st.Name))
				}
				walk(st.Then)
				walk(st.Else)
				walk(st.OnSuccess)
				walk(st.OnError)
			}
		}
		walk(app.Actions[id].Steps)
	}
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
	scopes := make([]string, 0, len(app.Scenes)+len(app.Components))
	roots := map[string]*model.Node{}
	for id, root := range app.Scenes {
		roots[id] = root
		scopes = append(scopes, id)
	}
	for name, root := range app.Components {
		roots["component:"+name] = root
		scopes = append(scopes, "component:"+name)
	}
	sort.Strings(scopes)
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
	}
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
		app.Components = map[string]*model.Node{}
		// Schema was parsed above (globalState precedes components in this
		// function), so component expressions are type-checked too.
		compVars := stateVars(app.GlobalState.Schema, false)
		for name, def := range comps {
			if m, ok := def.(map[string]any); ok {
				app.Components[name] = buildNode(m, diags, "component:"+name, compVars, nil)
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

func buildAction(doc map[string]any, diags *[]string, vars map[string]string) *model.Action {
	actID := asString(doc["id"])
	act := &model.Action{ID: actID}
	act.Steps = buildSteps(doc["steps"], diags, actID, vars, 0)
	return act
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
		To:        toVal,
		Back:      sm["back"] == true,
		From:      asString(sm["from"]),
		Condition: asString(sm["condition"]),
		Name:      asString(sm["name"]),
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
