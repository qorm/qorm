package canvas

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
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

// Caret-from-pixel on a textarea whose buffer starts with an empty line: the
// first text run sits one line down, so the y→line mapping must anchor at the
// first line's true top, not the first run's.
func TestCaretIndexLeadingEmptyLine(t *testing.T) {
	s := &InputState{Runes: []rune("\nab")}
	lineH := 16 // int(14 * 1.2)
	m := &InputMetrics{TextX: 0, TextY: lineH, FontSize: 14, LineH: lineH, Multiline: true}
	// Click the top of the visible "ab" row → caret at buffer index 1 (the
	// line's first rune), not 0 (before the newline).
	if idx := caretIndexFromPointer(m, s, 0, float64(lineH)); idx != 1 {
		t.Fatalf("caret at the visible row = %d, want 1", idx)
	}
}

// Two rapid presses on DIFFERENT fields must never merge into a double-click:
// the detector is keyed by the editable.
func TestClickDetectorFieldIsolation(t *testing.T) {
	now := time.Now()
	d := &ClickDetector{}
	a, b := &model.Node{ID: "a"}, &model.Node{ID: "b"}
	if c := d.Register(a, geom.Point{X: 10, Y: 10}, now); c != 1 {
		t.Fatalf("first press = %d, want 1", c)
	}
	if c := d.Register(b, geom.Point{X: 10, Y: 10}, now); c != 1 {
		t.Fatalf("press on a different field must be a fresh single click, got %d", c)
	}
	if c := d.Register(a, geom.Point{X: 11, Y: 10}, now.Add(10*time.Millisecond)); c != 1 {
		t.Fatalf("returning to field a is also fresh, got %d", c)
	}
	if c := d.Register(a, geom.Point{X: 11, Y: 10}, now.Add(20*time.Millisecond)); c != 2 {
		t.Fatalf("same-field fast second click must be a double-click, got %d", c)
	}
}

// A press held beyond the duration without drifting is a long-press; a quick
// tap or a drag past the slop is not.
func TestLongPressDetector(t *testing.T) {
	now := time.Now()
	d := &LongPressDetector{}
	d.Press(geom.Point{X: 10, Y: 10}, now)
	if d.Release(geom.Point{X: 10, Y: 10}, now.Add(50*time.Millisecond)) {
		t.Error("a 50ms tap must not be a long-press")
	}
	d.Press(geom.Point{X: 10, Y: 10}, now)
	if !d.Release(geom.Point{X: 10, Y: 10}, now.Add(LongPressMinDuration+10*time.Millisecond)) {
		t.Error("a 500ms+ hold must be a long-press")
	}
	d.Press(geom.Point{X: 10, Y: 10}, now)
	if d.Release(geom.Point{X: 30, Y: 10}, now.Add(LongPressMinDuration+10*time.Millisecond)) {
		t.Error("a long hold that drifted past the slop is a drag, not a long-press")
	}
}

// The caret blinks: visible for the first half period after the session
// opens, hidden in the off half — and the engine keeps animating while an
// edit session is live so the frame loop actually repaints the toggle.
func TestCaretBlink(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "hi")
	e.DrawFrame(surf)

	// A just-opened session (phase anchored at open) shows its caret.
	g := inputGroup(t, e, in)
	if findCaret(g) == nil {
		t.Fatal("a just-opened session must show its caret")
	}
	if !e.Animating() {
		t.Fatal("a live edit session must keep the engine animating (blink needs frames)")
	}

	// Advance the blink phase into the off half (just past the half-period
	// boundary, leaving ~490ms of margin before it flips visible again, so a
	// slow CI run cannot land the render back in the on half): the caret hides.
	e.Inter.Input.BlinkStart = e.Inter.Input.BlinkStart.Add(-(caretBlinkHalf + 10*time.Millisecond))
	e.MarkDirty()
	e.DrawFrame(surf)
	if findCaret(inputGroup(t, e, in)) != nil {
		t.Error("the caret must hide during the off half of its blink")
	}
}

