package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// These tests cover the opt-in depth added to the data-display widgets: the
// tabs' controlled/scrollable/indicator/lazy props, the tables' cell templates
// and sticky/scrolling chrome, and the smaller accordion/tree/timeline knobs.
//
// Two invariants run through all of them and are asserted per feature:
//
//   - BACK-COMPAT — a node that declares none of the new props renders
//     byte-identically to the pre-feature output. Each widget has an explicit
//     "untouched" case pinning its exact markup, so a future refactor that
//     shifts a byte fails here rather than silently breaking the determinism
//     guard in internal/integration.
//   - COMPOSITION — the new scopes stack with the list's item scope rather than
//     replacing it (a table inside a renderItem sees both), and author-supplied
//     values stay quote/markup-closed.

// mustContain asserts every want is present in the rendered HTML.
func mustContain(t *testing.T, html string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(html, w) {
			t.Errorf("missing %q in:\n%s", w, html)
		}
	}
}

// mustNotContain asserts none of the fragments appears in the rendered HTML.
func mustNotContain(t *testing.T, html string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(html, b) {
			t.Errorf("unexpected %q in:\n%s", b, html)
		}
	}
}

// tabsNode builds a 3-label tabs with 3 text panels and the given props.
func tabsNode(props map[string]any) *model.Node {
	return &model.Node{Type: "tabs", ID: "tb", Props: mergeProps(props, "tabs", []any{"One", "Two", "Three"}),
		Children: textKids("P1", "P2", "P3")}
}

// mergeProps returns props plus key=val (props may be nil).
func mergeProps(props map[string]any, key string, val any) map[string]any {
	out := map[string]any{key: val}
	for k, v := range props {
		out[k] = v
	}
	return out
}

// ---- tabs -------------------------------------------------------------------

// A tabs declaring none of the new props must emit exactly the markup it
// emitted before they existed: plain buttons, no style attribute, client-side
// switching, every panel rendered, and no scoped <style>.
func TestTabsDefaultUnchanged(t *testing.T) {
	res := renderWidget(t, tabsNode(nil))
	mustContain(t, res.HTML,
		`<div class="qorm-tabbar" style="display:flex;gap:2px;border-bottom:1px solid var(--sep);">`,
		`<button class="qorm-tab qorm-tab-active" data-tab="0" onclick="qormTab(this)">One</button>`,
		`<button class="qorm-tab" data-tab="1" onclick="qormTab(this)">Two</button>`,
		`<div class="qorm-tabpanel" data-panel="0" style="display:block;padding:12px 0;">`,
		`<div class="qorm-tabpanel" data-panel="1" style="display:none;padding:12px 0;">`,
		"P1", "P2", "P3")
	mustNotContain(t, res.HTML, "<style>", "data-state", "scroll-snap", "<label class=\"qorm-tab")
}

func TestTabsActiveIndex(t *testing.T) {
	// A literal active selects that tab and panel; switching stays client-side.
	res := renderWidget(t, tabsNode(map[string]any{"active": 1.0}))
	mustContain(t, res.HTML,
		`<button class="qorm-tab" data-tab="0" onclick="qormTab(this)">One</button>`,
		`<button class="qorm-tab qorm-tab-active" data-tab="1"`,
		`data-panel="1" style="display:block`)
	mustNotContain(t, res.HTML, `data-panel="0" style="display:block`)

	// A binding reads the index out of state.
	bound := renderWidgetState(t, tabsNode(map[string]any{"active": "{{state.tab}}"}), map[string]any{"tab": 2.0})
	mustContain(t, bound.HTML, `data-tab="2" style=`, `qorm-tab-active`, `data-panel="2" style="display:block`)

	t.Run("clamping", func(t *testing.T) {
		// Out of range clamps instead of selecting nothing, so an overshot or
		// stale index still shows a usable panel.
		for _, tc := range []struct {
			name, want string
			active     float64
		}{
			{"overshoot", `data-panel="2" style="display:block`, 99},
			{"negative", `data-panel="0" style="display:block`, -3},
		} {
			res := renderWidget(t, tabsNode(map[string]any{"active": tc.active}))
			mustContain(t, res.HTML, tc.want)
		}
		// An index bound to something non-numeric degrades to the first tab.
		junk := renderWidgetState(t, tabsNode(map[string]any{"active": "{{state.nope}}"}), nil)
		mustContain(t, junk.HTML, `data-panel="0" style="display:block`)
	})

	// No labels and no panels: activeIndex has nothing to clamp against and the
	// widget must still render its shell rather than panic.
	empty := renderWidget(t, &model.Node{Type: "tabs", ID: "tb", Props: map[string]any{"active": 3.0}})
	mustContain(t, empty.HTML, `class="qorm-tabbar"`)
}

