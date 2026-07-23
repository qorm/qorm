package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func tableColsRows() ([]any, []any) {
	cols := []any{
		map[string]any{"value": "name", "label": "Name", "width": float64(120)},
		map[string]any{"value": "age", "label": "Age", "width": "30%"},
	}
	rows := []any{
		map[string]any{"id": "1", "name": "Alice", "age": "30"},
		map[string]any{"id": "2", "name": "Bob", "age": "40"},
	}
	return cols, rows
}

// TestTableSortable covers the table's sortable-header branches: an app-wired
// onChange, the default built-in __sort handler, and the asc/desc indicator.
func TestTableSortable(t *testing.T) {
	cols, rows := tableColsRows()

	t.Run("onchange-sort-desc-indicator", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "table", ID: "tbl", Props: map[string]any{"columns": cols, "data": rows,
				"sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}, OnChange: &model.Invoke{Name: "sort"}},
			map[string]any{"sf": "name", "sd": "desc"})
		if !strings.Contains(res.HTML, "qdt-sort") || !strings.Contains(res.HTML, "qorm-sort-ind on") {
			t.Errorf("sortable header should render a sort button on the sorted column:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "▾") {
			t.Errorf("desc sort should show the ▾ indicator:\n%s", res.HTML)
		}
		// each column header dispatches onChange with its column name
		gotCols := map[string]bool{}
		for _, h := range res.Handlers {
			if h.Name == "sort" {
				gotCols[h.Args["column"]] = true
			}
		}
		if !gotCols["name"] || !gotCols["age"] {
			t.Errorf("table headers should dispatch onChange per column, got %v", gotCols)
		}
	})

	t.Run("onchange-sort-asc-indicator", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "table", ID: "tbl2", Props: map[string]any{"columns": cols, "data": rows,
				"sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}, OnChange: &model.Invoke{Name: "sort"}},
			map[string]any{"sf": "age", "sd": "asc"})
		if !strings.Contains(res.HTML, "▴") {
			t.Errorf("asc sort should show the ▴ indicator:\n%s", res.HTML)
		}
	})

	t.Run("default-builtin-sort-handler", func(t *testing.T) {
		// No onChange: with bound data/sortField/sortDir the header falls back to
		// the runtime's __sort built-in.
		res := renderWidgetState(t,
			&model.Node{Type: "table", ID: "tbl3", Props: map[string]any{"columns": cols,
				"data": "{{ state.rows }}", "sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}},
			map[string]any{"rows": rows, "sf": "name", "sd": "desc"})
		var sortH *Handler
		for i := range res.Handlers {
			if res.Handlers[i].Name == runtime.BuiltinSort {
				sortH = &res.Handlers[i]
			}
		}
		if sortH == nil {
			t.Fatalf("table should register the __sort built-in when data/sortField/sortDir are bound: %+v", res.Handlers)
		}
		if sortH.Args["data"] != "rows" || sortH.Args["field"] != "sf" || sortH.Args["dir"] != "sd" || sortH.Args["column"] == "" {
			t.Errorf("__sort handler args wrong: %+v", sortH.Args)
		}
	})

	t.Run("explicit-sortdata-path-wins", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "table", ID: "tbl4", Props: map[string]any{"columns": cols,
				"data": "{{ state.rows }}", "sortData": "{{ state.allRows }}", "sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}},
			map[string]any{"rows": rows, "allRows": rows, "sf": "name", "sd": "asc"})
		found := false
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinSort && h.Args["data"] == "allRows" {
				found = true
			}
		}
		if !found {
			t.Errorf("explicit sortData should be the __sort data path: %+v", res.Handlers)
		}
	})

	t.Run("no-sort-when-unbound", func(t *testing.T) {
		// Without bound sortField/sortDir there is no default sort handler and the
		// headers are plain.
		res := renderWidget(t, &model.Node{Type: "table", ID: "tbl5", Props: map[string]any{"columns": cols, "data": rows}})
		if strings.Contains(res.HTML, "qdt-sort") {
			t.Errorf("unbound table should have plain headers:\n%s", res.HTML)
		}
		if len(res.Handlers) != 0 {
			t.Errorf("unbound table should register no handlers: %+v", res.Handlers)
		}
	})
}

