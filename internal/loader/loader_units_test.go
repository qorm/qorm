package loader

import (
	"strings"
	"testing"
)

func TestParseMenuItemsNestedAndMalformed(t *testing.T) {
	raw := []any{
		map[string]any{
			"id": "file", "title": "File", "icon": "doc", "shortcut": "cmd+f", "role": "none",
			"items": []any{
				map[string]any{"id": "new", "title": "New"},
				map[string]any{"separator": true},
				"not-a-map", // skipped
			},
		},
		42.0,        // non-map entry skipped
		"str-entry", // non-map entry skipped
	}
	items := parseMenuItems(raw)
	if len(items) != 1 {
		t.Fatalf("want 1 top-level item, got %d: %+v", len(items), items)
	}
	f := items[0]
	if f.ID != "file" || f.Title != "File" || f.Icon != "doc" || f.Shortcut != "cmd+f" || f.Role != "none" {
		t.Errorf("fields mis-parsed: %+v", f)
	}
	if len(f.Items) != 2 || f.Items[0].ID != "new" || !f.Items[1].Separator {
		t.Errorf("nested items mis-parsed: %+v", f.Items)
	}

	if got := parseMenuItems("not-an-array"); got != nil {
		t.Errorf("non-array must give nil, got %v", got)
	}
	if got := parseMenuItems(nil); got != nil {
		t.Errorf("nil must give nil, got %v", got)
	}
}

func TestParseMenuGroupsMalformed(t *testing.T) {
	groups := parseMenuGroups([]any{
		map[string]any{"title": "Edit", "items": []any{map[string]any{"id": "copy", "role": "copy"}}},
		"junk",
	})
	if len(groups) != 1 || groups[0].Title != "Edit" || len(groups[0].Items) != 1 {
		t.Fatalf("groups mis-parsed: %+v", groups)
	}
	if got := parseMenuGroups(map[string]any{"title": "x"}); got != nil {
		t.Errorf("non-array must give nil, got %v", got)
	}
}

// TestApplyManifestFull exercises every manifest section with a fully-populated
// document, including component expression type-checking against the schema.
func TestApplyManifestFull(t *testing.T) {
	doc := map[string]any{
		"type": "app", "id": "full", "name": "Full", "entry": "home",
		"defaultLocale": "en", "theme": "material", "branding": false,
		"globalState": map[string]any{
			"schema":  map[string]any{"count": "string"},
			"initial": map[string]any{"count": "0"},
		},
		"widgets": []any{
			map[string]any{
				"id": "w1", "name": "W", "title": "Widget",
				"lines": []any{map[string]any{"label": "L", "value": "V"}, "junk"},
			},
			"not-a-widget",
		},
		"components": map[string]any{
			"card": map[string]any{"type": "text", "id": "ct", "text": "{{ state.count - 1 }}"},
			"bad":  "not-a-node",
		},
		"designTokens": map[string]any{
			"color.primary": map[string]any{"type": "color", "value": "#fff", "enforce": true},
			"broken":        "not-a-token",
		},
		"shortcuts": []any{
			map[string]any{"id": "s1", "title": "S", "subtitle": "sub", "icon": "star"},
			17.0,
		},
		"platforms": map[string]any{
			"desktop": map[string]any{
				"menu": []any{map[string]any{"title": "File", "items": []any{map[string]any{"id": "q", "role": "quit"}}}},
				"tray": map[string]any{"icon": "tray.png", "tip": "Hi", "items": []any{map[string]any{"id": "t1", "title": "T"}}},
				"window": map[string]any{
					"width": 800.0, "height": 600.0, "title": "Win",
					"resizable": true, "chromeless": true, "transparent": true,
				},
			},
		},
	}
	app, diags := loadManifest(doc)

	if app.ID != "full" || app.Name != "Full" || app.Entry != "home" ||
		app.DefaultLocale != "en" || app.Theme != "material" || app.Branding {
		t.Errorf("scalar fields mis-parsed: %+v", app)
	}
	if app.GlobalState.Schema["count"] != "string" || app.GlobalState.Initial["count"] != "0" {
		t.Errorf("globalState mis-parsed: %+v", app.GlobalState)
	}
	if len(app.Widgets) != 1 || app.Widgets[0].ID != "w1" ||
		len(app.Widgets[0].Lines) != 1 || app.Widgets[0].Lines[0].Label != "L" {
		t.Errorf("widgets mis-parsed: %+v", app.Widgets)
	}
	if len(app.Components) != 1 || app.Components["card"] == nil {
		t.Errorf("components mis-parsed: %+v", app.Components)
	}
	if len(app.DesignTokens) != 1 {
		t.Errorf("designTokens mis-parsed: %+v", app.DesignTokens)
	}
	if dt := app.DesignTokens["color.primary"]; dt.Type != "color" || dt.Value != "#fff" || !dt.Enforce {
		t.Errorf("token mis-parsed: %+v", dt)
	}
	if len(app.Shortcuts) != 1 || app.Shortcuts[0].Subtitle != "sub" || app.Shortcuts[0].Icon != "star" {
		t.Errorf("shortcuts mis-parsed: %+v", app.Shortcuts)
	}
	if len(app.DesktopMenu) != 1 || app.DesktopMenu[0].Items[0].Role != "quit" {
		t.Errorf("desktop menu mis-parsed: %+v", app.DesktopMenu)
	}
	if app.Tray.Icon != "tray.png" || app.Tray.Tip != "Hi" || len(app.Tray.Items) != 1 {
		t.Errorf("tray mis-parsed: %+v", app.Tray)
	}
	w := app.Window
	if w.Width != 800 || w.Height != 600 || w.Title != "Win" || !w.Resizable || !w.Chromeless || !w.Transparent {
		t.Errorf("window mis-parsed: %+v", w)
	}
	// Component expressions are type-checked with the schema parsed earlier in
	// the SAME manifest: count is string, so `state.count - 1` is a mismatch.
	found := false
	for _, d := range diags {
		if strings.Contains(d, "type mismatch") && strings.Contains(d, "state.count") {
			found = true
		}
	}
	if !found {
		t.Errorf("component type-check diagnostic missing, diags=%v", diags)
	}
}

