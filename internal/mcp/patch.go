package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// PatchOp is a single design operation on the scene tree.
//
//	setProp  — set a node field (text/label/value/placeholder) or merge style
//	addChild — append a child node (Node is a raw QORM node object)
//	remove   — delete a node by id
type PatchOp struct {
	Op     string         `json:"op"`
	Target string         `json:"target"`
	Key    string         `json:"key,omitempty"`
	Value  any            `json:"value,omitempty"`
	Node   map[string]any `json:"node,omitempty"`
	Into   string         `json:"into,omitempty"` // move destination parent id
}

// patchToken is a stable hash of a patch, binding apply to a prior preview.
func patchToken(ops []PatchOp) string {
	data, _ := json.Marshal(ops)
	sum := sha256.Sum256(data)
	return base64.RawStdEncoding.EncodeToString(sum[:])[:16]
}

// applyPatch mutates the scenes of app according to ops.
func applyPatch(app *model.App, ops []PatchOp) error {
	for i, op := range ops {
		if err := applyOne(app, op); err != nil {
			return fmt.Errorf("op %d (%s): %w", i, op.Op, err)
		}
	}
	return nil
}

func applyOne(app *model.App, op PatchOp) error {
	switch op.Op {
	case "setProp":
		n := findInScenes(app, op.Target)
		if n == nil {
			return fmt.Errorf("target %q not found", op.Target)
		}
		setProp(n, op.Key, op.Value)
		return nil
	case "addChild":
		parent := findInScenes(app, op.Target)
		if parent == nil {
			return fmt.Errorf("parent %q not found", op.Target)
		}
		if op.Node == nil {
			return fmt.Errorf("addChild requires a node")
		}
		parent.Children = append(parent.Children, loader.BuildNode(op.Node))
		return nil
	case "remove":
		if removeByID(app, op.Target) {
			return nil
		}
		return fmt.Errorf("target %q not found", op.Target)
	case "insertBefore", "insertAfter":
		parent, idx := findParentInScenes(app, op.Target)
		if parent == nil {
			return fmt.Errorf("sibling %q not found", op.Target)
		}
		if op.Node == nil {
			return fmt.Errorf("%s requires a node", op.Op)
		}
		pos := idx
		if op.Op == "insertAfter" {
			pos = idx + 1
		}
		parent.Children = insertAt(parent.Children, pos, loader.BuildNode(op.Node))
		return nil
	case "replace":
		if op.Node == nil {
			return fmt.Errorf("replace requires a node")
		}
		parent, idx := findParentInScenes(app, op.Target)
		if parent != nil {
			parent.Children[idx] = loader.BuildNode(op.Node)
			return nil
		}
		if replaceSceneRoot(app, op.Target, loader.BuildNode(op.Node)) {
			return nil
		}
		return fmt.Errorf("target %q not found", op.Target)
	case "wrap":
		target := findInScenes(app, op.Target)
		if target == nil {
			return fmt.Errorf("target %q not found", op.Target)
		}
		if op.Node == nil {
			return fmt.Errorf("wrap requires a container node")
		}
		wrapper := loader.BuildNode(op.Node)
		wrapper.Children = append(wrapper.Children, target)
		parent, idx := findParentInScenes(app, op.Target)
		if parent != nil {
			parent.Children[idx] = wrapper
			return nil
		}
		replaceSceneRoot(app, op.Target, wrapper)
		return nil
	case "move":
		target := findInScenes(app, op.Target)
		if target == nil {
			return fmt.Errorf("target %q not found", op.Target)
		}
		dest := findInScenes(app, op.Into)
		if dest == nil {
			return fmt.Errorf("destination %q not found", op.Into)
		}
		if findNode(target, dest.ID) != nil {
			return fmt.Errorf("cannot move %q into its own subtree", op.Target)
		}
		removeByID(app, op.Target)
		dest.Children = append(dest.Children, target)
		return nil
	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}
}

