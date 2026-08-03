package widgets

// The textarea (HTML: render_input.go:285): a multi-line text field sharing
// the single-line input's editing session (canvas/input.go) — the engine's
// edit routing was widened from "type == input" to "input or registered
// textarea" so a focused textarea opens the same InputState, and canvas
// input.go handleEditKey grows the multi-line keys (Enter inserts a newline,
// up/down move by visual line). This file owns the multi-line RENDERING the
// engine's input branch does not cover: one text run per line at fs*1.2
// (the engine's line height), the caret at its (line, column), placeholder in
// the secondary color, and a clip that cuts lines overflowing the box
// vertically (no scrolling yet — documented downgrade; the session edits the
// full buffer regardless).
//
// The session reaches the widget through a cached *canvas.Interaction: the
// Widget seam (Measure/Record) carries no interaction state, so HandlePointer
// stashes the interaction pointer the engine passes in (its address is stable
// for the engine's lifetime — Engine.Inter is a value field), and Record
// re-reads the live session from it every frame. Rendering falls back to the
// evaluated binding whenever no session is live, which per-keystroke
// write-back keeps current anyway.

import (
	"image"
	"image/color"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("textarea", &Textarea{inters: map[*model.Node]*canvas.Interaction{}})
}

// Textarea is the multi-line editor. Value binding, write-back, onChange
// dispatch, focus and disabled suppression behave exactly like the canvas
// input (the session is shared); the differences are Enter = newline, the
// line/column cursor and the taller box (rows prop, default 4 — the HTML
// rows attribute, render_input.go:288).
type Textarea struct {
	mu sync.Mutex
	// inters is the cached engine interaction per node (see the file header).
	inters map[*model.Node]*canvas.Interaction
}

const (
	textareaMinW = 160 // same usable-empty-field floor as the input (canvas/input.go)
	textareaPad  = 12  // same inner padding as the themed input chrome
)

// Measure sizes the box to the longest line (min 160px) and `rows` text
// lines (default 4), plus padding.
func (t *Textarea) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	fs := formFontSize(n, scale)
	pad := textareaPad * scale
	rows := 4
	if v, ok := n.Prop("rows"); ok {
		if r := int(formFloat(v)); r > 0 {
			rows = r
		}
	}
	lw := 0
	for _, line := range t.lines(n, rt) {
		if w := int(canvas.MeasureText(line, float64(fs))); w > lw {
			lw = w
		}
	}
	w = lw + 2*pad
	if min := textareaMinW * scale; w < min {
		w = min
	}
	h = rows*lineHeight(fs) + 2*pad
	return
}

