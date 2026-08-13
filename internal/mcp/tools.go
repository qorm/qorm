package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/a11y"
	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/expr"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/measure"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/sourcemap"
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
			Description: "Inspect the QORM app: id, name, entry scene, scene ids, state schema, current state, action ids, static compiler diagnostics, the resolved host window config (window: width/height/title/resizable/chromeless/transparent — set via qorm.config.json or qorm.json display/platforms.desktop.window), and the design-token system (designTokens: name -> {type,value,enforce}) when declared. Enforced color tokens hard-constrain apply_patch: a color style may only be set to one of their values. Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_render_html",
			Description: "Render the current app to HTML so the agent can see what the UI looks like — the scene the session is actually on, after its route guard has been resolved (a guarded scene the session may not enter is never rendered). Read-only.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_capture_subtree",
			Description: "Capture a specific node subtree by node id: returns isolated rendered HTML and child layout hierarchy for visual AI feedback. Read-only.",
			InputSchema: obj(map[string]any{"id": strProp}, "id"),
		},
		{
			Name:        "qorm_capture_canvas",
			Description: "Capture the actual last-presented native Canvas pixel plane as a base64 PNG. Optional id still returns the full surface plus a physical-pixel clip rectangle for that node; it does not pretend to isolate or re-render the subtree. Fails loudly outside a running native Canvas host, before its first frame, for absent/invisible nodes, or when safety limits are exceeded. Read-only.",
			InputSchema: obj(map[string]any{"id": strProp}),
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
			Name:        "qorm_source_location",
			Description: "Reverse-lookup: given a node id (e.g. one a human clicked in the devtool or you found via qorm_query), return where it is declared in the app's source — file (relative to the app dir), 1-based line, and that line's text. Lets you jump straight to the JSON to edit it. Unavailable for a signed bundle (no source tree) or a templated id. Read-only.",
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
			Description: "Read the shared session's live presence: returns {events:[who (human/agent) did what, oldest to newest], humanFocus:{element, secondsAgo}, humanTyping:{entry, secondsAgo}, humanFilled:{field, secondsAgo}, inflight:N} — so the agent sees what the human just did, the element they are on now, the text they last typed, AND which hidden (password) fields they filled (label only; a password value is never captured), and collaborates in context. Browser/WebView events and native canvas pointer/keyboard actions enter the same human activity stream and DevTool; both hosts report privacy-safe focus/typing/filled presence, and neither captures password values. `inflight` counts the background work the app still has open (async `http.*` requests plus waiting `delay` steps): 0 means the app has settled and what you read now is final, above 0 means a reply is still coming and the current frame is a loading state — read again before drawing conclusions. Only available in a running `qorm run` session. Read-only.",
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
			Description: "OPERATE the live app: set a state path to a value and return the new state and rendered HTML. A dotted path NESTS, exactly like the state.set action step: path 'user.name' writes name inside user, so a binding {{ state.user.name }} reads it back. Computed (derived) values are read-only — a path inside the computed namespace is rejected, because they are republished from their declarations at every frame.",
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
			Description: "INTERPRET the LIVE render precisely: returns every component joining what the user expressed (type, text, state binding) with how it actually rendered — x,y,w,h, visible, and computed color/background/fontSize/fontWeight/padding/borderRadius/border/opacity/zIndex/position/x-overflow. The active rendering host supplies the measurement: the native canvas window exports its retained render graph, while a browser/WebView reports its DOM. Requires a running app with a rendering window/client; `qorm mcp` over stdio alone has no render host. For a deterministic one-shot canvas measurement without a window, use CLI `qorm measure`. Use this tool to see exactly what the human's live host rendered.",
			InputSchema: obj(nil),
		},
		{
			Name:        "qorm_check_layout",
			Description: "VERIFY the LIVE render against expectations; returns per-check pass/fail with actual values. `checks` is an array of {id, <assertions>}. Assertions: visible(bool) | type(widget-type string) | text(substring the component must contain, matched vs expressed OR rendered text) | noOverflow(bool, no horizontal overflow) | minW|maxW|minH|maxH(px number) | x|y(px number, ±3 tolerance) | within(id: this box must sit inside that id's box) | below(id: must start below that id) | backgroundNot|colorNot(substring that must be ABSENT — e.g. \"255, 255, 255\" to assert not-white in dark mode) | role(the rendered ARIA role string) | hasAriaLabel(bool) | contrastRatio(min WCAG ratio, e.g. 4.5 for AA normal text). Browser/WebView DOM reports include renderer-injected roles and computed contrast; the canvas graph currently reports author-supplied role/ariaLabel and makes contrastRatio fail as unavailable. Example: [{\"id\":\"wifi\",\"type\":\"switchlisttile\",\"visible\":true,\"within\":\"settings\"},{\"id\":\"chart\",\"noOverflow\":true}]. Fail-loud: an unrecognised assertion key (e.g. a typo) fails, and a within/below target id that was not measured fails as 'not found' — nothing silently passes. Requires a running app with a rendering window/client: native canvas checks use its retained render graph; browser/WebView checks use its DOM report. `qorm mcp` over stdio alone has no render host. Optional viewportW/viewportH (px) set the runtime viewport before evaluating, so responsive `when` branches resolve as if the window were that size — note the measured rects still come from the host's REAL window (a live browser client also overwrites the viewport on its next load/resize).",
			InputSchema: obj(map[string]any{"checks": map[string]any{"type": "array"}, "viewportW": intProp, "viewportH": intProp}, "checks"),
		},
		{
			Name:        "qorm_validate",
			Description: "VALIDATE a QORM scene node or whole app against component schemas, widget catalog type rules, expression syntax, and design token constraints before patching or saving. Returns valid (bool) and an array of diagnostic warnings or errors.",
			InputSchema: obj(map[string]any{
				"node":    map[string]any{"type": "object"},
				"sceneId": strProp,
			}),
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
	s.bumped = false
	result, err := s.callTool(p.Name, p.Arguments)
	if err != nil {
		return ok(req.ID, toolText(true, err.Error()))
	}
	if s.afterMutate != nil && isMutating(p.Name) && !s.bumped {
		s.afterMutate() // must not take s.mu (server bumps rev lock-free)
	}
	return ok(req.ID, toolText(false, result))
}

