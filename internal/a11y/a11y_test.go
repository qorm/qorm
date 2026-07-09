package a11y

import "testing"

import "github.com/qorm/qorm/internal/model"

// TestBuildTreeAndAudit covers role/name/state derivation and the nameless-control
// / missing-alt audit over a small mixed scene.
func TestBuildTreeAndAudit(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "text", ID: "title", Text: "Settings"},
		{Type: "button", ID: "save", Label: "Save"},                             // named button — ok
		{Type: "button", ID: "icononly", Props: map[string]any{"icon": "gear"}}, // nameless button — issue
		{Type: "checkbox", ID: "wifi", Props: map[string]any{"checked": true, "ariaLabel": "Wi-Fi"}},
		{Type: "input", ID: "email", Placeholder: "you@example.com"},           // placeholder = name
		{Type: "image", ID: "hero"},                                            // no alt — issue
		{Type: "image", ID: "logo", Props: map[string]any{"alt": "QORM logo"}}, // alt ok
	}}
	tr := Build(root)

	byID := map[string]*Node{}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		byID[n.ID] = n
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tr.Root)

	// roles
	if byID["save"].Role != "button" || byID["email"].Role != "textbox" || byID["wifi"].Role != "checkbox" {
		t.Errorf("role derivation wrong: save=%q email=%q wifi=%q", byID["save"].Role, byID["email"].Role, byID["wifi"].Role)
	}
	// accessible names
	if byID["save"].Name != "Save" || byID["email"].Name != "you@example.com" || byID["wifi"].Name != "Wi-Fi" {
		t.Errorf("name derivation wrong: save=%q email=%q wifi=%q", byID["save"].Name, byID["email"].Name, byID["wifi"].Name)
	}
	// state
	if byID["wifi"].State["checked"] != true {
		t.Errorf("checkbox checked state not derived: %+v", byID["wifi"].State)
	}
	// audit: exactly the nameless button + the alt-less image
	if byID["icononly"].Issues == nil {
		t.Error("nameless icon button should be flagged")
	}
	if byID["hero"].Issues == nil {
		t.Error("image without alt should be flagged")
	}
	if byID["logo"].Issues != nil || byID["save"].Issues != nil {
		t.Error("named image/button must not be flagged")
	}
	if tr.Counts.Issues != 2 {
		t.Errorf("expected 2 issues (nameless button + alt-less image), got %d: %+v", tr.Counts.Issues, tr.Issues)
	}
	if tr.Counts.Interactive < 3 {
		t.Errorf("expected >=3 interactive controls, got %d", tr.Counts.Interactive)
	}
}

// TestListTemplateAndWhenBranches ensures a list's item template and a `when`
// node's branches are walked (so their controls are audited too).
func TestListTemplateAndWhenBranches(t *testing.T) {
	root := &model.Node{Type: "column", ID: "c", Children: []*model.Node{
		{Type: "list", ID: "l", Data: "{{state.items}}", Template: &model.Node{
			Type: "button", ID: "row", Props: map[string]any{"icon": "x"}, // nameless in template
		}},
		{Type: "when", ID: "w", Condition: "{{viewport.width > 600}}",
			Then: &model.Node{Type: "image", ID: "wide"}, // no alt
			Else: &model.Node{Type: "text", ID: "narrow", Text: "hi"}},
	}}
	tr := Build(root)
	if tr.Counts.Issues != 2 { // nameless template button + alt-less then-branch image
		t.Errorf("template/when branches should be audited: got %d issues: %+v", tr.Counts.Issues, tr.Issues)
	}
}
