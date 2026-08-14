package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// ---- frosted glass: the backdropBlur / backdropTint style keys ----------------

// TestBackdropStyleKeys covers the whole `backdropBlur` / `backdropTint`
// contract: the keys become CSS custom properties (the shell's @supports rules
// consume them, so the solid fallback is not the renderer's job), the radius is
// clamped, a non-positive or non-numeric radius emits nothing at all, and the
// tint only rides along with a radius.
func TestBackdropStyleKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style map[string]any
		want  []string
		gone  []string
	}{
		{"blur-only", map[string]any{"backdropBlur": float64(18)},
			[]string{"--qorm-bdb:18px;"}, []string{"--qorm-bdt"}},
		{"blur-and-tint", map[string]any{"backdropBlur": float64(8), "backdropTint": "rgba(255,255,255,.55)"},
			[]string{"--qorm-bdb:8px;", "--qorm-bdt:rgba(255,255,255,.55);"}, nil},
		{"clamped", map[string]any{"backdropBlur": float64(9999)},
			[]string{"--qorm-bdb:120px;"}, nil},
		{"zero-is-off", map[string]any{"backdropBlur": float64(0), "backdropTint": "#fff"},
			nil, []string{"--qorm-bdb", "--qorm-bdt"}},
		{"negative-is-off", map[string]any{"backdropBlur": float64(-4)},
			nil, []string{"--qorm-bdb"}},
		{"non-numeric-is-off", map[string]any{"backdropBlur": "lots"},
			nil, []string{"--qorm-bdb"}},
		{"tint-without-blur-is-off", map[string]any{"backdropTint": "#fff"},
			nil, []string{"--qorm-bdt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "box", ID: "b", Style: tc.style})
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("backdrop style lacks %q:\n%s", w, res.HTML)
				}
			}
			for _, w := range tc.gone {
				if strings.Contains(res.HTML, w) {
					t.Errorf("backdrop style must not emit %q:\n%s", w, res.HTML)
				}
			}
		})
	}
}

// TestBackdropBlurIsBindable proves the radius goes through resolveStyle like
// every other numeric style key, so an agent can drive the frost from state.
func TestBackdropBlurIsBindable(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "box", ID: "b", Style: map[string]any{"backdropBlur": "{{ state.blur }}"}},
		map[string]any{"blur": float64(12)})
	if !strings.Contains(res.HTML, "--qorm-bdb:12px;") {
		t.Errorf("a bound backdropBlur should resolve:\n%s", res.HTML)
	}
}

// TestFrostCSS covers the inline frost helper used by the widgets that are
// frosted by default (appbar, largetitle) — both prefixes, and nothing at all
// for a non-positive radius.
func TestFrostCSS(t *testing.T) {
	if got := frostCSS(0); got != "" {
		t.Errorf("frostCSS(0) = %q, want empty", got)
	}
	if got := frostCSS(-1); got != "" {
		t.Errorf("frostCSS(-1) = %q, want empty", got)
	}
	got := frostCSS(20)
	for _, w := range []string{"-webkit-backdrop-filter:blur(20px);", "backdrop-filter:blur(20px);"} {
		if !strings.Contains(got, w) {
			t.Errorf("frostCSS(20) = %q, lacks %q", got, w)
		}
	}
}

// TestFrostedWidgetsHonourBackdropBlur pins the opt-in override on the widgets
// that ship frosted: the default radius is unchanged (so no existing app's
// output moves), an explicit key retunes it, and 0 turns the frost off.
func TestFrostedWidgetsHonourBackdropBlur(t *testing.T) {
	for _, typ := range []string{"appbar", "largetitle"} {
		t.Run(typ+"-default", func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: typ, ID: "w", Label: "T"})
			if !strings.Contains(res.HTML, "backdrop-filter:blur(20px);") {
				t.Errorf("%s should stay frosted at 20px by default:\n%s", typ, res.HTML)
			}
		})
		t.Run(typ+"-override", func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: typ, ID: "w", Label: "T",
				Style: map[string]any{"backdropBlur": float64(4)}})
			if !strings.Contains(res.HTML, "backdrop-filter:blur(4px);") {
				t.Errorf("%s should honour backdropBlur:\n%s", typ, res.HTML)
			}
		})
		t.Run(typ+"-off", func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: typ, ID: "w", Label: "T",
				Style: map[string]any{"backdropBlur": float64(0)}})
			if strings.Contains(res.HTML, "backdrop-filter:blur") {
				t.Errorf("%s backdropBlur:0 should drop the frost entirely:\n%s", typ, res.HTML)
			}
		})
		t.Run(typ+"-clamped", func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: typ, ID: "w", Label: "T",
				Style: map[string]any{"backdropBlur": float64(500)}})
			if !strings.Contains(res.HTML, "backdrop-filter:blur(120px);") {
				t.Errorf("%s should clamp an absurd radius:\n%s", typ, res.HTML)
			}
		})
		t.Run(typ+"-negative", func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: typ, ID: "w", Label: "T",
				Style: map[string]any{"backdropBlur": float64(-9)}})
			if strings.Contains(res.HTML, "backdrop-filter:blur") {
				t.Errorf("%s should treat a negative radius as off:\n%s", typ, res.HTML)
			}
		})
	}
}

