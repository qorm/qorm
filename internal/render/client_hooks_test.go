package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// The renderer half of the client-side interactions that CSS cannot express:
// tab panel swiping, exclusive accordion panels, and carousel autoplay +
// indicator dots. The renderer's whole contribution is a data-attribute mount
// point per feature — the gesture, the clock and the active-dot derivation live
// in internal/server/app.js — so what these tests pin is exactly that contract:
//
//  1. the marker appears when, and only when, the prop asks for it;
//  2. an app that does not ask keeps its byte-identical old markup, which is
//     what makes every one of these props safe to add to a shipped app;
//  3. the marker carries the parameters the client re-reads at event time
//     (panel count, interval), never a handler index — those the client gets by
//     synthesizing the tab's own tap, so a re-render cannot renumber them out
//     from under it.
//
// The matching client-side assertions (guards, reconcile-from-morph, event-time
// re-reads) are in internal/server/client_frame_test.go.

func tabsWithProps(props map[string]any) *model.Node {
	return &model.Node{Type: "tabs", ID: "tb", Props: props, Children: []*model.Node{
		{Type: "text", ID: "p1", Text: "One"},
		{Type: "text", ID: "p2", Text: "Two"},
	}}
}

// TestTabsSwipeMarker: `swipe` mounts the gesture and nothing else, in BOTH
// modes — the uncontrolled buttons and the controlled radio labels are
// untouched, because the swipe activates a tab by clicking it rather than by
// carrying its own handler table.
func TestTabsSwipeMarker(t *testing.T) {
	off := renderWidget(t, tabsWithProps(map[string]any{"tabs": []any{"One", "Two"}}))
	if strings.Contains(off.HTML, "data-qorm-tabs") {
		t.Errorf("tabs without `swipe` must not carry the gesture marker:\n%s", off.HTML)
	}

	on := renderWidget(t, tabsWithProps(map[string]any{"tabs": []any{"One", "Two"}, "swipe": true}))
	if !strings.Contains(on.HTML, `data-qorm-tabs="2"`) {
		t.Errorf("swipe tabs must mount data-qorm-tabs with the panel count:\n%s", on.HTML)
	}
	// Uncontrolled: still the plain client-side switch, no extra handlers.
	if !strings.Contains(on.HTML, `onclick="qormTab(this)"`) {
		t.Errorf("swipe must not change how an uncontrolled tab switches:\n%s", on.HTML)
	}

	ctl := renderWidgetState(t, tabsWithProps(map[string]any{
		"tabs": []any{"One", "Two"}, "swipe": true, "active": "{{state.tab}}",
	}), map[string]any{"tab": 1.0})
	for _, want := range []string{`data-qorm-tabs="2"`, `<label class="qorm-tab`, `type="radio"`, `qorm-tab-active`} {
		if !strings.Contains(ctl.HTML, want) {
			t.Errorf("controlled+swipe tabs lack %q:\n%s", want, ctl.HTML)
		}
	}
	// The swipe target is the radio a tap would check — no swipe-only handler.
	if strings.Contains(ctl.HTML, "data-swipe-h") {
		t.Error("the swipe must reuse the tab's own tap, not register handlers")
	}
}

// TestAccordionSingleMarker: `single` is opt-in, so today's independently
// toggling accordions keep behaving as they do; with it, the exclusive-panel
// marker the client reads at click time appears on the root.
func TestAccordionSingleMarker(t *testing.T) {
	kids := []*model.Node{
		{Type: "column", ID: "s1", Props: map[string]any{"title": "A"}, Children: []*model.Node{{Type: "text", ID: "a", Text: "a"}}},
		{Type: "column", ID: "s2", Props: map[string]any{"title": "B"}, Children: []*model.Node{{Type: "text", ID: "b", Text: "b"}}},
	}
	def := renderWidget(t, &model.Node{Type: "accordion", ID: "ac", Children: kids})
	if strings.Contains(def.HTML, "data-qorm-acc") {
		t.Errorf("a plain accordion must keep independent panels (no marker):\n%s", def.HTML)
	}
	one := renderWidget(t, &model.Node{Type: "accordion", ID: "ac", Props: map[string]any{"single": true}, Children: kids})
	if !strings.Contains(one.HTML, `data-qorm-acc="single"`) {
		t.Errorf("`single` accordion must mount the exclusive marker:\n%s", one.HTML)
	}
	// The panels themselves are unchanged: the mode is a root attribute, so the
	// first panel still renders open and the rest closed.
	if !strings.Contains(one.HTML, `class="qorm-acc-panel" style="display:block`) ||
		!strings.Contains(one.HTML, `class="qorm-acc-panel" style="display:none`) {
		t.Errorf("`single` must not change which panel starts open:\n%s", one.HTML)
	}
}

