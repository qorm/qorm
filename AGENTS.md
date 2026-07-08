# AGENTS.md — orienting an AI agent in the QORM repo

**What this is.** QORM (Queryable Object Rendering Model) is a pure-Go,
agent-native declarative-UI runtime. A QORM app is JSON — a manifest
(`qorm.json`) + `scenes/*.json` + `actions/*.json`, plus an optional Go
middle-layer — that one runtime renders to HTML/CSS, ed25519-signs into a
verifiable bundle, delivers over-the-air, packages for web / iOS / Android /
desktop, and exposes to agents over MCP. Dual-consumer by design: every artifact
is meant to be read, written, and *verified* by both a person and an AI. Read
the acronym as verbs and you have the API surface: **Query** (HTTP/MCP reads),
**Observe** (SSE), **Render** (the runtime), **Mutate** (actions + writes).

**Full machine-readable map:** [llms.txt](llms.txt).

## Understand it
- [README.md](README.md) — what QORM is + the CLI at a glance.
- [`examples/`](examples) — the canonical runnable apps. **Trust these over any spec.**

## Write / edit an app
- Use the format the runtime accepts **today**: text via `text`, bind with
  `{{state.x}}`, `onPress` names an action in `actions/`, components in
  `qorm.json` use `{{prop.x}}`. See
  [getting-started](docs/tutorials/getting-started.md) and the
  [widget catalog](docs/reference/widgets.md) (auto-generated from the code, canonical).
- Do **not** use the old `value` / `on:{press}` / `{{count}}` / `scene://` forms —
  the runtime ignores them. When docs and a runnable example disagree, the example wins.
- **No emoji** in UI, code, or docs — use the built-in SVG icon set (icon *names*
  like `heart` / `star` / `zap`, listed in `internal/render/icons.go`).

## Drive a live app as an agent
- [`integrations/`](integrations) — drop-in MCP config + per-agent setup + a QORM **skill**.
- `qorm mcp <app-dir>` exposes the app over MCP (stdio JSON-RPC); a running
  `qorm run` serves the same tools at `/mcp`.
- Read with `qorm_inspect` / `qorm_get_node` / `qorm_query`; operate with
  `qorm_dispatch` / `qorm_set_state`; change design with `qorm_preview_patch` →
  `qorm_apply_patch` (apply must carry the preview's token). See
  [docs/agent/mcp-tools.md](docs/agent/mcp-tools.md).
- **Self-verify** every edit against the rendered reality with `qorm measure` /
  `qorm check` (or `qorm_measure` / `qorm_check_layout`) — see
  [docs/verification.md](docs/verification.md).

## Build & test
- Pure Go, no cgo in the default build: `go build ./... && go test ./...`.
- Run an example: `go run ./cmd/qorm run examples/counter`.
- Native desktop window (opt-in, per-platform): `-tags desktop`.