// ---- collapsing large title --------------------------------------------------

// TestLargeTitleCollapses asserts the markup the scroll collapse is built on:
// the sticky compact bar, the marker/classes the shell stylesheet and the app.js
// fallback both key on, and the duplicated title (big + compact) that the two
// cross-fade between.
func TestLargeTitleCollapses(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "largetitle", ID: "lt", Label: "Places",
		Props: map[string]any{"subtitle": "8 nearby"}})
	for _, w := range []string{
		`class="qorm-lt"`, `data-qorm-largetitle`, `class="qorm-lt-bar"`,
		`class="qorm-lt-mini"`, `class="qorm-lt-big"`,
		"position:sticky;top:0;z-index:20;", "border-bottom:.5px solid transparent;",
		"font-size:34px", "Places", "8 nearby",
	} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("collapsing largetitle lacks %q:\n%s", w, res.HTML)
		}
	}
	if n := strings.Count(res.HTML, "Places"); n != 2 {
		t.Errorf("the title should render twice (big + compact), got %d:\n%s", n, res.HTML)
	}
}

// TestLargeTitleCollapsibleFalse pins the opt-out: no sticky bar, no marker and
// exactly one title — the static header this widget used to be.
func TestLargeTitleCollapsibleFalse(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "largetitle", ID: "lt", Label: "Places",
		Props: map[string]any{"collapsible": false}})
	for _, w := range []string{"qorm-lt-mini", "qorm-lt-big", "qorm-lt-bar", "data-qorm-largetitle", "position:sticky"} {
		if strings.Contains(res.HTML, w) {
			t.Errorf("collapsible:false must not emit %q:\n%s", w, res.HTML)
		}
	}
	if n := strings.Count(res.HTML, "Places"); n != 1 {
		t.Errorf("a static largetitle renders one title, got %d:\n%s", n, res.HTML)
	}
	if !strings.Contains(res.HTML, "font-size:34px") {
		t.Errorf("collapsible:false still renders the big title:\n%s", res.HTML)
	}
}

// TestLargeTitleAliasesCollapse guards that the Flutter/Cupertino spellings get
// the same scroll behaviour as the canonical name (aliases are equivalences).
func TestLargeTitleAliasesCollapse(t *testing.T) {
	for _, typ := range []string{"sliverappbar", "cupertinolargetitle"} {
		res := renderWidget(t, &model.Node{Type: typ, ID: "lt", Label: "Big"})
		if !strings.Contains(res.HTML, "data-qorm-largetitle") {
			t.Errorf("%s should collapse like largetitle:\n%s", typ, res.HTML)
		}
	}
}

// TestLargeTitleEscapesTitleTwice is the escaping regression for the duplicated
// title: BOTH copies must be escaped, not just the big one.
func TestLargeTitleEscapesTitleTwice(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "largetitle", ID: "lt", Label: `"><img src=x>`})
	if strings.Contains(res.HTML, "<img src=x>") {
		t.Errorf("largetitle must escape both title copies:\n%s", res.HTML)
	}
	if n := strings.Count(res.HTML, "&lt;img src=x&gt;"); n != 2 {
		t.Errorf("expected 2 escaped titles, got %d:\n%s", n, res.HTML)
	}
}

// ---- draggable sheet ---------------------------------------------------------

func sheetNode(props map[string]any) *model.Node {
	return &model.Node{Type: "sheet", ID: "sh", Props: props,
		Children: []*model.Node{{Type: "text", ID: "k", Text: "body"}}}
}

