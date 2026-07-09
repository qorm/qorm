# Project structure

A QORM app is a small folder of JSON — no build step, no bundler. The runtime
loads this folder directly (`qorm run <dir>`), and the packager turns the same
folder into a desktop app, a mobile app, or a PWA. One optional Go file adds
native code that compiles into every target.

```
myapp/
  qorm.json            manifest — the one required file
  scenes/              one screen per file
    main.json          { "type": "scene", "id": "main", "root": { … node tree … } }
  actions/             one action per file
    addTodo.json       { "type": "action", "id": "addTodo", "steps": [ … ] }
  components/          optional — reusable component definitions ({ "type": "component" })
  native/             optional — the app's own middle-layer
    desktop.go         Go native ops (compiled into BOTH desktop and mobile/web WASM)
    web.js             optional browser-side ops for the pure-web build
  assets/             images / icons referenced by nodes (e.g. "assets/icon.png")
  *.test.json          optional — declarative tests run by `qorm test`
```

## `qorm.json` — the manifest

The only required file. It names the app, picks the entry scene, and declares
the global state:

```json
{
  "type": "app",
  "id": "qorm_todo",
  "name": "Productive Todo",
  "entry": "main",
  "theme": "apple",
  "globalState": {
    "schema":  { "items": "array", "inputValue": "string" },
    "initial": { "items": [], "inputValue": "" }
  },
  "platforms": {
    "desktop": { "window": { "width": 500, "height": 700, "icon": "assets/icon.png" } }
  }
}
```

| Key | Meaning |
|---|---|
| `id` · `name` | app identifier and display name |
| `entry` | the scene id shown first |
| `theme` | design token set (e.g. `apple`) |
| `globalState` | `schema` (typed shape) + `initial` (starting values) for `state.*` |
| `components` | reusable component definitions (or a folder of them) |
| `platforms` | per-platform config — desktop `window`, and packaging options |
| `defaultLocale` | initial language for multi-locale apps |

## Live development (hot-reload)

`qorm run <dir>` watches the app folder: edit a scene, action or the manifest and
save, and every connected browser/window updates instantly — no restart. The live
session is preserved across the reload (your in-progress state, the current scene
and the viewport survive), so you keep exactly where you were. A half-written file
that fails to parse is reported and the running app is kept until the next good
save. Pass `--no-watch` to turn it off.

## `scenes/` — screens

Each scene is one JSON file: `{ "type": "scene", "id": …, "root": <node> }`. The
`root` is a node tree — see [Node & widget props](/api/props.md) for the
node schema and [the widget catalog](/api/widgets.md) for every `type`.
Move between scenes with the `navigate` step; see [Navigation](/api/navigation.md).

## `actions/` — behavior

Each action is `{ "type": "action", "id": …, "steps": [ … ] }`, referenced from a
node's `onPress` / `onChange`. Steps mutate state, call a backend, or navigate —
the full vocabulary is in [Actions & state](/api/actions.md).

## `native/` — the app's own code

Optional. One Go file (`native/desktop.go`) registers the app's **own** native
ops via [`pkg/qormext`](/api/go-api.md); the packager compiles it into the
desktop binary **and** the mobile/web WASM, so the same custom logic runs on
every target. A `native/web.js` can add browser-only ops. This is the app's
extension point — see [the middle-layer guide](platforms/native-middlelayer.md).

## What's NOT in the app folder

The runtime, renderer, packager and the vendored WebView are QORM's job, not the
app's — an app never carries a toolchain. The app folder is only its own
declarations plus the one optional native file.
