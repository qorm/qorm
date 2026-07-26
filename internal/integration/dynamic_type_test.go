package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestDynamicTypePolymorphicFeedEndToEnd drives dynamic type binding through the
// full pipeline (files on disk -> LoadDir static checks -> render): a chat feed
// whose ONE renderItem template types itself from each message's own `kind`, so
// bubbles, system notices, status pills and separators come out of a single
// template instead of one `when`/`if` branch per kind.
//
// It locks in three things at once: the loader must stay silent (it cannot know
// the widget statically, so it must not guess), each kind must reach its real
// renderer — including an app COMPONENT name, which the render dispatch resolves
// before the built-in widget switch — and a kind that names nothing must degrade
// through the data-qorm-unknown path so the self-verify surface still catches
// the typo.
func TestDynamicTypePolymorphicFeedEndToEnd(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("qorm.json", `{ "type": "app", "id": "feed", "name": "Feed", "entry": "main",
	  "components": {
	    "Bubble": { "type": "card", "id": "bub", "children": [
	      { "type": "text", "id": "who", "text": "{{ prop.from }}" },
	      { "type": "text", "id": "body", "text": "{{ prop.text }}" }
	    ] }
	  },
	  "globalState": { "schema": { "feed": "array" }, "initial": { "feed": [
	    { "kind": "Bubble",  "from": "ada",  "body": "shall we ship it" },
	    { "kind": "text",    "from": "",     "body": "ada joined the room" },
	    { "kind": "divider", "from": "",     "body": "" },
	    { "kind": "badge",   "from": "",     "body": "unread" },
	    { "kind": "Bubble",  "from": "grace","body": "ship it" },
	    { "kind": "bubbl",   "from": "",     "body": "typo kind" }
	  ] } } }`)
	// ONE template node: `type` is the binding, and each resolved widget picks
	// the props it knows off the shared prop set.
	write("scenes/main.json", `{ "type": "scene", "id": "main", "root": { "type": "column", "id": "root", "children": [
	  { "type": "list", "id": "feed", "data": "{{ state.feed }}", "renderItem": {
	    "type": "{{ item.kind }}", "id": "msg", "text": "{{ item.body }}", "label": "{{ item.body }}",
	    "from": "{{ item.from }}"
	  } }
	] } }`)

	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("a polymorphic feed must load with zero diagnostics, got: %s", d)
	}
	// The binding survives loading verbatim — it is resolved per item, later.
	if tpl := app.Scenes["main"].Children[0].Template; tpl == nil || tpl.Type != "{{ item.kind }}" {
		t.Fatalf("template type must stay bound after LoadDir: %+v", tpl)
	}

	res := render.Render(qrt.New(app))
	for _, want := range []string{
		// component kind: instantiated with the instance's props, twice
		">ada<", ">shall we ship it<", ">grace<", ">ship it<",
		// built-in text kind
		">ada joined the room<",
		// built-in badge kind (a <span> pill, not a <div>)
		">unread</span>",
		// the typo'd kind degrades, tagged with the RESOLVED name
		`data-qorm-unknown="bubbl"`,
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("polymorphic feed output missing %q:\n%s", want, res.HTML)
		}
	}
	// divider renders its 1px rule, and nothing leaks an unevaluated binding.
	if !strings.Contains(res.HTML, "background:var(--sep);") {
		t.Errorf("the divider kind did not reach the divider renderer:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "{{") {
		t.Errorf("an unresolved binding leaked into the output:\n%s", res.HTML)
	}
	// Self-verify surface: exactly the one bad kind is reported, so a real typo
	// in the DATA is as visible as a typo in the scene JSON.
	if len(res.Unknown) != 1 || res.Unknown[0] != "bubbl" {
		t.Errorf("Result.Unknown should report only the typo'd kind, got %v", res.Unknown)
	}
	// Two Bubble instances, each rendered as its own card (the shared template
	// node is never mutated by a previous item's resolution).
	if got := strings.Count(res.HTML, ">shall we ship it<"); got != 1 {
		t.Errorf("want one bubble per item, got %d:\n%s", got, res.HTML)
	}
}
