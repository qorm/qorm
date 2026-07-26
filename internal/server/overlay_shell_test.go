package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func shellRT() *runtime.Runtime {
	return runtime.New(&model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	})
}

// TestShellBackdropRules pins the frosted-glass half of the `backdropBlur` /
// `backdropTint` style keys that lives in the shell stylesheet rather than in
// the renderer: a SOLID base rule, the blur only inside an @supports guard (so
// a browser without backdrop-filter never renders an unreadable see-through
// panel), and the -webkit- prefixed property alongside the standard one.
func TestShellBackdropRules(t *testing.T) {
	h := Page(shellRT(), "x", 0)
	for _, want := range []string{
		`[style*="--qorm-bdb"] { background:var(--surface); }`,
		"@supports ((-webkit-backdrop-filter:blur(1px)) or (backdrop-filter:blur(1px)))",
		"-webkit-backdrop-filter:blur(var(--qorm-bdb))",
		"backdrop-filter:blur(var(--qorm-bdb))",
		"var(--qorm-bdt,",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("shell stylesheet lacks the backdrop rule %q", want)
		}
	}
	// The solid fallback must come BEFORE the @supports block, or the cascade
	// would let it overwrite the blurred rule it is meant to back up.
	base := strings.Index(h, `[style*="--qorm-bdb"] { background:var(--surface); }`)
	sup := strings.Index(h, "@supports ((-webkit-backdrop-filter")
	if base < 0 || sup < 0 || base > sup {
		t.Errorf("the solid fallback must precede the @supports block (base=%d supports=%d)", base, sup)
	}
	// Neither rule may be !important: an author's own inline background wins.
	if i := strings.Index(h, `[style*="--qorm-bdb"]`); i >= 0 {
		block := h[i:sup]
		if strings.Contains(block, "!important") {
			t.Error("the backdrop fallback must not be !important — an inline background has to win")
		}
	}
}

// TestShellLargeTitleRules pins the pure-CSS collapse: a scroll-driven
// cross-fade behind an @supports guard, the .qorm-lt-stuck class the app.js
// fallback toggles for browsers without it, and both keyframes.
func TestShellLargeTitleRules(t *testing.T) {
	h := Page(shellRT(), "x", 0)
	for _, want := range []string{
		".qorm-lt-mini { opacity:0;",
		".qorm-lt-stuck .qorm-lt-mini { opacity:1; }",
		".qorm-lt-stuck .qorm-lt-big { opacity:0;",
		".qorm-lt-stuck .qorm-lt-bar { border-bottom-color:var(--sep); }",
		"@keyframes qorm-lt-collapse",
		"@keyframes qorm-lt-reveal",
		"@keyframes qorm-lt-hairline",
		"@supports (animation-timeline:view()) and (timeline-scope:--qorm-lt)",
		".qorm-lt { timeline-scope:--qorm-lt; }",
		"view-timeline-name:--qorm-lt;",
		"animation-timeline:--qorm-lt;",
		"animation-range:exit 0% exit 100%;",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("shell stylesheet lacks the large-title rule %q", want)
		}
	}
	// The grab row must opt out of browser touch panning, or the gesture never
	// gets a pointermove on a phone.
	if !strings.Contains(h, ".qorm-dsheet-grab { touch-action:none;") {
		t.Error("shell stylesheet lacks the sheet grab-row touch-action rule")
	}
}

// TestClientScriptOverlayWiring is the server-side assertion for the client
// behaviour these two widgets need: the gesture/fallback entry points exist,
// they are installed at load, and — the part a re-render could break — both are
// re-run from the morph path, which is what makes them idempotent.
func TestClientScriptOverlayWiring(t *testing.T) {
	js := qormAppJS(7, "tok")
	for _, want := range []string{
		"function qormSheetInit(", "function qormSheetSync(", "function qormSheetSet(",
		"function qormLargeTitleInit(", "function qormLargeTitleSync(",
		"function qormOverlayInit(",
		"window.__qormSheets",
		"window.__qormSheetReady", "window.__qormLTReady",
		"data-qorm-sheet", "data-qorm-largetitle",
		"qorm-dsheet-grab", "qorm-lt-stuck", "qorm-lt-big",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	// Installed once at load…
	if !strings.Contains(js, "qormTimersSync(); qormOverlayInit();") {
		t.Error("app.js must install the overlay wiring on load")
	}
	// …and reconciled after every DOM morph, exactly like the timer registry.
	morph := js[strings.Index(js, "function qormMorphInto("):]
	morph = morph[:strings.Index(morph, "\n}")]
	for _, want := range []string{"qormSheetSync();", "qormLargeTitleSync();"} {
		if !strings.Contains(morph, want) {
			t.Errorf("qormMorphInto must call %s so a re-render stays idempotent", want)
		}
	}
	// The gesture must read its parameters from the live DOM at event time
	// rather than closing over a handler index a re-render can renumber.
	if !strings.Contains(js, `p.getAttribute('data-snap-h')`) {
		t.Error("the sheet gesture must re-read data-snap-h at event time")
	}
}
