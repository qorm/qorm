package loader

import (
	"encoding/json"
	"github.com/qorm/qorm/internal/model"
	"path/filepath"
	"strings"
	"testing"
)

// TestNodeRoundTripReflectsEdits verifies serialize is the inverse of build and
// carries through both typed fields and extra props.
func TestNodeRoundTripReflectsEdits(t *testing.T) {
	app, err := LoadDir(filepath.Join("..", "..", "examples", "gallery"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	root := app.EntryRoot()

	// Simulate an apply_patch edit on a typed field.
	title := findByID(root, "hi")
	if title == nil {
		t.Fatal("expected node 'hi'")
	}
	title.Text = "EDITED GREETING"

	// Serialise -> JSON -> rebuild, and confirm the edit survived.
	doc := SceneToJSON(app.Entry, root)
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "EDITED GREETING") {
		t.Error("serialised scene should reflect the edited text")
	}

	var reparsed map[string]any
	_ = json.Unmarshal(raw, &reparsed)
	rebuilt := BuildNode(reparsed["root"].(map[string]any))
	if findByID(rebuilt, "hi").Text != "EDITED GREETING" {
		t.Error("round-trip lost the edit")
	}
	// Extra props (a select's options) must survive the round trip.
	sel := findByID(rebuilt, "plan_sel")
	if sel == nil {
		t.Fatal("expected 'plan_sel' after round trip")
	}
	if _, ok := sel.Prop("options"); !ok {
		t.Error("select's 'options' prop lost in round trip")
	}
}

func findByID(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if g := findByID(c, id); g != nil {
			return g
		}
	}
	return nil
}