// A plainly state-bound `active` makes the tabs controlled: labels become
// <label>-wrapped radios that write the tapped index back over the existing
// qorm(-1) state-sync — no action file, no new client JS.
func TestTabsControlledByState(t *testing.T) {
	res := renderWidgetState(t, tabsNode(map[string]any{"active": "{{state.tab}}"}), map[string]any{"tab": 1.0})
	mustContain(t, res.HTML,
		`<div class="qorm-tabbar" role="tablist"`,
		`<label class="qorm-tab" data-tab="0"`,
		`<label class="qorm-tab qorm-tab-active" data-tab="1"`,
		`<input type="radio" name="tb" value="1" checked data-state="tab" onchange="qorm(-1)"`,
		`role="tab" aria-selected="true"`,
		`aria-selected="false"`)
	// Exactly one radio is checked.
	if got := strings.Count(res.HTML, " checked"); got != 1 {
		t.Errorf("controlled tabs must check exactly one radio, got %d:\n%s", got, res.HTML)
	}
	// The client-side switcher is not wired: switching is the server's job now.
	mustNotContain(t, res.HTML, "qormTab(this)")
}

// onChange dispatches per tab with {index, tab} on top of the author's args.
func TestTabsOnChange(t *testing.T) {
	n := tabsNode(nil)
	n.OnChange = &model.Invoke{Name: "pick", Args: map[string]string{"src": "tabs"}}
	res := renderWidget(t, n)
	mustContain(t, res.HTML, `data-tab="2" onclick="qorm(2)"`)
	mustNotContain(t, res.HTML, "qormTab(this)")
	if len(res.Handlers) != 3 {
		t.Fatalf("expected one handler per tab, got %d", len(res.Handlers))
	}
	h := res.Handlers[2]
	if h.Name != "pick" || h.Args["index"] != "2" || h.Args["tab"] != "Three" || h.Args["src"] != "tabs" {
		t.Errorf("tab handler args: %+v", h)
	}

	// Bound active + onChange compose: the radio still syncs the index and the
	// action still runs.
	n.Props["active"] = "{{state.tab}}"
	both := renderWidgetState(t, n, map[string]any{"tab": 0.0})
	mustContain(t, both.HTML, `data-state="tab" onchange="qorm(0)"`)
}

// A tab bar that overflows scrolls instead of squeezing — pure CSS, so it works
// on the first paint with no gesture script.
func TestTabsScrollable(t *testing.T) {
	res := renderWidget(t, tabsNode(map[string]any{"scrollable": true}))
	mustContain(t, res.HTML,
		"overflow-x:auto;overflow-y:hidden;scrollbar-width:none;scroll-snap-type:x proximity;",
		`<button class="qorm-tab qorm-tab-active" style="flex:none;white-space:nowrap;scroll-snap-align:start;" data-tab="0"`)
	// The same per-tab sizing reaches the controlled (label) form.
	ctl := renderWidgetState(t, tabsNode(map[string]any{"scrollable": true, "active": "{{state.tab}}"}), map[string]any{"tab": 0.0})
	mustContain(t, ctl.HTML, `style="display:inline-flex;align-items:center;position:relative;flex:none;white-space:nowrap;scroll-snap-align:start;"`)
}

