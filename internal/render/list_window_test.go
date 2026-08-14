package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// TRUE list virtualization (`virtualize: "window"`): only the rows around the
// live scroll position are rendered, and the rows outside it are replaced by
// two spacers of exactly their combined height. What these tests pin is the
// arithmetic the human's scrollbar depends on (window bounds, spacer heights,
// their sum), the GLOBAL item scope surviving the slice, the mount point the
// client re-reads at scroll time, and — the back-compat half — that the CSS
// form and every list that asks for none of it are untouched.

// windowList renders a `virtualize:"window"` list of n rows with the given
// extra props, at the given reported scroll state ("" = never reported).
func windowList(t *testing.T, n int, scroll string, extra map[string]any) Result {
	t.Helper()
	props := map[string]any{"virtualize": "window", "itemHeight": 20.0}
	for k, v := range extra {
		props[k] = v
	}
	state := map[string]any{"items": nums(n)}
	if scroll != "" {
		state[virtualScrollKey("l")] = scroll
	}
	return renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}", Props: props,
		Template: textNode("n", "{{ index }}:{{ item }}"),
	}, state)
}

// nums only produces distinct labels for the first 26 rows; rowsIn counts the
// rendered item wrappers instead, which works at any size.
func rowsIn(html string) int { return strings.Count(html, "content-visibility:auto") }

var vpadRe = regexp.MustCompile(`class="qorm-vpad" style="flex:none;height:([0-9.]+)px;"`)

// vpads returns the heights of the spacers, in document order.
func vpads(html string) []float64 {
	var out []float64
	for _, m := range vpadRe.FindAllStringSubmatch(html, -1) {
		f, _ := strconv.ParseFloat(m[1], 64)
		out = append(out, f)
	}
	return out
}

func TestListWindowSlicesAroundScroll(t *testing.T) {
	// 500 rows of 20px, a 200px port reported at scrollTop 2000 (row 100).
	// rows = 200/20 + 1 = 11 visible, plus 4 overscan on each side = 19,
	// starting 4 rows above row 100.
	res := windowList(t, 500, "2000,200", nil)
	if got := rowsIn(res.HTML); got != 19 {
		t.Errorf("window should render 19 rows, got %d:\n%s", got, res.HTML)
	}
	if !strings.Contains(res.HTML, ">96:") || !strings.Contains(res.HTML, ">114:") {
		t.Errorf("window should span rows 96..114:\n%s", res.HTML)
	}
	for _, bad := range []string{">95:", ">115:", ">0:", ">499:"} {
		if strings.Contains(res.HTML, bad) {
			t.Errorf("row %q is outside the window and must not render:\n%s", bad, res.HTML)
		}
	}
	// The spacers stand in for exactly the rows that are not there, so the
	// scroll height — and therefore the scrollbar and the offset the next
	// window is computed from — is the same as an unwindowed list's.
	pads := vpads(res.HTML)
	if len(pads) != 2 || pads[0] != 96*20 || pads[1] != (500-115)*20 {
		t.Errorf("spacers should be %gpx / %gpx, got %v:\n%s", 96*20.0, (500-115)*20.0, pads, res.HTML)
	}
	if pads[0]+pads[1]+19*20 != 500*20 {
		t.Errorf("spacers + rendered rows must add up to the full list height, got %v", pads)
	}
	// Scroll anchoring OFF: the morph recycles row elements in place, so the
	// browser's anchor keeps its identity while its content changes meaning and
	// Chrome "corrects" the scroll by the window delta — a visible jump on every
	// re-render, verified against real Chrome. Only windowed lists carry it.
	if !strings.Contains(res.HTML, "overflow-anchor:none;") {
		t.Errorf("a windowed list must disable scroll anchoring:\n%s", res.HTML)
	}
	css := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}", Props: map[string]any{"virtualize": true},
		Template: textNode("n", "{{ index }}"),
	}, map[string]any{"items": nums(20)})
	if strings.Contains(css.HTML, "overflow-anchor") {
		t.Errorf("only a WINDOWED list opts out of anchoring:\n%s", css.HTML)
	}
	// The mount point carries every parameter the client re-reads at scroll
	// time — and a hidden state input, so the report rides the ordinary event
	// channel rather than a transport of its own.
	for _, want := range []string{
		`data-qorm-vlist="__vlist_l"`, `data-item-h="20"`, `data-overscan="4"`,
		`data-qorm-vstart="96"`, `data-qorm-vcount="19"`, `data-qorm-vtotal="500"`,
		`<input type="hidden" data-qorm-vscroll data-state="__vlist_l" value="2000,200">`,
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("windowed list lacks %q:\n%s", want, res.HTML)
		}
	}
}