// TestTableColumnWidths covers the <colgroup> sizing (colWidths/colWidth/colGroup)
// driven through the table widget.
func TestTableColumnWidths(t *testing.T) {
	cols, rows := tableColsRows()
	res := renderWidget(t, &model.Node{Type: "table", ID: "tw", Props: map[string]any{"columns": cols, "data": rows}})
	if !strings.Contains(res.HTML, "<colgroup>") {
		t.Errorf("table with column widths should emit a colgroup:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `<col style="width:120px">`) || !strings.Contains(res.HTML, `<col style="width:30%">`) {
		t.Errorf("colgroup should size both columns (px + css):\n%s", res.HTML)
	}

	// A table whose columns carry no widths emits no colgroup.
	res = renderWidget(t, &model.Node{Type: "table", ID: "tw2", Props: map[string]any{
		"columns": []any{map[string]any{"value": "n", "label": "N"}}, "data": rows}})
	if strings.Contains(res.HTML, "<colgroup>") {
		t.Errorf("width-less columns should not emit a colgroup:\n%s", res.HTML)
	}
}

// TestColWidthHelpers are direct unit tests of the column-width normalizers,
// including the malformed/odd inputs an untrusted tree could carry.
func TestColWidthHelpers(t *testing.T) {
	cw := []struct {
		in   any
		want string
	}{
		{float64(100), "100px"},
		{"30%", "30%"},
		{"50", "50px"}, // bare numeric string means px
		// A numeric string with surrounding whitespace is detected via the
		// trimmed value and the TRIMMED value + "px" is returned, so the CSS is
		// valid ("50px") — the former " 50 px" was malformed and browser-dropped
		// (regression guard for the round-5 colWidth fix).
		{" 50 ", "50px"},
		{"abc", "abc"}, // arbitrary css passes through
		{nil, ""},
		{42, ""}, // unsupported type -> ""
	}
	for _, tc := range cw {
		if got := colWidth(tc.in); got != tc.want {
			t.Errorf("colWidth(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}

	got := colWidths([]any{"name", map[string]any{"width": float64(100)}, map[string]any{"width": "30%"}, map[string]any{}})
	want := []string{"", "100px", "30%", ""}
	if len(got) != len(want) {
		t.Fatalf("colWidths len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("colWidths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if colWidths("nope") != nil {
		t.Errorf("colWidths(non-array) should be nil")
	}

	if colGroup([]string{"", ""}, false) != "" {
		t.Errorf("colGroup with no widths should be empty")
	}
	if got := colGroup([]string{"", "100px"}, false); got != `<colgroup><col><col style="width:100px"></colgroup>` {
		t.Errorf("colGroup = %q", got)
	}
	if got := colGroup([]string{"100px"}, true); got != `<colgroup><col><col style="width:100px"></colgroup>` {
		t.Errorf("colGroup(extraLeading) = %q", got)
	}
	// a width value cannot break out of the attribute
	if got := colGroup([]string{`"><script>`}, false); !strings.Contains(got, "width:&#34;&gt;&lt;script&gt;") {
		t.Errorf("colGroup must escape the width value, got %q", got)
	}
}

// TestDatatable covers the richer datatable: select-all + per-row selection,
// sortable headers (onChange and the __sort fallback) and the sort indicator.
func TestDatatable(t *testing.T) {
	cols, rows := tableColsRows()

	t.Run("select-all-and-rows", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "datatable", ID: "dt", Props: map[string]any{"columns": cols, "data": rows, "selected": "{{ state.sel }}"},
				OnPress: &model.Invoke{Name: "toggle"}},
			map[string]any{"sel": []any{"1"}})
		// header select-all + one handler per row
		allKeys := map[string]int{}
		for _, h := range res.Handlers {
			if h.Name == "toggle" {
				allKeys[h.Args["key"]]++
			}
		}
		if allKeys["__all__"] != 1 || allKeys["1"] != 1 || allKeys["2"] != 1 {
			t.Errorf("datatable should dispatch __all__ + per-row keys, got %v", allKeys)
		}
		// row 1 is selected -> qdt-sel row class + a checked cell glyph
		if !strings.Contains(res.HTML, `class="qdt-sel"`) {
			t.Errorf("selected row should carry qdt-sel:\n%s", res.HTML)
		}
	})

	t.Run("all-selected-header-checked", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "datatable", ID: "dt2", Props: map[string]any{"columns": cols, "data": rows, "selected": "{{ state.sel }}"},
				OnPress: &model.Invoke{Name: "toggle"}},
			map[string]any{"sel": []any{"1", "2"}})
		// when every row is selected the header box is the accent-filled check
		if !strings.Contains(res.HTML, checkboxCell(true)) {
			t.Errorf("all-selected datatable header should show a checked cell:\n%s", res.HTML)
		}
	})

	t.Run("sortable-onchange", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "datatable", ID: "dt3", Props: map[string]any{"columns": cols, "data": rows,
				"sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}, OnChange: &model.Invoke{Name: "sort"}},
			map[string]any{"sf": "name", "sd": "desc"})
		if !strings.Contains(res.HTML, "qdt-sort") || !strings.Contains(res.HTML, "▾") {
			t.Errorf("sortable datatable header should show the desc sort button:\n%s", res.HTML)
		}
	})

	t.Run("sortable-default-builtin", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "datatable", ID: "dt4", Props: map[string]any{"columns": cols,
				"data": "{{ state.rows }}", "sortField": "{{ state.sf }}", "sortDir": "{{ state.sd }}"}},
			map[string]any{"rows": rows, "sf": "age", "sd": "asc"})
		found := false
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinSort && h.Args["column"] != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("datatable should fall back to __sort headers: %+v", res.Handlers)
		}
		if !strings.Contains(res.HTML, "▴") {
			t.Errorf("datatable asc sort should show ▴:\n%s", res.HTML)
		}
	})

	t.Run("checkbox-column-without-onpress", func(t *testing.T) {
		// selectable:true but no OnPress: the checkbox column renders inert cells.
		res := renderWidget(t, &model.Node{Type: "datatable", ID: "dt5", Props: map[string]any{"columns": cols, "data": rows, "selectable": true}})
		if !strings.Contains(res.HTML, "qdt-check") {
			t.Errorf("selectable datatable should render the checkbox column:\n%s", res.HTML)
		}
		if len(res.Handlers) != 0 {
			t.Errorf("selectable without handlers should register none: %+v", res.Handlers)
		}
	})
}

