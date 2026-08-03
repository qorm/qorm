# AGENTS.md — orienting an AI agent in the QORM repo

**What this is.** QORM (Query · Observe · Render · Mutate) is a pure-Go,
agent-native cross-platform app platform. A QORM app is JSON — a manifest
(`qorm.json`) + `scenes/*.json` + `actions/*.json`, plus an optional Go
middle-layer — that one runtime renders to HTML/CSS, ed25519-signs into a
verifiable bundle, delivers over-the-air, packages for web / iOS / Android /
desktop, and exposes to agents over MCP. Dual-consumer by design: every artifact
is meant to be read, written, and *verified* by both a person and an AI. The
acronym is the API surface: **Query** (HTTP/MCP reads),
**Observe** (SSE), **Render** (the runtime), **Mutate** (actions + writes).

**Full machine-readable map:** [llms.txt](llms.txt).

## Understand it
- [README.md](README.md) — what QORM is + the CLI at a glance.
- [`examples/`](examples) — the canonical runnable apps. **Trust these over any spec.**

## Write / edit an app
- **One-Command Scaffold & Run**: Run `qorm run <app-dir>` (or `go run ./cmd/qorm run <app-dir>`) — if `<app-dir>` doesn't exist, it automatically scaffolds the starter app and launches the native standalone executable application window for the platform.
- Use the format the runtime accepts **today**: text via `text`, bind with
  `{{state.x}}`, `onPress` names an action in `actions/`, components in
  `qorm.json` use `{{prop.x}}`. See
  [getting-started](docs/tutorials/getting-started.md) and the
  [widget catalog](api/widgets.md) (auto-generated from the code, canonical).
- Do **not** use the old `value` / `on:{press}` / `{{count}}` / `scene://` forms —
  the runtime ignores them. When docs and a runnable example disagree, the example wins.
- App logic too big for steps goes in a **script action**: an action JSON with a
  `script` field holding qscript source (`internal/qscript` — `let`/`if`/`for in`/
  `while`/`fn`, the expression language's operators and builtins, `state` read/write,
  `args` in), or — the file-per-action spelling — an `actions/<id>.qs` file whose
  filename is the action id and whose full text is the script. The loader compiles
  it (parse errors name the line, and the file for `.qs`); `script` beats `steps`
  when both are declared. Canonical example: [examples/tetris](examples/tetris).
- Shared styles live in a **stylesheet**: `styles/<id>.qss` (QORM Style Sheet,
  `internal/qss`) — rules like `button { borderRadius: 12 }`, `.accent { … }`,
  `#submit { … }` (type / class / id selectors; `#` comments; numbers stay
  numbers, strings and `{{bindings}}` evaluate like inline style values; nested
  objects such as `margin: {top: …}` stay inline on the node). Scene nodes
  reference them with a `class` prop (space-separated, later classes win).
  Cascade: theme component default < type rule < class rule (declaration
  order) < id rule < inline `style`. Parse errors are load-time diagnostics
  naming file and line; unknown style keys warn. The canvas renderer applies
  them; the HTML renderer does not yet (inline styles only there). Example:
  [examples/tetris](examples/tetris) `styles/app.qss`.
- **No emoji** in UI, code, or docs — use the built-in SVG icon set (icon *names*
  like `heart` / `star` / `zap`, listed in `internal/render/icons.go`).
- Style against the theme variables (`var(--accent)`, `var(--label)`, …) so apps
  follow the OS light/dark setting (default theme is `auto`). Manifest
  `designTokens` render as stage-scoped CSS vars too — `color.primary` →
  `var(--qorm-token-color-primary)`. Unknown `style` keys are load-time
  warnings (the renderer ignores them).

## Drive a live app as an agent
- [`integrations/`](integrations) — drop-in MCP config + per-agent setup + a QORM **skill**.
- `qorm mcp <app-dir>` exposes the app over MCP (stdio JSON-RPC); a running
  `qorm run` serves the same tools at `/mcp`.
- Read with `qorm_inspect` / `qorm_get_node` / `qorm_query`; operate with
  `qorm_dispatch` / `qorm_set_state`; change design with `qorm_preview_patch` →
  `qorm_apply_patch` (apply must carry the preview's token). See
  [docs/agent/mcp-tools.md](docs/agent/mcp-tools.md).
- **CRITICAL: During development, editing, and live debugging, agents MUST always keep the human observation window (logwindow / DevTool) open and active as a standalone executable application window for the current platform (serving on http://127.0.0.1:10383/logwindow or /console) so humans can inspect updates in real time.**
- **Self-verify** every edit against the rendered reality with `qorm measure` /
  `qorm check` (or `qorm_measure` / `qorm_check_layout`) — see
  [docs/verification.md](docs/verification.md).

## Build & test
- Pure Go, no cgo in the default build: `go build ./... && go test ./...`.
- CJK text: the default build embeds the OFL-1.1 subset font (Source Han
  Sans SC == Noto Sans CJK SC; `fonts/assets/`, ~3.4MB) and renders real CJK
  glyphs via `golang.org/x/image/font/sfnt`. `-tags qorm_nocjk` opts out
  (bitmap font; non-ASCII renders as `?`; binary ~3.8MB smaller). Regenerate
  the subset with `scripts/subset-font.sh <NotoSansSC/SourceHanSansSC OTF>`
  (needs python3 + fonttools). Without the asset the build still compiles
  and falls back to the bitmap font.
- Run an example: `go run ./cmd/qorm run examples/counter`.
- Native desktop window (opt-in, per-platform): `-tags desktop`.
- Canvas window + real WKWebView overlays for `webview` widgets (macOS,
  cgo): `-tags canvaswebview`; every other build draws the widget's
  placeholder (HTML renderer uses an `<iframe>`). Demo: `go run -tags
  canvaswebview ./cmd/qorm run examples/webdemo`.
