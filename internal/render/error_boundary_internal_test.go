package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func TestRenderBoundaryWithoutFallbackSwallowsBrokenSubtree(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}}})
	rt.ClearPendingEnter()
	r := &renderer{rt: rt, scope: map[string]any{}}
	n := &model.Node{
		Type: "column", ID: "boundary",
		ErrorBoundary: &model.NodeErrorBoundary{},
		Children:      []*model.Node{{Type: "colunm", ID: "bad"}},
	}
	r.renderBoundary(n)
	if got := r.sb.String(); got != "" {
		t.Fatalf("boundary without fallback should render nothing, got %q", got)
	}
	if rt.LastBoundaryError.Level != "node" || rt.LastBoundaryError.NodeID != "boundary" {
		t.Fatalf("LastBoundaryError = %#v", rt.LastBoundaryError)
	}
}

func TestSpendNodeBoundaryTrapMarksFailure(t *testing.T) {
	r := &renderer{boundaryTrap: true, overBudget: true}
	if r.spendNode() {
		t.Fatal("over-budget boundary trap must refuse the node")
	}
	if !r.boundaryHit || r.boundaryMsg != BudgetExceeded {
		t.Fatalf("boundary trap = hit:%v msg:%q", r.boundaryHit, r.boundaryMsg)
	}
}

func TestSpendNodeOverBudgetWithoutTrapIsQuiet(t *testing.T) {
	r := &renderer{overBudget: true}
	if r.spendNode() {
		t.Fatal("over-budget renderer must refuse more nodes")
	}
	if r.sb.Len() != 0 || len(r.unknowns) != 0 {
		t.Fatalf("quiet over-budget path mutated renderer: html=%q unknown=%v", r.sb.String(), r.unknowns)
	}
}

func TestSpendNodeOverBudgetDegradesHTML(t *testing.T) {
	r := &renderer{}
	r.nodesRendered = maxRenderNodes
	if r.spendNode() {
		t.Fatal("budget should be exhausted")
	}
	if got := r.sb.String(); got != budgetMarker {
		t.Fatalf("budget marker = %q, want %q", got, budgetMarker)
	}
	if len(r.unknowns) != 1 || r.unknowns[0] != BudgetExceeded {
		t.Fatalf("unknowns = %#v", r.unknowns)
	}
}

func TestSpendNodeByteBudgetTrapMarksFailure(t *testing.T) {
	r := &renderer{boundaryTrap: true}
	r.sb.WriteString(strings.Repeat("x", maxRenderBytes+1))
	if r.spendNode() {
		t.Fatal("byte-budget trap should fail")
	}
	if !r.boundaryHit || r.boundaryMsg != BudgetExceeded {
		t.Fatalf("byte-budget trap = hit:%v msg:%q", r.boundaryHit, r.boundaryMsg)
	}
}

func TestRenderProtectedCopyDropsBoundaryOnlyOnCopy(t *testing.T) {
	n := &model.Node{
		Type:          "column",
		ID:            "x",
		ErrorBoundary: &model.NodeErrorBoundary{Fallback: &model.Node{Type: "text", ID: "fb", Text: "fallback"}},
	}
	cp := renderProtectedCopy(n)
	if cp == n {
		t.Fatal("renderProtectedCopy must return a copy")
	}
	if cp.ErrorBoundary != nil {
		t.Fatalf("copy should drop boundary metadata, got %#v", cp.ErrorBoundary)
	}
	if n.ErrorBoundary == nil || n.ErrorBoundary.Fallback == nil {
		t.Fatal("original node boundary must stay intact")
	}
	if renderProtectedCopy(nil) != nil {
		t.Fatal("nil input should stay nil")
	}
}

func TestChildRendererAndMergeChildCarryHandlerOffsets(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}}})
	parent := &renderer{rt: rt, scope: map[string]any{}, handlers: []Handler{{Name: "existing"}}}
	child := parent.childRenderer()
	id := child.register(&model.Invoke{Name: "save", Args: map[string]string{}})
	if id != 1 {
		t.Fatalf("child handler id = %d, want 1", id)
	}
	child.sb.WriteString("x")
	parent.mergeChild(child)
	if len(parent.handlers) != 2 || parent.handlers[1].Name != "save" {
		t.Fatalf("merged handlers = %#v", parent.handlers)
	}
	if parent.sb.String() != "x" {
		t.Fatalf("merged HTML = %q", parent.sb.String())
	}
}

func TestRenderNodeSafeRecoversPanics(t *testing.T) {
	r := &renderer{}
	ok := r.renderNodeSafe(&model.Node{Type: "text", ID: "t", Text: "{{state.x}}"}, false)
	if ok {
		t.Fatal("renderNodeSafe should recover a nil-runtime panic")
	}
	if !r.boundaryHit || r.boundaryMsg == "" {
		t.Fatalf("panic recovery not recorded: hit=%v msg=%q", r.boundaryHit, r.boundaryMsg)
	}
}

