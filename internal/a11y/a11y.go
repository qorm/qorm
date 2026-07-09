// Package a11y derives an accessibility tree from a QORM scene. Because QORM is
// an object-ized UI model, every widget's semantic role, accessible name and
// state are knowable from the declarative tree — no DOM, no screen-reader needed.
// The tree doubles as an audit: interactive nodes and images that would reach a
// screen reader with no accessible name are flagged, so an agent (or CI) can
// catch accessibility gaps the way it catches type errors.
package a11y

import (
	"fmt"
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/model"
)

// Node is one entry in the accessibility tree: a widget's derived ARIA role, its
// accessible name, any semantic state, the issues it raises, and its children.
type Node struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Role     string         `json:"role"`
	Name     string         `json:"name,omitempty"`
	State    map[string]any `json:"state,omitempty"`
	Issues   []string       `json:"issues,omitempty"`
	Children []*Node        `json:"children,omitempty"`
}

// Tree builds the accessibility tree for a scene root. Issues found anywhere in
// the tree are also collected flat (with the node id) so a caller gets an audit
// summary without walking the tree.
type Tree struct {
	Root   *Node    `json:"root"`
	Issues []Issue  `json:"issues"`
	Counts struct { // quick audit summary
		Nodes       int `json:"nodes"`
		Interactive int `json:"interactive"`
		Issues      int `json:"issues"`
	} `json:"counts"`
}

// Issue is one accessibility problem, located by node id + type.
type Issue struct {
	NodeID string `json:"nodeId,omitempty"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// roleFor maps a QORM node type to its ARIA role. "" means the node is a generic
// container/presentational grouping (rendered as role "group" only when it has an
// id or name worth exposing; otherwise its children are inlined by the caller).
func roleFor(t string) string {
	switch t {
	case "button", "fab", "floatingactionbutton", "iconbutton", "backbutton", "closebutton":
		return "button"
	case "link":
		return "link"
	case "input", "textarea", "field", "formfield", "textformfield", "autocomplete":
		return "textbox"
	case "checkbox":
		return "checkbox"
	case "switch":
		return "switch"
	case "radio":
		return "radio"
	case "slider", "rangeslider":
		return "slider"
	case "select", "dropdown", "dropdownbutton", "picker":
		return "combobox"
	case "image", "photo":
		return "img"
	case "icon", "avatar":
		return "img"
	case "text", "selectabletext", "richtext":
		return "text"
	case "largetitle", "appbar":
		return "heading"
	case "list", "gridview", "listsection":
		return "list"
	case "listtile", "listitem":
		return "listitem"
	case "bottomnav", "bottomnavigationbar", "navigationbar", "navigationrail", "navigationdrawer", "drawer":
		return "navigation"
	case "tabs":
		return "tablist"
	case "alert", "alertdialog", "dialog", "modal", "actionsheet", "snackbar", "banner":
		return "dialog"
	case "progress", "circularprogress", "spinner", "activityindicator":
		return "progressbar"
	case "form":
		return "form"
	case "divider", "verticaldivider", "spacer":
		return "separator"
	default:
		return "" // generic container
	}
}

// interactive reports whether a role reaches the user as an actionable control,
// so a missing accessible name is a real barrier (not just cosmetic).
func interactive(role string) bool {
	switch role {
	case "button", "link", "textbox", "checkbox", "switch", "radio", "slider", "combobox", "tab":
		return true
	}
	return false
}

// name derives a node's accessible name from the declarative props, in the order
// a screen reader would resolve it: an explicit ariaLabel wins, then the visible
// label/text, then a placeholder, then image alt, then a tooltip/title.
func name(n *model.Node) string {
	if v, ok := n.Prop("ariaLabel"); ok {
		if s := strings.TrimSpace(str(v)); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(n.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(n.Text); s != "" {
		return s
	}
	if s := strings.TrimSpace(n.Placeholder); s != "" {
		return s
	}
	for _, k := range []string{"alt", "tooltip", "title", "label"} {
		if v, ok := n.Prop(k); ok {
			if s := strings.TrimSpace(str(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// stateFor collects the semantic state a screen reader announces: checked for
// toggles, disabled, required, and the current value for text controls.
func stateFor(n *model.Node, role string) map[string]any {
	st := map[string]any{}
	switch role {
	case "checkbox", "switch", "radio":
		if v, ok := n.Prop("checked"); ok {
			st["checked"] = truthy(v)
		} else if n.Value != "" {
			st["checked"] = truthy(n.Value)
		}
	}
	if v, ok := n.Prop("disabled"); ok && truthy(v) {
		st["disabled"] = true
	}
	if v, ok := n.Prop("required"); ok && truthy(v) {
		st["required"] = true
	}
	if role == "textbox" && n.Value != "" {
		st["value"] = n.Value
	}
	if len(st) == 0 {
		return nil
	}
	return st
}

// Build produces the accessibility tree for a scene root.
func Build(root *model.Node) *Tree {
	t := &Tree{}
	if root != nil {
		t.Root = build(root, t)
	}
	t.tidy()
	return t
}

func build(n *model.Node, t *Tree) *Node {
	if n == nil {
		return nil
	}
	role := roleFor(n.Type)
	an := &Node{ID: n.ID, Type: n.Type, Role: role, Name: name(n), State: stateFor(n, role)}
	t.Counts.Nodes++
	if interactive(role) {
		t.Counts.Interactive++
	}

	// audit: a control or image that would reach a screen reader nameless.
	if an.Name == "" {
		switch {
		case interactive(role):
			an.Issues = append(an.Issues, "no accessible name")
			t.Issues = append(t.Issues, Issue{NodeID: n.ID, Type: "missing-name", Detail: n.Type + " control has no accessible name (add label/ariaLabel)"})
		case role == "img" && n.Type != "icon" && n.Type != "avatar":
			an.Issues = append(an.Issues, "no alt text")
			t.Issues = append(t.Issues, Issue{NodeID: n.ID, Type: "missing-alt", Detail: n.Type + " has no alt text (add alt/ariaLabel)"})
		}
	}

	// recurse: children, a list's item template, and a `when` node's branches.
	for _, c := range n.Children {
		if ch := build(c, t); ch != nil {
			an.Children = append(an.Children, ch)
		}
	}
	if n.Template != nil {
		if ch := build(n.Template, t); ch != nil {
			ch.Role = orRole(ch.Role, "listitem")
			an.Children = append(an.Children, ch)
		}
	}
	for _, br := range []*model.Node{n.Then, n.Else} {
		if ch := build(br, t); ch != nil {
			an.Children = append(an.Children, ch)
		}
	}
	return an
}

func orRole(cur, def string) string {
	if cur == "" {
		return def
	}
	return cur
}

// finalize sorts issues for stable output and sets the count. Called by Build via
// a deferred tidy so callers get deterministic JSON.
func (t *Tree) tidy() {
	sort.SliceStable(t.Issues, func(i, j int) bool {
		if t.Issues[i].Type != t.Issues[j].Type {
			return t.Issues[i].Type < t.Issues[j].Type
		}
		return t.Issues[i].NodeID < t.Issues[j].NodeID
	})
	t.Counts.Issues = len(t.Issues)
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(fmt.Sprint(v), "\n", " "))
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.TrimSpace(x)
		return s != "" && s != "false" && s != "0"
	default:
		return v != nil
	}
}