// TestListWindowIndexStaysGlobal: windowing must not renumber anything. `index`
// is the row's position in the DATA, not in the window, `last` still means the
// last row of the data, and a node keeps its id whatever window shows it —
// exactly the rule pagination already follows.
func TestListWindowIndexStaysGlobal(t *testing.T) {
	res := windowList(t, 500, "2000,200", nil)
	if !strings.Contains(res.HTML, `id="n-100"`) || !strings.Contains(res.HTML, ">100:") {
		t.Errorf("the windowed rows must carry their GLOBAL index and id:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, `id="n-0"`) {
		t.Error("windowing must not renumber the window's first row to index 0")
	}
	// `last` is the last row of the DATA: a window in the middle has none.
	tail := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}",
		Props:    map[string]any{"virtualize": "window", "itemHeight": 20.0},
		Template: textNode("n", "{{ index }}{{ last ? '-END' : '' }}"),
	}, map[string]any{"items": nums(500), virtualScrollKey("l"): "2000,200"})
	if strings.Contains(tail.HTML, "-END") {
		t.Errorf("`last` must describe the data, not the window:\n%s", tail.HTML)
	}
}

func TestListWindowClampsAndFallbacks(t *testing.T) {
	cases := []struct {
		name              string
		rows              int
		scroll            string
		extra             map[string]any
		wantRows          int
		wantStart         string
		padTop, padBottom float64
	}{
		// Never reported: the window starts at the top, sized from the fallback
		// port (800px / 20px + 1 + 8 = 49 rows) so the first paint is never short.
		{"first-paint", 500, "", nil, 49, `data-qorm-vstart="0"`, 0, (500 - 49) * 20},
		// A port the client has not measured yet falls back the same way.
		{"no-port", 500, "0,0", nil, 49, `data-qorm-vstart="0"`, 0, (500 - 49) * 20},
		// At the top the window cannot start above row 0 — no negative overscan.
		{"top", 500, "10,200", nil, 19, `data-qorm-vstart="0"`, 0, (500 - 19) * 20},
		// Scrolled past the end (a stale report, or data that shrank): the window
		// is pulled back so it is still full instead of rendering nothing.
		{"past-end", 40, "9999,200", nil, 19, `data-qorm-vstart="21"`, 21 * 20, 0},
		// Fewer rows than the window: everything renders and no spacer is emitted.
		{"short-list", 5, "0,200", nil, 5, `data-qorm-vstart="0"`, 0, 0},
		// overscan: 0 renders exactly the visible band.
		{"no-overscan", 500, "2000,200", map[string]any{"overscan": 0.0}, 11, `data-qorm-vstart="100"`, 100 * 20, (500 - 111) * 20},
		// A negative scroll report (hand-edited state) clamps to the top.
		{"negative", 500, "-99,200", nil, 19, `data-qorm-vstart="0"`, 0, (500 - 19) * 20},
		// A bare number is read as the scroll offset, port from the fallback.
		{"bare-number", 500, "2000", nil, 49, `data-qorm-vstart="96"`, 96 * 20, (500 - 145) * 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := windowList(t, c.rows, c.scroll, c.extra)
			if got := rowsIn(res.HTML); got != c.wantRows {
				t.Errorf("expected %d rendered rows, got %d:\n%s", c.wantRows, got, res.HTML)
			}
			if !strings.Contains(res.HTML, c.wantStart) {
				t.Errorf("expected %s:\n%s", c.wantStart, res.HTML)
			}
			var want []float64
			if c.padTop > 0 {
				want = append(want, c.padTop)
			}
			if c.padBottom > 0 {
				want = append(want, c.padBottom)
			}
			if got := vpads(res.HTML); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("spacers = %v, want %v:\n%s", got, want, res.HTML)
			}
		})
	}
	// The hidden input carries what the CLIENT measured, never the port the
	// server assumed: a first paint reports "0,0", so folding the input back
	// into state cannot freeze the fallback in as if it had been measured.
	if !strings.Contains(windowList(t, 500, "", nil).HTML, `data-qorm-vscroll data-state="__vlist_l" value="0,0"`) {
		t.Error("a never-reported window must render a 0,0 scroll input")
	}
}

