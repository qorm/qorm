package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// writeApp writes a minimal QORM app (manifest + one scene) into a temp dir.
func writeApp(t *testing.T, manifest, scene string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestComponentPipeline exercises the full load -> render pipeline for an
// app-defined component: its props (incl. the text shorthand) resolve inside the
// template and its children fill the slot.
func TestComponentPipeline(t *testing.T) {
	dir := writeApp(t,
		`{"type":"app","id":"cmp","entry":"main","components":{`+
			`"Card":{"type":"card","children":[`+
			`{"type":"text","id":"title","text":"{{prop.title}}"},`+
			`{"type":"slot"}]}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","children":[`+
			`{"type":"Card","id":"c1","title":"Heading","children":[{"type":"text","id":"body","text":"BODY1"}]},`+
			`{"type":"Card","id":"c2","title":"Other","children":[{"type":"text","id":"body","text":"BODY2"}]}]}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	html := render.Render(qrt.New(app)).HTML

	for _, w := range []string{"Heading", "Other", "BODY1", "BODY2"} {
		if !strings.Contains(html, w) {
			t.Errorf("component pipeline lacks %q:\n%s", w, html)
		}
	}
	// per-instance id suffixing survives the load/render round trip
	for _, w := range []string{`id="title_c1"`, `id="title_c2"`, `id="body_c1"`, `id="body_c2"`} {
		if !strings.Contains(html, w) {
			t.Errorf("component ids should be suffixed per instance, lacks %q:\n%s", w, html)
		}
	}
	if strings.Contains(html, "data-qorm-unknown") {
		t.Errorf("component instance rendered as unknown:\n%s", html)
	}
}

// TestDynamicComponentPipeline exercises the dynamic component model through
// the full load -> render -> dispatch pipeline from real JSON: state-bound
// props (typed), a spec-style nested props object, a callback prop resolving
// to a dispatchable action, and named slots with fallback content.
func TestDynamicComponentPipeline(t *testing.T) {
	dir := writeApp(t,
		`{"type":"app","id":"dyn","entry":"main",`+
			`"globalState":{"schema":{"open":"boolean","user":"string","saved":"boolean"},`+
			`"initial":{"open":true,"user":"Ada","saved":false}},`+
			`"components":{`+
			`"Panel":{"type":"card","children":[`+
			`{"type":"slot","name":"header","children":[{"type":"text","id":"hf","text":"NO_HEADER"}]},`+
			`{"type":"text","id":"who","text":"USER={{prop.user}}","if":"{{ prop.open }}"},`+
			`{"type":"button","id":"save","label":"Save","onPress":{"name":"{{prop.onSave}}"}},`+
			`{"type":"slot"}]}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","children":[`+
			`{"type":"Panel","id":"p1",`+
			`"props":{"user":"{{state.user}}","open":"{{state.open}}","onSave":"persist"},`+
			`"children":[`+
			`{"type":"text","id":"h","text":"MY_HEADER","slot":"header"},`+
			`{"type":"text","id":"b","text":"BODY"}]}]}}`)
	if err := os.MkdirAll(filepath.Join(dir, "actions"), 0o755); err != nil {
		t.Fatal(err)
	}
	action := `{"type":"action","id":"persist","steps":[{"type":"state.set","path":"saved","value":"{{ true }}"}]}`
	if err := os.WriteFile(filepath.Join(dir, "actions", "persist.json"), []byte(action), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rt := qrt.New(app)
	res := render.Render(rt)

	// typed state-bound props: open=true shows the guarded node, user resolves
	if !strings.Contains(res.HTML, "USER=Ada") {
		t.Errorf("nested props object with state binding not resolved:\n%s", res.HTML)
	}
	// named slot took the attributed child, fallback suppressed, default slot took the rest
	if !strings.Contains(res.HTML, "MY_HEADER") || strings.Contains(res.HTML, "NO_HEADER") {
		t.Errorf("named slot distribution/fallback wrong:\n%s", res.HTML)
	}
	if strings.Count(res.HTML, "BODY") != 1 {
		t.Errorf("default slot should render the unattributed child exactly once:\n%s", res.HTML)
	}
	// callback prop: handler carries the final action name and really dispatches
	var dispatched bool
	for _, h := range res.Handlers {
		if h.Name == "persist" {
			rt.Dispatch(h.Name, nil)
			dispatched = true
		}
	}
	if !dispatched {
		t.Fatalf("callback prop did not resolve to 'persist', handlers: %+v", res.Handlers)
	}
	if rt.State["saved"] != true {
		t.Errorf("dispatching the resolved callback did not run the action, state: %v", rt.State)
	}
	// flip the bound state: the prop re-evaluates on the next render
	rt.State["open"] = false
	if html := render.Render(rt).HTML; strings.Contains(html, "USER=Ada") {
		t.Errorf("prop bound to state.open=false should hide the guarded node:\n%s", html)
	}
}

// TestUnknownWidgetReportedThroughLoader guards that a typo'd widget type in a
// loaded scene is surfaced via Result.Unknown (the self-verify surface) while
// still rendering a container.
func TestUnknownWidgetReportedThroughLoader(t *testing.T) {
	dir := writeApp(t,
		`{"type":"app","id":"unk","entry":"main"}`,
		`{"type":"scene","id":"main","root":{"type":"colunm","id":"oops","children":[{"type":"text","text":"KEPT"}]}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := render.Render(qrt.New(app))
	if !strings.Contains(res.HTML, `data-qorm-unknown="colunm"`) {
		t.Errorf("typo'd widget should be tagged unknown:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "KEPT") {
		t.Errorf("unknown widget children should still render:\n%s", res.HTML)
	}
	found := false
	for _, u := range res.Unknown {
		if u == "colunm" {
			found = true
		}
	}
	if !found {
		t.Errorf("Result.Unknown should report 'colunm', got %v", res.Unknown)
	}
}
