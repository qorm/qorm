package canvas

// Tests for the W9 loosening of the edit-session routing: a focused node
// whose type is the REGISTERED textarea widget shares the input's edit
// session (input.go editableType), multi-line keys edit (return = newline,
// up/down = visual-line moves), and a widget-driven focus change funnels
// through syncEditSession (engine.go). The real textarea lives in
// internal/widgets and cannot be imported here (it would be an import
// cycle), so a minimal fake InteractiveWidget stands in — the same seam an
// app's custom component takes (widget_seam_test.go precedent).

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// fakeTextarea is a minimal InteractiveWidget: a press focuses the node,
// exactly what the real textarea's HandlePointer does.
type fakeTextarea struct{}

func (fakeTextarea) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (int, int) {
	return 160 * scale, 80 * scale
}

func (fakeTextarea) Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	r := graph.NewRect()
	r.Width, r.Height = float64(ln.Width), float64(ln.Height)
	r.Fill = color.RGBA{232, 232, 237, 255}
	return r
}

func (fakeTextarea) HandlePointer(n *model.Node, rt *runtime.Runtime, p PointerInput, inter *Interaction, _ image.Rectangle) bool {
	if p.Type != PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	return true
}

// textareaEditFixture builds a headless engine around one registered fake
// textarea bound to state.note.
func textareaEditFixture(t *testing.T) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	prev, had := LookupWidget("textarea")
	RegisterWidget("textarea", fakeTextarea{})
	t.Cleanup(func() {
		if had {
			RegisterWidget("textarea", prev)
		} else {
			UnregisterWidget("textarea")
		}
	})
	ta := &model.Node{Type: "textarea", ID: "ta", Value: "{{state.note}}"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{ta}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 300))
	return NewEngine(rt, SoftwareRenderer{}), surf, ta
}

// The gate: input always qualifies, textarea only once the widget library
// registered it, anything else never.
func TestEditableTypeGate(t *testing.T) {
	if !editableType("input") {
		t.Error("input must always open an edit session")
	}
	_, registered := LookupWidget("textarea")
	if got := editableType("textarea"); got != registered {
		t.Errorf("editableType(textarea) = %v, want the registry membership %v", got, registered)
	}
	prev, had := LookupWidget("textarea")
	RegisterWidget("textarea", fakeTextarea{})
	t.Cleanup(func() {
		if had {
			RegisterWidget("textarea", prev)
		} else {
			UnregisterWidget("textarea")
		}
	})
	if !editableType("textarea") {
		t.Error("a registered textarea must qualify")
	}
	if editableType("nosuchtype") {
		t.Error("an unknown type must never qualify")
	}
}

// A press on the registered textarea focuses it AND opens the edit session —
// the engine funnels the widget-driven focus change through syncEditSession.
// The press must also reach the dirty flag or the frame never repaints.
func TestTextareaPressOpensEditSession(t *testing.T) {
	e, surf, ta := textareaEditFixture(t)
	e.RT.State["note"] = "ab\ncd"
	e.DrawFrame(surf)

	e.dirty.Store(false)
	e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})
	if e.Inter.Focused != ta {
		t.Fatalf("press must focus the textarea, focused=%v", e.Inter.Focused)
	}
	s := e.Inter.Input
	if s == nil || s.Node != ta {
		t.Fatal("press must open the edit session via the engine funnel")
	}
	if got := string(s.Runes); got != "ab\ncd" {
		t.Errorf("session buffer = %q, want the bound value %q", got, "ab\ncd")
	}
	if s.Cursor != 5 {
		t.Errorf("cursor = %d, want at the end (5)", s.Cursor)
	}
	if !e.Dirty() {
		t.Error("a widget's redraw return must reach the dirty flag")
	}

	// Pressing blank space blurs and closes the session (HTML parity).
	e.HandlePointer(PointerInput{Type: PointerPress, X: 350, Y: 280})
	if e.Inter.Focused != nil || e.Inter.Input != nil {
		t.Error("blank press must blur the textarea and close its session")
	}
}

