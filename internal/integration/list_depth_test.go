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

// TestListDepthEndToEnd drives the list's built-in depth features through the
// full pipeline (files on disk -> LoadDir static checks -> render): pagination
// against a bound page, a separator between rows, sticky section headers from
// groupBy, and pull-to-refresh — all on one list, together with the item scope
// (as / index / first / last) they must coexist with. The app must load with
// ZERO diagnostics: the new props are ordinary node keys, so no unknown-style
// or missing-prefix warning may fire on them.
func TestListDepthEndToEnd(t *testing.T) {
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
	write("qorm.json", `{ "type": "app", "id": "depth", "name": "Depth", "entry": "main",
	  "globalState": { "schema": { "rows": "array", "page": "number" }, "initial": {
	    "page": 2,
	    "rows": [
	      { "name": "ann", "dept": "eng",   "label": "Engineering" },
	      { "name": "bob", "dept": "eng",   "label": "Engineering" },
	      { "name": "cid", "dept": "sales", "label": "Sales" },
	      { "name": "dee", "dept": "sales", "label": "Sales" },
	      { "name": "eve", "dept": "ops",   "label": "Operations" }
	    ] } } }`)
	write("actions/reload.json", `{ "type": "action", "id": "reload", "steps": [
	  { "type": "state.set", "path": "page", "value": "1" } ] }`)
	write("scenes/main.json", `{ "type": "scene", "id": "main", "root": { "type": "column", "id": "root", "children": [
	  { "type": "list", "id": "people", "data": "{{ state.rows }}", "as": "row",
	    "pageSize": 3, "page": "{{ state.page }}",
	    "groupBy": "dept", "sectionHeader": "{{ row.label }}", "stickyTop": 44,
	    "separator": { "inset": 16 },
	    "onRefresh": { "type": "invoke", "name": "reload", "args": {} },
	    "renderItem": { "type": "column", "id": "line", "children": [
	      { "type": "text", "id": "cell", "text": "{{ rowIndex + 1 }}. {{ row.name }}" },
	      { "type": "text", "id": "end", "text": "the end", "if": "{{ rowLast }}" }
	    ] } }
	] } }`)

	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("a list using the depth props must load with zero diagnostics, got: %s", d)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML

	// page 2 of size 3 over 5 rows = the last two rows, numbered globally.
	for _, want := range []string{
		">4. dee<", ">5. eve<", // global 1-based numbering from {{ rowIndex + 1 }}
		">the end<",                         // rowLast is the last row of the DATA
		"position:sticky;top:44px;",         // header parked under the appbar height
		">Sales</div>", ">Operations</div>", // sectionHeader read from the section's first row
		"qorm-refresh-spin",                             // pull-to-refresh affordance
		`qormRefresh(document.getElementById("people")`, // ...wired to the gesture
		"overflow-y:auto;overscroll-behavior:contain;",  // ...on the list's own scroll port
	} {
		if !strings.Contains(html, want) {
			t.Errorf("paginated grouped list output missing %q:\n%s", want, html)
		}
	}
	for _, bad := range []string{">1. ann<", ">2. bob<", ">3. cid<", ">Engineering</div>"} {
		if strings.Contains(html, bad) {
			t.Errorf("page 2 must not render %q:\n%s", bad, html)
		}
	}
	// Two sections on this page (sales, ops) → two headers, and exactly one
	// separator: after "dee" comes a header, after "eve" comes nothing.
	if got := strings.Count(html, `class="qorm-list-section"`); got != 2 {
		t.Errorf("want 2 section headers on page 2, got %d:\n%s", got, html)
	}
	if got := strings.Count(html, "margin-left:16px;"); got != 0 {
		t.Errorf("no separator may sit before a header or after the last row, got %d:\n%s", got, html)
	}

	// Page 1 shows the first three rows, one full section plus the start of the
	// next, with a separator between the two rows inside the eng section.
	rt.State["page"] = 1.0
	html = render.Render(rt).HTML
	for _, want := range []string{">1. ann<", ">2. bob<", ">3. cid<", ">Engineering</div>"} {
		if !strings.Contains(html, want) {
			t.Errorf("page 1 output missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, ">the end<") {
		t.Errorf("rowLast must not fire on a page that does not hold the data's last row:\n%s", html)
	}
	if got := strings.Count(html, "margin-left:16px;"); got != 1 {
		t.Errorf("want exactly 1 separator (ann|bob) on page 1, got %d:\n%s", got, html)
	}
}
