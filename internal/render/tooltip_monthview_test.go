package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// This file covers the two desktop-shaped widgets added on top of the legacy
// `tooltip` PROP: the tooltip WIDGET (r.tooltip, render_feedback.go) and the
// month-grid calendar (r.monthView, render_widgets.go). Three things are pinned
// here that nothing else in the suite would notice:
//
//   - the legacy `tooltip` prop keeps rendering BYTE-IDENTICALLY, so no shipped
//     app changes shape (the widget is additive, not a migration);
//   - the calendar's date arithmetic is right across month, leap-year and
//     min/max boundaries — the cases a hand-rolled grid always gets wrong;
//   - neither widget lets an author value out of its escaping: not into an
//     attribute, and not into a <style> block (where html.EscapeString would be
//     no defence at all — CSS has no entities, so a ';' or '}' would simply
//     start a new declaration or rule).

// xssPayload is one string carrying every character that could break out of an
// HTML attribute, an HTML text node or a CSS declaration.
const xssPayload = `" onmouseover=alert(1) x="` + "`" + `;}</style><script>alert(1)</script><style>{`

// assertNoInjection asserts that a rendered fragment neither opened a tag nor a
// style block that the author supplied.
func assertNoInjection(t *testing.T, what, html string) {
	t.Helper()
	// The payload opens with a raw double quote, so its verbatim presence would
	// prove an attribute was broken out of; "<script" / "<style" would prove
	// markup (or a stylesheet) was opened. Entity-encoded copies are fine — the
	// browser decodes them back into text, never into syntax.
	for _, bad := range []string{"<script", "<style", "</style", xssPayload} {
		if strings.Contains(html, bad) {
			t.Errorf("%s: author value escaped its context (%q present):\n%s", what, bad, html)
		}
	}
}

// ---- tooltip -----------------------------------------------------------------

// TestTooltipLegacyPropUnchanged is the backward-compatibility gate. The widget
// must not change how the `tooltip` PROP renders on ANY node — same attribute,
// same value, and none of the widget's own markup leaking in.
func TestTooltipLegacyPropUnchanged(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "text", ID: "t", Text: "x",
		Props: map[string]any{"tooltip": "hint"}})
	if !strings.Contains(res.HTML, `data-tooltip="hint"`) {
		t.Errorf("the tooltip prop must still emit data-tooltip:\n%s", res.HTML)
	}
	for _, leak := range []string{"qorm-tip", "role=\"tooltip\"", "aria-describedby"} {
		if strings.Contains(res.HTML, leak) {
			t.Errorf("the tooltip prop must not gain the widget's markup (%q):\n%s", leak, res.HTML)
		}
	}
	// The prop is deliberately NOT interpolated (a11y leaves role/title/tooltip
	// props literal) — an app relying on the literal must keep seeing it.
	raw := renderWidgetState(t, &model.Node{Type: "text", ID: "t", Text: "x",
		Props: map[string]any{"tooltip": "{{ state.hint }}"}}, map[string]any{"hint": "H"})
	if !strings.Contains(raw.HTML, `data-tooltip="{{ state.hint }}"`) {
		t.Errorf("the legacy prop must keep its verbatim (uninterpolated) behavior:\n%s", raw.HTML)
	}
}

