# Getting Started with QORM

This tutorial builds a minimal QORM app from scratch: a counter. Three files — manifest, scene, and action — and `qorm run` gets it going.

## Directory structure

A QORM app is just a directory: `qorm.json` is the manifest, `scenes/` holds the UI, and `actions/` holds the actions.

```text
my-app/
├─ qorm.json
├─ scenes/
│  └─ main.json
└─ actions/
   └─ increment.json
```

## qorm.json — manifest

Declares the app metadata, the entry scene (`entry`), and the global state (`globalState`: a schema plus initial values).

```json
{
  "type": "app",
  "id": "my_app",
  "name": "My App",
  "entry": "main",
  "globalState": {
    "schema":  { "count": "number" },
    "initial": { "count": 0 }
  }
}
```

## scenes/main.json — UI

Declare the UI as a node tree. Text goes in the `text` field, and `{{ state.count }}` interpolates global state; a button uses `onPress` to trigger an action (a string is the action name).

```json
{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style":  { "padding": 32, "gap": 16 },
    "layout": { "width": "fill", "height": "fill", "align": "center", "justify": "center" },
    "children": [
      { "type": "text",   "id": "count_text", "text": "Count: {{ state.count }}" },
      { "type": "button", "id": "inc", "text": "+1", "onPress": "increment" }
    ]
  }
}
```

## actions/increment.json — action

An action is a sequence of steps. Here `state.set` sets `count` to `{{ state.count + 1 }}` — inside `{{ … }}` is a full expression that can read global state and do arithmetic.

```json
{
  "type": "action",
  "id": "increment",
  "steps": [
    { "type": "state.set", "path": "count", "value": "{{ state.count + 1 }}" }
  ]
}
```

## Running

Point at the app directory (not a single file):

```bash
qorm run my-app          # opens live in the browser; click +1 and the count increments
```

The server hosts the app, handles button events, re-runs actions, and swaps the re-rendered UI back into the page — that is the run loop.

## Rendering a static snapshot

Render a static HTML snapshot without launching a browser (good for CI / previews):

```bash
qorm render my-app -o my-app.html
```

## Next steps

- [Widget catalog](/api/widgets.md) — every node type the renderer accepts (code-generated, authoritative).
- [Widget catalog](/api/widgets.md) — all available node types (auto-generated).
- [Capabilities](../platforms/capabilities.md) — native capabilities like camera, location, and Bluetooth.
- [User middle layer](../platforms/native-middlelayer.md) — add your own native ops with a single Go file.
- For more runnable examples, see [`examples/`](https://github.com/qorm/qorm/tree/main/examples) in the repo (counter / todo / dashboard / hardware / …).
