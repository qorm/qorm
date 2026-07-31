package server

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// shellCSS renders the page shell for a trivial app and returns its <style>
// block — the half of the pseudo-state feature that lives here. The renderer
// only publishes CSS custom properties (internal/render/render_style.go
// pseudoStateCSS); without the matching fixed rules below, an author's
// hover/pressed/focus/disabled declaration is inert, and nothing else in the
// codebase would notice.
func shellCSS(t *testing.T) string {
	t.Helper()
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	page := Page(runtime.New(app), "", 1)
	i := strings.Index(page, "<style>")
	j := strings.Index(page, "</style>")
	if i < 0 || j < 0 || j < i {
		t.Fatalf("page shell has no <style> block")
	}
	return page[i:j]
}

// TestShellPseudoStateRules pins the consuming half of the pseudo-state
// contract: for each variable the renderer can emit there must be a rule that
// (a) selects on that variable's presence in the style attribute and (b) binds
// it to the right pseudo-class. The variable names are the API between the two
// files, so a rename on either side is caught here rather than by a human
// noticing a dead hover state.
func TestShellPseudoStateRules(t *testing.T) {
	css := shellCSS(t)
	for _, want := range []string{
		`[style*="--qorm-hov-bg"]:hover { background:var(--qorm-hov-bg) !important; }`,
		`[style*="--qorm-hov-fg"]:hover { color:var(--qorm-hov-fg) !important; }`,
		`[style*="--qorm-hov-op"]:hover { opacity:var(--qorm-hov-op) !important; }`,
		`[style*="--qorm-prs-sc"]:active { transform:scale(var(--qorm-prs-sc)) !important; }`,
		`[style*="--qorm-prs-op"]:active { opacity:var(--qorm-prs-op) !important; }`,
		`[style*="--qorm-foc-bc"]:focus-within {`,
		`[style*="--qorm-foc-bc"]:focus-visible { outline:2px solid var(--qorm-foc-bc) !important; outline-offset:2px; }`,
		`[style*="--qorm-dis"] { opacity:var(--qorm-dop,.4) !important;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("shell stylesheet lacks the pseudo-state rule:\n  %s", want)
		}
	}
	// The focus ring must actually paint: border-color alone is invisible on a
	// node without a border, so the rule also draws an outline.
	if !strings.Contains(css, "outline:2px solid var(--qorm-foc-bc);") {
		t.Error("focus rule should draw a visible ring, not only recolor a border")
	}
	// Disabled is a state, not a tint: it must also stop interaction.
	if !strings.Contains(css, "pointer-events:none !important; cursor:not-allowed !important;") {
		t.Error("disabled rule should block pointer events and show the not-allowed cursor")
	}
}

// TestShellPseudoStateHoverIsPointerOnly: a :hover style that sticks after a tap
// is the classic touch-device bug, so the hover rules — and only the hover
// rules — must sit inside the shell's @media (hover:hover) block. :active and
// :focus-within are correct on touch and must stay outside it.
func TestShellPseudoStateHoverIsPointerOnly(t *testing.T) {
	css := shellCSS(t)
	block := hoverMediaBlock(t, css, `[style*="--qorm-hov-bg"]:hover`)
	for _, v := range []string{"--qorm-hov-bg", "--qorm-hov-fg", "--qorm-hov-op"} {
		if !strings.Contains(block, `[style*="`+v+`"]:hover`) {
			t.Errorf("the %s hover rule must live inside @media (hover:hover)", v)
		}
	}
	for _, rule := range []string{
		`[style*="--qorm-prs-sc"]:active`,
		`[style*="--qorm-prs-op"]:active`,
		`[style*="--qorm-foc-bc"]:focus-within`,
		`[style*="--qorm-foc-bc"]:focus-visible`,
		`[style*="--qorm-dis"] {`,
	} {
		if !strings.Contains(css, rule) {
			t.Fatalf("no rule for %s", rule)
		}
		if strings.Contains(block, rule) {
			t.Errorf("%s works on touch and must NOT be gated behind @media (hover:hover)", rule)
		}
	}
}

// hoverMediaBlock returns the body of the @media (hover:hover) block that
// contains needle, located by brace matching (the shell nests media queries, so
// a textual "next closing line" search would be wrong).
func hoverMediaBlock(t *testing.T, css, needle string) string {
	t.Helper()
	at := strings.Index(css, needle)
	if at < 0 {
		t.Fatalf("shell has no rule %q", needle)
	}
	start := strings.LastIndex(css[:at], "@media (hover:hover) {")
	if start < 0 {
		t.Fatalf("%q is not inside an @media (hover:hover) block", needle)
	}
	depth := 0
	for i := start; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if i < at {
					t.Fatalf("%q is not inside an @media (hover:hover) block", needle)
				}
				return css[start : i+1]
			}
		}
	}
	t.Fatalf("unterminated @media (hover:hover) block")
	return ""
}

// TestShellMotionTokens: every built-in palette must publish the shared motion
// token vocabulary (--qorm-motion-*) so animatedContainer/animatedOpacity can
// reference skin-level durations and easings, matching the themes/*.json
// motion section the canvas backend consumes.
func TestShellMotionTokens(t *testing.T) {
	css := shellCSS(t)
	for _, want := range []string{
		"--qorm-motion-fast:120ms;",
		"--qorm-motion-normal:250ms;",
		"--qorm-motion-slow:400ms;",
		"--qorm-motion-standard:cubic-bezier(.4,0,.2,1);",
		"--qorm-motion-emphasized:cubic-bezier(.2,0,0,1);",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("shell theme tokens lack the motion variable %q", want)
		}
	}
}

// TestShellPseudoStateTransition: state changes should ease rather than snap,
// and one combined rule must cover both hover and pressed — two separate
// `transition` declarations on the same element would let the later rule drop
// the earlier one's properties.
func TestShellPseudoStateTransition(t *testing.T) {
	css := shellCSS(t)
	want := `[style*="--qorm-hov-"], [style*="--qorm-prs-"] { transition:background .15s ease, color .15s ease, opacity .15s ease, transform .12s ease; }`
	if !strings.Contains(css, want) {
		t.Errorf("shell should ease pseudo-state changes with one combined rule:\n  %s", want)
	}
}