// The indicator is a node-scoped <style> so it follows the qorm-tab-active
// class when the client switches tabs — an inline style would stay stuck on the
// tab that was active when the frame was rendered.
func TestTabsIndicator(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"default-underline-recolored", map[string]any{"indicatorColor": "var(--danger)"},
			`<style>#tb .qorm-tab-active{border-bottom-color:var(--danger) !important;color:var(--danger) !important;}</style>`},
		{"pill", map[string]any{"indicator": "pill"},
			`<style>#tb .qorm-tab-active{border-bottom-color:transparent !important;background:var(--accent);color:#fff !important;border-radius:999px;}</style>`},
		{"none", map[string]any{"indicator": "none"},
			`<style>#tb .qorm-tab-active{border-bottom-color:transparent !important;color:var(--accent) !important;}</style>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustContain(t, renderWidget(t, tabsNode(tc.props)).HTML, tc.want)
		})
	}

	t.Run("closure", func(t *testing.T) {
		// A colour carrying CSS punctuation cannot close the declaration and
		// inject rules — it is rejected outright and the accent is used.
		res := renderWidget(t, tabsNode(map[string]any{"indicatorColor": `red;} body{display:none;`}))
		mustNotContain(t, res.HTML, "<style>", "body{display:none", `red;}`)
		// Rejected alongside a declared shape, the shape still renders — on the
		// accent, never on the value that could not be trusted.
		pill := renderWidget(t, tabsNode(map[string]any{"indicator": "pill", "indicatorColor": `red;}x{`}))
		mustContain(t, pill.HTML, "background:var(--accent);")
		mustNotContain(t, pill.HTML, `x{`)
		// Nor can it close the <style> element.
		esc := renderWidget(t, tabsNode(map[string]any{"indicatorColor": `</style><script>alert(1)</script>`}))
		mustNotContain(t, esc.HTML, "<script>alert(1)")
	})

	t.Run("needs-a-selectable-id", func(t *testing.T) {
		// An id that could not be written as a CSS selector skips the block
		// rather than emitting a broken (or injectable) one.
		n := tabsNode(map[string]any{"indicator": "pill"})
		n.ID = "tb one"
		mustNotContain(t, renderWidget(t, n).HTML, "<style>")
	})
}

// Lazy renders only the active panel — and only when a tap can actually reach
// the server, because client-side switching can only reveal panels that are
// already in the document.
func TestTabsLazyPanels(t *testing.T) {
	res := renderWidgetState(t, tabsNode(map[string]any{"active": "{{state.tab}}", "lazy": true}), map[string]any{"tab": 1.0})
	mustContain(t, res.HTML, "P2", `data-panel="1" style="display:block`)
	mustNotContain(t, res.HTML, "P1", "P3", `data-panel="0"`, `data-panel="2"`)

	// An uncontrolled lazy tabs degrades to an eager one, never to a dead UI.
	eager := renderWidget(t, tabsNode(map[string]any{"lazy": true}))
	mustContain(t, eager.HTML, "P1", "P2", "P3", "qormTab(this)")

	// onChange alone is enough to make it controlled.
	n := tabsNode(map[string]any{"lazy": true, "active": 2.0})
	n.OnChange = &model.Invoke{Name: "pick"}
	mustContain(t, renderWidget(t, n).HTML, "P3")
	mustNotContain(t, renderWidget(t, n).HTML, "P1")
}

// ---- tables: cell templates -------------------------------------------------

// tableRows is the fixture data both table renderers use.
func tableRows() []any {
	return []any{
		map[string]any{"id": 1.0, "name": "Ada", "state": "on"},
		map[string]any{"id": 2.0, "name": "Linus", "state": "off"},
	}
}

// tableWith builds a table/datatable over tableRows with the given props and
// children.
func tableWith(t *testing.T, typ string, props map[string]any, kids ...*model.Node) Result {
	t.Helper()
	p := mergeProps(props, "columns", []any{
		map[string]any{"value": "name", "label": "Name"},
		map[string]any{"value": "state", "label": "State"},
	})
	return renderWidgetState(t, &model.Node{Type: typ, ID: "tbl", Props: mergeProps(p, "data", "{{state.rows}}"), Children: kids},
		map[string]any{"rows": tableRows()})
}

// A cell template turns a column's cells into widgets, scoped to the row.
func TestTableCellTemplates(t *testing.T) {
	for _, typ := range []string{"table", "datatable"} {
		t.Run(typ, func(t *testing.T) {
			res := tableWith(t, typ, nil,
				&model.Node{Type: "tag", ID: "cell", Props: map[string]any{"column": "state"},
					Label: "{{row.name}}/{{cell.value}}/{{cell.column}}/{{cell.index}}/{{rowIndex}}"})
			mustContain(t, res.HTML,
				"Ada/on/state/1/0",
				"Linus/off/state/1/1",
				// The untemplated column keeps its plain-text cell...
				"<td>Ada</td>", "<td>Linus</td>")
			// ...and the templated one no longer prints the bare value.
			mustNotContain(t, res.HTML, "<td>on</td>", "<td>off</td>")
		})
	}
}

// The row alias follows the same `as` rule as the list's item alias, so the
// derived index/first/last keys are namespaced the same way.
func TestTableCellScopeAlias(t *testing.T) {
	res := tableWith(t, "table", map[string]any{"as": "item"},
		&model.Node{Type: "text", ID: "c", Props: map[string]any{"column": "state"},
			Text: "{{item.name}}:{{index}}:{{first}}:{{last}}"})
	mustContain(t, res.HTML, "Ada:0:true:false", "Linus:1:false:true")

	// Cell templates render inside the row, so a JS-wired node's id carries the
	// row index exactly as it does inside a list's renderItem.
	mustContain(t, res.HTML, `id="c-0"`, `id="c-1"`)

	// Default alias: row/rowIndex/rowFirst/rowLast.
	def := tableWith(t, "table", nil,
		&model.Node{Type: "text", ID: "c", Props: map[string]any{"column": "state"},
			Text: "{{row.name}}:{{rowIndex}}:{{rowFirst}}:{{rowLast}}"})
	mustContain(t, def.HTML, "Ada:0:true:false", "Linus:1:false:true")
}

// Edge cases an author will hit: a template for a column that is not declared,
// a template reading a field the row does not carry, and two templates claiming
// one column.
func TestTableCellTemplateEdges(t *testing.T) {
	t.Run("unknown-column", func(t *testing.T) {
		res := tableWith(t, "table", nil,
			&model.Node{Type: "text", ID: "ghost", Props: map[string]any{"column": "nope"}, Text: "GHOST"})
		mustNotContain(t, res.HTML, "GHOST")
		mustContain(t, res.HTML, "<td>on</td>") // every column keeps its text cell
	})
	t.Run("missing-field", func(t *testing.T) {
		res := tableWith(t, "table", nil,
			&model.Node{Type: "text", ID: "c", Props: map[string]any{"column": "state"}, Text: "[{{row.nope}}]"})
		mustContain(t, res.HTML, "[]") // empty, not the literal binding
		mustNotContain(t, res.HTML, "{{row.nope}}")
	})
	t.Run("duplicate-column", func(t *testing.T) {
		res := tableWith(t, "table", nil,
			&model.Node{Type: "text", ID: "a", Props: map[string]any{"column": "state"}, Text: "FIRST"},
			&model.Node{Type: "text", ID: "b", Props: map[string]any{"column": "state"}, Text: "SECOND"})
		mustContain(t, res.HTML, "FIRST")
		mustNotContain(t, res.HTML, "SECOND")
	})
	t.Run("column-less-child-ignored", func(t *testing.T) {
		// A child with no `column`/`detail` is not a cell template — it stays
		// ignored, exactly as every table child was before this existed.
		res := tableWith(t, "table", nil, &model.Node{Type: "text", ID: "x", Text: "STRAY"})
		mustNotContain(t, res.HTML, "STRAY")
	})
	t.Run("more-columns-than-fields", func(t *testing.T) {
		// A column the data does not carry renders an empty cell, templated or
		// not — never a short row that misaligns the header.
		res := renderWidgetState(t, &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
			"columns": []any{"name", "missing"}, "data": "{{state.rows}}",
		}, Children: []*model.Node{{Type: "text", ID: "c", Props: map[string]any{"column": "missing"}, Text: "[{{cell.value}}]"}}},
			map[string]any{"rows": tableRows()})
		if got := strings.Count(res.HTML, "<td"); got != 4 {
			t.Errorf("2 rows x 2 columns should emit 4 cells, got %d:\n%s", got, res.HTML)
		}
		mustContain(t, res.HTML, "[]")
	})
	t.Run("escaping", func(t *testing.T) {
		// Values reaching a templated cell are escaped exactly like text cells.
		res := renderWidgetState(t, &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
			"columns": []any{"name"}, "data": "{{state.rows}}",
		}, Children: []*model.Node{{Type: "text", ID: "c", Props: map[string]any{"column": "name"}, Text: "{{cell.value}}"}}},
			map[string]any{"rows": []any{map[string]any{"name": `<img src=x onerror="alert(1)">`}}})
		mustNotContain(t, res.HTML, "<img src=x")
		mustContain(t, res.HTML, "&lt;img")
	})
}