func TestApplyManifestBrandingDefaultTrue(t *testing.T) {
	app, _ := loadManifest(map[string]any{"id": "x"})
	if !app.Branding {
		t.Error("branding must default to true when the key is absent")
	}
	app, _ = loadManifest(map[string]any{"id": "x", "branding": false})
	if app.Branding {
		t.Error("branding:false must turn branding off")
	}
	// Non-bool branding values coerce to false via asBool (no panic).
	app, _ = loadManifest(map[string]any{"id": "x", "branding": "yes"})
	if app.Branding {
		t.Error("non-bool branding should coerce to false")
	}
}

// TestApplyManifestMalformedSections feeds wrong-typed section values; every
// one must be skipped without panicking or corrupting earlier fields.
func TestApplyManifestMalformedSections(t *testing.T) {
	doc := map[string]any{
		"type": "app", "id": "m", "name": "M",
		"globalState":  "oops",
		"widgets":      "oops",
		"components":   []any{"oops"},
		"designTokens": []any{"oops"},
		"shortcuts":    map[string]any{"oops": true},
		"platforms":    "oops",
		"branding":     map[string]any{"x": 1},
	}
	app, diags := loadManifest(doc)
	if app.ID != "m" || app.Name != "M" {
		t.Fatalf("scalars corrupted by malformed sections: %+v", app)
	}
	if app.GlobalState.Schema != nil || app.Widgets != nil || app.Components != nil ||
		app.DesignTokens != nil || app.Shortcuts != nil || app.DesktopMenu != nil {
		t.Errorf("malformed sections must be ignored: %+v", app)
	}
	if app.Branding {
		t.Error("non-bool branding must coerce to false")
	}
	if len(diags) != 0 {
		t.Errorf("malformed sections should not diagnose, got %v", diags)
	}
}

// TestApplyManifestWindowNonNumeric coerces non-numeric window sizes to 0
// rather than panicking on adversarial input.
func TestApplyManifestWindowNonNumeric(t *testing.T) {
	doc := map[string]any{
		"type": "app",
		"platforms": map[string]any{
			"desktop": map[string]any{
				"window": map[string]any{"width": "big", "height": nil},
			},
		},
	}
	app, _ := loadManifest(doc)
	if app.Window.Width != 0 || app.Window.Height != 0 {
		t.Errorf("non-numeric window sizes must coerce to 0, got %+v", app.Window)
	}
}

