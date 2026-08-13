package canvas

import "strings"

// CursorHint tells the host which mouse cursor to show over the currently
// hovered node — the native window's counterpart of the browser's automatic
// cursor styles (I-beam over text fields, pointing hand over pressables).
type CursorHint int

const (
	CursorArrow CursorHint = iota
	CursorText
	CursorPointer
	CursorNotAllowed
)

// CursorHint computes the hint from the hovered node: text fields
// (input/textarea, including registered widgets of those types) get the text
// cursor, anything pressable (button/link, OnPress, focusable, or a
// registered InteractiveWidget) gets the pointing hand, a disabled node gets
// the not-allowed cursor, everything else the arrow.
func (e *Engine) CursorHint() CursorHint {
	m := e.Inter.Hovered
	if m == nil {
		return CursorArrow
	}
	if nodeDisabled(m, e.RT) {
		return CursorNotAllowed
	}
	// An author cursor overrides the widget-derived default. Resolve through
	// the complete QSS cascade so class/id rules behave like inline style.
	if authored, ok := resolvedAuthorStyleProp(m, "cursor", e.RT).(string); ok {
		switch strings.ToLower(strings.TrimSpace(authored)) {
		case "pointer", "hand":
			return CursorPointer
		case "text", "ibeam":
			return CursorText
		case "not-allowed", "forbidden":
			return CursorNotAllowed
		case "default", "arrow":
			return CursorArrow
		}
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
	if canPress(m, e.RT) || m.Type == "link" {
		return CursorPointer
	}
	return CursorArrow
}
