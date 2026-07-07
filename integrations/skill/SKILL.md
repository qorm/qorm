---
name: qorm
description: Build, edit, and verify QORM apps — an agent-native declarative-UI runtime whose apps are language-neutral JSON (qorm.json + scenes/ + actions/). Use when creating or modifying a UI as QORM JSON, packaging it for web/iOS/Android/desktop, or driving a live QORM app over MCP.
---

# QORM skill

QORM (Queryable Object Rendering Model) runs a small JSON app: a manifest
(`qorm.json`), `scenes/*.json` (the UI node trees), and `actions/*.json` (declarative
behaviour). A pure-Go runtime renders it, signs it, and packages it everywhere.

## Write the runnable format (trust this, not the aspirational spec)

- Manifest: `{ "type":"app", "id":…, "entry":"main", "globalState":{ "schema":{…}, "initial":{…} } }`.
- Text: the `text` field (NOT `value`); bind with `{{ state.x }}` — e.g. `{ "type":"text", "text":"Count: {{ state.count }}" }`.
- Buttons: `"onPress":"increment"` (an action name; a string invokes it) — or `{ "name":…, "args":{…} }`.
- Actions (`actions/<id>.json`): `{ "type":"action","id":…,"steps":[ { "type":"state.set","path":"count","value":"{{ state.count + 1 }}" } ] }`. Step types: `state.set/increment/toggle/append/...` and `http.get`.
- Components: declared in `qorm.json` under `"components"`, referenced by a node whose `type` equals the component name; template uses `{{ prop.x }}` with a `{ "type":"slot" }` placeholder.
- Authoritative, code-generated references: the widget catalog (`docs/reference/widgets.md`) and capabilities (`docs/platforms/capabilities.md`). The JSON format spec is design-intent and diverges — prefer getting-started + `examples/`.

## Drive a live app over MCP

`qorm mcp <app-dir>` (or the `/mcp` endpoint of a running `qorm run`) exposes:
- Understand: `qorm_inspect`, `qorm_get_node`, `qorm_query`, `qorm_list_actions`, `qorm_render_html`.
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
- Package: `qorm package <app> -p web|ios|android|mac`. A custom app icon or `--no-branding` is commercial white-labeling (see TERMS.md) and prompts for a Patreon-membership confirmation.

## Don't

- Don't use `value`/`on:{press}`/`{{count}}`/`scene://` (the aspirational spec format — the runtime ignores it).
- Don't `apply_patch` without a matching `preview_patch` token.
- Don't add emoji to UI/code/docs — use the built-in SVG icon set.
