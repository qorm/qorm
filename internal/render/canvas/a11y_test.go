package canvas

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func TestBuildA11yTree(t *testing.T) {
	tests := []struct {
		name     string
		node     *model.Node
		expected *A11yNode
	}{
		{
			name: "Button with label",
			node: &model.Node{
				Type:  "button",
				Label: "Submit",
			},
			expected: &A11yNode{
				Role:  RoleButton,
				Label: "Submit",
			},
		},
		{
			name: "Group becomes button when clickable",
			node: &model.Node{
				Type: "container",
				Props: map[string]any{
					"ariaLabel": "Clickable Container",
				},
				OnPress: &model.Invoke{},
			},
			expected: &A11yNode{
				Role:  RoleButton,
				Label: "Clickable Container",
			},
		},
		{
			name: "Disabled toggle",
			node: &model.Node{
				Type: "checkbox",
				Props: map[string]any{
					"disabled": true,
					"checked":  true,
				},
			},
			expected: &A11yNode{
				Role: RoleToggle,
				State: A11yState{
					Disabled: true,
					Checked:  true,
				},
			},
		},
		{
			name: "Nested structure",
			node: &model.Node{
				Type: "container",
				Children: []*model.Node{
					{
						Type: "text",
						Text: "Hello",
					},
					{
						Type:  "image",
						Props: map[string]any{"alt": "Logo"},
					},
				},
			},
			expected: &A11yNode{
				Role: RoleGroup,
				Children: []*A11yNode{
					{Role: RoleText, Label: "Hello"},
					{Role: RoleImage, Label: "Logo"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildA11yTree(tt.node)
			if got == nil && tt.expected == nil {
				return
			}
			if got == nil || tt.expected == nil {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}

			if got.Role != tt.expected.Role {
				t.Errorf("Role: expected %v, got %v", tt.expected.Role, got.Role)
			}
			if got.Label != tt.expected.Label {
				t.Errorf("Label: expected %q, got %q", tt.expected.Label, got.Label)
			}
			if got.State.Disabled != tt.expected.State.Disabled {
				t.Errorf("Disabled: expected %v, got %v", tt.expected.State.Disabled, got.State.Disabled)
			}
			if got.State.Checked != tt.expected.State.Checked {
				t.Errorf("Checked: expected %v, got %v", tt.expected.State.Checked, got.State.Checked)
			}

			if len(got.Children) != len(tt.expected.Children) {
				t.Errorf("Children count: expected %d, got %d", len(tt.expected.Children), len(got.Children))
			} else {
				for i, child := range got.Children {
					expChild := tt.expected.Children[i]
					if child.Role != expChild.Role {
						t.Errorf("Child %d Role: expected %v, got %v", i, expChild.Role, child.Role)
					}
					if child.Label != expChild.Label {
						t.Errorf("Child %d Label: expected %q, got %q", i, expChild.Label, child.Label)
					}
				}
			}
		})
	}
}
