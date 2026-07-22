package measure

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// auditApp wraps a scene root node in a minimal runtime for Audit tests.
func auditApp(root *model.Node) *qrt.Runtime {
	app := &model.App{Name: "auditapp", Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	return qrt.New(app)
}

// runAudit marshals the measured rows and returns the parsed Audit report.
func runAudit(t *testing.T, rt *qrt.Runtime, rows []map[string]any) map[string]any {
	t.Helper()
	mb, _ := json.Marshal(rows)
	out, err := Audit(rt, mb)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatalf("audit report JSON: %v", err)
	}
	return rep
}

// issueList extracts the report's details array as typed maps.
func issueList(rep map[string]any) []map[string]any {
	ds, _ := rep["details"].([]any)
	out := make([]map[string]any, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.(map[string]any))
	}
	return out
}

// hasIssue reports whether the audit details contain an issue with the given
// component id and kind.
func hasIssue(rep map[string]any, id, kind string) bool {
	for _, d := range issueList(rep) {
		if fmt.Sprint(d["id"]) == id && fmt.Sprint(d["kind"]) == kind {
			return true
		}
	}
	return false
}

// visRow is a measured row for a visible component of the given type and box.
func visRow(id, typ string, x, y, w, h float64) map[string]any {
	return map[string]any{"id": id, "type": typ, "visible": true, "x": x, "y": y, "w": w, "h": h}
}

// simpleScene is a scaffold root with named column children, matching the ids
// used in the measured rows of the bounds tests.
func simpleScene(ids ...string) *model.Node {
	root := &model.Node{Type: "scaffold", ID: "root"}
	for _, id := range ids {
		root.Children = append(root.Children, &model.Node{Type: "column", ID: id})
	}
	return root
}

// TestAuditZeroSize flags visible components with a zero width or height,
// counts visible components, and ignores invisible ones entirely.
func TestAuditZeroSize(t *testing.T) {
	rt := auditApp(simpleScene("zw", "zh", "fine", "hidden"))
	rows := []map[string]any{
		visRow("zw", "column", 0, 0, 0, 10),
		visRow("zh", "column", 0, 0, 10, 0),
		visRow("fine", "column", 0, 0, 10, 10),
		// invisible zero-size row must produce no issue and no count
		{"id": "hidden", "type": "column", "visible": false, "x": 0.0, "y": 0.0, "w": 0.0, "h": 0.0},
	}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "zw", "zero-size") {
		t.Errorf("zero width should be flagged; details: %v", issueList(rep))
	}
	if !hasIssue(rep, "zh", "zero-size") {
		t.Errorf("zero height should be flagged; details: %v", issueList(rep))
	}
	if hasIssue(rep, "hidden", "zero-size") {
		t.Error("invisible components must be skipped, not flagged")
	}
	if got := rep["visibleComponents"]; got != float64(3) {
		t.Errorf("visibleComponents = %v, want 3", got)
	}
	if rep["ok"] != false {
		t.Error("audit with zero-size issues must not be ok")
	}
}

// TestAuditOutOfBounds checks the horizontal bounds enforcement and its exact
// +/-3 px tolerance on both edges.
func TestAuditOutOfBounds(t *testing.T) {
	rt := auditApp(simpleScene("rightEdge", "rightOver", "leftEdge", "leftOver", "inside"))
	rootRow := map[string]any{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0}
	rows := []map[string]any{
		rootRow,
		visRow("rightEdge", "column", 393, 0, 10, 10), // right = 403 = rootR+3: tolerated
		visRow("rightOver", "column", 394, 0, 10, 10), // right = 404 > rootR+3: out
		visRow("leftEdge", "column", -3, 0, 10, 10),   // left = -3 = rootL-3: tolerated
		visRow("leftOver", "column", -4, 0, 10, 10),   // left = -4 < rootL-3: out
		visRow("inside", "column", 10, 0, 100, 10),
	}
	rep := runAudit(t, rt, rows)
	if hasIssue(rep, "rightEdge", "out-of-bounds") {
		t.Error("right edge exactly at rootR+3 must be tolerated")
	}
	if !hasIssue(rep, "rightOver", "out-of-bounds") {
		t.Errorf("right=404 > 403 must be out-of-bounds; details: %v", issueList(rep))
	}
	if hasIssue(rep, "leftEdge", "out-of-bounds") {
		t.Error("left edge exactly at rootL-3 must be tolerated")
	}
	if !hasIssue(rep, "leftOver", "out-of-bounds") {
		t.Errorf("left=-4 < -3 must be out-of-bounds; details: %v", issueList(rep))
	}
	if hasIssue(rep, "inside", "out-of-bounds") {
		t.Error("fully contained component must not be flagged")
	}
}

