package canvas

import (
	"image/color"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// This file is the canvas counterpart of the HTML path's <input>
// (render_input.go:270): a single-line text field with two-way state binding.
// The edit session it defines is SHARED with the registered textarea widget
// (internal/widgets, W9): syncEditSession opens it for either editable type,
// and handleEditKey grows the multi-line keys (newline, line-wise cursor)
// only for textarea sessions — the multi-line rendering lives in the widget.
//
// Semantics mirrored from the HTML renderer:
//   - value = interp(n.Value); when the value is empty the placeholder shows
//     instead, in the secondary text color (render_input.go:279-283);
//   - a value spelled {{state.x}} is a two-way binding: edits write back to
//     state.x on every keystroke. The HTML client does the same thing by
//     POSTing every [data-state] control's live value with each event
//     (app.js qorm()), alongside dispatching a declared onChange handler with
//     the new {value} — write-back and dispatch are parallel, not exclusive;
//   - the write goes through Runtime.SetStatePath, so the read-only computed
//     namespace refuses it exactly as it refuses state.set steps and MCP
//     set_state (runtime.go:638).
//
// The caret is a static 1px line, not a blinking one: blink needs a
// timer-driven redraw source and the engine only repaints on dirty/animating,
// so a blinking caret would require the host to tick on a clock. Deliberate
// wave-1 trade-off.

// InputState is one live edit session: the text buffer, the insertion cursor
// and the selection, all in runes. It is interaction state (Interaction.Input)
// for the same reason Pressed/Focused are — the scene graph is rebuilt every
// frame, so the session survives here, keyed by the stable model pointer.
//
// Selection invariants: SelStart <= SelEnd, both in [0, len(Runes)]; the
// selection is collapsed iff SelStart == SelEnd; Cursor is the ACTIVE (moving)
// endpoint and Anchor the stationary one, so a normalized selection is always
// SelStart = min(Anchor, Cursor), SelEnd = max(Anchor, Cursor). Selecting is
// pointer-lifetime only — true from a press on the field until the release (or
// a button-less move) that ends a press-drag selection; it does not survive
// blur (the session dies).
type InputState struct {
	Node      *model.Node
	Runes     []rune
	Cursor    int // active caret, [0, len(Runes)]
	SelStart  int // normalized selection start, [0, len(Runes)]
	SelEnd    int // normalized selection end, [SelStart, len(Runes)]
	Anchor    int // stationary endpoint while extending
	Selecting bool
	// BlinkStart anchors the caret-blink phase to the moment the session opens
	// (the caret is visible for the first half period), so a just-focused field
	// always shows its caret deterministically.
	BlinkStart time.Time
	// Undo/redo are the edit-history stacks (buffer snapshots, newest last).
	// Every mutating commit pushes the pre-edit buffer onto Undo and clears
	// Redo; Cmd+Z pops Undo onto Redo and restores, Cmd+Shift+Z / Cmd+Y the
	// reverse. Capped at maxUndoEntries so a long typing session stays bounded.
	Undo, Redo [][]rune
}

// maxUndoEntries caps the undo history per session (the browser keeps ~100;
// 50 is a sane bounded default).
const maxUndoEntries = 50

// caretVisible reports whether the blinking caret should draw at this moment.
// The engine keeps animating while an edit session is live (engine.go
// animating store), so this flips every caretBlinkHalf.
const caretBlinkHalf = 500 * time.Millisecond

func caretVisible(s *InputState, now time.Time) bool {
	return int(now.Sub(s.BlinkStart)/caretBlinkHalf)%2 == 0
}

// pushUndo records a pre-edit buffer snapshot and clears the redo stack (a
// fresh edit invalidates the redo history, like every text editor).
func pushUndo(s *InputState, snap []rune) {
	s.Undo = append(s.Undo, snap)
	if len(s.Undo) > maxUndoEntries {
		s.Undo = s.Undo[len(s.Undo)-maxUndoEntries:]
	}
	s.Redo = nil
}

// undoEdit restores the most recent undo snapshot (pushing the current buffer
// onto Redo) and reports whether anything was undone.
func undoEdit(s *InputState) bool {
	if len(s.Undo) == 0 {
		return false
	}
	last := s.Undo[len(s.Undo)-1]
	s.Undo = s.Undo[:len(s.Undo)-1]
	s.Redo = append(s.Redo, append([]rune(nil), s.Runes...))
	s.Runes = last
	s.Cursor = len(s.Runes)
	collapseSel(s)
	return true
}

// redoEdit restores the most recent redo snapshot (pushing the current buffer
// back onto Undo) and reports whether anything was redone.
func redoEdit(s *InputState) bool {
	if len(s.Redo) == 0 {
		return false
	}
	last := s.Redo[len(s.Redo)-1]
	s.Redo = s.Redo[:len(s.Redo)-1]
	s.Undo = append(s.Undo, append([]rune(nil), s.Runes...))
	s.Runes = last
	s.Cursor = len(s.Runes)
	collapseSel(s)
	return true
}

// minInputWidth is the content-box width (logical px) an empty input keeps so
// it stays a usable click target — browsers size an empty text field to ~20
// characters by default; 160px is in that spirit at the default font size.
const minInputWidth = 160

// stateValueBindRe matches a whole-string state binding — the same shape the
// HTML renderer's boundPath recognizes (render_widgets.go:223).
var stateValueBindRe = regexp.MustCompile(`^\s*\{\{\s*state\.([a-zA-Z0-9_.]+)\s*\}\}$`)

// boundStatePath returns the state path a value binding writes back to, or ""
// when the value is not a whole-string {{state.x}} binding.
func boundStatePath(value string) string {
	if m := stateValueBindRe.FindStringSubmatch(value); m != nil {
		return m[1]
	}
	return ""
}

// editSession returns the live edit session for n, or nil.
func editSession(inter *Interaction, n *model.Node) *InputState {
	if inter != nil && inter.Input != nil && inter.Input.Node == n {
		return inter.Input
	}
	return nil
}

// inputDisplayText resolves what an input shows: the edit buffer while a
// session is live, else the evaluated value, else the placeholder (flagged so
// PerformLayout can paint it in the secondary color). A live session with an
// empty buffer still shows the placeholder — never the stale bound value.
// A secure input masks its VALUE (never the placeholder) with one bullet per
// rune, like the browser's type=password.
func inputDisplayText(n *model.Node, rt *runtime.Runtime, inter *Interaction) (text string, placeholder bool) {
	mask := func(s string) string {
		return strings.Repeat("•", len([]rune(s)))
	}
	if s := editSession(inter, n); s != nil {
		if len(s.Runes) > 0 {
			if secureInput(n) {
				return mask(string(s.Runes)), false
			}
			return string(s.Runes), false
		}
		return n.Placeholder, true
	}
	if v := evalPropStr(n.Value, rt); v != "" {
		if secureInput(n) {
			return mask(v), false
		}
		return v, false
	}
	return n.Placeholder, true
}

// secureInput reports whether the input masks its value (HTML: type=password
// via the secure/password prop or a "password"-named id, render_input.go).
func secureInput(n *model.Node) bool {
	for _, k := range []string{"secure", "password"} {
		if v, ok := n.Prop(k); ok {
			switch t := v.(type) {
			case bool:
				if t {
					return true
				}
			case string:
				if t == "true" || t == "1" {
					return true
				}
			}
		}
	}
	return strings.Contains(strings.ToLower(n.ID), "password")
}

// keyText returns the text a key event contributes, if any. The host's Rune
// channel (full unicode, the future IME seam) wins; the named-key fallback
// keeps the current hosts — which only send normalized key names
// ("a".."z"/"0".."9", "space") — typing ASCII until they fill Rune.
func keyText(k KeyInput) (rune, bool) {
	if k.Rune > 0 && unicode.IsPrint(k.Rune) {
		return k.Rune, true
	}
	if k.Key == "space" {
		return ' ', true
	}
	if len(k.Key) == 1 {
		if r := rune(k.Key[0]); r >= 33 && r <= 126 {
			if k.Shift && r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			return r, true
		}
	}
	return 0, false
}

// activeEdit returns the live edit session, re-validating it against the
// current tree: the session dies when its node lost focus, became disabled,
// or was hidden by a condition flip (if/visible/show or a when branch) — the
// same re-validation the activation path does before dispatching OnPress
// (engine.go HandleKey).
func (e *Engine) activeEdit() *InputState {
	s := e.Inter.Input
	if s == nil {
		return nil
	}
	if e.Inter.Focused != s.Node || nodeDisabled(s.Node, e.RT) ||
		readonlyInput(s.Node, e.RT) || !nodeMounted(e.sceneRoot(), s.Node, e.RT) {
		e.Inter.Input = nil
		return nil
	}
	return s
}

// editableType reports whether a focused node of this type owns the keyboard
// as a text edit session: the engine's built-in input, or the registered
// textarea widget (internal/widgets). The registry lookup keeps a plain
// {"type":"textarea"} container inert when the widget library is not
// imported — the loosening from "type == input" to "input or registered
// textarea" (W9) cannot hijack an unregistered scene type.
func editableType(typ string) bool {
	if typ == "input" {
		return true
	}
	if typ != "textarea" {
		return false
	}
	_, ok := LookupWidget(typ)
	return ok
}

// syncEditSession opens an edit session when focus rests on an enabled input
// and closes it otherwise. Every focus change (pointer, tab, escape) funnels
// here so the buffer never outlives the focus that owns it. (A scene switch
// resets Interaction wholesale, engine.go RenderInto.)
// readonlyInput reports whether an editable is read-only (the HTML
// `readonly` prop, render_input.go): it stays focusable and keyboard-visible
// but no edit session opens — syncEditSession's gate below. The prop is
// evaluated like `disabled` (nodeDisabled), so a bound readonly flips live.
func readonlyInput(n *model.Node, rt *runtime.Runtime) bool {
	for _, k := range []string{"readonly", "readOnly"} {
		v, ok := n.Prop(k)
		if !ok {
			continue
		}
		switch t := evalStyleProp(v, rt).(type) {
		case bool:
			if t {
				return true
			}
		case string:
			if t == "true" || t == "1" {
				return true
			}
		}
	}
	return false
}

func (e *Engine) syncEditSession() {
	f := e.Inter.Focused
	if f != nil && editableType(f.Type) && !nodeDisabled(f, e.RT) && !readonlyInput(f, e.RT) {
		if s := e.Inter.Input; s == nil || s.Node != f {
			// The buffer starts from the evaluated value with the cursor at
			// the end and a collapsed selection, like clicking into an HTML
			// field (a pointer click repositions it, input.go caretIndexFromPointer).
			runes := []rune(evalPropStr(f.Value, e.RT))
			e.Inter.Input = &InputState{Node: f, Runes: runes, Cursor: len(runes),
				SelStart: len(runes), SelEnd: len(runes), Anchor: len(runes), BlinkStart: time.Now()}
		}
		return
	}
	e.Inter.Input = nil
}

// handleEditKey applies one key-down to the live edit session and reports
// whether the key was consumed. Modifier shortcuts (Cmd/Ctrl+A/C/X/V, word and
// line jumps) are checked BEFORE any text insertion — Cmd+A must select all,
// not type "a" — and every buffer mutation (insert, delete, paste) replaces
// the selection first. Each mutation commits once: state write-back plus
// onChange dispatch (commitEdit); pure cursor/selection keys never commit.
//
// Multi-line sessions (a registered textarea) additionally consume: return /
// enter inserting a newline, and up/down moving the cursor by visual line —
// left/right already cross newlines rune by rune, matching HTML textarea
// behavior. A single-line input sees exactly the keys it always did (return
// falls through to the engine's activation dispatch).
func (e *Engine) handleEditKey(k KeyInput) bool {
	s := e.activeEdit()
	if s == nil {
		return false
	}
	multiline := s.Node.Type == "textarea"
	// Snapshot the buffer before any mutation: every commit below records it
	// on the undo stack, so Cmd+Z walks the edits back one at a time.
	before := append([]rune(nil), s.Runes...)

	// Modifier shortcuts. Alt (Option)+printable is NOT gated here: the Rune
	// channel already carries the option-modified character, which should type.
	if k.Meta || k.Ctrl {
		switch k.Key {
		case "a":
			s.SelStart, s.SelEnd, s.Anchor, s.Cursor = 0, len(s.Runes), 0, len(s.Runes)
			return true
		case "c":
			if s.SelStart < s.SelEnd {
				ClipboardSet(string(s.Runes[s.SelStart:s.SelEnd]))
			}
			return true
		case "x":
			if s.SelStart < s.SelEnd {
				ClipboardSet(string(s.Runes[s.SelStart:s.SelEnd]))
				deleteSel(s)
				e.commit(s, before)
			}
			return true
		case "v":
			text := ClipboardGet()
			if !multiline {
				// Single-line fields strip newlines (HTML parity: pasting a
				// multi-line value into <input> folds it to spaces).
				text = strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
			}
			deleteSel(s)
			insertRunes(s, []rune(text))
			e.commit(s, before)
			return true
		case "z":
			if k.Shift {
				if redoEdit(s) {
					e.commitEdit(s)
				}
			} else if undoEdit(s) {
				e.commitEdit(s)
			}
			return true
		case "y": // Ctrl+Y (Windows) / Cmd+Shift+Z redo
			if redoEdit(s) {
				e.commitEdit(s)
			}
			return true
		case "left":
			moveCursorTo(s, wordStartAt(s.Runes, s.Cursor), k.Shift)
			return true
		case "right":
			moveCursorTo(s, wordEndAt(s.Runes, s.Cursor), k.Shift)
			return true
		case "up":
			moveCursorTo(s, 0, k.Shift) // Cmd+Up = start
			return true
		case "down":
			moveCursorTo(s, len(s.Runes), k.Shift) // Cmd+Down = end
			return true
		}
		return false // an unknown Cmd/Ctrl chord never types its letter
	}

	switch k.Key {
	case "left":
		if s.Cursor > 0 {
			moveCursorTo(s, s.Cursor-1, k.Shift)
		}
		return true
	case "right":
		if s.Cursor < len(s.Runes) {
			moveCursorTo(s, s.Cursor+1, k.Shift)
		}
		return true
	case "up", "down":
		if !multiline {
			return false
		}
		moveCursorTo(s, moveCursorLineTarget(s, k.Key == "down"), k.Shift)
		return true
	case "home":
		moveCursorTo(s, homeFor(s), k.Shift)
		return true
	case "end":
		moveCursorTo(s, endFor(s), k.Shift)
		return true
	case "return", "enter":
		if !multiline {
			return false
		}
		deleteSel(s)
		s.Runes = append(s.Runes, 0)
		copy(s.Runes[s.Cursor+1:], s.Runes[s.Cursor:])
		s.Runes[s.Cursor] = '\n'
		s.Cursor++
		collapseSel(s)
		e.commit(s, before)
		return true
	case "deleteForward":
		if deleteSel(s) {
			e.commit(s, before)
		} else if s.Cursor < len(s.Runes) {
			// The rune AFTER the caret (HTML delete key / fn+delete).
			s.Runes = append(s.Runes[:s.Cursor], s.Runes[s.Cursor+1:]...)
			e.commit(s, before)
		}
		return true
	case "delete", "backspace":
		if deleteSel(s) {
			e.commit(s, before)
		} else if s.Cursor > 0 {
			s.Runes = append(s.Runes[:s.Cursor-1], s.Runes[s.Cursor:]...)
			s.Cursor--
			e.commit(s, before)
		}
		return true
	}
	if r, ok := keyText(k); ok {
		deleteSel(s)
		insertRunes(s, []rune{r})
		e.commit(s, before)
		return true
	}
	return false
}

// moveCursorLineTarget returns the buffer index one visual line up (down=false)
// or down (down=true) from the current cursor, keeping the column where the
// target line is long enough (HTML textarea parity: up on the first line clamps
// to the buffer start, down on the last line to the end). The caller applies it
// via moveCursorTo so shift extends the selection by whole lines.
func moveCursorLineTarget(s *InputState, down bool) int {
	start, col := lineStartCol(s.Runes, s.Cursor)
	if !down {
		if start == 0 {
			return 0
		}
		// start-1 is the newline ending the PREVIOUS line; its line start is
		// the target line's start.
		target, _ := lineStartCol(s.Runes, start-1)
		if tlen := lineEnd(s.Runes, target) - target; col > tlen {
			col = tlen
		}
		return target + col
	}
	next := lineEnd(s.Runes, start) + 1 // skip the newline itself
	if next > len(s.Runes) {
		return len(s.Runes)
	}
	target := next
	if tlen := lineEnd(s.Runes, target) - target; col > tlen {
		col = tlen
	}
	return target + col
}

// collapseSel collapses the selection onto the cursor.
func collapseSel(s *InputState) {
	s.SelStart, s.SelEnd, s.Anchor = s.Cursor, s.Cursor, s.Cursor
}

// moveCursorTo moves the cursor to a clamped index. With shift the selection
// extends from the anchor (fixed on the first shift-move); without, it
// collapses. The normalized [SelStart, SelEnd] is always recomputed from
// (Anchor, Cursor), so a selection that folds back over its anchor flips
// without losing it.
func moveCursorTo(s *InputState, to int, shift bool) {
	if to < 0 {
		to = 0
	}
	if to > len(s.Runes) {
		to = len(s.Runes)
	}
	if shift {
		if s.SelStart == s.SelEnd {
			s.Anchor = s.Cursor // first shift-move: anchor at the old caret
		}
		s.Cursor = to
	} else {
		s.Cursor = to
		collapseSel(s)
		return
	}
	if s.Anchor < s.Cursor {
		s.SelStart, s.SelEnd = s.Anchor, s.Cursor
	} else {
		s.SelStart, s.SelEnd = s.Cursor, s.Anchor
	}
}

// deleteSel removes the selection (if any), collapsing the cursor to its
// start; it does NOT commit — the caller does, exactly once per mutation.
// Returns whether anything was deleted.
func deleteSel(s *InputState) bool {
	if s.SelStart >= s.SelEnd {
		return false
	}
	s.Runes = append(s.Runes[:s.SelStart], s.Runes[s.SelEnd:]...)
	s.Cursor = s.SelStart
	collapseSel(s)
	return true
}

// insertRunes inserts ins at the cursor and advances past them (no commit).
// The anchor follows the cursor so the normalized-selection invariant
// (SelStart = min(Anchor, Cursor)) holds after every insert — the caller has
// already collapsed (deleteSel) before inserting.
func insertRunes(s *InputState, ins []rune) {
	if len(ins) == 0 {
		return
	}
	s.Runes = append(s.Runes, make([]rune, len(ins))...)
	copy(s.Runes[s.Cursor+len(ins):], s.Runes[s.Cursor:])
	copy(s.Runes[s.Cursor:], ins)
	s.Cursor += len(ins)
	s.Anchor = s.Cursor
}

// isWordRune is the word-selection alphabet: letters, digits and underscore —
// the browser's word-break for plain text.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// wordStartAt returns the start of the word that starts at or before pos:
// skipping back over any separators, then over the word itself (Cmd/Ctrl+Left).
func wordStartAt(r []rune, pos int) int {
	i := pos
	for i > 0 && !isWordRune(r[i-1]) {
		i--
	}
	for i > 0 && isWordRune(r[i-1]) {
		i--
	}
	return i
}

// wordEndAt returns the end of the word that starts at or after pos: forward
// over the word, then over the following separators (Cmd/Ctrl+Right).
func wordEndAt(r []rune, pos int) int {
	i := pos
	for i < len(r) && isWordRune(r[i]) {
		i++
	}
	for i < len(r) && !isWordRune(r[i]) {
		i++
	}
	return i
}

// wordRangeAt returns the whitespace-delimited word containing pos (the
// double-click selection). A pos on whitespace advances to the next word
// right; trailing whitespace with no word collapses to pos (nothing selected).
func wordRangeAt(r []rune, pos int) (int, int) {
	if pos > len(r) {
		pos = len(r)
	}
	i := pos
	for i < len(r) && !isWordRune(r[i]) {
		i++
	}
	if i >= len(r) {
		return pos, pos
	}
	start := i
	for start > 0 && isWordRune(r[start-1]) {
		start--
	}
	end := i
	for end < len(r) && isWordRune(r[end]) {
		end++
	}
	return start, end
}

// lineRangeAt returns the whole line containing pos (the triple-click
// selection in a textarea).
func lineRangeAt(r []rune, pos int) (int, int) {
	start, _ := lineStartCol(r, pos)
	return start, lineEnd(r, start)
}

// homeFor / endFor are the Home/End targets: the line start/end for a
// multiline session, the buffer start/end for a single-line one.
func homeFor(s *InputState) int {
	if s.Node.Type != "textarea" {
		return 0
	}
	start, _ := lineStartCol(s.Runes, s.Cursor)
	return start
}

func endFor(s *InputState) int {
	if s.Node.Type != "textarea" {
		return len(s.Runes)
	}
	start, _ := lineStartCol(s.Runes, s.Cursor)
	return lineEnd(s.Runes, start)
}

// lineStartCol returns the start offset of the line containing pos and the
// column of pos within it (runes).
func lineStartCol(r []rune, pos int) (start, col int) {
	if pos > len(r) {
		pos = len(r)
	}
	for i := 0; i < pos; i++ {
		if r[i] == '\n' {
			start = i + 1
		}
	}
	return start, pos - start
}

// lineEnd returns the offset of the newline ending the line that starts at
// lineStart, or len(r) on the last line.
func lineEnd(r []rune, lineStart int) int {
	for i := lineStart; i < len(r); i++ {
		if r[i] == '\n' {
			return i
		}
	}
	return len(r)
}

// commit is the mutating-edit funnel: record the pre-edit snapshot on the
// undo stack (unless nil — undo/redo restores commit directly) and publish.
func (e *Engine) commit(s *InputState, before []rune) {
	if before != nil {
		pushUndo(s, before)
	}
	e.commitEdit(s)
}

// commitEdit publishes an edit: the two-way binding writes the buffer back to
// its state path — SetStatePath carries the runtime's read-only computed
// constraint (runtime.go:638), the same refusal the MCP set_state door
// enforces — and a declared onChange dispatches with the new {value}. The
// redraw comes from the caller: HandleKey always flags the engine dirty.
func (e *Engine) commitEdit(s *InputState) {
	if path := boundStatePath(s.Node.Value); path != "" {
		e.RT.SetStatePath(path, string(s.Runes))
	}
	if evt := s.Node.OnChange; evt != nil {
		e.dispatch(evt, map[string]any{"value": string(s.Runes)})
	}
}

// InputMetrics is the rendered geometry the caret-from-pixel mapping needs:
// the scene origin of the editable's first text run and its font metrics,
// read on demand from the graph (a fresh click maps against the frame the
// user is actually looking at).
type InputMetrics struct {
	TextX, TextY int      // scene origin of the first text run (physical px)
	FontSize     float64  // physical px
	LineH        int      // int(FontSize * 1.2), the per-line advance
	Multiline    bool
}

// inputMetricsFromGraph locates the editable's graph node and its first text
// run (value or placeholder — both are graph.Text children). It returns nil
// when there is no text to position against, which the caller treats as "keep
// the cursor at the end": an empty field or a widget with no text run keeps
// the legacy session-open behavior.
func (e *Engine) inputMetricsFromGraph(n *model.Node) *InputMetrics {
	g := e.findGroupByModelIndex(n, e.Inter.FocusedItem)
	if g == nil {
		return nil
	}
	t := firstTextNode(g)
	if t == nil {
		return nil
	}
	bb := t.GetBBox()
	fs := t.FontSize
	if fs <= 0 {
		fs = 14
	}
	// On a zoomed board the glyphs render at Zoom×fs and the pointer is in
	// screen space, so the mapping must measure at the on-screen font size.
	// (The board transform is exactly Translate(Pan)·Scale(Zoom) — see
	// measure.go boardContent.)
	if e.Inter.Board.Active && e.Inter.Board.Zoom > 0 {
		fs *= e.Inter.Board.Zoom
	}
	return &InputMetrics{
		TextX:     int(bb.MinX),
		TextY:     int(bb.MinY),
		FontSize:  fs,
		LineH:     int(fs * 1.2),
		Multiline: n.Type == "textarea",
	}
}

// firstTextNode returns the first *graph.Text in the subtree (depth-first) —
// the editable's value/placeholder run; the caret and focus ring are Rect
// shapes, so the first text is the one the user reads.
func firstTextNode(g graph.Node) *graph.Text {
	if t, ok := g.(*graph.Text); ok {
		return t
	}
	if gr, ok := g.(*graph.Group); ok {
		for _, c := range gr.Children {
			if t := firstTextNode(c); t != nil {
				return t
			}
		}
	}
	return nil
}

// caretIndexFromPointer maps a pointer position to a buffer index. Single-line
// maps the x offset through the displayed runes (the secure mask is one bullet
// per rune, so indices align); multiline picks the line from y and maps the
// column within it. The result is clamped to the BUFFER, not the display.
func caretIndexFromPointer(m *InputMetrics, s *InputState, x, y float64) int {
	if !m.Multiline {
		disp := string(s.Runes)
		if secureInput(s.Node) {
			disp = strings.Repeat("•", len(s.Runes))
		}
		return clampInt(caretColFromX([]rune(disp), int(x)-m.TextX, m.FontSize), 0, len(s.Runes))
	}
	nLines := 1
	for _, r := range s.Runes {
		if r == '\n' {
			nLines++
		}
	}
	// m.TextY is the first NON-EMPTY line's top; a buffer starting with
	// newlines (leading empty lines) shifts the first text run down, so the
	// y→line mapping anchors at the first line's true top.
	firstLine := 0
	for firstLine < len(s.Runes) && s.Runes[firstLine] == '\n' {
		firstLine++
	}
	line := (int(y) - (m.TextY - firstLine*m.LineH)) / m.LineH
	if line < 0 {
		return 0
	}
	if line >= nLines {
		return len(s.Runes)
	}
	lineStart := 0
	for i := 0; i < line; i++ {
		lineStart = lineEnd(s.Runes, lineStart) + 1
	}
	lineEnd := lineEnd(s.Runes, lineStart)
	col := caretColFromX(s.Runes[lineStart:lineEnd], int(x)-m.TextX, m.FontSize)
	return clampInt(lineStart+col, 0, len(s.Runes))
}

// caretColFromX maps an x offset (physical px from the text origin) to a
// column within a rune slice, using the same per-rune advances DrawText paints
// with — a click lands on whichever rune's right edge it crossed. x <= 0 or an
// empty line → 0; past the last rune → len.
func caretColFromX(r []rune, x int, fs float64) int {
	if x <= 0 || len(r) == 0 {
		return 0
	}
	left := 0.0
	for i := range r {
		right := MeasureText(string(r[:i+1]), fs) // left edge of rune i+1
		if float64(x) < right {
			if float64(x) < left+(right-left)/2 {
				return i
			}
			return i + 1
		}
		left = right
	}
	return len(r)
}

// placeCaretFromPointer repositions the active edit session's caret for a
// press on the editable: single click places the caret and arms drag-select,
// double click selects the word, triple click selects the line (textarea) or
// the whole field (single-line). No-ops when the press did not open a session.
func (e *Engine) placeCaretFromPointer() {
	s := e.Inter.Input
	if s == nil {
		return
	}
	m := e.inputMetricsFromGraph(s.Node)
	if m == nil {
		return // no text run to position against: keep the session-open cursor
	}
	idx := caretIndexFromPointer(m, s, e.lastPtr.X, e.lastPtr.Y)
	if e.Inter.Click == nil {
		e.Inter.Click = &ClickDetector{}
	}
	switch e.Inter.Click.Register(s.Node, geom.Point{X: e.lastPtr.X, Y: e.lastPtr.Y}, time.Now()) {
	case 2:
		a, b := wordRangeAt(s.Runes, idx)
		s.SelStart, s.SelEnd, s.Anchor, s.Cursor = a, b, a, b
		s.Selecting = false
	case 3:
		if m.Multiline {
			a, b := lineRangeAt(s.Runes, idx)
			s.SelStart, s.SelEnd, s.Anchor, s.Cursor = a, b, a, b
		} else {
			s.SelStart, s.SelEnd, s.Anchor, s.Cursor = 0, len(s.Runes), 0, len(s.Runes)
		}
		s.Selecting = false
	default:
		s.Cursor = idx
		s.SelStart, s.SelEnd, s.Anchor = idx, idx, idx
		s.Selecting = true
	}
}

// layoutInput builds the input's graph children: the value/placeholder text
// left-aligned at the padding (the generic text block centers button labels;
// an input is left-aligned like HTML), and the caret while editing. The
// background/border rect comes from the shared style path in PerformLayout
// (the theme's input component styles: inputBg/inputBorder).
func layoutInput(ln *LayoutNode, group *graph.Group, rt *runtime.Runtime, scale int) {
	fs := ln.Style.FontSize
	if fs == 0 {
		fs = 14
	}
	txtH := int(float64(fs) * 1.2)
	tx := ln.Style.Padding
	ty := (ln.Height - txtH) / 2

	c := ln.Style.Color
	if ln.Placeholder {
		// Placeholder in the secondary label color — the themed spelling of
		// the browser's own ::placeholder gray.
		c = resolveColor("var(--textSecondary)", rt)
	}
	if c.A == 0 {
		c = color.RGBA{255, 255, 255, 255}
	}
	sel := ln.Editing && ln.SelStart < ln.SelEnd
	if sel {
		// Selection highlight BEHIND the text: a NoHit rect spanning the
		// selected runes in the theme's selection color, painted before the
		// text so glyphs draw over it. The indices map onto the DISPLAYED text
		// — for a secure input that is one bullet per rune, so a password
		// selection highlights exactly the bullets it masks.
		x0 := tx + int(MeasureText(prefixRunes(ln.Text, ln.SelStart), float64(fs)))
		x1 := tx + int(MeasureText(prefixRunes(ln.Text, ln.SelEnd), float64(fs)))
		hi := graph.NewRect()
		hi.NoHit = true
		hi.X = float64(x0)
		hi.Y = float64(ty)
		hi.Width = float64(x1 - x0)
		hi.Height = float64(txtH)
		hi.Fill = resolveSelectionColor(rt)
		group.AddChild(hi)
	}

	if ln.Text != "" {
		textNode := graph.NewText()
		textNode.X = float64(tx)
		textNode.Y = float64(ty)
		textNode.Content = ln.Text
		textNode.Fill = c
		textNode.FontSize = float64(fs)
		textNode.FontWeight = ln.Style.FontWeight
		group.AddChild(textNode)
	}

	if ln.Editing && !sel && ln.CaretVisible {
		// The caret: a 1-device-px line at the insertion point, blinking at
		// caretBlinkHalf (the engine keeps animating while a session is live,
		// measure.go stamps the phase), hidden while a selection is active.
		// NoHit keeps it from ever stealing pointer hits; the x position uses
		// the same per-rune advances DrawText paints with, so caret and text
		// cannot drift apart.
		w := scale
		if w < 1 {
			w = 1
		}
		caret := graph.NewRect()
		caret.NoHit = true
		caret.X = float64(tx + int(MeasureText(prefixRunes(ln.Text, ln.Cursor), float64(fs))))
		caret.Y = float64(ty)
		caret.Width = float64(w)
		caret.Height = float64(txtH)
		caret.Fill = ln.Style.Color
		if caret.Fill.A == 0 {
			caret.Fill = resolveFocusColor(rt)
		}
		group.AddChild(caret)
	}
}

// prefixRunes returns the first n runes of s (all of s when n exceeds it).
func prefixRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