// TestListWindowOptOuts: every combination whose height maths windowing cannot
// honour renders the list IN FULL (the CSS form), rather than a window that
// would lie about the scroll height. Each of these is a documented limit of the
// fixed-row-height contract, not an accident.
func TestListWindowOptOuts(t *testing.T) {
	full := func(t *testing.T, res Result, why string) {
		t.Helper()
		if got := rowsIn(res.HTML); got != 30 {
			t.Errorf("%s must render all 30 rows, got %d:\n%s", why, got, res.HTML)
		}
		for _, banned := range []string{"data-qorm-vlist", "qorm-vpad", "data-qorm-vscroll"} {
			if strings.Contains(res.HTML, banned) {
				t.Errorf("%s must not carry %q:\n%s", why, banned, res.HTML)
			}
		}
	}
	items := nums(30)
	state := map[string]any{"items": items, virtualScrollKey("l"): "400,200"}
	render := func(props map[string]any) Result {
		props["virtualize"] = "window"
		if _, ok := props["itemHeight"]; !ok {
			props["itemHeight"] = 20.0
		}
		return renderWidgetState(t, &model.Node{
			Type: "list", ID: "l", Data: "{{ state.items }}", Props: props,
			Template: textNode("n", "{{ index }}"),
		}, state)
	}
	// Section headers are boxes of their own height between rows, so row-count ×
	// itemHeight stops describing the scroll height.
	full(t, render(map[string]any{"groupBy": "x"}), "a grouped list")
	// The reorder gesture indexes the list's element children; the spacers are
	// two children with no rows behind them.
	full(t, render(map[string]any{"reorderable": true, "onReorder": "move"}), "a reorderable list")
	// No id, no stable key to report the scroll position under.
	anon := renderWidgetState(t, &model.Node{
		Type: "list", Data: "{{ state.items }}",
		Props:    map[string]any{"virtualize": "window", "itemHeight": 20.0},
		Template: textNode("n", "{{ index }}"),
	}, state)
	full(t, anon, "a list with no id")
	// A zero/negative itemHeight would make every spacer 0px.
	full(t, render(map[string]any{"itemHeight": 0.0}), "a list with no row height")
}

// TestListWindowComposesWithPageAndSeparator: windowing slices INSIDE the page
// (never across it), keeps the global index, and leaves the separator alone.
func TestListWindowComposesWithPageAndSeparator(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}",
		Props: map[string]any{
			"virtualize": "window", "itemHeight": 20.0, "overscan": 0.0,
			"pageSize": 100.0, "page": 3.0, "separator": true,
		},
		Template: textNode("n", "{{ index }}:"),
	}, map[string]any{"items": nums(500), virtualScrollKey("l"): "200,200"})
	// Page 3 covers rows 200..299; a 200px port at offset 200 shows 11 of them
	// starting at the page's row 10 — i.e. GLOBAL rows 210..220.
	if got := rowsIn(res.HTML); got != 11 {
		t.Errorf("expected 11 rendered rows, got %d:\n%s", got, res.HTML)
	}
	if !strings.Contains(res.HTML, ">210:") || !strings.Contains(res.HTML, ">220:") || strings.Contains(res.HTML, ">221:") {
		t.Errorf("the window must slice inside the page and keep global indices:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `data-qorm-vtotal="100"`) {
		t.Errorf("the window's universe is the PAGE, so vtotal is the page size:\n%s", res.HTML)
	}
	// Spacers cover the rest of the page, not the rest of the data.
	pads := vpads(res.HTML)
	if len(pads) != 2 || pads[0] != 10*20 || pads[1] != (100-21)*20 {
		t.Errorf("spacers should hold the page's other rows open, got %v:\n%s", pads, res.HTML)
	}
	if strings.Count(res.HTML, "background:var(--sep)") != 10 {
		t.Errorf("separators must still render between the windowed rows:\n%s", res.HTML)
	}
}

// TestListVirtualizeCSSUnchanged: the pre-existing CSS form, and a list that
// asks for nothing, must emit exactly what they did before windowing existed —
// this is the assertion that lets `virtualize: "window"` ship into apps that
// already use `virtualize: true`.
func TestListVirtualizeCSSUnchanged(t *testing.T) {
	css := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}",
		Props:    map[string]any{"virtualize": true},
		Template: textNode("n", "{{ index }}"),
	}, map[string]any{"items": nums(20), virtualScrollKey("l"): "400,200"})
	if got := rowsIn(css.HTML); got != 20 {
		t.Errorf("virtualize:true must keep every row in the DOM, got %d", got)
	}
	if strings.Contains(css.HTML, "data-qorm-vlist") || strings.Contains(css.HTML, "qorm-vpad") {
		t.Errorf("virtualize:true must gain no windowing markup:\n%s", css.HTML)
	}
	plain := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.items }}", Template: textNode("n", "{{ index }}"),
	}, map[string]any{"items": nums(20)})
	for _, banned := range []string{"content-visibility", "data-qorm-vlist", "qorm-vpad", "data-item-h"} {
		if strings.Contains(plain.HTML, banned) {
			t.Errorf("a plain list must not carry %q:\n%s", banned, plain.HTML)
		}
	}
}

// TestParseScrollReport pins the report parser directly, including the shapes
// only an agent or a hand-edited state file produces.
func TestParseScrollReport(t *testing.T) {
	cases := []struct {
		in        any
		top, port float64
	}{
		{nil, 0, 0},
		{"1200,640", 1200, 640},
		{" 1200 , 640 ", 1200, 640},
		{"1200", 1200, 0},
		{"", 0, 0},
		{"junk,junk", 0, 0},
		{"-5,-9", 0, 0},
		{1200.0, 1200, 0}, // an agent setting the offset with a plain number
	}
	for _, c := range cases {
		top, port := parseScrollReport(c.in)
		if top != c.top || port != c.port {
			t.Errorf("parseScrollReport(%#v) = %g,%g want %g,%g", c.in, top, port, c.top, c.port)
		}
	}
}
