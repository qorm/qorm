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

func TestLoaderDiagnostics(t *testing.T) {
	docs := []map[string]any{
		{
			"type": "scene",
			"id":   "main",
			"root": map[string]any{
				"type":  "view",
				"id":    "root_id",
				"value": "ignored_value",
				"on": map[string]any{
					"press": "some_action",
				},
				"children": []any{
					map[string]any{
						"type": "text",
						"id":   "text_id",
						"text": "Hello {{count}}",
					},
					map[string]any{
						"type":    "button",
						"id":      "btn_id",
						"onPress": "scene://other_scene",
					},
				},
			},
		},
		{
			"type": "action",
			"id":   "some_action",
			"steps": []any{
				map[string]any{
					"type": "navigate",
					"to":   "scene://target_scene",
				},
			},
		},
	}

	app := FromDocs(docs)
	if len(app.Diagnostics) == 0 {
		t.Fatal("expected compiler diagnostics, but got none")
	}

	hasOnWarning := false
	hasValueWarning := false
	hasExprWarning := false
	hasSceneWarning := false
	hasActionSceneWarning := false

	for _, d := range app.Diagnostics {
		if strings.Contains(d, "使用了已弃用的 'on' 属性") {
			hasOnWarning = true
		}
		if strings.Contains(d, "错误地配置了 'value'") {
			hasValueWarning = true
		}
		if strings.Contains(d, "使用了非标准的绑定") {
			hasExprWarning = true
		}
		if strings.Contains(d, "btn_id") && strings.Contains(d, "scene://") {
			hasSceneWarning = true
		}
		if strings.Contains(d, "some_action") && strings.Contains(d, "scene://") {
			hasActionSceneWarning = true
		}
	}

	if !hasOnWarning {
		t.Error("missing deprecated 'on' binding warning")
	}
	if !hasValueWarning {
		t.Error("missing text component 'value' warning")
	}
	if !hasExprWarning {
		t.Error("missing non-standard binding warning")
	}
	if !hasSceneWarning {
		t.Error("missing scene:// protocol warning in invoke")
	}
	if !hasActionSceneWarning {
		t.Error("missing scene:// protocol warning in action step")
	}
}
