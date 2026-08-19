package testrunner

import (
	"fmt"
	"strings"

	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

// matNode is one materialized node of the current scene: the model's static
// tree after conditional visibility (if/visible/show, `when`) and binding
// interpolation have been applied — the same view the renderer shows.
type matNode struct {
	n       *model.Node
	id, typ string
	// text is the node's materialized textual content in renderer order:
	// text, label, value, placeholder. Empty when the node carries none.
	text string
	// props holds every materialized scalar prop under its key (props, then
	// style, then layout — the same maps an author reads). Non-scalar values
	// (arrays, objects) are not materialized in the MVP.
	props map[string]string
	// ctx is the evaluation scope this node was materialized under (state +
	// any enclosing list item / JSON-component prop map). simulate_event
	// evaluates handler args against it so {{item.id}} / {{prop.x}} match
	// the HTML Handler.Scope and canvas itemInstance paths.
	ctx map[string]any
}

// bindingCtx is the renderer's expression context: state, t (catalog), route,
// viewport and computed — everything a {{...}} binding in this app can read.
func bindingCtx(rt *qrt.Runtime) map[string]any {
	return map[string]any{
		"state":    rt.State,
		"t":        rt.Catalog(),
		"viewport": rt.ViewportVars(),
		"route":    rt.RouteParams,
		"computed": rt.ComputedVars(),
	}
}

// sceneName is the scene the runtime currently shows ("" = the entry scene),
// for error messages.
func sceneName(rt *qrt.Runtime) string {
	if rt.Scene != "" {
		return rt.Scene
	}
	return rt.App.Entry
}

// materialize walks the runtime's current scene model tree and returns the
// nodes that would render, depth-first, with bindings evaluated. Mirror of
// the renderer's node()/when()/visible() rules: `when` nodes render only
// their chosen branch, nodes with a falsy if/visible/show render nothing,
// and every text/binding is interpolated against the live state. List and
// gridview `renderItem` templates are instantiated once per data item (item
// / index / first / last in scope, matching the HTML and canvas renderers).
// JSON-component instances expand their template with {{prop.x}} and slots,
// the same contract as HTML renderComponent and canvas instantiateComponent.
func materialize(rt *qrt.Runtime) []*matNode {
	root := rt.App.EntryRoot()
	if sc := rt.App.Scenes[rt.Scene]; sc != nil {
		root = sc
	}
	base := bindingCtx(rt)
	var out []*matNode
	var walk func(n *model.Node, ctx map[string]any, depth int)
	walk = func(n *model.Node, ctx map[string]any, depth int) {
		if n == nil {
			return
		}
		if n.Type == "when" {
			branch := n.Else
			if n.Condition != "" && asBool(qrt.EvalBinding(n.Condition, ctx)) {
				branch = n.Then
			}
			// The when container itself renders nothing; the branch does.
			if branch != nil {
				walk(branch, ctx, depth)
			}
			return
		}
		if !visible(rt, n, ctx) {
			return
		}
		typ := n.Type
		if strings.Contains(typ, "{{") {
			typ = qrt.Stringify(qrt.EvalBinding(typ, ctx)) // bound `type`
		}
		out = append(out, &matNode{
			n: n, id: nodeID(n, ctx), typ: typ,
			text: nodeText(rt, n, ctx), props: nodeProps(rt, n, ctx), ctx: ctx,
		})
		if name := componentRef(rt, n); name != "" && depth < maxCompDepth {
			clone, inner := instantiateComponent(n, rt.App.Components[name], name, ctx, rt)
			walk(clone, inner, depth+1)
			return
		}
		if isRepeat(typ) {
			tmpl := n.Template
			if tmpl == nil {
				for _, c := range n.Children {
					walk(c, ctx, depth)
				}
				return
			}
			items := listItems(n, ctx)
			if len(items) > maxListItems {
				items = items[:maxListItems]
			}
			as := ""
			if raw, ok := n.Prop("as"); ok {
				as = fmt.Sprint(raw)
			}
			alias, idxKey, firstKey, lastKey := listAliasNames(as)
			for i, it := range items {
				walk(tmpl, itemScope(ctx, alias, idxKey, firstKey, lastKey, it, i, len(items)), depth)
			}
			return
		}
		for _, c := range n.Children {
			walk(c, ctx, depth)
		}
	}
	walk(root, base, 0)
	return out
}

// nodeID is the node's materialized id: a binding in the id field evaluates
// in the current scope so list rows can be addressed as todo_done_{{item.id}}.
func nodeID(n *model.Node, ctx map[string]any) string {
	if n == nil || n.ID == "" {
		return ""
	}
	if strings.Contains(n.ID, "{{") {
		return qrt.Stringify(qrt.EvalBinding(n.ID, ctx))
	}
	return n.ID
}

// maxListItems caps repeat expansion in tests — same budget as the canvas
// renderer, so a runaway binding cannot hang the runner.
const maxListItems = 10000

func isRepeat(typ string) bool {
	switch strings.ToLower(typ) {
	case "list", "gridview":
		return true
	}
	return false
}

func listItems(n *model.Node, ctx map[string]any) []any {
	raw := n.Data
	if raw == "" {
		if v, ok := n.Prop("data"); ok {
			raw = fmt.Sprint(v)
		}
	}
	if raw == "" {
		return nil
	}
	items, _ := qrt.EvalBinding(raw, ctx).([]any)
	return items
}

func itemScope(outer map[string]any, alias, idxKey, firstKey, lastKey string, it any, i, total int) map[string]any {
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

// listAliasNames mirrors render.ListAliasNames / canvas.listAliasNames so an
// `as` alias means the same thing in tests as on both render paths.
func listAliasNames(as string) (alias, idxKey, firstKey, lastKey string) {
	if as == "" || as == "item" || reservedScopeAliases[as] || !isIdent(as) {
		return "item", "index", "first", "last"
	}
	return as, as + "Index", as + "First", as + "Last"
}

var reservedScopeAliases = map[string]bool{
	"state": true, "t": true, "viewport": true, "route": true, "prop": true,
}

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

// visible mirrors the renderer's visibility rule: the first of
// if/visible/show props decides (falsy hides the node).
func visible(rt *qrt.Runtime, n *model.Node, ctx map[string]any) bool {
	for _, key := range []string{"if", "visible", "show"} {
		if raw, ok := n.Prop(key); ok {
			return asBool(qrt.EvalBinding(fmt.Sprint(raw), ctx))
		}
	}
	return true
}

// nodeText is the materialized textual content of a node, in the order the
// author-facing widgets read: text, then button label, then input value /
// placeholder. Empty for structural containers.
func nodeText(rt *qrt.Runtime, n *model.Node, ctx map[string]any) string {
	for _, f := range []string{n.Text, n.Label, n.Value, n.Placeholder} {
		if f != "" {
			return qrt.Stringify(qrt.EvalBinding(f, ctx))
		}
	}
	return ""
}

// nodeProps materializes the node's props/style/layout map values. Keys keep
// their declared spelling (a prop_equals on "label", "fontSize" or
// "margin.top"-style nested keys resolves via propLookup); values that carry
// bindings are interpolated, scalars are stringified canonically.
func nodeProps(rt *qrt.Runtime, n *model.Node, ctx map[string]any) map[string]string {
	out := make(map[string]string, len(n.Props)+len(n.Style)+len(n.Layout))
	collect := func(m map[string]any) {
		for k, v := range m {
			out[propName(n.Type, k)] = materializeValue(rt, v, ctx)
			// Nested style objects stay addressable as "key.sub".
			if inner, ok := v.(map[string]any); ok {
				for sk, sv := range inner {
					out[k+"."+sk] = materializeValue(rt, sv, ctx)
				}
			}
		}
	}
	collect(n.Props)
	collect(n.Style)
	collect(n.Layout)
	return out
}

// propName keeps the keys an author would query ("label") free of a widget
// prefix; unused in the MVP but kept so the prop namespace has one defined
// rule as it grows.
func propName(nodeType, key string) string { return key }

// materializeValue stringifies a raw prop value with its bindings evaluated.
// A whole-string binding returns the typed value (canonically stringified),
// mixed text returns the interpolation, plain scalars pass through.
func materializeValue(rt *qrt.Runtime, v any, ctx map[string]any) string {
	s, ok := v.(string)
	if !ok {
		return qrt.Stringify(v)
	}
	return qrt.Stringify(qrt.EvalBinding(s, ctx))
}

// asBool is the runtime truthiness used by every expression context: booleans
// directly, strings "true"/"1"/"yes"/"on" (case-insensitive; anything else
// false), numbers non-zero.
func asBool(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "on":
			return true
		}
		return false
	case float64:
		return t != 0
	default:
		if s := qrt.Stringify(v); s != "" {
			return s != "false" && s != "0"
		}
		return false
	}
}

