package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// These tests cover the list's built-in depth features — pagination
// (pageSize/page), separators, sticky section headers (groupBy) and
// pull-to-refresh (onRefresh) — plus the two rules that keep them honest:
// every feature is inert unless its prop is present (back-compat), and each one
// composes with the item scope (index/first/last/as) instead of replacing it.

// nums returns a data array of n string items "i0".."i(n-1)".
func nums(n int) []any {
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, string(rune('a'+i)))
	}
	return out
}

// listItems renders a list over `data` with the given props and an item
// template printing "<index>:<item>".
func listItems(t *testing.T, props map[string]any, data []any) Result {
	t.Helper()
	return renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}", Props: props,
		Template: textNode("n", "{{ index }}:{{ item }}"),
	}, map[string]any{"items": data, "page": 2})
}

func TestListPagination(t *testing.T) {
	// pageSize + a bound page renders exactly one window of the data.
	res := listItems(t, map[string]any{"pageSize": 3.0, "page": "{{ state.page }}"}, nums(8))
	for _, want := range []string{">3:d<", ">4:e<", ">5:f<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("page 2 of size 3 should render %q:\n%s", want, res.HTML)
		}
	}
	for _, bad := range []string{">0:a<", ">2:c<", ">6:g<", ">7:h<"} {
		if strings.Contains(res.HTML, bad) {
			t.Errorf("page 2 of size 3 must not render %q:\n%s", bad, res.HTML)
		}
	}
	// The index (and the node id suffix) is the GLOBAL position, so numbering
	// keeps counting across pages and an element keeps its id on every page.
	if !strings.Contains(res.HTML, `id="n-3"`) {
		t.Errorf("paginated item ids should carry the global index:\n%s", res.HTML)
	}

	// No pageSize: the full data renders, exactly as before pagination existed.
	full := listItems(t, nil, nums(8))
	if got := strings.Count(full.HTML, ":"); got < 8 {
		t.Errorf("a list without pageSize must render every item:\n%s", full.HTML)
	}
	if strings.Contains(full.HTML, "page") {
		t.Errorf("an unpaginated list must not gain any pagination markup:\n%s", full.HTML)
	}

	t.Run("clamping", func(t *testing.T) {
		cases := []struct {
			name         string
			props        map[string]any
			want, absent []string
		}{
			// page past the end clamps to the LAST page (2 items on page 3),
			// never an empty screen the user cannot page back from.
			{"overshoot", map[string]any{"pageSize": 3.0, "page": 99.0},
				[]string{">6:g<", ">7:h<"}, []string{">5:f<"}},
			{"zero-page", map[string]any{"pageSize": 3.0, "page": 0.0},
				[]string{">0:a<", ">2:c<"}, []string{">3:d<"}},
			{"negative-page", map[string]any{"pageSize": 3.0, "page": -5.0},
				[]string{">0:a<"}, []string{">3:d<"}},
			// pageSize >= total: one page holding everything.
			{"oversized-page", map[string]any{"pageSize": 100.0, "page": 1.0},
				[]string{">0:a<", ">7:h<"}, nil},
			// a non-positive pageSize means "no pagination", not "no items".
			{"zero-size", map[string]any{"pageSize": 0.0, "page": 2.0},
				[]string{">0:a<", ">7:h<"}, nil},
			// page defaults to 1 when unset.
			{"default-page", map[string]any{"pageSize": 2.0},
				[]string{">0:a<", ">1:b<"}, []string{">2:c<"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res := listItems(t, tc.props, nums(8))
				for _, w := range tc.want {
					if !strings.Contains(res.HTML, w) {
						t.Errorf("missing %q:\n%s", w, res.HTML)
					}
				}
				for _, b := range tc.absent {
					if strings.Contains(res.HTML, b) {
						t.Errorf("unexpected %q:\n%s", b, res.HTML)
					}
				}
			})
		}
	})

	t.Run("empty-data", func(t *testing.T) {
		res := listItems(t, map[string]any{"pageSize": 3.0, "page": 2.0}, nil)
		if strings.Contains(res.HTML, ":") && strings.Contains(res.HTML, `id="n-`) {
			t.Errorf("an empty data set should render no items:\n%s", res.HTML)
		}
	})
}