func TestParseInvokeVariants(t *testing.T) {
	var diags []string

	if got := parseInvoke("", &diags, "s", "n", "onPress"); got != nil {
		t.Errorf("empty string shorthand must give nil, got %+v", got)
	}
	if got := parseInvoke(42.0, &diags, "s", "n", "onPress"); got != nil {
		t.Errorf("non-string/non-map must give nil, got %+v", got)
	}

	// Map form with args (values coerced to strings).
	got := parseInvoke(map[string]any{"name": "act", "args": map[string]any{"k": 5.0}}, &diags, "s", "n", "onPress")
	if got == nil || got.Name != "act" || got.Args["k"] != "5" {
		t.Errorf("map form mis-parsed: %+v", got)
	}

	// Deprecated scene:// map form warns AND strips the prefix.
	before := len(diags)
	got = parseInvoke(map[string]any{"name": "scene://target"}, &diags, "s", "n1", "onPress")
	if got == nil || got.Name != "target" {
		t.Errorf("scene:// prefix must be stripped, got %+v", got)
	}
	if len(diags) != before+1 || !strings.Contains(diags[before], "scene://") || !strings.Contains(diags[before], "n1") {
		t.Errorf("missing scene:// deprecation diagnostic: %v", diags[before:])
	}

	// Deprecated scene:// string form warns AND strips.
	before = len(diags)
	got = parseInvoke("scene://home", &diags, "s", "n2", "onChange")
	if got == nil || got.Name != "home" {
		t.Errorf("string scene:// must strip, got %+v", got)
	}
	if len(diags) != before+1 || !strings.Contains(diags[before], "onChange") {
		t.Errorf("missing string scene:// diagnostic: %v", diags[before:])
	}

	// Map with no args key yields an empty (non-nil) arg map.
	got = parseInvoke(map[string]any{"name": "x"}, &diags, "s", "n", "onPress")
	if got == nil || got.Args == nil || len(got.Args) != 0 {
		t.Errorf("map without args must give empty args map, got %+v", got)
	}
}

// TestBuildNodeSceneProtocolViaBuildNode pins the current behaviour of the
// exported BuildNode path (nil diagnostics): unlike FromDocs it does NOT strip
// a deprecated scene:// prefix from invoke names.
func TestBuildNodeSceneProtocolViaBuildNode(t *testing.T) {
	n := BuildNode(map[string]any{"type": "button", "id": "b", "onPress": "scene://home"})
	if n.OnPress == nil || n.OnPress.Name != "scene://home" {
		t.Fatalf("BuildNode should keep the raw name (no diags), got %+v", n.OnPress)
	}
}

func TestBuildActionFullStep(t *testing.T) {
	doc := map[string]any{
		"type": "action", "id": "sync",
		"steps": []any{
			map[string]any{
				"type": "http.request", "url": "https://x", "method": "POST",
				"body": "b", "headers": map[string]any{"K": "v"},
				"result": "resp", "error": "errMsg",
			},
			map[string]any{
				"type": "state.toggle", "path": "items",
				"matchKey": "id", "match": "{{ id }}", "field": "done",
			},
			map[string]any{"type": "navigate", "to": "scene://settings", "back": true, "from": "2"},
			map[string]any{"type": "state.appendObject", "path": "items", "item": map[string]any{"title": "t"}},
			"not-a-map", // skipped
		},
	}
	var diags []string
	act := buildAction(doc, &diags, nil)
	if act.ID != "sync" || len(act.Steps) != 4 {
		t.Fatalf("action mis-built: %+v", act)
	}
	s0 := act.Steps[0]
	if s0.URL != "https://x" || s0.Method != "POST" || s0.Body != "b" ||
		s0.Headers["K"] != "v" || s0.Result != "resp" || s0.Error != "errMsg" {
		t.Errorf("http step mis-parsed: %+v", s0)
	}
	s1 := act.Steps[1]
	if s1.MatchKey != "id" || s1.Match != "{{ id }}" || s1.Field != "done" {
		t.Errorf("toggle step mis-parsed: %+v", s1)
	}
	s2 := act.Steps[2]
	if s2.To != "settings" || !s2.Back || s2.From != "2" {
		t.Errorf("navigate step mis-parsed (scene:// must strip): %+v", s2)
	}
	if s3 := act.Steps[3]; s3.Object["title"] != "t" {
		t.Errorf("item step mis-parsed: %+v", s3)
	}
	// The navigate step's scene:// prefix produced exactly one deprecation diag.
	count := 0
	for _, d := range diags {
		if strings.Contains(d, "scene://") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("want 1 scene:// deprecation diag, got %d: %v", count, diags)
	}
}

