package loader

// Serialization ROUND-TRIP IDENTITY property tests.
//
// model.App and the raw document list ([]map[string]any) are two hand-
// maintained representations of the same app, and keeping them in sync is
// the bug class round 3 fixed: *ToJSON / AppToDocs silently dropped fields
// that FromDocs expects — a node's Data binding without a renderItem
// template, the navigate step's To/Back/From/Params, the manifest's
// branding / designTokens / pluginABI / widgets / menu / tray sections, and
// the Window Width/Height that ManifestToJSON writes as Go ints (read back
// via asFloat's integer path). Every such drop is data loss the moment a
// patched app is re-serialised (bundle.FromApp, the MCP patch surface).
//
// These tests make that drift fail CI permanently:
//
//   - TestSerializeRoundTripIdentity: FromDocs(AppToDocs(app)) must be
//     field-equal to app for every field the doc format represents, and the
//     docs form (the canonical wire/patch form) must round-trip as the
//     identity: AppToDocs(FromDocs(AppToDocs(app))) == AppToDocs(app).
//   - TestSerializeRoundTripRegressionFields: one minimal subtest per
//     round-3 lost field, so a future regression points at the exact field.
//   - FuzzSerializeRoundtrip: the serialize pipeline canon = AppToDocs∘FromDocs
//     must be IDEMPOTENT on arbitrary parseable docs — canon(canon(d)) ==
//     canon(d) — so one app has exactly one serialised form and therefore
//     exactly one contentHash (complements FuzzFromDocs).
//   - TestSerializeRoundTripZeroDiagnostics: a well-formed app must
//     round-trip with ZERO diagnostics, so a future change that starts
//     warning on valid input fails loudly.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/pkg/qormext"
)

