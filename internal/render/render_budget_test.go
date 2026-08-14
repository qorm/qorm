package render

import (
	"strings"
	"testing"
	"time"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// The component fan-out bomb. renderInner caps component recursion with
// `r.compDepth < 32`, which bounds DEPTH but not TOTAL WORK: a component whose
// template contains two instances of itself doubles at every level, so 32
// levels is 2^32 instantiations while the depth cap is never violated. Render()
// simply never returned — and every caller that renders (POST /event, the SSE
// catch-up, /poll, an MCP patch) holds the server mutex while it does, so a
// single such component wedged the whole server for every client, not just the
// request that triggered it. Any party that can write a component definition
// (an OTA bundle, an MCP patch, a local edit picked up by hot reload) could do
// it, and the loader cannot reject it statically — self-reference is legal.
//
// The fix charges every node against a per-render budget. These tests pin both
// halves: the bomb terminates with bounded output, and real apps are nowhere
// near the limits.

// bombApp builds the red team's exact construction:
//
//	{"type":"column","children":[{"type":"bomb"},{"type":"bomb"}]}
//
// registered as the component "bomb", with the entry scene instantiating it.
func bombApp() *model.App {
	return &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{"main": {
			ID: "root", Type: "column", Children: []*model.Node{{Type: "bomb"}},
		}},
		Components: map[string]*model.Node{
			"bomb": {Type: "column", Children: []*model.Node{{Type: "bomb"}, {Type: "bomb"}}},
		},
	}
}

// TestComponentFanOutBombIsBudgeted: the render must RETURN, quickly, with a
// bounded document — and say so.
func TestComponentFanOutBombIsBudgeted(t *testing.T) {
	done := make(chan Result, 1)
	go func() { done <- Render(runtime.New(bombApp())) }()

	var res Result
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Render() did not return within 10s on a self-referencing component: " +
			"the fan-out budget is not bounding total work (this render holds the server mutex)")
	}

	if len(res.HTML) > maxRenderBytes+len(budgetMarker)+4096 {
		t.Errorf("output is not bounded: %d bytes (budget %d)", len(res.HTML), maxRenderBytes)
	}
	if !strings.Contains(res.HTML, budgetMarker) {
		t.Errorf("a truncated render must carry the truncation marker %q so the client and the audit can see it", budgetMarker)
	}
	if strings.Count(res.HTML, budgetMarker) != 1 {
		t.Errorf("the marker must be emitted exactly once, got %d", strings.Count(res.HTML, budgetMarker))
	}
	found := false
	for _, u := range res.Unknown {
		if u == BudgetExceeded {
			found = true
		}
	}
	if !found {
		t.Errorf("a truncated render must report %q in Result.Unknown (the self-verify channel)", BudgetExceeded)
	}
	// Degrade, never panic and never bail out with an empty page: what did fit
	// is still a document.
	if !strings.HasPrefix(res.HTML, `<div id="root"`) && !strings.HasPrefix(res.HTML, `<div data-scene`) {
		t.Errorf("the truncated render must still start with the scene root, got %.120q", res.HTML)
	}
}

// TestFanOutBombIsFastEnoughToNotWedgeTheServer pins the property that actually
// matters operationally: the budget has to make the pathological render cheap,
// not merely finite. A minute-long bounded render would wedge the server just
// as well as an infinite one.
func TestFanOutBombIsFastEnoughToNotWedgeTheServer(t *testing.T) {
	rt := runtime.New(bombApp())
	start := time.Now()
	Render(rt)
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("a budgeted bomb render took %v — too slow to hold the server mutex for", d)
	}
}

// TestSelfReferencingComponentWithoutFanOutStillRenders: a component that
// references itself ONCE (a linked-list / tree template, a legitimate shape)
// only recurses to the depth cap, so it must render normally and NOT be
// reported as truncated.
func TestSelfReferencingComponentWithoutFanOutStillRenders(t *testing.T) {
	app := bombApp()
	app.Components["bomb"] = &model.Node{Type: "column", Children: []*model.Node{{Type: "bomb"}}}
	res := Render(runtime.New(app))
	if strings.Contains(res.HTML, budgetMarker) {
		t.Errorf("a linear self-reference (bounded by compDepth) must not hit the node budget:\n%s", res.HTML)
	}
	for _, u := range res.Unknown {
		if u == BudgetExceeded {
			t.Error("a linear self-reference must not report the budget as exceeded")
		}
	}
}

// TestRenderBudgetHasHeadroomForRealApps: the budget is a fail-safe, not a
// feature limit. If a normal scene ever approaches it, the constants are wrong.
// (The largest example in the repo renders ~100 KB / well under a thousand
// nodes; this asserts the ORDER of magnitude the constants were chosen against,
// so lowering them carelessly fails here rather than silently truncating apps.)
func TestRenderBudgetHasHeadroomForRealApps(t *testing.T) {
	const observedMaxNodes = 1600 // upper bound measured over every example
	if maxRenderNodes < observedMaxNodes*10 {
		t.Errorf("maxRenderNodes = %d leaves less than 10x headroom over the largest real scene (~%d nodes)",
			maxRenderNodes, observedMaxNodes)
	}
	const observedMaxBytes = 110 << 10
	if maxRenderBytes < observedMaxBytes*10 {
		t.Errorf("maxRenderBytes = %d leaves less than 10x headroom over the largest real scene (~%d bytes)",
			maxRenderBytes, observedMaxBytes)
	}

	// And an ordinary render must not spend the budget or report it.
	res := renderWidget(t, &model.Node{Type: "column", ID: "c", Children: textKids("x")})
	if strings.Contains(res.HTML, budgetMarker) || len(res.Unknown) != 0 {
		t.Errorf("an ordinary render must be untouched by the budget: unknown=%v\n%s", res.Unknown, res.HTML)
	}
}
