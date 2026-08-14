package canvas

import (
	"strings"

	"github.com/qorm/platform/internal/model"
)

// A11yRole defines the semantic role of an accessibility node.
type A11yRole string

const (
	RoleButton A11yRole = "button"
	RoleImage  A11yRole = "image"
	RoleText   A11yRole = "text"
	RoleGroup  A11yRole = "group"
	RoleInput  A11yRole = "input"
	RoleToggle A11yRole = "toggle"
)

// A11yState holds state information for an accessibility node.
type A11yState struct {
	Disabled bool
	Checked  bool
}

// A11yNode is an abstract OS-independent accessibility node representing a widget in the canvas.
type A11yNode struct {
	ID       string
	Role     A11yRole
	Label    string
	Value    string
	State    A11yState
	Children []*A11yNode
}

// BuildA11yTree maps a model.Node widget hierarchy into an abstract OS-independent A11y structure.
// It uses aria-label, labels, text, and clickable states to derive roles and names.
func BuildA11yTree(n *model.Node) *A11yNode {
	if n == nil {
		return nil
	}

	role := RoleGroup
	switch n.Type {
	case "button", "fab", "iconbutton":
		role = RoleButton
	case "image", "photo", "icon":
		role = RoleImage
	case "text", "richtext":
		role = RoleText
	case "input", "textarea":
		role = RoleInput
	case "checkbox", "switch", "radio":
		role = RoleToggle
	}

	// Derive the accessible name (label)
	label := n.Label
	if label == "" {
		label = n.Text
	}
	if v, ok := n.Prop("ariaLabel"); ok {
		if s, ok := v.(string); ok && s != "" {
			label = s
		}
	}
	if label == "" && n.Placeholder != "" {
		label = n.Placeholder
	}
	if label == "" {
		if v, ok := n.Prop("alt"); ok {
			if s, ok := v.(string); ok && s != "" {
				label = s
			}
		}
	}

	label = strings.TrimSpace(label)

	// Derive state
	disabled := false
	if v, ok := n.Prop("disabled"); ok {
		if b, ok := v.(bool); ok {
			disabled = b
		} else if s, ok := v.(string); ok {
			disabled = s == "true"
		}
	}

	checked := false
	if v, ok := n.Prop("checked"); ok {
		if b, ok := v.(bool); ok {
			checked = b
		} else if s, ok := v.(string); ok {
			checked = s == "true"
		}
	} else if n.Value != "" && (role == RoleToggle) {
		checked = n.Value == "true"
	}

	// Upgrade role if clickable
	if role == RoleGroup && n.OnPress != nil {
		role = RoleButton
	}

	node := &A11yNode{
		ID:    n.ID,
		Role:  role,
		Label: label,
		Value: n.Value,
		State: A11yState{
			Disabled: disabled,
			Checked:  checked,
		},
	}

	// Recurse children
	for _, child := range n.Children {
		if cNode := BuildA11yTree(child); cNode != nil {
			node.Children = append(node.Children, cNode)
		}
	}
	if n.Template != nil {
		if cNode := BuildA11yTree(n.Template); cNode != nil {
			node.Children = append(node.Children, cNode)
		}
	}
	if n.Then != nil {
		if cNode := BuildA11yTree(n.Then); cNode != nil {
			node.Children = append(node.Children, cNode)
		}
	}
	if n.Else != nil {
		if cNode := BuildA11yTree(n.Else); cNode != nil {
			node.Children = append(node.Children, cNode)
		}
	}

	return node
}
