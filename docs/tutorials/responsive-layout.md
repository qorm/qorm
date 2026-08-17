# Responsive layout

QORM renders responsive UIs server-side: the browser reports its viewport over
`/viewport`, the runtime re-evaluates bindings, and SSE pushes the updated HTML.
Use the `when` node to swap between alternative subtrees (unlike `if` / `visible`,
which hide a single node in place).

Runnable reference: [`examples/responsive`](https://github.com/qorm/qorm/tree/main/examples/responsive).

## Viewport variables

Every scene binding can read:

| Variable | Type | Meaning |
|---|---|---|
| `viewport.width` | number | window width in px (0 while unknown) |
| `viewport.height` | number | window height in px |
| `viewport.orientation` | string | `"portrait"` / `"landscape"` / `""` while unknown |

The server's first frame runs before the client reports its size, so
`viewport.width` is `0` and orientation is empty — `when` conditions evaluate
**falsy** and the `else` branch renders until the client posts its viewport.

## Breakpoint variables

Declare named width thresholds in `qorm.json`:

```json
{
  "breakpoints": { "sm": 640, "md": 768, "lg": 1024, "xl": 1280 }
}
```

Omit `breakpoints` to use the same defaults (`sm` 640, `md` 768, `lg` 1024,
`xl` 1280).

Each name becomes a boolean in expressions:

| Variable | True when |
|---|---|
| `breakpoint.sm` | `viewport.width >= 640` (and viewport is known) |
| `breakpoint.md` | `viewport.width >= 768` |
| … | … |

Example — prefer breakpoints over raw width checks:

```json
{
  "type": "when",
  "id": "layout",
  "condition": "{{ breakpoint.md }}",
  "then": { "type": "row", "id": "wide", "children": [ … ] },
  "else": { "type": "column", "id": "narrow", "children": [ … ] }
}
```

You can still mix `viewport.*` and `breakpoint.*` in one expression, e.g.
`{{ breakpoint.lg && viewport.orientation == 'landscape' }}`.

`qorm_inspect` returns the effective `breakpoints` map for agents.

## Client behaviour

On resize the inline script debounces a POST to `/viewport` `{w,h}`. The server
re-renders (full render for viewport changes) and broadcasts over SSE. The client
morphs the new HTML into `#qorm-root`, preserving focus and scroll where possible.

See also: [`/viewport`](/api/http-api.md) in the HTTP API reference.
