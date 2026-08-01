package canvas

import (
	"image/color"
	"regexp"
	"strings"
	"unicode"

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

// InputState is one live edit session: the text buffer and the insertion
// cursor, both in runes. It is interaction state (Interaction.Input) for the
// same reason Pressed/Focused are — the scene graph is rebuilt every frame,
// so the session survives here, keyed by the stable model pointer.
type InputState struct {
	Node   *model.Node
	Runes  []rune
	Cursor int // insertion point in runes, [0, len(Runes)]
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
	if e.Inter.Focused != s.Node || nodeDisabled(s.Node, e.RT) || !nodeMounted(e.sceneRoot(), s.Node, e.RT) {
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
func (e *Engine) syncEditSession() {
	f := e.Inter.Focused
	if f != nil && editableType(f.Type) && !nodeDisabled(f, e.RT) {
		if s := e.Inter.Input; s == nil || s.Node != f {
			// The buffer starts from the evaluated value with the cursor at
			// the end, like clicking into an HTML field.
			runes := []rune(evalPropStr(f.Value, e.RT))
			e.Inter.Input = &InputState{Node: f, Runes: runes, Cursor: len(runes)}
		}
		return
	}
	e.Inter.Input = nil
}

// handleEditKey applies one key-down to the live edit session and reports
// whether the key was consumed. Printable text inserts at the cursor,
// left/right move it, delete (the macOS backspace name; "backspace" covers
// other hosts) removes the rune before it. Every buffer mutation commits:
// state write-back plus onChange dispatch (commitEdit).
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
	switch k.Key {
	case "left":
		if s.Cursor > 0 {
			s.Cursor--
		}
		return true
	case "right":
		if s.Cursor < len(s.Runes) {
			s.Cursor++
		}
		return true
	case "up", "down":
		if !multiline {
			return false
		}
		moveCursorLine(s, k.Key == "down")
		return true
	case "return", "enter":
		if !multiline {
			return false
		}
		s.Runes = append(s.Runes, 0)
		copy(s.Runes[s.Cursor+1:], s.Runes[s.Cursor:])
		s.Runes[s.Cursor] = '\n'
		s.Cursor++
		e.commitEdit(s)
		return true
	case "delete", "backspace":
		if s.Cursor > 0 {
			s.Runes = append(s.Runes[:s.Cursor-1], s.Runes[s.Cursor:]...)
			s.Cursor--
			e.commitEdit(s)
		}
		return true
	}
	if r, ok := keyText(k); ok {
		s.Runes = append(s.Runes, 0)
		copy(s.Runes[s.Cursor+1:], s.Runes[s.Cursor:])
		s.Runes[s.Cursor] = r
		s.Cursor++
		e.commitEdit(s)
		return true
	}
	return false
}

// moveCursorLine moves the insertion cursor one visual line up (down=false)
// or down (down=true), keeping the column where the target line is long
// enough (HTML textarea parity: up on the first line clamps to the buffer
// start, down on the last line to the end).
func moveCursorLine(s *InputState, down bool) {
	start, col := lineStartCol(s.Runes, s.Cursor)
	target := start
	if !down {
		if start == 0 {
			s.Cursor = 0
			return
		}
		// start-1 is the newline ending the PREVIOUS line; its line start is
		// the target line's start.
		target, _ = lineStartCol(s.Runes, start-1)
	} else {
		next := lineEnd(s.Runes, start) + 1 // skip the newline itself
		if next > len(s.Runes) {
			s.Cursor = len(s.Runes)
			return
		}
		target = next
	}
	if tlen := lineEnd(s.Runes, target) - target; col > tlen {
		col = tlen
	}
	s.Cursor = target + col
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

	if ln.Editing {
		// The caret: a static 1-device-px line at the insertion point (see the
		// file header for why it does not blink). NoHit keeps it from ever
		// stealing pointer hits; the x position uses the same per-rune
		// advances DrawText paints with, so caret and text cannot drift apart.
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
