package canvas

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// tree builds:
//
//	column
//	├── text "a"            (not focusable)
//	├── button "b1"         (focusable by type)
//	├── column
//	│   └── button "b2"     (focusable by type)
//	├── text+onPress "t1"   (focusable via OnPress)
//	├── button "b3" focusable:false  (explicit opt-out)
//	└── text "t2" focusable:true     (explicit opt-in)
func focusTree() (*model.Node, map[string]*model.Node) {
	byID := map[string]*model.Node{}
	mk := func(id, typ string, props map[string]any, onPress bool) *model.Node {
		n := &model.Node{Type: typ, ID: id, Props: props}
		if onPress {
			n.OnPress = &model.Invoke{Name: "act"}
		}
		byID[id] = n
		return n
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		mk("a", "text", nil, false),
		mk("b1", "button", nil, false),
		{Type: "column", ID: "inner", Children: []*model.Node{
			mk("b2", "button", nil, false),
		}},
		mk("t1", "text", nil, true),
		mk("b3", "button", map[string]any{"focusable": false}, false),
		mk("t2", "text", map[string]any{"focusable": true}, false),
	}}
	return root, byID
}

func TestFocusablesOrderAndMembership(t *testing.T) {
	root, byID := focusTree()
	got := Focusables(root)

	want := []string{"b1", "b2", "t1", "t2"} // DFS order; a and b3 excluded
	if len(got) != len(want) {
		t.Fatalf("Focusables returned %d nodes, want %d (%v)", len(got), len(want), ids(got))
	}
	for i, id := range want {
		if got[i] != byID[id] {
			t.Errorf("position %d = %s, want %s (full: %v)", i, got[i].ID, id, ids(got))
		}
	}
}

func TestFocusablesTabIndexSortsFirst(t *testing.T) {
	root, byID := focusTree()
	// Give t2 (last in tree order) tabIndex 1 and b1 tabIndex 2:
	// tabIndex>0 nodes sort first ascending, then natural order.
	byID["t2"].Props["tabIndex"] = float64(1)
	byID["b1"].Props = map[string]any{"tabIndex": float64(2)}

	got := ids(Focusables(root))
	want := []string{"t2", "b1", "b2", "t1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestNextFocusWraps(t *testing.T) {
	root, byID := focusTree()
	list := Focusables(root)

	if n := NextFocus(list, nil, true); n != byID["b1"] {
		t.Errorf("forward from nil = %v, want first (b1)", n)
	}
	if n := NextFocus(list, nil, false); n != byID["t2"] {
		t.Errorf("backward from nil = %v, want last (t2)", n)
	}
	if n := NextFocus(list, byID["b1"], true); n != byID["b2"] {
		t.Errorf("forward from b1 = %v, want b2", n)
	}
	if n := NextFocus(list, byID["b2"], false); n != byID["b1"] {
		t.Errorf("backward from b2 = %v, want b1", n)
	}
	// Wrap at both ends.
	if n := NextFocus(list, byID["t2"], true); n != byID["b1"] {
		t.Errorf("forward from last = %v, want wrap to b1", n)
	}
	if n := NextFocus(list, byID["b1"], false); n != byID["t2"] {
		t.Errorf("backward from first = %v, want wrap to t2", n)
	}
	// A node absent from the list behaves like nil.
	other := &model.Node{Type: "button", ID: "x"}
	if n := NextFocus(list, other, true); n != byID["b1"] {
		t.Errorf("forward from absent = %v, want first", n)
	}
	if n := NextFocus(nil, nil, true); n != nil {
		t.Error("empty list must yield nil")
	}
}

func ids(ns []*model.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}
