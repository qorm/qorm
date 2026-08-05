---
name: qorm
description: Build, edit, and verify QORM apps — an agent-native declarative-UI runtime whose apps are language-neutral JSON (qorm.json + scenes/ + actions/). Use when creating or modifying a UI as QORM JSON, packaging it for web/iOS/Android/desktop, or driving a live QORM app over MCP.
---

# QORM skill

QORM (Query · Observe · Render · Mutate — exactly what you do to a live app)
runs a small JSON app: a manifest
(`qorm.json`), `scenes/*.json` (the UI node trees), and `actions/*.json` (declarative
behaviour). A pure-Go runtime renders it, signs it, and packages it everywhere.

## Write the runnable format (trust this, not the aspirational spec)

- Manifest: `{ "type":"app", "id":…, "entry":"main", "globalState":{ "schema":{…}, "initial":{…} } }`.
- Text: the `text` field (NOT `value`); bind with `{{ state.x }}` — e.g. `{ "type":"text", "text":"Count: {{ state.count }}" }`.
- Buttons: `"onPress":"increment"` (an action name; a string invokes it) — or `{ "name":…, "args":{…} }`.
- Actions (`actions/<id>.json`): `{ "type":"action","id":…,"steps":[ { "type":"state.set","path":"count","value":"{{ state.count + 1 }}" } ] }`. Step types: `state.set/increment/toggle/append/...` and `http.get`.
- Components: declared in `qorm.json` under `"components"`, referenced by a node whose `type` equals the component name; template uses `{{ prop.x }}` with a `{ "type":"slot" }` placeholder.
- Authoritative, code-generated references: the widget catalog (`api/widgets.md`) and capabilities (`docs/platforms/capabilities.md`). The JSON format spec is design-intent and diverges — prefer getting-started + `examples/`.

## Standard action patterns

Don't reinvent behaviour — reach for these load-clean shapes (full recipes in `docs/tutorials/first-action.md`, working apps in `examples/form` and `examples/tasks`):
- Loading state: `state.set loading=true` → `http.*` → `state.set loading=false`; bind `{{ state.loading }}`.
- Error handling: give `http.*` an `"error"` path; bind `{{ state.error }}` under an `if`. Success clears it.
- Optimistic update: mutate state, then `http.*`, then a rollback step whose `match` is `{{ len(state.error) > 0 ? id : "" }}` — a no-op on success, a revert on failure.
- Form validation: one conditional `state.set` per field writes `fieldErrors.<field>` (ternary → message or `""`); bind `{{ state.fieldErrors.<field> }}`.
- Pagination: keep a `page` counter, `state.increment` it, compute the offset in the URL binding.
- Debounce / cancel-token are **not** step types — debounce client-side via `onChange` throttling; cancellation is planned (last `result` write wins).

## Drive a live app over MCP

