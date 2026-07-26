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

// TestListScopeGroupedEndToEnd drives the list template scope through the full
// pipeline (files on disk -> LoadDir static checks -> render): a grouped list
// whose outer section list (as: "section") and inner row list are referenced
// simultaneously, ordinal numbering from {{index}}, zebra striping from
// {{index % 2}}, and first/last flags. The app must load with ZERO
// diagnostics — the loader's missing-prefix suggestion must not fire on the
// scope's bare names — and every binding must resolve in the output.
func TestListScopeGroupedEndToEnd(t *testing.T) {
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
	write("qorm.json", `{ "type": "app", "id": "scopes", "name": "Scopes", "entry": "main",
	  "globalState": { "schema": { "sections": "array" }, "initial": { "sections": [
	    { "title": "Fruit",   "rows": ["apple", "banana"] },
	    { "title": "Veggies", "rows": ["carrot"] }
	  ] } } }`)
	write("scenes/main.json", `{ "type": "scene", "id": "main", "root": { "type": "column", "id": "root", "children": [
	  { "type": "list", "id": "sections", "data": "{{ state.sections }}", "as": "section", "renderItem": {
	    "type": "column", "id": "sec", "children": [
	      { "type": "text", "id": "hdr", "text": "{{ sectionIndex + 1 }}. {{ section.title }}" },
	      { "type": "list", "id": "rows", "data": "{{ section.rows }}", "renderItem": {
	        "type": "column", "id": "line", "children": [
	          { "type": "text", "id": "cell", "text": "{{ index % 2 == 0 ? 'even' : 'odd' }}|{{ section.title }}/{{ item }}" },
	          { "type": "text", "id": "sep", "text": "----", "if": "{{ !last }}" }
	        ] } },
	      { "type": "text", "id": "foot", "text": "end of {{ section.title }}", "if": "{{ sectionLast }}" }
	    ] } }
	] } }`)

	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("scope-using app must load with zero diagnostics, got: %s", d)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, want := range []string{
		// ordinal header from the aliased outer index
		">1. Fruit<", ">2. Veggies<",
		// inner rows see the outer alias AND their own item/index (zebra)
		">even|Fruit/apple<", ">odd|Fruit/banana<", ">even|Veggies/carrot<",
		// last-flag: footer renders only for the last section
		">end of Veggies<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("grouped-list scope output missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "end of Fruit") {
		t.Errorf("sectionLast must be false on the first section:\n%s", html)
	}
	// The if:{{!last}} separator renders between the two Fruit rows only.
	if got := strings.Count(html, ">----<"); got != 1 {
		t.Errorf("want exactly 1 inner separator (2 Fruit rows, 1 Veggie row), got %d:\n%s", got, html)
	}
}
