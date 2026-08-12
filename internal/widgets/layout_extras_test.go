package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
)

func TestLayoutExtrasRegistered(t *testing.T) {
	for _, name := range []string{
		"aspectratio", "ignorepointer", "skeleton", "circularprogress",
		"circularprogressindicator", "activityindicator", "tag",
	} {
		if _, ok := canvas.LookupWidget(name); !ok {
			t.Errorf("widget %q not registered", name)
		}
	}
}

func TestAspectRatioMeasure(t *testing.T) {
	rt := &runtime.Runtime{State: map[string]any{}}
	n := &model.Node{Type: "aspectratio", ID: "ar", Props: map[string]any{"ratio": float64(2)}}
	w, h := AspectRatio{}.Measure(n, rt, nil, 1)
	if w != 100 || h != 50 {
		t.Fatalf("16:9-style 2:1 box = %dx%d, want 100x50", w, h)
	}
}

func TestCircularProgressDeterminate(t *testing.T) {
	rt := &runtime.Runtime{State: map[string]any{}}
	n := &model.Node{Type: "circularprogress", ID: "cp", Props: map[string]any{"value": float64(0.5)}}
	ln := &canvas.LayoutNode{Node: n, Width: 40, Height: 40}
	node := CircularProgress{}.Record(ln, rt, 1)
	if node == nil {
		t.Fatal("Record returned nil")
	}
	// Group should have track beads + progress beads.
	if len(node.Base().Children) < 10 {
		t.Errorf("expected many arc beads, got %d children", len(node.Base().Children))
	}
}

func TestIgnorePointerNoHit(t *testing.T) {
	rt := &runtime.Runtime{State: map[string]any{}}
	child := &model.Node{Type: "text", ID: "t", Text: "x"}
	n := &model.Node{Type: "ignorepointer", ID: "ip", Children: []*model.Node{child}}
	// Measure child into a layout node the way the engine would.
	cln := canvas.Measure(child, rt, nil, 1)
	if cln == nil {
		t.Fatal("child measure nil")
	}
	ln := &canvas.LayoutNode{
		Node: n, Width: 40, Height: 20,
		Children: []*canvas.LayoutNode{cln},
		AbsX:     0, AbsY: 0,
	}
	node := IgnorePointer{}.Record(ln, rt, 1)
	if node == nil || !node.Base().NoHit {
		t.Fatalf("ignorepointer group must be NoHit, got %#v", node)
	}
	_ = image.Point{}
}