func TestTooltipWidgetBasics(t *testing.T) {
	res := renderWidgetState(t, &model.Node{Type: "tooltip", ID: "tt",
		Props:    map[string]any{"tooltip": "Booked for {{ state.who }}"},
		Children: []*model.Node{{Type: "text", ID: "in", Text: "Hover"}}},
		map[string]any{"who": "Ada"})
	for _, want := range []string{
		`class="qorm-tip qorm-tip-top"`,          // default placement
		`aria-describedby="tt-tip"`,              // wrapper points at the bubble
		`role="tooltip" id="tt-tip"`,             // the bubble announces itself
		`>Booked for Ada</span>`,                 // {{ binding }} resolved
		`tabindex="0"`,                           // keyboard-reachable (inert child)
		"position:relative;display:inline-flex;", // the bubble's containing block
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("tooltip widget should render %q:\n%s", want, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "data-tooltip=") {
		t.Errorf("the tooltip widget must not ALSO emit the legacy attribute (two bubbles):\n%s", res.HTML)
	}
}

func TestTooltipWidgetPositions(t *testing.T) {
	for _, tc := range []struct{ prop, class string }{
		{"", "qorm-tip-top"},
		{"top", "qorm-tip-top"},
		{"bottom", "qorm-tip-bottom"},
		{"below", "qorm-tip-bottom"},
		{"left", "qorm-tip-left"},
		{"start", "qorm-tip-left"},
		{"right", "qorm-tip-right"},
		{"end", "qorm-tip-right"},
		{"  RIGHT  ", "qorm-tip-right"},       // trimmed + case-insensitive
		{"diagonal", "qorm-tip-top"},          // unknown falls back, never a bad class
		{"}{ x", "qorm-tip-top"},              // and can never be the author's string
		{"{{ state.pos }}", "qorm-tip-right"}, // the placement itself may be bound
	} {
		n := &model.Node{Type: "tooltip", ID: "tt", Props: map[string]any{"tooltip": "h"},
			Children: []*model.Node{{Type: "text", ID: "in", Text: "x"}}}
		if tc.prop != "" {
			n.Props["position"] = tc.prop
		}
		res := renderWidgetState(t, n, map[string]any{"pos": "right"})
		if !strings.Contains(res.HTML, `class="qorm-tip `+tc.class+`"`) {
			t.Errorf("position %q should render class %q:\n%s", tc.prop, tc.class, res.HTML)
		}
	}
	// `placement` is accepted as the alias an author is likely to reach for.
	res := renderWidget(t, &model.Node{Type: "tooltip", ID: "tt",
		Props:    map[string]any{"tooltip": "h", "placement": "bottom"},
		Children: []*model.Node{{Type: "text", ID: "in", Text: "x"}}})
	if !strings.Contains(res.HTML, "qorm-tip-bottom") {
		t.Errorf("`placement` should be an alias of `position`:\n%s", res.HTML)
	}
}

func TestTooltipWidgetMaxWidthAndEmpty(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "tooltip", ID: "tt",
		Props:    map[string]any{"tooltip": "a long hint that must wrap", "maxWidth": 320.0},
		Children: []*model.Node{{Type: "text", ID: "in", Text: "x"}}})
	if !strings.Contains(res.HTML, `id="tt-tip" style="max-width:320px;"`) {
		t.Errorf("maxWidth should cap the bubble so long text wraps:\n%s", res.HTML)
	}
	// A hint that resolves to nothing degrades to a plain wrapper — no empty
	// bubble to flash on hover, and no dangling aria-describedby.
	empty := renderWidgetState(t, &model.Node{Type: "tooltip", ID: "tt",
		Props:    map[string]any{"tooltip": "{{ state.missing }}"},
		Children: []*model.Node{{Type: "text", ID: "in", Text: "child"}}}, nil)
	if strings.Contains(empty.HTML, "qorm-tip-bubble") || strings.Contains(empty.HTML, "aria-describedby") {
		t.Errorf("an empty hint should render no bubble at all:\n%s", empty.HTML)
	}
	if !strings.Contains(empty.HTML, ">child</div>") {
		t.Errorf("an empty hint must still render the wrapped children:\n%s", empty.HTML)
	}
	// label/text stand in for the `tooltip` prop, so the terse form works.
	lbl := renderWidget(t, &model.Node{Type: "tooltip", ID: "tt", Text: "from text",
		Children: []*model.Node{{Type: "icon", ID: "ic", Text: "star"}}})
	if !strings.Contains(lbl.HTML, ">from text</span>") {
		t.Errorf("text/label should stand in for the tooltip prop:\n%s", lbl.HTML)
	}
}

