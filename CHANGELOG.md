# Changelog

All notable changes to QORM are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Releases are tagged
`vX.Y.Z`; curated release notes live in the tag annotations.

## [Unreleased]

## [v0.9.3] - 2026-08-14

### Changed
- **Repository renamed to `github.com/qorm/platform`**: Go module path, package imports, CI/CD workflows, container image targets (`ghcr.io/qorm/platform`), and official website links (`qorm.com`) updated from `github.com/qorm/qorm` to `github.com/qorm/platform`. The project and CLI name remain `qorm`.

## [v0.9.2] - 2026-08-13

### Added
- **`qorm.config.json` is the home of the host window**: the optional file
  beside `qorm.json` now takes a `window` block — `width` / `height` /
  `title` / `resizable` / `chromeless` / `transparent` / `hideLog` /
  `hideTray`. It is host/build-time config (never bundled or signed) and wins
  over the manifest's `platforms.desktop.window`; the legacy `display` block
  is still accepted and merges per key. Malformed JSON and unknown keys now
  surface as load diagnostics instead of being silently dropped, and an
  explicit `"width": 0` resets a manifest-declared size back to fluid.
- **`qorm_inspect` reports the resolved host window** (`window` block:
  width/height/title/resizable/chromeless/transparent), so an agent sees
  exactly what the host will open.
- **Shaped (异形) windows on Windows**: `chromeless` now strips the title bar
  there too (user32, no cgo); combined with `transparent` on the macOS WebView
  host this covers custom-shape windows on the two main desktop targets.

### Changed
- **`resizable` and `title` are actually enforced**: previously parsed but
  dead. `"resizable": false` locks the window to its declared size on every
  host (macOS WebView style mask, non-macOS webview `HintFixed`, canvas
  window mask) via a new `Window.Fixed` tri-state that distinguishes explicit
  false from absent; a declared `title` wins over the app name in the window
  title bar. A chromeless window that stays `resizable` keeps its sizing
  border (on Windows that frame bit is what makes a window user-resizable).
- **README carries no per-version sections** — version history lives in this
  changelog and the tag annotations.

### Fixed
- **Windows chromeless geometry and resize**: the decoration strip
  unconditionally cleared `WS_THICKFRAME`, silently ignoring `resizable`, and
  ran after `SetSize`, so the frame math used the decorated style and the
  content area came out ~16×39 px larger than declared. Chrome is now stripped
  before `SetSize` and the sizing border is kept when resizable.
- **macOS fixed windows keep their declared size**: a stale remembered frame
  (`window.txt`) no longer overrides a `resizable: false` window and locks it
  there — only resizable windows restore their remembered frame.
- **Audio respawn storm**: loop music is respawn-on-exit; on a host with no
  audio device the tool exits instantly and was respawned forever (an
  unbounded stream of failing processes — also the `-race` CI flake). A
  sub-500 ms lifetime now means "broken sink, stop".
- **Icon font completeness**: `go:generate` for the canvas bitmap font failed
  silently, leaving 6 Mario glyphs (brick/coin/flag/goomba/ground/mario) out;
  the generator now takes `-o` and fails loudly. 47 → 53 glyphs, exactly
  `widgets.IconSet()`.
