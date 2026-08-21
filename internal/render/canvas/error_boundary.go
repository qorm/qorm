package canvas

import (
	"fmt"
	"strings"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// boundaryTrap records a render failure while measuring under a node
// errorBoundary. Mirrors HTML renderer's boundaryHit/boundaryMsg: unknown
// widget types and panics trip it so the boundary can swap in fallback.
type boundaryTrap struct {
	hit bool
	msg string
}

func (t *boundaryTrap) trip(msg string) {
	if t == nil {
		return
	}
	t.hit = true
	if t.msg == "" {
		t.msg = msg
	}
}

func scopeTrap(sc *listScope) *boundaryTrap {
	if sc == nil {
		return nil
	}
	return sc.trap
}

// withTrap returns a listScope that carries trap (and copies vars/index/depth
// from sc when present). Used so nested measure under an errorBoundary still
// reports unknown widgets / panics back to the boundary root.
func withTrap(sc *listScope, trap *boundaryTrap) *listScope {
	if sc == nil {
		return &listScope{trap: trap}
	}
	out := *sc
	out.trap = trap
	return &out
}

// resolveBoundType mirrors HTML render.resolveType: {"type":"{{state.kind}}"}
// becomes a concrete widget name against the current scope. Returns n unchanged
// when there is no binding or evaluation fails.
func resolveBoundType(n *model.Node, rt *runtime.Runtime, sc *listScope) *model.Node {
	if n == nil || !strings.Contains(n.Type, "{{") {
		return n
	}
	t := strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(n.Type, evalCtxScope(rt, sc))))
	if t == "" || strings.Contains(t, "{{") {
		return n
	}
	cp := *n
	cp.Type = t
	return &cp
}

// canvasTypeKnown reports whether typ is a type the canvas engine (or the
// registered widget / JSON-component catalog) understands. Unknown names
// under an errorBoundary trap fail the boundary — matching HTML unknown().
func canvasTypeKnown(rt *runtime.Runtime, typ string) bool {
	if typ == "" || strings.Contains(typ, "{{") {
		return false
	}
	switch typ {
	case "text", "button", "richtext", "input", "image",
		"row", "column", "columns", "list", "grid", "gridview",
		"stack", "absolute", "scroll", "scrollview", "board",
		"when", "slot", "path", "tilemap", "timer",
		"animated_container", "animatedcontainer", "component",
		// HTML container aliases that canvas lays out as flex boxes.
		"card", "flex", "box", "div", "container", "group", "view",
		"fragment", "wrapper", "panel", "body", "content", "main",
		"section", "header", "footer", "aside", "nav",
		"center", "start", "end", "between", "around", "evenly", "stretch",
		"vstack", "hstack", "zstack":
		return true
	}
	if _, ok := LookupWidget(typ); ok {
		return true
	}
	if rt != nil && rt.App != nil && rt.App.Components[typ] != nil {
		return true
	}
	return false
}

// measureErrorBoundary tries the protected subtree under a trap; on failure
// (unknown widget or panic) records LastBoundaryError and measures fallback.
func measureErrorBoundary(n *model.Node, rt *runtime.Runtime, inter *Interaction, scale int, root *model.Node, sc *listScope, underBoard bool, scrollCtx *listScrollCtx) (ln *LayoutNode) {
	trap := &boundaryTrap{}
	var panicked string
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicked = fmt.Sprint(rec)
			}
		}()
		cp := *n
		cp.ErrorBoundary = nil
		ln = measure(&cp, rt, inter, scale, root, withTrap(sc, trap), underBoard, scrollCtx)
	}()
	if panicked == "" && !trap.hit {
		return ln
	}
	msg := panicked
	if msg == "" {
		msg = trap.msg
	}
	if msg == "" {
		msg = "boundary render failed"
	}
	if rt != nil {
		scene := ""
		if rt.App != nil {
			scene = rt.CurrentScene()
		}
		rt.LastBoundaryError = runtime.BoundaryError{
			Level:   "node",
			Phase:   "render",
			Scene:   scene,
			NodeID:  n.ID,
			Message: msg,
		}
	}
	if n.ErrorBoundary == nil || n.ErrorBoundary.Fallback == nil {
		return nil
	}
	return measure(n.ErrorBoundary.Fallback, rt, inter, scale, root, sc, underBoard, scrollCtx)
}
