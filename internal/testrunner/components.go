package testrunner

// JSON-component expansion for qorm test, mirrored from
// internal/render/canvas/components.go (and HTML renderComponent): an instance
// node instantiates its template with the instance's props as {{prop.x}} and
// the instance's children filling {type:slot} nodes. Depth caps at 32.

import (
	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

const maxCompDepth = 32

// componentRef resolves an instance node's component name: the node type,
// or the `ref` prop of the explicit {"type":"component","ref":…} form.
func componentRef(rt *qrt.Runtime, n *model.Node) string {
	if rt == nil || rt.App == nil || n == nil {
		return ""
	}
	name := n.Type
	if name == "component" {
		ref, _ := n.Props["ref"].(string)
		name = model.ComponentRefName(ref)
		if name == "" {
			return ""
		}
	}
	if rt.App.Components[name] == nil {
		return ""
	}
	return name
}

func instantiateComponent(n *model.Node, comp *model.Node, name string, outer map[string]any, rt *qrt.Runtime) (*model.Node, map[string]any) {
	evalProp := func(v any) any {
		if s, ok := v.(string); ok {
			return qrt.EvalBinding(s, outer)
		}
		return v
	}
	prop := map[string]any{}
	for k, v := range n.Props {
		if k == "props" || k == "ref" {
			continue
		}
		prop[k] = evalProp(v)
	}
	if n.Text != "" {
		prop["text"] = evalProp(n.Text)
	}
	if n.Label != "" {
		prop["label"] = evalProp(n.Label)
	}
	if n.Value != "" {
		prop["value"] = evalProp(n.Value)
	}
	if pm, ok := n.Props["props"].(map[string]any); ok {
		for k, v := range pm {
			prop[k] = evalProp(v)
		}
	}
	if rt != nil && rt.App != nil {
		if sc := rt.App.ComponentSchemas[name]; sc != nil {
			for k, spec := range sc.Props {
				if spec.Default == nil {
					continue
				}
				if _, given := prop[k]; !given {
					prop[k] = spec.Default
				}
			}
		}
	}

	suffix := ""
	if n.ID != "" {
		suffix = "_" + n.ID
	}
	clone := cloneCompTree(comp, n.Children, suffix)

	vars := make(map[string]any, len(outer)+1)
	for k, v := range outer {
		vars[k] = v
	}
	vars["prop"] = prop
	return clone, vars
}

func cloneCompTree(n *model.Node, instKids []*model.Node, suffix string) *model.Node {
	if n == nil {
		return nil
	}
	if n.Type == "slot" {
		g := &model.Node{Type: "column", ID: "slot" + suffix}
		g.Children = slotFill(n, instKids)
		return g
	}
	c := *n
	c.Props = map[string]any{}
	for k, v := range n.Props {
		c.Props[k] = v
	}
	if n.Style != nil {
		c.Style = map[string]any{}
		for k, v := range n.Style {
			c.Style[k] = v
		}
	}
	if n.Layout != nil {
		c.Layout = map[string]any{}
		for k, v := range n.Layout {
			c.Layout[k] = v
		}
	}
	if suffix != "" && c.ID != "" {
		c.ID += suffix
	}
	c.Children = nil
	for _, kid := range n.Children {
		if kid.Type == "slot" {
			c.Children = append(c.Children, slotFill(kid, instKids)...)
			continue
		}
		c.Children = append(c.Children, cloneCompTree(kid, instKids, suffix))
	}
	if n.Template != nil {
		c.Template = cloneCompTree(n.Template, instKids, suffix)
	}
	return &c
}

func slotFill(slot *model.Node, instKids []*model.Node) []*model.Node {
	name, _ := slot.Props["name"].(string)
	var out []*model.Node
	for _, c := range instKids {
		cn, _ := c.Props["slot"].(string)
		if cn == name {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return slot.Children
	}
	return out
}
