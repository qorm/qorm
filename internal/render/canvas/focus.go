package canvas

import (
	"sort"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// Focusables returns the focusable model nodes under root in traversal order
// (DFS over Children, matching canvas layout order): buttons, inputs, nodes
// with an OnPress handler, or nodes with focusable:true; focusable:false
// always opts out. Nodes with an explicit tabIndex > 0 sort first (ascending, stable);
// tabIndex 0 or absent keeps natural tree order. Disabled nodes are never
// focusable (web parity); rt resolves bound `disabled` keys and may be nil
// (static keys still apply).
func Focusables(root *model.Node, rt *runtime.Runtime) []*model.Node {
	if root == nil {
		return nil
	}
	type item struct {
		node *model.Node
		idx  int
		tab  int
	}
	var items []item
	idx := 0
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if isFocusable(n, rt) {
			items = append(items, item{node: n, idx: idx, tab: tabIndex(n)})
		}
		idx++
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := items[i].tab, items[j].tab
		if pi > 0 && pj > 0 && pi != pj {
			return pi < pj
		}
		if (pi > 0) != (pj > 0) {
			return pi > 0
		}
		return items[i].idx < items[j].idx
	})

	out := make([]*model.Node, len(items))
	for i, it := range items {
		out[i] = it.node
	}
	return out
}

func isFocusable(n *model.Node, rt *runtime.Runtime) bool {
	if nodeDisabled(n, rt) {
		return false // a disabled node is not focusable (web parity)
	}
	if v, ok := n.Prop("focusable"); ok {
		if b, ok := v.(bool); ok {
			return b // explicit opt-in or opt-out wins
		}
	}
	if n.Type == "button" || n.Type == "input" || n.OnPress != nil {
		return true
	}
	// Registered interactive widgets (checkbox, switch, radio, slider, select,
	// tabs, bottomnav, textarea, …) join the Tab order so the keyboard seam can
	// reach them — they focus on pointer press today, Tab is the missing half.
	if w, ok := LookupWidget(n.Type); ok {
		if _, yes := w.(InteractiveWidget); yes {
			return true
		}
	}
	return false
}

func tabIndex(n *model.Node) int {
	if v, ok := n.Prop("tabIndex"); ok {
		switch t := v.(type) {
		case float64:
			return int(t)
		case int:
			return t
		}
	}
	return 0
}

// NextFocus returns the node after (forward) or before cur in list, wrapping
// at both ends. A nil or absent cur yields the first (forward) or last node.
func NextFocus(list []*model.Node, cur *model.Node, forward bool) *model.Node {
	n := len(list)
	if n == 0 {
		return nil
	}
	i := -1
	for j, node := range list {
		if node == cur {
			i = j
			break
		}
	}
	switch {
	case i == -1 && forward:
		return list[0]
	case i == -1:
		return list[n-1]
	case forward:
		return list[(i+1)%n]
	default:
		return list[(i-1+n)%n]
	}
}
