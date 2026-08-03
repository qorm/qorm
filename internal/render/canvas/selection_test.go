package canvas

import (
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
)

// setFakeClip installs an in-memory clipboard seam and returns a pointer to
// its current text for assertions.
func setFakeClip(t *testing.T) *string {
	t.Helper()
	s := ""
	SetClipboard(ClipboardFunc{
		GetFn: func() string { return s },
		SetFn: func(x string) { s = x },
	})
	t.Cleanup(func() { SetClipboard(nil) })
	return &s
}

// Shift+arrows extend the selection from an anchor fixed at the old caret; a
// plain move collapses it. The normalized range follows (anchor, cursor).
func TestInputShiftArrowSelection(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello world")

	// cursor is at 11; three Shift+Left select "d l"? No — the tail backwards:
	s := e.Inter.Input
	s.Cursor = 11
	e.HandleKey(KeyInput{Key: "left", Shift: true, Down: true})
	if s.Cursor != 10 || s.SelStart != 10 || s.SelEnd != 11 {
		t.Fatalf("after 1 Shift+Left: cur=%d sel=[%d,%d), want (10,10,11)", s.Cursor, s.SelStart, s.SelEnd)
	}
	e.HandleKey(KeyInput{Key: "left", Shift: true, Down: true})
	if s.SelStart != 9 || s.SelEnd != 11 {
		t.Fatalf("after 2 Shift+Left: sel=[%d,%d), want [9,11)", s.SelStart, s.SelEnd)
	}
	// Shift+Right folds back toward the anchor without losing it.
	e.HandleKey(KeyInput{Key: "right", Shift: true, Down: true})
	if s.SelStart != 10 || s.SelEnd != 11 {
		t.Fatalf("after Shift+Right: sel=[%d,%d), want [10,11)", s.SelStart, s.SelEnd)
	}
	// A plain move collapses.
	e.HandleKey(KeyInput{Key: "left", Down: true})
	if s.SelStart != s.SelEnd || s.Cursor != 9 {
		t.Fatalf("plain move must collapse: sel=[%d,%d) cursor=%d, want collapsed@9", s.SelStart, s.SelEnd, s.Cursor)
	}
}

// Cmd+A selects the whole buffer; typing then replaces the selection.
func TestInputSelectAllAndType(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "abc")
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	s := e.Inter.Input
	if s.SelStart != 0 || s.SelEnd != 3 {
		t.Fatalf("Cmd+A sel=[%d,%d), want [0,3)", s.SelStart, s.SelEnd)
	}
	typeRunes(e, "X")
	if got := e.RT.State["name"]; got != "X" {
		t.Errorf("typing over selection: state.name = %v, want %q", got, "X")
	}
}

// Home/End move to the buffer ends; Shift+Home selects the whole field.
func TestInputHomeEndWithShift(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello")
	e.HandleKey(KeyInput{Key: "home", Down: true})
	if c := e.Inter.Input.Cursor; c != 0 {
		t.Fatalf("home cursor = %d, want 0", c)
	}
	e.HandleKey(KeyInput{Key: "end", Down: true})
	if c := e.Inter.Input.Cursor; c != 5 {
		t.Fatalf("end cursor = %d, want 5", c)
	}
	e.HandleKey(KeyInput{Key: "home", Shift: true, Down: true})
	if s := e.Inter.Input; s.SelStart != 0 || s.SelEnd != 5 {
		t.Fatalf("shift+home sel=[%d,%d), want [0,5)", s.SelStart, s.SelEnd)
	}
}

// deleteForward removes the rune AFTER the caret (fn+delete); with a selection
// it deletes the selection, like backspace.
func TestInputDeleteForward(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "abc")
	e.Inter.Input.Cursor = 1
	e.HandleKey(KeyInput{Key: "deleteForward", Down: true})
	if got := e.RT.State["name"]; got != "ac" {
		t.Errorf("deleteForward: state.name = %v, want %q", got, "ac")
	}
	// With a selection it deletes the selection first.
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	e.HandleKey(KeyInput{Key: "deleteForward", Down: true})
	if got := e.RT.State["name"]; got != "" {
		t.Errorf("deleteForward over selection: state.name = %v, want empty", got)
	}
}