// matchSelector returns the materialized nodes a target selector matches.
// Selectors carry only id/type/text keys; anything else is a query error the
// caller has already surfaced through validateSelector.
func matchSelector(nodes []*matNode, sel map[string]any) []*matNode {
	id := selField(sel, "id")
	typ := selField(sel, "type")
	text := selField(sel, "text")
	var out []*matNode
	for _, mn := range nodes {
		if id != "" && strings.ToLower(mn.id) != id {
			continue
		}
		if typ != "" && strings.ToLower(mn.typ) != typ {
			continue
		}
		if text != "" && mn.text != text {
			continue
		}
		out = append(out, mn)
	}
	return out
}

// selField reads a selector field as its string value. Absent fields read ""
// — never fmt.Sprint's "<nil>", which would silently poison every match.
func selField(sel map[string]any, key string) string {
	s, _ := sel[key].(string)
	return s
}

// selectorKeys are the fields a target selector may carry in the MVP: the
// spec's id/type/text match fields. semantic is deferred (the model has no
// semantic-slot yet).
var selectorKeys = map[string]bool{"id": true, "type": true, "text": true}

// validateSelector enforces the spec's query rules: a target selector must be
// a map whose keys are all selector fields. `path` is refused with
// query_invalid_selector and a hint to use state_equals (path selects on
// state, not nodes); the scoped-selector forms (`within`, `match`) and any
// unrecognised key are refused the same way, so a query that is not what the
// runner can answer never degrades into a green false positive.
func validateSelector(sel map[string]any) error {
	if len(sel) == 0 {
		return fmt.Errorf("%s: empty target selector (give {\"id\": ...} or a type/text match)", ErrInvalidSelector)
	}
	for _, k := range sortedKeys(sel) {
		if k == "path" {
			return fmt.Errorf("%s: target selector carries \"path\" — path reads STATE, not nodes; use a state_equals assert on %q", ErrInvalidSelector, fmt.Sprint(sel[k]))
		}
		if !selectorKeys[k] {
			return fmt.Errorf("%s: target selector key %q not supported in the MVP (supported: id, type, text; within/match/semantic and scoped selectors are deferred)", ErrInvalidSelector, k)
		}
	}
	return nil
}

