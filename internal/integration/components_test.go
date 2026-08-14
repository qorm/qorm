package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/render"
	qrt "github.com/qorm/platform/internal/runtime"
)

// TestJSONComponents verifies app-defined JSON components: a component used
// twice with different props renders both prop sets, a {slot} fills with the
// instance's children, and repeated uses get unique ids.
func TestJSONComponents(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(`{
      "type":"app","id":"c","name":"C","entry":"main",
      "components":{
        "stat":{"type":"column","id":"sc","children":[{"type":"text","id":"sv","text":"{{prop.value}}"}]},
        "panel":{"type":"card","id":"pc","children":[{"type":"text","id":"pt","text":"{{prop.title}}"},{"type":"slot","id":"ps"}]}
      }}`), 0o644)
	os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(`{"type":"scene","id":"main","root":{"type":"scaffold","id":"root","children":[
      {"type":"stat","id":"a","value":"AAA"},
      {"type":"stat","id":"b","value":"BBB"},
      {"type":"panel","id":"p","title":"TITLE","children":[{"type":"text","id":"kid","text":"SLOTTED"}]}
    ]}}`), 0o644)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := render.Render(qrt.New(app))
	if len(res.Unknown) != 0 {
		t.Errorf("component types should be recognised, got unknown: %v", res.Unknown)
	}
	for _, want := range []string{"AAA", "BBB", "TITLE", "SLOTTED", `id="sv_a"`, `id="sv_b"`} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

// writeAppFile writes one file of a temp on-disk app, creating parent dirs.
func writeAppFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCrossFileComponentApp is the end-to-end test of the declared component
// model on a real directory: components live in their own components/*.json
// documents alongside scenes/ and actions/, declare typed props (with a
// default) and a required slot, and one of them instantiates the other. The
// app must load with ZERO diagnostics and render every declared surface —
// declared default included.
func TestCrossFileComponentApp(t *testing.T) {
	dir := t.TempDir()
	writeAppFile(t, dir, "qorm.json", `{
		"type":"app","id":"cf","name":"Cross-file components","entry":"main",
		"globalState":{"schema":{"total":"number"},"initial":{"total":42}}
	}`)
	writeAppFile(t, dir, "components/metric.json", `{
		"qorm":"0.1","type":"component","id":"metric",
		"props":{
			"label":{"type":"string","required":true},
			"value":{"type":"number","required":true},
			"unit":{"type":"string","default":"pts"}
		},
		"template":{"type":"column","id":"metric_root","children":[
			{"type":"text","id":"metric_label","text":"{{ prop.label }}"},
			{"type":"text","id":"metric_value","text":"{{ prop.value }} {{ prop.unit }}"}
		]}
	}`)
	writeAppFile(t, dir, "components/panel.json", `{
		"qorm":"0.1","type":"component","id":"panel",
		"props":{"title":"string"},
		"slots":{"header":{"required":false},"body":{"required":true}},
		"template":{"type":"card","id":"panel_root","children":[
			{"type":"text","id":"panel_title","text":"{{ prop.title }}"},
			{"type":"slot","name":"header"},
			{"type":"slot","name":"body","children":[{"type":"text","id":"panel_fallback","text":"EMPTY"}]}
		]}
	}`)
	writeAppFile(t, dir, "scenes/main.json", `{
		"type":"scene","id":"main",
		"root":{"type":"column","id":"root","children":[
			{"type":"panel","id":"p1","props":{"title":"Scores"},"children":[
				{"type":"metric","id":"m1","slot":"body","props":{"label":"Total","value":"{{ state.total }}"}},
				{"type":"metric","id":"m2","slot":"body","props":{"label":"Bonus","value":5,"unit":"x"}}
			]},
			{"type":"component","id":"p2","ref":"component://panel","props":{"title":"Ref form"},"children":[
				{"type":"text","id":"rb","slot":"body","text":"REFBODY"}
			]}
		]}
	}`)

	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Diagnostics) != 0 {
		t.Fatalf("a declared cross-file component app must load clean: %v", app.Diagnostics)
	}
	for _, name := range []string{"metric", "panel"} {
		if app.Components[name] == nil || app.ComponentSchemas[name] == nil {
			t.Fatalf("component %q did not register with its declaration", name)
		}
	}

	res := render.Render(qrt.New(app))
	if len(res.Unknown) != 0 {
		t.Errorf("declared components must render, got unknown: %v", res.Unknown)
	}
	for _, want := range []string{
		"Scores", "Ref form", "REFBODY",
		"Total", "42 pts", // required props + the declared default
		"Bonus", "5 x", // an instance value overrides the default
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("rendered HTML missing %q:\n%s", want, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "EMPTY") {
		t.Errorf("a filled slot must not render its fallback:\n%s", res.HTML)
	}
}

// TestCrossFileComponentDiagnostics is the negative of the above: the same
// declarations must reject a wrong instance at load time, with one error per
// violated rule and nothing else.
func TestCrossFileComponentDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeAppFile(t, dir, "qorm.json", `{"type":"app","id":"cf","entry":"main"}`)
	writeAppFile(t, dir, "components/metric.json", `{
		"type":"component","id":"metric",
		"props":{"label":{"type":"string","required":true},"value":{"type":"number"}},
		"slots":{"body":{"required":true}},
		"template":{"type":"column","id":"mr","children":[{"type":"slot","name":"body"}]}
	}`)
	writeAppFile(t, dir, "scenes/main.json", `{
		"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"metric","id":"bad","props":{"value":"not a number","stray":1}}
		]}}`)

	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`缺少必填 prop "label"`,
		`prop "value" 声明类型为 "number"`,
		`未声明的 prop "stray"`,
		`缺少必填 slot "body"`,
	}
	for _, w := range want {
		found := false
		for _, d := range app.Diagnostics {
			if strings.Contains(d, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing diagnostic %q in %v", w, app.Diagnostics)
		}
	}
	if len(app.Diagnostics) != len(want) {
		t.Errorf("want exactly %d diagnostics, got %d: %v", len(want), len(app.Diagnostics), app.Diagnostics)
	}
}