- **darwin cross-compile without cgo**: the video decoder's build tags
  excluded its pure-Go fallback from `CGO_ENABLED=0` cross-builds
  (`startVideoDecoder` undefined — this is what broke the v0.9.1 release
  workflow's binaries), re-tagged.
- **CI**: desktop matrix compile errors in the chromeless path (untyped
  `Hint` constant, `uintptr(-16)` overflow) — both root-caused and now
  verified locally via mingw cross-compile; `TestRaidenPerf` skips its
  perf budget under `-race` (logic assertions kept); iOS/Android packaging
  jobs dropped from CI (kept: mac/ubuntu/windows + docker).

### Docs
- **SKILL/MCP/docs/API fully re-synced**: `skills.md` referenced seven skill
  files that never existed — rewritten around the single real `SKILL.md`
  (with its 4 workflows); counts corrected to 53 icons / 146 widgets / 108
  style keys in all three places; missing/awkward `doc.go` ZH translations
  fixed; the generated `mcp-tools.md` (en+zh) gained tool categories and
  examples, guarded by `TestMCPDocInSync`.
- **`qorm.config.json` documented everywhere it matters**: dedicated section
  + key table + precedence in `project-structure.md` (en+zh, surfaced in
  `llms.txt`), window block in `SKILL.md` / `skills.md` / MCP docs /
  `AGENTS.md`; `examples/mario` converted to the new `window` block.
- **Go toolchain requirement clarified**: build-time vs runtime (a shipped
  app runs without Go).

## [v0.9.1] - 2026-08-13

### Added
- **`qorm test` headless test runner** (Phase 9 MVP of
  `planning/spec/test-runner-spec.md`): declarative `type:"test"` documents
  (steps `mount_scene` / `simulate_event` / `set_state`; asserts
  `state_equals` / `node_exists` / `node_not_exists` / `text_equals` /
  `prop_equals`) run against a fresh runtime each, queries evaluate the
  materialized scene tree, the spec's JSON report goes to stdout verbatim,
  exit 0 iff every test passed. `examples/counter` ships 4 test docs.
- **Path widget** (`type:"path"`): SVG-subset `d` (M/L/H/V/Q/T/S/Z, absolute
  and relative) rendered on both engines — canvas software raster with fill
  + stroke, HTML inline `<svg><path>` with `preserveAspectRatio="none"`;
  demoed by the canvas-advanced morph path.

### Changed
- **LAN gate is now two-token**: a non-loopback bind (`--lan` / `--tls`)
  prints an ADMIN token that is never embedded in any served page; `/mcp`,
  `/update`, `/rollback`, `/window` and the diagnostics reads (`/dev/state`,
  `/log`, `/presence` GET) require it. The page token stays valid only on
  browser-needed endpoints (`/event`, `/measure`, in-page writes). Loopback
  binds are byte-for-byte unchanged.

### Security
- **Fixed LAN gate bypass**: the gate secret was the page token, which the
  unauthenticated index page embeds — any LAN peer could harvest it with one
  GET and pass the gate. The admin token is generated separately and never
  serialized into served HTML (adversarially verified with live curl repros).
- **Diagnostics reads gated on --lan**: `/dev/state`, `/log`, `/presence`
  were tokenless on non-loopback binds and leaked live state/activity; the
  gate covers every non-POST verb (a GET-only check leaked the same data
  through OPTIONS/PUT/PATCH/DELETE/HEAD/TRACE — adversarially reproduced).
- **Computed dynamic keys rejected at load**: `computed[...]` refs whose key
  is not a string literal now fail loading with a diagnostic naming the
  computed (previously unresolvable refs surfaced as silent misses).
- **WASM OTA checks revocation**: the web host passes its shipped revocation
  snapshot to bundle verification at boot, rollback and update-check
  (previously `nil`), failing closed on malformed lists.

### Fixed
- **Green baseline restored**: the canvas-ultimate commit swept debug-session
  artifacts into the repo root and `cmd/` — an empty `scratch.go`, duplicate
  `package main` stubs, patch/script scratch, and two ungated cgo/ObjC
  prototype tools (`qorm-video-stream`, `qorm-audio-extract`) — which made
  `go build ./...` fail outright on main. All removed; the shipped video
  widget keeps its own build-gated decoder.
- **Canvas path parser cannot hang**: unsupported/stray tokens (arc
  parameters, bare numbers after `Z`, overflowing coordinates like `1e309`)
  no longer loop the parser — every iteration consumes at least one token.
  Arcs (`A`/`a`) parse their 7 parameters and approximate with a chord to
  the endpoint; arc-to-bezier flattening is deferred.
- **Canvas path offset crop**: without explicit width/height the widget now
  measures an origin-anchored box (`bbox` max, mirroring HTML
  `viewBox="0 0 w h"`), so offset paths paint fully instead of a corner
  sliver.
- **`qorm test` enter-hook crash handling**: a scene whose `onEnter` action
  raises a qscript runtime error now fails the test (`test_runtime_error`)
  instead of reporting green — including crashes anywhere in a
  navigate-then-enter chain (the runtime accumulates the chain's FIRST crash
  in `EnterScriptError`; each link's dispatch used to wipe the previous
  link's error). `mount_scene` steps now actually fire the target's
  `onEnter` (only the document-level mount fired before). `-`-prefixed
  arguments are rejected as unsupported flags instead of being misread as
  file paths.
- **Bundle hash-mismatch hint**: verification failure now names the declared
  version when the bundle carries one (message-only; fail-closed unchanged).
- **API doc generator preserves hand-written sections**: the `QORM_UPDATE_DOCS`
  regeneration of `api/widgets.md` / `api/props.md` keeps the RichText /
  Video / Accessibility trailer and the hand-edited schema rows (ported into
  the generator); `api/http-api.md` (en + zh) regenerated in sync.

### Docs
- Homepage and docs feature canvas-ultimate; site deployed.

## [v0.9.0] - 2026-08-13

### Added
- **Video widget**: per-frame native AVFoundation decoding on macOS
  (`video_decoder_darwin.go`), HTML5 `<video>` on the web host, poster
  fallback elsewhere; `src` / `loop` / `autoplay` / `muted` props; demo asset
  `assets/video.mp4`.
- **Canvas-advanced showcase** (`examples/canvas-advanced`): board camera
  follow + drag interactions, RichText mixed-style paragraph, timeline path
  follow with `orient`/`cubic`, fx triggers.
- **Canvas-native measure / shot flows**: `qorm measure` runs the canvas
  layout pipeline without a WebView (`measure_flow_canvas.go`); `qorm shot`
  gains a canvas path on non-darwin hosts.
- **Live canvas capture**: console / MCP capture of the running canvas frame
  (`internal/server/canvas_capture.go`, `cmd/qorm/live_canvas_capture.go`).
- **canvaswebview demo app** for the `-tags canvaswebview` overlay path.

### Changed
- **Fixed conditional rendering**: `render` steps evaluate `when` conditions
  against post-step state (runtime + render-step regression tests).
- **Advanced text styling**: RichText spans with per-span color / weight /
  size / shadow in the canvas graph renderer.
- Docs refresh across styles / verification / desktop / collaboration /
  MCP-tools (EN + ZH).

## [v0.8.11] - 2026-08-12

### Changed
- **Mario is NES 1-1 at 2x, not a generic platformer**: 32 px tiles with a
  24×32 (big 24×64) hitbox; walk/run/jump/gravity are 2x SMB numbers;
  camera dead-zone + `cameraLockLeft` (no scroll back); HUD overlays the
  512×480 playfield (MARIO / COIN / WORLD / TIME); `?` blocks are coins
  except one mushroom; Koopa stomps into a kickable shell. Side-panel HUD,
  16 px hitbox-in-32 px-world, and 1x gravity in a 2x stage are gone.
- **Mario canvas frames stay on budget**: one `viewTiles` list (not one
  list per tile kind), skip rebuild while the camera column is unchanged,
  `fill`+index instead of `push`. Engine: axis-aligned `RRectOp` / nearest
  `ImageOp` blit via Pix (sky + tiles); board lists frustum-cull off-camera
  items at measure so far goombas are not laid out.
- **Mario R rewinds the camera**: board `cameraResetToken` (Mario binds
  `state.cameraGen`) snaps follow-cam past `cameraLockLeft` / dead-zone
  sticky pan so restart is not stuck mid-level.
- **Native audio + keys**: OS key-repeat no longer re-fires scene actions
  (jump/restart). `StdoutSink` keeps looping music in its own `afplay`
  slot so `playSound` cannot kill BGM or hitch the frame.
- **Mario Super form**: mushroom pickup stands him up 32px so the 64px
  hitbox is not buried in the floor (that cancelled jumps). Dedicated
  32x64 sprites (`mario_big*.png`) instead of `cover`-cropping a 32x32
  square. `viewTiles` keeps a 4-column slack so running does not rebuild
  the tile list every column; board pan is integer.
- **Canvas `tilemap`**: bakes a char-grid + atlas into one world bitmap
  (cached until rows/bump change). Mario paints the whole 1-1 layer as a
  single image and pans it — no per-tile list on the hot path.
- **Board dirty rects follow the camera**: pan/zoom is a content
  transform, so layout AbsX never moved and a walk only redrew Mario —
  the world stayed on the previous frame. Camera motion now forces a
  full-frame raster.
- **Board scenes always full-frame**: enemy motion with a still camera
  used world-space dirty rects and strobed. Super Mario is unstuck from
  the floor every tick (and on jump); physics uses a fixed 1/60 dt.
- **Mario jump height**: standing jump 540 px/s, hold gravity 1040
  (about 4.4 tiles), fall 2080 (tap still ~2 tiles). Old 480/1120/3150
  peaked at ~3.2 / 1.1 tiles and missed the row-9 `?`.
- **Agent skill / MCP docs**: skill lists the full MCP tool surface
  (`qorm_validate`, `qorm_window`, `qorm_a11y_tree`, …); documents
  `tilemap`, board camera (`cameraLockLeft` / `cameraResetToken`),
  qscript audio, and game motion rules. `api/props.md` board/tilemap
  props regenerated from render. `docs/styles.md` (EN/ZH) now has the
  side-scroller + tilemap section.

### Added
- **Canvas stacking / skew / blend extras**: `zIndex` now sorts sibling paint
  and hit order on canvas (`0` = auto; already in `KnownStyleKeys` for HTML);
  `skewX` / `skewY` (degrees; layout box unchanged; graph shear);
  `mixBlendMode` `difference` / `exclusion` / `color-dodge` / `color-burn` /
  `hard-light` (plus the existing five). Showcase sections 23–25 in
  `examples/canvas-fx`.
- **Canvas transform-origin / clip polygon / plus-lighter**: style
  `transformOrigin` (CSS `center`, `left top`, `50% 0`, `12px 8px`; default
  center) pivots rotate/scale/flip/skew; `clipPath` `polygon(...)` (optional
  `evenodd` / `nonzero`); `mixBlendMode` `plus-lighter` / `lighter` (additive;
  `lighter` is the Porter-Duff alias). Showcase sections 26–28 in
  `examples/canvas-fx`.

## [v0.8.10] - 2026-08-12

### Added
- **Canvas game-engine motion**: Phaser/Godot/DOTween easings; declarative
  `fx` + `fxToken` (shake/punch/flash/hit/float/wobble/knockback/burst);
  `timeline` Sequence (Append/`parallel` Join, wait, `path` polyline/cubic +
  `orient`, yoyo/loop); `timelineOnComplete`; list `stagger`; style
  `transitionYoyo` / `transitionLoop` / `transitionRepeat`. Docs:
  [api/animation.md](api/animation.md).
- **Canvas sprite / filter / transform style**: `filter: invert()` / `sepia()`;
  `tint` RGB modulate; `imageRendering: pixelated`; persistent `rotate` /
  `scale` / `scaleX` / `scaleY` / `flipX` / `flipY` (layout box unchanged).
- **`examples/canvas-fx`**: 22-section showcase (snap, mask, conic, clip,
  cache, FLIP, fx, timeline, path, stagger, yoyo, text chrome, frost, press,
  invert/sepia, tint, transform, pixelated). QSS + qscript; `TestCanvasFx*`.
- **Games**:
  - Tetris: bevel minos, local clear flash + banner + gold outline (no
    board shake), original chiptune (`tools/gentetrisaudio`).
  - 2048: local spawn/merge flashes only; the 4x4 frame does not move.
  - Mario: NES sprites, squash stomp, invuln blink, NES death bounce,
    `flipX` from `dir`, `imageRendering: pixelated`.
  - Raiden: grow-in-place explosions, muzzle/engine, hit flash, dual
    ground, pixelated sprites. Physics `x`/`y` stay untweened.
- **Canvas visual effects** (style / QSS): clip-path (circle/ellipse/inset),
  `layerCache`, scroll-snap, `maskFade` / `maskImage`, text decoration /
  transform / `lineClamp` / stroke / shadow, outline, conic- + radial-
  gradient (stop %), full CSS filter + mix-blend-mode, overflow:hidden
  (rounded clip), inset box-shadow, spring `transition`, FLIP
  `layoutMotion`, dirty-region redraw, ops fingerprint skip, local-space
  rotated RRect sampling.

### Changed
- Puzzle-grid docs: Tetris / 2048 no longer described as whole-board
  shake/burst. Live design is local flash + banner / tile color only.
- Mario death keeps physics running through the NES bounce (`deathDone`)
  instead of freezing on `status=dead`.

## [v0.8.9] - 2026-08-12

### Added
- **Canvas measure settle + visibility**: `MeasureScene` force-settles entrance
  clocks for deterministic CLI/MCP snapshots; live rows report effective
  opacity (style × entrance fade), `animating`, and HTML-like `visible`
  (size + opacity > 0.01). `qorm measure --physical` opts into device pixels.
- **Canvas measure depth**: CollectMeasure enriches rows with HTML-parity
  style fields (color, background, fontSize/Weight, padding, margin,
  borderRadius, border, overflowX/Y), reports a `__stage` frame, and supports
  **logical CSS px** (`MeasureOpts.Logical`) so HiDPI physical layout still
  matches design-time agent checks. Canvas host / `MeasureScene` use logical.
- **Nested scroll drag chaining**: touch-drag at the inner viewport's hard
  edge bubbles to the outer scroller (innermost rubber-band otherwise).
- **Canvas scroll steals from list rows**: a vertical drag that starts on an
  InteractiveWidget inside a scroll viewport (checkbox/listtile/…) activates
  scroll after slop and axis lock — list rows scroll instead of trapping the
  gesture. Horizontal-dominant travel leaves the child (slider/swipe) alone.
- **Canvas-native measure (agent ground truth)**: `Engine.CollectMeasure` and
  headless `MeasureScene` emit HTML-compatible measurement rows from the pure-
  Go graph. `qorm measure` / `qorm check --audit` work without `-tags desktop`;
  the native canvas host pushes rows to `/measure` every frame for MCP
  `qorm_measure` / `qorm_check_layout`.
- **Canvas touch-drag scroll + rubber-band**: finger/pointer drag on `scroll` /
  `scrollview` with slop, content-follows-finger deltas, rubber-band
  overscroll, spring settle, and coast momentum. Pull past the top threshold
  fires `onRefresh` on the scroll node or a wrapping `refreshindicator`.
- **Canvas refreshindicator polish**: rubber pull height, 8-spoke spinner,
  700ms busy phase (`AnimatedWidget`), softer arm slop so taps pass through.
- **Canvas entrance motion (competitive)**: entrance effects now drive real
  **scale + rotation** about the node center (pop/scale/zoomout/rotate/flip/
  spin/pulse/bounce), not fade-only fallbacks. Bound `animation` name changes
  restart the clock so agents can switch effects live.
- **Canvas selectabletext**: true selection session (focus, drag/shift/Cmd+A)
  with **Cmd+C clipboard copy**; typing/cut/paste blocked (read-only).
- **Canvas transform skew**: `skew` / `skewX` / `skewY` (degrees) via graph
  BaseNode shear; geom.Matrix.Skew shared by hit-test and raster.
- **Canvas remaining catalog ports**: `actionsheet`, `alertdialog`,
  `descriptions`, `dropdownbutton` (Select alias), `materialstepper`,
  `monthview` (calendar grid + events), `motion` (entrance container),
  `picker` (option wheel), `rating` (stars/dots + tap), `refreshindicator`
  (pull-to-refresh), `selectabletext`, and `transform` (rotate/scale/translate/
  skew). HTML↔canvas parity allowlist is empty for core example types.
- **Canvas rangeslider / pageview / tree / autocomplete**: dual-thumb range
  control (low/high bindings), one-page-at-a-time pageview with tap navigation,
  hierarchical tree with expand/collapse (bound `data`, per-node `expanded`),
  and autocomplete with filtered options overlay (shared edit session;
  options may be a bound array).
- **Canvas multi-stop linear gradients + backdrop frost**: `gradient` /
  `background: linear-gradient(...)` paints multi-stop fills along a true
  CSS angle (not axis-snapped); `backdropBlur` (+ optional `backdropTint`)
  frosts pixels under the node.
- **Canvas field / textformfield / richtext / carousel**: labelled field
  wrapper, form field with edit session, span-based richtext with multi-line
  wrap when width-constrained, and a swipe/tap carousel with optional
  indicators.
- **Canvas searchbar**: single-line edit session + filtered results overlay;
  `onSelect` receives `{label}`; typing uses the shared input session.
- **Canvas checkboxlisttile / radiolisttile**: list rows with leading
  checkbox or radio (group selection via bound `value`).
- **Canvas text styles**: `letterSpacing`, `lineHeight` (multiplier or px), and
  `fontStyle: italic|oblique` (faux-italic) flow through measure/wrap/draw;
  tracking is shared by MeasureTextTracking and the rasterizer.
- **Canvas FAB + SwitchListTile**: `fab` / `floatingactionbutton` and
  `switchlisttile` with trailing toggle (binding + onChange like `switch`).
- **Canvas layout/feedback widgets**: `aspectratio`, `ignorepointer`, `skeleton`,
  `circularprogress` (+ `circularprogressindicator`), plus aliases
  `activityindicator`→spinner, `tag`→chip, and `animatedcontainer` as the
  camelCase twin of `animated_container`.
- **Canvas style keys**: `gradient` (first-stop solid), `flexGrow`/`flexShrink`/
  `alignSelf`, `boxShadowX`, `disabledOpacity`, and `size` are first-class
  (no longer false-warn as unsupported).

## [v0.8.8] - 2026-08-12

### Fixed
- **Host `go build ./...`**: move `CanonicalAssetURL` to `internal/playcore` so
  `cmd/qorm-wasm` is fully `js && wasm` gated (a host-only `package main` file
  without `main` broke pure-Go CI).
- **HTML QSS residuals**: `aria-disabled`, spacer `size`, limitedBox max
  constraints, and chart SVG width/height now read the effective style
  (QSS + inline + bindings), matching boxCSS/textCSS cascade instead of raw
  `n.Style` only.
- **Web WASM games (Raiden / Mario)**: asset preloader cache keys now strip
  `?v=` / `#fragment` so they match the engine's `BaseDir + src` lookups.
  The games page had been preloading successfully while every measure pass
  still missed the cache and fell back to main-thread sync XHR — image-heavy
  games failed to render; Tetris/2048 were unaffected (no PNGs). Also: single
  post-`InitFromBundle` preload (the pre-init pass was discarded), fuller
  Mario asset list, and a bumped games-page WASM pin.
- **Canvas golden frames on non-Linux**: SFNT text AA drift for
  `counter_light` / `counter_dark` no longer fails local macOS/Windows runs;
  Linux CI baseline stays strict (`QORM_GOLDEN_FORCE=1` restores hard compare).

### Changed
- **Docs**: clarify that the pure-Go canvas engine (`qorm_canvas` / default
  macOS window) is not the `-tags desktop` native WebView path, and that
  Raiden/Mario-class games need the canvas WASM host (not default HTML web
  packaging).

### Added
- **HTML board camera props**: `cameraTarget` / `cameraCenter` / `cameraViewport` / `cameraDeadZone` / `cameraMax` now drive the HTML board content transform (same pan math as canvas `applyBoardCamera`); `RenderOpts.Board` still wins when Active or pan is non-zero.

- **HTML ↔ canvas widget parity harness**: `TestHTMLCanvasWidgetParity` in
  `internal/integration` compares HTML `node()` switch types (catalog source)
  against canvas engine-native types + `RegisterWidget` names, failing CI on
  core drift outside documented allowlists (`htmlOnlyCoreAllowlist` /
  `canvasOnlyAllowlist`). Not a pixel compare.

- **HTML scene swipes**: scene JSON `swipes` (e.g. `{"left":"slideLeft"}`) now wire on the live server and offline/WASM HTML clients via `app.js`, matching the canvas recognizer (24px floor, 1.3 axis dominance) and key-binding dispatch path.

- **WASM / web audio sink**: qscript `playSound` / `playMusic` / `stopMusic` play
  WAVs via `HTMLAudioElement` on `js/wasm` (games page, packaged web). Paths
  resolve as `BaseDir + src` like images; autoplay policy failures log once and
  fail soft. Desktop `StdoutSink` unchanged.
- **HTML QSS stylesheets**: the HTML render path now merges matching
  `styles/*.qss` rules into node CSS with the same cascade as canvas
  (type < class order < id < inline). Class/type/id selectors work for web
  and `-tags desktop` HTML hosts; `{{bindings}}` in rules evaluate at render
  time like inline style values.
- **Canvas interaction polish**: pressed/hover scale effects now animate through the
  declarative `transition` system instead of snapping — a node with
  `"pressedScale": 0.95` and `"transition": "0.2s"` gets a smooth spring-like scale
  animation on press and release.
- **Scroll momentum**: scroll viewports coast naturally after wheel/trackpad input
  with frame-rate-independent friction decay (0.88). Per-viewport velocity tracked
  via exponential moving average; momentum is skipped in the same frame as the
  scroll event to prevent double-counting.
- **Board pan momentum**: the infinite-canvas board coasts after a drag release
  (0.92 friction for a floaty feel), using the same infrastructure as scroll
  momentum. Velocity is tracked during drag via EMA and cancelled on any new
  blank-space press.
- **Disabled visual dimming**: disabled non-widget nodes (text, box, button, column,
  row, …) automatically render at 50% opacity. Supports a per-node
  `"disabledOpacity"` style key for custom values. Registered interactive widgets
  (switch, checkbox, slider, select, …) are excluded — they handle their own
  disabled visuals via `formDisabled`.
- **Full input editing system**: text selection (Shift+arrow, Cmd+A, click-to-place,
  double-click word, triple-click line/all, drag-select), system clipboard
  (Cmd+C/X/V), undo/redo (Cmd+Z / Cmd+Shift+Z), Home/End, word navigation
  (Cmd+Left/Right), caret blink, IME composition preview, secure input masking,
  number input with min/max/step clamp, readonly mode, selection highlight
  rendering.
- **66-widget native canvas showcase** (`examples/widget-showcase`): card-grouped
  layout demonstrating all structural and data widgets (menu, segmented, accordion,
  listtile, table, breadcrumb, steps, pagination, timeline, navigation rail,
  drawer, empty, icons, …).
- **Unified bitmap icon font**: 66 hand-crafted + auto-generated icons on Unicode
  Private Use Area U+E000+, rendered as variable-width bitmap glyphs at any scale,
  with a `go:generate` SVG→bitmap pipeline (`tools/genicons`).

## [v0.5.5] - 2026-07-30

### Added
- `qorm_validate` MCP tool in `internal/mcp`: validates scene nodes or whole app against component schemas, widget catalog type rules, and expression syntax (`{{...}}`) before patching or saving.
- Headless Pure Go Layout Verification Fallback in `internal/measure`: synthesizes node AST intent geometry for `qorm_check_layout` and `qorm_measure` when running without a connected live browser.

### Optimized
- Thread-safe `ComputedOrder` caching in `internal/model`: caches dependency graph topology on `model.App`, eliminating per-frame Kahn algorithm recalculation overhead.
- IME Composition Protection in `internal/server/app.js`: tracks CJK `compositionstart`/`compositionend` events to prevent DOM morphing from interrupting active IME candidate windows.

## [v0.5.4] - 2026-07-29

### Added
- `RenderNodeDiff` in `internal/render`: renders isolated node subtrees wrapped in morph template tags for live SSE updates.
- `qorm_capture_subtree` MCP tool in `internal/mcp`: allows AI agents to capture node subtree HTML and layout structure for visual feedback.
- `SwapAppPreservingState` in `internal/runtime`: preserves user form inputs and state bindings when hot reloading app manifests.

## [v0.5.3] - 2026-07-29

### Added
- Container Queries DSL (`cq-sm`, `cq-md`, `cq-lg` breakpoints & `container: true` style key) in `internal/render`.
- Multi-token agent self-healing & enforcement for `fontSize` and `spacing` tokens in `internal/mcp`.
- Enhanced `datatable` widget with key/title column parsing and sticky headers.

## [v0.5.2] - 2026-07-29

### Fixed
- Respect `--no-open` in `-tags desktop` builds: avoid invoking `launchWindow` / WebKitGTK window initialization when `--no-open` is passed, enabling headless Linux/Ubuntu servers and CI runners to start cleanly.

## [v0.5.1] - 2026-07-29

### Added
- Single-Prompt Quickstart & Native Executable App Window Default: `qorm run <dir>` automatically scaffolds missing directories and launches as a standalone native executable application window for the host platform by default (`--web` / `--browser` for browser tab mode). DevTool (`logwindow` / `/console`) also opens as a standalone native executable window.
- Declarative Responsive Breakpoints DSL: `sm` (<=640px), `md` (641-1024px), `lg` (>=1025px) breakpoint style maps in `internal/render`.
- MCP Structured Agent Self-Healing Error Payloads: `qorm_preview_patch` and `qorm_apply_patch` include `validTokens` and `suggestedFix` when `designTokens` are violated.
- Hardware Capability Requirement Enforcement: `qorm build --require-capability` stamps requirements into bundle manifests and `qorm run` validates against host platform capability matrix at startup.
- Subtree Partial Rendering: `RenderSubtree(rt, nodeID)` in `internal/render` for isolated node subtree rendering without full-scene string allocation.
- Bracket-Spelled Computed Dependency Scanning: `computedRefs` in `internal/model` supports bracket-rooted paths (`state['computed']['total']`).

## [v0.5.0] - 2026-07-27

### Added
- `computed` — derived values declared once in the manifest (beside or inside
  `globalState`) instead of repeating the same expression in every binding.
  They are evaluated once per frame boundary rather than once per read, may
  read one another in any declaration order, and are published read-only at
  `state.computed.<name>` (`{{ state.computed.total }}` in a scene binding,
  `{{ computed.total }}` inside an action). A dependency cycle is an error
  diagnostic and those values evaluate to nothing instead of recursing; a step
  that writes into the namespace is an error diagnostic and the whole step is
  dropped at dispatch, so a gated `http.get` never issues its request. A
  derived expression sees `state.*`, `t` and `viewport.*`, but not `route.*`.
  An app that declares none is untouched — `computed` stays an ordinary state
  key.
- Scene route `guard` — `{"guard": {"condition": …, "redirect": …, "params": …}}`
  on a scene document declares the precondition for entering it. It runs on
  every entry path (an action's `navigate` step, browser Back/Forward, a deep
  link, and the initial entry scene), before the scene's `onEnter`, so a
  protected route cannot be reached by spelling a URL and a private-data hook
  never fires for a refused visitor. A redirect out of the entry scene replaces
  the current frame rather than pushing one, guards chain through the redirect
  target, and a chain that revisits a scene refuses the navigation (capped at 8
  hops) — reported at load time as a possible redirect cycle. A guard
  re-evaluates derived values privately first, so `set user` → `navigate` in one
  action is not bounced by the pre-login view; `navigate` with `back: true` is
  not guarded.
- `forEach` step — `{"type":"forEach","in":"{{ state.items }}","as":"row",
  "steps":[…]}` runs its body once per element, binding the element under `as`
  (default `item`) plus `index` / `first` / `last`, exactly like a list's
  `renderItem` scope. Non-array collections iterate zero times, `in` is
  evaluated once so the body cannot extend its own loop, iterations are capped
  at 10000, and the body shares the `if` depth cap and the `invoke` call cap, so
  no nesting can hang a dispatch.
- `http.*` steps: optional `"async": true` runs the request in the background.
  The dispatch returns as soon as the request is issued — so the frame at its
  boundary already shows the loading state, and the rest of the session stays
  usable for the whole round trip instead of queueing behind it — and the
  `onSuccess` / `onError` branch runs when the reply lands, publishing a second
  frame over live-sync. Inside the branch, `{{ state.x }}` reads live while the
  action's args stay frozen at dispatch time; `{{ response }}` / `{{ error }}`
  are unchanged. Defaults to `false`, so a step whose sibling steps read its
  response keeps its blocking behaviour, and the loader warns when `async` is
  written on a step that is not an `http.*` call.
- `http.*` steps: three governance fields for background requests.
  `"key": "search"` names a request slot — starting a new request on a key
  cancels whichever request is still open on it and discards that one's outcome
  entirely (no `result`/`error` write, no branch), so search-as-you-type lands
  the reply to the *last* keystroke rather than whichever round trip finished
  last. `"pending": "searching"` holds a state path `true` for exactly as long
  as the request is open — cleared on success, failure, timeout and refusal
  alike, and reference-counted so overlapping requests hold it until the last
  settles — which retires the hand-written loading-flag pair. `"timeout": 4000`
  caps one request in milliseconds instead of the shared 20s ceiling, reporting
  expiry through the ordinary error path as `request timed out after 4000ms`.
  Alongside them, a runtime now caps background work at **64** open units:
  past that a step fails immediately on its error path with
  `too many concurrent requests (64 in flight)` instead of queueing invisibly or
  leaking goroutines — the same class of self-protection as the 250ms timer
  floor. `examples/netdemo` ships all three on one search box.
- `delay` step — `{"type":"delay","ms":500}` runs the steps that follow it *in
  the same list* when the wait expires, so `render` / `delay` / `render` paces a
  staged reveal without a timer node or a second action. It never blocks: the
  wait goes to the same background sink `async` uses, and on a host with no sink
  (an offline render, an MCP simulation) it degrades to no wait at all, so the
  action still reaches the same final state. A missing or non-positive `ms` is a
  load-time error.
- Load-time diagnostic for the invisible loading state: an action that raises a
  flag, calls a backend synchronously and lowers the flag again — all in one
  dispatch, which renders one frame at its boundary, so the flag is never seen —
  is now reported, naming the three one-word cures (`render`, `"async": true`,
  `"pending"`). It fires only on the full shape, so an ordinary boolean that
  happens to precede a request is not reported.
- `qorm_activity` now carries `inflight`: the background work the app still has
  open (async `http.*` requests plus waiting `delay` steps). `0` is the
  quiescence signal — the state an agent reads is final; above zero means a
  reply is still coming and the current frame is a loading state.
- The client-side hosts — the standalone WASM runtime, the offline package that
  embeds it, and the live playground — install the same two sinks the server
  has: `window.qormApplyFrame(frame)` is the push channel, and
  `playcore.InstallSinks` wires it to `Runtime.Commit` / `Runtime.Async`. A
  `render` step's loading frame now reaches the screen in a packaged app, and an
  `http.*` completion arrives as a later frame instead of being invisible.
- `Runtime.Async` — the host-installed background work sink `"async": true`
  needs, and `Runtime.Inflight()`, the exact count of requests still open. Like
  `Runtime.Commit`, neither `runtime.New` nor `Clone` ever installs it, so a
  bare runtime, an offline render and an MCP simulation stay fully synchronous
  and side-effect free; the server installs it wherever it installs the frame
  sink. `qorm_dispatch` detaches it for the duration of an agent's call, so an
  agent always receives the settled state rather than a loading frame.
- Components: a **declaration form** for a component definition — wrap the
  template in `{"props": {…}, "slots": {…}, "template": {…}}` and the component
  states the interface it expects. A prop is a type string (`"label": "string"`)
  or an object with `type` / `default` / `required`; the types are `string`,
  `number`, `boolean`, `array`, `object` and `any`. A missing required prop or
  slot is a load-time error, a literal that cannot satisfy its declared type is
  an error, and an undeclared key inside an instance's nested `props` object is
  a warning — bindings are never type-checked here, since they are only known
  at render time. Declared prop types also join the template's expression
  checker, so `{{ prop.title * 2 }}` on a string prop is caught inside the
  component. A default is a literal from the definition and never satisfies
  `required`. The `template` key is the discriminator, so every component
  written before this is untouched.
- Components: a component may live in its own `type: "component"` document with
  its name in `id` — by convention `components/<name>.json`, though the split
  is type-driven like scenes and actions, so the file can sit anywhere.
  Component documents and the manifest's inline `components` map fill one
  registry (the manifest is read first; defining a name twice is an error and
  the first definition wins). An instance may also use the spec's `ref` form —
  `{"type":"component","ref":"panel"}`, the canonical `component://panel`
  spelling, or a binding resolved in the instance's own scope.
- `largetitle` / `sliverappbar` now actually collapse: the compact bar is
  sticky and the big title scrolls behind it, cross-fading into the compact
  title. The collapse itself needs no JavaScript and no modern CSS; the
  cross-fade rides a CSS scroll-driven animation off the main thread where the
  browser has one, and a small script drives the same declarations where it
  does not. `"collapsible": false` restores the previous static header.
- New `sheet` widget (aliases `bottomsheet`, `draggablesheet`,
  `draggablescrollablesheet`, `modalbottomsheet`) — a bottom panel dragged
  between snap points. `snapPoints` is a ladder of fractions (a value above 1
  reads as a percentage) and may itself be state-bound; `initialSnap` picks the
  opening stop; `onSnap` is registered once per stop and each registration
  carries that stop's index as the `snap` arg; `onClose`, a scrim tap and a
  fling below the lowest stop all close it, and a bound `open` gets the
  built-in dismiss for free. A falsy `open` renders nothing at all. Only the
  grab row claims the drag gesture, so a sheet can hold a list, a swipeable row
  or a reorderable list without the two fighting.
- Style: `backdropBlur` (a number of px, capped at 120) and `backdropTint` (the
  translucent fill the blur shows through) give any node the frosted-glass
  look. The shell rules they feed start from a **solid** surface, so a browser
  without `backdrop-filter` gets an opaque panel rather than an unreadable
  transparent one. `appbar` and `largetitle` are frosted by default, so there
  the key retunes an existing radius and `0` turns the frost off.
- `tabs` — bindable `active` (a plain state binding switches the widget to the
  hidden-radio idiom `segmented` already uses, so controlled tabs need no
  action file and no new JS; an out-of-range index clamps), a `scrollable` tab
  bar that scrolls its active tab into view however the tab changed, a
  styleable `indicator` / `indicatorColor` underline or pill, `onChange`
  carrying the `index` and the `tab` label of the tab switched to, opt-in
  `lazy` panels (gated on controlled mode — client-side switching can only
  reveal panels already in the DOM), and `swipe: true` to drag a panel sideways
  to its neighbour. The swipe carries no handler indices at all: it activates
  the neighbour by synthesizing that tab's own tap, so it behaves identically
  in uncontrolled, controlled and `onChange` modes.
- `table` / `datatable` — a child carrying `column: "<key>"` becomes that
  column's **cell template**, rendering widgets instead of plain text and
  scoped through the same alias machinery lists use (`{{ row.x }}`,
  `{{ rowIndex }}`, `{{ cell.value }}`, all renamed by `as`). A child with
  `detail: true` adds a native expandable row under every row.
  `stickyHeader` / `stickyTop`, `scrollX` / `maxHeight` / `minWidth` and a
  per-column `sticky` give frozen headers and frozen leading columns.
- `accordion` — `active` selects the open panel (bindable, clamped), and
  `single: true` makes the panels exclusive (opening one closes the rest);
  the default stays independent toggles. `tree` — `collapsed` starts every
  branch closed, with a data item's own `expanded` still winning for its own
  node. `timeline` — items take `color`, `icon` and `time`.
- `carousel` — `autoplay: <ms>` advances the track on a clock (floored at
  250ms, paused while pointed at or while the tab is hidden, wrapping at the
  end) and `indicators: true` renders a tappable dot row whose active dot is
  derived from the live scroll position.
- Forms: native constraint validation can now block the action. `button` gains
  `submit` — `true` emits a real submit button plus an inline validity gate, so
  a button carrying its own `onPress` inside a `form` no longer dispatches past
  a failing `required` / `pattern`, and the browser's own message bubble comes
  free; `false` emits an ordinary button that is never gated, which is what a
  Cancel button needs. It is deliberately not inferred from being inside a form
  (the renderer has no ancestor channel, and inferring it would break Cancel).
  `novalidate` on the button emits `formnovalidate`, `form` gains `novalidate`,
  and the button's gate reads the form's flag so the two switch off together.
  `textformfield` echoes its `error` into `title` / `aria-invalid` when the
  field opts into native validation, so the author's wording rides the native
  bubble, and a field the user has actually interacted with draws a red border
  natively (`:user-invalid`) — an untouched required field does not look wrong
  before anyone typed.
- `examples/places` — scroll surfaces: a `largetitle` that collapses into a
  sticky compact bar, a frosted `backdropBlur` panel, and a `sheet` dragged
  between three snap points.
- `examples/derived` — a cart demonstrating the other three: `computed`
  subtotal / shipping / total, a `guard` bouncing an unauthenticated checkout
  to sign-in, and a `forEach` bulk edit.

### Fixed
- **A hard freeze in packaged apps.** An `http.*` step in the WASM build ran its
  round trip on the goroutine servicing the JS callback, which is exactly the
  self-wait the single-threaded js/wasm scheduler cannot break — the whole app
  died with "all goroutines are asleep - deadlock!", so a packaged app calling
  `http.get` was bricked rather than merely slow. Those hosts now set
  `Runtime.AsyncAll`, so every request takes the background sink whether or not
  its JSON opted in. **This is a user-visible semantic**: on such a host an
  `http.*` step's siblings run while the request is still open, so the steps
  that depend on the reply must go in `onSuccess` / `onError`. Server-side
  semantics are unchanged — the server never sets the flag.
- `qorm package` produced a bundle whose components all rendered as unknown when
  they were declared in cross-file `type: "component"` documents: `bundle.Build`
  dropped them. The field is `omitempty`, so existing bundle bytes and content
  hashes are unchanged.
- **A top-level state key named `state` silently emptied every binding in the
  app.** Action and derived-value contexts expose each top-level state key under
  its bare name (so `{{ count + 1 }}` means `{{ state.count + 1 }}`), and those
  bare names were written OVER the `state` / `t` / `viewport` roots rather than
  under them. A key called `state` therefore repointed `{{ state.x }}` at
  itself: every derived value collapsed to nothing, on every frame, with no
  diagnostic anywhere and no way for the app to recover. The reserved roots now
  always win — over state keys, over action args and over an async
  continuation's frozen bindings — and the colliding key stays readable under
  its bare name. The mis-rooted step path that creates such a key in the first
  place (`{"path": "state.count"}`, copied from the `{{ state.count }}` two
  lines up in the scene) is now a load-time **warning**, and the spelling
  `state.computed.<name>` is refused as a write into the read-only derived
  namespace exactly as the bare `computed.<name>` already was.
- **One frame per revision, everywhere.** A deep link (`GET /?scene=X`), a
  `/poll` catch-up, an SSE catch-up, an MCP `qorm_check_layout` viewport write,
  a first page load whose entry scene's `onEnter` contains a `render` step, and
  a read-only MCP tool draining that hook before the browser loads could each
  re-render and overwrite the handler table of an already-published revision
  without bumping it — so a second tab honestly reporting that revision
  dispatched actions from the *other* scene (deterministically, no race), and
  a first page could ship buttons whose handles were never published, their
  clicks silently swallowed. Every mutation now bumps the revision before
  publishing; the handler ring refuses a different table at the same revision
  (keeping the published one, with a log); and the first-load / agent-drain
  paths re-bump when draining advanced the revision. Guards:
  `TestSameRevIsNeverRenderedTwiceDifferently`,
  `TestFirstLoadOnEnterRenderStepKeepsPageButtonsLive`,
  `TestAgentReadToolDrainLeavesFirstPageButtonsLive`.
- **The handler ring was smaller than the frame budget.** `handlerHistory` (8)
  vs `MaxFrames` (64) meant one `forEach` + `render` could evict a live
  revision eight times over, silently. The ring is now derived as
  `MaxFrames + 1`, an eviction that actually bites a client is logged, and
  async continuations share the dispatch's frame budget instead of resetting it
  — one click costs at most 64 intermediate frames, continuations included.
- **`computed['x']` produced no dependency edge.** The derived-value dependency
  scan was a string match that missed bracket spellings, so a real cycle
  through `computed['a']` was reported acyclic and silently published garbage
  with zero diagnostics. Reference extraction is now a token-level scan
  mirroring the expression lexer — brackets, whitespace variants, mixed
  spellings and predicate string literals all produce edges. Guard:
  `TestComputedRefsSeesBracketSpelling`.
- **The WASM host executed `onEnter` at different times than the server.** The
  wasm frame sink drained pending enter hooks on every `render` step while the
  server drains only at dispatch/entry boundaries, so the same JSON ran in two
  orders. The wasm host now mirrors the server's `bump()`/`frame()` split:
  render steps never drain, entry/dispatch boundaries drain exactly once.
- **`examples/tasks` and `examples/netdemo` showed the async pattern the
  packaged host breaks.** `saveTask`'s failure rollback and `getFact`'s
  loading flag were sibling steps of a blocking-looking `http.*` — correct on
  the server, skipped or instantly overwritten on AsyncAll hosts (packaged /
  offline / playground). Both actions now use the governed form (`async: true`
  with `pending`), and `TestExampleActionsConvergeUnderAsyncAll` proves their
  settled output is byte-identical under both host semantics.

### Changed
- The determinism guard grows a third tier: instantaneous determinism (two
  renders of the same state agree byte for byte) and confluence under
  intermediate frames are now joined by quiescent determinism — once
  `Inflight()` reaches zero, an async action must render byte-identically to
  the same action written synchronously. Async may change the sequence of
  frames a dispatch produces; it may never change where the dispatch lands.
  The example sweep behind it widens too: it now renders **every** scene of
  every example (not only the entry one), three times rather than twice, and
  re-checks the output after each of the app's actions has been dispatched —
  the states where timers, intermediate frames, derived-value refreshes and
  `forEach` actually exist.
- **Rendered output changes.** Three of the additions above alter the HTML an
  unchanged app produces. Twenty-three of the twenty-seven examples are
  byte-identical to v0.4.0; these are the ones that are not.
  - `image` now emits `loading="lazy"` by default. Off-screen images are no
    longer fetched on first paint. Opt out per node with `"lazy": false`.
  - `width` / `height` given as a **string** now take effect. `"100%"`,
    `"30vw"`, `"40vh"` and `"120px"` were previously parsed as a number,
    failed, and were dropped silently; they are now parsed and re-emitted as
    the corresponding CSS length. An app that carried such a value was being
    laid out by the fallback and will now be laid out as written —
    `examples/floating`, whose stage is `"width": "100%"`, changes shape.
  - `input.maxLength` (and `textarea` / `textformfield`) now reaches the DOM as
    `maxlength`, so the field **actually truncates typing** instead of merely
    documenting an intent. `examples/components`' email field caps at 40
    characters where it previously accepted any length.

## [v0.4.0] - 2026-07-26

### Added
- Execution model: `if` action step — `{"type":"if","condition":"{{ … }}",
  "then":[…],"else":[…]}` branches on the expression language's truthiness
  and nests (depth-capped at 32, enforced at both load and dispatch time).
  The loader diagnoses a missing `condition` and warns when the condition
  carries no `{{ … }}` binding (a constant would always take `then`).
- Execution model: `invoke` action step — `{"type":"invoke","name":"other",
  "args":{…}}` calls another action, evaluating `args` in the caller's
  context and merging them into the callee's scope (the same semantics as an
  event invoke's args). Call depth is capped at 16, so recursive or
  mutually-recursive actions terminate; the loader diagnoses a statically
  unknown target action.
- `http.*` steps: optional `onSuccess` / `onError` step lists run
  synchronously after the request returns. `onSuccess` sees the decoded
  response as `{{ response }}` (the `result` state path is written first);
  `onError` sees the failure message as `{{ error }}` (the classic `error`
  state-path write is preserved and happens first).
- Declarative timers: an invisible `timer` node —
  `{"type":"timer","id":"poll","every":5000,"onTick":{"name":"refresh"}}`
  (or `after` for a one-shot) — schedules client-side ticks that dispatch
  through the exact same `/event` (server) / `qormEvent` (WASM/playground)
  chain as a button press. The scheduler reconciles markers after every
  morph, so re-renders are idempotent (same id never double-scheduled), an
  `if` condition controls the timer's existence (a countdown stops itself),
  and `every` is floored at 250 ms (loader warning + render clamp) to
  prevent self-inflicted event floods.
- Scene lifecycle: a scene-level `"onEnter": {"name":"loadData","args":{…}}`
  (or string shorthand) dispatches once each time navigation enters the
  scene — including the entry scene's first load, deep links straight into a
  scene, and navigate-back re-entry. A page refresh of the live session, an
  SSE reconnect, and a dev hot-reload never replay it; onEnter chains that
  navigate are capped at 8 hops and a self-navigate cannot loop.
- `examples/lifecycle` — a small app wiring all of the above together:
  onEnter data load, a poll timer, an `after` one-shot hint, an
  `if`-guarded countdown timer, and a form submit with if/else branches
  plus an `invoke` reset.
- Intermediate rendering: a `render` action step — `{"type":"render"}` —
  publishes the state written so far as a frame, mid-action, so a loading
  flag set before a slow step (`set saving=true` → `render` → `http.put` →
  `set saving=false`) actually reaches the screen. It is a no-op on a host
  that installs no frame sink, so the same JSON runs unchanged everywhere;
  `runtime.New` and `Clone` never install one, which keeps bare runtimes and
  MCP simulations synchronous and side-effect free. Frames are capped at 64
  per top-level dispatch (nested `invoke`s share the caller's budget), and
  the loader warns past 8 `render` steps in one action. An intermediate frame
  deliberately does not drain a pending `onEnter`, so a scene entered
  mid-action still fires its hook exactly once, at the dispatch boundary.
  `examples/tasks` now uses it: its "Saving…" indicator was unreachable
  before and now shows a spinner for the duration of the save.
- `POST /event` accepts the revision the click was rendered on
  (`{"h":…,"rev":…}`) and resolves the handler index against that frame's
  table, keeping the last 8 frames. Handler indices are positional, so a
  frame landing between the paint and the click — an agent edit, or an
  intermediate frame — previously renumbered them and could dispatch a
  different action than the one pressed; intermediate frames widen that
  window from milliseconds to the length of a whole action. A request that
  reports no revision (curl, the offline WASM driver) keeps the previous
  newest-table behaviour.
- Components: props are now evaluated in the instance's scope with type
  fidelity, so `{{state.x}}` / `{{item.x}}` / route params resolve instead of
  leaking as literal text, and a single whole-string binding keeps its Go
  type — a boolean stays a boolean (`if="{{prop.open}}"` works), a number
  stays a number (`{{prop.count * 2}}` works), a list stays a list
  (`data="{{prop.rows}}"` works). This is what makes a component usable as a
  list-row template.
- Components: callback props — an invoke's `name` interpolates at handler
  registration time, so `"onPress":{"name":"{{prop.onConfirm}}"}` dispatches
  the action the instance passed in instead of silently no-op'ing on a
  literal lookup.
- Components: named slots — a template's `slot` node takes a `name`, and an
  instance child is routed to it with a `slot` field; children with no `slot`
  fill the unnamed slot. A slot's own `children` are its fallback, rendered
  only when nothing fills it. A single unnamed slot keeps its previous
  behaviour, so existing components are unaffected.
- Components: the nested `"props":{…}` instance syntax now works (one level,
  merged in the renderer so the serialize round-trip stays identity); nested
  keys win over top-level keys of the same name.
- Expressions: postfix index access — `arr[i]`, `obj[key]`, `grid[1][0]`,
  `users[0].name`, and a dynamic key or index from any expression. Semantics
  are lenient: out of range, a missing key, or indexing a non-collection is
  `null` rather than an error. A negative literal index is `null` — only
  `at()` counts from the end.
- Expressions: 13 collection and formatting builtins — `at`, `first`, `last`,
  `sum`, `avg`, `count`, `join`, `split`, `keys`, `values`, `map`, `filter`,
  `format`. `map` / `filter` / two-argument `count` take a sub-expression
  written as a string with the element bound to `it`
  (`sum(map(state.cart, "it.price * it.qty"))`), evaluated in an isolated
  context with a 256-deep stack guard and a 100k-evaluation work guard so
  self-referential data cannot hang a render. `keys` / `values` sort by key
  for render determinism. The loader statically checks arity and per-argument
  types for these calls and recursively checks literal sub-expressions.
- Lists: the item scope of `list` and `gridview` now carries `index`, `first`
  and `last` alongside `item`, as separate scope names — a data field called
  `index` is never shadowed (`{{item.index}}` is your data, `{{index}}` is the
  loop position).
- Lists: `"as":"row"` binds the item under an alias with namespaced
  `rowIndex` / `rowFirst` / `rowLast`, so a nested list keeps the outer item
  reachable (`{{section.title}}` next to `{{row.name}}`). A reserved or
  malformed alias degrades to `item` with a loader warning, and the loader's
  static checks learn each template's scope names token-exactly.
- Lists: built-in pagination — `pageSize` (+ 1-based `page`, typically bound
  to state) renders one window of the data. `page` clamps into range, so an
  overshooting page shows the last one instead of a blank screen.
  `gridview` takes the same pair. `index` / `first` / `last` stay global to
  the full data, so numbering counts across pages and node ids stay stable.
- Lists: `separator` draws a hairline between items — `true`, or
  `{"height":…,"inset":…,"color":…}` — never after the last item and never
  before a section header.
- Lists: `groupBy` splits consecutive runs of an equal key into sections with
  a `sectionHeader` label evaluated in the first item's scope; `sticky`
  (default true) and `stickyTop` park the header under an existing appbar.
  The renderer never reorders the data.
- Lists: `onRefresh` exposes pull-to-refresh inline, reusing the
  `refreshindicator` gesture verbatim (one shared implementation, byte-
  identical output). Grouping does not wire reorder and reorder does not wire
  refresh — both combinations would misindex or fight over the same drag.
- Bound node types: `"type":"{{item.kind}}"` resolves against the current
  scope at render time, so one list template renders a different widget per
  row instead of stacking `when`/`if` gates. Resolution happens after the
  visibility check (a hidden node pays no evaluation) and before the
  animation wrap, and returns a shallow copy so the shared template node is
  never mutated. An unresolvable or unknown result keeps the raw binding as
  the type and degrades through the existing unknown-node path, with the
  diagnostic naming the offending expression.
- Inputs: `input` / `textarea` / `textformfield` accept `maxLength`,
  `autofocus`, `readonly`, `required`, `autocomplete` and `inputMode`
  (normalizing `number`→`numeric`, `phone`→`tel`); `pattern` on `input`.
  These are the browser's native constraint channels — there is deliberately
  no JS validation engine, and they do not block an action dispatch.
- Images: `placeholder` (a CSS color, or a low-resolution URL painted as a
  blur-up background), `fallback` swapped in by a self-stopping `onerror`,
  and native lazy loading **on by default** (`"lazy":false` opts out).
- Style keys: `zIndex`, `alignSelf`, `flexShrink`; `width` / `height` now
  accept `"50%"`, `"30vw"`, `"40vh"` and `"120px"` (parsed and re-rendered,
  never passed through verbatim).
- Pseudo-state style keys: `hoverBackground`, `hoverColor`, `hoverOpacity`,
  `pressedScale`, `pressedOpacity`, `focusBorderColor`, `disabled` and
  `disabledOpacity`. The renderer publishes a CSS custom property inside the
  node's own escaped style attribute and the shell stylesheet carries fixed
  rules that consume it — no JavaScript, morph-safe, and values still go
  through the normal style-attribute escaping. Hover rules sit inside
  `@media (hover:hover)` so a tap cannot strand a hover state, and `disabled`
  also emits `aria-disabled`.
- `docs/expressions.md` (+ Chinese mirror) — a reference page for the
  `{{ … }}` language: scopes, operators and truthiness, index access, and
  every builtin function.

### Fixed
- `image`, `avatar` and `video` now interpolate `src` (and `image`'s `alt`),
  so a data-driven row bound to `{{item.src}}` loads the image instead of
  emitting the literal binding as its URL — exactly the image-message case a
  polymorphic feed needs.
- The semantic alias containers `center` / `start` / `end` / `between` /
  `around` / `evenly` / `stretch` now mean what they say: `center`/`start`/
  `end` pack both axes, `between`/`around`/`evenly` set `justify-content`,
  and `stretch` aligns. Previously they fell through to a plain column and
  silently did nothing, which read as a renderer bug. An explicit
  `layout.align` / `layout.justify` still wins. No app in the repo used these
  types, so nothing renders differently.

### Changed
- `examples/dragdrop` is rewritten data-driven: one `gridview` with
  `as:"column"` and a single parameterized move action replace three
  hand-copied columns and three duplicate actions.

## [v0.3.7] - 2026-07-26

### Security
- OTA SSRF guard on the live server's `POST /update` source fetch: private
  (RFC 1918 / RFC 4193), link-local (including the `169.254.169.254` /
  `fe80::` cloud metadata addresses), multicast and unspecified
  destinations are refused, with the check enforced at dial time after DNS
  resolution — so redirect hops and DNS rebinding cannot route around it —
  and redirects capped at 5 http(s)-only hops. Loopback destinations and
  local file paths stay allowed (local-dev bundle servers keep working).
  Implemented as `ota.BlockPrivate()`, wired only into the `/update` fetch.
- MCP tools no longer silently degrade on malformed arguments: 8 tools
  (`qorm_window`, `qorm_query`, `qorm_get_node`, `qorm_source_location`,
  `qorm_simulate_action`, `qorm_dispatch`, `qorm_check_layout`,
  `qorm_assert`) now return a tool error when arguments are present but
  unparseable, instead of decaying to zero-value behaviour — a malformed
  `qorm_assert` previously reported overall pass on an empty check list,
  and a malformed `qorm_query` matched every node. Absent or `null`
  arguments still mean all-defaults.

### Fixed
- Runtime `{{...}}` binding splitting now uses the same quote/escape-aware
  close-delimiter scan as the loader (extracted as `expr.CloseIndex`), so a
  binding whose string literal contains `}}` — e.g. `{{ '}}' }}` — evaluates
  correctly at runtime instead of being mis-split by the old regex.
- Loader: a `when` node whose `condition` is a non-empty string without a
  `{{...}}` binding now emits a load-time warning — such a condition is a
  constant (any non-empty string is truthy), so the `then` branch would
  render unconditionally and the `else` branch would be dead; it is almost
  always a forgotten `{{ }}` around the expression.
- Loader: the "value is misconfigured" diagnostic no longer fires for the
  ~40 widget types that genuinely consume `value` as their bound-state or
  data API (`segmented`, `progress`, `rating`, `stat`, `controlTile`, …) —
  previously only 4 input widgets were exempt, producing 49 false-positive
  warnings across the shipped examples. Misuse on plain nodes (`text`,
  `view`, `button`, …) still warns, and a new integration guard pins every
  example to zero load-time warnings.
- `examples/floating`: the toggle action was missing `"type": "action"`
  (dropped on the docs compile path) and used `state.toggle` on a scalar
  bool — an array-only step, so the button did nothing. It now flips
  `running` via `state.set` with `{{ !state.running }}`.
- `examples/login`: the sign-in button invoked a `performLogin` action that
  did not exist; the action now ships with local-state feedback.
- `qorm shot` (macOS): captures no longer race window occlusion — an
  unbundled CLI's capture window never reaches
  `NSWindowOcclusionStateVisible`, so WKWebView deferred first paint
  indefinitely and some pages were captured as all-white. The capture
  window is now presented above all Spaces with WebKit occlusion detection
  disabled (SPI guarded by `respondsToSelector`), and page load failures
  are reported to stderr instead of silently writing a white PNG.

### Changed
- CI additionally runs the full test suite under the race detector
  (`go test -race ./...`).

## [v0.3.6] - 2026-07-24

### Added
- Live WASM playground: `qormCompile(docsJSON)` + `qormSetViewport`
  exports re-render from edited source through the in-browser runtime on
  a 200ms debounce, with a diagnostics strip and counter / todo / form /
  nav templates. The doc-array → {html, diagnostics, unknown} core is
  pure, host-testable `internal/playcore`; the page ships via deploy
  under the gitignored `web_server/site/playground/`.
- Docs screenshots + per-page SEO: the stdlib-only mdsite now emits
  `<img loading=lazy decoding=async>`, copies non-md assets, and parses
  front-matter (title / description / og-image); every docs / api page
  gains a meta description + OG + Twitter + canonical + hreflang +
  JSON-LD; the render-blocking font `@import` becomes a preconnect +
  stylesheet link; new captures (web PWA, two-pane /console,
  first-scene) plus the existing iOS / DevTool shots are embedded across
  the tutorials / platforms / agent / verification pages.
- `sitemap.xml` + `robots.txt` generated from the staged tree at deploy;
  a 1200x630 social card backs the site-default og:image / twitter:image.

### Changed
- Quality guardrails (no product change): `scripts/release.sh` archives
  the CHANGELOG at tag time and refuses an empty / missing /
  already-archived section; an `internal/integration` coverage gate
  enforces per-package floors via `go test ./...` (and gates releases);
  persistent go fuzz targets cover the expr / loader / bundle parse
  surfaces.

### Fixed
- `loader.FromDocs` no longer silently drops a no-`type` / unknown-`type`
  doc; it emits an `error:` diagnostic.

## [v0.3.5] - 2026-07-23

### Security
- Renderer attribute-injection regression lint: a new
  `internal/integration` test (`TestAttrInjectionLint`) statically scans
  every `fmt` verb interpolated into an HTML attribute in the renderer and
  fails CI unless the value is escaped by a known helper, is a constant, or
  is on a comment-justified allowlist — with a self-proof subtest that
  plants a raw interpolation and asserts it is flagged. This puts the
  attribute-injection class (the bug behind the v0.3.3 / v0.3.4 fixes)
  under a permanent static lock; the current tree scans 448 interpolations
  as 428 safe-by-rule + 20 allowlisted + 0 unsafe.
- Self-update trust chain completed: the `go install @latest` path now
  checks the installed binary's reported version (a compromised / stale
  module proxy resolving `@latest` to an older module no longer reports
  success), and the signed-binary path checks the downloaded binary's
  version *before* swapping it over the running one (a stale-but-validly-
  signed binary is refused while the old binary is still in place).
  `go install` into a `GOBIN` / `GOPATH/bin` other than the running
  binary's dir is now reconciled instead of falsely failing closed.

### Added
- 8 new loopback self-update subtests (executable-stub payloads, no network
  / toolchain) plus the lint test; no existing test weakened.

## [v0.3.4] - 2026-07-23

### Security
- Renderer injection hardening completed: the style-attribute breakout
  (author / bound style values interpolated raw into the quoted `style="…"`)
  and the SVG `fill=` / `stroke=` breakout (chart / circular-progress
  colours) are now entity-encoded — closing the last of the attribute-
  injection classes alongside the `id=` / `name=` / `data-scene` /
  `data-title` / `data-body` escaping, the `</script>` close-tag
  neutralisation and the `safeURL` href scheme allowlist from v0.3.3.
- Cross-origin leaks on the live server: `/events` (an `EventSource` does
  not enforce CORS, so any page could read the live UI) and the
  unauthenticated `/measure` sink (CSRF / DNS-rebind write poisoning of the
  agent's layout decisions) are now behind the cross-origin guard.
- Self-update downgrade gap: `qorm update` decided "update available" by
  version equality only, so a compromised or misconfigured release endpoint
  serving an OLDER validly-signed release would have been installed as a
  "self-update" — a rollback to a stale, potentially vulnerable build. The
  gate now parses both versions with a dependency-free semver compare
  (numeric `X.Y.Z` core, optional leading `v`, a prerelease of the same
  `X.Y.Z` sorts older than the release, build metadata ignored) and installs
  only a STRICTLY NEWER release; equal, older, and unparseable remote tags
  are refused without replacing the binary and exit 0.
- `qorm verify --revoked` without `--trust` silently skipped the revocation
  check yet reported the bundle as revocation-verified; it now fails closed.

### Fixed
- Server live-sync: SSE reconnect now honours `Last-Event-ID` (catch-up
  snapshot when behind) and a first-connect revision handshake closes the
  lost-mutation window between page render and `EventSource` open; frames
  carry `id: <rev>`. The audit hash chain now covers the display time field
  (a timestamp edit was previously undetectable); a deep-link no longer
  leaks a navigation direction into the next broadcast; `/measure` rejects
  empty / non-JSON bodies with `400`.
- Runtime correctness: descending `state.sort` was unstable (an invalid
  `!less` comparator reversed equal-key runs); a nested ICU plural `#`
  resolved to the outer argument; `state.clear` on a boolean yielded `""`
  instead of `false`; an `http.*` step with a structured (map / list) body
  sent Go `%v` syntax instead of valid JSON.

### Added
- Regression / adversarial coverage for all of the above; the round-7
  top-up returns `internal/loader` and `internal/bundle` to 100%.

## [v0.3.3] - 2026-07-23

### Security
- Stored / reflected XSS in the renderer: node `id`s were interpolated
  into the `id=` attribute (and notification `data-title` / `data-body`)
  with Go `%q` but no HTML escaping, so a `"><script>…` id or title broke
  out into live markup. All 123 `id` sites now route through an
  `html.EscapeString` helper, and the notify data attributes are escaped
  too. Transparent to clients — the browser decodes entities back into
  `element.id`, safe ids render byte-identically, and handler tables are
  index-based.
- Bundle revocation failed open: `LoadRevocation` accepted `null` / `{}`
  / `{"revoked":null}` / foreign JSON as an *empty* revocation list, so a
  hijacked revocation endpoint could silently re-enable a revoked signing
  key. It now fails closed; only `{"revoked":[]}` (and the array
  shorthand) is a valid "nothing revoked" list.

### Fixed
- Loader round-trip data loss (MCP patch / bundle re-packaging):
  `NodeToJSON` dropped `data` without a template; `ActionToJSON` dropped
  navigate `to` / `back` / `from` / `params`; `ManifestToJSON` dropped
  branding / designTokens / pluginABI / widgets / menu / tray and zeroed
  window dimensions on a direct `FromDocs` round trip; a missing entry
  scene produced no diagnostic; `forEachExpr` mis-split `}}` inside string
  literals into a false binding warning.
- Expression parser: malformed number literals (`1.2.3`) silently
  evaluated to `0`, and unterminated strings were accepted; both now error.
- `colWidth(" 50 ")` emitted invalid ` 50 px`; now `50px`.

### Added
- Test coverage + adversarial bug-hunt for the parse / validate / sign /
  render core (`internal/expr` → 100%, `internal/render` → 99.5%,
  `internal/bundle` → 99.2%, `internal/loader` → 95.6%; 134 new tests) —
  the round that surfaced every fix above.

## [v0.3.2] - 2026-07-22

### Fixed
- Data race in the OTA update path: `serveUpdate` read the current
  bundle/trust flag without the server lock (confirmed with `-race`);
  it is now snapshotted under the lock.
- `/mcp` returned a silent `204` on an unparseable request body; it now
  returns a JSON-RPC `-32700` parse-error with HTTP `400`, and the stdio
  server emits the spec-required parse-error line.
- `/presence` with malformed JSON now returns `400` (matching `/event`
  and `/viewport`), and its focus truncation is rune-based so multi-byte
  UTF-8 labels are never split into invalid bytes.
- Runtime: a column-less `__sort` is now a no-op instead of clobbering
  the recorded sort field; `Clone()` deep-copies the navigation stack so
  simulation clones can navigate back; negative amounts format as
  `-$1,234.50` (CLDR sign placement); `http.*` steps store the response
  body only on a 2xx success.

### Added
- Test coverage for the server / runtime / mcp core (`internal/server`
  64.9% → 98.3%, `internal/runtime` 66.4% → 100%, `internal/mcp` 67.0% →
  99.4%; 129 new tests) — the round that surfaced every fix above.

## [v0.3.1] - 2026-07-22

### Added
- Test-coverage push (115 new tests / 13 files): `internal/render`
  15.2% → 85.4% (widgets, handlers, components, soft-fail),
  `internal/measure` 55.2% → 100% (audit bounds, eval checks, report),
  `pkg/qormext` 37.0% → 100% (ABI compat, `Emit`/`jsStr` escaping),
  `cmd/qorm` 16.9% → 42.3% (CLI dispatch, sign/verify flow, helpers).
- `TestAPIRefCLI` contract test: guards the hand-written `api/cli.md`
  (EN + ZH) against the `cmd/qorm` implementation — every documented
  flag and subcommand must exist in the code, and the two language
  pages must document the same subcommand set.

### Changed
- `.gitignore` covers optimized WASM outputs (`qorm_optimized.wasm[.gz]`)
  and `*-web-opt/` package directories.

### Fixed
- `qorm run --tls` always failed: the self-signed certificate's
  `NotAfter` was the year 36812 (an unencodable ASN.1 GeneralizedTime);
  certificates are now valid for +10y, covered by a real-handshake
  test.
- `qorm check` / `qorm_check_layout` fail loud: a `within`/`below`
  target id that was not measured now fails as not-found (was: a
  silent pass), and an unrecognised assertion key (a typo such as
  `visble`) fails instead of being ignored into a vacuous pass. The
  MCP tool descriptions carry the same note.
- qormext: the native bridge's `jsStr` escaped only quote and
  backslash; it now escapes newline, CR, tab, BS/FF, all C0 controls,
  and U+2028/U+2029.
- Hardware-widget default button labels (camera, notification,
  location, motion, audio recorder) no longer hardcode emoji — the
  defaults now render the built-in SVG icon set via a shared
  `iconLabel` helper; an app-authored `label` still renders as plain
  text.
- The camera's live-capture button no longer wipes its SVG icon when
  its label switches Retake/Capture (the client now replaces just the
  label text).
- `qorm check` prints `[--width N]` in its inline usage (the flag was
  parsed but absent from the usage line).
- Docs: `api/cli.md` (EN + ZH) gave the wrong default output for
  `qorm docs` (`site/`, which clobbers the hand-kept landing copy —
  correct: `docs-site/`), and the `qorm` usage text now matches the
  implemented flags (miniapp platform, `--version`, `--width`,
  `--eval`, the `shot` flags, `--name`, optional `[bundles-dir]`).

## [v0.3.0] - 2026-07-20

### Added
- `searchbar` widget: SearchBar + anchored results panel — declarative
  `items` (literal or bound), client-side label filter, `onSelect` emits a
  plain label string.
- `segmented` `multiple: true` (ToggleButtons): selection is a plain array
  in state, membership via `state.toggle`.
- `table`/`datatable` column widths (`width` column key, emitted as
  `<colgroup>` only when used).
- Overlays ship default actions: a plainly state-bound `open` on `modal` /
  `actionsheet` closes on backdrop tap and Escape, and an un-wired
  cancel-style button on `actionsheet` / `alertdialog` closes by default —
  all via the runtime's built-in `__dismiss` action (works over server, WASM
  and MCP). Opt out with `dismissable: false`; an explicit `onPress` always
  wins.
- `timepicker` widget (alias `cupertinotimepicker`): iOS hour/minute wheels,
  `value` "HH:MM" with `minuteStep`, dispatches the plain time string.
- `ignorepointer` / `absorbpointer`: layout-transparent behavior wrapper
  (`display:contents` + `pointer-events:none`) — the subtree becomes inert
  with zero layout impact.
- `menu` accepts a declarative `items` prop ([{label, icon, disabled,
  onPress}]) alongside arbitrary children — PopupMenuButton complete.
- `autocomplete` `options` accepts a bound array (`{{state.suggestions}}`)
  in addition to a literal list.
- `slice(array, start, end)` expression builtin — expression-linked paging
  between `pagination` and `datatable` (no baked-in coupling).
- `state.reset` action step: restore one path (or all) to the manifest's
  initial state — form reset recipe.
- DataTable recipes in `examples/components`: row-select/select-all/sort +
  paged windows; datepicker-in-modal dialog recipe; `state.toggle` now
  toggles scalar membership in arrays.
- `qorm.com/demo` — the counter as an offline WASM PWA linked from the
  landing page; README links it too.
- Tests for `internal/model`, `internal/keys`, `internal/ota` (previously
  none), incl. the OTA verify-before-activate guarantees.

### Fixed
- Audit bounds in `qorm check --audit` come from the measured `#qorm-root`
  (was: hardcoded 400px fallback when the scene root wasn't id'd `root`).
- `Node.Prop` is nil-receiver safe.
- Docs extractor documents props read via `boundArray` (datatable/table/
  tree/bottomnav/steps rows were `—`).
- Examples animations/payment migrated to theme variables (all 27 examples
  now follow OS dark mode).
- OTA payloads over the 32 MiB cap now fail with an explicit error instead of
  being silently truncated; the file source enforces the same cap.
- Widget style pass (Apple HIG): tabs are real iOS underline tabs (accent
  indicator, secondary inactive, 44px targets); tree is a Finder outline
  (rotating chevron, indent, hover fill); table/datatable headers dropped
  the gray fill + grid borders for hairlines, and every sortable column shows
  a faint chevron at all times (discoverable without hover — touch-friendly),
  deepening on hover, with the sorted column's persistent accent chevron
  (hover effects pointer-devices only).
- The DevTool (logwindow) and collaboration console are multilingual
  (English · 中文 · 日本語 · 한국어 · Español · Français · Deutsch): a
  header language picker, persisted in the shared `qorm-lang` localStorage
  key — the same preference the website and docs use — defaulting from
  navigator.language.
- Sortable table/datatable headers toggle asc/desc by default via the
  runtime's built-in `__sort` action (clicking the sorted column flips
  direction; a new column starts ascending). `sortData` names an explicit
  bound array when `data` is a sliced window.

## [v0.2.6] - 2026-07-19

### Added
- Manifest `designTokens` now render as stage-scoped CSS variables
  (`color.primary` → `var(--qorm-token-color-primary)`), so scenes can style
  against the declared palette — in the live server, the offline/WASM package,
  and the miniapp export. Palette source-of-truth consolidated in
  `internal/render/theme.go`.
- The default theme is now `auto`: the Apple palette follows the OS
  light/dark setting via `prefers-color-scheme`. An explicit manifest
  `theme` or `state.theme` (including `"apple"`) opts out.
- Loader warns about unknown `style` keys (the renderer silently ignored
  them before). The valid key set is exported as `render.KnownStyleKeys`.
- `api/cli.md` — a full CLI reference page (EN + ZH).
- `CHANGELOG.md` (this file).
- Screenshots: `scripts/shoot-ios.sh` reshoots example apps in the iOS
  Simulator; gallery/showcase shots added, counter/dashboard reshot.

### Changed
- Examples restyled onto theme variables (`var(--label)`/`var(--surface)`/
  `var(--bg)`…), so they follow the OS dark mode; gallery now dogfoods its
  own `designTokens`.
- `qorm docs` default output is `docs-site/` (was `site/`, which clobbered a
  hand-kept landing copy).
- Website: theme choice persists across pages; landing pages fixed (zh meta
  description, Inter `@import` position).

### Fixed
- iOS packager: generated `ViewController.swift` called the nonexistent
  `UIViewController.attemptRotationToDeviceIfNeeded()` — now the real
  `attemptRotationToDeviceOrientation()` API (Xcode 26 build works again).
- Dashboard example no longer overflows at phone width (`when`-switched
  grid columns).
- Docs: removed/replaced references to unimplemented commands (`qorm test`,
  `--target`, `qorm inspect/validate/preview-patch/profile`); stale
  `docs/reference/` links point at `api/`; `bundle-signing.md` rewritten to
  match the implementation.
- `scripts/verify.sh` works on macOS (no GNU `timeout` dependency).
- `qorm check --audit` bounded elements against a hardcoded 400px box whenever
  the scene's root node wasn't literally id'd `root` — six examples
  (animations/dragdrop/navigation/payment/reorder/swipe) false-failed at
  desktop widths. Bounds now come from the measured `#qorm-root` container.

## [v0.2.5] - 2026-07-10

### Added
- Demo-recording pipelines: `scripts/record-demo-headless.sh` (Docker/Chromium)
  and `scripts/record-demo-live.sh`, plus refreshed demo GIFs (live desktop
  capture of a shared human + AI session).

### Fixed
- `qorm shot --live` prefers an exact window title, so the app window and the
  DevTool window are distinguishable.
- `qorm shot` captures via Apple `screencapture` instead of the broken macOS 26
  APIs.
- Desktop: no longer crashes when setting the Dock icon on macOS 26.

## [v0.2.4] - 2026-07-10

### Added
- MCP source reverse-lookup: map a rendered node id to the `file:line` it is
  declared in.
- Accessibility: the runtime derives an accessibility tree + audit, exposed to
  the agent.
- `qorm run` filesystem hot-reload: edit a source file and the live app
  updates (parse errors keep the current app).

### Fixed
- Accessibility audit now names the flagged controls; picker and rangeslider
  emit aria attributes.

## [v0.2.3] - 2026-07-10

### Added
- Widgets: `Draggable` / `DragTarget` for free-form drag-and-drop, with a
  kanban example (`examples/dragdrop`).
- The release version is stamped into the docs/api site header.

### Fixed
- `Draggable` / `DragTarget` use pointer events instead of HTML5 drag-and-drop.
- qormext: deterministic go-api docs (unified CallJS/Native comments).

## [v0.2.2] - 2026-07-09

### Added
- URL routing + deep-linking: the address bar and the navigation stack stay in
  sync.
- Widgets: `NavigationDrawer`, `BottomAppBar`, `LimitedBox`, `IndexedStack`
  (mount all children, paint one by index), `Form` (submit-gating), `Offstage`,
  and `BackButton` / `CloseButton` built on the URL router.
- Regenerated `api/props.md` for the new back/close buttons.

## [v0.2.1] - 2026-07-09

### Added
- Navigation parameters, with a navigation/routing spec.
- qormext: plugin ABI versioning.
- MCP: design-token constraint layer for agent edits.
- Mobile: Android qrscan via CameraX + ML Kit barcode scanning; iOS orientation
  lock; Android screenrecord/videocapture.
- Release workflow: `scripts/release.sh` + `RELEASE.md`.
- Docs: standard action pattern library; docs-site icon nav with a fluid,
  landing-consistent header.

## [v0.2.0] - 2026-07-09

Trust, release pipelines, and platform depth.

### Added
- Bundle `requiredCapabilities` gate: a bundle declares the capabilities it
  needs and the runtime refuses to activate it where they are missing; MCP
  read-only mode disables mutating agent tools.
- OTA update loop for packaged mobile/PWA shells: updates are verified with
  ed25519 against an embedded trust key before activation, with tiered fallback
  to the bundled payload (`qorm package --update-url` + `--trust`, enforced as
  a pair).
- CLI self-update verifies downloaded release binaries (ed25519-signed
  checksums) before install.
- Release packaging: iOS `--release` archives/exports a distributable .ipa;
  Android `--release` signs an AAB with full-density adaptive icons; macOS
  `--release` signs with Developer ID, builds a DMG, and notarizes.
- Static expression type checking for `{{ }}` bindings; static compile
  diagnostics surfaced through MCP inspect and printed at build/run time.
- Responsive `when` node with a live viewport channel.
- Accessibility assertions in `qorm check` / `qorm_check_layout`.
- Human/agent isolation: `/event` and `/presence` bound to a page-embedded
  human token; tamper-evident, hash-chained activity audit log
  (`qorm run --audit-log`, `qorm audit`).
- Desktop native layer: Linux tray, notification click-through, and Secret
  Service storage; Windows volume (Core Audio COM), WinRT toast, and
  screenshot.
- DevTool: the log window is prefilled with buffered activity entries.

### Fixed
- Android packaging: the generated project compiles (Kotlin BOM alignment,
  JSON quote escaping in the generated Java bridge).
- Desktop: Windows notify was dead code; `speakStop` killed every PowerShell.

## [v0.1.3] - 2026-07-08

### Added
- QORM DevTool: the log window upgraded to a full developer tool with a
  Properties Inspector panel, unit + SSE integration tests.
- `qorm update` self-update command.
- Patreon links and licensing details on the landing pages and in the docs.

## [v0.1.2] - 2026-07-08

Re-tag of the v0.1.1 commit; no code changes.

## [v0.1.1] - 2026-07-08

### Added
- Gestures: swipe-to-reveal row actions (`swipeactions`) and drag-to-reorder
  lists.
- Navigation: scene navigation actions plus coordinated iOS-style page
  transitions (push/pop, parallax, depth).
- Human-AI collaboration: human presence surfaced to the agent, spatial
  attribution (flash the elements the AI just edited), hidden-field privacy
  (the agent is told a hidden field was filled, never its value), and retained
  human input visible to the agent.
- `animation` as a cross-cutting prop on widgets and components;
  `examples/animations` and `examples/payment` (coordinated complex animation).
- `qorm shot --url` capture with frozen animations.
- Generated API-reference site (props, actions, HTTP/SSE, Go package) as a
  sibling of the docs site; docs restyled to the QORM brand; Chinese docs and
  a localized landing page with a persistent language switcher.

### Fixed
- `AnimatedContainer` honours layout align/justify; `{{ }}` bindings resolve
  inside nested style objects; top safe-area inset applies to app-bar-less
  scaffolds; the navigation transition actually plays.

## [v0.1.0] - 2026-07-07

Initial release: QORM, an agent-native declarative-UI runtime in pure Go.

### Added
- A QORM app is JSON — a manifest (`qorm.json`) plus `scenes/*.json` and
  `actions/*.json` — rendered to HTML/CSS by one runtime, with examples under
  `examples/`.
- ed25519-signed, content-addressed bundles with verification, plus OTA
  delivery primitives.
- MCP agent surface (`qorm mcp`, `/mcp` on a running server) with
  collaboration tools: the agent sees the human's live actions, and "AI
  edited" presence is visible to the human.
- Packaging for web (WASM), iOS, Android, desktop, and a mini-program
  (小程序) foundation.
- Built-in SVG icon set (emoji removed from UI, code, and docs).
- English docs with a Simplified Chinese mirror (`docs/zh/`, `README.zh.md`).
- CI: cross-compiled downloadable binaries on every push; Docker execution
  environment published to GHCR.
- Render performance: cached parsed expressions and reflection-free CSS
  numeric writes in the hot path.

[v0.9.3]: https://github.com/qorm/platform/compare/v0.9.2...v0.9.3
[v0.9.2]: https://github.com/qorm/platform/compare/v0.9.1...v0.9.2
[v0.9.1]: https://github.com/qorm/platform/compare/v0.9.0...v0.9.1
[v0.9.0]: https://github.com/qorm/platform/compare/v0.8.11...v0.9.0
[v0.8.11]: https://github.com/qorm/platform/compare/v0.8.10...v0.8.11
[v0.8.10]: https://github.com/qorm/platform/compare/v0.8.9...v0.8.10
[v0.8.9]: https://github.com/qorm/platform/compare/v0.8.8...v0.8.9
[v0.8.8]: https://github.com/qorm/platform/compare/v0.5.4...v0.8.8
[v0.5.4]: https://github.com/qorm/platform/compare/v0.5.3...v0.5.4
[v0.5.3]: https://github.com/qorm/platform/compare/v0.5.2...v0.5.3
[v0.5.2]: https://github.com/qorm/platform/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/qorm/platform/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/qorm/platform/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/qorm/platform/compare/v0.3.7...v0.4.0
[v0.3.7]: https://github.com/qorm/platform/compare/v0.3.6...v0.3.7
[v0.3.6]: https://github.com/qorm/platform/compare/v0.3.5...v0.3.6
[v0.3.5]: https://github.com/qorm/platform/compare/v0.3.4...v0.3.5
[v0.3.4]: https://github.com/qorm/platform/compare/v0.3.3...v0.3.4
[v0.3.3]: https://github.com/qorm/platform/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/qorm/platform/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/qorm/platform/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/qorm/platform/compare/v0.2.6...v0.3.0
[v0.2.5]: https://github.com/qorm/platform/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/qorm/platform/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/qorm/platform/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/qorm/platform/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/qorm/platform/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/qorm/platform/compare/v0.1.3...v0.2.0
[v0.1.3]: https://github.com/qorm/platform/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/qorm/platform/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/qorm/platform/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/qorm/platform/releases/tag/v0.1.0
