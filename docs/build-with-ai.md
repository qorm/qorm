# Build QORM apps with your AI assistant

QORM is agent-native: point your AI coding assistant (Claude Code, Claude Desktop,
Cursor, Windsurf, …) at it and have the AI **scaffold, edit, run, and verify**
QORM apps — then collaborate with you on a live app in real time. This is the
human's side of the workflow.

## See it first

The 60-second version: [`scripts/demo.sh`](https://github.com/qorm/qorm/blob/main/scripts/demo.sh) starts a shared session and plays a scripted set of AI edits — open the printed URL, hit record, and watch the app change live with an "AI edited" toast:

```sh
./scripts/demo.sh                 # examples/counter
./scripts/demo.sh examples/dashboard
```

## 1. Install QORM

```sh
go install github.com/qorm/qorm/cmd/qorm@latest   # puts `qorm` on your PATH
# or use the container: ghcr.io/qorm/qorm
```

## 2. Give your AI the QORM tools + skill

QORM ships a drop-in MCP server (so the AI can read, edit, and verify a live app)
and a skill (so it writes the format the runtime actually accepts). Per-agent
setup is in [`integrations/`](https://github.com/qorm/qorm/tree/main/integrations). In short:

- **Claude Code:** `claude mcp add qorm -- qorm mcp .`
- **Claude Desktop / Cursor / Windsurf:** merge the block from
  [`integrations/mcp.json`](https://github.com/qorm/qorm/blob/main/integrations/mcp.json) into your agent's MCP config.
- Point the AI at the skill
  [`integrations/skill/SKILL.md`](https://github.com/qorm/qorm/blob/main/integrations/skill/SKILL.md) (or this repo's
  [`llms.txt`](https://github.com/qorm/qorm/blob/main/llms.txt) / [`AGENTS.md`](https://github.com/qorm/qorm/blob/main/AGENTS.md)) so it uses the runnable
  format instead of guessing.

## 3. Ask it to build something

With the tools attached, ask in plain language, e.g.:

> "Scaffold a QORM habit-tracker in ./habits — a list of habits with a daily
> check-off and a streak count."

The AI writes `qorm.json` + `scenes/` + `actions/`, and can run `qorm run ./habits`
and `qorm check ./habits` to see and verify what it built.

## 4. Collaborate on the live app

Start a shared session and work alongside the AI:

```sh
qorm run ./habits          # opens in your browser; agent endpoint at /mcp
```

- You click in the browser; the AI sees your actions via `qorm_activity`.
- The AI edits over MCP; the change appears in your browser instantly, with an
  **"AI edited"** toast so you watch it happen.
- The AI's design changes are review-bound (preview → apply), and it self-verifies
  its edits with `qorm measure` / `qorm check`.

See [Human-AI collaboration](collaboration.md) for the full loop.

## Design tokens (keep the AI on-palette)

You can declare a **design-token system** in `qorm.json` so the AI's style edits
stay inside your design system instead of drifting to arbitrary colors. Add an
optional `designTokens` map — each entry is a named, typed value:

```json
"designTokens": {
  "color.primary": { "type": "color", "value": "#0a84ff", "enforce": true },
  "color.bg":      { "type": "color", "value": "#f2f2f7", "enforce": true },
  "spacing.md":    { "type": "number", "value": 16,        "enforce": false }
}
```

- **`type`** — `color`, `number`, … (values are stored as strings; `16` → `"16"`).
- **`enforce`** — the switch between a *hard constraint* and a *suggestion*.

**How it constrains the agent.** When you mark a `color` token `enforce: true`,
`qorm_apply_patch` (and its side-effect-free `qorm_preview_patch`) will **reject**
any `setProp` style op that sets a color style — `color`, `background`,
`backgroundColor`, `borderColor` — to a value that isn't one of your enforced
color tokens. The rejection is a clear error that lists the allowed values, e.g.:

```
design token violation: color "#ff0000" is not an allowed token (allowed: #0a84ff, #f2f2f7)
```

Comparison is normalized for hex case and the leading `#`, so `#0A84FF`,
`0a84ff` and `#0a84ff` all match the same token.

- `enforce: false` tokens are **advisory** — surfaced to the agent but never
  blocking.
- An app that declares **no** `designTokens` (or no enforced color tokens)
  behaves exactly as before — nothing is constrained.

The agent discovers your tokens through `qorm_inspect`, which now returns a
`designTokens` field, so it knows which values it's allowed to use before it
edits. See the [gallery example](https://github.com/qorm/qorm/blob/main/examples/gallery/qorm.json)
for a working declaration.

## Prompts that work well

- "Add a dark-theme toggle to the settings scene and verify the layout."
- "This button overflows on mobile — measure it and fix the width."
- "Turn the task row into a reusable component."
- "Package this as an installable web app."

The AI has the whole surface at hand: the [widget catalog](/api/widgets.md),
the [capabilities](platforms/capabilities.md), and the
[MCP tools](agent/mcp-tools.md).