// TestListPaginationScope pins the page/scope contract: index/first/last
// describe the position in the FULL data, so on an inner page nothing is first
// or last, and the true last row is flagged on the final page only.
func TestListPaginationScope(t *testing.T) {
	tmpl := &model.Node{Type: "column", ID: "row", Children: []*model.Node{
		textNode("n", "{{ rowIndex }}:{{ row }}"),
		{Type: "text", ID: "f", Text: "FIRST", Props: map[string]any{"if": "{{ rowFirst }}"}},
		{Type: "text", ID: "z", Text: "LAST", Props: map[string]any{"if": "{{ rowLast }}"}},
	}}
	render := func(page float64) string {
		return renderWidgetState(t, &model.Node{
			Type: "list", ID: "l", Data: "{{ state.items }}", Template: tmpl,
			Props: map[string]any{"as": "row", "pageSize": 2.0, "page": page},
		}, map[string]any{"items": nums(5)}).HTML
	}
	first, middle, last := render(1), render(2), render(3)
	if !strings.Contains(first, ">FIRST<") || strings.Contains(first, ">LAST<") {
		t.Errorf("page 1 holds the data's first item and not its last:\n%s", first)
	}
	if strings.Contains(middle, ">FIRST<") || strings.Contains(middle, ">LAST<") {
		t.Errorf("an inner page holds neither the first nor the last item:\n%s", middle)
	}
	if !strings.Contains(last, ">LAST<") || strings.Contains(last, ">FIRST<") {
		t.Errorf("the final page holds the data's last item:\n%s", last)
	}
	if !strings.Contains(middle, ">2:c<") || !strings.Contains(last, ">4:e<") {
		t.Errorf("the `as` alias must keep working under pagination:\n%s\n%s", middle, last)
	}
}

