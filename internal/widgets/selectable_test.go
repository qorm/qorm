package widgets

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
)

func TestSelectableTextSessionAndCopy(t *testing.T) {
	var clip string
	canvas.SetClipboard(canvas.ClipboardFunc{
		GetFn: func() string { return clip },
		SetFn: func(s string) { clip = s },
	})
	t.Cleanup(func() { canvas.SetClipboard(nil) })

	n := &model.Node{Type: "selectabletext", ID: "st", Text: "hello world"}
	e, surf := formEngine(t, n)
	e.DrawFrame(surf)

	// Focus via pointer on the text (column origin top-left).
	clickAt(e, 5, 5)
	e.DrawFrame(surf)

	s := e.Inter.Input
	if s == nil || s.Node != n {
		t.Fatalf("selectabletext must open a read-only edit session, got %#v", s)
	}
	if string(s.Runes) != "hello world" {
		t.Fatalf("buffer = %q", string(s.Runes))
	}

	// Cmd+A then Cmd+C copies the whole string.
	e.HandleKey(canvas.KeyInput{Key: "a", Meta: true, Down: true})
	e.HandleKey(canvas.KeyInput{Key: "c", Meta: true, Down: true})
	if clip != "hello world" {
		t.Fatalf("clipboard = %q, want full text", clip)
	}

	// Typing must not mutate.
	e.HandleKey(canvas.KeyInput{Key: "x", Down: true, Rune: 'x'})
	if string(s.Runes) != "hello world" {
		t.Fatalf("selectabletext must reject typing, buffer=%q", string(s.Runes))
	}
}
