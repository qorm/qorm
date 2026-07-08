# First Scene

A scene is QORM's UI entry point. A scene has an `id` and a root node `root`; the node tree describes the UI. Text content goes in the `text` field, and `{{ state.x }}` interpolates global state.

## Minimal scene

```json
{
  "type": "scene",
  "id": "main",
  "root": { "type": "text", "id": "hello", "text": "Hello QORM" }
}
```

## Scene with layout

Container nodes (`column` / `row`) arrange their children using `style` (padding, spacing, background) and `layout` (size, alignment).

```json
{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style":  { "padding": 24, "gap": 12 },
    "layout": { "width": "fill", "height": "fill", "align": "center" },
    "children": [
      { "type": "text",   "id": "title",  "text": "Title" },
      { "type": "button", "id": "submit", "text": "Submit", "onPress": "save" }
    ]
  }
}
```

- Use `text` (not `value`) for text; template interpolation like `"Welcome, {{ state.user }}"` also goes in `text`.
- Buttons use `onPress` to trigger actions (see [First Action](first-action.md)).
- For all available node types, see the [Widget catalog](/api/widgets.md).
