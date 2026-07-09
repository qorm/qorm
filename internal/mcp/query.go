package mcp

import (
	"strings"

	"github.com/qorm/qorm/internal/model"
)

// selector matches nodes by structural properties. Empty fields are ignored;
// all provided fields must match (AND).
type selector struct {
	Type         string `json:"type"`
	TextContains string `json:"textContains"`
	IDContains   string `json:"idContains"`
	HasProp      string `json:"hasProp"`
}

// queryNodes walks the tree and returns compact descriptors of matching nodes,
// each with its ancestor id path so an agent knows where it sits.
func queryNodes(root *model.Node, sel selector) []map[string]any {
	var out []map[string]any
	var walk func(n *model.Node, path []string)
	walk = func(n *model.Node, path []string) {
		if n == nil {
			return
		}
		if matches(n, sel) {
			out = append(out, map[string]any{
				"id":       n.ID,
				"type":     n.Type,
				"label":    labelText(n),
				"path":     strings.Join(path, "/"),
				"children": childIDs(n),
			})
		}
		childPath := append(path, n.ID)
		for _, c := range n.Children {
			walk(c, childPath)
		}
		if n.Template != nil {
			walk(n.Template, childPath)
		}
		// both branches of a `when` node are searchable, whichever is live
		walk(n.Then, childPath)
		walk(n.Else, childPath)
	}
	walk(root, nil)
	return out
}

func matches(n *model.Node, sel selector) bool {
	if sel.Type != "" && !strings.EqualFold(n.Type, sel.Type) {
		return false
	}
	if sel.IDContains != "" && !strings.Contains(strings.ToLower(n.ID), strings.ToLower(sel.IDContains)) {
		return false
	}
	if sel.TextContains != "" {
		hay := strings.ToLower(labelText(n))
		if !strings.Contains(hay, strings.ToLower(sel.TextContains)) {
			return false
		}
	}
	if sel.HasProp != "" {
		if _, ok := n.Prop(sel.HasProp); !ok {
			return false
		}
	}
	return true
}

func labelText(n *model.Node) string {
	if n.Text != "" {
		return n.Text
	}
	return n.Label
}

func childIDs(n *model.Node) []string {
	ids := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		ids = append(ids, c.ID)
	}
	return ids
}