// Backspace with an active selection deletes the selection, not one rune.
func TestInputBackspaceReplacesSelection(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello")
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := e.RT.State["name"]; got != "" {
		t.Errorf("backspace over selection: state.name = %v, want empty", got)
	}
}

// Cmd+Left/Right jump by word (wordStart/wordEnd across separators).
func TestInputWordNav(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello world foo")
	e.HandleKey(KeyInput{Key: "left", Meta: true, Down: true})
	if c := e.Inter.Input.Cursor; c != 12 {
		t.Fatalf("cmd+left to word start = %d, want 12", c)
	}
	e.HandleKey(KeyInput{Key: "left", Meta: true, Down: true})
	if c := e.Inter.Input.Cursor; c != 6 {
		t.Fatalf("cmd+left twice = %d, want 6", c)
	}
	e.HandleKey(KeyInput{Key: "right", Meta: true, Down: true})
	if c := e.Inter.Input.Cursor; c != 12 {
		t.Fatalf("cmd+right = %d, want 12 (start of \"foo\": skips the word + separator)", c)
	}
}

// Cmd+C copies the selection; Cmd+V pastes it at the caret.
func TestInputClipboardCopyPaste(t *testing.T) {
	clip := setFakeClip(t)
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello")
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	e.HandleKey(KeyInput{Key: "c", Meta: true, Down: true})
	if *clip != "hello" {
		t.Fatalf("copy: clipboard = %q, want %q", *clip, "hello")
	}
	// Collapse the selection to the start, then paste: the copy is appended
	// before the original text.
	e.HandleKey(KeyInput{Key: "home", Down: true})
	e.HandleKey(KeyInput{Key: "v", Meta: true, Down: true})
	if got := e.RT.State["name"]; got != "hellohello" {
		t.Errorf("paste: state.name = %v, want %q", got, "hellohello")
	}
}

// Cmd+X copies then deletes the selection.
func TestInputClipboardCut(t *testing.T) {
	clip := setFakeClip(t)
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello world")
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	e.HandleKey(KeyInput{Key: "x", Meta: true, Down: true})
	if *clip != "hello world" {
		t.Fatalf("cut: clipboard = %q, want %q", *clip, "hello world")
	}
	if got := e.RT.State["name"]; got != "" {
		t.Errorf("cut: state.name = %v, want empty", got)
	}
}

// Paste over a selection replaces it.
func TestInputPasteReplacesSelection(t *testing.T) {
	setFakeClip(t)
	SetClipboard(ClipboardFunc{GetFn: func() string { return "X" }})
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "hello")
	s := e.Inter.Input
	s.Cursor = 2
	s.Anchor = 1
	s.SelStart, s.SelEnd = 1, 2
	e.HandleKey(KeyInput{Key: "v", Meta: true, Down: true})
	if got := e.RT.State["name"]; got != "hXllo" {
		t.Errorf("paste over selection: state.name = %v, want %q", got, "hXllo")
	}
}

// A single-line input folds pasted newlines to spaces (HTML <input> parity).
func TestInputPasteStripsNewlines(t *testing.T) {
	SetClipboard(ClipboardFunc{GetFn: func() string { return "a\nb\r\nc" }})
	t.Cleanup(func() { SetClipboard(nil) })
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	e.HandleKey(KeyInput{Key: "v", Meta: true, Down: true})
	if got := e.RT.State["name"]; got != "a b  c" {
		t.Errorf("paste folds newlines: state.name = %v, want %q", got, "a b  c")
	}
}

// resetClick ages out the double-click detector so the next press is a fresh
// single click (the click that opens the session would otherwise merge into a
// double-click with the very next press — both are microseconds apart).
func resetClick(e *Engine) {
	e.Inter.Click = &ClickDetector{lastTime: time.Now().Add(-time.Second)}
}

// A click positions the caret at the clicked character, not the buffer end.
func TestInputClickPositionsCaret(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hello world")
	m := e.inputMetricsFromGraph(in)
	if m == nil {
		t.Fatal("no input metrics")
	}
	resetClick(e)
	target := float64(m.TextX) + MeasureText("hello", m.FontSize)
	e.HandlePointer(PointerInput{Type: PointerPress, X: target, Y: float64(m.TextY)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: target, Y: float64(m.TextY)})
	if c := e.Inter.Input.Cursor; c != 5 {
		t.Fatalf("caret after click at end of \"hello\" = %d, want 5", c)
	}
}

