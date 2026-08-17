// Package policy adjudicates MCP tool access from an app's manifest agent policy
// and optional CLI overrides (--mcp-read-only).
package policy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/qorm/platform/internal/model"
)

const (
	LevelReadOnly     = "read-only"
	LevelPreviewOnly  = "preview-only"
	LevelOperate      = "operate"
	LevelDesign       = "design"
	LevelFull         = "full"
	LevelLegacyFull   = "full" // apps without agent.policy behave as full
	errPolicyDenied   = "agent policy denied"
	rpcCodePolicyDeny = -32001
)

// RPCCodePolicyDenied is the JSON-RPC error code for a policy rejection.
const RPCCodePolicyDenied = rpcCodePolicyDeny

// AllTools is the complete MCP tool surface (keep in sync with internal/mcp).
var AllTools = []string{
	"qorm_window",
	"qorm_inspect",
	"qorm_render_html",
	"qorm_a11y_tree",
	"qorm_capabilities",
	"qorm_get_node",
	"qorm_query",
	"qorm_list_actions",
	"qorm_activity",
	"qorm_export_scene",
	"qorm_export_bundle",
	"qorm_simulate_action",
	"qorm_dispatch",
	"qorm_set_state",
	"qorm_assert",
	"qorm_preview_patch",
	"qorm_diff",
	"qorm_apply_patch",
	"qorm_undo",
	"qorm_measure",
	"qorm_check_layout",
}

var levelTools = map[string]map[string]bool{
	LevelReadOnly: toolSet(
		"qorm_inspect", "qorm_render_html", "qorm_a11y_tree", "qorm_capabilities",
		"qorm_get_node", "qorm_query", "qorm_list_actions", "qorm_activity", "qorm_measure",
	),
	LevelPreviewOnly: toolSet(
		"qorm_inspect", "qorm_render_html", "qorm_a11y_tree", "qorm_capabilities",
		"qorm_get_node", "qorm_query", "qorm_list_actions", "qorm_activity", "qorm_measure",
		"qorm_simulate_action", "qorm_preview_patch", "qorm_diff", "qorm_assert", "qorm_check_layout",
	),
	LevelOperate: toolSet(
		"qorm_inspect", "qorm_render_html", "qorm_a11y_tree", "qorm_capabilities",
		"qorm_get_node", "qorm_query", "qorm_list_actions", "qorm_activity", "qorm_measure",
		"qorm_simulate_action", "qorm_preview_patch", "qorm_diff", "qorm_assert", "qorm_check_layout",
		"qorm_dispatch", "qorm_set_state",
	),
	LevelDesign: toolSet(
		"qorm_inspect", "qorm_render_html", "qorm_a11y_tree", "qorm_capabilities",
		"qorm_get_node", "qorm_query", "qorm_list_actions", "qorm_activity", "qorm_measure",
		"qorm_simulate_action", "qorm_preview_patch", "qorm_diff", "qorm_assert", "qorm_check_layout",
		"qorm_dispatch", "qorm_set_state",
		"qorm_apply_patch", "qorm_undo", "qorm_export_scene",
	),
	LevelFull: toolSet(AllTools...),
}

func toolSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// EffectiveLevel returns the policy level in effect. An empty manifest policy
// keeps legacy full access.
func EffectiveLevel(p model.AgentPolicy) string {
	if p.Level != "" {
		return normalizeLevel(p.Level)
	}
	if p.HasPolicy() {
		// Partial policy without level: safest explicit default.
		return LevelPreviewOnly
	}
	return LevelFull
}

func normalizeLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case LevelReadOnly, "readonly":
		return LevelReadOnly
	case LevelPreviewOnly, "previewonly":
		return LevelPreviewOnly
	case LevelOperate:
		return LevelOperate
	case LevelDesign:
		return LevelDesign
	case LevelFull:
		return LevelFull
	default:
		return level
	}
}

// ToolAllowed reports whether tool may run under the effective policy.
func ToolAllowed(p model.AgentPolicy, tool string) bool {
	return AuthorizeTool(p, tool, nil) == nil
}

// AuthorizeTool checks manifest policy for one MCP tool call. toolArgs may be
// nil except for qorm_window host-call checks (op=eval).
func AuthorizeTool(p model.AgentPolicy, tool string, toolArgs json.RawMessage) error {
	level := EffectiveLevel(p)
	base, ok := levelTools[level]
	if !ok {
		// Unknown level string from manifest: treat as full so a typo does not
		// brick the app, but the loader should have warned.
		base = levelTools[LevelFull]
	}
	allowed := base[tool]

	// Per-tool overrides from manifest.
	if v, ok := p.Tools[tool]; ok {
		allowed = v
	}
	if p.RequiresPreview[tool] {
		allowed = base[tool] || allowed // requiresPreview only matters once allowed at design+
	}

	if !allowed {
		return fmt.Errorf("%s: tool %q is not allowed (effective level %q)", errPolicyDenied, tool, level)
	}

	if tool == "qorm_window" && isHostEvalCall(toolArgs) {
		if err := authorizeHostCall(p, "eval"); err != nil {
			return err
		}
	}
	return nil
}

func isHostEvalCall(args json.RawMessage) bool {
	if len(args) == 0 {
		return false
	}
	var a struct {
		Op string `json:"op"`
		JS string `json:"js"`
	}
	_ = json.Unmarshal(args, &a)
	return a.Op == "eval" || a.JS != ""
}

func authorizeHostCall(p model.AgentPolicy, op string) error {
	if !p.HostCall.Allowed {
		if p.HasPolicy() {
			return fmt.Errorf("%s: host call %q is not allowed by agent.policy.hostCall", errPolicyDenied, op)
		}
		// No hostCall block: design/full levels may use window eval; read-only
		// levels never reach here because qorm_window is denied at those levels.
		return nil
	}
	if len(p.HostCall.Ops) == 0 {
		return nil
	}
	if slices.Contains(p.HostCall.Ops, op) {
		return nil
	}
	return fmt.Errorf("%s: host call %q is not in agent.policy.hostCall.ops", errPolicyDenied, op)
}

// EffectivePermissions returns a JSON-friendly summary for qorm_inspect.
func EffectivePermissions(p model.AgentPolicy, readOnlyCLI bool) map[string]any {
	level := EffectiveLevel(p)
	tools := map[string]bool{}
	for _, name := range AllTools {
		tools[name] = ToolAllowed(p, name)
	}
	out := map[string]any{
		"effectiveLevel": level,
		"tools":          tools,
	}
	if p.HasPolicy() {
		out["declared"] = map[string]any{
			"level":    p.Level,
			"hostCall": p.HostCall,
		}
		if len(p.Tools) > 0 {
			out["declared"].(map[string]any)["tools"] = p.Tools
		}
		if len(p.RequiresPreview) > 0 {
			out["declared"].(map[string]any)["requiresPreview"] = p.RequiresPreview
		}
	}
	if readOnlyCLI {
		out["cliReadOnly"] = true
	}
	return out
}

// ValidLevel reports whether level is a known preset name (after normalization).
func ValidLevel(level string) bool {
	n := normalizeLevel(level)
	for _, k := range KnownLevels() {
		if k == n {
			return true
		}
	}
	return false
}

// KnownLevels returns valid preset names for validation.
func KnownLevels() []string {
	return []string{LevelReadOnly, LevelPreviewOnly, LevelOperate, LevelDesign, LevelFull}
}