// roundTripApp builds a model.App exercising EVERY field the round-3 bugs
// touched plus a broad cross-section of the doc format. It is deliberately
// well-formed (entry scene exists, pluginABI compatible, state.-prefixed
// bindings declared in the schema, value only on input/select nodes, style
// keys from render.KnownStyleKeys) so it also feeds the zero-diagnostics
// invariant.
func roundTripApp() *model.App {
	return &model.App{
		ID:            "roundtrip",
		Name:          "Round Trip",
		Entry:         "main",
		DefaultLocale: "en",
		Theme:         "material",
		// The loader defaults branding to true when the key is absent, so an
		// explicit false is the only form that must survive the round trip.
		Branding:  false,
		PluginABI: strconv.Itoa(qormext.ABIVersion),
		GlobalState: model.GlobalState{
			Schema: map[string]string{
				"count": "number",
				"name":  "string",
				"items": "array",
			},
			Initial: map[string]any{
				"count": float64(0),
				"name":  "qorm",
				"items": []any{},
			},
		},
		DesignTokens: map[string]model.DesignToken{
			"color.primary": {Type: "color", Value: "#0a84ff", Enforce: true},
			"radius.md":     {Type: "number", Value: "12"}, // Enforce false: the omit branch
		},
		Widgets: []model.Widget{
			{
				ID: "w1", Name: "Counter", Title: "Count",
				Lines: []model.WidgetLine{{Label: "Count", Value: "0"}},
			},
			{ID: "w2", Name: "Plain"}, // no lines: the omit branch
		},
		DesktopMenu: []model.MenuGroup{
			{
				Title: "File",
				Items: []model.MenuItem{
					{ID: "new", Title: "New", Icon: "new.png", Shortcut: "CmdOrCtrl+N"},
					{Separator: true},
					{Title: "Recent", Items: []model.MenuItem{{ID: "r1", Title: "doc.qorm"}}},
				},
			},
			{Title: "Empty"}, // itemless group: the omit branch
		},
		Tray: model.TrayConfig{
			Icon: "tray.png", Tip: "Running",
			Items: []model.MenuItem{
				{ID: "show", Title: "Show"},
				{Title: "Quit", Role: "quit"},
			},
		},
		Window: model.Window{
			// Width/Height take the int path: ManifestToJSON emits Go ints and
			// applyManifest reads them back via asFloat (the round-3 zeroing bug).
			Width: 640, Height: 480, Title: "RT",
			Resizable: true, Chromeless: true, Transparent: true,
			HideLog: true, HideTray: true,
		},
		Components: map[string]*model.Node{
			// An undeclared component: serialised as its bare template, the
			// spelling every app used before declarations existed.
			"card": {Type: "text", ID: "card_text", Text: "{{ state.count }}"},
			// A component with a declared props/slots schema: serialised in the
			// declaration form ({props, slots, template}).
			"panel": {Type: "card", ID: "panel_root", Children: []*model.Node{
				{Type: "slot", ID: "panel_body", Props: map[string]any{"name": "body"}},
			}},
		},
		ComponentSchemas: map[string]*model.ComponentSchema{
			"panel": {
				Props: map[string]model.PropSpec{
					"title": {Type: "string"},                      // shorthand-equivalent
					"count": {Type: "number", Default: float64(3)}, // default branch
					"open":  {Type: "boolean", Required: true},     // required branch
					"extra": {},                                    // unconstrained branch
				},
				Slots: map[string]model.SlotSpec{
					"header": {},               // optional branch
					"body":   {Required: true}, // required branch
				},
			},
		},
		Shortcuts: []model.Shortcut{
			{ID: "s1", Title: "New"},
			{ID: "s2", Title: "Open", Subtitle: "Open recent", Icon: "star"},
		},
		Scenes: map[string]*model.Node{
			"main": {
				Type: "view", ID: "root",
				Layout: map[string]any{"align": "center"},
				Children: []*model.Node{
					{
						Type: "text", ID: "title", Text: "{{ state.count }}",
						Style: map[string]any{"color": "#333", "fontSize": float64(20)},
					},
					{
						Type: "input", ID: "name_in", Value: "{{ state.name }}", Placeholder: "Name",
						OnChange: &model.Invoke{Name: "save", Args: map[string]string{"v": "{{ state.name }}"}},
					},
					{
						// An untyped extra prop (options) carried through verbatim.
						Type: "select", ID: "plan_sel", Value: "{{ state.name }}",
						Props: map[string]any{"options": []any{"free", "pro"}},
					},
					// Round-3: a Data binding with NO renderItem template.
					{Type: "list", ID: "data_only", Data: "{{ state.items }}"},
					// A list with both data and a renderItem template.
					{
						Type: "list", ID: "lst", Data: "{{ state.items }}",
						Template: &model.Node{Type: "text", ID: "row", Text: "{{ prop.title }}"},
					},
					// A responsive when node with condition/then/else.
					{
						Type: "when", ID: "w", Condition: "{{ viewport.width >= 768 }}",
						Then: &model.Node{Type: "text", ID: "wide", Text: "wide"},
						Else: &model.Node{Type: "text", ID: "narrow", Text: "narrow"},
					},
					{
						Type: "button", ID: "go", Label: "Go",
						OnPress: &model.Invoke{Name: "openSettings", Args: map[string]string{"from": "'main'"}},
					},
					// The string-shorthand invoke (Args left nil on purpose).
					{Type: "button", ID: "inc", Label: "Add", OnPress: &model.Invoke{Name: "increment"}},
				},
			},
			"settings": {Type: "view", ID: "settings_root"},
		},
		Actions: map[string]*model.Action{
			"increment": {
				ID: "increment",
				Steps: []model.Step{
					{Type: "state.set", Path: "count", Value: "{{ count + 1 }}"},
				},
			},
			"save": {
				ID: "save",
				Steps: []model.Step{
					{Type: "state.set", Path: "name", Value: "{{ name }}"},
				},
			},
			"openSettings": {
				ID: "openSettings",
				// Round-3: a navigate step carrying To + Back + From + Params.
				Steps: []model.Step{
					{
						Type: "navigate", To: "settings", Back: true, From: "1",
						Params: map[string]string{"userId": "{{ state.name }}"},
					},
				},
			},
			"shuffle": {
				ID: "shuffle",
				Steps: []model.Step{
					{Type: "state.move", Path: "items", To: "0", From: "1"},
					{Type: "state.appendObject", Path: "items", Object: map[string]string{"title": "'x'"}},
					{Type: "state.toggle", Path: "items", MatchKey: "id", Match: "{{ id }}", Field: "done"},
				},
			},
			"sync": {
				ID: "sync",
				Steps: []model.Step{
					{
						Type: "http.request", URL: "https://api.example.com/x", Method: "PUT", Body: " ",
						Headers: map[string]string{"X-Token": "{{ state.name }}"},
						Result:  "resp", Error: "errMsg",
						// M4 governance fields on a real step. None of them
						// shows up in a rendered diff when it is lost: a
						// dropped `key` silently un-deduplicates, a dropped
						// `timeout` silently restores the 20s ceiling.
						Async: true, Key: "sync", TimeoutMS: 2500, Pending: "syncing",
						// http result branches with nested steps.
						OnSuccess: []model.Step{
							{Type: "state.set", Path: "name", Value: "{{ response.title }}"},
						},
						OnError: []model.Step{
							{Type: "state.set", Path: "name", Value: "{{ error }}"},
						},
					},
					{Type: "delay", DelayMS: 400},
					{Type: "state.set", Path: "name", Value: "'done'"},
				},
			},
			// `if` step with nested branches (a nested if inside else) and an
			// `invoke` step carrying args — the execution-model round-3-style
			// fields that must survive serialisation.
			"branchy": {
				ID: "branchy",
				Steps: []model.Step{
					{
						Type:      "if",
						Condition: "{{ state.count > 0 }}",
						Then:      []model.Step{{Type: "state.increment", Path: "count"}},
						Else: []model.Step{{
							Type:      "if",
							Condition: "{{ state.count < 0 }}",
							Then:      []model.Step{{Type: "state.clear", Path: "count"}},
						}},
					},
					{Type: "invoke", Name: "increment", Args: map[string]string{"count": "{{ state.count }}"}},
				},
			},
			// A `forEach` step: collection + item alias + a nested body.
			"bulk": {
				ID: "bulk",
				Steps: []model.Step{{
					Type: "forEach", In: "{{ state.items }}", As: "row",
					Steps: []model.Step{
						{Type: "state.updateWhere", Path: "items", MatchKey: "id",
							Match: "{{ row.id }}", Object: map[string]string{"done": "{{ true }}"}},
					},
				}},
			},
		},
		// Scene lifecycle: an onEnter hook on the entry scene.
		SceneEnter: map[string]*model.Invoke{
			"main": {Name: "increment", Args: map[string]string{"count": "{{ state.count }}"}},
		},
		// Routing: a guard protecting the settings scene, with redirect params.
		SceneGuards: map[string]*model.SceneGuard{
			"settings": {
				Condition: "{{ state.name != '' }}",
				Redirect:  "main",
				Params:    map[string]string{"next": "{{ 'settings' }}"},
			},
		},
		// Scene-level controls: key AND swipe bindings on the entry scene —
		// the declarative game control scheme; a round trip that dropped them
		// would ship a game with no controls.
		SceneKeys: map[string]map[string]string{
			"main": {"left": "increment", "space": "increment"},
		},
		SceneSwipes: map[string]map[string]string{
			"main": {"left": "increment", "down": "increment"},
		},
		// Derived values, including one that reads another.
		Computed: map[string]string{
			"itemCount": "{{ len(state.items) }}",
			"isEmpty":   "{{ computed.itemCount == 0 }}",
		},
	}
}

// TestSerializeRoundTripIdentity is the core property: AppToDocs must be the
// inverse of FromDocs for every field the doc format represents, and the docs
// form — the canonical wire/patch form bundle.FromApp hashes and the MCP
// surface edits — must round-trip as the identity.
func TestSerializeRoundTripIdentity(t *testing.T) {
	app := roundTripApp()

	docs := AppToDocs(app)
	if docs[0]["type"] != "app" {
		t.Fatalf("manifest must be the first doc (the loader applies it first), got %v", docs[0]["type"])
	}

	app2 := FromDocs(docs)
	checkAppFields(t, app, app2)

	// The docs fixpoint: re-serialising the rebuilt app must yield the exact
	// same doc set. This is the strongest guard — it fails on any field the
	// builder did not even think to check explicitly.
	docs2 := AppToDocs(app2)
	if d1, d2 := canonicalDocSet(t, docs), canonicalDocSet(t, docs2); !reflect.DeepEqual(d1, d2) {
		t.Errorf("AppToDocs is not a fixpoint over FromDocs:\nfirst:  %v\nsecond: %v", d1, d2)
	}
}

