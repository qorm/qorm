package canvas

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func TestCursorHintMapping(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "input", ID: "in"},
		{Type: "button", ID: "btn", Props: map[string]any{"label": "Hi"}},
		{Type: "text", ID: "t", Props: map[string]any{"text": "plain"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})

	if got := e.CursorHint(); got != CursorArrow {
		t.Errorf("no hover: hint = %v, want Arrow", got)
	}
	e.Inter.Hovered = root.Children[0]
	if got := e.CursorHint(); got != CursorText {
		t.Errorf("hover input: hint = %v, want Text", got)
	}
	e.Inter.Hovered = root.Children[1]
	if got := e.CursorHint(); got != CursorPointer {
		t.Errorf("hover button: hint = %v, want Pointer", got)
	}
	e.Inter.Hovered = root.Children[2]
	if got := e.CursorHint(); got != CursorArrow {
		t.Errorf("hover plain text: hint = %v, want Arrow", got)
	}

	// A link (no OnPress) still gets the pointing hand.
	link := &model.Node{Type: "link", ID: "l"}
	e.Inter.Hovered = link
	if got := e.CursorHint(); got != CursorPointer {
		t.Errorf("hover link: hint = %v, want Pointer", got)
	}
	// A disabled node gets the not-allowed cursor.
	e.Inter.Hovered = &model.Node{Type: "button", ID: "db", Style: map[string]any{"disabled": true}}
	if got := e.CursorHint(); got != CursorNotAllowed {
		t.Errorf("hover disabled button: hint = %v, want NotAllowed", got)
	}
}