// TestStatEmptyDescriptionsTimeline covers the remaining branches of the
// display widgets: stat delta direction + value fallback, empty's icon fallback,
// descriptions skipping odd elements, timeline/tree accepting bare strings.
func TestStatEmptyDescriptionsTimeline(t *testing.T) {
	t.Run("stat-delta-down", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "stat", ID: "s", Props: map[string]any{"value": "5", "delta": "-2", "deltaType": "down"}})
		if !strings.Contains(res.HTML, "#dc2626") {
			t.Errorf("down delta should be red:\n%s", res.HTML)
		}
	})
	t.Run("stat-value-falls-back-to-text", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "stat", ID: "s2", Text: "42", Props: map[string]any{"label": "Answer"}})
		if !strings.Contains(res.HTML, ">42</div>") {
			t.Errorf("stat without value should use its text:\n%s", res.HTML)
		}
	})

	t.Run("empty-unknown-icon-fallback", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "empty", ID: "em", Props: map[string]any{"icon": "notarealicon", "title": "None"}})
		if !strings.Contains(res.HTML, "font-size:40px") || !strings.Contains(res.HTML, "notarealicon") {
			t.Errorf("empty should fall back to text for an unknown icon:\n%s", res.HTML)
		}
	})
	t.Run("empty-with-children", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "empty", ID: "em2", Props: map[string]any{"title": "None"}, Children: []*model.Node{
			{Type: "button", ID: "retry", Label: "Retry"},
		}})
		if !strings.Contains(res.HTML, "Retry") {
			t.Errorf("empty should render its action children:\n%s", res.HTML)
		}
	})

	t.Run("descriptions-skips-odd-elements", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "descriptions", ID: "ds", Props: map[string]any{"items": []any{
			map[string]any{"label": "Name", "value": "Al"},
			"not-a-map",
		}}})
		if !strings.Contains(res.HTML, "Al") {
			t.Errorf("descriptions should render the valid item:\n%s", res.HTML)
		}
	})

	t.Run("timeline-bare-string-item", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "timeline", ID: "tl", Props: map[string]any{"items": []any{"JustAString"}}})
		if !strings.Contains(res.HTML, "JustAString") {
			t.Errorf("timeline should accept a bare string item:\n%s", res.HTML)
		}
	})

	t.Run("tree-bare-string-leaf", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "tree", ID: "tr", Props: map[string]any{"data": []any{"leaf"}}})
		if !strings.Contains(res.HTML, "qorm-tree-leaf") || !strings.Contains(res.HTML, "leaf") {
			t.Errorf("tree should render a bare string as a leaf:\n%s", res.HTML)
		}
	})
}

