package mcp

import (
	"encoding/json"
	"fmt"
	"github.com/qorm/qorm/internal/capability"
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/a11y"
	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/measure"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func toolList() []tool {
	strProp := map[string]any{"type": "string"}
	intProp := map[string]any{"type": "integer"}
	return []tool{
		{
			Name:        "qorm_window",
			Description: "Control the desktop app window: op=move needs x,y,w,h (top-left px); op=focus/minimize/pin/unpin act on the window. The control engine positions the user's window. Supported on macOS and Windows desktop apps.",
			InputSchema: obj(map[string]any{"id": strProp, "url": strProp, "js": strProp, "op": map[string]any{"type": "string", "enum": []string{"move", "open", "close", "eval", "tile", "focus", "minimize", "pin", "unpin"}}, "x": intProp, "y": intProp, "w": intProp, "h": intProp}),
		},
		{
			Name:        "qorm_inspect",
			Description: "Inspect the QORM app: id, name, entry scene, scene ids, state schema, current state, action ids, static compiler diagnostics, and the design-token system (designTokens: name -> {type,value,enforce}) when declared. Enforced color tokens hard-constrain apply_patch: a color style may only be set to one of their values. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_render_html",
			Description: "Render the current app to HTML so the agent can see what the UI looks like. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_a11y_tree",
			Description: "Derive the accessibility tree for the entry scene: every node's ARIA role, accessible name and semantic state (checked/disabled/required/value), plus an audit of accessibility issues — interactive controls and images that would reach a screen reader with no accessible name. Use it to check a11y coverage or find what to fix. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_capabilities",
			Description: "List all built-in hardware/native capabilities: each capability's canonical name + widget type, the qormToNative op strings it accepts, its qormOn<Name> callback, and which platforms (ios/android/mac/linux/windows/web) implement it. Read-only — how an agent discovers what hardware exists and exactly how to call it. Mini-program is a static export target: no live tools apply.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_get_node",
			Description: "Return a node's type, props, and child ids by node id. Read-only.",
			InputSchema: obj(map[string]any{"id": strProp}, "id"),
		},
		{
			Name:        "qorm_query",
			Description: "Find nodes matching a selector (any of: type, textContains, idContains, hasProp — combined with AND). Returns each match's id, type, label and ancestor path. Use this to locate nodes before patching. Read-only.",
			InputSchema: obj(map[string]any{
				"type":         strProp,
				"textContains": strProp,
				"idContains":   strProp,
				"hasProp":      strProp,
			}),
		},
		{
			Name:        "qorm_list_actions",
			Description: "List available actions and a summary of each action's steps. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_activity",
			Description: "Read the shared session's live presence: returns {events:[who (human/agent) did what, oldest to newest], humanFocus:{element, secondsAgo}, humanTyping:{entry, secondsAgo}, humanFilled:{field, secondsAgo}} — so the agent sees what the human just did, the element they are on now, the text they last typed, AND which hidden (password) fields they filled (label only; a password value is never captured), and collaborates in context. Only available in a running `qorm run` session. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_export_scene",
			Description: "Serialise the current (possibly patched) entry scene back to QORM JSON, so design work done via apply_patch can be saved or shipped. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_export_bundle",
			Description: "Serialise the whole current app (manifest + scenes + actions) into an UNSIGNED bundle (with content hash). A human/CI signs it (`qorm sign`) before OTA deploy — the agent never holds the signing key. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_simulate_action",
			Description: "Dispatch an action against a COPY of state and return before/after/changed. Side-effect-free: the live app is never modified.",
			InputSchema: obj(map[string]any{
				"action": strProp,
				"args":   map[string]any{"type": "object"},
			}, "action"),
		},
		// ---- operate ----
		{
			Name:        "qorm_dispatch",
			Description: "OPERATE the live app: dispatch an action (mutating state) and return the new state and rendered HTML.",
			InputSchema: obj(map[string]any{
				"action": strProp,
				"args":   map[string]any{"type": "object"},
			}, "action"),
		},
		{
			Name:        "qorm_set_state",
			Description: "OPERATE the live app: set a state path to a value and return the new state and rendered HTML.",
			InputSchema: obj(map[string]any{
				"path":  strProp,
				"value": map[string]any{},
			}, "path", "value"),
		},
		// ---- test ----
		{
			Name:        "qorm_assert",
			Description: "TEST the app: evaluate checks against current state and rendered HTML. Each check is {kind: 'stateEquals'|'htmlContains'|'nodeExists', ...}. Returns per-check pass/fail and overall.",
			InputSchema: obj(map[string]any{
				"checks": map[string]any{"type": "array"},
			}, "checks"),
		},
		// ---- design (preview -> apply safety model) ----
		{
			Name:        "qorm_preview_patch",
			Description: "DESIGN (safe): apply patch ops to a COPY of the app and return the resulting HTML plus a previewToken. Side-effect-free — the live app is unchanged. Ops: {op:'setProp',target,key,value} | {op:'addChild',target,node} | {op:'insertBefore'|'insertAfter',target,node} | {op:'replace',target,node} | {op:'wrap',target,node} | {op:'move',target,into} | {op:'remove',target}.",
			InputSchema: obj(map[string]any{
				"ops": map[string]any{"type": "array"},
			}, "ops"),
		},
		{
			Name:        "qorm_diff",
			Description: "DESIGN (safe): show the structural diff a patch would make (added/removed node ids and, per changed node, which fields) without touching the live app. Review before apply.",
			InputSchema: obj(map[string]any{"ops": map[string]any{"type": "array"}}, "ops"),
		},
		{
			Name:        "qorm_apply_patch",
			Description: "DESIGN (commit): apply patch ops to the LIVE app. Must pass the previewToken returned by qorm_preview_patch for the same ops — apply is bound to a review. Snapshots the pre-image so it can be undone. If the app declares enforced color design tokens (see qorm_inspect designTokens), a setProp style op that sets a color style to a non-token value is rejected (also at preview time).",
			InputSchema: obj(map[string]any{
				"ops":          map[string]any{"type": "array"},
				"previewToken": strProp,
			}, "ops", "previewToken"),
		},
		{
			Name:        "qorm_undo",
			Description: "DESIGN: revert the last applied patch, restoring the app to its state before that apply. Returns the reverted HTML and remaining undo depth.",
			InputSchema: obj(nil),
		},
		// ---- interpret & verify the real rendered result ----
		{
			Name:        "qorm_measure",
			Description: "INTERPRET the LIVE render precisely: returns every component joining what the user expressed (type, text, state binding) with how it actually rendered — x,y,w,h, visible, and computed color/background/fontSize/fontWeight/padding/borderRadius/border/opacity/zIndex/position/x-overflow — as measured by the running app in its own window. Requires the app to be open in a window/browser (it self-measures on load and after every change). Use to see exactly how the user's app rendered.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_check_layout",
			Description: "VERIFY the LIVE render against expectations; returns per-check pass/fail with actual values. `checks` is an array of {id, <assertions>}. Assertions: visible(bool) | type(widget-type string) | text(substring the component must contain, matched vs expressed OR rendered text) | noOverflow(bool, no horizontal overflow) | minW|maxW|minH|maxH(px number) | x|y(px number, ±3 tolerance) | within(id: this box must sit inside that id's box) | below(id: must start below that id) | backgroundNot|colorNot(substring that must be ABSENT — e.g. \"255, 255, 255\" to assert not-white in dark mode) | role(the rendered ARIA role string, incl. roles the renderer injects) | hasAriaLabel(bool) | contrastRatio(min WCAG ratio, e.g. 4.5 for AA normal text — computed against the effective background). Example: [{\"id\":\"wifi\",\"type\":\"switchlisttile\",\"visible\":true,\"within\":\"settings\"},{\"id\":\"chart\",\"noOverflow\":true}]. Requires the app open in a window (it self-measures). Optional viewportW/viewportH (px) set the runtime viewport before evaluating, so responsive `when` branches resolve as if the window were that size — note the measured rects still come from the client's REAL window (a live client also overwrites the viewport on its next load/resize).",
			InputSchema: obj(map[string]any{"checks": map[string]any{"type": "array"}, "viewportW": intProp, "viewportH": intProp}, "checks"),
		},
	}
}

