package canvas

import (
	"image"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
)

func TestInspectNodeAppendsScreenOverlay(t *testing.T) {
	e, surf, btn := engineFixture(t)
	e.DrawFrame(surf)
	e.SetInspectNode(btn.ID)
	e.DrawFrame(surf)
	operations := e.ops.Operations()
	if len(operations) == 0 {
		t.Fatal("inspect frame recorded no operations")
	}
	overlay, ok := operations[len(operations)-1].(op.RRectOp)
	if !ok {
		t.Fatalf("last operation = %T, want inspector RRectOp", operations[len(operations)-1])
	}
	if overlay.StrokeWidth <= 0 || overlay.Stroke.A == 0 || overlay.Rect.Empty() {
		t.Fatalf("invalid inspector overlay: %+v", overlay)
	}
	bb := e.findGroupByModel(btn).GetBBox()
	want := image.Rect(int(bb.MinX), int(bb.MinY), int(bb.MaxX), int(bb.MaxY))
	if overlay.Rect != want {
		t.Fatalf("overlay rect = %v, want selected bbox %v", overlay.Rect, want)
	}

	e.SetInspectNode("")
	e.DrawFrame(surf)
	operations = e.ops.Operations()
	if len(operations) > 0 {
		if last, ok := operations[len(operations)-1].(op.RRectOp); ok && last.Stroke == overlay.Stroke && last.Rect == overlay.Rect {
			t.Fatal("clearing inspector id left its overlay mounted")
		}
	}
}

func TestHumanPresenceFocusTypingAndPasswordPrivacy(t *testing.T) {
	plain := &model.Node{Type: "input", ID: "email", Placeholder: "Email"}
	secret := &model.Node{Type: "input", ID: "password", Placeholder: "Password", Props: map[string]any{"password": true}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", Children: []*model.Node{plain, secret}}}})
	e := NewEngine(rt, SoftwareRenderer{})
	var got []string
	e.SetHumanPresenceSink(func(element string) { got = append(got, element) })

	e.Inter.Focused = plain
	e.Inter.Input = &InputState{Node: plain, Runes: []rune("ada@example.test")}
	e.notifyHumanPresence()
	e.notifyHumanPresence() // unchanged input must not flood presence
	e.Inter.Focused = secret
	e.Inter.Input = &InputState{Node: secret, Runes: []rune("never-export-me")}
	e.notifyHumanPresence()
	e.Inter.Focused, e.Inter.Input = nil, nil
	e.notifyHumanPresence()

	if len(got) != 3 {
		t.Fatalf("presence events = %#v, want typing, hidden, blur", got)
	}
	if got[0] != "Email = ada@example.test" {
		t.Fatalf("plain typing presence = %q", got[0])
	}
	if got[1] != "Password = (hidden)" || strings.Contains(strings.Join(got, "|"), "never-export-me") {
		t.Fatalf("secure presence leaked or malformed: %#v", got)
	}
	if got[2] != "" {
		t.Fatalf("blur presence = %q, want empty", got[2])
	}
}
