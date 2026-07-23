package loader

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/pkg/qormext"
)

func TestNodeToJSONNil(t *testing.T) {
	if got := NodeToJSON(nil); got != nil {
		t.Fatalf("NodeToJSON(nil) = %v, want nil", got)
	}
}

// TestNodeToJSONFullRoundTrip serialises a richly-populated node through JSON
// and back, asserting every typed field, both handlers, extra props and
// children survive — the guarantee the MCP patch surface depends on.
func TestNodeToJSONFullRoundTrip(t *testing.T) {
	src := map[string]any{
		"type": "column", "id": "root", "text": "T", "label": "L",
		"placeholder": "P", "value": "V",
		"style":    map[string]any{"color": "red", "padding": float64(8)},
		"layout":   map[string]any{"align": "center"},
		"onPress":  map[string]any{"name": "act", "args": map[string]any{"a": "1"}},
		"onChange": "changed",
		"checked":  true,            // extra prop carried through
		"options":  []any{"a", "b"}, // extra prop carried through
		"children": []any{
			map[string]any{"type": "text", "id": "c1", "text": "child"},
		},
	}
	n := BuildNode(src)
	out := NodeToJSON(n)

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reparsed map[string]any
	if err := json.Unmarshal(raw, &reparsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	re := BuildNode(reparsed)

	if re.Type != "column" || re.ID != "root" || re.Text != "T" || re.Label != "L" ||
		re.Placeholder != "P" || re.Value != "V" {
		t.Errorf("typed scalar fields lost: %+v", re)
	}
	if re.Style["color"] != "red" || re.Layout["align"] != "center" {
		t.Errorf("style/layout lost: style=%v layout=%v", re.Style, re.Layout)
	}
	if re.OnPress == nil || re.OnPress.Name != "act" || re.OnPress.Args["a"] != "1" {
		t.Errorf("onPress lost: %+v", re.OnPress)
	}
	if re.OnChange == nil || re.OnChange.Name != "changed" {
		t.Errorf("onChange string shorthand lost: %+v", re.OnChange)
	}
	if v, ok := re.Prop("checked"); !ok || v != true {
		t.Errorf("extra prop 'checked' lost: %v", re.Props)
	}
	if _, ok := re.Prop("options"); !ok {
		t.Errorf("extra prop 'options' lost: %v", re.Props)
	}
	if len(re.Children) != 1 || re.Children[0].ID != "c1" || re.Children[0].Text != "child" {
		t.Errorf("children lost: %+v", re.Children)
	}
}

// TestNodeToJSONSparse verifies empty fields are omitted (no noise for patch
// diffs): no children -> no "children" key, partial when -> no "else" key.
func TestNodeToJSONSparse(t *testing.T) {
	out := NodeToJSON(&model.Node{Type: "view"})
	for _, k := range []string{"children", "renderItem", "style", "layout", "onPress", "onChange", "id", "text"} {
		if _, ok := out[k]; ok {
			t.Errorf("sparse node must omit empty %q: %v", k, out)
		}
	}
	if out["type"] != "view" {
		t.Errorf("type must always be emitted: %v", out)
	}

	w := NodeToJSON(&model.Node{
		Type: "when", ID: "w", Condition: "{{ viewport.width > 1 }}",
		Then: &model.Node{Type: "text", ID: "t"},
	})
	if _, ok := w["else"]; ok {
		t.Errorf("when without Else must omit 'else': %v", w)
	}
	if w["condition"] != "{{ viewport.width > 1 }}" {
		t.Errorf("when condition lost: %v", w)
	}
}

// TestNodeToJSONTemplateAndData covers the list renderItem + data binding path.
func TestNodeToJSONTemplateAndData(t *testing.T) {
	src := map[string]any{
		"type": "list", "id": "lst",
		"data":       "{{ state.items }}",
		"renderItem": map[string]any{"type": "text", "id": "row", "text": "{{ prop.title }}"},
	}
	n := BuildNode(src)
	out := NodeToJSON(n)
	tmpl, ok := out["renderItem"].(map[string]any)
	if !ok || tmpl["id"] != "row" {
		t.Fatalf("renderItem not serialised: %v", out)
	}
	if out["data"] != "{{ state.items }}" {
		t.Fatalf("data binding lost: %v", out["data"])
	}
	re := BuildNode(out)
	if re.Template == nil || re.Template.ID != "row" || re.Data != "{{ state.items }}" {
		t.Fatalf("template/data round-trip lost: tmpl=%+v data=%q", re.Template, re.Data)
	}
}

// TestNodeToJSONKeepsDataWithoutTemplate verifies the BuildNode/NodeToJSON
// inverse keeps a `data` binding on a node that has no renderItem: "data" is
// in typedKeys, so it must be emitted from the struct field whenever it is
// set (an MCP patch op that serialises such a node must not lose the
// binding).
func TestNodeToJSONKeepsDataWithoutTemplate(t *testing.T) {
	n := BuildNode(map[string]any{"type": "list", "id": "l", "data": "{{ state.items }}"})
	if n.Data != "{{ state.items }}" {
		t.Fatalf("parse side must keep data, got %q", n.Data)
	}
	out := NodeToJSON(n)
	if out["data"] != "{{ state.items }}" {
		t.Fatalf("data must be serialised without a renderItem, got %v", out["data"])
	}
	if re := BuildNode(out); re.Data != "{{ state.items }}" {
		t.Fatalf("data binding must survive the round trip, got %q", re.Data)
	}
}

func TestManifestToJSONFull(t *testing.T) {
	app := &model.App{
		ID: "m", Name: "M", Entry: "home", DefaultLocale: "en", Theme: "dark",
		GlobalState: model.GlobalState{
			Schema:  map[string]string{"count": "number"},
			Initial: map[string]any{"count": float64(0)},
		},
		Window: model.Window{Width: 300, Height: 200, Title: "W", Resizable: true, Chromeless: true, Transparent: true},
		Components: map[string]*model.Node{
			"card": {Type: "text", ID: "c", Text: "hi"},
		},
		Shortcuts: []model.Shortcut{
			{ID: "s1", Title: "One"},
			{ID: "s2", Title: "Two", Subtitle: "sub", Icon: "ic"},
		},
	}
	m := ManifestToJSON(app)
	if m["type"] != "app" || m["id"] != "m" || m["name"] != "M" || m["entry"] != "home" ||
		m["defaultLocale"] != "en" || m["theme"] != "dark" {
		t.Errorf("scalar fields wrong: %v", m)
	}
	gs, _ := m["globalState"].(map[string]any)
	if gs == nil || gs["schema"].(map[string]any)["count"] != "number" ||
		gs["initial"].(map[string]any)["count"] != float64(0) {
		t.Errorf("globalState wrong: %v", m["globalState"])
	}
	win := m["platforms"].(map[string]any)["desktop"].(map[string]any)["window"].(map[string]any)
	if win["width"] != 300 || win["height"] != 200 || win["title"] != "W" ||
		win["resizable"] != true || win["chromeless"] != true || win["transparent"] != true {
		t.Errorf("window wrong: %v", win)
	}
	card := m["components"].(map[string]any)["card"].(map[string]any)
	if card["type"] != "text" || card["id"] != "c" || card["text"] != "hi" {
		t.Errorf("component wrong: %v", card)
	}
	scs := m["shortcuts"].([]any)
	if len(scs) != 2 {
		t.Fatalf("shortcuts wrong: %v", scs)
	}
	byID := map[string]map[string]any{}
	for _, s := range scs {
		sm := s.(map[string]any)
		byID[sm["id"].(string)] = sm
	}
	if byID["s1"]["title"] != "One" {
		t.Errorf("shortcut s1 wrong: %v", byID["s1"])
	}
	// putIf omits empty optional fields on s1.
	if _, ok := byID["s1"]["subtitle"]; ok {
		t.Errorf("empty subtitle must be omitted: %v", byID["s1"])
	}
	if byID["s2"]["subtitle"] != "sub" || byID["s2"]["icon"] != "ic" {
		t.Errorf("shortcut s2 wrong: %v", byID["s2"])
	}
}

func TestManifestToJSONMinimal(t *testing.T) {
	m := ManifestToJSON(&model.App{})
	if m["type"] != "app" {
		t.Fatalf("manifest must keep type=app: %v", m)
	}
	for _, k := range []string{"id", "name", "entry", "defaultLocale", "theme", "globalState", "platforms", "components", "shortcuts"} {
		if _, ok := m[k]; ok {
			t.Errorf("empty app must omit %q: %v", k, m)
		}
	}
}

// TestManifestRoundTripStable checks that the fields ManifestToJSON DOES
// support survive a parse -> serialise -> parse cycle, and that serialising is
// a fixpoint (second pass emits an identical document). Window width/height
// are excluded: they are the subject of TestManifestWindowDimsLossy below.
func TestManifestRoundTripStable(t *testing.T) {
	doc := map[string]any{
		"type": "app", "id": "rt", "name": "RT", "entry": "home",
		"defaultLocale": "en", "theme": "dark",
		"globalState": map[string]any{
			"schema":  map[string]any{"n": "number"},
			"initial": map[string]any{"n": float64(1)},
		},
		"components": map[string]any{"card": map[string]any{"type": "text", "id": "c"}},
		"shortcuts":  []any{map[string]any{"id": "s", "title": "S", "subtitle": "b", "icon": "i"}},
	}
	app1 := FromDocs([]map[string]any{doc})
	out := ManifestToJSON(app1)
	app2 := FromDocs([]map[string]any{out})

	if app2.ID != "rt" || app2.Name != "RT" || app2.Entry != "home" ||
		app2.DefaultLocale != "en" || app2.Theme != "dark" {
		t.Errorf("scalars lost: %+v", app2)
	}
	if app2.GlobalState.Schema["n"] != "number" || app2.GlobalState.Initial["n"] != float64(1) {
		t.Errorf("globalState lost: %+v", app2.GlobalState)
	}
	if app2.Components["card"] == nil || app2.Components["card"].ID != "c" {
		t.Errorf("components lost: %+v", app2.Components)
	}
	if len(app2.Shortcuts) != 1 || app2.Shortcuts[0].Subtitle != "b" {
		t.Errorf("shortcuts lost: %+v", app2.Shortcuts)
	}

	if out2 := ManifestToJSON(app2); !reflect.DeepEqual(out, out2) {
		t.Errorf("serialise is not a fixpoint:\nfirst  %v\nsecond %v", out, out2)
	}
}

// TestManifestWindowDimsRoundTrip verifies the window dimensions survive the
// documented FromDocs(ManifestToJSON(app)) inverse without an intermediate
// JSON encode/decode: ManifestToJSON emits the dims as Go ints, and asFloat
// accepts integer Go types (JSON decoding yields float64, which must keep
// working too).
func TestManifestWindowDimsRoundTrip(t *testing.T) {
	app1 := &model.App{
		ID:     "w",
		Window: model.Window{Width: 300, Height: 200, Title: "T"},
	}
	out := ManifestToJSON(app1)
	win := out["platforms"].(map[string]any)["desktop"].(map[string]any)["window"].(map[string]any)
	if win["width"] != 300 || win["height"] != 200 {
		t.Fatalf("window dims must be serialised, got %v", win)
	}

	// Direct round trip (no intermediate JSON encode): the int dims are read
	// back exactly instead of being zeroed.
	app2 := FromDocs([]map[string]any{out})
	if app2.Window.Width != 300 || app2.Window.Height != 200 {
		t.Fatalf("window dims must survive the direct round trip, got %+v", app2.Window)
	}
	if app2.Window.Title != "T" {
		t.Errorf("window title must survive: %+v", app2.Window)
	}

	// The JSON-decoded form of the SAME document (dims as float64) must keep
	// working as well.
	raw, _ := json.Marshal(out)
	var viaJSON map[string]any
	_ = json.Unmarshal(raw, &viaJSON)
	app3 := FromDocs([]map[string]any{viaJSON})
	if app3.Window.Width != 300 || app3.Window.Height != 200 {
		t.Errorf("JSON round trip must keep dims, got %+v", app3.Window)
	}
}

// TestManifestToJSONRoundTripsManifestSections verifies ManifestToJSON (and
// therefore AppToDocs / bundle.FromApp) emits branding:false, designTokens,
// pluginABI, widgets and the desktop menu/tray, so reloading the emitted
// manifest keeps branding off and preserves the design-token system + ABI.
func TestManifestToJSONRoundTripsManifestSections(t *testing.T) {
	doc := map[string]any{
		"type": "app", "id": "lossy", "name": "L", "branding": false,
		"pluginABI":    "1",
		"designTokens": map[string]any{"color.primary": map[string]any{"type": "color", "value": "#fff", "enforce": true}},
		"widgets":      []any{map[string]any{"id": "w", "name": "W"}},
		"platforms": map[string]any{"desktop": map[string]any{
			"menu": []any{map[string]any{"title": "File"}},
			"tray": map[string]any{"icon": "t.png"},
		}},
	}
	app1 := FromDocs([]map[string]any{doc})
	if app1.Branding || len(app1.DesignTokens) != 1 || len(app1.Widgets) != 1 ||
		len(app1.DesktopMenu) != 1 || app1.Tray.Icon != "t.png" || app1.PluginABI != "1" {
		t.Fatalf("precondition: manifest fields must parse, got %+v", app1)
	}

	out := ManifestToJSON(app1)
	if out["branding"] != false {
		t.Errorf("branding:false must be serialised, got %v", out["branding"])
	}
	if out["pluginABI"] != "1" {
		t.Errorf("pluginABI must be serialised, got %v", out["pluginABI"])
	}
	toks, _ := out["designTokens"].(map[string]any)
	tok, _ := toks["color.primary"].(map[string]any)
	if tok == nil || tok["type"] != "color" || tok["value"] != "#fff" || tok["enforce"] != true {
		t.Errorf("designTokens must be serialised, got %v", out["designTokens"])
	}
	ws, _ := out["widgets"].([]any)
	if len(ws) != 1 {
		t.Fatalf("widgets must be serialised, got %v", out["widgets"])
	}
	desk, _ := out["platforms"].(map[string]any)
	if desk == nil {
		t.Fatalf("menu/tray platforms must be serialised, got %v", out)
	}
	d := desk["desktop"].(map[string]any)
	menu, _ := d["menu"].([]any)
	if len(menu) != 1 || menu[0].(map[string]any)["title"] != "File" {
		t.Errorf("desktop menu must be serialised, got %v", d["menu"])
	}
	tray, _ := d["tray"].(map[string]any)
	if tray == nil || tray["icon"] != "t.png" {
		t.Errorf("tray must be serialised, got %v", d["tray"])
	}

	// Reloading the emitted manifest must preserve every section: branding
	// stays off; tokens, widgets, pluginABI, menu and tray all survive.
	app2 := FromDocs([]map[string]any{out})
	if app2.Branding {
		t.Error("branding:false must survive the round trip (no flip back to true)")
	}
	if dt := app2.DesignTokens["color.primary"]; dt.Type != "color" || dt.Value != "#fff" || !dt.Enforce {
		t.Errorf("designTokens lost on reload: %+v", app2.DesignTokens)
	}
	if len(app2.Widgets) != 1 || app2.Widgets[0].ID != "w" || app2.Widgets[0].Name != "W" {
		t.Errorf("widgets lost on reload: %+v", app2.Widgets)
	}
	if app2.PluginABI != "1" {
		t.Errorf("pluginABI lost on reload: %q", app2.PluginABI)
	}
	if len(app2.DesktopMenu) != 1 || app2.DesktopMenu[0].Title != "File" {
		t.Errorf("desktop menu lost on reload: %+v", app2.DesktopMenu)
	}
	if app2.Tray.Icon != "t.png" {
		t.Errorf("tray lost on reload: %+v", app2.Tray)
	}

	// Serialise must be a fixpoint over these sections.
	if out2 := ManifestToJSON(app2); !reflect.DeepEqual(out, out2) {
		t.Errorf("serialise is not a fixpoint:\nfirst  %v\nsecond %v", out, out2)
	}
}

// TestActionToJSONRoundTripSupportedFields verifies the step fields
// ActionToJSON serialises survive a parse -> serialise -> parse cycle.
func TestActionToJSONRoundTripSupportedFields(t *testing.T) {
	doc := map[string]any{
		"type": "action", "id": "a",
		"steps": []any{
			map[string]any{"type": "state.set", "path": "n", "value": "{{ n + 1 }}"},
			map[string]any{
				"type": "http.request", "url": "u", "method": "PUT", "body": "b",
				"headers": map[string]any{"H": "v"}, "result": "r", "error": "e",
			},
			map[string]any{"type": "state.toggle", "path": "p", "matchKey": "id", "match": "m", "field": "f"},
			map[string]any{"type": "state.appendObject", "path": "q", "item": map[string]any{"k": "v"}},
		},
	}
	app1 := FromDocs([]map[string]any{doc})
	out := ActionToJSON(app1.Actions["a"])
	if out["type"] != "action" || out["id"] != "a" {
		t.Fatalf("action doc header wrong: %v", out)
	}
	steps := out["steps"].([]any)
	if len(steps) != 4 {
		t.Fatalf("want 4 steps, got %v", steps)
	}
	// putIf omits empty fields: the state.set step carries no url/headers.
	s0 := steps[0].(map[string]any)
	for _, k := range []string{"url", "method", "body", "result", "error", "item", "headers"} {
		if _, ok := s0[k]; ok {
			t.Errorf("state.set step must omit empty %q: %v", k, s0)
		}
	}

	app2 := FromDocs([]map[string]any{out})
	if !reflect.DeepEqual(app1.Actions["a"].Steps, app2.Actions["a"].Steps) {
		t.Errorf("supported step fields do not round-trip:\nfirst  %+v\nsecond %+v",
			app1.Actions["a"].Steps, app2.Actions["a"].Steps)
	}
}

// TestActionToJSONRoundTripsNavigationFields verifies ActionToJSON (used by
// AppToDocs / bundle.FromApp) emits the navigate step's `to`, `back`, `from`
// and `params`, so a re-serialised app can still change scenes.
func TestActionToJSONRoundTripsNavigationFields(t *testing.T) {
	doc := map[string]any{
		"type": "action", "id": "nav",
		"steps": []any{
			map[string]any{
				"type": "navigate", "to": "settings", "back": true, "from": "1",
				"params": map[string]any{"userId": "{{ state.user }}"},
			},
		},
	}
	app1 := FromDocs([]map[string]any{doc})
	st := app1.Actions["nav"].Steps[0]
	if st.To != "settings" || !st.Back || st.From != "1" || st.Params["userId"] != "{{ state.user }}" {
		t.Fatalf("precondition: navigate step must parse, got %+v", st)
	}

	out := ActionToJSON(app1.Actions["nav"])
	ost := out["steps"].([]any)[0].(map[string]any)
	if ost["to"] != "settings" || ost["back"] != true || ost["from"] != "1" {
		t.Fatalf("navigate fields must be serialised, got %v", ost)
	}
	params, _ := ost["params"].(map[string]any)
	if params == nil || params["userId"] != "{{ state.user }}" {
		t.Fatalf("navigate params must be serialised, got %v", ost["params"])
	}

	// Reloading the emitted action must keep the step fully navigable:
	// target, back flag, source index and route params all survive.
	app2 := FromDocs([]map[string]any{out})
	st2 := app2.Actions["nav"].Steps[0]
	if st2.To != "settings" || !st2.Back || st2.From != "1" || st2.Params["userId"] != "{{ state.user }}" {
		t.Fatalf("navigate step must round-trip, got %+v", st2)
	}
}

func TestAppToDocsShape(t *testing.T) {
	app := &model.App{
		ID: "x", Entry: "main",
		Scenes: map[string]*model.Node{
			"main":  {Type: "view", ID: "r"},
			"other": {Type: "view", ID: "o"},
		},
		Actions: map[string]*model.Action{
			"a1": {ID: "a1"},
		},
	}
	docs := AppToDocs(app)
	if len(docs) != 4 {
		t.Fatalf("want 1 manifest + 2 scenes + 1 action docs, got %d", len(docs))
	}
	if docs[0]["type"] != "app" {
		t.Errorf("manifest must be the first doc (loader applies it first), got %v", docs[0])
	}
	scenes, actions := 0, 0
	ids := map[string]bool{}
	for _, d := range docs[1:] {
		switch d["type"] {
		case "scene":
			scenes++
			ids[d["id"].(string)] = true
		case "action":
			actions++
			ids[d["id"].(string)] = true
		}
	}
	if scenes != 2 || actions != 1 {
		t.Errorf("doc counts wrong: scenes=%d actions=%d", scenes, actions)
	}
	for _, want := range []string{"main", "other", "a1"} {
		if !ids[want] {
			t.Errorf("missing doc for %q in %v", want, ids)
		}
	}
}

// TestMissingEntrySceneDiagnosed verifies a manifest whose entry points at a
// nonexistent scene produces a diagnostic naming the bad entry, instead of
// being silently masked by EntryRoot's any-scene fallback.
func TestMissingEntrySceneDiagnosed(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "a", "entry": "settings"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "t"}},
	}
	app := FromDocs(docs)
	if app.Entry != "settings" {
		t.Fatalf("entry should be the (missing) declared scene, got %q", app.Entry)
	}
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "entry") && strings.Contains(d, `"settings"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing entry scene must be diagnosed, got %v", app.Diagnostics)
	}
	// The runtime still falls back to an existing scene so the app renders.
	if app.EntryRoot() == nil || app.EntryRoot().ID != "t" {
		t.Fatalf("EntryRoot fallback broken: %+v", app.EntryRoot())
	}
}

// TestBraceLiteralNoFalsePositiveBindingWarning verifies forEachExpr tracks
// quote state, so a "}}" inside a binding's string literal ({{ '}}' }}) is
// not treated as the closing delimiter: no spurious "non-standard binding"
// warning, and the whole expression is extracted for type-checking.
func TestBraceLiteralNoFalsePositiveBindingWarning(t *testing.T) {
	docs := []map[string]any{{
		"type": "scene", "id": "main",
		"root": map[string]any{"type": "text", "id": "t", "text": "x {{ '}}' }} y"},
	}}
	app := FromDocs(docs)
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a '}}' inside a string literal must not diagnose, got %v", app.Diagnostics)
	}
}

// TestManifestDesktopMenuTrayFullFidelity round-trips a manifest carrying a
// real desktop menu (groups + items with id/title/icon/shortcut/role,
// separators and a nested submenu, plus an itemless group) and a tray with
// items through ManifestToJSON -> FromDocs. This is the inverse of
// parseMenuGroups/parseMenuItems that the round-6 fix added
// (menuGroupsToJSON/menuItemsToJSON): every field must survive and serialise
// must be a fixpoint. Widgets (with and without lines), branding:false,
// pluginABI and the window dims (asFloat's Go-int path, since ManifestToJSON
// emits them as ints) ride along in the same fidelity pass.
func TestManifestDesktopMenuTrayFullFidelity(t *testing.T) {
	doc := map[string]any{
		"type": "app", "id": "desk", "name": "D", "entry": "home",
		"branding":  false,
		"pluginABI": strconv.Itoa(qormext.ABIVersion),
		"widgets": []any{
			map[string]any{
				"id": "w1", "name": "Disk", "title": "Disk usage",
				"lines": []any{
					map[string]any{"label": "Used", "value": "10 GB"},
					map[string]any{"label": "Free", "value": "90 GB"},
				},
			},
			map[string]any{"id": "w2", "name": "Clock"}, // no lines: the omit branch
		},
		"platforms": map[string]any{"desktop": map[string]any{
			"menu": []any{
				map[string]any{
					"title": "File",
					"items": []any{
						map[string]any{"id": "new", "title": "New", "icon": "new.png", "shortcut": "CmdOrCtrl+N"},
						map[string]any{"separator": true},
						map[string]any{
							"title": "Recent",
							"items": []any{
								map[string]any{"id": "r1", "title": "doc.qorm"},
								map[string]any{"title": "About", "role": "about"},
							},
						},
					},
				},
				map[string]any{"title": "Empty"}, // itemless group: the omit branch
			},
			"tray": map[string]any{
				"icon": "tray.png", "tip": "Running",
				"items": []any{
					map[string]any{"id": "show", "title": "Show"},
					map[string]any{"separator": true},
					map[string]any{"title": "Quit", "role": "quit", "shortcut": "CmdOrCtrl+Q"},
				},
			},
			"window": map[string]any{"width": float64(640), "height": float64(480), "title": "D"},
		}},
	}
	app1 := FromDocs([]map[string]any{doc})
	if len(app1.Diagnostics) != 0 {
		t.Fatalf("precondition: manifest must load clean, got %v", app1.Diagnostics)
	}
	if len(app1.DesktopMenu) != 2 || app1.Tray.Icon != "tray.png" || len(app1.Tray.Items) != 3 {
		t.Fatalf("precondition: menu/tray must parse, got menu=%+v tray=%+v", app1.DesktopMenu, app1.Tray)
	}

	out := ManifestToJSON(app1)

	// The emitted document must carry the menu items (not just the group
	// titles), the tray items, the widget lines and branding:false.
	if out["branding"] != false {
		t.Errorf("branding:false must be serialised, got %v", out["branding"])
	}
	d := out["platforms"].(map[string]any)["desktop"].(map[string]any)
	file := d["menu"].([]any)[0].(map[string]any)
	fileItems, _ := file["items"].([]any)
	if len(fileItems) != 3 {
		t.Fatalf("menu group items must be serialised, got %v", file["items"])
	}
	if _, ok := d["menu"].([]any)[1].(map[string]any)["items"]; ok {
		t.Errorf("itemless group must omit 'items': %v", d["menu"].([]any)[1])
	}
	trayOut, _ := d["tray"].(map[string]any)
	if trayOut == nil || len(trayOut["items"].([]any)) != 3 {
		t.Fatalf("tray items must be serialised, got %v", d["tray"])
	}
	w0 := out["widgets"].([]any)[0].(map[string]any)
	if lines, _ := w0["lines"].([]any); len(lines) != 2 {
		t.Errorf("widget lines must be serialised, got %v", w0)
	}
	if _, ok := out["widgets"].([]any)[1].(map[string]any)["lines"]; ok {
		t.Errorf("widget without lines must omit 'lines': %v", out["widgets"].([]any)[1])
	}

	// Reloading the emitted manifest must reproduce the model exactly —
	// nested submenus, roles, shortcuts, separators and all.
	app2 := FromDocs([]map[string]any{out})
	if len(app2.Diagnostics) != 0 {
		t.Errorf("reloaded manifest must stay clean, got %v", app2.Diagnostics)
	}
	if !reflect.DeepEqual(app1.DesktopMenu, app2.DesktopMenu) {
		t.Errorf("desktop menu lost fidelity:\nfirst  %+v\nsecond %+v", app1.DesktopMenu, app2.DesktopMenu)
	}
	if !reflect.DeepEqual(app1.Tray, app2.Tray) {
		t.Errorf("tray lost fidelity:\nfirst  %+v\nsecond %+v", app1.Tray, app2.Tray)
	}
	if !reflect.DeepEqual(app1.Widgets, app2.Widgets) {
		t.Errorf("widgets lost fidelity:\nfirst  %+v\nsecond %+v", app1.Widgets, app2.Widgets)
	}
	if app2.Branding {
		t.Error("branding:false must survive the round trip (no flip back to true)")
	}
	if app2.PluginABI != strconv.Itoa(qormext.ABIVersion) {
		t.Errorf("pluginABI lost on reload: %q", app2.PluginABI)
	}
	// ManifestToJSON wrote the dims as Go ints (no intermediate JSON encode),
	// so this reload exercises asFloat's integer path and must not zero them.
	if app2.Window.Width != 640 || app2.Window.Height != 480 || app2.Window.Title != "D" {
		t.Errorf("window lost on reload (asFloat int path): %+v", app2.Window)
	}
	if sub := app2.DesktopMenu[0].Items[2].Items; len(sub) != 2 || sub[1].Role != "about" {
		t.Errorf("nested submenu lost on reload: %+v", sub)
	}

	// Serialise is a fixpoint over these sections.
	if out2 := ManifestToJSON(app2); !reflect.DeepEqual(out, out2) {
		t.Errorf("serialise is not a fixpoint:\nfirst  %v\nsecond %v", out, out2)
	}
}

// TestEscapedQuoteBindingNoFalsePositiveBindingWarning verifies forEachExpr's
// quote tracking honours backslash escapes: an ESCAPED quote inside a
// binding's string literal ({{ 'a\'b' }}) must not terminate the literal, so
// the binding is extracted whole (no spurious "non-standard binding"
// diagnostic), and scanning continues to the FOLLOWING binding so its real
// expression is still type-checked. Without escape handling the first literal
// would swallow the rest of the string and the type error would go unreported.
func TestEscapedQuoteBindingNoFalsePositiveBindingWarning(t *testing.T) {
	docs := []map[string]any{
		{
			"type": "app", "id": "a", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"name": "string"}},
		},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{"type": "text", "id": "t", "text": "{{ 'a\\'b' }}{{ state.name - 1 }}"},
		},
	}
	app := FromDocs(docs)
	var mismatches, spurious []string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "非标准的绑定") {
			spurious = append(spurious, d)
		}
		if strings.Contains(d, "type mismatch") {
			mismatches = append(mismatches, d)
		}
	}
	if len(spurious) != 0 {
		t.Errorf("escaped-quote string literal must not warn as a bare binding: %v", spurious)
	}
	// The real expression after the literal must still be type-checked:
	// state.name is string, so `state.name - 1` is exactly one mismatch.
	if len(mismatches) != 1 || !strings.Contains(mismatches[0], "state.name - 1") {
		t.Fatalf("type-checking must run on the real expression after the escaped-quote literal, got %v", app.Diagnostics)
	}
	if len(app.Diagnostics) != 1 {
		t.Errorf("want exactly the one type-mismatch diagnostic, got %v", app.Diagnostics)
	}
}

// TestEscapedBraceInLiteralNoFalsePositiveBindingWarning covers a "}}" that
// is preceded by a backslash inside a binding's string literal (alongside an
// escaped quote): the escape skip keeps the literal intact, so the binding
// emits no diagnostic and the trailing real expression is still checked.
func TestEscapedBraceInLiteralNoFalsePositiveBindingWarning(t *testing.T) {
	docs := []map[string]any{
		{
			"type": "app", "id": "a", "entry": "main",
			"globalState": map[string]any{"schema": map[string]any{"name": "string"}},
		},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{"type": "text", "id": "t", "text": "x {{ 'a\\'b\\}}c' }} y {{ state.name - 2 }}"},
		},
	}
	app := FromDocs(docs)
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "非标准的绑定") {
			t.Errorf("literal with escaped quote + escaped }} must not warn: %v", d)
		}
	}
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "type mismatch") && strings.Contains(d, "state.name - 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("type-checking must still reach the real expression, got %v", app.Diagnostics)
	}
	if len(app.Diagnostics) != 1 {
		t.Errorf("want exactly the one type-mismatch diagnostic, got %v", app.Diagnostics)
	}
}