func (s *Server) handleToolCall(req request) *response {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return fail(req.ID, -32602, "invalid params")
	}
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	result, err := s.callTool(p.Name, p.Arguments)
	if err != nil {
		return ok(req.ID, toolText(true, err.Error()))
	}
	if s.afterMutate != nil && isMutating(p.Name) {
		s.afterMutate() // must not take s.mu (server bumps rev lock-free)
	}
	return ok(req.ID, toolText(false, result))
}

func isMutating(name string) bool {
	switch name {
	case "qorm_dispatch", "qorm_set_state", "qorm_apply_patch", "qorm_undo":
		return true
	}
	return false
}

// toolText wraps text in the MCP tools/call result shape.
func toolText(isError bool, text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func (s *Server) callTool(name string, args json.RawMessage) (string, error) {
	switch name {
	case "qorm_window":
		if s.windowMover == nil {
			return "", fmt.Errorf("window control unavailable (not a native desktop app)")
		}
		var a struct {
			ID, Op, URL, JS string
			X, Y, W, H      int
		}
		_ = json.Unmarshal(args, &a)
		if a.ID == "" {
			a.ID = "main"
		}
		switch a.Op {
		case "open":
			if s.windowOpen != nil {
				s.windowOpen(a.ID, a.URL, a.W, a.H)
			}
			return "opened window " + a.ID, nil
		case "eval":
			if s.windowEval != nil {
				s.windowEval(a.ID, a.JS)
			}
			return "eval sent to " + a.ID, nil
		case "", "move":
			s.windowMover(a.ID, a.X, a.Y, a.W, a.H)
			return fmt.Sprintf("moved window %s to (%d,%d) %dx%d", a.ID, a.X, a.Y, a.W, a.H), nil
		default:
			if s.windowOp != nil {
				s.windowOp(a.ID, a.Op)
			}
			return "window " + a.ID + " op: " + a.Op, nil
		}
	case "qorm_inspect":
		return jsonPretty(s.inspect()), nil
	case "qorm_render_html":
		return render.Render(s.rt).HTML, nil
	case "qorm_a11y_tree":
		return jsonPretty(a11y.Build(s.rt.App.EntryRoot())), nil
	case "qorm_capabilities":
		return jsonPretty(capability.All), nil
	case "qorm_list_actions":
		return jsonPretty(s.listActions()), nil
	case "qorm_activity":
		if s.activityProv == nil {
			return "", fmt.Errorf("activity log unavailable (only in a running `qorm run` shared session)")
		}
		return s.activityProv(), nil
	case "qorm_export_scene":
		return jsonPretty(loader.SceneToJSON(s.rt.App.Entry, s.rt.App.EntryRoot())), nil
	case "qorm_export_bundle":
		b, err := bundle.FromApp(s.rt.App)
		if err != nil {
			return "", err
		}
		data, err := bundle.Marshal(b)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "qorm_query":
		var sel selector
		_ = json.Unmarshal(args, &sel)
		matches := queryNodes(s.rt.App.EntryRoot(), sel)
		return jsonPretty(map[string]any{"count": len(matches), "matches": matches}), nil
	case "qorm_get_node":
		var a struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(args, &a)
		node := findNode(s.rt.App.EntryRoot(), a.ID)
		if node == nil {
			return "", fmt.Errorf("node %q not found", a.ID)
		}
		return jsonPretty(nodeInfo(node)), nil
	case "qorm_simulate_action":
		var a struct {
			Action string         `json:"action"`
			Args   map[string]any `json:"args"`
		}
		_ = json.Unmarshal(args, &a)
		return jsonPretty(s.simulate(a.Action, a.Args)), nil
	case "qorm_dispatch":
		var a struct {
			Action string         `json:"action"`
			Args   map[string]any `json:"args"`
		}
		_ = json.Unmarshal(args, &a)
		if _, ok := s.rt.App.Actions[a.Action]; !ok {
			return "", fmt.Errorf("unknown action %q", a.Action)
		}
		s.rt.Dispatch(a.Action, a.Args)
		return jsonPretty(s.stateAndHTML()), nil
	case "qorm_set_state":
		var a struct {
			Path  string `json:"path"`
			Value any    `json:"value"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Path == "" {
			return "", fmt.Errorf("set_state requires path and value")
		}
		s.rt.State[a.Path] = a.Value
		return jsonPretty(s.stateAndHTML()), nil
	case "qorm_measure":
		if s.measureProv == nil {
			return "", fmt.Errorf("measurement unavailable (server not wired for it)")
		}
		m := s.measureProv()
		if len(m) <= 2 {
			return "", fmt.Errorf("no measurement yet — open the app in a window/browser so it can self-measure")
		}
		rep, err := measure.Report(s.rt, m)
		return string(rep), err
	case "qorm_check_layout":
		var a struct {
			Checks    json.RawMessage `json:"checks"`
			ViewportW int             `json:"viewportW"`
			ViewportH int             `json:"viewportH"`
		}
		json.Unmarshal(args, &a)
		if a.ViewportW > 0 || a.ViewportH > 0 {
			// Simulate a viewport so responsive `when` branches resolve for the
			// check. The live browser client re-reports its real size on its next
			// load/resize, overwriting this.
			s.rt.Viewport = qrt.Viewport{W: a.ViewportW, H: a.ViewportH}
		}
		if s.measureProv == nil {
			return "", fmt.Errorf("measurement unavailable")
		}
		m := s.measureProv()
		if len(m) <= 2 {
			return "", fmt.Errorf("no measurement yet — open the app in a window/browser so it can self-measure")
		}
		rep, err := measure.Eval(s.rt, m, a.Checks)
		return string(rep), err
	case "qorm_assert":
		var a struct {
			Checks []map[string]any `json:"checks"`
		}
		_ = json.Unmarshal(args, &a)
		return jsonPretty(s.assert(a.Checks)), nil
	case "qorm_preview_patch":
		var a struct {
			Ops []PatchOp `json:"ops"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid ops")
		}
		return jsonPretty(s.previewPatch(a.Ops)), nil
	case "qorm_diff":
		var a struct {
			Ops []PatchOp `json:"ops"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid ops")
		}
		clone := cloneApp(s.rt.App)
		if err := applyPatch(clone, a.Ops); err != nil {
			return "", err
		}
		return jsonPretty(diffApps(s.rt.App, clone)), nil
	case "qorm_apply_patch":
		var a struct {
			Ops          []PatchOp `json:"ops"`
			PreviewToken string    `json:"previewToken"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid ops")
		}
		res, err := s.applyPatchTool(a.Ops, a.PreviewToken)
		if err != nil {
			return "", err
		}
		return jsonPretty(res), nil
	case "qorm_undo":
		res, err := s.undo()
		if err != nil {
			return "", err
		}
		return jsonPretty(res), nil
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// stateAndHTML is the standard result of an operate/design mutation.
func (s *Server) stateAndHTML() map[string]any {
	return map[string]any{
		"state": s.rt.State,
		"html":  render.RenderScene(s.rt, s.rt.CurrentScene()).HTML,
	}
}

// assert evaluates test checks against live state and rendered HTML.
func (s *Server) assert(checks []map[string]any) map[string]any {
	htmlOut := render.Render(s.rt).HTML
	results := make([]map[string]any, 0, len(checks))
	allPass := true
	for _, c := range checks {
		kind, _ := c["kind"].(string)
		pass := false
		detail := ""
		switch kind {
		case "stateEquals":
			path, _ := c["path"].(string)
			got := s.rt.State[path]
			pass = fmt.Sprint(got) == fmt.Sprint(c["value"])
			detail = fmt.Sprintf("state[%s]=%v want %v", path, got, c["value"])
		case "htmlContains":
			sub, _ := c["text"].(string)
			pass = strings.Contains(htmlOut, sub)
			detail = fmt.Sprintf("html contains %q", sub)
		case "nodeExists":
			id, _ := c["id"].(string)
			pass = findNode(s.rt.App.EntryRoot(), id) != nil
			detail = fmt.Sprintf("node %q exists", id)
		default:
			detail = "unknown check kind " + kind
		}
		if !pass {
			allPass = false
		}
		results = append(results, map[string]any{"kind": kind, "pass": pass, "detail": detail})
	}
	return map[string]any{"pass": allPass, "checks": results}
}

// previewPatch applies ops to a clone and returns HTML + a binding token.
func (s *Server) previewPatch(ops []PatchOp) map[string]any {
	clone := cloneApp(s.rt.App)
	if err := applyPatch(clone, ops); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	token := patchToken(ops)
	s.preview = &previewState{token: token, ops: ops}
	previewRt := &qrt.Runtime{App: clone, State: s.rt.State, Viewport: s.rt.Viewport}
	return map[string]any{
		"ok":           true,
		"previewToken": token,
		"html":         render.Render(previewRt).HTML,
	}
}

// applyPatchTool commits ops to the live app, requiring a matching preview.
func (s *Server) applyPatchTool(ops []PatchOp, token string) (map[string]any, error) {
	if s.preview == nil || s.preview.token != token || patchToken(ops) != token {
		return nil, fmt.Errorf("apply_patch must be bound to a matching qorm_preview_patch (call preview first and pass its previewToken)")
	}
	// Atomic (all-or-nothing): apply the whole batch to a clone; only if every
	// op succeeds do we swap it in. On any failure the live app is untouched.
	working := cloneApp(s.rt.App)
	if err := applyPatch(working, ops); err != nil {
		return nil, err
	}
	s.history = append(s.history, s.rt.App) // pre-image for undo
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	s.rt.App = working // atomic swap
	s.preview = nil
	return map[string]any{"ok": true, "undoDepth": len(s.history), "html": render.Render(s.rt).HTML}, nil
}

// undo restores the app to the state before the last apply_patch.
func (s *Server) undo() (map[string]any, error) {
	if len(s.history) == 0 {
		return nil, fmt.Errorf("nothing to undo")
	}
	s.rt.App = s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.preview = nil
	return map[string]any{"ok": true, "undoDepth": len(s.history), "html": render.Render(s.rt).HTML}, nil
}

func (s *Server) inspect() map[string]any {
	sceneIDs := make([]string, 0, len(s.rt.App.Scenes))
	for id := range s.rt.App.Scenes {
		sceneIDs = append(sceneIDs, id)
	}
	sort.Strings(sceneIDs)
	actionIDs := make([]string, 0, len(s.rt.App.Actions))
	for id := range s.rt.App.Actions {
		actionIDs = append(actionIDs, id)
	}
	sort.Strings(actionIDs)
	out := map[string]any{
		"id":           s.rt.App.ID,
		"name":         s.rt.App.Name,
		"entry":        s.rt.App.Entry,
		"scenes":       sceneIDs,
		"actions":      actionIDs,
		"stateSchema":  s.rt.App.GlobalState.Schema,
		"currentState": s.rt.State,
		"diagnostics":  s.rt.App.Diagnostics,
	}
	// Surface the design-token system so the agent knows which token values it
	// may use; enforced color tokens hard-constrain apply_patch style edits.
	if len(s.rt.App.DesignTokens) > 0 {
		out["designTokens"] = s.rt.App.DesignTokens
	}
	return out
}

func (s *Server) listActions() []map[string]any {
	out := make([]map[string]any, 0, len(s.rt.App.Actions))
	ids := make([]string, 0, len(s.rt.App.Actions))
	for id := range s.rt.App.Actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		act := s.rt.App.Actions[id]
		steps := make([]string, 0, len(act.Steps))
		for _, st := range act.Steps {
			steps = append(steps, fmt.Sprintf("%s %s = %s", st.Type, st.Path, st.Value))
		}
		out = append(out, map[string]any{"id": id, "steps": steps})
	}
	return out
}

func (s *Server) simulate(action string, args map[string]any) map[string]any {
	if _, ok := s.rt.App.Actions[action]; !ok {
		return map[string]any{"error": fmt.Sprintf("unknown action %q", action)}
	}
	sim := s.rt.Clone() // side-effect-free: mutate the copy only
	before := jsonClone(s.rt.State)
	sim.Dispatch(action, args)
	after := sim.State
	return map[string]any{
		"action":  action,
		"args":    args,
		"before":  before,
		"after":   after,
		"changed": jsonPretty(before) != jsonPretty(after),
	}
}

func nodeInfo(n *model.Node) map[string]any {
	kids := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		kids = append(kids, c.ID)
	}
	info := map[string]any{"id": n.ID, "type": n.Type, "children": kids}
	if n.Text != "" {
		info["text"] = n.Text
	}
	if n.Label != "" {
		info["label"] = n.Label
	}
	if n.OnPress != nil {
		info["onPress"] = map[string]any{"action": n.OnPress.Name, "args": n.OnPress.Args}
	}
	if n.Style != nil {
		info["style"] = n.Style
	}
	return info
}

func findNode(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if got := findNode(c, id); got != nil {
			return got
		}
	}
	if n.Template != nil {
		if got := findNode(n.Template, id); got != nil {
			return got
		}
	}
	// both branches of a `when` node are reachable, whichever is live
	for _, b := range []*model.Node{n.Then, n.Else} {
		if got := findNode(b, id); got != nil {
			return got
		}
	}
	return nil
}

func jsonPretty(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

func jsonClone(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
}
