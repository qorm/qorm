package server

import (
	"strings"
	"testing"
)

// The tooltip contract is split across two packages: internal/render emits the
// markup (a .qorm-tip wrapper + a role="tooltip" bubble, or — for the legacy
// `tooltip` PROP — a data-tooltip attribute), and the shell stylesheet here
// carries every rule that makes either visible. Without the rules below the
// renderer's output is inert and nothing else in the codebase would notice, so
// this file pins the consuming half exactly the way pseudostate_test.go pins the
// pseudo-state one.

// TestShellLegacyTooltipRuleIsIntact is the backward-compatibility gate for the
// SHELL half: the legacy attribute rule must keep its original declarations
// byte-for-byte, so an app using the `tooltip` prop looks exactly as it did.
func TestShellLegacyTooltipRuleIsIntact(t *testing.T) {
	css := shellCSS(t)
	for _, want := range []string{
		`[data-tooltip] { position:relative; }`,
		`content:attr(data-tooltip); position:absolute; bottom:100%; left:50%; transform:translateX(-50%);`,
		`background:#111827; color:#fff; padding:4px 8px; border-radius:6px; font-size:12px; white-space:nowrap; margin-bottom:6px; z-index:100; pointer-events:none; }`,
		`[data-tooltip]:hover::after`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the legacy data-tooltip rule changed — shipped apps would look different:\n  missing %s", want)
		}
	}
	// The one ADDITIVE change: the same hint is now reachable from the keyboard.
	// It shares the hover rule's declarations, so it cannot drift from it.
	if !strings.Contains(css, `[data-tooltip]:hover::after, [data-tooltip]:focus-visible::after {`) {
		t.Error("the legacy tooltip should also trigger on :focus-visible (keyboard reachability)")
	}
}

// TestShellTooltipWidgetRules pins the rules the tooltip WIDGET depends on. The
// class names are the API between render_feedback.go and this stylesheet, so a
// rename on either side fails here rather than shipping a tooltip that never
// appears.
func TestShellTooltipWidgetRules(t *testing.T) {
	css := shellCSS(t)
	for _, want := range []string{
		`.qorm-tip { position:relative; }`,
		`.qorm-tip-bubble {`,
		`.qorm-tip-top > .qorm-tip-bubble {`,
		`.qorm-tip-bottom > .qorm-tip-bubble {`,
		`.qorm-tip-left > .qorm-tip-bubble {`,
		`.qorm-tip-right > .qorm-tip-bubble {`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("shell stylesheet lacks the tooltip-widget rule:\n  %s", want)
		}
	}
	// Long text must WRAP inside a bounded box — the whole reason the widget
	// exists next to the nowrap attribute.
	for _, want := range []string{"white-space:normal;", "max-width:240px;", "overflow-wrap:anywhere;"} {
		if !strings.Contains(css, want) {
			t.Errorf("the bubble must wrap long text inside a max-width (%s)", want)
		}
	}
	// It must not participate in layout or eat the pointer.
	for _, want := range []string{"position:absolute;", "pointer-events:none;"} {
		if !strings.Contains(css, want) {
			t.Errorf("the bubble should be an overlay, not a layout box (%s)", want)
		}
	}
}

// TestShellTooltipTriggers: hover — and ONLY hover — belongs inside
// @media (hover:hover), or a tap on a touch device leaves the bubble stuck open
// with no way to dismiss it. The keyboard triggers must stay outside.
func TestShellTooltipTriggers(t *testing.T) {
	css := shellCSS(t)
	block := hoverMediaBlock(t, css, `.qorm-tip:hover > .qorm-tip-bubble`)
	if !strings.Contains(block, `.qorm-tip:hover > .qorm-tip-bubble`) {
		t.Error("the tooltip hover trigger must live inside @media (hover:hover)")
	}
	for _, kb := range []string{
		`.qorm-tip:focus-visible > .qorm-tip-bubble`,
		`.qorm-tip:focus-within > .qorm-tip-bubble`,
	} {
		if !strings.Contains(css, kb) {
			t.Fatalf("shell has no keyboard trigger %s", kb)
		}
		if strings.Contains(block, kb) {
			t.Errorf("%s works without a pointer and must NOT be gated behind @media (hover:hover)", kb)
		}
	}
	// Both triggers reveal it the same way, so a focused tooltip and a hovered
	// one cannot look different.
	if strings.Count(css, "opacity:1; visibility:visible; }") < 2 {
		t.Error("hover and focus should reveal the bubble with the same declarations")
	}
}
