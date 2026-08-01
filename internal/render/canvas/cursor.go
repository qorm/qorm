package canvas

// CursorHint tells the host which mouse cursor to show over the currently
// hovered node — the native window's counterpart of the browser's automatic
// cursor styles (I-beam over text fields, pointing hand over pressables).
type CursorHint int

const (
	CursorArrow CursorHint = iota
	CursorText
	CursorPointer
)

// CursorHint computes the hint from the hovered node: text fields
// (input/textarea, including registered widgets of those types) get the text
// cursor, anything pressable (button/link, OnPress, focusable, or a
// registered InteractiveWidget) gets the pointing hand, everything else the
// arrow.
func (e *Engine) CursorHint() CursorHint {
	m := e.Inter.Hovered
	if m == nil {
		return CursorArrow
	}
	switch m.Type {
	case "input", "textarea", "searchfield":
		return CursorText
	}
	if w, ok := LookupWidget(m.Type); ok {
		if _, interactive := w.(InteractiveWidget); interactive {
			return CursorPointer
		}
	}
	if canPress(m, e.RT) {
		return CursorPointer
	}
	return CursorArrow
}