// checkAppFields asserts got is field-equal to want for every field the doc
// format represents.
//
// Fields intentionally NOT compared (they are not part of the docs
// representation, by design):
//   - BaseDir: set by LoadDir from the source directory; bundles and
//     in-memory apps leave it empty and AppToDocs must not fabricate one.
//   - Locales: loaded from <dir>/locales/*.json, never part of the doc list;
//     bundle.FromApp carries them alongside the docs instead.
//   - Diagnostics: recomputed by FromDocs on every load — covered by
//     TestSerializeRoundTripZeroDiagnostics.
//   - Node.Props directly: Props is the parse detail that holds the WHOLE raw
//     doc map (overlapping the typed fields); its semantic content is the set
//     of UNTYPED extras, compared via nodeExtras.
//
// nil vs empty maps/slices are treated as equivalent (normAnyMap /
// equivStrMap): serialise omits empty sections and the loader leaves the
// corresponding field nil, so e.g. an Invoke built with nil Args round-trips
// to an empty (non-nil) arg map without any wire-form difference.
func checkAppFields(t *testing.T, want, got *model.App) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID: want %q got %q", want.ID, got.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name: want %q got %q", want.Name, got.Name)
	}
	if got.Entry != want.Entry {
		t.Errorf("Entry: want %q got %q", want.Entry, got.Entry)
	}
	if got.DefaultLocale != want.DefaultLocale {
		t.Errorf("DefaultLocale: want %q got %q", want.DefaultLocale, got.DefaultLocale)
	}
	if got.Theme != want.Theme {
		t.Errorf("Theme: want %q got %q", want.Theme, got.Theme)
	}
	if got.Branding != want.Branding {
		t.Errorf("Branding: want %v got %v (loader defaults true when the key is absent; false must be written out)", want.Branding, got.Branding)
	}
	if got.PluginABI != want.PluginABI {
		t.Errorf("PluginABI: want %q got %q", want.PluginABI, got.PluginABI)
	}
	if !equivStrMap(want.GlobalState.Schema, got.GlobalState.Schema) {
		t.Errorf("GlobalState.Schema: want %v got %v", want.GlobalState.Schema, got.GlobalState.Schema)
	}
	if !reflect.DeepEqual(normAnyMap(want.GlobalState.Initial), normAnyMap(got.GlobalState.Initial)) {
		t.Errorf("GlobalState.Initial: want %v got %v", want.GlobalState.Initial, got.GlobalState.Initial)
	}
	if !reflect.DeepEqual(want.DesignTokens, got.DesignTokens) {
		t.Errorf("DesignTokens: want %+v got %+v", want.DesignTokens, got.DesignTokens)
	}
	if !reflect.DeepEqual(want.Widgets, got.Widgets) {
		t.Errorf("Widgets: want %+v got %+v", want.Widgets, got.Widgets)
	}
	if !reflect.DeepEqual(want.DesktopMenu, got.DesktopMenu) {
		t.Errorf("DesktopMenu: want %+v got %+v", want.DesktopMenu, got.DesktopMenu)
	}
	if !reflect.DeepEqual(want.Tray, got.Tray) {
		t.Errorf("Tray: want %+v got %+v", want.Tray, got.Tray)
	}
	if !reflect.DeepEqual(want.Window, got.Window) {
		t.Errorf("Window: want %+v got %+v (round-3 zeroed dims via the int-vs-float64 asFloat path)", want.Window, got.Window)
	}
	if !reflect.DeepEqual(want.Shortcuts, got.Shortcuts) {
		t.Errorf("Shortcuts: want %+v got %+v", want.Shortcuts, got.Shortcuts)
	}
	checkNodeMap(t, "Components", want.Components, got.Components)
	if !reflect.DeepEqual(want.ComponentSchemas, got.ComponentSchemas) {
		t.Errorf("ComponentSchemas: want %+v got %+v (a declared component must round-trip through the manifest's declaration form)",
			want.ComponentSchemas, got.ComponentSchemas)
	}
	checkNodeMap(t, "Scenes", want.Scenes, got.Scenes)
	checkActionMap(t, "Actions", want.Actions, got.Actions)
	for id, inv := range want.SceneEnter {
		checkInvoke(t, "SceneEnter["+strconv.Quote(id)+"]", inv, got.SceneEnter[id])
	}
	for id := range got.SceneEnter {
		if _, ok := want.SceneEnter[id]; !ok {
			t.Errorf("SceneEnter[%q]: hook appeared out of nowhere in the round trip", id)
		}
	}
	if !equivStrMap(want.Computed, got.Computed) {
		t.Errorf("Computed: want %v got %v (derived declarations must survive the round trip)", want.Computed, got.Computed)
	}
	checkGuardMap(t, want.SceneGuards, got.SceneGuards)
	if !reflect.DeepEqual(want.SceneKeys, got.SceneKeys) {
		t.Errorf("SceneKeys: want %v got %v (scene key bindings must survive the round trip)", want.SceneKeys, got.SceneKeys)
	}
	if !reflect.DeepEqual(want.SceneSwipes, got.SceneSwipes) {
		t.Errorf("SceneSwipes: want %v got %v (scene swipe bindings must survive the round trip)", want.SceneSwipes, got.SceneSwipes)
	}
}

// checkGuardMap asserts the scene route guards survive the round trip, field by
// field and in both directions (nothing lost, nothing invented).
func checkGuardMap(t *testing.T, want, got map[string]*model.SceneGuard) {
	t.Helper()
	for id, wg := range want {
		gg, ok := got[id]
		if !ok {
			t.Errorf("SceneGuards[%q]: guard lost in the round trip", id)
			continue
		}
		if gg.Condition != wg.Condition {
			t.Errorf("SceneGuards[%q].Condition: want %q got %q", id, wg.Condition, gg.Condition)
		}
		if gg.Redirect != wg.Redirect {
			t.Errorf("SceneGuards[%q].Redirect: want %q got %q", id, wg.Redirect, gg.Redirect)
		}
		if !equivStrMap(wg.Params, gg.Params) {
			t.Errorf("SceneGuards[%q].Params: want %v got %v", id, wg.Params, gg.Params)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("SceneGuards[%q]: guard appeared out of nowhere in the round trip", id)
		}
	}
}

