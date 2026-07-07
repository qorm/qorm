package mcp

import (
	"fmt"
	"sort"

	"github.com/qorm/qorm/internal/model"
)

// diffApps returns a structural diff of the entry scene between two apps:
// which node ids were added, removed, and (for common ids) which fields changed.
func diffApps(before, after *model.App) map[string]any {
	b := nodesByID(before.EntryRoot())
	a := nodesByID(after.EntryRoot())

	var added, removed []string
	var changed []map[string]any
	for id := range a {
		if _, ok := b[id]; !ok {
			added = append(added, id)
		}
	}
	for id, bn := range b {
		an, ok := a[id]
		if !ok {
			removed = append(removed, id)
			continue
		}
		if fields := changedFields(bn, an); len(fields) > 0 {
			changed = append(changed, map[string]any{"id": id, "fields": fields})
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Slice(changed, func(i, j int) bool { return changed[i]["id"].(string) < changed[j]["id"].(string) })

	return map[string]any{
		"added":   added,
		"removed": removed,
		"changed": changed,
		"summary": fmt.Sprintf("%d added, %d removed, %d changed", len(added), len(removed), len(changed)),
	}
}

func nodesByID(root *model.Node) map[string]*model.Node {
	out := map[string]*model.Node{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if n.ID != "" {
			out[n.ID] = n
		}
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.Template)
	}
	walk(root)
	return out
}

// changedFields lists the node fields that differ between two versions.
func changedFields(x, y *model.Node) []string {
	var f []string
	if x.Type != y.Type {
		f = append(f, "type")
	}
	if x.Text != y.Text {
		f = append(f, "text")
	}
	if x.Label != y.Label {
		f = append(f, "label")
	}
	if x.Value != y.Value {
		f = append(f, "value")
	}
	if !sameMap(x.Style, y.Style) {
		f = append(f, "style")
	}
	if !sameMap(x.Props, y.Props) {
		f = append(f, "props")
	}
	if len(x.Children) != len(y.Children) {
		f = append(f, "children")
	}
	return f
}

func sameMap(x, y map[string]any) bool {
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if fmt.Sprint(y[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}