// Press-drag selects the range from the press anchor to the pointer.
func TestInputDragSelects(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hello world")
	m := e.inputMetricsFromGraph(in)
	if m == nil {
		t.Fatal("no input metrics")
	}
	resetClick(e)
	start := float64(m.TextX) + 1 // just right of the text origin → index 0
	end := float64(m.TextX) + MeasureText("hello world", m.FontSize)
	e.HandlePointer(PointerInput{Type: PointerPress, X: start, Y: float64(m.TextY), Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: end, Y: float64(m.TextY), Buttons: 1})
	if s := e.Inter.Input; s.SelStart != 0 || s.SelEnd != 11 {
		t.Fatalf("drag selection = [%d,%d), want [0,11)", s.SelStart, s.SelEnd)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: end, Y: float64(m.TextY)})
	if e.Inter.Input.Selecting {
		t.Fatal("release must end the drag selection")
	}
}

// A double-click selects the word under the pointer.
func TestInputDoubleClickSelectsWord(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hello world")
	m := e.inputMetricsFromGraph(in)
	if m == nil {
		t.Fatal("no input metrics")
	}
	resetClick(e)
	// Double-click in the middle of "hello": the word is [0,5).
	spot := float64(m.TextX) + MeasureText("he", m.FontSize)
	for i := 0; i < 2; i++ {
		e.HandlePointer(PointerInput{Type: PointerPress, X: spot, Y: float64(m.TextY), Buttons: 1})
		e.HandlePointer(PointerInput{Type: PointerRelease, X: spot, Y: float64(m.TextY)})
	}
	if s := e.Inter.Input; s.SelStart != 0 || s.SelEnd != 5 {
		t.Fatalf("double-click selection = [%d,%d), want the word [0,5)", s.SelStart, s.SelEnd)
	}
}

// A triple-click selects the whole single-line field.
func TestInputTripleClickSelectsAll(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hello world")
	m := e.inputMetricsFromGraph(in)
	if m == nil {
		t.Fatal("no input metrics")
	}
	resetClick(e)
	spot := float64(m.TextX) + MeasureText("h", m.FontSize)
	for i := 0; i < 3; i++ {
		e.HandlePointer(PointerInput{Type: PointerPress, X: spot, Y: float64(m.TextY), Buttons: 1})
		e.HandlePointer(PointerInput{Type: PointerRelease, X: spot, Y: float64(m.TextY)})
	}
	if s := e.Inter.Input; s.SelStart != 0 || s.SelEnd != 11 {
		t.Fatalf("triple-click selection = [%d,%d), want the whole field [0,11)", s.SelStart, s.SelEnd)
	}
}

// A non-empty selection renders as a highlight rect spanning the selected
// runes and hides the caret.
func TestInputSelectionRendersHighlight(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hello")
	s := e.Inter.Input
	s.Cursor, s.Anchor = 4, 1
	s.SelStart, s.SelEnd = 1, 4
	e.MarkDirty()
	e.DrawFrame(surf)

	g := inputGroup(t, e, in)
	var caret, hi *graph.Rect
	for _, c := range g.Children {
		r, ok := c.(*graph.Rect)
		if !ok || !r.Base().NoHit || r.Fill.A == 0 || r.StrokeWidth != 0 {
			continue
		}
		if r.Width <= 1 {
			caret = r
		} else {
			hi = r
		}
	}
	if caret != nil {
		t.Error("caret must be hidden while a selection is active")
	}
	if hi == nil {
		t.Fatal("selection highlight not rendered")
	}
	// The highlight starts at the selected runes' left edge (the text origin
	// plus the pre-selection advance) and spans them.
	text := firstText(g)
	fs := text.FontSize
	wantX := int(text.X) + int(MeasureText(prefixRunes("hello", 1), fs))
	wantW := int(MeasureText(prefixRunes("hello", 4), fs)) - (wantX - int(text.X))
	if int(hi.X) != wantX || int(hi.Width) != wantW {
		t.Errorf("highlight = x%d w%d, want x%d w%d", int(hi.X), int(hi.Width), wantX, wantW)
	}
}

func (e *Engine) findModelByID(id string) *model.Node {
	return findModel(e.sceneRoot(), id)
}
