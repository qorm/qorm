# Build QORM apps with your AI assistant

QORM is agent-native: point your AI coding assistant (Claude Code, Claude Desktop,
Cursor, Windsurf, …) at it and have the AI **scaffold, edit, run, and verify**
QORM apps — then collaborate with you on a live app in real time. This is the
human's side of the workflow.

## 1. Install QORM

```sh
go install github.com/qorm/qorm/cmd/qorm@latest   # puts `qorm` on your PATH
# or use the container: ghcr.io/qorm/qorm
```

## 2. Give your AI the QORM tools + skill

QORM ships a drop-in MCP server (so the AI can read, edit, and verify a live app)
and a skill (so it writes the format the runtime actually accepts). Per-agent
setup is in [`integrations/`](../integrations). In short:

- **Claude Code:** `claude mcp add qorm -- qorm mcp .`
- **Claude Desktop / Cursor / Windsurf:** merge the block from
  [`integrations/mcp.json`](../integrations/mcp.json) into your agent's MCP config.
- Point the AI at the skill
  [`integrations/skill/SKILL.md`](../integrations/skill/SKILL.md) (or this repo's
  [`llms.txt`](../llms.txt) / [`AGENTS.md`](../AGENTS.md)) so it uses the runnable
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

## Prompts that work well

- "Add a dark-theme toggle to the settings scene and verify the layout."
- "This button overflows on mobile — measure it and fix the width."
- "Turn the task row into a reusable component."
- "Package this as an installable web app."

The AI has the whole surface at hand: the [widget catalog](reference/widgets.md),
the [capabilities](platforms/capabilities.md), and the
[MCP tools](agent/mcp-tools.md).
