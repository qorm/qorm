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
  naming file and line; unknown style keys warn. Canvas and HTML both apply
  matching rules (HTML merges them into emitted node CSS). Canvas visual
  effects (filter, mask, scroll-snap, FLIP, spring, text stroke/shadow, …)
  are documented in [docs/styles.md](docs/styles.md) and listed under
  [common style props](api/props.md#common-style-props); showcase
  [examples/canvas-fx](examples/canvas-fx). Example stylesheet:
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
- `-tags desktop` = native WebView window (HTML path: WKWebView / WebView2 /
  WebKitGTK), opt-in and per-platform (cgo).
- macOS default without `-tags desktop` opens a pure-Go canvas window
  (software renderer). Other platforms without desktop fall back to a browser
  window.
- Games WASM uses `-tags qorm_canvas` (site games page). Raiden/Mario-class
  games need that canvas host; default `qorm package -p web` is HTML morph,
  not full canvas game fidelity.
- Canvas window + real WKWebView overlays for `webview` widgets (macOS,
  cgo): `-tags canvaswebview`; every other build draws the widget's
  placeholder (HTML renderer uses an `<iframe>`). Demo: `go run -tags
  canvaswebview ./cmd/qorm run examples/webdemo`.

## Canvas engine (`internal/render/canvas`)

The native canvas backend is a pure-Go retained-mode software renderer used by
the macOS default window (no `-tags desktop`) and by games WASM
(`qorm_canvas`). It is **not** the `-tags desktop` path — that tag is the
native WebView (HTML). Same runtime model as the HTML path (same model tree,
same state, same actions) but renders to a pixel buffer via a display-list
rasterizer.

**Architecture:** `input/state → layout → record (display list) → render`
- **Single-threaded**: input and rendering both run on the host's main thread;
  external mutations (HTTP/MCP) are enqueued and drained at the frame boundary.
- **Display list**: rebuilt from scratch each frame (`op.Ops`), reusing backing
  arrays in steady state.
- **Graph scene**: model nodes → draw graph (`graph.Node`); hit-testing walks
  the same tree that painting uses, so what you see is what you click.

**Interaction:**
- **Pointer**: hit testing, press/release dispatch, event bubbling, pointer
  capture for InteractiveWidgets, hover tracking with onHoverIn/out.
- **Keyboard**: Tab/Shift+Tab focus traversal (includes all InteractiveWidget
  types), Enter/Space activation, Escape blur, modifier keys (Ctrl/Meta/Alt),
  onKeyDown/Up bubbling.
- **Text editing**: full selection model (SelStart/SelEnd/Anchor), Shift+arrow
  extend, Cmd+A/C/X/V/Z/Y, word/line navigation, caret blink, IME composition
  preview, undo/redo (50-entry stack), number input with min/max/step clamp.
- **Scroll**: 2-axis viewports with chaining (inner→outer bubbling),
  scroll-to-focus on keyboard Tab, scrollbar thumbs, **momentum inertia**
  (frame-rate-independent friction decay, 0.88).
- **Board**: infinite-canvas pan (drag-to-pan, Ctrl+scroll zoom), **momentum
  coast** after drag release (0.92 friction).

**Visual effects (declarative — any node can declare them):**
- `pressedScale` / `hoverScale`: scale transform, animated through the
  `transition` system (no snapping).
- `pressedBackground` / `hoverBackground`: theme or per-node color swap.
- `pressedOpacity` / `hoverOpacity`: per-node opacity.
- `transition`: CSS-style duration (`"0.2s"`, `"200ms"`, `"0.3s spring"`) —
  interaction, absolute `x`/`y`, and FLIP; `transitionEasing: "spring"` for
  underdamped spring.
- `layoutMotion`: FLIP ease of absolute position/size (stable `id` +
  `transition`).
- `filter` / `blur` / `mixBlendMode` / `maskFade` / `clipPath` / `layerCache`:
  CSS filter stack (incl. `invert()` / `sepia()`), blend modes (`multiply` /
  `screen` / `overlay` / `darken` / `lighten` / `difference` / `exclusion` /
  `color-dodge` / `color-burn` / `hard-light` / `plus-lighter` / `lighter`),
  soft masks, clip-path (`circle` / `ellipse` / `inset` / `polygon`),
  offscreen layer reuse.
- `tint`: RGB modulate on the subtree (Godot modulate / Phaser tint).
- `imageRendering: pixelated`: nearest-neighbour even on fractional scales.
- `rotate` / `scale` / `scaleX` / `scaleY` / `flipX` / `flipY` / `skewX` /
  `skewY` / `transformOrigin`: persistent visual transform (skew is degrees,
  graph shear; origin is CSS `center` / `left top` / `50% 0` / `12px 8px`,
  default center); layout box unchanged.
- `zIndex`: sibling paint + hit order (canvas; `0` = auto). Already in
  `KnownStyleKeys` (HTML `z-index`); canvas now sorts paint/hit.
- `scrollSnapType` / `scrollSnapAlign`: scroll-snap after coast/drag release.
- Text: `textStroke*` / `textShadow*` / `textDecoration` / `textTransform` /
  `lineClamp` / `textOverflow`.
- Chrome: `boxShadow*` (incl. inset) / `outline*` / `stroke*` / conic-gradient.
- `disabled`: blocks pointer activation, shows not-allowed cursor, dims to
  50% opacity on non-widget nodes (interactive widgets handle their own).
- `animation`: entrance effects (fade, slide, bounce, shake, pulse, spin);
  optional `curve` (game-engine easings: `backOut`, `elastic`, `bounce`, …).
- `fx` / `fxToken`: game feedback (shake, punch, flash, hit, float, wobble,
  knockback, burst) — restarts when token bumps; fire from qscript. Inspired by
  DOTween / Phaser / Godot one-shots.
- `timeline` / `timelineToken`: DOTween Sequence / Godot Tween chain
  (Append steps, `parallel` Join, wait, `path` follow + orient, yoyo/loop).
- `timelineOnComplete` / `onComplete`: fire action once when finite timeline ends.
- `stagger`: list index × ms delay for entrance / fx / timeline.
- Style `transitionYoyo` / `transitionLoop` / `transitionRepeat` on property
  tweens (DOTween SetLoops).
- Showcase: `examples/canvas-fx`. Docs: [docs/styles.md](docs/styles.md),
  [api/props.md](api/props.md), [api/animation.md](api/animation.md).

**Widgets:** 146 canonical types registered (`internal/widgets`; the
auto-generated `api/widgets.md` catalog is the exact list), 30+ interactive
(custom HandlePointer), 5 overlay (drawer, menu, modal, snackbar, tooltip),
plus engine `tilemap` (baked char-grid world bitmap). The canonical showcase
is `examples/widget-showcase`; side-scrollers are `examples/mario`.

**Board camera (side-scrollers):** `cameraTarget` / `cameraCenter` /
`cameraCell` / `cameraViewport` / `cameraDeadZone` / `cameraLockLeft` /
`cameraResetToken` / `cameraMax` / `disablePan`. Restart must bump
`cameraResetToken` or lock-left will not rewind. Do not tween physics `x`/`y`.

**Style system:** 108 keys in `render.KnownStyleKeys` (box/text/chrome/
filter/mask/snap/motion/pseudo/backdrop). Theme cascade: default ← QSS type
rule ← class rule ← id rule ← inline style. `warnUnsupportedStyleKeys`
reports unknown keys once per scene.

**Fonts:** 5×7 bitmap for ASCII, TrueType/SFNT for CJK (Source Han Sans SC
subset, ~3.4MB), unified bitmap icon font on U+E000+ PUA (53 glyphs from the
`widgets.IconSet()` SVG set: 17 hand-crafted in `icon_font_data.go` take
precedence over the rest, auto-rasterized into `icon_font_auto.go` via
`tools/genicons`).

**Testing:** ~435 canvas tests, ~123 widget tests. Run: `go test
./internal/render/canvas/... ./internal/widgets/...`.