func checkNodeMap(t *testing.T, path string, want, got map[string]*model.Node) {
	t.Helper()
	for id, wn := range want {
		gn, ok := got[id]
		if !ok {
			t.Errorf("%s[%q]: node lost in the round trip", path, id)
			continue
		}
		checkNode(t, path+"["+strconv.Quote(id)+"]", wn, gn)
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("%s[%q]: node appeared out of nowhere in the round trip", path, id)
		}
	}
}

func checkNode(t *testing.T, path string, want, got *model.Node) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Errorf("%s: nil mismatch: want %v got %v", path, want, got)
		}
		return
	}
	if got.Type != want.Type {
		t.Errorf("%s.Type: want %q got %q", path, want.Type, got.Type)
	}
	if got.ID != want.ID {
		t.Errorf("%s.ID: want %q got %q", path, want.ID, got.ID)
	}
	if got.Text != want.Text {
		t.Errorf("%s.Text: want %q got %q", path, want.Text, got.Text)
	}
	if got.Label != want.Label {
		t.Errorf("%s.Label: want %q got %q", path, want.Label, got.Label)
	}
	if got.Placeholder != want.Placeholder {
		t.Errorf("%s.Placeholder: want %q got %q", path, want.Placeholder, got.Placeholder)
	}
	if got.Value != want.Value {
		t.Errorf("%s.Value: want %q got %q", path, want.Value, got.Value)
	}
	if got.Data != want.Data {
		t.Errorf("%s.Data: want %q got %q (round-3 dropped data without a renderItem)", path, want.Data, got.Data)
	}
	if got.Condition != want.Condition {
		t.Errorf("%s.Condition: want %q got %q", path, want.Condition, got.Condition)
	}
	if !reflect.DeepEqual(normAnyMap(want.Style), normAnyMap(got.Style)) {
		t.Errorf("%s.Style: want %v got %v", path, want.Style, got.Style)
	}
	if !reflect.DeepEqual(normAnyMap(want.Layout), normAnyMap(got.Layout)) {
		t.Errorf("%s.Layout: want %v got %v", path, want.Layout, got.Layout)
	}
	if !reflect.DeepEqual(nodeExtras(want), nodeExtras(got)) {
		t.Errorf("%s extra props: want %v got %v", path, nodeExtras(want), nodeExtras(got))
	}
	checkInvoke(t, path+".OnPress", want.OnPress, got.OnPress)
	checkInvoke(t, path+".OnChange", want.OnChange, got.OnChange)
	if len(want.Children) != len(got.Children) {
		t.Errorf("%s.Children: want %d got %d", path, len(want.Children), len(got.Children))
	} else {
		for i := range want.Children {
			checkNode(t, path+".Children["+strconv.Itoa(i)+"]", want.Children[i], got.Children[i])
		}
	}
	checkNode(t, path+".Template", want.Template, got.Template)
	checkNode(t, path+".Then", want.Then, got.Then)
	checkNode(t, path+".Else", want.Else, got.Else)
}

func checkInvoke(t *testing.T, path string, want, got *model.Invoke) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Errorf("%s: nil mismatch: want %v got %v", path, want, got)
		}
		return
	}
	if got.Name != want.Name {
		t.Errorf("%s.Name: want %q got %q", path, want.Name, got.Name)
	}
	if !equivStrMap(want.Args, got.Args) {
		t.Errorf("%s.Args: want %v got %v", path, want.Args, got.Args)
	}
}

func checkActionMap(t *testing.T, path string, want, got map[string]*model.Action) {
	t.Helper()
	for id, wa := range want {
		ga, ok := got[id]
		if !ok {
			t.Errorf("%s[%q]: action lost in the round trip", path, id)
			continue
		}
		if ga.ID != wa.ID {
			t.Errorf("%s[%q].ID: want %q got %q", path, id, wa.ID, ga.ID)
		}
		if len(wa.Steps) != len(ga.Steps) {
			t.Errorf("%s[%q].Steps: want %d got %d", path, id, len(wa.Steps), len(ga.Steps))
			continue
		}
		for i := range wa.Steps {
			checkStep(t, path+"["+strconv.Quote(id)+"].Steps["+strconv.Itoa(i)+"]", wa.Steps[i], ga.Steps[i])
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("%s[%q]: action appeared out of nowhere in the round trip", path, id)
		}
	}
}

func checkStep(t *testing.T, path string, want, got model.Step) {
	t.Helper()
	if got.Type != want.Type {
		t.Errorf("%s.Type: want %q got %q", path, want.Type, got.Type)
	}
	if got.Path != want.Path {
		t.Errorf("%s.Path: want %q got %q", path, want.Path, got.Path)
	}
	if got.Value != want.Value {
		t.Errorf("%s.Value: want %q got %q", path, want.Value, got.Value)
	}
	if got.MatchKey != want.MatchKey {
		t.Errorf("%s.MatchKey: want %q got %q", path, want.MatchKey, got.MatchKey)
	}
	if got.Match != want.Match {
		t.Errorf("%s.Match: want %q got %q", path, want.Match, got.Match)
	}
	if got.Field != want.Field {
		t.Errorf("%s.Field: want %q got %q", path, want.Field, got.Field)
	}
	// Round-3 dropped all four navigation fields.
	if got.To != want.To {
		t.Errorf("%s.To: want %q got %q", path, want.To, got.To)
	}
	if got.Back != want.Back {
		t.Errorf("%s.Back: want %v got %v", path, want.Back, got.Back)
	}
	if got.From != want.From {
		t.Errorf("%s.From: want %q got %q", path, want.From, got.From)
	}
	if !equivStrMap(want.Params, got.Params) {
		t.Errorf("%s.Params: want %v got %v", path, want.Params, got.Params)
	}
	if !equivStrMap(want.Object, got.Object) {
		t.Errorf("%s.Object: want %v got %v", path, want.Object, got.Object)
	}
	if got.URL != want.URL {
		t.Errorf("%s.URL: want %q got %q", path, want.URL, got.URL)
	}
	if got.Method != want.Method {
		t.Errorf("%s.Method: want %q got %q", path, want.Method, got.Method)
	}
	if got.Body != want.Body {
		t.Errorf("%s.Body: want %q got %q", path, want.Body, got.Body)
	}
	if !equivStrMap(want.Headers, got.Headers) {
		t.Errorf("%s.Headers: want %v got %v", path, want.Headers, got.Headers)
	}
	if got.Result != want.Result {
		t.Errorf("%s.Result: want %q got %q", path, want.Result, got.Result)
	}
	if got.Error != want.Error {
		t.Errorf("%s.Error: want %q got %q", path, want.Error, got.Error)
	}
	// Async governance fields: a dropped `key` silently un-deduplicates a
	// search box, a dropped `timeout` silently restores the 20s ceiling, a
	// dropped `pending` leaves a spinner up — all invisible in a diff of the
	// re-serialised app, which is exactly why they are pinned here.
	if got.Async != want.Async {
		t.Errorf("%s.Async: want %v got %v", path, want.Async, got.Async)
	}
	if got.Key != want.Key {
		t.Errorf("%s.Key: want %q got %q", path, want.Key, got.Key)
	}
	if got.TimeoutMS != want.TimeoutMS {
		t.Errorf("%s.TimeoutMS: want %d got %d", path, want.TimeoutMS, got.TimeoutMS)
	}
	if got.Pending != want.Pending {
		t.Errorf("%s.Pending: want %q got %q", path, want.Pending, got.Pending)
	}
	if got.DelayMS != want.DelayMS {
		t.Errorf("%s.DelayMS: want %d got %d", path, want.DelayMS, got.DelayMS)
	}
	// Execution-model fields: `if` condition + branches, invoke name/args,
	// http result branches — all recursive step lists.
	if got.Condition != want.Condition {
		t.Errorf("%s.Condition: want %q got %q", path, want.Condition, got.Condition)
	}
	if got.Name != want.Name {
		t.Errorf("%s.Name: want %q got %q", path, want.Name, got.Name)
	}
	if !equivStrMap(want.Args, got.Args) {
		t.Errorf("%s.Args: want %v got %v", path, want.Args, got.Args)
	}
	checkStepList(t, path+".Then", want.Then, got.Then)
	checkStepList(t, path+".Else", want.Else, got.Else)
	checkStepList(t, path+".OnSuccess", want.OnSuccess, got.OnSuccess)
	checkStepList(t, path+".OnError", want.OnError, got.OnError)
}