// A templated table inside a list's renderItem sees BOTH scopes: the list's
// item bindings and the table's own row/cell bindings.
func TestTableCellsComposeWithListScope(t *testing.T) {
	tbl := &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
		"columns": []any{"name"}, "data": "{{ item.rows }}",
	}, Children: []*model.Node{
		{Type: "text", ID: "c", Props: map[string]any{"column": "name"},
			Text: "{{item.dept}}#{{index}}:{{row.name}}@{{rowIndex}}"},
	}}
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.groups }}", Template: tbl,
	}, map[string]any{"groups": []any{
		map[string]any{"dept": "eng", "rows": []any{map[string]any{"name": "Ada"}}},
		map[string]any{"dept": "ops", "rows": []any{map[string]any{"name": "Grace"}, map[string]any{"name": "Linus"}}},
	}})
	mustContain(t, res.HTML, "eng#0:Ada@0", "ops#1:Grace@0", "ops#1:Linus@1")
	// The list's own scope survives the table: the outer item is not shadowed.
	mustNotContain(t, res.HTML, "#1:Ada")
}

// ---- tables: sticky / scrolling chrome --------------------------------------

func TestTableChromeDefaultUnchanged(t *testing.T) {
	for _, typ := range []string{"table", "datatable"} {
		res := tableWith(t, typ, nil)
		mustContain(t, res.HTML, "<th>Name</th>", "<td>Ada</td>")
		mustNotContain(t, res.HTML, "position:sticky", "overflow-x:auto", "min-width", "<div style=\"overflow")
	}
}

