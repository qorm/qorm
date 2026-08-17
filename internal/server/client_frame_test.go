package server

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/runtime"
)

// Server-side assertions for the parts of the client script no Go code can
// execute: the host frame sink the WASM builds push into, and the three
// gestures/clocks that back the tabs / accordion / carousel props.
//
// They are string assertions on purpose, and they are not decoration: each one
// pins a property whose failure mode is SILENT. A missing qormApplyFrame turns
// an async http step into a UI that never updates; a missing reconcile call
// from qormMorphInto turns a re-render into duplicated timers or a stale
// registry; a handler index captured in a closure instead of re-read at event
// time dispatches the WRONG action after a re-render, which no test that only
// checks markup would ever notice.
//
// jsBody extracts one function's body from the client script so an assertion
// can be about WHERE a call is made, not merely that the file contains it
// somewhere (the pattern TestClientScriptOverlayWiring established).
func jsBody(t *testing.T, js, fn string) string {
	t.Helper()
	i := strings.Index(js, "function "+fn+"(")
	if i < 0 {
		t.Fatalf("app.js has no function %s", fn)
	}
	rest := js[i:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("could not find the end of %s", fn)
	}
	return rest[:end]
}

// TestClientScriptFrameSink: the WASM runtime publishes frames by calling
// window.qormApplyFrame(res) — an intermediate frame from a `render` step, or
// an http.* completion that came back on a background goroutine. Installing the
// runtime's Async sink without this entry point would trade the js/wasm
// deadlock for a UI that silently never updates, so the two ship together.
func TestClientScriptFrameSink(t *testing.T) {
	js := qormAppJS(7, "tok", "null")
	if !strings.Contains(js, "function qormApplyFrame(") {
		t.Fatal("app.js must define qormApplyFrame — the host frame sink for the WASM runtimes")
	}
	body := jsBody(t, js, "qormApplyFrame")
	for _, want := range []string{
		"typeof res.html!=='string'",    // ignore an error/empty result, never throw back into wasm
		"qormTheme(res.theme)",          // a frame may change the theme
		"typeof qormDir==='function'",   // qormDir exists only in the offline boot
		"qormMorphInto(root, res.html)", // the one path that re-runs every reconcile pass
	} {
		if !strings.Contains(body, want) {
			t.Errorf("qormApplyFrame must contain %q:\n%s", want, body)
		}
	}
}

// TestOfflineHTMLCarriesFrameSink: a packaged app is the host that NEEDS the
// sink most (it runs the WASM runtime with no server and no SSE), and it gets
// it from the shared app.js rather than a second copy in the boot string — so
// both boot variants have it, including the OTA one, whose runtime is swapped
// under the page after an update.
func TestOfflineHTMLCarriesFrameSink(t *testing.T) {
	for _, tc := range []struct {
		name string
		upd  *UpdateConfig
	}{
		{"plain", nil},
		{"ota", &UpdateConfig{URL: "https://u.example.com", App: "a", Trust: "cHVia2V5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html, err := OfflineHTML(runtime.New(offlineTestApp()), `{"entry":"main"}`, tc.upd)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, "function qormApplyFrame(") {
				t.Error("the offline package must carry the host frame sink")
			}
			// qormApplyFrame calls qormDir, which the boot defines — the guard in
			// the sink keeps the SERVER page (which has no qormDir) working too.
			if !strings.Contains(html, "function qormDir(d)") {
				t.Error("the offline boot must still define qormDir")
			}
			if !strings.Contains(html, "qormEvent(h, JSON.stringify(inputs))") {
				t.Error("the offline driver must still dispatch in-process")
			}
		})
	}
}