// TestTooltipWidgetFocusTarget: the tooltip must be reachable from the keyboard,
// but wrapping a control that is ALREADY a tab stop must not create a second
// one — the shell's :focus-within rule covers that case.
func TestTooltipWidgetFocusTarget(t *testing.T) {
	tip := func(child *model.Node, props map[string]any) string {
		p := map[string]any{"tooltip": "h"}
		for k, v := range props {
			p[k] = v
		}
		return renderWidget(t, &model.Node{Type: "tooltip", ID: "tt", Props: p,
			Children: []*model.Node{child}}).HTML
	}
	if h := tip(&model.Node{Type: "text", ID: "c", Text: "x"}, nil); !strings.Contains(h, `tabindex="0"`) {
		t.Errorf("a tooltip around inert content must become the tab stop:\n%s", h)
	}
	if h := tip(&model.Node{Type: "button", ID: "c", Label: "x"}, nil); strings.Contains(h, `tabindex="0"`) {
		t.Errorf("a tooltip around a button must not double the tab order:\n%s", h)
	}
	if h := tip(&model.Node{Type: "text", ID: "c", Text: "x"}, map[string]any{"focusable": false}); strings.Contains(h, `tabindex="0"`) {
		t.Errorf("focusable:false must remove the tab stop:\n%s", h)
	}
	if h := tip(&model.Node{Type: "button", ID: "c", Label: "x"}, map[string]any{"focusable": true}); !strings.Contains(h, `tabindex="0"`) {
		t.Errorf("focusable:true must force a tab stop:\n%s", h)
	}
}

// TestTooltipWidgetWrapsAnything is the coverage claim: the widget reaches nodes
// the `tooltip` PROP never could, because a11y() is not called by every renderer
// (video is the canonical example).
func TestTooltipWidgetWrapsAnything(t *testing.T) {
	vid := &model.Node{Type: "video", ID: "v", Props: map[string]any{"src": "/a.mp4"}}
	bare := renderWidget(t, vid)
	if strings.Contains(bare.HTML, "data-tooltip") {
		t.Fatalf("precondition changed: video now emits data-tooltip itself:\n%s", bare.HTML)
	}
	res := renderWidget(t, &model.Node{Type: "tooltip", ID: "tt",
		Props: map[string]any{"tooltip": "Trailer"}, Children: []*model.Node{vid}})
	if !strings.Contains(res.HTML, "<video") || !strings.Contains(res.HTML, ">Trailer</span>") {
		t.Errorf("the tooltip widget should tip a node that never calls a11y():\n%s", res.HTML)
	}
}

func TestTooltipWidgetInjectionClosure(t *testing.T) {
	res := renderWidgetState(t, &model.Node{Type: "tooltip", ID: xssPayload,
		Props: map[string]any{
			"tooltip":  "{{ state.evil }}",
			"position": xssPayload,
			"maxWidth": 200.0,
		},
		Children: []*model.Node{{Type: "text", ID: "in", Text: "x"}}},
		map[string]any{"evil": xssPayload})
	assertNoInjection(t, "tooltip widget", res.HTML)
	if strings.Contains(res.HTML, `class="qorm-tip "`) || strings.Contains(res.HTML, xssPayload) {
		t.Errorf("the placement class must come from the closed switch, never the author:\n%s", res.HTML)
	}
}

// ---- monthview ---------------------------------------------------------------

// dayButtons returns the data-date values of the grid cells, in render order.
var dayRe = regexp.MustCompile(`data-date="([0-9-]+)"`)

func dayButtons(html string) []string {
	var out []string
	for _, m := range dayRe.FindAllStringSubmatch(html, -1) {
		out = append(out, m[1])
	}
	return out
}

func monthNode(id string, props map[string]any) *model.Node {
	return &model.Node{Type: "monthview", ID: id, Props: props,
		OnChange: &model.Invoke{Name: "pick"}}
}