// Record paints the input-style chrome, the text lines inside a clipped
// content group (vertical overflow is cut), and the caret while editing.
func (t *Textarea) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	g := draw.NewGroup()

	chrome := draw.NewRect()
	chrome.Width = float64(ln.Width)
	chrome.Height = float64(ln.Height)
	chrome.BorderRadius = 10 * float64(scale)
	chrome.Fill = themeColor(rt, "inputBg", color.RGBA{232, 232, 237, 255})
	chrome.Stroke = themeColor(rt, "inputBorder", color.RGBA{198, 198, 200, 255})
	chrome.StrokeWidth = float64(scale)
	g.AddChild(chrome)

	// The clip leaf emits the ClipOp the scroll viewport gets from canvas's
	// (unexported) clipNode — lines that leave the box vertically are cut
	// instead of painting over the widgets below. It mounts on the ROOT group,
	// a SIBLING BEFORE the content, so it stays fixed while the content
	// scrolls through it (a clip inside the content group would travel with
	// the offset and clip nothing).
	g.AddChild(newClipLeaf(ln.Width, ln.Height))
	content := draw.NewGroup()

	fs := formFontSizeLN(ln, scale)
	pad := float64(textareaPad * scale)
	lineH := lineHeight(fs)
	lines, placeholder := t.lines(ln.Node, rt), false
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		lines, placeholder = []string{ln.Node.Placeholder}, true
	}
	ink := formInk(ln.Node, ln, rt)
	if placeholder {
		ink = themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	}
	s := t.sessionFor(ln.Node)
	sel := s != nil && s.SelStart < s.SelEnd
	lineStart := 0

	// Internal scrolling: when the caret leaves the visible box (content taller
	// than the viewport), the content shifts so the caret stays in view — the
	// same scroll-to-caret a browser's textarea does while typing/arrowing.
	// The offset persists in Interaction.ScrollOffsets (the engine's scroll
	// state, keyed by node) so re-measures keep it; content.Y is the scroll.
	if inter := t.inters[ln.Node]; inter != nil && s != nil {
		offset := inter.ScrollOffsets[ln.Node].Y
		caretLine, _ := runeLineCol(s.Runes, s.Cursor)
		boxH := float64(ln.Height) - 2*pad
		// The content's total extent is pad + all lines (the trailing area
		// below the last line), so the last line can scroll fully visible.
		contentExtent := pad + float64(len(lines))*float64(lineH)
		// Clamp the offset unconditionally: when the content shrank to fit,
		// maxOffset is 0 and the stale offset is reset — otherwise a shrunk
		// field would keep its old scroll and render blank while holding text.
		maxOffset := contentExtent - boxH
		if maxOffset < 0 {
			maxOffset = 0
		}
		if contentExtent > boxH {
			caretTop := pad + float64(caretLine)*float64(lineH)
			if offset > caretTop {
				offset = caretTop // caret above → scroll up so it sits at the top
			}
			if caretBottom := caretTop + float64(lineH); offset < caretBottom-boxH {
				offset = caretBottom - boxH // caret below → scroll down to reveal it
			}
			if offset < 0 {
				offset = 0
			}
			if offset > maxOffset {
				offset = maxOffset
			}
		} else {
			offset = 0
		}
		if inter.ScrollOffsets == nil {
			inter.ScrollOffsets = map[*model.Node]canvas.ScrollPos{}
		}
		if inter.ScrollOffsets[ln.Node].Y != offset {
			inter.ScrollOffsets[ln.Node] = canvas.ScrollPos{Y: offset}
		}
		content.Y = -offset
	}
	for i, line := range lines {
		// Per-line selection highlight: the line's absolute buffer span
		// [lineStart, lineEnd) — counted in RUNES, matching the session's
		// SelStart/SelEnd — is intersected with the selection and the overlap
		// painted as a rect behind the line's glyphs (formText draws over it).
		lineEnd := lineStart + utf8.RuneCountInString(line)
		if sel {
			lo := max(s.SelStart, lineStart)
			hi := min(s.SelEnd, lineEnd)
			if lo < hi {
				x0 := int(canvas.MeasureText(prefixRunesOf([]rune(line), lo-lineStart), float64(fs)))
				x1 := int(canvas.MeasureText(prefixRunesOf([]rune(line), hi-lineStart), float64(fs)))
				hiRect := draw.NewRect()
				hiRect.NoHit = true
				hiRect.X = pad + float64(x0)
				hiRect.Y = pad + float64(i*lineH)
				hiRect.Width = float64(x1 - x0)
				hiRect.Height = float64(lineH)
				hiRect.Fill = canvas.SelectionColor(rt)
				content.AddChild(hiRect)
			}
		}
		if line != "" {
			content.AddChild(formText(line, pad, pad+float64(i*lineH), fs, ink))
		}
		lineStart = lineEnd + 1 // skip the line's newline
	}

	// The caret while the edit session is live (and the selection collapsed):
	// a static 1-device-px line at the insertion (line, column) — non-blinking
	// like the input's (canvas/input.go), NoHit, clipped with the content.
	if s != nil && !sel {
		lineIdx, col := runeLineCol(s.Runes, s.Cursor)
		prefix := prefixRunesOf(lineAt(s.Runes, lineIdx), col)
		cw := scale
		caret := draw.NewRect()
		caret.NoHit = true
		caret.X = pad + float64(int(canvas.MeasureText(prefix, float64(fs))))
		caret.Y = pad + float64(lineIdx*lineH)
		caret.Width = float64(cw)
		caret.Height = float64(lineH)
		caret.Fill = ink
		if placeholder {
			caret.Fill = formInk(ln.Node, ln, rt)
		}
		content.AddChild(caret)
	}

	g.AddChild(content)
	return g
}

