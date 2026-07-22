# Changelog

All notable changes to QORM are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Releases are tagged
`vX.Y.Z`; curated release notes live in the tag annotations.

## [Unreleased]

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

[v0.3.2]: https://github.com/qorm/qorm/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/qorm/qorm/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/qorm/qorm/compare/v0.2.6...v0.3.0
[v0.2.5]: https://github.com/qorm/qorm/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/qorm/qorm/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/qorm/qorm/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/qorm/qorm/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/qorm/qorm/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/qorm/qorm/compare/v0.1.3...v0.2.0
[v0.1.3]: https://github.com/qorm/qorm/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/qorm/qorm/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/qorm/qorm/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/qorm/qorm/releases/tag/v0.1.0