func TestFromDocsSkipsIDlessActionAndRootlessScene(t *testing.T) {
	docs := []map[string]any{
		{"type": "scene", "id": "noroot"},                  // no root -> skipped
		{"type": "scene", "id": "badroot", "root": "oops"}, // non-map root -> skipped
		{"type": "action", "steps": []any{}},               // no id -> not stored
		{"type": "action", "id": "ok", "steps": []any{}},   // kept
	}
	app := FromDocs(docs)
	if len(app.Scenes) != 0 {
		t.Errorf("rootless scenes must be skipped, got %v", app.Scenes)
	}
	if len(app.Actions) != 1 || app.Actions["ok"] == nil {
		t.Errorf("only the id'd action must be stored, got %v", app.Actions)
	}
	if app.Entry != "main" {
		t.Errorf("no manifest -> Entry defaults to main, got %q", app.Entry)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("skipped docs should not diagnose, got %v", app.Diagnostics)
	}
}

func TestFromDocsEmpty(t *testing.T) {
	app := FromDocs(nil)
	if app == nil || app.Scenes == nil || app.Actions == nil || app.Entry != "main" {
		t.Fatalf("empty doc set must yield an initialized app, got %+v", app)
	}
}

func TestForEachExpr(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"plain text, no braces", nil},
		{"{{ state.count }}", []string{"state.count"}},
		{"{{ a.b }} then {{ c.d }}", []string{"a.b", "c.d"}},
		{"{{x}}{{ y }}", []string{"x", "y"}},
		{"{{ spaced   }}", []string{"spaced"}},
		{"{{ unclosed", nil},                   // no closing }} -> no expressions
		{"{{ x }} trailing }}", []string{"x"}}, // stray }} after a complete expr is inert
		{"before {{ y }}", []string{"y"}},
		{"empty {{}} expr", []string{""}},
		// "}}" inside a string literal is not the closing delimiter (quote
		// tracking mirrors the expression lexer, incl. backslash escapes).
		{"{{ '}}' }}", []string{"'}}'"}},
		{"{{ \"}}\" }}", []string{"\"}}\""}},
		{"a {{ x + '}}' }} b", []string{"x + '}}'"}},
		{"{{ 'esc \\\\ }}' }}", []string{"'esc \\\\ }}'"}}, // escaped quote stays inside the literal
		{"{{ 'unterminated }}", nil},                       // }} inside an open string closes nothing
		{"{{ '}}' }} then {{ z }}", []string{"'}}'", "z"}},
	}
	for _, c := range cases {
		var got []string
		forEachExpr(c.in, func(src string) { got = append(got, src) })
		if len(got) != len(c.want) {
			t.Errorf("forEachExpr(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("forEachExpr(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestAsStringCoercions(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"s", "s"},
		{float64(5), "5"},
		{float64(2.5), "2.5"},
		{float64(-3), "-3"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
		{[]any{float64(1), float64(2)}, "[1 2]"},
	}
	for _, c := range cases {
		if got := asString(c.in); got != c.want {
			t.Errorf("asString(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAsFloatCoercions(t *testing.T) {
	if got := asFloat(float64(3.5)); got != 3.5 {
		t.Errorf("asFloat(3.5) = %v", got)
	}
	// Integer Go types are accepted so ManifestToJSON's int window dims
	// round-trip through FromDocs without an intermediate JSON encode.
	if got := asFloat(300); got != 300 {
		t.Errorf("asFloat(300) = %v, want 300", got)
	}
	if got := asFloat(int64(200)); got != 200 {
		t.Errorf("asFloat(int64(200)) = %v, want 200", got)
	}
	for _, v := range []any{"x", nil, true, float64(0)} {
		if got := asFloat(v); got != 0 {
			t.Errorf("asFloat(%#v) = %v, want 0", v, got)
		}
	}
}

// TestCheckStepExprTypesRecurseIntoSubMaps verifies action-step type checking
// descends into nested objects (headers/params/item), not just top-level
// string fields.
func TestCheckStepExprTypesRecurseIntoSubMaps(t *testing.T) {
	docs := []map[string]any{
		{
			"type": "app",
			"globalState": map[string]any{
				"schema": map[string]any{"count": "string"},
			},
		},
		{
			"type": "action", "id": "call",
			"steps": []any{
				map[string]any{
					"type":    "http.request",
					"url":     "https://x",
					"headers": map[string]any{"X-Count": "{{ count - 1 }}"},
					"item":    map[string]any{"n": "{{ count * 2 }}"},
				},
			},
		},
	}
	app := FromDocs(docs)
	errs := typeErrors(app.Diagnostics)
	if len(errs) != 2 {
		t.Fatalf("want 2 nested type errors (headers + item), got %d: %v", len(errs), app.Diagnostics)
	}
	for _, e := range errs {
		if !strings.Contains(e, "[Action: call]") {
			t.Errorf("nested diagnostic missing action attribution: %s", e)
		}
	}
}