// TestSheetRenders covers the default panel: the client contract (marker + snap
// ladder + initial stop), the grab row, and the independently scrolling body.
func TestSheetRenders(t *testing.T) {
	res := renderWidget(t, sheetNode(nil))
	for _, w := range []string{
		`class="qorm-dsheet"`, `data-qorm-sheet="sh"`,
		`data-snaps="25,50,90"`, `data-snap="0"`, `data-close-h="-1"`,
		`class="qorm-dsheet-grab"`, `class="qorm-dsheet-body"`,
		`class="qorm-dsheet-scrim"`, "height:25%;", "body",
		`role="dialog"`, `aria-modal="true"`,
	} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("sheet lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestSheetOpenBinding: like every overlay, a falsy `open` renders nothing at
// all (no hidden markup for a screen reader to find), and a truthy one shows.
func TestSheetOpenBinding(t *testing.T) {
	closed := renderWidgetState(t, sheetNode(map[string]any{"open": "{{ state.on }}"}),
		map[string]any{"on": false})
	if strings.Contains(closed.HTML, "qorm-dsheet") {
		t.Errorf("a closed sheet must render nothing:\n%s", closed.HTML)
	}
	open := renderWidgetState(t, sheetNode(map[string]any{"open": "{{ state.on }}"}),
		map[string]any{"on": true})
	if !strings.Contains(open.HTML, "qorm-dsheet") {
		t.Errorf("an open sheet must render:\n%s", open.HTML)
	}
	// A bound `open` also wires the built-in dismiss, so the scrim closes it
	// with no app-authored action.
	if !strings.Contains(open.HTML, `data-close-h="0"`) || !strings.Contains(open.HTML, `onclick="qorm(0)"`) {
		t.Errorf("a bound open should wire the built-in dismiss:\n%s", open.HTML)
	}
}

// TestSheetSnapPoints exercises the ladder parser: percentages, fractions, junk
// values, ordering, and the fallback when nothing usable is left.
func TestSheetSnapPoints(t *testing.T) {
	for _, tc := range []struct {
		name string
		pts  any
		want string
	}{
		{"fractions", []any{0.3, 0.6, 0.92}, `data-snaps="30,60,92"`},
		{"percentages", []any{30.0, 60.0}, `data-snaps="30,60"`},
		{"unsorted", []any{0.9, 0.2}, `data-snaps="20,90"`},
		{"drops-junk", []any{0.0, -1.0, "nope", 0.5}, `data-snaps="50"`},
		{"all-junk-falls-back", []any{0.0, -3.0}, `data-snaps="25,50,90"`},
		{"not-a-list", "0.5", `data-snaps="25,50,90"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, sheetNode(map[string]any{"snapPoints": tc.pts}))
			if !strings.Contains(res.HTML, tc.want) {
				t.Errorf("snapPoints %v should render %s:\n%s", tc.pts, tc.want, res.HTML)
			}
		})
	}
}

// TestSheetSnapPointsBound proves the ladder itself can come from state.
func TestSheetSnapPointsBound(t *testing.T) {
	res := renderWidgetState(t, sheetNode(map[string]any{"snapPoints": "{{ state.pts }}"}),
		map[string]any{"pts": []any{0.4, 0.8}})
	if !strings.Contains(res.HTML, `data-snaps="40,80"`) {
		t.Errorf("a bound snapPoints ladder should resolve:\n%s", res.HTML)
	}
}

// TestSheetInitialSnap: the initial stop selects the server-rendered height, and
// an out-of-range index falls back to the lowest stop rather than panicking.
func TestSheetInitialSnap(t *testing.T) {
	for _, tc := range []struct {
		name               string
		idx                float64
		wantSnap, wantHeit string
	}{
		{"middle", 1, `data-snap="1"`, "height:50%;"},
		{"top", 2, `data-snap="2"`, "height:90%;"},
		{"past-the-end", 7, `data-snap="0"`, "height:25%;"},
		{"negative", -1, `data-snap="0"`, "height:25%;"},
	} {
		t.Run("initial-"+tc.name, func(t *testing.T) {
			res := renderWidget(t, sheetNode(map[string]any{"initialSnap": tc.idx}))
			if !strings.Contains(res.HTML, tc.wantSnap) || !strings.Contains(res.HTML, tc.wantHeit) {
				t.Errorf("initialSnap %g should render %s + %s:\n%s", tc.idx, tc.wantSnap, tc.wantHeit, res.HTML)
			}
		})
	}
}

// TestSheetOnSnap pins the per-stop handler registration: one handler per stop,
// each carrying that stop's index as an ordinary action arg, so the client
// dispatches a plain qorm(h) with no bespoke event plumbing.
func TestSheetOnSnap(t *testing.T) {
	res := renderWidget(t, sheetNode(map[string]any{
		"snapPoints": []any{0.3, 0.7},
		"onSnap":     map[string]any{"name": "setSnap", "args": map[string]any{"who": "sheet"}},
	}))
	if !strings.Contains(res.HTML, `data-snap-h="0,1"`) {
		t.Errorf("onSnap should register one handler per stop:\n%s", res.HTML)
	}
	if len(res.Handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(res.Handlers))
	}
	for i, h := range res.Handlers {
		if h.Name != "setSnap" {
			t.Errorf("handler %d name = %q", i, h.Name)
		}
		if h.Args["who"] != "sheet" {
			t.Errorf("handler %d should keep the author's args: %v", i, h.Args)
		}
	}
	if res.Handlers[0].Args["snap"] != "0" || res.Handlers[1].Args["snap"] != "1" {
		t.Errorf("each stop's handler should carry its own index: %v / %v",
			res.Handlers[0].Args, res.Handlers[1].Args)
	}
}

// TestSheetOnSnapAbsent: with no onSnap there is nothing to dispatch, and the
// attribute is empty rather than carrying stale indexes.
func TestSheetOnSnapAbsent(t *testing.T) {
	res := renderWidget(t, sheetNode(nil))
	if !strings.Contains(res.HTML, `data-snap-h=""`) {
		t.Errorf("a sheet without onSnap should render an empty handler list:\n%s", res.HTML)
	}
}

// TestSheetOnClose: an explicit onClose wins over the built-in dismiss.
func TestSheetOnClose(t *testing.T) {
	res := renderWidgetState(t, sheetNode(map[string]any{
		"open":    "{{ state.on }}",
		"onClose": map[string]any{"name": "closeIt"},
	}), map[string]any{"on": true})
	if len(res.Handlers) != 1 || res.Handlers[0].Name != "closeIt" {
		t.Fatalf("onClose should be the registered close handler: %v", res.Handlers)
	}
	if !strings.Contains(res.HTML, `data-close-h="0"`) {
		t.Errorf("the close handler should reach the client:\n%s", res.HTML)
	}
}

// TestSheetOptOuts covers the two chrome switches and the title.
func TestSheetOptOuts(t *testing.T) {
	t.Run("no-backdrop", func(t *testing.T) {
		res := renderWidget(t, sheetNode(map[string]any{"backdrop": false}))
		if strings.Contains(res.HTML, "qorm-dsheet-scrim") {
			t.Errorf("backdrop:false should drop the scrim:\n%s", res.HTML)
		}
	})
	t.Run("no-handle", func(t *testing.T) {
		res := renderWidget(t, sheetNode(map[string]any{"handle": false}))
		if strings.Contains(res.HTML, "width:36px;height:5px") {
			t.Errorf("handle:false should drop the grabber pill:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "qorm-dsheet-grab") {
			t.Errorf("the drag row itself must survive handle:false:\n%s", res.HTML)
		}
	})
	t.Run("title", func(t *testing.T) {
		res := renderWidget(t, sheetNode(map[string]any{"title": "Filters"}))
		if !strings.Contains(res.HTML, "Filters") {
			t.Errorf("sheet should render its title:\n%s", res.HTML)
		}
	})
}

// TestSheetEscaping is the injection regression for every author-controlled
// value the sheet emits.
func TestSheetEscaping(t *testing.T) {
	xss := `"><script>alert(1)</script>`
	res := renderWidget(t, &model.Node{Type: "sheet", ID: xss, Props: map[string]any{"title": xss}})
	if strings.Contains(res.HTML, "<script>alert(1)</script>") {
		t.Errorf("sheet must escape its id/title:\n%s", res.HTML)
	}
}

// TestSheetAliases guards the Flutter spellings.
func TestSheetAliases(t *testing.T) {
	for _, typ := range []string{"bottomsheet", "draggablesheet", "draggablescrollablesheet", "modalbottomsheet"} {
		res := renderWidget(t, &model.Node{Type: typ, ID: "sh"})
		if !strings.Contains(res.HTML, "qorm-dsheet") {
			t.Errorf("%s should render the draggable sheet:\n%s", typ, res.HTML)
		}
	}
}