func TestRenderNodeSafeTrapOnUnknown(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}}})
	r := &renderer{rt: rt, scope: map[string]any{}}
	if r.renderNodeSafe(&model.Node{Type: "colunm", ID: "bad"}, true) {
		t.Fatal("trap render should fail on unknown widget")
	}
	if !r.boundaryHit || r.boundaryMsg == "" {
		t.Fatalf("trap state = hit:%v msg:%q", r.boundaryHit, r.boundaryMsg)
	}
}

func TestRenderSubtreeWithOptsFallbackBranches(t *testing.T) {
	if got := RenderSubtreeWithOpts(nil, "x", RenderOpts{}).HTML; got == "" {
		t.Fatal("nil runtime should render a placeholder")
	}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}}})
	if got := RenderSubtreeWithOpts(rt, "missing", RenderOpts{}).HTML; got == "" {
		t.Fatal("missing node should render a placeholder")
	}
}

func TestRenderSceneWithOptsBlockedAndAlternateScene(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main":  &model.Node{Type: "text", ID: "mainText", Text: "MAIN"},
			"other": &model.Node{Type: "text", ID: "otherText", Text: "OTHER"},
		},
	}
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	res := RenderSceneWithOpts(rt, "other", RenderOpts{})
	if res.HTML == "" || !contains(res.HTML, "OTHER") {
		t.Fatalf("alternate scene render = %q", res.HTML)
	}
	rt.Scene = runtime.GuardBlocked
	res = RenderSceneWithOpts(rt, "", RenderOpts{})
	if !contains(res.HTML, "no scene to render") {
		t.Fatalf("blocked render = %q", res.HTML)
	}
}

func TestRenderSceneWithOptsTagsSceneKey(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main":  &model.Node{Type: "text", ID: "mainText", Text: "MAIN"},
			"other": &model.Node{Type: "text", ID: "otherText", Text: "OTHER"},
		},
	}
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	if html := RenderSceneWithOpts(rt, "other", RenderOpts{}).HTML; !contains(html, `data-scene="other"`) {
		t.Fatalf("other scene missing scene tag: %q", html)
	}
	if html := Render(rt).HTML; !contains(html, `data-scene="entry"`) {
		t.Fatalf("entry render missing scene tag: %q", html)
	}
}

func TestRenderBoundaryFallbackSuccessPath(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": &model.Node{Type: "text", ID: "mainText", Text: "main"}}})
	rt.ClearPendingEnter()
	r := &renderer{rt: rt, scope: map[string]any{}}
	n := &model.Node{
		Type: "column", ID: "boundary",
		ErrorBoundary: &model.NodeErrorBoundary{Fallback: &model.Node{Type: "text", ID: "fb", Text: "fallback"}},
		Children:      []*model.Node{{Type: "colunm", ID: "bad"}},
	}
	r.renderBoundary(n)
	if !contains(r.sb.String(), "fallback") {
		t.Fatalf("fallback HTML = %q", r.sb.String())
	}
}

func TestRenderSubtreeWithOptsUsesCurrentScene(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main":  &model.Node{Type: "text", ID: "mainText", Text: "MAIN"},
			"other": &model.Node{Type: "text", ID: "otherText", Text: "OTHER"},
		},
	}
	rt := runtime.New(app)
	rt.Scene = "other"
	rt.ClearPendingEnter()
	if html := RenderSubtreeWithOpts(rt, "otherText", RenderOpts{}).HTML; !contains(html, "OTHER") {
		t.Fatalf("subtree render = %q", html)
	}
}

func TestMergeChildNilAndNodeNilAreNoops(t *testing.T) {
	r := &renderer{}
	r.mergeChild(nil)
	r.node(nil)
	if r.sb.Len() != 0 || len(r.handlers) != 0 {
		t.Fatalf("noop calls mutated renderer: html=%q handlers=%d", r.sb.String(), len(r.handlers))
	}
}

func TestRenderNodeDiffAndFindNodeInTreeBranches(t *testing.T) {
	root := &model.Node{
		Type: "when", ID: "root", Condition: "{{ false }}",
		Then:     &model.Node{Type: "text", ID: "thenText", Text: "THEN"},
		Else:     &model.Node{Type: "text", ID: "elseText", Text: "ELSE"},
		Template: &model.Node{Type: "text", ID: "tmplText", Text: "TMPL"},
	}
	if got := findNodeInTree(root, "tmplText"); got == nil || got.ID != "tmplText" {
		t.Fatalf("findNodeInTree(template) = %#v", got)
	}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rt.ClearPendingEnter()
	if html := RenderNodeDiff(rt, "elseText").HTML; !contains(html, `data-morph-target="elseText"`) {
		t.Fatalf("node diff = %q", html)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