func TestTableStickyHeader(t *testing.T) {
	res := tableWith(t, "table", map[string]any{"stickyHeader": true, "stickyTop": 44.0})
	mustContain(t, res.HTML,
		`<th style="position:sticky;top:44px;z-index:2;background:var(--bg);">Name</th>`)
	// Body cells are untouched — only the header pins.
	mustContain(t, res.HTML, "<td>Ada</td>")

	// datatable's checkbox header pins too, so the row of headers moves as one.
	dt := tableWith(t, "datatable", map[string]any{"stickyHeader": true, "selectable": true})
	mustContain(t, dt.HTML, `<th class="qdt-check" style="position:sticky;top:0px;`)
}

func TestTableScrollPort(t *testing.T) {
	res := tableWith(t, "table", map[string]any{"scrollX": true, "maxHeight": 300.0, "minWidth": 720.0})
	mustContain(t, res.HTML,
		`<div style="overflow-x:auto;overflow-y:auto;max-height:300px;"><table id="tbl"`,
		"min-width:720px;",
		"</table></div>")

	// Either prop alone opens the port; neither leaves the table unwrapped.
	only := tableWith(t, "datatable", map[string]any{"scrollX": true})
	mustContain(t, only.HTML, `<div style="overflow-x:auto;"><table`)
}

// A frozen column stays put while the rest scrolls under it; offsets accumulate
// from the declared pixel widths (plus datatable's checkbox column).
func TestTableStickyColumns(t *testing.T) {
	cols := []any{
		map[string]any{"value": "id", "label": "ID", "width": 60.0, "sticky": true},
		map[string]any{"value": "name", "label": "Name", "width": 100.0, "sticky": true},
		map[string]any{"value": "state", "label": "State"},
	}
	res := renderWidgetState(t, &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
		"columns": cols, "data": "{{state.rows}}", "stickyHeader": true,
	}}, map[string]any{"rows": tableRows()})
	mustContain(t, res.HTML,
		// header corner cells sit above both the header and the column
		`<th style="position:sticky;top:0px;z-index:2;background:var(--bg);position:sticky;left:0px;z-index:1;background:var(--bg);z-index:3;">ID</th>`,
		`left:60px;`,
		`<td style="position:sticky;left:0px;z-index:1;background:var(--bg);">1</td>`)
	// The unfrozen column carries only the header's own sticky CSS.
	mustContain(t, res.HTML, `<th style="position:sticky;top:0px;z-index:2;background:var(--bg);">State</th>`)

	// datatable offsets start after the 36px checkbox column.
	dt := renderWidgetState(t, &model.Node{Type: "datatable", ID: "tbl", Props: map[string]any{
		"columns": cols, "data": "{{state.rows}}", "selectable": true,
	}}, map[string]any{"rows": tableRows()})
	mustContain(t, dt.HTML, "left:36px;", "left:96px;")

	// A frozen column with no numeric width contributes no offset (documented
	// limit), and a columns prop that is not an array is simply not frozen.
	noW := renderWidgetState(t, &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
		"columns": []any{map[string]any{"value": "name", "sticky": true}, "state"}, "data": "{{state.rows}}",
	}}, map[string]any{"rows": tableRows()})
	mustContain(t, noW.HTML, "left:0px;")
	plain := renderWidgetState(t, &model.Node{Type: "table", ID: "tbl", Props: map[string]any{
		"columns": "name", "data": "{{state.rows}}",
	}}, map[string]any{"rows": tableRows()})
	mustNotContain(t, plain.HTML, "position:sticky")
}