// withInflight folds the runtime's open-background-work count into the host's
// activity payload, so one read answers both "what just happened" and "is the
// app still working on it".
//
// It matters because an agent's mental model of a QORM session is
// dispatch-then-read, and an async `http.*` step breaks that: the reply lands
// one or more revisions LATER, so an agent that inspects immediately sees a
// loading frame and can conclude the app is broken, or worse act on the stale
// state. `inflight: 0` is the quiescence signal — nothing is outstanding, the
// state has settled and what it reads now is final; anything above zero means
// "read again in a moment".
//
// The count is read under the same lock as every other tool call, so it is a
// consistent snapshot alongside the events. A payload that is not a JSON object
// is passed through untouched rather than mangled: an unparseable activity log
// is the host's business, and losing it to add a counter would be a bad trade.
func withInflight(payload string, n int) string {
	var m map[string]any
	if json.Unmarshal([]byte(payload), &m) != nil || m == nil {
		return payload
	}
	m["inflight"] = n
	out, err := json.Marshal(m)
	if err != nil {
		return payload
	}
	return string(out)
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

// decodeArgs decodes tool arguments, treating absent/null arguments as
// all-defaults but present-and-malformed arguments as a hard error. Without
// this, a bad payload silently decayed into zero values — most dangerously on
// mutating tools (a malformed qorm_window call became "move to 0,0", a
// malformed qorm_assert became a trivially passing empty check list). The
// error is returned to the MCP client as a tool error instead.
func decodeArgs(name string, args json.RawMessage, v any) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		return fmt.Errorf("%s: invalid arguments: %v", name, err)
	}
	return nil
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
		// Mutating tool: malformed args must never decay into a zero-value
		// action (an unparseable call used to become "move main to 0,0").
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
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
		return s.currentHTML(), nil
	case "qorm_capture_subtree":
		var a struct{ ID string }
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		if a.ID == "" {
			return "", fmt.Errorf("qorm_capture_subtree: missing id parameter")
		}
		res := render.RenderSubtree(s.rt, a.ID)
		return jsonPretty(map[string]any{
			"id":       a.ID,
			"html":     res.HTML,
			"unknowns": res.Unknown,
		}), nil
	case "qorm_capture_canvas":
		var a struct {
			ID string `json:"id"`
		}
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		if s.canvasCapture == nil {
			return "", fmt.Errorf("canvas pixel capture unavailable (requires a running native Canvas host with a presented frame)")
		}
		captured, err := s.canvasCapture(a.ID)
		if err != nil {
			return "", fmt.Errorf("canvas pixel capture: %w", err)
		}
		return canvasCaptureJSON(captured, a.ID)
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
		return withInflight(s.activityProv(), s.rt.Inflight()), nil
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
		// Read-only, but a malformed selector used to decay into the empty
		// selector — which matches every node — so the caller got a confident,
		// wrong answer. Surface the parse error instead.
		var sel selector
		if err := decodeArgs(name, args, &sel); err != nil {
			return "", err
		}
		matches := queryNodes(s.rt.App.EntryRoot(), sel)
		return jsonPretty(map[string]any{"count": len(matches), "matches": matches}), nil
	case "qorm_get_node":
		var a struct {
			ID string `json:"id"`
		}
		// Read-only; strict decode so a malformed call reports the real
		// problem instead of a misleading `node "" not found`.
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		node := findNode(s.rt.App.EntryRoot(), a.ID)
		if node == nil {
			return "", fmt.Errorf("node %q not found", a.ID)
		}
		return jsonPretty(nodeInfo(node)), nil
	case "qorm_source_location":
		var a struct {
			ID string `json:"id"`
		}
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		if a.ID == "" {
			return "", fmt.Errorf("id is required")
		}
		loc, ok := sourcemap.Locate(s.rt.App.BaseDir, a.ID)
		if !ok {
			if s.rt.App.BaseDir == "" {
				return "", fmt.Errorf("no source tree (app is a bundle, not a directory)")
			}
			return "", fmt.Errorf("id %q is not declared literally in the source (templated or unknown)", a.ID)
		}
		return jsonPretty(loc), nil
	case "qorm_simulate_action":
		var a struct {
			Action string         `json:"action"`
			Args   map[string]any `json:"args"`
		}
		// Side-effect-free, but a malformed call used to decay into
		// `unknown action ""` — surface the actual parse error.
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		return jsonPretty(s.simulate(a.Action, a.Args)), nil
	case "qorm_dispatch":
		var a struct {
			Action string         `json:"action"`
			Args   map[string]any `json:"args"`
		}
		// Mutating tool: never dispatch on silently-zeroed args.
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		if _, ok := s.rt.App.Actions[a.Action]; !ok {
			return "", fmt.Errorf("unknown action %q", a.Action)
		}
		s.dispatchSettled(a.Action, a.Args)
		return jsonPretty(s.stateAndHTML()), nil
	case "qorm_set_state":
		var a struct {
			Path  string `json:"path"`
			Value any    `json:"value"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Path == "" {
			return "", fmt.Errorf("set_state requires path and value")
		}
		// SetStatePath is the same write the `state.set` step makes: dotted
		// paths NEST (the raw assignment this replaced built a literal "a.b"
		// key no binding could ever read), and the read-only computed namespace
		// is refused. The loader refuses such a write in JSON and the runtime
		// refuses it in a step; refusing it here closes the third door, through
		// which an agent could publish a value that looked derived and was not.
		if !s.rt.SetStatePath(a.Path, a.Value) {
			return "", fmt.Errorf("%q is a computed (derived) value: it is republished from its declaration at every frame, so it cannot be set — write the state it reads instead", a.Path)
		}
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
		// Writes s.rt.Viewport, so malformed args must not slip through as
		// zero values (and a garbled checks payload must not "pass").
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
		if a.ViewportW > 0 || a.ViewportH > 0 {
			// Simulate a viewport so responsive `when` branches resolve for the
			// check. The live browser client re-reports its real size on its next
			// load/resize, overwriting this.
			vp := qrt.Viewport{W: a.ViewportW, H: a.ViewportH}
			if s.rt.Viewport != vp {
				s.rt.Viewport = vp
				// A viewport write changes what renders, so it is a MUTATION
				// and must publish a fresh revision like every other mutation
				// path (serveViewport does the same). Writing it bare would let
				// the next same-revision re-render (serveIndex, /poll, an SSE
				// catch-up) file a different handler table for a rev clients
				// already hold (P0-4).
				if s.afterMutate != nil {
					s.afterMutate()
				}
			}
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
		// A TEST tool must fail loud: malformed args used to decay into an
		// empty check list, which reported overall pass:true.
		if err := decodeArgs(name, args, &a); err != nil {
			return "", err
		}
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
	case "qorm_validate":
		var a struct {
			Node    map[string]any `json:"node"`
			SceneID string         `json:"sceneId"`
		}
		json.Unmarshal(args, &a)
		return jsonPretty(s.validate(a.Node, a.SceneID)), nil
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// stateAndHTML is the standard result of an operate/design mutation.
// dispatchSettled runs an action and guarantees the caller observes the state
// it SETTLES on, never a loading frame. An `http.*` step marked async would
// otherwise hand its round trip to the host's background sink and return at
// once, so qorm_dispatch would answer the agent with the half-finished state
// the request was launched from — and the agent would reason (and act) on it.
//
// The fix is the runtime's own fallback rather than a wait: detaching the
// background sink for the duration of the call makes every async step take its
// synchronous path, which is by definition the settled result. That keeps the
// tool call atomic — the alternative, dropping the lock to poll Inflight(),
// would open a window in which a human event or an OTA activate could land (or
// swap the runtime out) halfway through a tool call that is currently
// indivisible. Safe because the whole call holds the host lock, so no other
// dispatch can observe the sink while it is detached.
func (s *Server) dispatchSettled(action string, args map[string]any) {
	s.settled(func() { s.rt.Dispatch(action, args) })
}

// settled runs fn with the host's background sink detached, so every async step
// it reaches takes its synchronous path and the caller observes the state the
// work SETTLES on. See dispatchSettled for why that is a detach rather than a
// wait. Safe because the whole tool call holds the host lock.
func (s *Server) settled(fn func()) {
	background := s.rt.Async
	s.rt.Async = nil
	defer func() { s.rt.Async = background }()
	fn()
}

// currentHTML renders what this session may actually see — and resolves the
// route guards BEFORE it renders, which is the whole point of the method.
//
// Every MCP render point used to run ahead of the guard: a tool mutated state
// (a dispatch whose `navigate` step entered a protected scene) and rendered
// immediately, while the guard only ran later, in the host's afterMutate. One
// qorm_dispatch therefore handed the agent the full HTML of a scene the guard
// was about to refuse — no forged token, no race. RunPendingEnter is the same
// choke point the server drains in bump(), so an MCP result now shows exactly
// the scene a browser attached to the same session is shown, never the one
// behind the guard.
//
// It is drained with the background sink detached, for dispatchSettled's
// reason: an onEnter action that fires an async request must land inside this
// call, not one revision later.
//
// A runtime the guards blocked outright renders as an EMPTY frame: there is, by
// construction, no scene it may show. Falling back to the entry scene here —
// what a plain render.Render does with an id it does not know — would render
// the very scene the guard refused whenever the refusal happened AT the entry.
//
// Draining a pending hook is a MUTATION (the hook's action writes state, and a
// `render` step in it publishes intermediate frames), so the settled state must
// ship as a NEW revision: without the bump, the next render at the current
// revision — a browser's first load arriving after the agent's read, /poll, an
// SSE catch-up — would file a different frame under it (the "one revision, one
// frame" violation, with dead buttons on the page). Read-only tools therefore
// bump too, but ONLY when a hook actually drained: once per scene entry, never
// per read, so agents polling qorm_render_html cause no rev churn.
func (s *Server) currentHTML() string {
	drained := s.rt.PendingEnter()
	s.settled(s.rt.RunPendingEnter)
	if drained && s.afterMutate != nil {
		s.afterMutate()
		// Remember the bump: a MUTATING tool reaching this via stateAndHTML is
		// bumped again by handleToolCall, and one call must publish one frame.
		s.bumped = true
	}
	scene := s.rt.CurrentScene()
	if scene == qrt.GuardBlocked {
		return `<div data-scene="blocked" style="padding:24px;color:#888">no scene available: a route guard refused every entry</div>`
	}
	return render.RenderScene(s.rt, scene).HTML
}

func (s *Server) stateAndHTML() map[string]any {
	html := s.currentHTML() // guard-resolved first: never the refused scene
	return map[string]any{
		"state": s.rt.State,
		"html":  html,
	}
}

// assert evaluates test checks against live state and rendered HTML.
func (s *Server) assert(checks []map[string]any) map[string]any {
	htmlOut := s.currentHTML() // guard-resolved: a test never reads a refused scene
	results := make([]map[string]any, 0, len(checks))
	allPass := true
	for _, c := range checks {
		kind, _ := c["kind"].(string)
		pass := false
		detail := ""
		switch kind {
		case "stateEquals":
			path, _ := c["path"].(string)
			got := s.rt.StatePath(path) // dotted paths nest, as everywhere else
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
		res := map[string]any{"ok": false, "error": err.Error()}
		if strings.Contains(err.Error(), "design token violation") {
			_, display := enforcedColorTokens(s.rt.App)
			res["validTokens"] = display
			res["suggestedFix"] = fmt.Sprintf("Use one of the allowed design tokens: %s", strings.Join(display, ", "))
		}
		return res
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
		if strings.Contains(err.Error(), "design token violation") {
			_, display := enforcedColorTokens(s.rt.App)
			return map[string]any{
				"ok":           false,
				"error":        err.Error(),
				"validTokens":  display,
				"suggestedFix": fmt.Sprintf("Use one of the allowed design tokens: %s", strings.Join(display, ", ")),
			}, nil
		}
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
	win := s.rt.App.Window
	out := map[string]any{
		"id":           s.rt.App.ID,
		"name":         s.rt.App.Name,
		"entry":        s.rt.App.Entry,
		"scenes":       sceneIDs,
		"actions":      actionIDs,
		"stateSchema":  s.rt.App.GlobalState.Schema,
		"currentState": s.rt.State,
		"diagnostics":  s.rt.App.Diagnostics,
		// Resolved host-window config (qorm.config.json > platforms.desktop.window
		// > top-level display). width/height 0 = fluid. resizable reflects the
		// effective value (windows are resizable unless "resizable": false).
		"window": map[string]any{
			"width":       win.Width,
			"height":      win.Height,
			"title":       win.Title,
			"resizable":   !win.Fixed,
			"chromeless":  win.Chromeless,
			"transparent": win.Transparent,
		},
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

type diagItem struct {
	ID      string `json:"id,omitempty"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (s *Server) validate(rawNode map[string]any, sceneID string) map[string]any {
	var diags []diagItem
	if s.rt == nil || s.rt.App == nil {
		return map[string]any{"valid": false, "diagnostics": []diagItem{{Level: "error", Message: "runtime or app uninitialized"}}}
	}
	var rootNode *model.Node
	if rawNode != nil && len(rawNode) > 0 {
		rootNode = loader.BuildNode(rawNode)
		s.validateNodeTree(rootNode, &diags)
	} else {
		targetScenes := s.rt.App.Scenes
		if sceneID != "" {
			if sc, ok := s.rt.App.Scenes[sceneID]; ok {
				targetScenes = map[string]*model.Node{sceneID: sc}
			}
		}
		for _, sc := range targetScenes {
			s.validateNodeTree(sc, &diags)
		}
	}
	testApp := *s.rt.App
	if rootNode != nil {
		testApp.Scenes = map[string]*model.Node{"val": rootNode}
		testApp.Entry = "val"
	}
	res := render.Render(qrt.New(&testApp))
	for _, unk := range res.Unknown {
		diags = append(diags, diagItem{
			Level:   "error",
			Message: fmt.Sprintf("Unrecognised widget type %q", unk),
		})
	}
	valid := true
	for _, d := range diags {
		if d.Level == "error" {
			valid = false
			break
		}
	}
	return map[string]any{
		"valid":       valid,
		"diagnostics": diags,
	}
}

func (s *Server) validateNodeTree(n *model.Node, diags *[]diagItem) {
	if n == nil {
		return
	}
	for _, val := range []string{n.Text, n.Label, n.Value, n.Placeholder, n.Condition, n.Data} {
		if val != "" && strings.Contains(val, "{{") {
			if expr.CloseIndex(val) < 0 {
				*diags = append(*diags, diagItem{
					ID:      n.ID,
					Level:   "error",
					Message: fmt.Sprintf("Unclosed or invalid expression binding in %q", val),
				})
			}
		}
	}
	for _, c := range n.Children {
		s.validateNodeTree(c, diags)
	}
	s.validateNodeTree(n.Then, diags)
	s.validateNodeTree(n.Else, diags)
	s.validateNodeTree(n.Template, diags)
}
