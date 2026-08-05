package canvas

// App-defined JSON components (qorm.json "components"): an instance node
// ({"type":"metric", label:"…"} or {"type":"component","ref":"metric"})
// instantiates its template with the instance's props as {{prop.x}} and the
// instance's children filling the template's {type:slot} nodes — the HTML
// contract (render.go:240 renderComponent + render.go:329 slot) mirrored for
// the native engine.
//
// Instantiation happens at MEASURE time, per frame, the same way list
// templates re-instantiate: prop values evaluate in the instance's scope
// (so {{state.x}}, {{item.x}} and an outer {{prop.x}} in nested components
// all resolve live), the template deep-clones (ids suffix with the instance
// id so repeated uses stay distinct), and the clone measures in a scope
// carrying the prop map. Depth caps at 32 (render.go's compDepth).

import (
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

const maxCompDepth = 32

// componentRef resolves an instance node's component name: the node type,
// or the `ref` prop of the explicit {"type":"component","ref":…} form.
func componentRef(rt *runtime.Runtime, n *model.Node) string {
	if rt == nil || rt.App == nil || n == nil {
		return ""
	}
	name := n.Type
	if name == "component" {
		if r, ok := n.Props["ref"].(string); ok {
			name = r
		} else {
			return ""
		}
	}
	if rt.App.Components[name] == nil {
		return ""
	}
	return name
}

// instantiateComponent builds the prop scope for an instance (HTML
// renderComponent: instance props/text/label/value evaluate in the instance
// scope, a nested "props":{…} object wins on conflict, declared schema
// defaults fill the gaps) and deep-clones the template with slots filled and
// ids suffixed. outer is the instance's evaluation scope (already carrying
// state/viewport/t/route and any item/prop from an enclosing list or
// component).
func instantiateComponent(n *model.Node, comp *model.Node, name string, outer map[string]any, rt *runtime.Runtime) (*model.Node, map[string]any) {
	evalProp := func(v any) any {
		if s, ok := v.(string); ok {
			return runtime.EvalBinding(s, outer)
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
	// Declared defaults fill only what the instance omitted (HTML: a default
	// can never shadow a real value).
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

// cloneCompTree deep-clones a component template, splicing the instance's
// children into {type:slot} placeholders (named slots take children with a
// matching `slot` prop, the default slot takes the unattributed ones; an
// unfilled slot renders its own children as fallback) and suffixing template
// ids with the instance id.
func cloneCompTree(n *model.Node, instKids []*model.Node, suffix string) *model.Node {
	if n == nil {
		return nil
	}
	if n.Type == "slot" {
		// The slot node itself expands below in the parent's child loop —
		// reaching one directly (a slot AS the template root) wraps the
		// filling children in a plain group so the tree stays a single node.
		g := &model.Node{Type: "column", ID: "slot" + suffix}
		g.Children = slotFill(n, instKids)
		return g
	}
	c := *n // shallow copy first; maps/slices get their own below
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

// slotFill returns the instance children that belong to a slot: a named slot
// takes children declaring slot:"name"; the default slot takes children with
// no slot attribution; unfilled slots fall back to the slot's own children
// (render.go:329).
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
