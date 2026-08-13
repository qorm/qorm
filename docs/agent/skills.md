# QORM Skills

A QORM Skill is a battle-tested workflow description for AI agents — each skill
encodes the exact steps, allowed tools, input files, and output format needed to
complete a specific QORM task reliably. Skills are shipped in
[`integrations/skill/`](https://github.com/qorm/qorm/tree/main/integrations/skill) and loaded by the agent at
session start.

## Available Skills

| Skill | What it does |
|---|---|
| `scene-authoring` | Author a new QORM scene: choose the right layout container, pick widgets from the catalog, wire state bindings with `{{state.x}}` syntax, declare actions for interactivity, and validate the result with `qorm_check_layout`. Covers the full authoring loop: scaffold → wire → verify. |
| `layout-debugging` | Diagnose and fix layout issues: measure rendered positions with `qorm_measure`, compare against expected bounds, identify overflow/clipping/z-index problems, and apply targeted patches. Uses the verification pipeline to confirm fixes. |
| `agent-patch` | Design and apply structural patches to a live app: use `qorm_query` to find target nodes, `qorm_preview_patch` to preview changes safely, `qorm_diff` to review the structural impact, and `qorm_apply_patch` to commit — with the preview token binding every apply to a prior review. |
| `platform-porting` | Adapt a QORM app for a new platform: audit capabilities with `qorm_capabilities`, add platform-specific style overrides (safe-area insets, native font stacks, platform-correct spacing), and stamp the capability manifest so the runtime gates what the platform actually provides. |
| `motion-design` | Add declarative animations and transitions: entrance `animation` + `curve`, `transition` / spring / FLIP `layoutMotion`, `fx`+`fxToken`, `timeline`+`timelineToken` (path/yoyo/onComplete), `stagger`, QSS-driven canvas filters/tint/pixelated. Games: `board` camera + `tilemap`; never tween physics `x`/`y`. |
| `host-capability-check` | Audit which hardware/native capabilities are available on the current host: camera, biometrics, GPS, accelerometer, clipboard, Bluetooth, battery, brightness control, and more — cross-referenced against the platform support matrix so the agent knows what will actually work. |
| `mobile-adaptation` | Adapt a desktop-first QORM app for mobile: responsive breakpoints (`sm`/`md`/`lg`), touch-friendly hit targets, bottom navigation patterns, safe-area handling, and viewport-aware `when` branches that swap layouts at device widths. |

## Skill Structure

Each skill is a markdown file with this structure:

```text
Goal              — the specific outcome (one sentence)
Applicable scope  — when to use (trigger phrases, app state, file patterns)
Input files       — which app files to read
Recommended tools — which MCP tools to use and in what order
Steps             — the numbered workflow
Prohibited actions — what to NEVER do
Output format     — how to report results
Permission requirements — which qorm_* tools are needed
```

## scene-authoring

Purpose: let the agent create or modify scene JSON.

Rules:
- Use the canonical widget names from the [widget catalog](/api/widgets.md).
- Prefer `column`/`row` for layout, `scroll` for scrollable content, `box` for
  card/surface containers, `board` + `tilemap` for side-scrollers (not a
  `list` of every tile).
- Wire state with `{{ state.x }}` bindings; use `if`/`visible`/`show` for
  conditional nodes.
- `onPress` names an action in `actions/`; `onChange` fires on input/select
  changes.
- Prefer QSS stylesheets (`styles/*.qss`) for shared styles; use inline `style`
  for one-off overrides.
- Validate with `qorm_check_layout` after every edit.
- Check the [widget catalog](/api/widgets.md) for available props per widget type.

## layout-debugging

Purpose: analyze and fix layout anomalies on the live app.

Steps:
1. `qorm_measure` — get every node's rendered x,y,w,h and computed styles.
2. Identify overflow: any node where `x-overflow` is true or the node extends
   past its parent's bounds.
3. Identify clipping: nodes inside a `scroll` whose box sits outside the
   viewport.
4. Check z-index: overlay widgets (drawer, menu, modal) must paint above siblings.
5. `qorm_preview_patch` → `qorm_diff` → `qorm_apply_patch` with fixes.
6. Re-measure to confirm.

Common causes: missing `scroll` wrapper, `width`/`height` too small for content,
absolute-positioned nodes overlapping, `gap`/`padding` collapsing.

## AI Usage Guidance

### Best practices for agents working with QORM

1. **Read before write.** Always run `qorm_inspect` at session start to understand
   the app's state schema, scenes, actions, and design tokens. Run
   `qorm_measure` to see the current rendered layout.

2. **Preview before apply.** Never call `qorm_apply_patch` without first calling
   `qorm_preview_patch` with the same ops — the preview token binds every commit
   to a prior review. Use `qorm_diff` to see what changed structurally.

3. **Use the widget catalog.** The [widget catalog](/api/widgets.md) is
   auto-generated from the runtime — it is always correct. Prefer canonical
   widget names; aliases work but may confuse future agents.

4. **Verify after every edit.** Run `qorm_check_layout` with assertions that
   match what the human asked for (visible, within bounds, correct type, no
   overflow). A passing verification is the only proof the edit worked.

5. **Keep the DevTool open.** The human observation window (`/logwindow` or
   `/console`) lets the human see what the agent is doing in real time. Never
   close it during development.

6. **Use QSS for shared styles.** Instead of repeating `style` blocks on every
   node, write a `styles/app.qss` with type/class/id rules. Theme variables
   (`var(--accent)`, `var(--label)`) follow OS light/dark automatically.

7. **Use qscript for complex logic.** When `steps` in an action JSON get too
   long, write an `actions/<id>.qs` file — `let`/`if`/`for in`/`while`/`fn`
   are easier to read and maintain than nested step arrays.

8. **Respect the canvas engine.** macOS default `qorm run` is the software
   canvas window (`-tags desktop` is WebView/HTML). Games WASM needs
   `-tags qorm_canvas`. Use `board` camera props + `tilemap` for large
   worlds; `playSound` / `playMusic` from qscript; do not tween physics
   `x`/`y`. Showcase: `examples/canvas-fx`, `examples/mario`.

9. **Don't guess capabilities.** Run `qorm_capabilities` to see what the host
   actually provides. A capability that exists on iOS may not exist on desktop.

10. **Self-heal from errors.** If `qorm_validate` reports issues, fix them before
    proceeding. If `qorm_preview_patch` returns `validTokens` and
    `suggestedFix`, use them.