func TestListSeparator(t *testing.T) {
	const hair = `background:var(--sep);`
	res := listItems(t, map[string]any{"separator": true}, nums(3))
	if got := strings.Count(res.HTML, hair); got != 2 {
		t.Errorf("3 items want 2 separators (none after the last), got %d:\n%s", got, res.HTML)
	}
	// The separator follows its item inside the item's own wrapper, so the list
	// keeps one element child per item (drag-to-reorder indices stay right).
	if !strings.Contains(res.HTML, `>0:a</div><div style="height:0.5px`) {
		t.Errorf("the separator should follow the item inside its wrapper:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, `2:c</div><div style="height:0.5px`) {
		t.Errorf("no separator may follow the last item:\n%s", res.HTML)
	}

	t.Run("configured", func(t *testing.T) {
		res := listItems(t, map[string]any{"separator": map[string]any{
			"height": 2.0, "inset": 16.0, "color": "var(--accent)"}}, nums(2))
		if !strings.Contains(res.HTML, `height:2px;background:var(--accent);margin-left:16px;`) {
			t.Errorf("the object form should tune height/color/inset:\n%s", res.HTML)
		}
	})
	t.Run("inert-when-absent-or-false", func(t *testing.T) {
		none := listItems(t, nil, nums(3)).HTML
		off := listItems(t, map[string]any{"separator": false}, nums(3)).HTML
		if strings.Contains(none, hair) || strings.Contains(off, hair) {
			t.Errorf("no separator prop (or false) must draw nothing:\n%s\n%s", none, off)
		}
		if none != off {
			t.Errorf("separator:false must render byte-identically to no separator:\n%s\n%s", none, off)
		}
	})
	t.Run("single-item", func(t *testing.T) {
		if got := strings.Count(listItems(t, map[string]any{"separator": true}, nums(1)).HTML, hair); got != 0 {
			t.Errorf("a one-item list needs no separator, got %d", got)
		}
	})
}

// people is grouped data for the section-header tests: two Eng rows then one
// Sales row, in data order.
func people() []any {
	return []any{
		map[string]any{"name": "ann", "dept": "eng", "label": "Engineering"},
		map[string]any{"name": "bob", "dept": "eng", "label": "Engineering"},
		map[string]any{"name": "cid", "dept": "sales", "label": "Sales"},
	}
}

func groupedList(t *testing.T, props map[string]any) Result {
	t.Helper()
	return renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}", Props: props,
		Template: textNode("n", "{{ item.name }}"),
	}, map[string]any{"items": people()})
}

func TestListStickySectionHeaders(t *testing.T) {
	res := groupedList(t, map[string]any{"groupBy": "dept"})
	if got := strings.Count(res.HTML, `class="qorm-list-section"`); got != 2 {
		t.Errorf("two consecutive-key runs want two headers, got %d:\n%s", got, res.HTML)
	}
	if got := strings.Count(res.HTML, "position:sticky;top:0px;"); got != 2 {
		t.Errorf("headers should be sticky at top 0 by default:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, ">eng</div>") || !strings.Contains(res.HTML, ">sales</div>") {
		t.Errorf("the header label defaults to the group key:\n%s", res.HTML)
	}
	// The header opens its section: it precedes the section's first row.
	if strings.Index(res.HTML, ">eng</div>") > strings.Index(res.HTML, ">ann<") {
		t.Errorf("a header must precede its section's items:\n%s", res.HTML)
	}
	if strings.Index(res.HTML, ">sales</div>") < strings.Index(res.HTML, ">bob<") {
		t.Errorf("the second header must open after the first section's items:\n%s", res.HTML)
	}

	t.Run("bound-groupBy-and-label", func(t *testing.T) {
		res := groupedList(t, map[string]any{
			"groupBy": "{{ item.dept }}", "sectionHeader": "{{ item.label }}"})
		if !strings.Contains(res.HTML, ">Engineering</div>") || !strings.Contains(res.HTML, ">Sales</div>") {
			t.Errorf("sectionHeader is evaluated in the section's first item's scope:\n%s", res.HTML)
		}
	})
	t.Run("sticky-off-and-offset", func(t *testing.T) {
		if !strings.Contains(groupedList(t, map[string]any{"groupBy": "dept", "sticky": false}).HTML, "position:static;") {
			t.Error("sticky:false should render a header that scrolls away")
		}
		if !strings.Contains(groupedList(t, map[string]any{"groupBy": "dept", "stickyTop": 44.0}).HTML, "position:sticky;top:44px;") {
			t.Error("stickyTop should park the header under a pinned appbar")
		}
	})
	t.Run("inert-when-absent", func(t *testing.T) {
		if strings.Contains(groupedList(t, nil).HTML, "qorm-list-section") {
			t.Error("a list without groupBy must emit no section header")
		}
	})
	t.Run("separator-stops-at-a-section-boundary", func(t *testing.T) {
		res := groupedList(t, map[string]any{"groupBy": "dept", "separator": true})
		// 3 rows, 2 sections: only the ann|bob gap gets a rule — the boundary is
		// the header itself and the last row ends the list.
		if got := strings.Count(res.HTML, "background:var(--sep);"); got != 1 {
			t.Errorf("want exactly 1 separator across a 2-section list, got %d:\n%s", got, res.HTML)
		}
	})
	t.Run("with-pagination", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{
			Type: "list", ID: "l", Data: "{{ state.items }}",
			Props:    map[string]any{"groupBy": "dept", "pageSize": 2.0, "page": 2.0},
			Template: textNode("n", "{{ index }}:{{ item.name }}"),
		}, map[string]any{"items": people()})
		if !strings.Contains(res.HTML, ">2:cid<") || strings.Contains(res.HTML, ">ann<") {
			t.Errorf("page 2 should hold only the sales row:\n%s", res.HTML)
		}
		if got := strings.Count(res.HTML, `class="qorm-list-section"`); got != 1 {
			t.Errorf("the page's single section wants a single header, got %d:\n%s", got, res.HTML)
		}
	})
}

func TestListPullToRefresh(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}",
		Props: map[string]any{"onRefresh": map[string]any{"name": "reload", "args": map[string]any{}}},
		// virtualize must stay orthogonal to the refresh wiring
		Template: textNode("n", "{{ item }}"),
	}, map[string]any{"items": nums(2)})
	for _, want := range []string{
		"qorm-refresh-spin",                            // the same affordance refreshindicator uses
		`qormRefresh(document.getElementById("l")`,     // the same client gesture
		"overflow-y:auto;overscroll-behavior:contain;", // the list becomes its own scroll port
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("a list with onRefresh should emit %q:\n%s", want, res.HTML)
		}
	}
	if len(res.Handlers) != 1 || res.Handlers[0].Name != "reload" {
		t.Errorf("onRefresh should register the refresh action: %+v", res.Handlers)
	}
	// The spinner is the container's first child — what qormRefresh animates.
	if !strings.Contains(res.HTML, `overscroll-behavior:contain;"><div class="qorm-refresh-spin"`) {
		t.Errorf("the spinner must be the list's first child:\n%s", res.HTML)
	}

	t.Run("inert-when-absent", func(t *testing.T) {
		if strings.Contains(listItems(t, nil, nums(2)).HTML, "qorm-refresh-spin") {
			t.Error("a list without onRefresh must not gain a scroll port or spinner")
		}
	})
	t.Run("reorder-wins-over-refresh", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{
			Type: "list", ID: "l", Data: "{{ state.items }}", Template: textNode("n", "{{ item }}"),
			Props: map[string]any{
				"reorderable": true,
				"onReorder":   map[string]any{"name": "move", "args": map[string]any{}},
				"onRefresh":   map[string]any{"name": "reload", "args": map[string]any{}},
			},
		}, map[string]any{"items": nums(2)})
		if !strings.Contains(res.HTML, "qormReorder(") {
			t.Errorf("reorder must stay wired:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "qormRefresh(") {
			t.Errorf("two pointer-drag gestures on one element: refresh must yield to reorder:\n%s", res.HTML)
		}
		if len(res.Handlers) != 1 || res.Handlers[0].Name != "move" {
			t.Errorf("only the reorder handler should be registered: %+v", res.Handlers)
		}
	})
	t.Run("grouping-drops-reorder", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{
			Type: "list", ID: "l", Data: "{{ state.items }}", Template: textNode("n", "{{ item.name }}"),
			Props: map[string]any{
				"groupBy":     "dept",
				"reorderable": true,
				"onReorder":   map[string]any{"name": "move", "args": map[string]any{}},
			},
		}, map[string]any{"items": people()})
		if strings.Contains(res.HTML, "qormReorder(") {
			t.Errorf("section headers shift the child indices reorder counts on:\n%s", res.HTML)
		}
	})
}

// TestGridViewPagination pins gridview's parity with list: the same pageSize /
// page window and the same global index + id suffix.
func TestGridViewPagination(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "gridview", ID: "g", Data: "{{ state.items }}",
		Props:    map[string]any{"pageSize": 2.0, "page": "{{ state.page }}"},
		Template: textNode("c", "{{ index }}:{{ item }}"),
	}, map[string]any{"items": nums(5), "page": 3})
	if !strings.Contains(res.HTML, ">4:e<") || strings.Contains(res.HTML, ">0:a<") {
		t.Errorf("gridview page 3 of size 2 should hold the 5th cell only:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `id="c-4"`) {
		t.Errorf("gridview cell ids should carry the global index:\n%s", res.HTML)
	}
}
