package mcp

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// TestSetPropFieldBranches covers the value/placeholder/custom-prop targets of
// setProp (text/label/style are exercised elsewhere).
func TestSetPropFieldBranches(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ops := []PatchOp{
		{Op: "setProp", Target: "title", Key: "label", Value: "LBL"},
		{Op: "setProp", Target: "title", Key: "value", Value: "V1"},
		{Op: "setProp", Target: "title", Key: "placeholder", Value: "P1"},
		{Op: "setProp", Target: "title", Key: "customFlag", Value: true},
	}
	if err := applyPatch(app, ops); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	n := findInScenes(app, "title")
	if n.Label != "LBL" || n.Value != "V1" || n.Placeholder != "P1" {
		t.Errorf("label/value/placeholder not set: %+v", n)
	}
	if v, ok := n.Props["customFlag"]; !ok || v != true {
		t.Errorf("custom prop should land in Props, got %v", n.Props)
	}
}

// TestSetPropInitializesNilProps covers a hand-built node whose Props map was
// never allocated: a custom-key setProp must allocate it, not panic.
func TestSetPropInitializesNilProps(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Children: []*model.Node{{Type: "text", ID: "bare"}},
	}}}
	if err := applyPatch(app, []PatchOp{{Op: "setProp", Target: "bare", Key: "custom", Value: 7}}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if v := findInScenes(app, "bare").Props["custom"]; v != 7 {
		t.Errorf("Props should be initialized and carry custom=7, got %v", findInScenes(app, "bare").Props)
	}
}

// TestPatchReachesNonEntryScene asserts patches can target nodes in scenes
// other than the entry, and that when an id exists in several scenes the entry
// scene wins (deterministic lookup order).
func TestPatchReachesNonEntryScene(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main":  {Type: "column", ID: "home_root", Children: []*model.Node{{Type: "text", ID: "home_text", Text: "H"}}},
		"about": {Type: "column", ID: "about_root", Children: []*model.Node{{Type: "text", ID: "about_text", Text: "A"}}},
	}}
	if err := applyPatch(app, []PatchOp{{Op: "setProp", Target: "about_text", Key: "text", Value: "EDITED"}}); err != nil {
		t.Fatalf("patch should reach non-entry scenes: %v", err)
	}
	if got := findInScenes(app, "about_text").Text; got != "EDITED" {
		t.Errorf("about_text = %q, want EDITED", got)
	}

	// Duplicate id across scenes: the entry scene's node must be the one patched.
	app2 := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main":  {Type: "column", ID: "r1", Children: []*model.Node{{Type: "text", ID: "dup", Text: "HOME"}}},
		"other": {Type: "column", ID: "r2", Children: []*model.Node{{Type: "text", ID: "dup", Text: "OTHER"}}},
	}}
	if err := applyPatch(app2, []PatchOp{{Op: "setProp", Target: "dup", Key: "text", Value: "X"}}); err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if got := app2.Scenes["main"].Children[0].Text; got != "X" {
		t.Errorf("entry scene's duplicate id should be patched, got %q", got)
	}
	if got := app2.Scenes["other"].Children[0].Text; got != "OTHER" {
		t.Errorf("non-entry duplicate must be left alone, got %q", got)
	}
}

func TestPatchOpErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		ops  []PatchOp
		want string
	}{
		{"setProp missing target", []PatchOp{{Op: "setProp", Target: "ghost", Key: "text", Value: "x"}}, `target "ghost" not found`},
		{"addChild missing parent", []PatchOp{{Op: "addChild", Target: "ghost", Node: map[string]any{"type": "text"}}}, `parent "ghost" not found`},
		{"addChild without node", []PatchOp{{Op: "addChild", Target: "root"}}, "addChild requires a node"},
		{"remove missing target", []PatchOp{{Op: "remove", Target: "ghost"}}, `target "ghost" not found`},
		{"insertBefore missing sibling", []PatchOp{{Op: "insertBefore", Target: "ghost", Node: map[string]any{"type": "text"}}}, `sibling "ghost" not found`},
		{"insertBefore without node", []PatchOp{{Op: "insertBefore", Target: "title"}}, "insertBefore requires a node"},
		{"insertAfter without node", []PatchOp{{Op: "insertAfter", Target: "title"}}, "insertAfter requires a node"},
		{"replace missing target", []PatchOp{{Op: "replace", Target: "ghost", Node: map[string]any{"type": "text"}}}, `target "ghost" not found`},
		{"replace without node", []PatchOp{{Op: "replace", Target: "title"}}, "replace requires a node"},
		{"wrap missing target", []PatchOp{{Op: "wrap", Target: "ghost", Node: map[string]any{"type": "card"}}}, `target "ghost" not found`},
		{"wrap without node", []PatchOp{{Op: "wrap", Target: "title"}}, "wrap requires a container node"},
		{"move missing target", []PatchOp{{Op: "move", Target: "ghost", Into: "root"}}, `target "ghost" not found`},
		{"move missing destination", []PatchOp{{Op: "move", Target: "title", Into: "ghost"}}, `destination "ghost" not found`},
		{"unknown op", []PatchOp{{Op: "frobnicate", Target: "title"}}, `unknown op "frobnicate"`},
	}
	for _, tc := range cases {
		app, err := loader.LoadDir(counterDir())
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		err = applyPatch(app, tc.ops)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

// TestReplaceAndWrapSceneRoot covers the scene-root swap path: when the target
// id is a scene root (no parent), replace/wrap swap the scene's root node.
func TestReplaceAndWrapSceneRoot(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// The scene root node's id is "root" (the scene map key "main" is not a
	// node id); it has no parent, so replace swaps the scene root itself.
	repl := map[string]any{"type": "column", "id": "new_root", "text": "FRESH"}
	if err := applyPatch(app, []PatchOp{{Op: "replace", Target: "root", Node: repl}}); err != nil {
		t.Fatalf("replace scene root: %v", err)
	}
	if app.Scenes["main"].ID != "new_root" {
		t.Errorf("scene root should be replaced, got id %q", app.Scenes["main"].ID)
	}
	if findInScenes(app, "title") != nil {
		t.Error("old subtree should be gone after replacing the scene root")
	}

	app2, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := applyPatch(app2, []PatchOp{{Op: "wrap", Target: "root", Node: map[string]any{"type": "card", "id": "root_wrap"}}}); err != nil {
		t.Fatalf("wrap scene root: %v", err)
	}
	root := app2.Scenes["main"]
	if root.ID != "root_wrap" || len(root.Children) != 1 || root.Children[0].ID != "root" {
		t.Fatalf("wrap should nest the old root under the wrapper, got %+v", root)
	}
	if findInScenes(app2, "title") == nil {
		t.Error("wrapped tree must stay reachable")
	}
}

func TestMoveBetweenParents(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := applyPatch(app, []PatchOp{{Op: "move", Target: "title", Into: "display"}}); err != nil {
		t.Fatalf("move: %v", err)
	}
	display := findInScenes(app, "display")
	if len(display.Children) != 3 || display.Children[2].ID != "title" {
		t.Errorf("title should be appended to display, got %+v", display.Children)
	}
	for _, c := range findInScenes(app, "root").Children {
		if c.ID == "title" {
			t.Error("title should have left the root after move")
		}
	}
}

// TestInsertAtClampsBounds pins the defensive index clamping: out-of-range
// inserts degrade to prepend/append instead of panicking.
func TestInsertAtClampsBounds(t *testing.T) {
	s := []*model.Node{{ID: "a"}, {ID: "b"}}
	out := insertAt(s, -5, &model.Node{ID: "z"})
	if len(out) != 3 || out[0].ID != "z" {
		t.Errorf("negative index should prepend, got %v", idsOf(out))
	}
	out = insertAt(s, 99, &model.Node{ID: "y"})
	if len(out) != 3 || out[len(out)-1].ID != "y" {
		t.Errorf("overflow index should append, got %v", idsOf(out))
	}
}

func idsOf(ns []*model.Node) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

// TestDesignTokenEdgeCases covers the corners of enforcement: non-string and
// empty color values are unconstrained, and every color style key alias is
// enforced with normalization.
func TestDesignTokenEdgeCases(t *testing.T) {
	// Non-string color value: not a token concern.
	app := tokenApp()
	if err := applyPatch(app, styleOp(42)); err != nil {
		t.Errorf("non-string color must be unconstrained: %v", err)
	}
	// Empty color value: skipped.
	app = tokenApp()
	if err := applyPatch(app, styleOp("")); err != nil {
		t.Errorf("empty color must be unconstrained: %v", err)
	}
	// borderColor is a color key and is enforced.
	app = tokenApp()
	err := applyPatch(app, []PatchOp{{Op: "setProp", Target: "title", Key: "style",
		Value: map[string]any{"borderColor": "#ff0000"}}})
	if err == nil || !strings.Contains(err.Error(), "design token violation") {
		t.Errorf("borderColor violation expected, got %v", err)
	}
	// backgroundColor alias matches a token after whitespace/case/# normalization.
	app = tokenApp()
	if err := applyPatch(app, []PatchOp{{Op: "setProp", Target: "title", Key: "style",
		Value: map[string]any{"backgroundColor": "  #0A84FF "}}}); err != nil {
		t.Errorf("normalized token value should be allowed: %v", err)
	}
}

// TestChangedFieldsAllKinds asserts the diff reports every per-node field
// change kind (type/text/label/value/style/props) and children-count changes.
func TestChangedFieldsAllKinds(t *testing.T) {
	before := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {
		Type: "column", ID: "root", Children: []*model.Node{
			{Type: "text", ID: "n", Text: "old", Label: "L", Value: "V",
				Style: map[string]any{"color": "#000"}, Props: map[string]any{"p": 1}},
		},
	}}}
	after := cloneApp(before)
	n := findInScenes(after, "n")
	n.Type = "button"
	n.Text = "new"
	n.Label = "L2"
	n.Value = "V2"
	n.Style["color"] = "#fff"
	n.Style["fontSize"] = 20 // also a length difference vs the 1-key original
	n.Props["p"] = 2
	findInScenes(after, "root").Children = append(findInScenes(after, "root").Children,
		&model.Node{Type: "text", ID: "extra"})

	d := diffApps(before, after)
	if d["summary"] != "1 added, 0 removed, 2 changed" {
		t.Fatalf("summary = %v", d["summary"])
	}
	byID := map[string][]string{}
	for _, c := range d["changed"].([]map[string]any) {
		byID[c["id"].(string)] = c["fields"].([]string)
	}
	wantN := []string{"type", "text", "label", "value", "style", "props"}
	if got := strings.Join(byID["n"], ","); got != strings.Join(wantN, ",") {
		t.Errorf("node n changed fields = %v, want %v", byID["n"], wantN)
	}
	if got := strings.Join(byID["root"], ","); got != "children" {
		t.Errorf("root changed fields = %v, want [children]", byID["root"])
	}
	if added := d["added"].([]string); len(added) != 1 || added[0] != "extra" {
		t.Errorf("added = %v, want [extra]", added)
	}
}

// TestToolParamsRequiredVariants covers toolParams accepting `required` both as
// the []string obj() produces and the []any a decoded JSON schema carries.
func TestToolParamsRequiredVariants(t *testing.T) {
	schema := obj(map[string]any{"a": map[string]any{"type": "string"}}, "a")
	if got := toolParams(schema); got != "`a`* (string)" {
		t.Errorf("toolParams([]string required) = %q", got)
	}
	decoded := map[string]any{
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
		"required":   []any{"a"},
	}
	if got := toolParams(decoded); got != "`a`* (string)" {
		t.Errorf("toolParams([]any required) = %q", got)
	}
	if got := toolParams(obj(nil)); got != "—" {
		t.Errorf("toolParams(no props) = %q, want em dash", got)
	}
}
