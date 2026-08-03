package widgets

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// A non-empty selection renders as per-line highlight rects behind the
// selected runes (each line intersected with the selection) and hides the
// caret. Buffer "ab\ncd" with selection [1,4) selects "b" on line 0 and "c"
// on line 1.
func TestTextareaSelectionRendersHighlight(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	ta := &model.Node{Type: "textarea", ID: "ta"}

	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	inter := &canvas.Interaction{Input: &canvas.InputState{
		Node: ta, Runes: []rune("ab\ncd"), Cursor: 4, Anchor: 1, SelStart: 1, SelEnd: 4,
	}}
	tw.mu.Lock()
	tw.inters[ta] = inter
	tw.mu.Unlock()
	ln := &canvas.LayoutNode{Node: ta, Width: 160, Height: 72}
	shape := tw.Record(ln, rt, 1)

	var highlights []*draw.Rect
	hasCaret := false
	var walk func(n draw.Node)
	walk = func(n draw.Node) {
		if r, ok := n.(*draw.Rect); ok {
			if r.NoHit && r.StrokeWidth == 0 && r.Fill.A > 0 {
				if r.Width > 1 {
					highlights = append(highlights, r)
				} else {
					hasCaret = true
				}
			}
		}
		if g, ok := n.(*draw.Group); ok {
			for _, c := range g.Children {
				walk(c)
			}
		}
	}
	walk(shape)

	if hasCaret {
		t.Error("caret must be hidden while a selection is active")
	}
	if len(highlights) != 2 {
		t.Fatalf("selection highlights = %d, want 2 (line 0 col1, line 1 col0)", len(highlights))
	}
	// Line 0: the selected "b" starts at pad + width("a").
	got := highlights[0]
	wantX := float64(12 + int(canvas.MeasureText("a", 14)))
	if got.X != wantX || got.Y != float64(12) {
		t.Errorf("line0 highlight = (%v,%v), want (%v,%v)", got.X, got.Y, wantX, 12)
	}
	if got.Width != float64(int(canvas.MeasureText("b", 14))) {
		t.Errorf("line0 highlight width = %v, want width(b)", got.Width)
	}
	// Line 1: the selected "c" starts at pad, one row down.
	got2 := highlights[1]
	if got2.X != float64(12) || got2.Y != float64(12+lineHeight(14)) {
		t.Errorf("line1 highlight = (%v,%v), want (12,%v)", got2.X, got2.Y, 12+lineHeight(14))
	}
}

// The per-line buffer span is counted in RUNES, not bytes: with a multi-byte
// rune on line 0 ("héllo"), selecting the whole second line must highlight all
// of "world" — byte-based lineStart would shift the span and clip the trailing
// rune.
func TestTextareaSelectionNonASCIILines(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	ta := &model.Node{Type: "textarea", ID: "ta"}

	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	buf := []rune("héllo\nworld")
	inter := &canvas.Interaction{Input: &canvas.InputState{
		Node: ta, Runes: buf, Cursor: 11, Anchor: 6, SelStart: 6, SelEnd: 11,
	}}
	tw.mu.Lock()
	tw.inters[ta] = inter
	tw.mu.Unlock()
	shape := tw.Record(&canvas.LayoutNode{Node: ta, Width: 200, Height: 72}, rt, 1)

	var highlights []*draw.Rect
	var walk func(n draw.Node)
	walk = func(n draw.Node) {
		if r, ok := n.(*draw.Rect); ok && r.NoHit && r.StrokeWidth == 0 && r.Fill.A > 0 && r.Width > 1 {
			highlights = append(highlights, r)
		}
		if g, ok := n.(*draw.Group); ok {
			for _, c := range g.Children {
				walk(c)
			}
		}
	}
	walk(shape)
	if len(highlights) != 1 {
		t.Fatalf("selection highlights = %d, want 1 (line 1)", len(highlights))
	}
	// The highlight starts at the line's left edge and spans ALL of "world".
	got := highlights[0]
	if got.X != float64(12) || got.Y != float64(12+lineHeight(14)) {
		t.Errorf("highlight = (%v,%v), want (12,%v)", got.X, got.Y, 12+lineHeight(14))
	}
	if got.Width != float64(int(canvas.MeasureText("world", 14))) {
		t.Errorf("highlight width = %v, want width(world) = %v", got.Width, int(canvas.MeasureText("world", 14)))
	}
}