// TestMonthViewGrid pins the calendar arithmetic: the grid always starts on the
// configured first weekday, always holds whole weeks, and the leading/trailing
// cells carry the REAL neighbouring-month dates (a booking that straddles a
// month boundary has to be clickable).
func TestMonthViewGrid(t *testing.T) {
	for _, tc := range []struct {
		name, month, weekStart string
		first, last            string
		cells                  int
	}{
		// Feb 2026 starts on a Sunday and has 28 days: with a Sunday start it is
		// exactly four rows and no adjacent day at all.
		{"feb-2026-sunday-start", "2026-02", "", "2026-02-01", "2026-02-28", 28},
		// Same month, Monday start: one leading week-tail from January.
		{"feb-2026-monday-start", "2026-02", "monday", "2026-01-26", "2026-03-01", 35},
		// Leap year: February 2024 really has 29 days.
		{"feb-2024-leap", "2024-02", "", "2024-01-28", "2024-03-02", 35},
		// Century non-leap: 1900 is NOT a leap year (the %100 rule).
		{"feb-1900-not-leap", "1900-02", "", "1900-01-28", "1900-03-03", 35},
		// Century leap: 2000 IS (the %400 rule).
		{"feb-2000-leap", "2000-02", "", "2000-01-30", "2000-03-04", 35},
		// A 31-day month that needs six rows.
		{"may-2026", "2026-05", "", "2026-04-26", "2026-06-06", 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			props := map[string]any{"month": tc.month}
			if tc.weekStart != "" {
				props["weekStart"] = tc.weekStart
			}
			days := dayButtons(renderWidget(t, monthNode("cal", props)).HTML)
			if len(days) != tc.cells {
				t.Fatalf("grid should hold %d cells (whole weeks), got %d: %v", tc.cells, len(days), days)
			}
			if days[0] != tc.first || days[len(days)-1] != tc.last {
				t.Errorf("grid spans %s..%s, want %s..%s", days[0], days[len(days)-1], tc.first, tc.last)
			}
			if len(days)%7 != 0 {
				t.Errorf("grid must be whole weeks, got %d cells", len(days))
			}
		})
	}
}

// TestMonthViewLeapDayIsRendered: the 29th of a leap February must exist as a
// cell, and must NOT exist in the same month of a common year.
func TestMonthViewLeapDayIsRendered(t *testing.T) {
	leap := renderWidget(t, monthNode("cal", map[string]any{"month": "2024-02"})).HTML
	if !strings.Contains(leap, `data-date="2024-02-29"`) {
		t.Error("2024-02-29 should be a cell of February 2024")
	}
	common := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-02"})).HTML
	if strings.Contains(common, `data-date="2026-02-29"`) {
		t.Error("2026-02-29 does not exist and must not be rendered")
	}
	// A `selected` that is not a real day is ignored rather than clamped.
	bogus := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-02", "selected": "2026-02-30"})).HTML
	if strings.Contains(bogus, `aria-current="date"`) {
		t.Error("an impossible selected date must select nothing")
	}
}

// TestMonthViewSelectionAndEvents: the selection and the event markers are both
// data-bound, and the marker colour goes through the repo's CSS allowlist.
func TestMonthViewSelectionAndEvents(t *testing.T) {
	res := renderWidgetState(t, monthNode("cal", map[string]any{
		"month":    "2026-07",
		"selected": "{{ state.day }}",
		"today":    "2026-07-01",
		"events": []any{
			"2026-07-04",
			map[string]any{"date": "2026-07-09", "color": "var(--danger)"},
			map[string]any{"date": "2026-07-11", "color": "red;}#x{background:url(evil)"},
			map[string]any{"date": "not-a-date"},
		},
	}), map[string]any{"day": "2026-07-15"})

	if !strings.Contains(res.HTML, `data-date="2026-07-15" aria-label="2026-07-15" aria-current="date"`) {
		t.Errorf("the bound day should be marked selected:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "background:var(--accent);color:var(--on-accent);") {
		t.Errorf("the selected day should paint in the accent colour:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "box-shadow:inset 0 0 0 1.5px var(--accent);") {
		t.Errorf("the `today` prop should ring its day:\n%s", res.HTML)
	}
	dots := strings.Count(res.HTML, "border-radius:50%;background:")
	if dots != 3 {
		t.Errorf("three parseable events should draw three dots, got %d:\n%s", dots, res.HTML)
	}
	if !strings.Contains(res.HTML, "background:var(--danger);") {
		t.Errorf("an allowlisted event colour should survive:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "url(evil)") || strings.Contains(res.HTML, "#x{") {
		t.Errorf("a colour carrying CSS punctuation must be dropped by cssValue, not escaped:\n%s", res.HTML)
	}

	// A colour whose CHARSET is legal but which still reaches off the page (a
	// url() the browser fetches) or truncates the declarations after it (/*) is
	// dropped just the same, and the dot falls back to the accent.
	for _, payload := range []string{"url(//attacker/beacon.png)", "red/*",
		"#fff;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999"} {
		bad := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-07",
			"events": []any{map[string]any{"date": "2026-07-09", "color": payload}}})).HTML
		for _, frag := range []string{"//attacker", "/*", "100vw", "z-index:99999"} {
			if strings.Contains(bad, frag) {
				t.Errorf("event colour %q leaked %q:\n%s", payload, frag, bad)
			}
		}
		if !strings.Contains(bad, "border-radius:50%;background:var(--accent);") {
			t.Errorf("a rejected event colour must fall back to the accent dot:\n%s", bad)
		}
	}
}

