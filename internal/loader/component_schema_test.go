package loader

// Component MODEL tests: declared props/slots schemas, the standalone
// type:"component" definition document, and the load-time checks both enable.
//
// The invariants under test:
//
//   - A component that declares nothing behaves exactly as it did before
//     schemas existed (no checks, no diagnostics, bare-template serialisation).
//   - A declaration is checked against every instance in every scene AND every
//     component template, deterministically ordered.
//   - The inline qorm.json "components" map and standalone component documents
//     are the same registry: they merge, and a name collision is diagnosed
//     rather than silently resolved by file order.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// diagsWith returns the diagnostics containing sub (for asserting on one
// specific message without pinning the whole set).
func diagsWith(app *model.App, sub string) []string {
	var out []string
	for _, d := range app.Diagnostics {
		if strings.Contains(d, sub) {
			out = append(out, d)
		}
	}
	return out
}

// compDiag reports whether any diagnostic contains every one of subs.
func compDiag(app *model.App, subs ...string) bool {
	for _, d := range app.Diagnostics {
		all := true
		for _, s := range subs {
			if !strings.Contains(d, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// componentApp assembles a manifest whose components map is the given raw JSON
// plus one scene whose root children are the given raw JSON nodes.
func componentApp(t *testing.T, components, sceneChildren string) *model.App {
	t.Helper()
	doc := func(src string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(src), &m); err != nil {
			t.Fatalf("fixture is not valid JSON: %v\n%s", err, src)
		}
		return m
	}
	return FromDocs([]map[string]any{
		doc(`{"type":"app","id":"c","entry":"main","components":` + components + `}`),
		doc(`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[` + sceneChildren + `]}}`),
	})
}

// ---- declaration parsing ---------------------------------------------------

// TestComponentSchemaShorthandAndLongForm covers both spellings of a prop
// declaration and the slots object, exactly as the JSON format spec writes them.
func TestComponentSchemaShorthandAndLongForm(t *testing.T) {
	app := componentApp(t, `{
		"panel": {
			"props": {
				"title": "string",
				"count": {"type": "number", "default": 0, "required": false},
				"open":  {"type": "boolean", "required": true}
			},
			"slots": {"header": {"required": false}, "body": {"required": true}},
			"template": {"type": "card", "id": "p", "children": [
				{"type": "slot", "name": "header"},
				{"type": "slot", "name": "body"}
			]}
		}
	}`, `{"type":"panel","id":"i1","title":"T","open":true,"children":[
		{"type":"text","id":"b","slot":"body","text":"B"}]}`)

	sc := app.ComponentSchemas["panel"]
	if sc == nil {
		t.Fatal("component declaration was not parsed")
	}
	want := map[string]model.PropSpec{
		"title": {Type: "string"},
		"count": {Type: "number", Default: float64(0)},
		"open":  {Type: "boolean", Required: true},
	}
	if !reflect.DeepEqual(sc.Props, want) {
		t.Errorf("props schema: want %+v got %+v", want, sc.Props)
	}
	wantSlots := map[string]model.SlotSpec{"header": {}, "body": {Required: true}}
	if !reflect.DeepEqual(sc.Slots, wantSlots) {
		t.Errorf("slots schema: want %+v got %+v", wantSlots, sc.Slots)
	}
	// The template — not the declaration wrapper — is what got built.
	if root := app.Components["panel"]; root == nil || root.Type != "card" || root.ID != "p" {
		t.Errorf("declaration form must register the template node, got %+v", root)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("a well-formed declaration must load clean: %v", app.Diagnostics)
	}
}

// TestComponentPropTypeAliases pins the accepted type spellings (the same
// aliases the expression checker accepts) and the unknown-type warning.
func TestComponentPropTypeAliases(t *testing.T) {
	cases := map[string]string{
		"string": "string", "str": "string", "text": "string",
		"number": "number", "num": "number", "int": "number",
		"integer": "number", "float": "number", "double": "number",
		"bool": "boolean", "boolean": "boolean", "BOOLEAN": "boolean",
		"array": "array", "list": "array",
		"object": "object", "map": "object",
		"any": "", "": "",
	}
	for decl, want := range cases {
		var diags []string
		if got := normalizePropType("c", "p", decl, &diags); got != want {
			t.Errorf("normalizePropType(%q) = %q, want %q", decl, got, want)
		}
		if len(diags) != 0 {
			t.Errorf("normalizePropType(%q) must not warn: %v", decl, diags)
		}
	}
	var diags []string
	if got := normalizePropType("c", "p", "widget", &diags); got != "" {
		t.Errorf("an unknown type must be unconstrained, got %q", got)
	}
	if len(diags) != 1 || !strings.Contains(diags[0], "未知类型") {
		t.Errorf("an unknown type must warn once, got %v", diags)
	}
}

// TestComponentSchemaMalformedDeclarations covers the declaration-side
// diagnostics: an unusable prop/slot spelling, a default that contradicts its
// own declared type, and required+default together.
func TestComponentSchemaMalformedDeclarations(t *testing.T) {
	app := componentApp(t, `{
		"c": {
			"props": {
				"bad":   ["not", "a", "spec"],
				"none":  null,
				"wrong": {"type": "number", "default": "abc"},
				"both":  {"type": "string", "default": "x", "required": true}
			},
			"slots": {"ok": {}, "weird": "yes", "nul": null},
			"template": {"type": "text", "id": "t", "text": "x"}
		}
	}`, "")
	for _, want := range []string{
		`prop "bad" 声明格式无法识别`,
		`prop "wrong" 声明类型为 "number"`,
		`prop "both" 同时声明了 required 与 default`,
		`slot "weird" 声明格式无法识别`,
	} {
		if len(diagsWith(app, want)) != 1 {
			t.Errorf("missing (or duplicated) diagnostic %q in %v", want, app.Diagnostics)
		}
	}
	sc := app.ComponentSchemas["c"]
	if sc.Props["bad"].Type != "" || sc.Props["none"].Type != "" {
		t.Errorf("an unusable prop declaration must degrade to unconstrained: %+v", sc.Props)
	}
	if sc.Slots["weird"].Required || sc.Slots["nul"].Required {
		t.Errorf("an unusable slot declaration must degrade to optional: %+v", sc.Slots)
	}
}

// TestComponentSchemaOnlySlots guards the props-less declaration: an undeclared
// prop must NOT be reported when the component declared no props map at all.
func TestComponentSchemaOnlySlots(t *testing.T) {
	app := componentApp(t, `{
		"box": {"slots": {"body": {}}, "template": {"type":"card","id":"bx","children":[{"type":"slot","name":"body"}]}}
	}`, `{"type":"box","id":"b1","props":{"anything":"goes"}}`)
	if sc := app.ComponentSchemas["box"]; sc == nil || sc.Props != nil {
		t.Errorf("a slots-only declaration must leave Props nil, got %+v", sc)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("a slots-only declaration must not police props: %v", app.Diagnostics)
	}
}

// ---- instance checks -------------------------------------------------------

// TestComponentInstanceRequiredProp: a missing required prop is an error, and
// supplying it — by any of the accepted spellings — clears it.
func TestComponentInstanceRequiredProp(t *testing.T) {
	comps := `{"chip": {"props": {"label": {"type":"string","required":true}},
		"template": {"type":"text","id":"c","text":"{{ prop.label }}"}}}`

	missing := componentApp(t, comps, `{"type":"chip","id":"c1"}`)
	if !compDiag(missing, "error:", `实例 (id: "c1")`, `缺少必填 prop "label"`) {
		t.Errorf("a missing required prop must be an error: %v", missing.Diagnostics)
	}
	// The label shorthand (a typed node field, not a props key) counts.
	viaShorthand := componentApp(t, comps, `{"type":"chip","id":"c1","label":"Hi"}`)
	if len(viaShorthand.Diagnostics) != 0 {
		t.Errorf("the label shorthand must satisfy the declaration: %v", viaShorthand.Diagnostics)
	}
	// So does the spec's nested props object.
	viaNested := componentApp(t, comps, `{"type":"chip","id":"c1","props":{"label":"Hi"}}`)
	if len(viaNested.Diagnostics) != 0 {
		t.Errorf("the nested props object must satisfy the declaration: %v", viaNested.Diagnostics)
	}
	// A default does not satisfy `required` — the instance still has to pass it.
	if compDiag(viaNested, "缺少必填") {
		t.Fatal("unreachable guard tripped")
	}
}

// TestComponentInstancePropTypes walks every branch of the literal type check:
// what matches, what is diagnosed, and what is deliberately unknowable.
func TestComponentInstancePropTypes(t *testing.T) {
	type row struct {
		decl, value string
		wantErr     bool
	}
	rows := []row{
		{"string", `"text"`, false},
		{"string", `7`, false},               // numbers stringify losslessly
		{"string", `true`, false},            // so do booleans
		{"string", `["a"]`, true},            // an array cannot be a string
		{"string", `{"a":1}`, true},          // nor can an object
		{"number", `7`, false},               //
		{"number", `"7.5"`, false},           // the shorthand fields are strings by construction
		{"number", `"abc"`, true},            //
		{"number", `true`, true},             //
		{"number", `[1]`, true},              //
		{"boolean", `true`, false},           //
		{"boolean", `"false"`, false},        //
		{"boolean", `"nope"`, true},          //
		{"boolean", `3`, true},               //
		{"array", `[1,2]`, false},            //
		{"array", `"x"`, true},               //
		{"array", `{"a":1}`, true},           //
		{"object", `{"a":1}`, false},         //
		{"object", `[1]`, true},              //
		{"object", `"x"`, true},              //
		{"any", `[1]`, false},                // an unconstrained declaration matches anything
		{"string", `null`, false},            // an explicit null is "not supplied enough" to judge
		{"number", `"{{ state.n }}"`, false}, // a binding's value only exists at render time
	}
	for _, r := range rows {
		app := componentApp(t,
			`{"w":{"props":{"v":"`+r.decl+`"},"template":{"type":"text","id":"w","text":"x"}}}`,
			`{"type":"w","id":"i","props":{"v":`+r.value+`}}`)
		got := compDiag(app, "error:", `的 prop "v" 声明类型为`)
		if got != r.wantErr {
			t.Errorf("decl %s value %s: mismatch reported = %v, want %v (%v)",
				r.decl, r.value, got, r.wantErr, app.Diagnostics)
		}
	}
}

// TestComponentInstanceUndeclaredProp: an undeclared key in the nested props
// object warns, while the same key spelled as a top-level node field does not —
// top-level keys are indistinguishable from structural node fields.
func TestComponentInstanceUndeclaredProp(t *testing.T) {
	comps := `{"w":{"props":{"v":"string"},"template":{"type":"text","id":"w","text":"x"}}}`

	nested := componentApp(t, comps, `{"type":"w","id":"i","props":{"v":"a","stray":"b"}}`)
	if !compDiag(nested, "warning:", `未声明的 prop "stray"`) {
		t.Errorf("an undeclared nested prop must warn: %v", nested.Diagnostics)
	}
	if compDiag(nested, `未声明的 prop "v"`) {
		t.Errorf("a declared prop must not warn: %v", nested.Diagnostics)
	}
	topLevel := componentApp(t, comps, `{"type":"w","id":"i","v":"a","stray":"b","style":{"gap":4}}`)
	if len(topLevel.Diagnostics) != 0 {
		t.Errorf("top-level keys must never be policed as undeclared props: %v", topLevel.Diagnostics)
	}
}

// TestComponentInstanceSlots covers the slot declaration checks: a missing
// required slot is an error, an undeclared slot attribution warns, and the
// unnamed (default) slot is never policed.
func TestComponentInstanceSlots(t *testing.T) {
	comps := `{"panel":{"slots":{"header":{"required":false},"body":{"required":true}},
		"template":{"type":"card","id":"p","children":[
			{"type":"slot","name":"header"},{"type":"slot","name":"body"}]}}}`

	missing := componentApp(t, comps, `{"type":"panel","id":"p1","children":[
		{"type":"text","id":"h","slot":"header","text":"H"}]}`)
	if !compDiag(missing, "error:", `实例 (id: "p1")`, `缺少必填 slot "body"`) {
		t.Errorf("a missing required slot must be an error: %v", missing.Diagnostics)
	}
	if compDiag(missing, `缺少必填 slot "header"`) {
		t.Errorf("an optional slot must not be required: %v", missing.Diagnostics)
	}

	filled := componentApp(t, comps, `{"type":"panel","id":"p1","children":[
		{"type":"text","id":"b","slot":"body","text":"B"}]}`)
	if len(filled.Diagnostics) != 0 {
		t.Errorf("a filled required slot must load clean: %v", filled.Diagnostics)
	}

	stray := componentApp(t, comps, `{"type":"panel","id":"p1","children":[
		{"type":"text","id":"b","slot":"body","text":"B"},
		{"type":"text","id":"x","slot":"footer","text":"F"},
		{"type":"text","id":"d","text":"default slot child"}]}`)
	if !compDiag(stray, "warning:", `子节点 (id: "x")`, `未声明的 slot "footer"`) {
		t.Errorf("an undeclared slot attribution must warn: %v", stray.Diagnostics)
	}
	if len(stray.Diagnostics) != 1 {
		t.Errorf("the unnamed default-slot child must not be policed: %v", stray.Diagnostics)
	}
}

// TestComponentChecksReachNestedScopes proves the instance checks walk
// everything: list item templates, `when` branches, and other components'
// templates (a component instantiating a component).
func TestComponentChecksReachNestedScopes(t *testing.T) {
	app := componentApp(t, `{
		"chip": {"props":{"label":{"type":"string","required":true}},
			"template":{"type":"text","id":"c","text":"{{ prop.label }}"}},
		"host": {"template":{"type":"column","id":"h","children":[{"type":"chip","id":"inner"}]}}
	}`, `{"type":"list","id":"l","data":"{{ state.items }}","renderItem":{"type":"chip","id":"row"}},
		{"type":"when","id":"w","condition":"{{ viewport.width > 10 }}",
		 "then":{"type":"chip","id":"thenChip"},"else":{"type":"chip","id":"elseChip"}}`)

	for _, id := range []string{"row", "thenChip", "elseChip", "inner"} {
		if !compDiag(app, `实例 (id: "`+id+`") 缺少必填 prop "label"`) {
			t.Errorf("instance %q was not checked: %v", id, app.Diagnostics)
		}
	}
	if !compDiag(app, "[Scene: component:host]") {
		t.Errorf("a component template must be its own diagnostic scope: %v", app.Diagnostics)
	}
}

// TestComponentDeclaredPropsTypeCheckTemplate: declared prop types join the
// template's expression type-check scope, so a numeric operator on a string
// prop is caught inside the component itself.
func TestComponentDeclaredPropsTypeCheckTemplate(t *testing.T) {
	app := componentApp(t, `{
		"c": {"props":{"title":"string","n":"number"},
			"template":{"type":"text","id":"c","text":"{{ prop.title * 2 }} {{ prop.n + 1 }}"}}
	}`, "")
	if !compDiag(app, "[Scene: component:c]", "type mismatch") {
		t.Errorf("declared prop types must type-check the template: %v", app.Diagnostics)
	}
	if len(diagsWith(app, "type mismatch")) != 1 {
		t.Errorf("only the string*number expression is wrong: %v", app.Diagnostics)
	}
}

// ---- backward compatibility ------------------------------------------------

// TestUndeclaredComponentIsUnchecked is the back-compat guard: a component with
// no declaration accepts anything, exactly as before schemas existed.
func TestUndeclaredComponentIsUnchecked(t *testing.T) {
	app := componentApp(t,
		`{"card":{"type":"card","id":"cd","children":[{"type":"text","id":"t","text":"{{ prop.title }}"},{"type":"slot"}]}}`,
		`{"type":"card","id":"c1","props":{"whatever":[1,2,3]},"children":[{"type":"text","id":"k","slot":"nope","text":"x"}]}`)
	if len(app.ComponentSchemas) != 0 {
		t.Errorf("an undeclared component must have no schema: %+v", app.ComponentSchemas)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("an undeclared component must be unchecked: %v", app.Diagnostics)
	}
	if root := app.Components["card"]; root == nil || root.Type != "card" {
		t.Errorf("the legacy form registers the definition object itself as the template, got %+v", root)
	}
}

// ---- cross-file component documents ---------------------------------------

// TestComponentDocument loads a standalone type:"component" document and checks
// it registers exactly like an inline definition, declaration included.
func TestComponentDocument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"),
		`{"type":"app","id":"x","entry":"main","globalState":{"schema":{"n":"number"}}}`)
	writeFile(t, filepath.Join(dir, "components", "panel.json"), `{
		"qorm": "0.1", "type": "component", "id": "panel",
		"props": {"title": "string"},
		"slots": {"body": {"required": true}},
		"template": {"type":"card","id":"panel_root","children":[
			{"type":"text","id":"panel_title","text":"{{ prop.title }}"},
			{"type":"slot","name":"body"}]}
	}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"), `{
		"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"panel","id":"p1","title":"Hello","children":[
				{"type":"text","id":"pb","slot":"body","text":"{{ state.n }}"}]}]}}`)

	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a well-formed cross-file component must load clean: %v", app.Diagnostics)
	}
	if root := app.Components["panel"]; root == nil || root.ID != "panel_root" {
		t.Fatalf("component document did not register its template: %+v", root)
	}
	sc := app.ComponentSchemas["panel"]
	if sc == nil || sc.Props["title"].Type != "string" || !sc.Slots["body"].Required {
		t.Fatalf("component document did not register its declaration: %+v", sc)
	}
	// The declaration is enforced against instances in other files.
	bad := componentDocApp(t, dir, `{"type":"panel","id":"p2"}`)
	if !compDiag(bad, `缺少必填 slot "body"`) {
		t.Errorf("a cross-file declaration must police instances: %v", bad.Diagnostics)
	}
}

// componentDocApp reloads dir with the scene root children replaced, for
// exercising a cross-file component's checks against different instances.
func componentDocApp(t *testing.T, dir, children string) *model.App {
	t.Helper()
	writeFile(t, filepath.Join(dir, "scenes", "main.json"),
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[`+children+`]}}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// TestComponentDocumentMalformed covers the two ways a component document is
// unusable: no id (no name to register under) and no template (nothing to
// render). Both are errors, and neither pollutes the registry.
func TestComponentDocumentMalformed(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "r"}},
		{"type": "component", "template": map[string]any{"type": "text"}},
		{"type": "component", "id": "noTemplate", "props": map[string]any{"a": "string"}},
	})
	if !compDiag(app, "error:", `缺少 "id"`) {
		t.Errorf("an id-less component document must error: %v", app.Diagnostics)
	}
	if !compDiag(app, "error:", `组件文档 "noTemplate" 缺少 "template"`) {
		t.Errorf("a template-less component document must error: %v", app.Diagnostics)
	}
	if len(app.Components) != 0 || len(app.ComponentSchemas) != 0 {
		t.Errorf("a malformed component document must register nothing: %+v %+v", app.Components, app.ComponentSchemas)
	}
}

// TestComponentRedefinitionDiagnosed: the inline map and the documents are one
// registry, so a name collision is reported and resolved deterministically (the
// manifest is applied first, so its definition is the one that renders).
func TestComponentRedefinitionDiagnosed(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main", "components": map[string]any{
			"panel": map[string]any{"type": "card", "id": "inline"},
		}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "r"}},
		{"type": "component", "id": "panel", "template": map[string]any{"type": "row", "id": "fromFile"}},
		{"type": "component", "id": "panel", "template": map[string]any{"type": "row", "id": "fromFile2"}},
	})
	if n := len(diagsWith(app, `组件 "panel" 被重复定义`)); n != 2 {
		t.Errorf("both redefinitions must be diagnosed, got %d: %v", n, app.Diagnostics)
	}
	if root := app.Components["panel"]; root == nil || root.ID != "inline" {
		t.Errorf("the first definition (the manifest's) must win, got %+v", root)
	}
}

// TestComponentDocumentIsNotAnUnknownDocument guards the doc-type dispatch: a
// component document must not fall into the "unknown or missing type" branch.
func TestComponentDocumentIsNotAnUnknownDocument(t *testing.T) {
	app := FromDocs([]map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "r"}},
		{"type": "component", "id": "c", "template": map[string]any{"type": "text", "id": "t", "text": "x"}},
	})
	if compDiag(app, "unknown or missing") {
		t.Errorf("component documents must be a recognised type: %v", app.Diagnostics)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("want a clean load: %v", app.Diagnostics)
	}
}

// ---- the spec's explicit instance form -------------------------------------

// TestComponentRefInstanceChecked: {"type":"component","ref":"panel"} is a
// component instance for the checks too, in both the shorthand and the
// canonical component:// spelling — and an unresolvable ref is left alone.
func TestComponentRefInstanceChecked(t *testing.T) {
	comps := `{"panel":{"props":{"title":{"type":"string","required":true}},
		"template":{"type":"card","id":"p","children":[{"type":"text","id":"pt","text":"{{ prop.title }}"}]}}}`

	for _, ref := range []string{"panel", "component://panel"} {
		app := componentApp(t, comps, `{"type":"component","id":"i","ref":"`+ref+`"}`)
		if !compDiag(app, `实例 (id: "i") 缺少必填 prop "title"`) {
			t.Errorf("ref %q was not resolved to its component: %v", ref, app.Diagnostics)
		}
	}
	ok := componentApp(t, comps, `{"type":"component","id":"i","ref":"panel","props":{"title":"T"}}`)
	if len(ok.Diagnostics) != 0 {
		t.Errorf("a satisfied ref instance must load clean: %v", ok.Diagnostics)
	}
	for _, ref := range []string{"", "nosuch", "{{ state.which }}"} {
		app := componentApp(t, comps, `{"type":"component","id":"i","ref":"`+ref+`"}`)
		if len(app.Diagnostics) != 0 {
			t.Errorf("an unresolvable ref %q must not be checked: %v", ref, app.Diagnostics)
		}
	}
}

// ---- serialisation ---------------------------------------------------------

// TestComponentSchemaRoundTrip: a declared component survives
// AppToDocs -> FromDocs unchanged, and an undeclared one keeps its historical
// bare-template spelling on the way out.
func TestComponentSchemaRoundTrip(t *testing.T) {
	app := componentApp(t, `{
		"panel": {
			"props": {"title":"string","count":{"type":"number","default":3,"required":false},
			          "open":{"type":"boolean","required":true},"free":{}},
			"slots": {"header":{},"body":{"required":true}},
			"template": {"type":"card","id":"p","children":[{"type":"slot","name":"body"}]}
		},
		"plain": {"type":"text","id":"pl","text":"x"}
	}`, "")

	docs := AppToDocs(app)
	comps, ok := docs[0]["components"].(map[string]any)
	if !ok {
		t.Fatal("manifest lost its components")
	}
	if _, declared := comps["panel"].(map[string]any)["template"]; !declared {
		t.Errorf("a declared component must serialise in the declaration form: %v", comps["panel"])
	}
	if plain := comps["plain"].(map[string]any); plain["type"] != "text" {
		t.Errorf("an undeclared component must serialise as its bare template: %v", plain)
	}

	back := FromDocs(docs)
	if !reflect.DeepEqual(app.ComponentSchemas, back.ComponentSchemas) {
		t.Errorf("schemas lost in the round trip:\nwant %+v\ngot  %+v", app.ComponentSchemas["panel"], back.ComponentSchemas["panel"])
	}
	if root := back.Components["panel"]; root == nil || root.ID != "p" {
		t.Errorf("template lost in the round trip: %+v", root)
	}
	if len(back.Diagnostics) != 0 {
		t.Errorf("the round trip must not invent diagnostics: %v", back.Diagnostics)
	}
	// And the doc form is a fixpoint (what bundle.FromApp hashes).
	if !reflect.DeepEqual(jsonNorm(t, docs), jsonNorm(t, AppToDocs(back))) {
		t.Error("AppToDocs is not a fixpoint over a declared component")
	}
}

// TestCrossFileComponentRoundTrip: components authored as their own documents
// come back nested in the manifest (the one canonical output form) with their
// declaration intact.
func TestCrossFileComponentRoundTrip(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "x", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "r"}},
		{"type": "component", "id": "panel",
			"props":    map[string]any{"title": "string"},
			"slots":    map[string]any{"body": map[string]any{"required": true}},
			"template": map[string]any{"type": "card", "id": "p", "children": []any{map[string]any{"type": "slot", "name": "body"}}}},
	}
	app := FromDocs(docs)
	out := AppToDocs(app)
	if len(out) != 2 {
		t.Fatalf("components must fold into the manifest, got %d docs", len(out))
	}
	back := FromDocs(out)
	if !reflect.DeepEqual(app.ComponentSchemas, back.ComponentSchemas) {
		t.Errorf("cross-file schema lost: want %+v got %+v", app.ComponentSchemas, back.ComponentSchemas)
	}
	if back.Components["panel"] == nil || back.Components["panel"].ID != "p" {
		t.Errorf("cross-file template lost: %+v", back.Components)
	}
}

// jsonNorm canonicalises a doc set through JSON so maps of any compare by value.
func jsonNorm(t *testing.T, docs []map[string]any) any {
	t.Helper()
	b, err := json.Marshal(docs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestComponentDirOnDiskMergesWithInline is the on-disk convention test:
// components/*.json and the manifest's inline map fill one registry.
func TestComponentDirOnDiskMergesWithInline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "qorm.json"), `{"type":"app","id":"x","entry":"main",
		"components":{"inlineOnly":{"type":"text","id":"io","text":"i"}}}`)
	writeFile(t, filepath.Join(dir, "components", "a.json"),
		`{"type":"component","id":"fileA","template":{"type":"text","id":"fa","text":"a"}}`)
	writeFile(t, filepath.Join(dir, "components", "b.json"),
		`{"type":"component","id":"fileB","props":{"n":"number"},"template":{"type":"text","id":"fb","text":"{{ prop.n }}"}}`)
	writeFile(t, filepath.Join(dir, "scenes", "main.json"),
		`{"type":"scene","id":"main","root":{"type":"column","id":"r","children":[
			{"type":"inlineOnly","id":"x1"},{"type":"fileA","id":"x2"},{"type":"fileB","id":"x3","props":{"n":2}}]}}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Diagnostics) != 0 {
		t.Fatalf("merged registry must load clean: %v", app.Diagnostics)
	}
	for _, name := range []string{"inlineOnly", "fileA", "fileB"} {
		if app.Components[name] == nil {
			t.Errorf("component %q missing from the merged registry", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "components")); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}