// A textarea taller than its box scrolls its content so the caret stays
// visible: a caret on a line below the fold advances the persisted offset and
// the content group shifts up by it.
func TestTextareaCaretAutoScroll(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	ta := &model.Node{Type: "textarea", ID: "ta"}
	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	inter := &canvas.Interaction{Input: &canvas.InputState{
		Node: ta, Runes: []rune("l1\nl2\nl3\nl4\nl5\nl6"), Cursor: 21, // on the last line
	}}
	tw.mu.Lock()
	tw.inters[ta] = inter
	tw.mu.Unlock()
	// A 2-row box over 6 lines of the default fs (14): lineH = lineHeight(14),
	// boxH = Height - 2*pad, content extent = pad + 6*lineH.
	lineH := lineHeight(14)
	boxH := 62 - 2*textareaPad
	wantOffset := textareaPad + 6*lineH - boxH // scroll so the last line's bottom sits at the box bottom
	ln := &canvas.LayoutNode{Node: ta, Width: 200, Height: 62}
	shape := tw.Record(ln, rt, 1)

	// The content group (the root's last child) is scrolled so the caret line
	// is fully visible.
	root := shape.(*draw.Group)
	var content *draw.Group
	for _, c := range root.Children {
		if gr, ok := c.(*draw.Group); ok {
			content = gr
		}
	}
	if content == nil {
		t.Fatal("content group missing")
	}
	if content.Y != -float64(wantOffset) {
		t.Errorf("content.Y = %v, want %v (scrolled so the last line is visible)", content.Y, -wantOffset)
	}
	// The offset is persisted for the next measure.
	if pos := inter.ScrollOffsets[ta]; pos.Y != float64(wantOffset) {
		t.Errorf("persisted offset = %v, want %v", pos.Y, wantOffset)
	}
}

// Keyboard focus (Tab) reaches the textarea through the FocusHookWidget seam
// — the Tab path never routes through HandlePointer — so a Tab-focused
// textarea caches the interaction and its session is visible to Record.
func TestTextareaTabFocusCachesSession(t *testing.T) {
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}"}
	e, surf := formEngine(t, ta)
	e.DrawFrame(surf)
	e.HandleKey(canvas.KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != ta {
		t.Fatal("tab must focus the textarea")
	}
	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	tw.mu.Lock()
	cached := tw.inters[ta]
	tw.mu.Unlock()
	if cached == nil || cached.Input == nil {
		t.Fatal("tab-focus must cache the interaction + session for the widget's Record")
	}
}

// The shared session drives both the caret and the selection through the same
// fields the engine writes: this pins that a collapsed selection draws the
// caret again (the pre-selection behavior).
func TestTextareaCollapsedSelectionShowsCaret(t *testing.T) {
	rt := runtime.New(&model.App{})
	rt.Theme = theme.GetDefault()
	ta := &model.Node{Type: "textarea", ID: "ta"}

	w, _ := canvas.LookupWidget("textarea")
	tw := w.(*Textarea)
	inter := &canvas.Interaction{Input: &canvas.InputState{
		Node: ta, Runes: []rune("ab\ncd"), Cursor: 4, Anchor: 4, SelStart: 4, SelEnd: 4,
	}}
	tw.mu.Lock()
	tw.inters[ta] = inter
	tw.mu.Unlock()
	shape := tw.Record(&canvas.LayoutNode{Node: ta, Width: 160, Height: 72}, rt, 1)

	found := false
	var walk func(n draw.Node)
	walk = func(n draw.Node) {
		if r, ok := n.(*draw.Rect); ok && r.NoHit && r.Width == 1 && r.StrokeWidth == 0 {
			found = true
		}
		if g, ok := n.(*draw.Group); ok {
			for _, c := range g.Children {
				walk(c)
			}
		}
	}
	walk(shape)
	if !found {
		t.Error("a collapsed selection must still paint the caret")
	}
}