func checkStepList(t *testing.T, path string, want, got []model.Step) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s: want %d steps got %d", path, len(want), len(got))
		return
	}
	for i := range want {
		checkStep(t, path+"["+strconv.Itoa(i)+"]", want[i], got[i])
	}
}

// canonicalDocSet normalises a doc set for comparison. AppToDocs iterates the
// app.Scenes and app.Actions maps, whose Go iteration order is randomised, so
// the doc SLICE order is non-deterministic — a raw reflect.DeepEqual on the
// slices would flake. encoding/json emits map keys sorted, so each doc has
// canonical bytes (this is exactly the form bundle.fromDocs hashes), and
// sorting the list makes the comparison order-independent. Two doc sets
// compare equal here iff they are equal on the wire.
func canonicalDocSet(t *testing.T, docs []map[string]any) []string {
	t.Helper()
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("marshal doc %v: %v", d, err)
		}
		out = append(out, string(b))
	}
	sort.Strings(out)
	return out
}

// nodeExtras returns the UNTYPED props of a node: everything in Props that is
// not a struct-backed field (and, for when nodes, not condition/then/else).
// That set — not Props itself, which on a parsed node also holds the typed
// keys verbatim — is what NodeToJSON carries through and what must survive.
func nodeExtras(n *model.Node) map[string]any {
	out := map[string]any{}
	for k, v := range n.Props {
		if typedKeys[k] || (n.Type == "when" && whenKeys[k]) {
			continue
		}
		out[k] = v
	}
	return out
}

// equivStrMap treats a nil and an empty map as equal (see checkAppFields).
func equivStrMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// normAnyMap canonicalises a nil/empty map to nil for reflect.DeepEqual.
func normAnyMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return m
}

// minimalApp is the smallest app with a live entry scene, for the per-field
// regression subtests: each mutates exactly ONE field and asserts it survives
// AppToDocs -> FromDocs, so any future regression points at the exact field.
func minimalApp() *model.App {
	return &model.App{
		Entry:   "main",
		Scenes:  map[string]*model.Node{"main": {Type: "view", ID: "root"}},
		Actions: map[string]*model.Action{},
	}
}