// TestListTileTrailingAndChevron covers the listTile trailing slot and the
// chevron:false opt-out, plus expansionTile's leading icon and the template-less
// list degrading to a plain container.
func TestListTileTrailingAndChevron(t *testing.T) {
	t.Run("trailing-slot", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "listtile", ID: "lt", Label: "T", Props: map[string]any{"trailing": "NEW"}})
		if !strings.Contains(res.HTML, "NEW") {
			t.Errorf("listtile should render its trailing slot:\n%s", res.HTML)
		}
	})
	t.Run("chevron-opt-out", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "listtile", ID: "lt2", Label: "T", Props: map[string]any{"chevron": "false"}, OnPress: &model.Invoke{Name: "x"}})
		if strings.Contains(res.HTML, "›") {
			t.Errorf("chevron:false should suppress the disclosure chevron:\n%s", res.HTML)
		}
	})
	t.Run("expansiontile-leading", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "expansiontile", ID: "ex", Label: "More", Props: map[string]any{"leading": "star"}, Children: textKids("b")})
		if !strings.Contains(res.HTML, iconSVG("star", 20)) {
			t.Errorf("expansiontile should render its leading icon:\n%s", res.HTML)
		}
	})
	t.Run("list-without-template-is-container", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "list", ID: "lc", Children: textKids("static-row")})
		if !strings.Contains(res.HTML, "static-row") || strings.Contains(res.HTML, "content-visibility") {
			t.Errorf("template-less list should render its children as a plain container:\n%s", res.HTML)
		}
	})
}

// TestRemainingCapabilityWidgets covers the hardware widgets (hwList family) not
// exercised by the main capability table, so every alias in the switch dispatches
// and emits its bridge wiring.
func TestRemainingCapabilityWidgets(t *testing.T) {
	cases := []struct {
		typ, class, js, label string
	}{
		{"nfc", "qorm-nfc", "qormNfc(this)", "Read NFC Tag"},
		{"vibrate", "qorm-vibrate", "qormVibrate(this)", "Vibrate"},
		{"share", "qorm-share", "qormShare(this)", "Share"},
		{"deviceinfo", "qorm-deviceinfo", "qormDeviceInfo(this)", "Device Info"},
		{"network", "qorm-network", "qormNetwork(this)", "Network Status"},
		{"haptics", "qorm-haptics", "qormHaptic(this)", "Haptic Feedback"},
		{"storage", "qorm-storage", "qormStorage(this)", "Save to Storage"},
		{"videocapture", "qorm-videocapture", "qormRecordVideo(this)", "Record Video"},
		{"orientation", "qorm-orientation", "qormOrientation(this)", "Lock Portrait"},
		{"proximity", "qorm-proximity", "qormProximity(this)", "Start Proximity"},
		{"pedometer", "qorm-pedometer", "qormPedometer(this)", "Start Pedometer"},
		{"barometer", "qorm-barometer", "qormBarometer(this)", "Start Barometer"},
		{"contacts", "qorm-contacts", "qormPickContact(this)", "Pick Contact"},
		{"calendar", "qorm-calendar", "qormAddEvent(this)", "Add Event"},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: tc.typ, ID: "hw-" + tc.typ})
			for _, w := range []string{tc.class, tc.js, tc.label} {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("%s lacks %q:\n%s", tc.typ, w, res.HTML)
				}
			}
			if len(res.Unknown) != 0 {
				t.Errorf("%s reported unknown: %v", tc.typ, res.Unknown)
			}
		})
	}

	// biometric/location/recorder with a bound value show the synced readout and
	// hidden state input (the value-present branch).
	t.Run("biometric-with-value", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{Type: "biometric", ID: "bio", Value: "{{ state.auth }}"}, map[string]any{"auth": "OK"})
		if !strings.Contains(res.HTML, "OK") || !strings.Contains(res.HTML, `data-state="auth"`) {
			t.Errorf("biometric should show the bound readout and sync path:\n%s", res.HTML)
		}
	})
	t.Run("recorder-with-value", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{Type: "recorder", ID: "rec", Value: "{{ state.clip }}"}, map[string]any{"clip": "data:audio/x"})
		if !strings.Contains(res.HTML, `src="data:audio/x"`) || !strings.Contains(res.HTML, "display:block") {
			t.Errorf("recorder with a clip should show the player:\n%s", res.HTML)
		}
	})
}
