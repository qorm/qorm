# Human-AI collaboration on a live app

QORM's premise: a person and an AI agent work on the **same running app at the
same time**, and each sees the other. `qorm run` serves one live runtime over
three channels — a browser for the human, MCP for the agent, and Server-Sent
Events (SSE) to keep every viewer in sync.

## Start a shared session

```sh
qorm run examples/counter          # browser UI + agent endpoint at /mcp
```

- **Human** — open the printed URL. Clicks POST `/event`; the UI updates live.
- **AI** — connect over MCP: `qorm mcp examples/counter` (stdio), or POST JSON-RPC
  to `http://127.0.0.1:PORT/mcp`. It shares the *same* runtime the browser renders.

## The loop — each sees the other

- **The human sees the AI.** When the agent edits the app (`qorm_apply_patch`,
  `qorm_dispatch`, `qorm_set_state`), the change appears in every connected
  browser **instantly** over SSE, and a live **"AI edited · &lt;what&gt;"** toast
  shows who did it — you watch the AI work in real time.
- **The AI sees the human.** `qorm_activity` returns the shared activity log —
  who (human / agent) did what, in order — so the agent can respond to the
  human's clicks instead of guessing from state. The human's actions are also
  reflected in the agent's next `qorm_inspect`.

## Safe edits — review-bound

The agent's design changes are gated so a live app can't be changed unreviewed:

- `qorm_simulate_action`, `qorm_preview_patch` and `qorm_diff` run against a copy
  and never touch the live app.
- `qorm_apply_patch` commits only if it carries the `previewToken` from a matching
  `qorm_preview_patch` of the same ops — every committed change was previewed.
- `qorm_undo` reverts the last apply.

## Self-verify

The agent proves its edits against the rendered reality, not assumptions:
`qorm_measure` / `qorm_check_layout` (or the CLI `qorm measure` / `qorm check`)
render the app and report real geometry. See [verifying an app](verification.md).

## Tools at a glance

| role | tools |
|---|---|
| understand | `qorm_inspect`, `qorm_query`, `qorm_get_node`, `qorm_render_html`, `qorm_activity` |
| operate | `qorm_dispatch`, `qorm_set_state` |
| design (safe → commit) | `qorm_preview_patch` / `qorm_diff` → `qorm_apply_patch`, `qorm_undo` |
| verify | `qorm_measure`, `qorm_check_layout` |

Full reference: [MCP tools](agent/mcp-tools.md). To add QORM to your agent, see
[`integrations/`](../integrations).
