package mcp

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func TestInsertAfterAndRemove(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	newNode := map[string]any{"type": "text", "id": "inserted", "text": "NEW"}

	if err := applyPatch(app, []PatchOp{{Op: "insertAfter", Target: "title", Node: newNode}}); err != nil {
		t.Fatalf("insertAfter: %v", err)
	}
	if findInScenes(app, "inserted") == nil {
		t.Fatal("inserted node should exist after insertAfter")
	}
	// it landed right after "title" under the same parent
	parent, idx := findParentInScenes(app, "inserted")
	if parent.Children[idx-1].ID != "title" {
		t.Errorf("inserted node should follow 'title', prev is %q", parent.Children[idx-1].ID)
	}

	if err := applyPatch(app, []PatchOp{{Op: "remove", Target: "inserted"}}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if findInScenes(app, "inserted") != nil {
		t.Error("node should be gone after remove")
	}
}

func TestWrapAndReplace(t *testing.T) {
	app, _ := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))

	// wrap the title in a new card container
	wrap := map[string]any{"type": "card", "id": "title_card"}
	if err := applyPatch(app, []PatchOp{{Op: "wrap", Target: "title", Node: wrap}}); err != nil {
		t.Fatalf("wrap: %v", err)
	}
	card := findInScenes(app, "title_card")
	if card == nil || len(card.Children) != 1 || card.Children[0].ID != "title" {
		t.Fatalf("wrap should nest title inside title_card, got %+v", card)
	}

	// replace the title node entirely
	repl := map[string]any{"type": "text", "id": "title", "text": "REPLACED"}
	if err := applyPatch(app, []PatchOp{{Op: "replace", Target: "title", Node: repl}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if n := findInScenes(app, "title"); n == nil || n.Text != "REPLACED" {
		t.Errorf("replace should swap the node, got %+v", n)
	}
}

func TestMoveRejectsOwnSubtree(t *testing.T) {
	app, _ := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	// moving root's descendant "controls" into itself must fail.
	err := applyPatch(app, []PatchOp{{Op: "move", Target: "root", Into: "controls"}})
	if err == nil {
		t.Error("moving a node into its own subtree must be rejected")
	}
}

func TestUndoRevertsApply(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s := &Server{rt: qrt.New(app)}

	ops := []PatchOp{{Op: "setProp", Target: "title", Key: "text", Value: "CHANGED"}}
	tok := patchToken(ops)
	s.previewPatch(ops) // establish the preview binding
	if _, err := s.applyPatchTool(ops, tok); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if findInScenes(s.rt.App, "title").Text != "CHANGED" {
		t.Fatal("apply should change the title text")
	}

	res, err := s.undo()
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := findInScenes(s.rt.App, "title").Text; got != "COUNTER" {
		t.Errorf("undo should restore original title, got %q", got)
	}
	if res["undoDepth"].(int) != 0 {
		t.Errorf("undo depth should be 0, got %v", res["undoDepth"])
	}
	if _, err := s.undo(); err == nil {
		t.Error("undo with empty history should error")
	}
}

func TestApplyPatchIsAtomic(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s := &Server{rt: qrt.New(app)}

	// A batch whose 2nd op targets a nonexistent node must fail wholesale —
	// the 1st op's change must NOT land on the live app.
	ops := []PatchOp{
		{Op: "setProp", Target: "title", Key: "text", Value: "SHOULD_NOT_STICK"},
		{Op: "setProp", Target: "does_not_exist", Key: "text", Value: "x"},
	}
	tok := patchToken(ops)
	s.previewPatch(ops)
	if _, err := s.applyPatchTool(ops, tok); err == nil {
		t.Fatal("batch with a bad op should fail")
	}
	if got := findInScenes(s.rt.App, "title").Text; got == "SHOULD_NOT_STICK" {
		t.Errorf("partial apply leaked: title = %q", got)
	}

	// A fully-valid batch applies all ops, and one undo reverts the whole batch.
	good := []PatchOp{
		{Op: "setProp", Target: "title", Key: "text", Value: "A"},
		{Op: "setProp", Target: "number", Key: "text", Value: "B"},
	}
	gt := patchToken(good)
	s.previewPatch(good)
	if _, err := s.applyPatchTool(good, gt); err != nil {
		t.Fatalf("valid batch: %v", err)
	}
	if findInScenes(s.rt.App, "title").Text != "A" || findInScenes(s.rt.App, "number").Text != "B" {
		t.Error("valid batch should apply all ops")
	}
	s.undo()
	if findInScenes(s.rt.App, "title").Text != "COUNTER" {
		t.Error("one undo should revert the whole batch")
	}
}

func TestDiffApps(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	after := cloneApp(app)
	ops := []PatchOp{
		{Op: "setProp", Target: "title", Key: "text", Value: "NEW"},
		{Op: "addChild", Target: "root", Node: map[string]any{"type": "text", "id": "added_node", "text": "x"}},
		{Op: "remove", Target: "status_text"},
	}
	if err := applyPatch(after, ops); err != nil {
		t.Fatalf("apply: %v", err)
	}
	d := diffApps(app, after)

	added := d["added"].([]string)
	removed := d["removed"].([]string)
	changed := d["changed"].([]map[string]any)
	if len(added) != 1 || added[0] != "added_node" {
		t.Errorf("added = %v", added)
	}
	if len(removed) != 1 || removed[0] != "status_text" {
		t.Errorf("removed = %v", removed)
	}
	// title's text changed; root's children count changed
	foundTitle := false
	for _, c := range changed {
		if c["id"] == "title" {
			foundTitle = true
			if fmt.Sprint(c["fields"]) != "[text]" {
				t.Errorf("title changed fields = %v, want [text]", c["fields"])
			}
		}
	}
	if !foundTitle {
		t.Errorf("title should be reported as changed; changed=%v", changed)
	}
}