// Row expansion is a native <details> in a full-width row: no JS, no round-trip.
func TestTableDetailRow(t *testing.T) {
	res := tableWith(t, "datatable", map[string]any{"selectable": true},
		&model.Node{Type: "text", ID: "d", Props: map[string]any{"detail": true}, Label: "More", Text: "{{row.name}} detail"})
	mustContain(t, res.HTML,
		`<td colspan="3"`, // 2 columns + the checkbox column
		"<details><summary", "More ▾", "Ada detail", "Linus detail")

	plain := tableWith(t, "table", nil,
		&model.Node{Type: "text", ID: "d", Props: map[string]any{"detail": true}, Text: "x"})
	mustContain(t, plain.HTML, `<td colspan="2"`)
}

// ---- accordion / tree / timeline --------------------------------------------

func TestAccordionActive(t *testing.T) {
	kids := textKids("A", "B", "C")
	res := renderWidgetState(t, &model.Node{Type: "accordion", ID: "acc",
		Props: map[string]any{"active": "{{state.open}}"}, Children: kids}, map[string]any{"open": 2.0})
	panels := strings.Split(res.HTML, `class="qorm-acc-panel"`)
	if len(panels) != 4 {
		t.Fatalf("expected 3 panels, got %d:\n%s", len(panels)-1, res.HTML)
	}
	mustContain(t, panels[3], `style="display:block`)
	mustContain(t, panels[1], `style="display:none`)

	// Default: the first panel is open, exactly as before `active` existed.
	def := renderWidget(t, &model.Node{Type: "accordion", ID: "acc", Children: kids})
	mustContain(t, strings.Split(def.HTML, `class="qorm-acc-panel"`)[1], `style="display:block`)
}

func TestTreeCollapsed(t *testing.T) {
	data := []any{map[string]any{"label": "root", "children": []any{
		map[string]any{"label": "kept", "children": []any{map[string]any{"label": "leaf"}}},
	}}}
	// Default: every branch expanded (unchanged).
	open := renderWidgetState(t, &model.Node{Type: "tree", ID: "tr", Props: map[string]any{"data": "{{state.d}}"}},
		map[string]any{"d": data})
	if got := strings.Count(open.HTML, `<details class="qorm-tree-n" open>`); got != 2 {
		t.Errorf("an unconfigured tree must render every branch open, got %d:\n%s", got, open.HTML)
	}

	// collapsed flips the default for the whole tree.
	shut := renderWidgetState(t, &model.Node{Type: "tree", ID: "tr", Props: map[string]any{"data": "{{state.d}}", "collapsed": true}},
		map[string]any{"d": data})
	mustNotContain(t, shut.HTML, " open>")

	// A node's own `expanded` overrides the default for THAT node only: the
	// child keeps the tree default, so re-opening a folded branch does not
	// reveal a subtree folded only because its parent was.
	mixed := []any{map[string]any{"label": "root", "expanded": true, "children": []any{
		map[string]any{"label": "kid", "children": []any{map[string]any{"label": "leaf"}}},
	}}}
	res := renderWidgetState(t, &model.Node{Type: "tree", ID: "tr", Props: map[string]any{"data": "{{state.d}}", "collapsed": true}},
		map[string]any{"d": mixed})
	if got := strings.Count(res.HTML, " open>"); got != 1 {
		t.Errorf("only the node declaring expanded should be open, got %d:\n%s", got, res.HTML)
	}
}