`qorm mcp <app-dir>` (or the `/mcp` endpoint of a running `qorm run`) exposes:
- Understand: `qorm_inspect`, `qorm_get_node`, `qorm_query`, `qorm_list_actions`, `qorm_render_html`, `qorm_activity` (what the human just did in the shared session).
- Operate: `qorm_dispatch` (run an action), `qorm_set_state`.
- Design (review-bound): `qorm_preview_patch` → `qorm_apply_patch` (must carry the preview's `previewToken`); `qorm_undo`, `qorm_diff`.
- Reason without side effects: `qorm_simulate_action`.

## Always self-verify

After an edit, prove it against the rendered reality rather than assuming:
- `qorm_measure` / `qorm_check_layout` (or the CLI `qorm measure` / `qorm check <app> --audit`) render the app and report real geometry + unknown-widget issues.
- `qorm_assert` checks state/text/node facts.

## Ship it

- Run: `qorm run <app>`. Static snapshot: `qorm render <app> -o out.html`.
- Sign + verify: `qorm build <app> -o app.bundle --key k`; `qorm verify app.bundle --trust k.pub`.
- Package: `qorm package <app> -p web|ios|android|mac`. A custom app icon or `--no-branding` is commercial white-labeling (see ops/TERMS.md) and prompts for a Patreon-membership confirmation.

## Keep QORM updated

Before starting work on a QORM app, check for and apply framework updates:
- Run `qorm update` (or `go run ./cmd/qorm update`) to pull the latest QORM binary.
- If working from a cloned repo, run `git pull` in the QORM directory to get the latest source, then rebuild with `go build ./cmd/qorm`.
- Run `qorm version` to confirm the current version. If the version is behind the latest release, update before proceeding.
- After updating, re-read `AGENTS.md` and this skill file — new features, widgets, or action types may have been added.

## Canvas engine (native desktop window)

On `-tags desktop`, the app opens as a standalone native window with a pure-Go
software renderer — no browser engine. The same JSON scene runs identically:

- **Layout**: identical to the HTML path (same measure/layout pipeline).
- **Interaction**: full keyboard navigation (Tab/Shift-Tab, Enter/Space, Escape,
  modifier keys), scroll viewports with momentum inertia, text editing with
  selection/clipboard/undo, animated pressed/hover transitions, disabled visual
  dimming.
- **Widgets**: all 80+ widgets work, including overlay panels (drawer, menu,
  modal, snackbar, tooltip) and interactive controls (switch, slider, checkbox,
  select, textarea, draggable).
- **Build**: `go run -tags desktop ./cmd/qorm run <app>`.

### Declarative interaction effects (any node)

These style keys work on ANY node — no per-widget logic needed:

| Key | Effect |
|---|---|
| `pressedScale` / `hoverScale` | Scale transform on press/hover |
| `pressedBackground` / `hoverBackground` | Color swap on press/hover |
| `pressedOpacity` / `hoverOpacity` | Opacity change on press/hover |
| `transition` | CSS-style duration (`"0.2s"`, `"200ms"`) — animates all the above |
| `disabled` | Blocks pointer, dims to 50% opacity, shows not-allowed cursor |

### Style system

- **QSS stylesheets**: `styles/<id>.qss` with type/class/id selectors.
  Cascade: theme default < type rule < class rule < id rule < inline style.
- **Theme variables**: `var(--accent)`, `var(--label)`, etc. follow OS
  light/dark.
- **36 supported style keys**: background, color, padding, margin, gap,
  width/height, min/max constraints, fontSize/fontWeight, textAlign,
  borderRadius, strokeWidth/borderWidth, opacity, boxShadow*, and all
  interaction keys above.

### Script actions (qscript)

When `steps` arrays get too long, write `actions/<id>.qs`:
- `let x = <expr>` / `if <cond> { ... }` / `for item in <list> { ... }` /
  `while <cond> { ... }` / `fn name(p) { ... }`
- `state.x = <expr>` reads and writes state
- `args.name` accesses action arguments
- Full expression language: arithmetic, comparisons, ternary, builtins
- Compile errors name file and line
- See `examples/tetris` for a game written entirely in qscript

### Text input editing (canvas backend)

When an `input` or `textarea` is focused on the native canvas:

- **Selection**: Shift+arrow extends, Cmd+A selects all, click places caret,
  double-click selects word, triple-click selects line, drag selects text.
- **Clipboard**: Cmd+C/X/V with system clipboard (macOS pbcopy/pbpaste).
- **Undo/redo**: Cmd+Z / Cmd+Shift+Z, 50-entry stack.
- **Navigation**: Left/Right, Cmd+Left/Right (word), Home/End, Up/Down
  (textarea).
- **Secure input**: `"secure": true` masks with bullets.
- **Number input**: `"inputType": "number"` with `min`/`max`/`step` clamp.
- **Readonly**: `"readonly": true` — focusable but not editable.
- **IME**: composing text drawn with underline.

### Icon font

66 icons on Unicode Private Use Area U+E000+. Use the `icon` widget:
```json
{ "type": "icon", "props": { "icon": "heart" }, "style": { "color": "#FF3B30" } }
```
Available names: home, folder, star, heart, settings, search, user, bell, mail,
check, plus, minus, trash, copy, info, chevron-right, chevron-down, and 47 more
(auto-generated from the SVG set). See `internal/render/canvas/icon_font_data.go`.

## Don't

- Don't use `value`/`on:{press}`/`{{count}}`/`scene://` (the aspirational spec format — the runtime ignores it).
- Don't `apply_patch` without a matching `preview_patch` token.
- Don't add emoji to UI/code/docs — use the built-in icon font (66 icons).
- Don't skip verification — always run `qorm_check_layout` after edits.
- Don't guess what a widget supports — check the [widget catalog](api/widgets.md) (auto-generated, canonical).
