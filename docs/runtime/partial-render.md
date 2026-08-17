# Partial render

On every state mutation the server re-renders the current scene and pushes HTML
to connected clients. For simple scalar updates (e.g. a counter increment) a full
scene render is unnecessary — QORM can patch only the nodes whose bindings read
the changed state paths.

## How it works

1. **Dirty paths** — each `state.*` step in an action (`state.set`, `state.increment`, …)
   records the mutated path on the runtime (`DirtyPaths`).
2. **Dependency index** — on a full render the server builds a map from state
   paths to node ids by scanning `state.*` references in the scene tree.
3. **Patch** — on the next `bump()`, if every affected node is a *patchable*
   type (`text`, `badge`, `progress`, `icon`), only those subtrees are
   re-rendered and spliced into the cached HTML by `id`. Handlers are reused.
4. **Fallback** — viewport changes, navigation, scene patches, list `data`
   mutations, or any non-patchable widget trigger a full render and rebuild the
   index.

The client still receives a complete `#qorm-root` HTML fragment and morphs it —
no client-side patch protocol.

## Implications for app authors

- Bind frequently changing values to simple widgets when possible (`text` for
  numbers/labels) — they benefit most.
- Lists, `when` branches, buttons, and inputs always need a full render when
  their bound data changes structurally.
- Partial render is transparent: the same JSON produces the same UI; this is a
  server optimisation only.

## Agent / inspect

Partial render does not change the MCP or HTTP API surface. Use `qorm_render_html`
or SSE observation as before — the HTML is identical to a full render, just
produced faster for hot paths like `examples/counter`.