// selectorString renders a selector stably for failure messages and reports.
func selectorString(sel map[string]any) string {
	if len(sel) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sel))
	for _, k := range sortedKeys(sel) {
		parts = append(parts, fmt.Sprintf("%s:%s", k, selField(sel, k)))
	}
	return strings.Join(parts, ", ")
}

// evalAssert runs one assertion against the materialized tree and live state,
// returning nil on pass or a Failure (code set per outcome) on anything else.
func evalAssert(rt *qrt.Runtime, a Assert) *Failure {
	switch a.Type {
	case "state_equals":
		got := qrt.Stringify(rt.StatePath(a.Path))
		want := qrt.Stringify(a.Value)
		if got != want {
			return &Failure{Code: ErrAssertionFailed, Assert: a.Type, Target: a.Path, Expected: want, Actual: got}
		}
		return nil
	case "node_exists", "node_not_exists":
		if err := validateSelector(a.Target); err != nil {
			return &Failure{Code: ErrInvalidSelector, Assert: a.Type, Target: selectorString(a.Target), Message: err.Error()}
		}
		n := len(matchSelector(materialize(rt), a.Target))
		exists := n > 0
		wantExists := a.Type == "node_exists"
		if exists != wantExists {
			msg := fmt.Sprintf("expected node to %s", verb(wantExists))
			return &Failure{Code: ErrAssertionFailed, Assert: a.Type, Target: selectorString(a.Target), Expected: verb(wantExists), Actual: verb(exists), Message: msg}
		}
		return nil
	case "text_equals", "prop_equals":
		if err := validateSelector(a.Target); err != nil {
			return &Failure{Code: ErrInvalidSelector, Assert: a.Type, Target: selectorString(a.Target), Message: err.Error()}
		}
		matches := matchSelector(materialize(rt), a.Target)
		if len(matches) == 0 {
			return &Failure{Code: ErrTargetNotFound, Assert: a.Type, Target: selectorString(a.Target), Expected: qrt.Stringify(a.Value), Message: fmt.Sprintf("no node matches selector {%s} in scene %q", selectorString(a.Target), sceneName(rt))}
		}
		if len(matches) > 1 {
			return &Failure{Code: ErrQueryAmbiguous, Assert: a.Type, Target: selectorString(a.Target), Expected: qrt.Stringify(a.Value), Message: fmt.Sprintf("selector {%s} matches %d nodes; %s needs exactly one", selectorString(a.Target), len(matches), a.Type)}
		}
		mn := matches[0]
		if a.Type == "text_equals" {
			got := mn.text
			want := qrt.Stringify(a.Value)
			if got != want {
				return &Failure{Code: ErrAssertionFailed, Assert: a.Type, Target: "id:" + mn.id, Expected: want, Actual: got}
			}
			return nil
		}
		got, present := lookupProp(mn, a.Prop)
		want := qrt.Stringify(a.Value)
		if !present || got != want {
			actual := got
			if !present {
				actual = "<missing>"
			}
			return &Failure{Code: ErrAssertionFailed, Assert: a.Type, Target: "id:" + mn.id + " prop:" + a.Prop, Expected: want, Actual: actual}
		}
		return nil
	}
	return &Failure{Code: ErrAssertUnknown, Assert: a.Type, Message: fmt.Sprintf("unknown assert type %q", a.Type)}
}

// lookupProp reads a materialized prop, resolving a dotted key against nested
// style/layout maps (the flat "key.sub" mirrors materialize's nesting).
func lookupProp(mn *matNode, key string) (string, bool) {
	if v, ok := mn.props[key]; ok {
		return v, true
	}
	// A dotted key already flattened by nodeProps; fall through if missing.
	return "", false
}

// verb turns an existence boolean into the message word the reports read.
func verb(exists bool) string {
	if exists {
		return "exist"
	}
	return "not exist"
}
