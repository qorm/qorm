# Add QORM to your AI agent

QORM ships an **MCP server** (`qorm mcp <app-dir|bundle>`, stdio JSON-RPC) that
lets an agent read, edit, run, and verify a live QORM app, plus a **skill**
([`skill/SKILL.md`](skill/SKILL.md)) that teaches an agent the runnable format.

Prerequisite: `qorm` on PATH — `go install github.com/qorm/qorm/cmd/qorm@latest`
(or use the container: `ghcr.io/qorm/qorm`).

## MCP tools the server exposes

`qorm_inspect` · `qorm_get_node` · `qorm_query` · `qorm_list_actions` ·
`qorm_render_html` · `qorm_measure` · `qorm_check_layout` · `qorm_dispatch` ·
`qorm_set_state` · `qorm_simulate_action` · `qorm_assert` · `qorm_preview_patch` ·
`qorm_apply_patch` · `qorm_undo` · `qorm_diff` · `qorm_capabilities` ·
`qorm_export_scene` · `qorm_export_bundle` · `qorm_window`

`apply_patch` requires the `previewToken` from a prior `preview_patch` of the same
ops, so every committed change is bound to a review.

## Register it

### Claude Code
```sh
claude mcp add qorm -- qorm mcp .
```
Or drop [`mcp.json`](mcp.json) into your project as `.mcp.json`.

### Claude Desktop
Add the block from [`mcp.json`](mcp.json) to
`~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) /
`%APPDATA%\Claude\claude_desktop_config.json` (Windows), then restart Claude.

### Cursor
Copy [`mcp.json`](mcp.json) to `.cursor/mcp.json` (project) or `~/.cursor/mcp.json`
(global).

### Windsurf
Merge the `mcpServers` block into `~/.codeium/windsurf/mcp_config.json`.

### Any other MCP client
Run `qorm mcp <app-dir>` and speak MCP (JSON-RPC 2.0) over stdio. A live
`qorm run` also serves the same tools over HTTP at `/mcp`.

## The skill

Point your agent at [`skill/SKILL.md`](skill/SKILL.md) (or the repo's
[`llms.txt`](../llms.txt)) so it writes the format the runtime actually accepts
and self-verifies its edits with `qorm_measure` / `qorm_check_layout`.
