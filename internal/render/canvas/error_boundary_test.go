package canvas

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
)

func TestNodeErrorBoundaryFallsBackAndKeepsSiblings(t *testing.T) {
	root := &model.Node{
		Type: "column", ID: "root",
		Children: []*model.Node{
			{Type: "text", ID: "before", Text: "BEFORE"},
			{
				Type: "column", ID: "boundary",
				ErrorBoundary: &model.NodeErrorBoundary{
					Fallback: &model.Node{Type: "text", ID: "fallback", Text: "FALLBACK"},
				},
				Children: []*model.Node{
					{Type: "colunm", ID: "bad", Children: []*model.Node{
						{Type: "text", ID: "nested", Text: "BAD"},
					}},
				},
			},
			{Type: "text", ID: "after", Text: "AFTER"},
		},
	}
	rt := runtime.New(&model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
	})
	rt.ClearPendingEnter()
	g := layoutScene(root, rt, image.Pt(300, 200))
	if walkModel(g, "before") == nil {
		t.Fatal("sibling before boundary missing")
	}
	if walkModel(g, "after") == nil {
		t.Fatal("sibling after boundary missing")
	}
	if walkModel(g, "fallback") == nil {
		t.Fatal("fallback text missing after unknown child tripped boundary")
	}
	if walkModel(g, "bad") != nil || walkModel(g, "nested") != nil {
		t.Fatal("failed subtree should not appear in the graph")
	}
	if rt.LastBoundaryError.Level != "node" || rt.LastBoundaryError.NodeID != "boundary" {
		t.Fatalf("LastBoundaryError = %#v, want node/boundary", rt.LastBoundaryError)
	}
}

func TestNodeErrorBoundaryResolvesBoundType(t *testing.T) {
	root := &model.Node{
		Type: "column", ID: "root",
		Children: []*model.Node{
			{
				Type: "column", ID: "boundary",
				ErrorBoundary: &model.NodeErrorBoundary{
					Fallback: &model.Node{Type: "text", ID: "fallback", Text: "FALLBACK"},
				},
				Children: []*model.Node{
					{Type: "{{state.brokenKind}}", ID: "fragile", Text: "ok-when-text"},
				},
			},
		},
	}
	rt := runtime.New(&model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
	})
	rt.State["brokenKind"] = "colunm"
	rt.ClearPendingEnter()
	g := layoutScene(root, rt, image.Pt(200, 100))
	if walkModel(g, "fallback") == nil {
		t.Fatal("bound unknown type should trip boundary fallback")
	}
	rt.State["brokenKind"] = "text"
	rt.LastBoundaryError = runtime.BoundaryError{}
	g = layoutScene(root, rt, image.Pt(200, 100))
	if walkModel(g, "fragile") == nil {
		t.Fatal("bound text type should render the fragile node")
	}
	if walkModel(g, "fallback") != nil {
		t.Fatal("fallback must not appear when bound type is valid")
	}
}

func TestSceneBoundaryLayoutPanicRedirects(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "mainRoot", Children: []*model.Node{
				{Type: "text", ID: "mainText", Text: "MAIN"},
			}},
			"oops": {Type: "text", ID: "oopsText", Text: "OOPS"},
		},
		ErrorBoundary: &model.SceneErrorBoundary{Scene: "oops"},
	}
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	e := &Engine{RT: rt}

	old := layoutHook
	layoutHook = func(ops *op.Ops, root *model.Node, size image.Point, rt *runtime.Runtime, inter *Interaction, scale int) (graph.Node, bool, map[graph.Node]itemInstance, *LayoutNode) {
		if root != nil && root.ID == "mainRoot" {
			panic("boom layout")
		}
		return layout(ops, root, size, rt, inter, scale)
	}
	t.Cleanup(func() { layoutHook = old })

	ops := &op.Ops{}
	g, _, _, _ := e.layoutWithSceneBoundary(ops, app.Scenes["main"], image.Pt(160, 80), rt, 1)
	if rt.CurrentScene() != "oops" {
		t.Fatalf("CurrentScene() = %q, want oops after layout panic", rt.CurrentScene())
	}
	if walkModel(g, "oopsText") == nil {
		t.Fatal("fallback scene root should be laid out after redirect")
	}
	if rt.LastBoundaryError.Phase != "render" || rt.LastBoundaryError.Level != "scene" {
		t.Fatalf("LastBoundaryError = %#v, want scene/render", rt.LastBoundaryError)
	}
}

func TestErrorBoundaryExampleLoads(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "examples", "error-boundary")
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load example: %v", err)
	}
	rt := runtime.New(app)
	rt.ClearPendingEnter()
	root := app.Scenes["main"]
	g := layoutScene(root, rt, image.Pt(400, 400))
	if walkModel(g, "local_fallback") == nil {
		t.Fatal("example starts with brokenKind=colunm; boundary fallback should render")
	}
	if rt.LastBoundaryError.Level != "node" || !strings.Contains(rt.LastBoundaryError.Message, "colunm") {
		t.Fatalf("LastBoundaryError = %#v, want node/colunm", rt.LastBoundaryError)
	}
}