// HandlePointer focuses the field on press (pointer-driven focus, no
// keyboard ring); the engine's post-widget syncEditSession then opens the
// edit session — the same funnel a pointer press on the built-in input uses.
func (t *Textarea) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	t.mu.Lock()
	t.inters[n] = inter
	t.mu.Unlock()
	if formDisabled(n, rt) {
		return false
	}
	if p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	return true
}

// OnFocused caches the engine interaction when KEYBOARD focus lands on this
// textarea (the FocusHookWidget seam): the Tab path never routes through
// HandlePointer, so without this a Tab-focused textarea would render without
// its caret and scroll offset. Pointer focus caches via HandlePointer.
func (t *Textarea) OnFocused(n *model.Node, inter *canvas.Interaction) {
	t.mu.Lock()
	t.inters[n] = inter
	t.mu.Unlock()
}

// sessionFor returns the live edit session for n, re-validated against the
// cached interaction (nil when no session exists or it belongs elsewhere).
func (t *Textarea) sessionFor(n *model.Node) *canvas.InputState {
	t.mu.Lock()
	inter := t.inters[n]
	t.mu.Unlock()
	if inter == nil || inter.Input == nil || inter.Input.Node != n {
		return nil
	}
	return inter.Input
}

// lines resolves the text to render: the live session buffer first, then the
// evaluated value (bindings included). Empty means "show the placeholder".
func (t *Textarea) lines(n *model.Node, rt *runtime.Runtime) []string {
	if s := t.sessionFor(n); s != nil {
		return strings.Split(string(s.Runes), "\n")
	}
	if v := formEvalStr(n.Value, rt); v != "" {
		return strings.Split(v, "\n")
	}
	return nil
}

// clipLeaf emits the clip rect cutting a textarea's overflowing lines — the
// widget-space counterpart of canvas's scroll clipNode (scroll.go), built on
// an embedded *draw.Group so no graph internals leak into widget code. Draw
// emits no Save/Translate of its own: the parent group already established
// the local coordinate space, and the clip must outlive the siblings' own
// Save/Restore pairs.
type clipLeaf struct {
	*draw.Group
}

func newClipLeaf(w, h int) *clipLeaf {
	g := draw.NewGroup()
	g.NoHit = true
	g.Width, g.Height = float64(w), float64(h)
	return &clipLeaf{Group: g}
}

// Draw emits just the clip.
func (c *clipLeaf) Draw(ctx *draw.Context) {
	ctx.ClipRect(image.Rect(0, 0, int(c.Width), int(c.Height)))
}

// runeLineCol converts an absolute cursor (runes) to its (line, column).
func runeLineCol(r []rune, cursor int) (line, col int) {
	if cursor > len(r) {
		cursor = len(r)
	}
	for i := 0; i < cursor; i++ {
		if r[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return
}

// lineAt returns the runes of the idx-th line ("" when out of range).
func lineAt(r []rune, idx int) []rune {
	start, line := 0, 0
	for i := 0; i <= len(r); i++ {
		if i == len(r) || r[i] == '\n' {
			if line == idx {
				return r[start:i]
			}
			line++
			start = i + 1
		}
	}
	return nil
}

// prefixRunesOf returns the first n runes of r.
func prefixRunesOf(r []rune, n int) string {
	if n > len(r) {
		n = len(r)
	}
	if n < 0 {
		n = 0
	}
	return string(r[:n])
}
