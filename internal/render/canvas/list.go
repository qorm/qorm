package canvas

import (
	"fmt"
	"os"
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// This file is the canvas counterpart of the HTML path's `list`
// (render_data.go:41): a `data` binding evaluated to a JSON array plus a
// `template` (renderItem) subtree instantiated once per item.
//
// Semantics mirrored from the HTML renderer:
//
//   - `data` evaluates against the current scope (state + viewport + any
//     OUTER list's item keys, so nested lists can bind {{item.children}}).
//     Anything that is not an array — a missed binding, an object, a scalar —
//     yields zero items, the HTML quiet degradation (render_data.go:46); a
//     list without a template degrades to a plain column of its children
//     (render_data.go:42-45, handled by the caller in measure).
//   - each item gets an item scope: the outer scope copied, then the item
//     bound under its alias plus the derived index/first/last keys, so the
//     innermost list wins name collisions (itemVars, mirroring itemScope at
//     render_data.go:499). `as` renames the four keys (listAliasNames,
//     mirroring ListAliasNames at render_data.go:465).
//   - handlers dispatch with the item scope live: the invoke's args (and a
//     {{bound}} action name) evaluate against state + the instance's scope,
//     the canvas mirror of the HTML Handler.Scope capture (server.go:1336).
//
// Identity: interaction state is keyed by *model.Node (Interaction), but
// every instance of one template shares that pointer. The composite key is
// therefore (node, index): the index travels from the hit's graph node via a
// per-frame sidecar (itemInstance, recorded by PerformLayout) into the
// interaction companions (PressedItem & friends), and PerformLayout stamps
// the flags back only onto the matching instance.
//
// Documented wave-2 limits:
//   - NESTED repeats collapse the identity to the innermost (node, index)
//     pair: pressing item 2 of an inner list lights item 2 under EVERY outer
//     item. The HTML path's id-suffix chain (render_data.go:113) has no
//     canvas equivalent yet.
//   - style values, if/visible/show, when conditions and an input's `value`
//     inside a template DO get the item scope; the style.go parse path
//     (colors, paddings, widths) does NOT — {{item.x}} there expands to ""
//     (parseStyle has no scope parameter; threading one is style.go surgery
//     this wave avoids).
//   - an input inside a template shares one edit session across instances
//     (the session key is the template pointer, input.go): only the focused
//     instance shows buffer+caret, and {{item.x}} values neither display nor
//     write back.
//   - keyboard focus traversal never enters instances (Focusables walks the
//     model tree, which contains the template once, not the repeats), so
//     Enter/Space activation and the focus ring are pointer-only there.
//   - pagination/separator/groupBy/reorder/refresh/virtualize are HTML list
//     features a later wave may port; item count is capped at maxListItems as
//     the render-budget guard (HTML: maxRenderNodes, render.go:104).

// maxListItems caps repeat expansion — the canvas render budget. A native
// frame re-measures and re-records everything, so an unbounded repeat over a
// runaway binding would hang the render thread; the HTML path bounds the same
// risk with maxRenderNodes.
const maxListItems = 10000

// listScope carries the repeat context down one item's subtree during
// measure: vars is the item's evaluation scope (state + viewport + the
// item/index/first/last keys) and index the item's position in the data —
// the identity half Interaction pairs with the shared template pointer.
// A nil *listScope means "outside any list".
type listScope struct {
	vars  map[string]any
	index int
	// compDepth counts nested JSON-component instantiations on this scope
	// chain (components.go) — self-referential templates are capped at 32.
	compDepth int
}

// itemInstance is the per-frame sidecar entry Layout records for each repeat
// instance's root graph node: the index (identity disambiguation) and the
// scope (handler dispatch). The engine resolves a hit's instance by walking
// ancestors to the first entry — the innermost list wins, matching the scope
// shadowing rules.
type itemInstance struct {
	index int
	vars  map[string]any
}

// listData evaluates a list's `data` binding to its item slice. Non-arrays
// degrade quietly to nil (HTML parity).
func listData(n *model.Node, rt *runtime.Runtime, sc *listScope) []any {
	items, _ := runtime.EvalBinding(n.Data, evalCtxScope(rt, sc)).([]any)
	return items
}

// itemVars builds one item's evaluation scope — the canvas mirror of the HTML
// itemScope (render_data.go:499): the outer scope copied, then the item bound
// under its alias plus the 0-based index and first/last flags. Writing the
// four keys after the copy means the innermost list wins collisions while
// everything outer (including an outer list's `as` bindings) stays visible.
// index/first/last are scope keys, never injected into the item value, so a
// data field named `index` is untouched.
func itemVars(outer map[string]any, alias, idxKey, firstKey, lastKey string, it any, i, total int) map[string]any {
	s := make(map[string]any, len(outer)+4)
	for k, v := range outer {
		s[k] = v
	}
	s[alias] = it
	s[idxKey] = i
	s[firstKey] = i == 0
	s[lastKey] = i == total-1
	return s
}

// reservedScopeAliases are evaluation-context roots an `as` alias must never
// shadow. MUST mirror the renderer's set (render_data.go:449) — the canvas
// backend stays independent of the HTML package, and the parity test pins the
// two so an alias means the same thing on both paths.
var reservedScopeAliases = map[string]bool{
	"state": true, "t": true, "viewport": true, "route": true, "prop": true,
}

// listAliasNames resolves an `as` prop to the four scope names — mirroring
// the HTML renderer's ListAliasNames (render_data.go:465): a reserved or
// non-identifier alias falls back to the defaults, a usable one namespaces
// its derived keys.
func listAliasNames(as string) (alias, idxKey, firstKey, lastKey string) {
	if as == "" || as == "item" || reservedScopeAliases[as] || !isIdent(as) {
		return "item", "index", "first", "last"
	}
	return as, as + "Index", as + "First", as + "Last"
}

// isIdent reports whether s is a plain identifier ([A-Za-z_][A-Za-z0-9_]*) —
// the only names the expression language can reference.
func isIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || (i > 0 && '0' <= c && c <= '9') {
			continue
		}
		return false
	}
	return s != ""
}