// TestMonthViewMinMax: days outside the range are natively disabled and carry no
// handler, and an arrow whose whole target month is out of range disables too.
func TestMonthViewMinMax(t *testing.T) {
	res := renderWidget(t, monthNode("cal", map[string]any{
		"month": "2026-07", "min": "2026-07-10", "max": "2026-07-20"}))
	if !strings.Contains(res.HTML, `data-date="2026-07-09" aria-label="2026-07-09" disabled`) {
		t.Errorf("a day before `min` should be a disabled button:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `data-date="2026-07-21" aria-label="2026-07-21" disabled`) {
		t.Errorf("a day after `max` should be a disabled button:\n%s", res.HTML)
	}
	// Exactly 11 selectable days (10th..20th) — the boundaries are inclusive.
	if got := strings.Count(res.HTML, `onclick="qorm(`); got != 11 {
		t.Errorf("min..max should leave 11 clickable days, got %d:\n%s", got, res.HTML)
	}
	// Both neighbouring months lie entirely outside the range.
	if strings.Count(res.HTML, `aria-label="Previous month" style=`) != 1 ||
		!strings.Contains(res.HTML, `aria-label="Previous month"`) {
		t.Fatalf("no previous-month arrow rendered:\n%s", res.HTML)
	}
	for _, arrow := range []string{"Previous month", "Next month"} {
		i := strings.Index(res.HTML, `aria-label="`+arrow+`"`)
		j := strings.Index(res.HTML[i:], ">")
		if !strings.Contains(res.HTML[i:i+j], " disabled") {
			t.Errorf("the %s arrow should be disabled when its month is out of range:\n%s", arrow, res.HTML[i:i+j])
		}
	}
}

// TestMonthViewMonthNavigation covers both paging paths: the explicit
// `onMonthChange` invoke, and the fallback through the day handler that makes a
// calendar wired with one action page correctly for free.
func TestMonthViewMonthNavigation(t *testing.T) {
	t.Run("onMonthChange-invoke", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "monthview", ID: "cal", Props: map[string]any{
			"month":         "2026-01",
			"onMonthChange": map[string]any{"name": "goMonth", "args": map[string]any{"src": "cal"}},
		}})
		if len(res.Handlers) != 2 {
			t.Fatalf("both arrows should register a handler, got %d", len(res.Handlers))
		}
		// December of the PREVIOUS year, January of the NEXT one.
		if got := res.Handlers[0]; got.Name != "goMonth" || got.Args["month"] != "2025-12" || got.Args["src"] != "cal" {
			t.Errorf("prev arrow = %+v, want goMonth month=2025-12 src=cal", got)
		}
		if got := res.Handlers[1]; got.Name != "goMonth" || got.Args["month"] != "2026-02" {
			t.Errorf("next arrow = %+v, want goMonth month=2026-02", got)
		}
	})
	t.Run("onMonthChange-bare-name", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "monthview", ID: "cal",
			Props: map[string]any{"month": "2026-01", "onMonthChange": "goMonth"}})
		if len(res.Handlers) != 2 || res.Handlers[0].Name != "goMonth" {
			t.Fatalf("a bare action name should wire the arrows: %+v", res.Handlers)
		}
	})
	t.Run("falls-back-to-onChange", func(t *testing.T) {
		// No onMonthChange: the arrows move the SELECTION, and since `month`
		// defaults to the selected month the grid follows.
		res := renderWidget(t, monthNode("cal", map[string]any{"selected": "2026-03-31"}))
		if res.Handlers[0].Name != "pick" || res.Handlers[0].Args["value"] != "2026-02-28" {
			t.Errorf("prev arrow should clamp 31 March back to the last day of February, got %+v", res.Handlers[0])
		}
		if res.Handlers[1].Args["value"] != "2026-04-30" {
			t.Errorf("next arrow should clamp into April, got %+v", res.Handlers[1])
		}
		if !strings.Contains(res.HTML, ">March 2026</div>") {
			t.Errorf("`month` should default to the month of `selected`:\n%s", res.HTML)
		}
	})
	t.Run("no-handler-disables-the-arrows", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "monthview", ID: "cal",
			Props: map[string]any{"month": "2026-01"}})
		if len(res.Handlers) != 0 {
			t.Errorf("an unwired calendar should register nothing, got %+v", res.Handlers)
		}
		if strings.Count(res.HTML, " disabled") < 2 {
			t.Errorf("unwired arrows should render disabled, not dead:\n%s", res.HTML)
		}
	})
}