// TestClientScriptTabsWiring pins the two tab behaviours CSS cannot do, and the
// idempotence discipline both follow.
func TestClientScriptTabsWiring(t *testing.T) {
	js := qormAppJS(7, "tok", "null")
	for _, want := range []string{
		"function qormTabRevealBar(", "function qormTabReveal(",
		"function qormTabSwipeInit(", "function qormTabActivate(", "function qormTabActive(",
		"function qormTabScrollsX(",
		"window.__qormTabSwipeReady", // installed once, so a morph cannot stack listeners
		"data-qorm-tabs",             // the renderer's opt-in marker
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	// ONE delegated document listener, not one per widget.
	swipe := jsBody(t, js, "qormTabSwipeInit")
	if !strings.Contains(swipe, "document.addEventListener('pointerdown'") {
		t.Error("the tab swipe must be a single delegated document listener")
	}
	if !strings.Contains(swipe, "closest('[data-qorm-tabs]')") {
		t.Error("the tab swipe must resolve its widget from the live DOM at event time")
	}
	if !strings.Contains(swipe, "qormTabActive(root)") {
		t.Error("the swipe must re-read the active index at event time, not capture it")
	}
	// The activation path must reuse the tab's own tap (which is what makes the
	// controlled/uncontrolled/onChange forms all work) rather than a captured
	// handler index a re-render could renumber.
	act := jsBody(t, js, "qormTabActivate")
	if !strings.Contains(act, "input[type=radio]") || !strings.Contains(act, ".click()") {
		t.Errorf("qormTabActivate must synthesize the tab's own tap:\n%s", act)
	}
	if strings.Contains(act, "qorm(") {
		t.Error("qormTabActivate must not dispatch a handler index of its own")
	}
	// Reveal runs after every morph (the active tab can change without a click)
	// and after a client-side switch.
	morph := jsBody(t, js, "qormMorphInto")
	if !strings.Contains(morph, "qormTabReveal();") {
		t.Error("qormMorphInto must re-run qormTabReveal so a re-render keeps the active tab in view")
	}
	if !strings.Contains(jsBody(t, js, "qormTab"), "qormTabRevealBar(bar)") {
		t.Error("a client-side tab switch must reveal the newly active tab")
	}
	if !strings.Contains(js, "qormTabSwipeInit();") {
		t.Error("the tab swipe must be installed at load (qormOverlayInit)")
	}
}

// TestClientScriptAccordionSingle: exclusive panels are opt-in and the mode is
// read from the live DOM at click time, so the default (independent toggles)
// cannot regress and a re-render cannot leave a stale mode behind.
func TestClientScriptAccordionSingle(t *testing.T) {
	acc := jsBody(t, qormAppJS(7, "tok", "null"), "qormAcc")
	if !strings.Contains(acc, `closest('[data-qorm-acc="single"]')`) {
		t.Errorf("qormAcc must read the exclusive mode off the live DOM:\n%s", acc)
	}
	if !strings.Contains(acc, ".qorm-acc-panel") {
		t.Errorf("qormAcc must close the sibling panels in single mode:\n%s", acc)
	}
	// Independent toggling stays the default: closing others is gated on BOTH
	// the marker and the panel actually opening.
	if !strings.Contains(acc, "if(open && root)") {
		t.Errorf("closing siblings must be gated on the marker AND on opening:\n%s", acc)
	}
}

// TestClientScriptCarouselWiring: autoplay is a registry with exactly the
// timer registry's reconcile contract, and the indicator dots hold no state at
// all — the active one is derived from the live scroll position.
func TestClientScriptCarouselWiring(t *testing.T) {
	js := qormAppJS(7, "tok", "null")
	for _, want := range []string{
		"function qormCarouselSync(", "function qormCarouselTick(", "function qormCarouselDots(",
		"function qormCarouselGo(", "function qormCarouselIndex(", "function qormCarouselInit(",
		"window.__qormCarousels",     // the one piece of client-owned state
		"window.__qormCarouselReady", // installed once
		"data-qorm-carousel", "data-qorm-dots", "data-qorm-dot",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	sync := jsBody(t, js, "qormCarouselSync")
	for _, want := range []string{
		"document.contains(",                 // forget tracks that left the DOM
		"clearInterval(",                     // …and stop their clocks
		"getAttribute('data-qorm-carousel')", // the interval is re-read, never captured
		"e.ms!==ms",                          // an unchanged interval is never rescheduled
	} {
		if !strings.Contains(sync, want) {
			t.Errorf("qormCarouselSync must contain %q:\n%s", want, sync)
		}
	}
	morph := jsBody(t, js, "qormMorphInto")
	if !strings.Contains(morph, "qormCarouselSync();") {
		t.Error("qormMorphInto must call qormCarouselSync so a re-render stays idempotent")
	}
	tick := jsBody(t, js, "qormCarouselTick")
	for _, want := range []string{"document.hidden", "matches(':hover')", "document.contains("} {
		if !strings.Contains(tick, want) {
			t.Errorf("autoplay must skip a tick when %q says so:\n%s", want, tick)
		}
	}
	dots := jsBody(t, js, "qormCarouselDots")
	if !strings.Contains(dots, "qormCarouselIndex(el)") || !strings.Contains(dots, "aria-current") {
		t.Errorf("the active dot must be derived from the live scroll position:\n%s", dots)
	}
	if !strings.Contains(js, "qormCarouselInit();") {
		t.Error("the carousel wiring must be installed at load (qormOverlayInit)")
	}
	// Same floor as the declarative timer widget, so a 1ms autoplay cannot spin
	// the page.
	if !strings.Contains(sync, "ms<250") {
		t.Error("autoplay must be floored at 250ms, like the timer widget")
	}
}