// measureListItems expands a list's template once per data item, in document
// order. A conditionally hidden template (if/visible/show, when) yields no
// instance for that item — the same shape the HTML path produces when
// node() drops it.
func measureListItems(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int, root *model.Node, sc *listScope) []*LayoutNode {
	items := listData(n, rt, sc)
	if len(items) == 0 {
		return nil
	}
	if len(items) > maxListItems {
		warnListCapped(root, len(items))
		items = items[:maxListItems]
	}
	var as string
	if v, ok := n.Prop("as"); ok {
		as, _ = v.(string)
	}
	alias, idxKey, firstKey, lastKey := listAliasNames(as)
	outer := evalCtxScope(rt, sc)
	out := make([]*LayoutNode, 0, len(items))
	for i, it := range items {
		vars := itemVars(outer, alias, idxKey, firstKey, lastKey, it, i, len(items))
		cln := measure(n.Template, rt, inter, scale, root, &listScope{vars: vars, index: i})
		if cln == nil {
			continue
		}
		// The instance root carries the scope so PerformLayout can record the
		// dispatch sidecar; descendants only needed it at evaluation time.
		cln.ItemScope = vars
		out = append(out, cln)
	}
	return out
}

// listWarn* implement the one-shot item-cap warning, keyed by the scene root
// like the style-key warnings (style.go): the per-frame measure pass never
// spams, a scene switch or reload re-arms.
var (
	listWarnMu   sync.Mutex
	listWarnRoot *model.Node
	listWarned   bool
)

func warnListCapped(root *model.Node, total int) {
	listWarnMu.Lock()
	defer listWarnMu.Unlock()
	if root != listWarnRoot {
		listWarnRoot = root
		listWarned = false
	}
	if listWarned {
		return
	}
	listWarned = true
	fmt.Fprintf(os.Stderr, "[qorm canvas] list data has %d items, capped at %d (render budget)\n", total, maxListItems)
}