// Cmd+Z undoes the last edit, Cmd+Shift+Z redoes it; a fresh edit clears the
// redo history.
func TestInputUndoRedo(t *testing.T) {
	e, surf, _ := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, e.findModelByID("in1"))
	typeRunes(e, "abc")
	typeRunes(e, "X") // "abcX"
	e.HandleKey(KeyInput{Key: "z", Meta: true, Down: true})
	if got := e.RT.State["name"]; got != "abc" {
		t.Fatalf("after undo state.name = %v, want %q", got, "abc")
	}
	e.HandleKey(KeyInput{Key: "z", Meta: true, Down: true})
	if got := e.RT.State["name"]; got != "ab" {
		t.Fatalf("second undo state.name = %v, want %q", got, "ab")
	}
	e.HandleKey(KeyInput{Key: "z", Meta: true, Shift: true, Down: true})
	if got := e.RT.State["name"]; got != "abc" {
		t.Fatalf("redo state.name = %v, want %q", got, "abc")
	}
	// A fresh edit clears the redo history.
	typeRunes(e, "Y")
	e.HandleKey(KeyInput{Key: "z", Meta: true, Shift: true, Down: true})
	if got := e.RT.State["name"]; got != "abcY" {
		t.Fatalf("redo after a fresh edit must be a no-op, state.name = %v", got)
	}
}

// A numeric input (inputType: number) rejects non-numeric characters, allows
// one leading minus and one decimal point, and clamps complete values to
// min/max on the state write-back (the buffer keeps what was typed).
func TestInputNumberType(t *testing.T) {
	in := &model.Node{Type: "input", ID: "num", Value: "{{state.n}}",
		Props: map[string]any{"inputType": "number", "min": 0.0, "max": 1000.0}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	clickNode(t, e, in)
	typeRunes(e, "12a3") // "a" rejected
	if got := rt.State["n"]; got != "123" {
		t.Fatalf("non-numeric characters must be rejected: state.n = %v", got)
	}
	typeRunes(e, "-") // a minus mid-string is rejected
	if got := rt.State["n"]; got != "123" {
		t.Fatalf("a minus mid-string must be rejected: state.n = %v", got)
	}
	typeRunes(e, ".5") // one decimal point allowed
	if got := rt.State["n"]; got != "123.5" {
		t.Fatalf("a decimal point must be allowed: state.n = %v", got)
	}
	typeRunes(e, ".") // a second decimal point is rejected
	if got := string(e.Inter.Input.Runes); got != "123.5" {
		t.Fatalf("a second decimal point must be rejected: %q", got)
	}
	// A complete value above max clamps on the write-back; the buffer keeps
	// what was typed.
	e.HandleKey(KeyInput{Key: "a", Meta: true, Down: true})
	typeRunes(e, "9999")
	if got := rt.State["n"]; got != "1000" {
		t.Fatalf("an out-of-range value must clamp to max: state.n = %v", got)
	}
	if got := string(e.Inter.Input.Runes); got != "9999" {
		t.Fatalf("the buffer must keep what was typed: %q", got)
	}
}

// Pasting into a numeric field filters the text to its numeric characters.
func TestInputNumberPasteFilters(t *testing.T) {
	SetClipboard(ClipboardFunc{GetFn: func() string { return "1a2.5x" }})
	t.Cleanup(func() { SetClipboard(nil) })
	in := &model.Node{Type: "input", ID: "num", Value: "{{state.n}}",
		Props: map[string]any{"inputType": "number"}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	clickNode(t, e, in)
	e.HandleKey(KeyInput{Key: "v", Meta: true, Down: true})
	if got := rt.State["n"]; got != "12.5" {
		t.Fatalf("pasted text must be filtered to numeric characters: state.n = %v", got)
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