// scenesInOrder returns scene roots with the entry scene first, so lookups are
// deterministic and prefer the scene the user is actually viewing.
func scenesInOrder(app *model.App) []*model.Node {
	roots := make([]*model.Node, 0, len(app.Scenes))
	if r := app.Scenes[app.Entry]; r != nil {
		roots = append(roots, r)
	}
	for id, r := range app.Scenes {
		if id != app.Entry {
			roots = append(roots, r)
		}
	}
	return roots
}

// findParentInScenes returns the parent node and child index of id, or (nil,-1).
func findParentInScenes(app *model.App, id string) (*model.Node, int) {
	for _, root := range scenesInOrder(app) {
		if p, idx := findParent(root, id); p != nil {
			return p, idx
		}
	}
	return nil, -1
}

func findParent(root *model.Node, id string) (*model.Node, int) {
	for i, c := range root.Children {
		if c.ID == id {
			return root, i
		}
		if p, idx := findParent(c, id); p != nil {
			return p, idx
		}
	}
	return nil, -1
}

func insertAt(s []*model.Node, i int, n *model.Node) []*model.Node {
	if i < 0 {
		i = 0
	}
	if i > len(s) {
		i = len(s)
	}
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = n
	return s
}

// replaceSceneRoot swaps a scene's root when its id matches, returning true.
func replaceSceneRoot(app *model.App, id string, n *model.Node) bool {
	for sid, root := range app.Scenes {
		if root.ID == id {
			app.Scenes[sid] = n
			return true
		}
	}
	return false
}

func setProp(n *model.Node, key string, value any) {
	switch key {
	case "text":
		n.Text = fmt.Sprint(value)
	case "label":
		n.Label = fmt.Sprint(value)
	case "value":
		n.Value = fmt.Sprint(value)
	case "placeholder":
		n.Placeholder = fmt.Sprint(value)
	case "style":
		if m, ok := value.(map[string]any); ok {
			if n.Style == nil {
				n.Style = map[string]any{}
			}
			for k, v := range m {
				n.Style[k] = v
			}
		}
	default:
		if n.Props == nil {
			n.Props = map[string]any{}
		}
		n.Props[key] = value
	}
}

func findInScenes(app *model.App, id string) *model.Node {
	for _, root := range scenesInOrder(app) {
		if n := findNode(root, id); n != nil {
			return n
		}
	}
	return nil
}

func removeByID(app *model.App, id string) bool {
	for _, root := range scenesInOrder(app) {
		if removeChild(root, id) {
			return true
		}
	}
	return false
}

func removeChild(parent *model.Node, id string) bool {
	for i, c := range parent.Children {
		if c.ID == id {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return true
		}
		if removeChild(c, id) {
			return true
		}
	}
	return false
}

// ---- deep clone (for side-effect-free preview) ----

func cloneApp(app *model.App) *model.App {
	// Copy every field first (Locales, DefaultLocale, BaseDir, Theme, Branding,
	// Components, Widgets, DesktopMenu, Tray, Shortcuts, ...) — apply_patch swaps
	// this clone in as the live app, so anything dropped here vanishes from every
	// later render. Only the Scenes trees are mutated by a patch, so deep-copy
	// just those; the rest are read-only here and can be shared by reference.
	c := *app
	c.Scenes = make(map[string]*model.Node, len(app.Scenes))
	for id, root := range app.Scenes {
		c.Scenes[id] = cloneNode(root)
	}
	return &c
}

func cloneNode(n *model.Node) *model.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Style = cloneMap(n.Style)
	c.Layout = cloneMap(n.Layout)
	c.Props = cloneMap(n.Props)
	c.Children = make([]*model.Node, len(n.Children))
	for i, ch := range n.Children {
		c.Children[i] = cloneNode(ch)
	}
	c.Template = cloneNode(n.Template)
	return &c
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