func TestSerializeRoundTripRegressionFields(t *testing.T) {
	// withNavStep installs a single navigate step carrying st on a "nav" action.
	withNavStep := func(st model.Step) func(*model.App) {
		return func(a *model.App) {
			st.Type = "navigate"
			a.Actions["nav"] = &model.Action{ID: "nav", Steps: []model.Step{st}}
		}
	}
	navStep0 := func(t *testing.T, got *model.App) model.Step {
		t.Helper()
		act := got.Actions["nav"]
		if act == nil || len(act.Steps) != 1 {
			t.Fatalf("nav action/step lost in the round trip: %+v", act)
		}
		return act.Steps[0]
	}

	cases := []struct {
		name  string
		mut   func(*model.App)
		check func(*testing.T, *model.App)
	}{
		{
			name: "node data without template",
			mut: func(a *model.App) {
				a.Scenes["main"] = &model.Node{Type: "list", ID: "l", Data: "{{ state.items }}"}
			},
			check: func(t *testing.T, got *model.App) {
				root := got.Scenes["main"]
				if root == nil || root.Data != "{{ state.items }}" {
					t.Fatalf("data binding lost on a node without a renderItem: %+v", root)
				}
				if root.Template != nil {
					t.Errorf("no template was set, got %+v", root.Template)
				}
			},
		},
		{
			name: "node data with template",
			mut: func(a *model.App) {
				a.Scenes["main"] = &model.Node{
					Type: "list", ID: "l", Data: "{{ state.items }}",
					Template: &model.Node{Type: "text", ID: "row", Text: "{{ prop.title }}"},
				}
			},
			check: func(t *testing.T, got *model.App) {
				root := got.Scenes["main"]
				if root == nil || root.Data != "{{ state.items }}" || root.Template == nil || root.Template.ID != "row" {
					t.Fatalf("data/template lost: %+v", root)
				}
			},
		},
		{
			name: "navigate to",
			mut:  withNavStep(model.Step{To: "settings"}),
			check: func(t *testing.T, got *model.App) {
				if st := navStep0(t, got); st.To != "settings" {
					t.Errorf("navigate To lost: %+v", st)
				}
			},
		},
		{
			name: "navigate back",
			mut:  withNavStep(model.Step{Back: true}),
			check: func(t *testing.T, got *model.App) {
				if st := navStep0(t, got); !st.Back {
					t.Errorf("navigate Back lost: %+v", st)
				}
			},
		},
		{
			name: "navigate from",
			mut:  withNavStep(model.Step{From: "2"}),
			check: func(t *testing.T, got *model.App) {
				if st := navStep0(t, got); st.From != "2" {
					t.Errorf("navigate From lost: %+v", st)
				}
			},
		},
		{
			name: "navigate params",
			mut:  withNavStep(model.Step{To: "settings", Params: map[string]string{"userId": "{{ state.user }}"}}),
			check: func(t *testing.T, got *model.App) {
				if st := navStep0(t, got); st.Params["userId"] != "{{ state.user }}" {
					t.Errorf("navigate Params lost: %+v", st)
				}
			},
		},
		{
			name: "manifest branding false",
			mut:  func(a *model.App) { a.Branding = false },
			check: func(t *testing.T, got *model.App) {
				if got.Branding {
					t.Error("branding:false flipped back to the default true (the key must be written out)")
				}
			},
		},
		{
			name: "manifest designTokens",
			mut: func(a *model.App) {
				a.DesignTokens = map[string]model.DesignToken{
					"color.primary": {Type: "color", Value: "#fff", Enforce: true},
				}
			},
			check: func(t *testing.T, got *model.App) {
				dt := got.DesignTokens["color.primary"]
				if dt.Type != "color" || dt.Value != "#fff" || !dt.Enforce {
					t.Errorf("designTokens (incl. enforce) lost: %+v", got.DesignTokens)
				}
			},
		},
		{
			name: "manifest pluginABI",
			mut:  func(a *model.App) { a.PluginABI = strconv.Itoa(qormext.ABIVersion) },
			check: func(t *testing.T, got *model.App) {
				if got.PluginABI != strconv.Itoa(qormext.ABIVersion) {
					t.Errorf("pluginABI lost: %q", got.PluginABI)
				}
			},
		},
		{
			name: "manifest widgets",
			mut: func(a *model.App) {
				a.Widgets = []model.Widget{{
					ID: "w", Name: "W", Title: "T",
					Lines: []model.WidgetLine{{Label: "L", Value: "V"}},
				}}
			},
			check: func(t *testing.T, got *model.App) {
				if len(got.Widgets) != 1 || got.Widgets[0].ID != "w" ||
					len(got.Widgets[0].Lines) != 1 || got.Widgets[0].Lines[0].Label != "L" {
					t.Errorf("widgets lost: %+v", got.Widgets)
				}
			},
		},
		{
			name: "manifest desktop menu",
			mut: func(a *model.App) {
				a.DesktopMenu = []model.MenuGroup{{
					Title: "File",
					Items: []model.MenuItem{{ID: "n", Title: "New", Items: []model.MenuItem{{ID: "r", Title: "Recent"}}}},
				}}
			},
			check: func(t *testing.T, got *model.App) {
				if len(got.DesktopMenu) != 1 || len(got.DesktopMenu[0].Items) != 1 ||
					len(got.DesktopMenu[0].Items[0].Items) != 1 {
					t.Errorf("desktop menu (incl. nested items) lost: %+v", got.DesktopMenu)
				}
			},
		},
		{
			name: "manifest tray",
			mut: func(a *model.App) {
				a.Tray = model.TrayConfig{Icon: "t.png", Tip: "Hi", Items: []model.MenuItem{{ID: "q", Title: "Quit"}}}
			},
			check: func(t *testing.T, got *model.App) {
				if got.Tray.Icon != "t.png" || got.Tray.Tip != "Hi" || len(got.Tray.Items) != 1 {
					t.Errorf("tray lost: %+v", got.Tray)
				}
			},
		},
		{
			name: "window dimensions (int path)",
			mut:  func(a *model.App) { a.Window = model.Window{Width: 640, Height: 480} },
			check: func(t *testing.T, got *model.App) {
				// ManifestToJSON writes the dims as Go ints (no intermediate
				// JSON encode), so applyManifest's asFloat must accept integer
				// Go types — the round-3 bug zeroed them here.
				if got.Window.Width != 640 || got.Window.Height != 480 {
					t.Errorf("window dims lost on the int path: %+v", got.Window)
				}
			},
		},
		{
			name: "window hideLog hideTray",
			mut:  func(a *model.App) { a.Window = model.Window{HideLog: true, HideTray: true} },
			check: func(t *testing.T, got *model.App) {
				if !got.Window.HideLog || !got.Window.HideTray {
					t.Errorf("window hideLog/hideTray lost: %+v", got.Window)
				}
			},
		},
		{
			name: "window resizable:false (Fixed)",
			mut:  func(a *model.App) { a.Window = model.Window{Width: 320, Height: 240, Fixed: true} },
			check: func(t *testing.T, got *model.App) {
				// "resizable": false must survive the round trip — a fixed game
				// board that came back resizable would let users drag it out of
				// aspect ratio.
				if !got.Window.Fixed {
					t.Errorf("window Fixed lost on round trip: %+v", got.Window)
				}
			},
		},
		{
			name: "if step condition and branches",
			mut: func(a *model.App) {
				a.Actions["cond"] = &model.Action{ID: "cond", Steps: []model.Step{{
					Type:      "if",
					Condition: "{{ state.x }}",
					Then:      []model.Step{{Type: "state.set", Path: "x", Value: "1"}},
					Else:      []model.Step{{Type: "state.clear", Path: "x"}},
				}}}
			},
			check: func(t *testing.T, got *model.App) {
				act := got.Actions["cond"]
				if act == nil || len(act.Steps) != 1 {
					t.Fatalf("if action lost: %+v", act)
				}
				st := act.Steps[0]
				if st.Condition != "{{ state.x }}" || len(st.Then) != 1 || len(st.Else) != 1 {
					t.Errorf("if condition/branches lost: %+v", st)
				}
			},
		},
		{
			name: "invoke step name and args",
			mut: func(a *model.App) {
				a.Actions["callee"] = &model.Action{ID: "callee"}
				a.Actions["caller"] = &model.Action{ID: "caller", Steps: []model.Step{{
					Type: "invoke", Name: "callee", Args: map[string]string{"v": "{{ state.x }}"},
				}}}
			},
			check: func(t *testing.T, got *model.App) {
				act := got.Actions["caller"]
				if act == nil || len(act.Steps) != 1 {
					t.Fatalf("invoke action lost: %+v", act)
				}
				if st := act.Steps[0]; st.Name != "callee" || st.Args["v"] != "{{ state.x }}" {
					t.Errorf("invoke name/args lost: %+v", st)
				}
			},
		},
		{
			name: "http onSuccess and onError",
			mut: func(a *model.App) {
				a.Actions["sync"] = &model.Action{ID: "sync", Steps: []model.Step{{
					Type: "http.get", URL: "https://x.example/y", Result: "r",
					OnSuccess: []model.Step{{Type: "state.set", Path: "p", Value: "ok"}},
					OnError:   []model.Step{{Type: "state.set", Path: "p", Value: "{{ error }}"}},
				}}}
			},
			check: func(t *testing.T, got *model.App) {
				act := got.Actions["sync"]
				if act == nil || len(act.Steps) != 1 {
					t.Fatalf("http action lost: %+v", act)
				}
				if st := act.Steps[0]; len(st.OnSuccess) != 1 || len(st.OnError) != 1 {
					t.Errorf("http branches lost: %+v", st)
				}
			},
		},
		{
			name: "http async flag",
			mut: func(a *model.App) {
				a.Actions["bg"] = &model.Action{ID: "bg", Steps: []model.Step{
					{Type: "http.get", URL: "https://x.example/y", Result: "r", Async: true},
					{Type: "http.get", URL: "https://x.example/z", Result: "s"},
				}}
			},
			check: func(t *testing.T, got *model.App) {
				act := got.Actions["bg"]
				if act == nil || len(act.Steps) != 2 {
					t.Fatalf("async action lost: %+v", act)
				}
				if !act.Steps[0].Async {
					t.Errorf("the async flag must survive the round trip: %+v", act.Steps[0])
				}
				if act.Steps[1].Async {
					t.Errorf("a synchronous step must not grow an async flag: %+v", act.Steps[1])
				}
			},
		},
		{
			name: "scene onEnter",
			mut: func(a *model.App) {
				a.Actions["load"] = &model.Action{ID: "load"}
				a.SceneEnter = map[string]*model.Invoke{"main": {Name: "load", Args: map[string]string{"v": "1"}}}
			},
			check: func(t *testing.T, got *model.App) {
				inv := got.SceneEnter["main"]
				if inv == nil || inv.Name != "load" || inv.Args["v"] != "1" {
					t.Errorf("scene onEnter lost: %+v", got.SceneEnter)
				}
			},
		},
		{
			name: "computed derived values",
			mut: func(a *model.App) {
				a.Computed = map[string]string{
					"total":   "{{ len(state.items) }}",
					"isEmpty": "{{ computed.total == 0 }}",
				}
			},
			check: func(t *testing.T, got *model.App) {
				if got.Computed["total"] != "{{ len(state.items) }}" || got.Computed["isEmpty"] != "{{ computed.total == 0 }}" {
					t.Errorf("computed declarations lost: %+v", got.Computed)
				}
			},
		},
		{
			name: "scene route guard",
			mut: func(a *model.App) {
				a.Scenes["login"] = &model.Node{Type: "view", ID: "login_root"}
				a.SceneGuards = map[string]*model.SceneGuard{"main": {
					Condition: "{{ state.user != null }}",
					Redirect:  "login",
					Params:    map[string]string{"next": "{{ 'main' }}"},
				}}
			},
			check: func(t *testing.T, got *model.App) {
				g := got.SceneGuards["main"]
				if g == nil || g.Condition != "{{ state.user != null }}" || g.Redirect != "login" {
					t.Fatalf("scene guard lost: %+v", got.SceneGuards)
				}
				if g.Params["next"] != "{{ 'main' }}" {
					t.Errorf("guard redirect params lost: %+v", g.Params)
				}
			},
		},
		{
			name: "forEach step",
			mut: func(a *model.App) {
				a.Actions["bulk"] = &model.Action{ID: "bulk", Steps: []model.Step{{
					Type: "forEach", In: "{{ state.items }}", As: "row",
					Steps: []model.Step{{Type: "state.increment", Path: "count"}},
				}}}
			},
			check: func(t *testing.T, got *model.App) {
				act := got.Actions["bulk"]
				if act == nil || len(act.Steps) != 1 {
					t.Fatalf("forEach action lost: %+v", act)
				}
				st := act.Steps[0]
				if st.In != "{{ state.items }}" || st.As != "row" {
					t.Errorf("forEach collection/alias lost: %+v", st)
				}
				if len(st.Steps) != 1 || st.Steps[0].Type != "state.increment" {
					t.Errorf("forEach body lost: %+v", st.Steps)
				}
			},
		},
		{
			name: "globalState schema and initial",
			mut: func(a *model.App) {
				a.GlobalState = model.GlobalState{
					Schema:  map[string]string{"count": "number"},
					Initial: map[string]any{"count": float64(7)},
				}
			},
			check: func(t *testing.T, got *model.App) {
				if got.GlobalState.Schema["count"] != "number" || got.GlobalState.Initial["count"] != float64(7) {
					t.Errorf("globalState lost: %+v", got.GlobalState)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := minimalApp()
			c.mut(app)
			c.check(t, FromDocs(AppToDocs(app)))
		})
	}
}

// TestSerializeRoundTripZeroDiagnostics is the diagnostic invariant: a
// well-formed app (every type present, entry scene exists, schema declares
// its bindings, compatible pluginABI) must round-trip with ZERO diagnostics,
// so a future change that starts warning on valid input fails loudly.
func TestSerializeRoundTripZeroDiagnostics(t *testing.T) {
	app2 := FromDocs(AppToDocs(roundTripApp()))
	if len(app2.Diagnostics) != 0 {
		t.Fatalf("well-formed app must round-trip with zero diagnostics, got %v", app2.Diagnostics)
	}
}

// canonDocs is the app pipeline written as a function on documents:
// canon(d) = AppToDocs(FromDocs(d)). It is what every consumer of the doc form
// actually applies — bundle.FromApp, an MCP export, a patch write-back — and
// its output is sorted here because document ORDER comes from the file walk and
// is not part of an app's meaning.
func canonDocs(docs []map[string]any) []map[string]any {
	out := AppToDocs(FromDocs(docs))
	sort.SliceStable(out, func(i, j int) bool {
		key := func(d map[string]any) string { return asString(d["type"]) + "\x00" + asString(d["id"]) }
		return key(out[i]) < key(out[j])
	})
	return out
}

// FuzzSerializeRoundtrip enforces the IDEMPOTENCE of that pipeline:
//
//	canon(canon(d)) == canon(d)
//
// which is strictly stronger than "does not panic" and is the property every
// content-addressed use of the doc form silently assumes. If a second pass can
// change the documents, then the same app has more than one serialised form,
// and therefore more than one contentHash — so a bundle rebuilt from a live
// app stops matching the bundle built from its sources, an exported design
// cannot be pinned, and re-saving an app in an editor mutates it. It also
// pins the LOSSY steps in place: canonicalisation (a prop shorthand becoming
// its long form, a component document keeping its own spelling) is allowed to
// change the input once, never twice.
//
// A counterexample is a real defect even when nothing crashes, which is why
// this replaced the panic-only assertion.
func FuzzSerializeRoundtrip(f *testing.F) {
	for _, s := range []string{
		// A valid counter-shaped doc set.
		`[{"type":"app","id":"counter","entry":"main","globalState":{"schema":{"count":"number"},"initial":{"count":0}}},` +
			`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"{{ state.count }}"}}]`,
		// A kitchen-sink manifest exercising every field type (strings, ints,
		// floats via encoding, bools, arrays, nested maps).
		`[{"type":"app","id":"sink","entry":"main","name":"S","defaultLocale":"en","theme":"material",` +
			`"branding":false,"pluginABI":"1",` +
			`"globalState":{"schema":{"n":"number"},"initial":{"n":1,"s":"x","b":true,"list":[1,2],"obj":{"k":"v"}}},` +
			`"designTokens":{"c":{"type":"color","value":"#fff","enforce":true}},` +
			`"widgets":[{"id":"w","name":"W","lines":[{"label":"L","value":"V"}]}],` +
			`"components":{"card":{"type":"text","id":"c"}},"shortcuts":[{"id":"s","title":"T"}],` +
			`"platforms":{"desktop":{"menu":[{"title":"File","items":[{"id":"n","title":"New"}]}],` +
			`"tray":{"icon":"i","tip":"t","items":[{"id":"q","role":"quit"}]},` +
			`"window":{"width":640,"height":480,"title":"W","resizable":true,"chromeless":true,"transparent":true,"hideLog":true,"hideTray":true}}}},` +
			`{"type":"scene","id":"main","root":{"type":"view","id":"r","style":null,"data":"{{ state.list }}",` +
			`"renderItem":{"type":"text","id":"row"},"children":[{"type":"when","id":"w","condition":"{{ viewport.width >= 1 }}","then":{"type":"text","id":"a"}}]}},` +
			`{"type":"action","id":"go","steps":[{"type":"navigate","to":"main","back":true,"from":"1","params":{"u":"{{ state.s }}"}}]}]`,
		// Malformed-ish but parseable maps: wrong types in every section, so
		// each coercion path runs without panicking.
		`[{"type":"app","globalState":{"schema":42,"initial":[]},"widgets":{"x":1},"branding":"yes",` +
			`"designTokens":{"t":7},"components":{"c":[1]},"shortcuts":{"s":1},` +
			`"platforms":{"desktop":{"window":{"width":"big"},"menu":{"m":1},"tray":[]}}},` +
			`{"type":"scene","id":"s","root":{"type":"when","condition":{"deep":true},"then":[1],"children":["x",null,2],"style":{"k":{"nested":"{{ x }}"}}}},` +
			`{"type":"action","id":"a","steps":[{"type":"navigate","to":42,"params":[1],"back":"no","item":{"k":{}}},"junk"]},` +
			`{"id":"typeless"},{"type":"bogus","id":"b"}]`,
		// Standalone component documents in both spellings (declared and bare
		// template), plus one that collides with a manifest-inline component
		// and one with a non-string id — the shapes where the serializer has
		// to remember WHERE a component came from.
		`[{"type":"app","id":"c","entry":"main","components":{"inline":{"type":"text","id":"i"}}},` +
			`{"type":"component","id":"declared","props":{"n":"number","t":{"type":"string","required":true}},` +
			`"slots":{"body":{"required":true}},"template":{"type":"card","id":"d","children":[{"type":"slot","name":"body"}]}},` +
			`{"type":"component","id":"bare","template":{"type":"text","id":"b","text":"x"}},` +
			`{"type":"component","id":"inline","template":{"type":"row","id":"shadow"}},` +
			`{"type":"component","id":7,"template":{"type":"row","id":"numbered"}},` +
			`{"type":"scene","id":"main","root":{"type":"declared","id":"inst","t":"T","children":[{"type":"text","id":"bd","slot":"body"}]}}]`,
		// Duplicates and an onEnter/guard-bearing scene: first-wins resolution
		// must itself be idempotent.
		`[{"type":"app","id":"a","entry":"main"},{"type":"app","id":"b","entry":"other"},` +
			`{"type":"scene","id":"main","root":{"type":"text","id":"x"},"onEnter":"go",` +
			`"guard":{"condition":"{{ state.u }}","redirect":"main","params":{"n":"1"}}},` +
			`{"type":"scene","id":"main","root":{"type":"text","id":"dup"}},` +
			`{"type":"action","id":"go","steps":[]},{"type":"action","id":"go","steps":[{"type":"render"}]}]`,
		`[]`,
		`[{}]`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		var docs []map[string]any
		if json.Unmarshal([]byte(data), &docs) != nil {
			return // not an array of objects, as collect() would skip
		}
		once := canonDocs(docs)  // assemble + serialise: must not panic
		twice := canonDocs(once) // and doing it again must change nothing
		a, err := json.Marshal(once)
		if err != nil {
			return // not comparable as JSON; the no-panic half still held
		}
		b, err := json.Marshal(twice)
		if err != nil {
			t.Fatalf("the serialised form must stay JSON-encodable: %v\ninput: %s", err, data)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("AppToDocs∘FromDocs is not idempotent\n input: %s\n once:  %s\n twice: %s", data, a, b)
		}
		_ = FromDocs(twice).EntryRoot() // must not panic
	})
}
