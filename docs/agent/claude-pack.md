# QORM Claude Pack

The Claude Pack provides a QORM workflow for Claude-style agents — oriented
around understanding, designing, and verifying QORM apps safely.

## Applicable Tasks

- Scene authoring: create or modify scene JSON with proper widget selection
- Layout inspection: measure rendered positions, diagnose overflow/clipping
- Style design: QSS stylesheets, theme variables, interaction effects
- Animation design: entrance effects, interaction transitions, motion tokens
- Agent patching: structural edits via preview→diff→apply pipeline
- Platform analysis: capability audit, platform-specific adaptations
- Documentation: explain app structure, write tutorials

## Recommended Tools

```text
qorm_inspect          — understand the app
qorm_measure          — see rendered layout
qorm_query            — find target nodes
qorm_get_node         — read node details
qorm_source_location  — find node in source
qorm_preview_patch    — preview changes safely
qorm_diff             — review structural impact
qorm_check_layout     — verify rendered result
qorm_validate         — validate against schemas
qorm_capabilities     — audit host capabilities
```

## Security Requirements

By default, the Claude Pack should not permit:

```text
qorm_apply_patch      — requires user confirmation (preview token binding)
qorm_dispatch         — requires user confirmation (mutates state)
qorm_set_state        — requires user confirmation (mutates state)
host.call             — blocked
filesystem.write      — blocked (read-only mode)
shell                 — blocked
deploy                — blocked
```

Operations with side effects require user confirmation before they can run.
`qorm_apply_patch` must carry the `previewToken` from a matching
`qorm_preview_patch` — every commit is bound to a prior review.

## Canvas Engine Awareness

When the app runs on `-tags desktop`, the agent should know:

- The app renders in a native window with a pure-Go software renderer.
- All 146 canonical widget types (see `api/widgets.md`) work identically to
  the HTML path.
- Interaction features: keyboard navigation, scroll momentum, text editing
  (selection/clipboard/undo), animated transitions, disabled dimming.
- Overlay widgets (drawer, menu, modal, tooltip) position above siblings via
  the `OverlayWidget` seam.
- Style keys: `pressedScale`/`hoverScale` with `transition: "0.2s"` animate
  smoothly; `disabled` dims non-widget nodes to 50% opacity.
- QSS stylesheets (`styles/*.qss`) cascade over theme defaults.
- QScript actions (`actions/*.qs`) support `let`/`if`/`for`/`while`/`fn`.
- Icon font: 53 icons on U+E000+, use the `icon` widget with `"icon": "name"`.

## Permission Boundaries

- The Claude Pack cannot expand the scope granted by the primary permission model.
- Even if the Pack layer permits an operation, it must still pass the platform /
  app / host policy.
- The approval relationship between `preview_patch` and `apply_patch` is
  governed by the Agent Protocol and Permission Model.

## Prompt Rules

Claude should follow these rules:

1. **Read first.** Run `qorm_inspect` and `qorm_measure` before any edits.
2. **Preview before apply.** Never call `qorm_apply_patch` without a matching
   `qorm_preview_patch` token.
3. **Verify after edit.** Run `qorm_check_layout` with assertions matching the
   human's request. A passing check is proof.
4. **Use the widget catalog.** The auto-generated [widget catalog](/api/widgets.md)
   is canonical — prefer it over memory.
5. **Respect the canvas engine.** Features work on both HTML and native paths.
   Test with `go test ./internal/render/canvas/... ./internal/widgets/...`.
6. **Don't guess.** If unsure about a widget's props, check the catalog. If
   unsure about a platform's capabilities, run `qorm_capabilities`.
7. **Keep the DevTool open.** The human observation window shows real-time
   activity.