// return inserts a newline (committed to state per keystroke); up/down move
// the cursor by visual line, keeping the column where the line is long
// enough.
func TestTextareaMultiLineKeys(t *testing.T) {
	e, surf, _ := textareaEditFixture(t)
	e.RT.State["note"] = "ab\ncd"
	e.DrawFrame(surf)
	e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})

	s := e.Inter.Input
	if s == nil {
		t.Fatal("session must be open")
	}
	// Cursor 5 (end of "cd"). Up → line 0, column 2 → cursor 2.
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if s.Cursor != 2 {
		t.Fatalf("up: cursor = %d, want 2", s.Cursor)
	}
	// Down back to line 1, column clamps to the line length 2 → cursor 5.
	e.HandleKey(KeyInput{Key: "down", Down: true})
	if s.Cursor != 5 {
		t.Fatalf("down: cursor = %d, want 5", s.Cursor)
	}
	// Down on the last line clamps to the buffer end (already there).
	e.HandleKey(KeyInput{Key: "down", Down: true})
	if s.Cursor != 5 {
		t.Fatalf("down on last line: cursor = %d, want 5", s.Cursor)
	}
	// return at cursor 5 appends a newline; write-back lands per keystroke.
	e.HandleKey(KeyInput{Key: "return", Down: true})
	if got := e.RT.State["note"]; got != "ab\ncd\n" {
		t.Fatalf("state.note = %q, want %q", got, "ab\ncd\n")
	}
	if s.Cursor != 6 {
		t.Fatalf("cursor after newline = %d, want 6", s.Cursor)
	}
	// Cursor 6 is column 0 of the empty last line; up lands at the start of
	// "cd" (column 0), up again at the start of "ab".
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if s.Cursor != 3 {
		t.Fatalf("up to middle line: cursor = %d, want 3", s.Cursor)
	}
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if s.Cursor != 0 {
		t.Fatalf("up to first line: cursor = %d, want 0", s.Cursor)
	}
	// Up on the first line clamps to the buffer start (no-op here).
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if s.Cursor != 0 {
		t.Fatalf("up on first line: cursor = %d, want 0", s.Cursor)
	}
	// Down restores the middle line; the column rides along when it fits.
	e.HandleKey(KeyInput{Key: "down", Down: true})
	e.HandleKey(KeyInput{Key: "right", Down: true})
	e.HandleKey(KeyInput{Key: "right", Down: true})
	if s.Cursor != 5 {
		t.Fatalf("right along middle line: cursor = %d, want 5", s.Cursor)
	}
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if s.Cursor != 2 {
		t.Fatalf("up keeps column 2: cursor = %d, want 2", s.Cursor)
	}
	// Left/right cross the newline rune by rune (HTML parity).
	e.HandleKey(KeyInput{Key: "right", Down: true})
	if s.Cursor != 3 {
		t.Fatalf("right across newline: cursor = %d, want 3", s.Cursor)
	}
	// Backspace deletes the newline itself.
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := e.RT.State["note"]; got != "abcd\n" {
		t.Fatalf("after deleting newline state.note = %q, want %q", got, "abcd\n")
	}
}

// A single-line input must NOT eat return/up/down: those keep their engine
// semantics (activation dispatch / nothing) — the multi-line keys are gated
// on the session being a textarea.
func TestInputStillIgnoresMultiLineKeys(t *testing.T) {
	in := &model.Node{
		Type: "input", ID: "in1", Value: "{{state.name}}",
		OnPress: &model.Invoke{Name: "submit"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{in}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	app.Actions = map[string]*model.Action{
		"submit": {ID: "submit", Steps: []model.Step{{Type: "state.set", Path: "sent", Value: "yes"}}},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 300))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})
	if e.Inter.Input == nil {
		t.Fatal("press must open the input session")
	}
	if e.handleEditKey(KeyInput{Key: "return", Down: true}) {
		t.Error("a single-line input must not consume return")
	}
	if e.handleEditKey(KeyInput{Key: "up", Down: true}) {
		t.Error("a single-line input must not consume up")
	}
	// return falls through to the activation dispatch (engine.go HandleKey).
	e.HandleKey(KeyInput{Key: "return", Down: true})
	if got := e.RT.State["sent"]; got != "yes" {
		t.Errorf("return must still dispatch the input's onPress, sent = %v", got)
	}
}
