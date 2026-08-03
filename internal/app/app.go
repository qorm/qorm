package app

import (
	"image"
)

// Event is the interface for all window events.
type Event interface {
	isEvent()
}

// Pt is a convenience re-export of image.Pt for hosts building events.
func Pt(x, y int) image.Point { return image.Pt(x, y) }

// PointerEvent represents a mouse or touch interaction.
type PointerEvent struct {
	Type     PointerType
	Position image.Point
	Buttons  int
}

func (PointerEvent) isEvent() {}

type PointerType uint8

const (
	PointerPress PointerType = iota
	PointerRelease
	PointerMove
)

// KeyEvent represents a keyboard interaction.
type KeyEvent struct {
	Type  KeyEventType
	Code  int    // Raw platform keycode (macOS virtual keycode).
	Key   string // Normalized name: "tab", "return", "escape", "space", "up", "down", "left", "right", "delete", or "a".."z" / "0".."9".
	Shift bool   // True when the shift modifier was held.
	// Rune is the printable character the key produces (0 = none), from the
	// platform text channel (macOS -characters — modifier-aware, so shift is
	// already applied). Filled on KeyDown only; control keys leave it 0.
	Rune rune
}

func (KeyEvent) isEvent() {}

type KeyEventType uint8

const (
	KeyDown KeyEventType = iota
	KeyUp
)

// ScrollEvent represents a mouse wheel or trackpad scroll interaction. Ctrl is
// true when the scroll carried the control modifier — macOS trackpad pinch is
// delivered exactly this way (a precise scroll with the control flag set) — so
// hosts can tell an infinite-canvas board's zoom gesture from a plain scroll.
type ScrollEvent struct {
	DeltaX float64
	DeltaY float64
	Ctrl   bool
}

func (ScrollEvent) isEvent() {}

// Window represents an OS window.
type Window struct {
	w             *windowImpl
	width, height int // Content size in points as passed to NewWindow.
}

// Size returns the window's content size in points.
func (w *Window) Size() image.Point {
	return image.Pt(w.width, w.height)
}

// Scale returns the device-pixel ratio (w.w.scale, set from the platform's
// backing scale factor; 0/1 == 1).
func (w *Window) Scale() int {
	if w.w.scale < 1 {
		return 1
	}
	return w.w.scale
}
