# QORM Skills

A QORM Skill is a battle-tested instruction set for AI agents — it encodes the
runnable app format, the MCP tool surface, and the verify-loops an agent needs
to complete QORM tasks reliably. Skills are shipped in
[`integrations/skill/`](https://github.com/qorm/qorm/tree/main/integrations/skill)
and loaded by the agent at session start.

## The shipped skill

QORM ships **one comprehensive skill**: [`integrations/skill/SKILL.md`](https://github.com/qorm/qorm/blob/main/integrations/skill/SKILL.md).
It is a single markdown file with a Claude-Code-compatible frontmatter
(`name: qorm` + a trigger description) followed by the sections below. Load it
by pointing your agent at the file (or via the repo's
[`llms.txt`](https://github.com/qorm/qorm/blob/main/llms.txt), which links it).

| Section | What it teaches |
|---|---|
| Write the runnable format | The exact JSON the runtime accepts today: manifest, `text` + `{{ state.x }}` bindings, `richtext` spans, `video`, `path`, `onPress`, action steps, components — and which spec formats to NOT use. |
| Standard action patterns | Loading state, error handling, optimistic update, form validation, pagination — load-clean shapes with recipes in `docs/tutorials/first-action.md`. |
| Drive a live app over MCP | All **25 MCP tools** grouped by purpose (Understand / Operate / Window / Design / Verify / Simulate), matching [mcp-tools.md](mcp-tools.md). |
| Always self-verify | When to use `qorm_measure` / `qorm_check_layout` (live host) vs CLI `qorm measure` / `qorm check` (headless), pixel evidence via `qorm_capture_canvas` / `qorm shot`. |
| Ship it | `qorm run` / `render` / `build` / `verify` / `package`, signing flow. |
| Keep QORM updated | `qorm update` discipline before starting work. |
| Canvas engine | Default macOS canvas window vs `-tags desktop` WebView; layout, interaction, shared activity, all 146 widget types. |
| Declarative interaction & canvas effects | The full any-node style-key table: pressed/hover, transitions, FLIP, transforms, filters, masks, fx/timeline, stagger. |
| Side-scrollers and tile worlds | `board` camera props + `tilemap` baking; game motion rules. |
| Style system | QSS cascade, theme variables, gradients, the 108 known style keys. |
| Script actions (qscript) | `let`/`if`/`for`/`while`/`fn`, state writes, audio builtins. |
| Text input editing | Canvas selection/clipboard/undo/IME behaviour. |
| Icon font | The 53 built-in icon names. |
| Don't | Hard rules: no aspirational-spec format, no apply without preview token, no emoji, never skip verification. |

## Workflows encoded in the skill

The single skill file encodes these reusable workflows; each names the exact
MCP tools to call, in order.

### Scene authoring loop

scaffold → wire → verify.

1. Read `qorm_inspect` output (state schema, scenes, actions, design tokens).
2. Author scene JSON with canonical widget names from the
   [widget catalog](/api/widgets.md); bind state with `{{ state.x }}`.
3. Wire behaviour via `onPress` action names or `actions/*.json` steps.
4. Validate with `qorm_validate`, then `qorm_check_layout` after every edit.

### Layout debugging loop

1. `qorm_measure` — every node's rendered x, y, w, h + computed styles.
2. Find overflow (`x-overflow`), clipping (box outside its `scroll` viewport),
   z-index stacking of overlays.
3. `qorm_preview_patch` → `qorm_diff` → `qorm_apply_patch` the fix.
4. Re-measure to confirm.

### Review-bound patch loop

`qorm_query` (locate) → `qorm_preview_patch` (safe copy + `previewToken`) →
`qorm_diff` (structural review) → `qorm_apply_patch` (commit, token-bound) →
`qorm_undo` if wrong. Every commit is bound to a prior review.

### Shared-session collaboration loop

`qorm_activity` (what the human just did; `inflight: 0` = settled) →
`qorm_dispatch` / `qorm_set_state` (operate) → `qorm_render_html` (read the
guard-resolved result).

## Skill structure (format)

A QORM skill file follows this anatomy:

```text
frontmatter         — name + description (agent trigger)
Write the format    — the runnable JSON, with anti-patterns
Tool groups         — which MCP tools to use and in what order
Workflows           — the numbered loops above
Prohibited actions  — what to NEVER do
Verification        — how to prove an edit against the rendered reality
```

## Keeping the skill in sync

The skill references only auto-generated, code-backed surfaces — the widget
catalog (`api/widgets.md`), the MCP tool list (`docs/agent/mcp-tools.md`,
generated from `internal/mcp/tools.go`), and the capability matrix
(`docs/platforms/capabilities.md`). When QORM changes, re-read `AGENTS.md`
and the skill file after updating (`qorm update`); the skill itself is
updated in the same commit as the feature it documents.