// TestAuditBoundsFromQormRoot verifies the measured #qorm-root container wins
// over a scene node id'd "root": a component inside the legacy root box but
// outside the qorm-root box must be flagged on both edges.
func TestAuditBoundsFromQormRoot(t *testing.T) {
	rt := auditApp(simpleScene("leftOfStage", "rightOfStage"))
	rows := []map[string]any{
		// legacy scene root: 0..400
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		// centered stage container: 50..350, must be the effective bounds
		{"id": "qorm-root", "type": "scaffold", "visible": true, "x": 50.0, "y": 0.0, "w": 300.0, "h": 800.0},
		visRow("leftOfStage", "column", 10, 0, 10, 10),   // inside root box, left of stage
		visRow("rightOfStage", "column", 345, 0, 20, 10), // inside root box, right of stage
	}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "leftOfStage", "out-of-bounds") {
		t.Errorf("component left of the qorm-root stage must be flagged; details: %v", issueList(rep))
	}
	if !hasIssue(rep, "rightOfStage", "out-of-bounds") {
		t.Errorf("component right of the qorm-root stage must be flagged; details: %v", issueList(rep))
	}
	if hasIssue(rep, "qorm-root", "zero-size") || hasIssue(rep, "qorm-root", "out-of-bounds") {
		t.Error("qorm-root is the bounds source itself and must never be flagged")
	}
	// qorm-root is skipped before the visibility count; root + both components
	// remain
	if got := rep["visibleComponents"]; got != float64(3) {
		t.Errorf("visibleComponents = %v, want 3 (qorm-root excluded)", got)
	}
}

// TestAuditBoundsLegacyRootFallback verifies a scene node id'd "root" supplies
// the bounds when no #qorm-root row is present (both edges moved).
func TestAuditBoundsLegacyRootFallback(t *testing.T) {
	rt := auditApp(simpleScene("offLeft", "offRight"))
	rows := []map[string]any{
		// root box shifted to 100..300; nothing id'd qorm-root
		{"id": "root", "type": "scaffold", "visible": true, "x": 100.0, "y": 0.0, "w": 200.0, "h": 800.0},
		visRow("offLeft", "column", 50, 0, 10, 10),   // left=50 < 97: out (would pass default 0)
		visRow("offRight", "column", 295, 0, 20, 10), // right=315 > 303: out (would pass default 400)
	}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "offLeft", "out-of-bounds") {
		t.Errorf("left of the legacy root box must be flagged; details: %v", issueList(rep))
	}
	if !hasIssue(rep, "offRight", "out-of-bounds") {
		t.Errorf("right of the legacy root box must be flagged; details: %v", issueList(rep))
	}
}

// TestAuditBoundsDefaultWindow verifies the 0..400 default bounds when no root
// row of either kind is measured.
func TestAuditBoundsDefaultWindow(t *testing.T) {
	rt := auditApp(simpleScene("far", "ok"))
	rows := []map[string]any{
		visRow("far", "column", 410, 0, 20, 10), // right=430 > 403
		visRow("ok", "column", 380, 0, 20, 10),  // right=400 <= 403
	}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "far", "out-of-bounds") {
		t.Errorf("past the default 400 bound must be flagged; details: %v", issueList(rep))
	}
	if hasIssue(rep, "ok", "out-of-bounds") {
		t.Error("inside the default bounds must not be flagged")
	}
}

// TestAuditXOverflow verifies overflowX is flagged for ordinary components but
// exempted for inherently scrolling/paged widget types.
func TestAuditXOverflow(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "root", Children: []*model.Node{
		{Type: "column", ID: "clip"},
		{Type: "scroll", ID: "sc"},
	}}
	rt := auditApp(root)
	rows := []map[string]any{
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		{"id": "clip", "type": "column", "visible": true, "x": 0.0, "y": 0.0, "w": 100.0, "h": 10.0, "overflowX": true},
		{"id": "sc", "type": "scroll", "visible": true, "x": 0.0, "y": 0.0, "w": 100.0, "h": 10.0, "overflowX": true},
	}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "clip", "x-overflow") {
		t.Errorf("ordinary component with overflowX must be flagged; details: %v", issueList(rep))
	}
	if hasIssue(rep, "sc", "x-overflow") {
		t.Error("a scroll container legitimately overflows and must be exempt")
	}
}

// TestAuditScrollTypesBoundsExempt verifies scrolling/paged widget types are
// exempt from bounds checks (their content is legitimately off-screen), while
// the zero-size invariant still applies to them.
func TestAuditScrollTypesBoundsExempt(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "root", Children: []*model.Node{
		{Type: "pageview", ID: "pv"},
		{Type: "carousel", ID: "car"},
		{Type: "table", ID: "tab"},
		{Type: "badge", ID: "bdg"},
	}}
	rt := auditApp(root)
	rows := []map[string]any{
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		// all far out of bounds, must NOT be flagged
		visRow("pv", "pageview", 2000, 0, 400, 100),
		visRow("car", "carousel", 2000, 0, 400, 100),
		visRow("tab", "table", 2000, 0, 400, 100),
		// exempt from bounds but NOT from the zero-size invariant
		visRow("bdg", "badge", 2000, 0, 0, 20),
	}
	rep := runAudit(t, rt, rows)
	for _, id := range []string{"pv", "car", "tab", "bdg"} {
		if hasIssue(rep, id, "out-of-bounds") {
			t.Errorf("scroll-type %q must be exempt from bounds checks", id)
		}
	}
	if !hasIssue(rep, "bdg", "zero-size") {
		t.Errorf("zero-size must still apply to exempt types; details: %v", issueList(rep))
	}
}

