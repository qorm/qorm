package mcp

import (
	"sort"
	"strings"
)

// ToolsMarkdown renders the live MCP tool set as docs/agent/mcp-tools.md, so the
// agent-facing reference is generated from the server and can never drift from
// it. Kept in sync by TestMCPDocInSync.
func ToolsMarkdown() string {
	var b strings.Builder
	b.WriteString("# QORM MCP tools\n\n")
	b.WriteString("> Generated from `internal/mcp/tools.go` (`TestMCPDocInSync`) — do not edit by hand.\n> Regenerate with `QORM_UPDATE_DOCS=1 go test ./internal/mcp/`.\n\n")
	b.WriteString("QORM exposes a [Model Context Protocol](https://modelcontextprotocol.io) server so an AI agent can **read, operate, design, and verify** a live QORM app. Start it with `qorm mcp <app-dir|bundle>` (stdio JSON-RPC), or reach the same tools over HTTP at `/mcp` on a running `qorm run` — the agent and the browser then share one live runtime.\n\n")
	b.WriteString("**Safety model.** `qorm_simulate_action`, `qorm_preview_patch` and `qorm_diff` run against a copy and never touch the live app. `qorm_apply_patch` commits a change, but it must carry the `previewToken` returned by a matching `qorm_preview_patch` of the same ops — so every committed edit is bound to a prior review. `qorm_undo` reverts the last apply.\n\n")
	b.WriteString("| Tool | Parameters | What it does |\n|---|---|---|\n")
	for _, t := range toolList() {
		b.WriteString("| `" + t.Name + "` | " + toolParams(t.InputSchema) + " | " + escapeCell(t.Description) + " |\n")
	}
	b.WriteString("\nParameters marked `*` are required; the rest are optional.\n")
	return b.String()
}

// toolParams summarises an input schema as a compact parameter list.
func toolParams(schema map[string]any) string {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return "—"
	}
	required := map[string]bool{}
	switch r := schema["required"].(type) {
	case []string:
		for _, k := range r {
			required[k] = true
		}
	case []any:
		for _, k := range r {
			if s, ok := k.(string); ok {
				required[s] = true
			}
		}
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		label := "`" + k + "`"
		if required[k] {
			label += "*"
		}
		if pm, ok := props[k].(map[string]any); ok {
			if en, ok := pm["enum"].([]string); ok {
				label += " (" + strings.Join(en, "\\|") + ")"
			} else if ts, ok := pm["type"].(string); ok {
				label += " (" + ts + ")"
			}
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

// escapeCell makes a description safe inside a Markdown table cell.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}
