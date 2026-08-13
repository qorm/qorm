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
- RichText: the `richtext` widget takes a `spans` array (each with `text` and `style`).
- Video: use `{ "type":"video", "src":"...", "loop":true, "autoplay":true, "muted":true }` for video playback or looping backgrounds.
- Paths: the `path` widget takes a `d` property for SVG path data, which can morph with `transition` and state bindings.
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

`qorm mcp <app-dir>` (or `/mcp` on a running `qorm run`) exposes the same
tool list as [docs/agent/mcp-tools.md](../../docs/agent/mcp-tools.md)
(auto-generated from `internal/mcp/tools.go`):

- **Understand**: `qorm_inspect`, `qorm_get_node`, `qorm_query`,
  `qorm_source_location`, `qorm_list_actions`, `qorm_render_html`,
  `qorm_capture_subtree`, `qorm_capture_canvas`, `qorm_a11y_tree`, `qorm_capabilities`,
  `qorm_activity` (shared session: what the human just did),
  `qorm_export_scene`, `qorm_export_bundle`.
- **Operate**: `qorm_dispatch`, `qorm_set_state`.
- **Window** (desktop): `qorm_window` (move / open / close / eval / tile /
  focus / minimize / pin).
- **Design** (review-bound): `qorm_preview_patch` → `qorm_apply_patch`
  (must carry the preview's `previewToken`); `qorm_undo`, `qorm_diff`.
- **Verify**: `qorm_measure`, `qorm_check_layout`, `qorm_assert`,
  `qorm_validate`.
- **No side effects**: `qorm_simulate_action`.
- **Canvas Specifics**: To trigger visual effects (`fx` or `timeline`) over MCP, use `qorm_set_state` to increment the state bound to `fxToken` or `timelineToken`. To apply or test visual effects (`filter`, `maskFade`), use `qorm_preview_patch` to inject the style changes. Always verify Canvas specific UI with `qorm_capture_canvas`.

## Always self-verify

After an edit, prove it against the rendered reality rather than assuming:
- In a live `qorm run` session, use `qorm_inspect` for app/state/diagnostics,
  then `qorm_measure` and `qorm_check_layout` over its HTTP `/mcp`. The macOS
  canvas window supplies its retained render graph; browser/WebView hosts
  supply their DOM report.
- For native pixel evidence, call `qorm_capture_canvas`. It reads the last frame
  actually presented by the live Canvas host. Optional `id` returns the full
  surface PNG plus a physical-pixel `clip` (not an isolated subtree); absence of
  a Canvas host/frame or an invisible/missing node is an error. For deterministic
  offline pixels use `qorm shot <app> -o out.png` (headless pure Canvas).
- A standalone stdio `qorm mcp <app>` has no rendering host, so live measure /
  layout check is unavailable there. Use CLI `qorm measure <app>` /
  `qorm check <app> --audit` for deterministic, headless canvas verification.
- Default CLI `measure` / `check` is pure-Go canvas. Canvas `{"steps":[…]}`
  flows can dispatch/set state, press, type, send keys, scroll, and advance
  waits deterministically. Build with `-tags desktop` only when you need
  WebView/DOM parity (that flow keeps its dispatch/setState subset).
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

macOS **default** `qorm run` (no `-tags desktop`) is the pure-Go software
canvas window. `-tags desktop` is the **native WebView** (HTML), not canvas.
Games WASM uses `-tags qorm_canvas`. Both backends consume the same JSON,
state, actions, QSS cascade, and widget model, but canvas layout/paint and HTML
DOM/CSS are independent implementations — verify the backend you will ship:

- **Layout**: canvas uses its retained graph (`measure → layout → record →
  raster`); HTML uses DOM/CSS. `qorm measure` selects canvas by default and
  WebView when built with `-tags desktop`.
  Inline/QSS `width: "fill"` / `height: "fill"` fills the containing content
  box on canvas (minus child margins); HTML emits `100%`. Numeric sizes are
  pixels.
- **Interaction**: full keyboard navigation (Tab/Shift-Tab, Enter/Space, Escape,
  modifier keys), scroll viewports with momentum inertia, text editing with
  selection/clipboard/undo, animated pressed/hover transitions, disabled visual
  dimming.
- **Shared activity**: actions dispatched through native canvas pointer or
  keyboard input are attributed to `human` in the same DevTool and
  `qorm_activity` event stream as browser/WebView actions. Canvas also reports
  privacy-safe focus/typing/filled presence; password values are never sent.
- **Widgets**: all 146 canonical widget types work (full catalog:
  `api/widgets.md`), including overlay panels (drawer, menu, modal, snackbar,
  tooltip) and interactive controls (switch, slider, checkbox, select,
  textarea, draggable).
- **Run canvas**: `go run ./cmd/qorm run <app>` (macOS default). WebView:
  `go run -tags desktop ./cmd/qorm run <app>`.

### Declarative interaction & canvas effects (any node)

These style keys work on ANY node — no per-widget logic needed:

| Key | Effect |
|---|---|
| `pressedScale` / `hoverScale` | Scale transform on press/hover |
| `pressedBackground` / `hoverBackground` | Color swap on press/hover |
| `hoverColor` / `focusBorderColor` | Hover text color / canvas focus-ring color (QSS cascade applies) |
| `pressedOpacity` / `hoverOpacity` | Opacity change on press/hover |
| `transition` | Duration (`"0.2s"`, `"200ms"`, `"0.3s spring"`) — interaction, x/y, FLIP |
| `transitionEasing` | `"spring"` (underdamped) or named ease |
| `layoutMotion` | FLIP ease of absolute position/size (needs stable `id` + `transition`) |
| `aspectRatio` | Width/height; with exactly one explicit positive axis Canvas derives the other |
| `position: absolute` + `left`/`top`/`right`/`bottom` | Out-of-flow placement; `x`/`y` alias left/top, and leading anchors win on the same axis |
| `cursor` | QSS/inline native mapping: pointer, text, not-allowed, default (plus hand/ibeam/forbidden/arrow aliases) |
| `filter` / `blur` / `filterBlur` | CSS filter stack (incl. `invert()` / `sepia()` / `hue-rotate()`) / group blur |
| `tint` | RGB modulate (Godot modulate / Phaser tint) |
| `imageRendering` | `pixelated` = nearest-neighbour (pixel art) |
| `rotate` / `scale` / `scaleX` / `scaleY` / `flipX` / `flipY` / `skewX` / `skewY` | Persistent transform (skew in degrees; graph shear); layout box unchanged |
| `transformOrigin` | CSS pivot for rotate/scale/flip/skew: `center`, `left top`, `50% 0`, `12px 8px` (default center) |
| `zIndex` | Sibling paint + hit order (canvas; `0` = auto). HTML already emitted CSS `z-index` |
| `mixBlendMode` | `multiply` / `screen` / `overlay` / `darken` / `lighten` / `difference` / `exclusion` / `color-dodge` / `color-burn` / `hard-light` / `plus-lighter` / `lighter` |
| `maskFade` / `maskFadeSize` / `maskImage` | Soft edge dissolve |
| `clipPath` | `circle()` / `ellipse()` / `inset(… round …)` / `polygon(...)` |
| `layerCache` | Reuse offscreen layer when content fingerprint unchanged |
| `scrollSnapType` / `scrollSnapAlign` | Scroll-snap on viewports / children |
| `textStroke*` / `textShadow*` | Glyph outline and drop shadow |
| `boxShadow*` / `outline*` / `stroke*` | Box shadow (incl. inset) and outer outline. `stroke*` supports `strokeDasharray` and `strokeDashoffset` |
| `disabled` / `disabledOpacity` | Blocks pointer/keyboard activation, dims (default 50%), shows not-allowed cursor |
| `aria-label` | Accessibility label read by screen readers (critical for A11y instrumentation) |
| `fx` + `fxToken` | Game feedback (shake/punch/flash/hit/float/wobble/knockback/burst); bump token from qs |
| `timeline` + `timelineToken` | Sequence: Append, Join, path/cubic, yoyo/loop |
| `timelineOnComplete` / `onComplete` | Action when finite timeline finishes |
| `stagger` | List index × ms delay (entrance / fx / timeline) |
| `transitionYoyo` / `transitionLoop` / `transitionRepeat` | Style property tween loops (DOTween SetLoops) |
| `animation` + `curve` | Entrance effects; `curve` uses game-engine easings (`backOut`, `elastic`, `bounce`, …) |

Runnable showcase: `examples/canvas-fx` (structure in `scenes/`, style in
`styles/app.qss`, logic in `actions/*.qs`). Games:
`examples/mario` · `examples/raiden` · `examples/tetris` · `examples/g2048`.
Full key list: `api/props.md`, `api/animation.md`, `docs/styles.md`.

### Side-scrollers and tile worlds (board + tilemap)

Use a `board` as the **world plane** (not a `row`/`column` of tiles):

| Prop | Meaning |
|---|---|
| `cameraTarget` | `{{state.player}}` object with `x`/`y` in px |
| `cameraCenter` | `true` / `"x"` / `"y"` |
| `cameraCell` + `cameraViewport` | e.g. `32` and `16` → 512 px wide follow window |
| `cameraDeadZone` | px; NES-style left band before the camera scrolls |
| `cameraLockLeft` | never scroll back (SMB). Pair with `cameraResetToken` |
| `cameraResetToken` | bump `{{state.cameraGen}}` on restart so lock-left rewinds |
| `cameraMax` | `{ "x": (levelW - viewportW) * cell }` |
| `disablePan` | no user drag-to-pan (games) |

`tilemap` bakes a char-grid + atlas into **one** world bitmap (do not
`list` hundreds of tile images every frame):

```json
{ "type": "tilemap", "id": "tiles",
  "rows": "{{state.rows}}", "cell": 32,
  "bumpX": "{{state.bumpCX}}", "bumpY": "{{state.bumpCY}}", "bumpT": "{{state.bumpT}}",
  "atlas": { "1": "assets/ground.png", "2": "assets/brick.png" },
  "style": { "x": 0, "y": 0 } }
```

Actors (player, enemies) stay as `image` / `list` children **on the same
board**, with absolute `x`/`y`. HUD lives **outside** the board (stack overlay).

**Game motion rules (do not ignore):**
- Do **not** tween physics `x`/`y` or put `layoutMotion` on 60 fps movers.
- Puzzle boards (Tetris / 2048): local FX only — never shake the whole grid.
- Hold-to-move keys need `keyReleases` (and a no-op keyup for one-shots
  like restart so OS key-repeat does not re-fire).
- Drive style from qscript via `state` + `{{bindings}}` / QSS — do not
  assign style objects in qs.
- Audio: `playSound(src)` / `playMusic(src)` / `stopMusic()`; WAV under
  the app dir. Native SFX do not kill looping BGM.
- Pixel art: `imageRendering: pixelated` (QSS `image { … }` is enough).
- Super-size sprites need their own bitmap (do not `cover`-crop a square
  into a tall box). Grow/shrink must keep feet on the ground.

### Style system

- **QSS stylesheets**: `styles/<id>.qss` with type/class/id selectors.
  Cascade: theme default < type rule < class rule < id rule < inline style.
  **Same keys as inline `style`** — including every canvas FX key above.
  Values may be numbers, strings, `var(--x)`, or `{{bindings}}` (re-evaluated
  each frame). Nested objects stay on the node.
- **qscript drives styles via state**: scripts write `state.x`; QSS/inline
  bindings pick it up (no direct style assignment in qs). Example:
  `state.filterOn = !state.filterOn` + `.filterCard { filter: {{ state.filterOn ? "…" : "none" }} }`.
- **Theme variables**: `var(--accent)`, `var(--label)`, etc. follow OS
  light/dark.
- **Backgrounds**: supports solid colors, `linear-gradient(...)`, `radial-gradient(...)`, and `conic-gradient(...)`.
- **~100 supported style keys** (`render.KnownStyleKeys`): box model, text
  (decoration/transform/clamp/stroke/shadow), chrome/shadow/outline, filter/
  mask/clip, scroll-snap, layout motion, interaction, backdrop — see
  [api/props.md](../../api/props.md) common style props.

### Script actions (qscript)

When `steps` arrays get too long, write `actions/<id>.qs`:
- `let x = <expr>` / `if <cond> { ... }` / `for item in <list> { ... }` /
  `while <cond> { ... }` / `fn name(p) { ... }`
- `state.x = <expr>` reads and writes state
- `args.name` accesses action arguments
- Full expression language: arithmetic, comparisons, ternary, builtins
- Audio builtins: `playSound` / `playMusic` / `stopMusic`
- Compile errors name file and line
- See `examples/tetris` / `examples/mario` for games written in qscript

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

53 icons on Unicode Private Use Area U+E000+. Use the `icon` widget:
```json
{ "type": "icon", "props": { "icon": "heart" }, "style": { "color": "#FF3B30" } }
```
Available names (the built-in SVG set, `widgets.IconSet()`; the canvas bitmap
font rasterizes them via `go generate ./internal/render/canvas/`):
alert, battery, bell, bluetooth, book, brick, brightness, camera, check,
chevron-down, chevron-right, clipboard, coin, compass, copy, database, device,
download, fingerprint, flag, flashlight, folder, globe, goomba, ground, heart,
home, image, inbox, info, location, lock, mail, mario, menu, mic, minus, nfc,
plus, screenshot, search, settings, share, star, sun, trash, upload, user,
video, volume, wifi, x, zap.
Hand-crafted glyphs in `internal/render/canvas/icon_font_data.go` take
precedence; the rest come from `icon_font_auto.go` (generated).

## Don't

- Don't use `value`/`on:{press}`/`{{count}}`/`scene://` (the aspirational spec format — the runtime ignores it).
- Don't `apply_patch` without a matching `preview_patch` token.
- Don't add emoji to UI/code/docs — use the built-in icon font (53 icons).
- Don't skip verification — always run `qorm_check_layout` after edits.
- Don't guess what a widget supports — check the [widget catalog](api/widgets.md) (auto-generated, canonical).
- Don't tween physics `x`/`y` or `layoutMotion` a 60 fps sprite.
- Don't `list` a whole tile level — use `tilemap`.
- Don't confuse `-tags desktop` (WebView/HTML) with the default macOS canvas window.