// TestMonthViewDayDispatch: clicking a day dispatches the ordinary onChange with
// the day as `value` — the same contract datepicker/picker already use, so no
// new event channel is introduced.
func TestMonthViewDayDispatch(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "monthview", ID: "cal",
		Props:    map[string]any{"month": "2026-07"},
		OnChange: &model.Invoke{Name: "pick", Args: map[string]string{"slot": "am"}}})
	var seen bool
	for _, h := range res.Handlers {
		if h.Args["value"] == "2026-07-04" {
			seen = true
			if h.Name != "pick" || h.Args["slot"] != "am" {
				t.Errorf("day handler = %+v, want pick with the author's extra args preserved", h)
			}
		}
	}
	if !seen {
		t.Errorf("no handler carries the clicked day as `value`:\n%+v", res.Handlers)
	}
}

func TestMonthViewOptions(t *testing.T) {
	t.Run("weekday-labels-and-title", func(t *testing.T) {
		res := renderWidget(t, monthNode("cal", map[string]any{
			"month": "2026-07", "heading": "Juillet 2026",
			"weekdays": []any{"di", "lu", "ma", "me", "je", "ve", "sa"}}))
		if !strings.Contains(res.HTML, ">Juillet 2026</div>") {
			t.Errorf("`heading` should override the generated header:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, ">di</div>") || !strings.Contains(res.HTML, ">sa</div>") {
			t.Errorf("`weekdays` should replace the built-in column labels:\n%s", res.HTML)
		}
		// A wrong-length list falls back rather than rendering a broken header.
		bad := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-07", "weekdays": []any{"a", "b"}}))
		if !strings.Contains(bad.HTML, ">Su</div>") {
			t.Errorf("a weekdays list that is not 7 long should fall back:\n%s", bad.HTML)
		}
	})
	t.Run("weekStart-rotates-the-header", func(t *testing.T) {
		res := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-07", "weekStart": "monday"}))
		i, j := strings.Index(res.HTML, ">Mo</div>"), strings.Index(res.HTML, ">Su</div>")
		if i < 0 || j < 0 || i > j {
			t.Errorf("weekStart:monday should put Mo first and Su last:\n%s", res.HTML)
		}
	})
	t.Run("showAdjacent-false-blanks-the-neighbours", func(t *testing.T) {
		res := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-05", "showAdjacent": "false"}))
		for _, d := range dayButtons(res.HTML) {
			if !strings.HasPrefix(d, "2026-05-") {
				t.Errorf("showAdjacent:false should leave only May cells, found %s", d)
			}
		}
	})
	t.Run("aliases-render-identically", func(t *testing.T) {
		base := renderWidget(t, monthNode("cal", map[string]any{"month": "2026-07"})).HTML
		for _, alias := range []string{"calendarview", "datepickercalendar"} {
			n := monthNode("cal", map[string]any{"month": "2026-07"})
			n.Type = alias
			if got := renderWidget(t, n).HTML; got != base {
				t.Errorf("alias %q should render identically to monthview", alias)
			}
		}
	})
	t.Run("default-epoch-is-fixed-not-the-clock", func(t *testing.T) {
		// A widget may not read the clock: the same state must render the same
		// bytes forever (determinism guard + OTA bundle hashes).
		res := renderWidget(t, monthNode("cal", nil))
		if !strings.Contains(res.HTML, ">July 2026</div>") {
			t.Errorf("an unconfigured calendar should open on the fixed default epoch:\n%s", res.HTML)
		}
	})
}

func TestMonthViewInjectionClosure(t *testing.T) {
	res := renderWidgetState(t, monthNode(xssPayload, map[string]any{
		"month":     "{{ state.evil }}",
		"selected":  xssPayload,
		"min":       xssPayload,
		"max":       xssPayload,
		"today":     xssPayload,
		"title":     "{{ state.evil }}",
		"weekStart": xssPayload,
		"weekdays":  []any{xssPayload, "b", "c", "d", "e", "f", "g"},
		"events":    []any{map[string]any{"date": "2026-07-04", "color": xssPayload}},
	}), map[string]any{"evil": xssPayload})
	assertNoInjection(t, "monthview", res.HTML)
	if strings.Contains(res.HTML, "<style") {
		t.Errorf("monthview must never open a <style> block (no CSS-rule surface at all):\n%s", res.HTML)
	}
	// Every rendered date is machine-generated, never the author's string.
	for _, d := range dayButtons(res.HTML) {
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(d) {
			t.Errorf("data-date should always be a generated date, got %q", d)
		}
	}
}

// TestMonthViewDeterministic: the same state must render the same bytes, twice.
func TestMonthViewDeterministic(t *testing.T) {
	n := func() *model.Node {
		return monthNode("cal", map[string]any{"month": "2026-07", "selected": "2026-07-15",
			"events": []any{"2026-07-04", "2026-07-09", "2026-07-22"}})
	}
	if a, b := renderWidget(t, n()).HTML, renderWidget(t, n()).HTML; a != b {
		t.Error("monthview render is not byte-deterministic")
	}
}

// TestTooltipWidgetInsideList: ids must stay unique when the wrapper is repeated
// by a renderItem, or aria-describedby would point every row at the first row's
// bubble.
func TestTooltipWidgetInsideList(t *testing.T) {
	res := renderWidgetState(t, &model.Node{Type: "list", ID: "l",
		Data: "{{ state.rows }}",
		Template: &model.Node{Type: "tooltip", ID: "tt",
			Props:    map[string]any{"tooltip": "{{ item.hint }}"},
			Children: []*model.Node{{Type: "text", ID: "row", Text: "{{ item.hint }}"}}}},
		map[string]any{"rows": []any{
			map[string]any{"hint": "one"}, map[string]any{"hint": "two"}}})
	ids := regexp.MustCompile(`aria-describedby="([^"]+)"`).FindAllStringSubmatch(res.HTML, -1)
	if len(ids) != 2 || ids[0][1] == ids[1][1] {
		t.Fatalf("each repeated tooltip needs its own bubble id, got %v:\n%s", ids, res.HTML)
	}
	for _, m := range ids {
		if !strings.Contains(res.HTML, `id="`+m[1]+`"`) {
			t.Errorf("aria-describedby=%q points at no bubble:\n%s", m[1], res.HTML)
		}
	}
	if !strings.Contains(res.HTML, ">one</span>") || !strings.Contains(res.HTML, ">two</span>") {
		t.Errorf("the hint should resolve per item:\n%s", res.HTML)
	}
}