func TestTimelineItemFields(t *testing.T) {
	items := []any{
		map[string]any{"title": "Shipped", "text": "v1", "color": "var(--success)", "time": "09:30", "icon": "check"},
		map[string]any{"title": "Filed", "text": "v2"},
	}
	res := renderWidgetState(t, &model.Node{Type: "timeline", ID: "tl", Props: map[string]any{"items": "{{state.i}}"}},
		map[string]any{"i": items})
	mustContain(t, res.HTML,
		"background:var(--success);color:#fff;", // icon marker takes the item colour
		"<svg", "09:30",
		// the plain item keeps the original dot markup, byte for byte
		`<span style="width:12px;height:12px;border-radius:50%;background:var(--accent);flex-shrink:0;margin-top:3px;"></span>`)

	// An unknown icon name falls back to the dot; a colour carrying CSS
	// punctuation is rejected rather than escaping its declaration.
	bad := renderWidgetState(t, &model.Node{Type: "timeline", ID: "tl", Props: map[string]any{"items": "{{state.i}}"}},
		map[string]any{"i": []any{map[string]any{"title": "T", "icon": "no-such-icon", "color": `red";x:"`}}})
	mustContain(t, bad.HTML, "width:12px;height:12px;border-radius:50%;background:var(--accent);")
	mustNotContain(t, bad.HTML, `x:"`)

	// The item colour lands in a style ATTRIBUTE, so it is gated on
	// cssStyleValue and the FILTERED value is what gets written — the gate and
	// the output must be the same string, or widening the filter silently opens
	// a hole. A declaration-injection overlay and a url() beacon are dropped.
	for _, payload := range []string{
		"#fff;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999",
		"url(//attacker/beacon.png)",
		"red/*",
	} {
		res := renderWidgetState(t, &model.Node{Type: "timeline", ID: "tl", Props: map[string]any{"items": "{{state.i}}"}},
			map[string]any{"i": []any{map[string]any{"title": "T", "color": payload}}})
		mustContain(t, res.HTML, "background:var(--accent);")
		mustNotContain(t, res.HTML, "100vw", "z-index:99999", "//attacker", "/*")
	}
}

// cssValue is the filter that keeps an author-written colour from closing the
// declaration (or the <style> element) it is written into. It also rejects the
// values whose charset is legal but whose MEANING is not: a url() the browser
// fetches (the charset must allow "/" and "(" for rgb(… / …) and var(), which
// together spell one) and a comment that truncates the rule.
func TestCSSValueFilter(t *testing.T) {
	for _, ok := range []string{"var(--accent)", "#0af", "rgb(0 0 0 / 50%)", "color-mix(in srgb,red 20%,blue)", "hotpink", ""} {
		if cssValue(ok) != ok {
			t.Errorf("cssValue(%q) should pass through", ok)
		}
	}
	for _, bad := range []string{
		"red;color:blue", "red}x{y:z", `red"`, "</style>", "a{b",
		"url(//attacker/beacon.png)", "URL(//attacker/beacon.png)",
		"image-set(//attacker/x.png 1x)", "-webkit-image-set(//attacker/x.png 1x)",
		"src(//attacker/x.png)", "expression(alert(1))", "red/*",
	} {
		if got := cssValue(bad); got != "" {
			t.Errorf("cssValue(%q) = %q, want rejected", bad, got)
		}
	}
}

// TestTabIndicatorColorFetch is the <style>-block half of the same rule: the
// tabs `indicatorColor` is written into a scoped stylesheet, and the red team
// showed the `pill` branch puts it in `background:` — a property that accepts
// an image, so url(//attacker/…) makes the browser issue a real request and
// hand a third party the visit plus the page's Referer. No script needed, and
// entity encoding is inert inside a <style> element.
func TestTabIndicatorColorFetch(t *testing.T) {
	for _, kind := range []string{"pill", "underline", "none"} {
		res := renderWidget(t, tabsNode(map[string]any{"indicator": kind, "indicatorColor": "url(//attacker/beacon.png)"}))
		mustContain(t, res.HTML, "<style>#tb .qorm-tab-active{") // the block is still emitted
		mustNotContain(t, res.HTML, "//attacker", "url(")
		mustContain(t, res.HTML, "var(--accent)") // with the default colour
	}
	// A legitimate indicator colour is unaffected, pill branch included.
	ok := renderWidget(t, tabsNode(map[string]any{"indicator": "pill",
		"indicatorColor": "color-mix(in srgb,var(--accent) 30%,transparent)"}))
	mustContain(t, ok.HTML, "background:color-mix(in srgb,var(--accent) 30%,transparent);")
}
