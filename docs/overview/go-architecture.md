# QORM Architecture (Go)

Current, accurate reference for the QORM runtime as implemented in Go. QORM is a
pure-Go, agent-native declarative-UI runtime: a small language-neutral JSON
format for UI, plus everything needed to run, ship, and collaboratively design
apps built with it.

## Pipeline

```
QORM app (JSON: manifest + scenes + actions + locales)
        │  loader        parse dir/bundle -> model.App
        ▼
   model.App            Node tree · Actions · GlobalState · Locales
        │  runtime       state store + expr eval + i18n + action dispatch
        ▼
   render target        one of two (see below)
```

The loader, runtime, state, actions, expression language and i18n are shared by
**all** render targets — only the final draw differs.

## Two render paths

| build | render | draws widgets | cross-compile |
|---|---|---|---|
| default | HTML/CSS → system browser (`--app` = chromeless window) | web engine | [done] pure Go, every platform |
| `-tags desktop` | HTML/CSS → native WebView (WKWebView/WebView2/WebKitGTK) | web engine | per-platform (cgo) |

Both paths render HTML/CSS in a web engine, so both keep the full agent
collaboration stack (shared live session over SSE + MCP). The default path is
pure Go (no cgo) and cross-compiles to macOS/Linux/Windows × amd64/arm64 from one
machine (`scripts/build-all.sh`); the WebView window is opt-in, built per-platform
(`build-desktop.sh`).

## Packages (`internal/`)

| package | role |
|---|---|
| `model` | App / Node / Action / GlobalState data model |
| `loader` | load a dir or bundle (skips `type:test`); serialize back (`NodeToJSON`, `AppToDocs`); locales |
| `expr` | expression language: literals, member access, `?:`, `&&/||`, comparisons, arithmetic, **function calls** (`len`, `matches`, `min`, `trim`, …) |
| `runtime` | state, `{{…}}` binding, 10 action steps, **i18n** (ICU-lite) |
| `render` | web widget set → HTML/CSS; conditional `if`, `onChange`, a11y, virtualization |
| `server` | live HTTP server: `/event`, **SSE `/events`**, `/poll`, `/mcp`, OTA `/update`+`/rollback` |
| `mcp` | Model Context Protocol server (stdio + HTTP) — the agent surface |
| `bundle` | compile + sha256 hash + ed25519 sign/verify + revocation |
| `keys` | ed25519 keypair generation/storage |
| `ota` | fetch (http/file) + verify-before-activate |
| `updates` | OTA publish server with staged (canary) rollout |
| `mdsite` | pure-Go markdown → static HTML docs site |

## Widgets (web, ~43)

- **Layout**: row, column, stack, absolute, scroll, grid, card, spacer, divider
- **Typography**: text, link, icon, badge, tag
- **Forms**: field, input (+date/number/color/file), textarea, select, checkbox,
  switch, radio, slider, segmented
- **Data display**: list (virtualized), table (sortable), tree, descriptions,
  stat, avatar, chart, rating
- **Feedback**: progress, spinner, skeleton, alert, empty
- **Overlay**: modal, drawer, tooltip
- **Navigation**: tabs, accordion, menu, breadcrumb, steps, pagination
- **Media**: image, video, carousel, timeline

Every node also supports conditional rendering (`"if": "{{…}}"`), accessibility
(`role`/`ariaLabel`/`tooltip`; the page emits `<html lang/dir>`), and a uniform
style set (background/gradient, full typography, border+radius, shadow, padding/
margin, width/height/min/max/aspectRatio, flex align/justify/gap, opacity,
transition).

## Actions (10 declarative steps)

`state.set` · `state.append` · `state.appendObject` · `state.toggle` ·
`state.increment` · `state.remove` · `state.updateWhere` · `state.merge` ·
`state.sort` (bindable field) · `state.clear`

Triggered by `onPress`/`onChange`; args are `{{…}}` expressions. Buttons/inputs
dispatch to actions; inputs two-way-bind to `state.*`.

## i18n

`locales/<lang>.json` catalogs + manifest `defaultLocale`; `{{ t.key }}` resolves
against the active locale (`state.locale`) with default fallback. ICU-lite
MessageFormat: params `{name}`, plural with **full CLDR rules** (Slavic
one/few/many, Arabic zero/two/few, …), select, number/currency/date formatting,
and RTL (`direction:rtl` + `<html dir>`). Locales travel in the signed bundle.

## Agent surface (MCP, 15 tools)

Shares one live runtime with the browser (SSE keeps every client in sync).

- **understand**: `qorm_inspect`, `qorm_render_html`, `qorm_get_node`, `qorm_query`, `qorm_list_actions`, `qorm_export_scene`, `qorm_export_bundle`
- **operate**: `qorm_dispatch`, `qorm_set_state`
- **test**: `qorm_assert`
- **design**: `qorm_diff`, `qorm_preview_patch` → `qorm_apply_patch` (atomic, bound to a preview) → `qorm_undo`
- **reason**: `qorm_simulate_action` (side-effect-free)

## Trust / delivery

sign (ed25519) → verify (integrity + signature + **revocation**) → OTA update
(hot-swap, rollback on failure) → **publish server** with canary rollout. The
agent never holds the signing key: it `export_bundle`s unsigned; a human `qorm
sign`s.

## CLI

`new` · `run` · `render` · `build` · `sign` · `keygen` · `verify` · `mcp` ·
`docs` · `updates`

## Quality

`go test ./...` (10 packages) + `-race` clean; fuzz harnesses on the three
parsers (expr/markdown/message); determinism conformance across all examples;
6-platform pure-Go cross-compile. See `README.md` for build/run recipes.
