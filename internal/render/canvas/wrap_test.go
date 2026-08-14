package canvas

// Text-wrap engine tests (wrap.go): a long text in a column folds into
// multiple lines, the column's height grows by the folded lines, the folded
// lines are what reaches the graph, and CJK text hard-breaks per rune. Rows
// and scroll viewports keep their measured boxes.

import (
	"image"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func wrapEngine(t *testing.T, root *model.Node, w, h int) (*Engine, *HeadlessSurface) {
	t.Helper()
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	return e, NewHeadlessSurface(image.Pt(w, h))
}

func TestTextWrapsToColumnWidth(t *testing.T) {
	long := strings.Repeat("wrap me around the column edge ", 12)
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "p", Props: map[string]any{"text": long}, Style: map[string]any{"fontSize": 14.0}},
		{Type: "text", ID: "after", Props: map[string]any{"text": "after"}, Style: map[string]any{"fontSize": 14.0}},
	}}
	e, surf := wrapEngine(t, root, 300, 800)
	e.DrawFrame(surf)

	ln := Measure(root, e.RT, nil, 1)
	wrapTree(ln, 300)
	p := ln.Children[0]
	if len(p.Wrapped) < 2 {
		t.Fatalf("text did not wrap (Wrapped=%v)", p.Wrapped)
	}
	for i, line := range p.Wrapped {
		if w := int(MeasureText(line, 14)); w > 300 {
			t.Errorf("line %d is %dpx wide, over the 300 column", i, w)
		}
	}
	if got, want := p.Height, len(p.Wrapped)*textLineH(14); got != want {
		t.Errorf("wrapped text height = %d, want %d lines × %d", got, len(p.Wrapped), textLineH(14))
	}
	// The sibling must move down by the folded height (column recomputed).
	if ln.Height <= p.Height {
		t.Errorf("column height %d did not grow past the folded text %d", ln.Height, p.Height)
	}
	// A scroll root keeps its viewport box; only ContentH grows.
	sc := &model.Node{Type: "scroll", ID: "sc", Style: map[string]any{"height": 100.0}, Children: root.Children}
	ln2 := Measure(sc, e.RT, nil, 1)
	wrapTree(ln2, 300)
	if ln2.Height != 100 {
		t.Errorf("scroll viewport height = %d, want its explicit 100", ln2.Height)
	}
	if ln2.ContentH <= 100 {
		t.Errorf("scroll ContentH = %d, want the folded content height", ln2.ContentH)
	}
}

func TestTextWrapCJKBreaksPerRune(t *testing.T) {
	// 20 CJK chars at 14px ≈ 280px+ — must fold without any space.
	cjk := strings.Repeat("界面渲染引擎", 4)
	lines := wrapText(cjk, 14, 120)
	if len(lines) < 2 {
		t.Fatalf("CJK text did not hard-break: %v", lines)
	}
	for i, line := range lines {
		if w := int(MeasureText(line, 14)); w > 120 {
			t.Errorf("CJK line %d is %dpx, over 120", i, w)
		}
	}
	if got := strings.Join(lines, ""); got != cjk {
		t.Errorf("folding lost runes: %q != %q", got, cjk)
	}
}

func TestTextWrapSkipsRowsAndShortText(t *testing.T) {
	short := "fits"
	if lines := wrapText(short, 14, 300); lines != nil {
		t.Errorf("short text wrapped: %v", lines)
	}
	row := &model.Node{Type: "row", ID: "r", Children: []*model.Node{
		{Type: "text", ID: "p", Props: map[string]any{"text": strings.Repeat("row child stays one line ", 10)}, Style: map[string]any{"fontSize": 14.0}},
	}}
	e, _ := wrapEngine(t, row, 300, 800)
	ln := Measure(row, e.RT, nil, 1)
	wrapTree(ln, 300)
	if ln.Children[0].Wrapped != nil {
		t.Error("row child must not wrap in v1")
	}
}

func TestEllipsizeTextFitsUnchanged(t *testing.T) {
	s := ellipsizeText("hi", 14, 0, 200)
	if s != "hi" {
		t.Fatalf("got %q", s)
	}
}

func TestEllipsizeTextTruncates(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz"
	s := ellipsizeText(long, 14, 0, 40)
	if s == long {
		t.Fatal("expected truncation")
	}
	if int(MeasureText(s, 14)) > 40 {
		t.Fatalf("ellipsized width %v exceeds 40", MeasureText(s, 14))
	}
	if len([]rune(s)) >= len([]rune(long)) {
		t.Fatalf("expected fewer runes, got %q", s)
	}
}

func TestMultilineEllipsisInLayout(t *testing.T) {
	// Tall text forced into a short box with textOverflow:ellipsis should
	// only paint as many lines as height allows.
	long := strings.Repeat("line of wrapping prose ", 20)
	txt := &model.Node{Type: "text", ID: "p", Props: map[string]any{"text": long},
		Style: map[string]any{
			"fontSize": 14.0, "width": 120.0, "height": 36.0, // ~2 lines at 14*1.2
			"textOverflow": "ellipsis",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{txt}}
	e, surf := wrapEngine(t, root, 200, 200)
	e.DrawFrame(surf)
	g := e.findGroupByModel(txt)
	if g == nil {
		t.Fatal("missing text group")
	}
	nText := 0
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		if _, ok := n.(*graph.Text); ok {
			nText++
		}
		for _, c := range n.Base().Children {
			walk(c)
		}
	}
	walk(g)
	if nText < 1 || nText > 2 {
		t.Fatalf("expected 1–2 text lines for height 36, got %d", nText)
	}
}