// TestCarouselAutoplayAndDots: both props are opt-in and independent; the
// interval travels on the marker (the client re-reads it every reconcile) and
// the dot row is a SIBLING of the scroll track, with the first dot current
// because that is the slide a fresh render shows.
func TestCarouselAutoplayAndDots(t *testing.T) {
	kids := []*model.Node{
		{Type: "text", ID: "c1", Text: "p1"},
		{Type: "text", ID: "c2", Text: "p2"},
	}
	plain := renderWidget(t, &model.Node{Type: "carousel", ID: "cr", Children: kids})
	for _, banned := range []string{"data-qorm-carousel", "data-qorm-dots", "data-qorm-dot="} {
		if strings.Contains(plain.HTML, banned) {
			t.Errorf("a plain carousel must not carry %q:\n%s", banned, plain.HTML)
		}
	}

	auto := renderWidget(t, &model.Node{Type: "carousel", ID: "cr",
		Props: map[string]any{"autoplay": 3000.0}, Children: kids})
	if !strings.Contains(auto.HTML, `data-qorm-carousel="3000"`) {
		t.Errorf("autoplay must mount the interval on the track:\n%s", auto.HTML)
	}
	if strings.Contains(auto.HTML, "data-qorm-dots") {
		t.Error("autoplay alone must not emit indicator dots")
	}

	dots := renderWidget(t, &model.Node{Type: "carousel", ID: "cr",
		Props: map[string]any{"indicators": true}, Children: kids})
	for _, want := range []string{
		`<div class="qorm-carousel-dots" data-qorm-dots=""`,
		`<button data-qorm-dot="0" aria-current="true" aria-label="Slide 1"`,
		`<button data-qorm-dot="1" aria-current="false" aria-label="Slide 2"`,
		"background:var(--accent);", // the current dot
		"background:var(--sep);",    // the rest
	} {
		if !strings.Contains(dots.HTML, want) {
			t.Errorf("carousel indicators lack %q:\n%s", want, dots.HTML)
		}
	}
	// The row must FOLLOW the track: the client finds one from the other by
	// sibling order, and dots inside the scroller would scroll away with it.
	if strings.Index(dots.HTML, `data-qorm-dots`) < strings.Index(dots.HTML, `id="cr"`) {
		t.Error("the indicator row must be rendered after the scroll track")
	}
	if strings.Contains(dots.HTML, `data-qorm-dots`) &&
		strings.Contains(dots.HTML[strings.Index(dots.HTML, `data-qorm-dots`):], `scroll-snap-align`) {
		t.Error("the indicator row must be a sibling of the track, not inside it")
	}

	both := renderWidget(t, &model.Node{Type: "carousel", ID: "cr",
		Props: map[string]any{"autoplay": 2500.0, "indicators": true}, Children: kids})
	if !strings.Contains(both.HTML, `data-qorm-carousel="2500"`) || !strings.Contains(both.HTML, `data-qorm-dots=""`) {
		t.Errorf("autoplay and indicators must compose:\n%s", both.HTML)
	}
	// An empty carousel emits an empty (not broken) dot row.
	empty := renderWidget(t, &model.Node{Type: "carousel", ID: "cr", Props: map[string]any{"indicators": true}})
	if !strings.Contains(empty.HTML, `padding:8px 0;"></div>`) {
		t.Errorf("an empty carousel must emit an empty dot row:\n%s", empty.HTML)
	}
}
