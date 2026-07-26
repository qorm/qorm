package server

import (
	"strings"
	"testing"
)

// Server-side assertions for the client half of the interaction batch: windowed
// lists, input debounce, and the three native-validation behaviours. Same
// contract as client_frame_test.go — these pin properties whose failure mode is
// SILENT (a listener stacked per re-render, a handler index captured instead of
// re-read, a reconcile pass that qormMorphInto forgot to call), which no test
// that only looks at markup would ever notice.

// TestClientScriptWindowedList: the client's whole job is measure-and-report,
// over the ORDINARY event channel, at most once per frame.
func TestClientScriptWindowedList(t *testing.T) {
	js := qormAppJS(7, "tok")
	for _, want := range []string{
		"function qormVListMetrics(", "function qormVListReport(",
		"function qormVListSync(", "function qormVListInit(", "function qormVListScroll(",
		"window.__qormVLists",     // the one piece of client-owned state
		"window.__qormVListReady", // installed once
		"data-qorm-vlist",         // the renderer's opt-in marker
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	init := jsBody(t, js, "qormVListInit")
	if !strings.Contains(init, "document.addEventListener('scroll', qormVListScroll, true)") {
		t.Error("the scroll report must be ONE delegated capturing document listener")
	}
	if !strings.Contains(init, "window.addEventListener('resize'") {
		t.Error("a resized port needs a new window too")
	}
	scroll := jsBody(t, js, "qormVListScroll")
	if !strings.Contains(scroll, "window.__qormVListRAF") || !strings.Contains(scroll, "requestAnimationFrame") {
		t.Errorf("the scroll listener must be rAF-throttled:\n%s", scroll)
	}
	report := jsBody(t, js, "qormVListReport")
	for _, want := range []string{
		"getAttribute('data-item-h')",      // every parameter is re-read at event time,
		"getAttribute('data-qorm-vstart')", // never captured
		"getAttribute('data-qorm-vcount')",
		"getAttribute('data-qorm-vtotal')",
		"input[data-qorm-vscroll]", // the report rides a [data-state] input…
		"qorm(-1)",                 // …through the ordinary state-sync dispatch
		"now-last1.t<100",          // and no more than one round trip per 100ms
	} {
		if !strings.Contains(report, want) {
			t.Errorf("qormVListReport must contain %q:\n%s", want, report)
		}
	}
	// The whole point: scrolling INSIDE the rendered window costs no round trip,
	// and a window that is covered clears the in-flight guard.
	if !strings.Contains(report, "first>=at && last<at+count") || !strings.Contains(report, "delete window.__qormVLists[key]") {
		t.Errorf("a covered window must report nothing and clear its guard:\n%s", report)
	}
	if !strings.Contains(js, "qormVListInit();") {
		t.Error("the windowed-list wiring must be installed at load (qormOverlayInit)")
	}
	morph := jsBody(t, js, "qormMorphInto")
	if !strings.Contains(morph, "qormVListSync();") {
		t.Error("qormMorphInto must re-check the window: a fling can outrun a frame")
	}
	sync := jsBody(t, js, "qormVListSync")
	if !strings.Contains(sync, "if(!live[k]) delete window.__qormVLists[k]") {
		t.Errorf("qormVListSync must forget lists that left the DOM:\n%s", sync)
	}
	// The three scroll-port cases: own scroller, scrolling ancestor, page.
	metrics := jsBody(t, js, "qormVListMetrics")
	for _, want := range []string{"el.scrollTop", "qormScrollParent(el)", "window.innerHeight"} {
		if !strings.Contains(metrics, want) {
			t.Errorf("qormVListMetrics must handle %q:\n%s", want, metrics)
		}
	}
}

// TestClientScriptInputDebounce: one delegated listener, one timer per live
// control, the handler index re-read when it fires, and a morph that cannot
// strand a timer on a field that is gone.
func TestClientScriptInputDebounce(t *testing.T) {
	js := qormAppJS(7, "tok")
	for _, want := range []string{
		"function qormDebounceArm(", "function qormDebounceFire(", "function qormDebounceClear(",
		"function qormDebounceSync(", "function qormDebounceInit(", "function qormDebouncePending(",
		"window.__qormDebounces", "window.__qormDebounceReady",
		"data-qorm-debounce",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	init := jsBody(t, js, "qormDebounceInit")
	if !strings.Contains(init, "document.addEventListener('input'") {
		t.Error("the debounce must be a single delegated document listener")
	}
	if !strings.Contains(init, "document.addEventListener('focusout'") {
		t.Error("leaving the field must flush a pending timer, or keystrokes are lost")
	}
	fire := jsBody(t, js, "qormDebounceFire")
	if !strings.Contains(fire, "getAttribute('data-qorm-debounce-h')") {
		t.Errorf("the handler index must be re-read when the timer fires:\n%s", fire)
	}
	if !strings.Contains(fire, "document.contains(el)") {
		t.Errorf("a field that left the DOM must not dispatch:\n%s", fire)
	}
	arm := jsBody(t, js, "qormDebounceArm")
	if !strings.Contains(arm, "qormDebounceClear(el)") || !strings.Contains(arm, "setTimeout(") {
		t.Errorf("every keystroke must restart the timer:\n%s", arm)
	}
	if !strings.Contains(arm, "getAttribute('data-qorm-debounce')") {
		t.Errorf("the interval must be re-read from the live DOM:\n%s", arm)
	}
	sync := jsBody(t, js, "qormDebounceSync")
	if !strings.Contains(sync, "document.contains(") || !strings.Contains(sync, "clearTimeout(") {
		t.Errorf("qormDebounceSync must cancel timers of controls that left the DOM:\n%s", sync)
	}
	if !strings.Contains(jsBody(t, js, "qormMorphInto"), "qormDebounceSync();") {
		t.Error("qormMorphInto must call qormDebounceSync so a re-render stays idempotent")
	}
	if !strings.Contains(js, "qormDebounceInit();") {
		t.Error("the debounce wiring must be installed at load (qormOverlayInit)")
	}
}

// TestClientScriptValidity: the custom required message must never pin a field
// invalid on its own, the first invalid field is brought on screen, and Enter
// presses the button the app named.
func TestClientScriptValidity(t *testing.T) {
	js := qormAppJS(7, "tok")
	for _, want := range []string{
		"function qormValidity(", "function qormValiditySync(", "function qormValidityInit(",
		"window.__qormValidityReady", "data-qorm-error", "data-qorm-enter",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	v := jsBody(t, js, "qormValidity")
	if !strings.Contains(v, "setCustomValidity('')") {
		t.Errorf("the native verdict must be recomputed before a custom message:\n%s", v)
	}
	if !strings.Contains(v, "validity.valueMissing") {
		t.Errorf("the custom message must apply ONLY to a missing value, or the field is invalid forever:\n%s", v)
	}
	if !strings.Contains(jsBody(t, js, "qormMorphInto"), "qormValiditySync();") {
		t.Error("qormMorphInto must re-apply custom validity: a morph resets attributes")
	}
	init := jsBody(t, js, "qormValidityInit")
	// `invalid` does not bubble — it has to be caught in the capture phase.
	if !strings.Contains(init, "document.addEventListener('invalid'") || !strings.Contains(init, "}, true);") {
		t.Errorf("the invalid listener must be delegated in the CAPTURE phase:\n%s", init)
	}
	for _, want := range []string{"__qormInvalidSeen", "scrollIntoView", "el.focus"} {
		if !strings.Contains(init, want) {
			t.Errorf("the first invalid field must be brought on screen (%q):\n%s", want, init)
		}
	}
	for _, want := range []string{
		"e.key!=='Enter'", "t.tagName==='TEXTAREA'", // Enter stays a newline in a textarea
		"querySelector('[data-qorm-enter]')", // re-read from the live DOM at keypress time
		"btn.click()",                        // the button's OWN wiring, gate included
	} {
		if !strings.Contains(init, want) {
			t.Errorf("Enter-to-submit must contain %q:\n%s", want, init)
		}
	}
	if !strings.Contains(js, "qormValidityInit();") {
		t.Error("the validation wiring must be installed at load (qormOverlayInit)")
	}
}