// TestAuditHScrollDescendantsExempt verifies children inside a horizontally
// scrolling/paged container are exempt from bounds checks, while the same
// child inside a VERTICAL scroll is still flagged.
func TestAuditHScrollDescendantsExempt(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "root", Children: []*model.Node{
		{Type: "pageview", ID: "pv", Children: []*model.Node{
			{Type: "column", ID: "inPage"},
			// a list template inside the pager is exempt too
			{Type: "list", ID: "lst", Template: &model.Node{Type: "column", ID: "inTemplate"}},
			// both branches of a `when` inside the pager are exempt
			{Type: "when", Condition: "true",
				Then: &model.Node{Type: "column", ID: "inThen"},
				Else: &model.Node{Type: "column", ID: "inElse"}},
		}},
		{Type: "carousel", ID: "car", Children: []*model.Node{{Type: "column", ID: "inCarousel"}}},
		{Type: "scroll", ID: "hsc", Props: map[string]any{"orientation": "horizontal"},
			Children: []*model.Node{{Type: "column", ID: "inHScroll"}}},
		// vertical scroll: container exempt (scrollType) but its children are NOT
		{Type: "scroll", ID: "vsc", Children: []*model.Node{{Type: "column", ID: "inVScroll"}}},
	}}
	rt := auditApp(root)
	off := func(id string) map[string]any { return visRow(id, "column", 3000, 0, 300, 100) }
	rows := []map[string]any{
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		off("inPage"), off("inTemplate"), off("inThen"), off("inElse"),
		off("inCarousel"), off("inHScroll"), off("inVScroll"),
	}
	rep := runAudit(t, rt, rows)
	for _, id := range []string{"inPage", "inTemplate", "inThen", "inElse", "inCarousel", "inHScroll"} {
		if hasIssue(rep, id, "out-of-bounds") {
			t.Errorf("descendant %q of an h-scroll container must be exempt; details: %v", id, issueList(rep))
		}
	}
	if !hasIssue(rep, "inVScroll", "out-of-bounds") {
		t.Errorf("child of a VERTICAL scroll is not exempt; details: %v", issueList(rep))
	}
}

// TestAuditFixedPositionExempt verifies position:fixed components are exempt
// from bounds/overflow checks (they are window-anchored, not stage-anchored).
func TestAuditFixedPositionExempt(t *testing.T) {
	rt := auditApp(simpleScene("toast", "flow"))
	rows := []map[string]any{
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		{"id": "toast", "type": "column", "visible": true, "position": "fixed", "x": 5000.0, "y": 0.0, "w": 100.0, "h": 20.0, "overflowX": true},
		// same box without fixed: must be flagged
		visRow("flow", "column", 5000, 0, 100, 20),
	}
	rep := runAudit(t, rt, rows)
	if hasIssue(rep, "toast", "out-of-bounds") || hasIssue(rep, "toast", "x-overflow") {
		t.Errorf("fixed-position component must be exempt; details: %v", issueList(rep))
	}
	if !hasIssue(rep, "flow", "out-of-bounds") {
		t.Error("the same box in normal flow must be flagged")
	}
}

// TestAuditClean verifies a well-laid-out render reports ok with zero issues
// and the app name.
func TestAuditClean(t *testing.T) {
	rt := auditApp(simpleScene("a", "b"))
	rows := []map[string]any{
		{"id": "root", "type": "scaffold", "visible": true, "x": 0.0, "y": 0.0, "w": 400.0, "h": 800.0},
		visRow("a", "column", 8, 8, 100, 20),
		visRow("b", "column", 8, 40, 100, 20),
	}
	rep := runAudit(t, rt, rows)
	if rep["ok"] != true {
		t.Errorf("clean layout must be ok; details: %v", issueList(rep))
	}
	if rep["issues"] != float64(0) {
		t.Errorf("issues = %v, want 0", rep["issues"])
	}
	if rep["app"] != "auditapp" {
		t.Errorf("app = %v, want auditapp", rep["app"])
	}
	if got := rep["visibleComponents"]; got != float64(3) {
		t.Errorf("visibleComponents = %v, want 3 (root + a + b)", got)
	}
}

// TestAuditNoScene verifies Audit degrades gracefully when the app has no
// renderable scene: no crash, and measured rows are still checked against the
// default bounds.
func TestAuditNoScene(t *testing.T) {
	app := &model.App{Name: "empty", Entry: "missing", Scenes: map[string]*model.Node{}}
	rt := qrt.New(app)
	rows := []map[string]any{visRow("ghost", "column", 900, 0, 50, 10)}
	rep := runAudit(t, rt, rows)
	if !hasIssue(rep, "ghost", "out-of-bounds") {
		t.Errorf("row past default bounds must still be flagged with no scene; details: %v", issueList(rep))
	}
}
