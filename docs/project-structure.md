# Project structure

A QORM app is a small folder of JSON — no build step, no bundler. The runtime
loads this folder directly (`qorm run <dir>`), and the packager turns the same
folder into a desktop app, a mobile app, or a PWA. One optional Go file adds
native code that compiles into every target.

```
myapp/
  qorm.json            manifest — the one required file
  qorm.config.json     optional — host window / build-time config (NOT bundled, NOT signed)
  scenes/              one screen per file
    main.json          { "type": "scene", "id": "main", "root": { … node tree … } }
  actions/             one action per file
    addTodo.json       { "type": "action", "id": "addTodo", "steps": [ … ] }
  components/          optional — reusable component definitions ({ "type": "component" })
  native/             optional — the app's own middle-layer
    desktop.go         Go native ops (compiled into BOTH desktop and mobile/web WASM)
    web.js             optional browser-side ops for the pure-web build
  assets/             images / icons referenced by nodes (e.g. "assets/icon.png")
```

Documents are found by **type, not by folder**: a `{ "type": "component" }` file
works wherever you put it, and the folder names above are convention. Three
boundaries apply when the app is collected, because a bundle is signed and
shipped and must contain only what you can review:

- `locales/` is read separately as message catalogs, not as app documents.
- A subfolder with its own `qorm.json` is a nested project and is skipped, so
  its documents cannot collide with the parent's ids.
- A symbolic link pointing outside the app folder is refused (a `.json` link
  fails the load; an escaping `locales/` is skipped). Links *within* the folder
  work normally.

Two documents claiming the same id is an error. `qorm run` still starts, so you
can keep iterating, but `qorm build` and `qorm package` refuse — a signed
artifact must not depend on which file happened to sort last.

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
    "desktop": { "window": { "width": 500, "height": 700 } }
  }
}
```

| Key | Meaning |
|---|---|
| `id` · `name` | app identifier and display name |
| `entry` | the scene id shown first |
| `theme` | `apple` / `material` / `dark`, or `auto` (default — Apple palette that follows the OS light/dark setting) |
| `globalState` | `schema` (typed shape) + `initial` (starting values) for `state.*` |
| `computed` | derived values — name → expression, read as `{{ state.computed.x }}` and re-evaluated once per frame ([Expressions](expressions.md)) |
| `components` | reusable component definitions (or a folder of them) |
| `platforms` | per-platform config — desktop `window`, and packaging options |
| `defaultLocale` | initial language for multi-locale apps |
| `breakpoints` | optional width thresholds (px) exposed as `breakpoint.<name>` booleans |
| `agent.policy` | optional MCP tool permissions for agents (see [permissions](agent/permissions.md)) |
| `capabilities` | optional runtime native-op gate (`used-only` / `manifest` / `open`) |

## `qorm.config.json` — the host window (optional)

An optional file **beside** `qorm.json` that configures the window the app opens
in. It is host/build-time config: it is never bundled or signed (the signed
payload is the app's *content*), so a build farm or a local checkout can
re-window an app without editing it. Window settings that are part of the
app's identity and must ship with a signed bundle belong in the manifest
instead (`platforms.desktop.window`).

```json
{
  "window": {
    "width": 1024,
    "height": 480,
    "title": "Raiden",
    "resizable": false,
    "chromeless": true,
    "transparent": true,
    "hideLog": true,
    "hideTray": true
  }
}
```

| Key | Meaning |
|---|---|
| `width` · `height` | window size in points at launch; the runtime Viewport is seeded from them before the first render (no client round-trip). `0`/absent = fluid |
| `title` | window title; falls back to the app `name` |
| `resizable` | default `true`; `false` locks the window to its declared size (fixed game boards, HUDs) |
| `chromeless` | no title bar / border — widget/overlay style; drag via the app's drag region |
| `transparent` | transparent background → a **shaped window**: the app's own rendered content defines the visible shape, the rest is click-through |
| `hideLog` · `hideTray` | don't spawn the Activity-log window / the menu-bar tray icon |

Precedence (highest wins): `qorm.config.json` `window` → `qorm.json`
`platforms.desktop.window` → `qorm.json` top-level `display`. The top-level
`display` block is the backwards-compatible spelling (geometry only); a
`display` block inside `qorm.config.json` is still accepted too — when both
blocks are present they merge per key (`window` overrides only the keys it
declares). Because the config is the override layer, an explicit `"width": 0`
resets a manifest-declared size back to fluid. `chromeless` + `transparent`
together make a 异形 (custom-shape) window — fully supported on the macOS
**WebView** host (`-tags desktop`); the default pure-Go canvas window honours
size/resizable but not the chrome flags. On Windows `chromeless` is honoured
(`transparent` pending); a chromeless window that is still `resizable` keeps a
thin edge-resize border, since that frame bit is what makes a window
user-resizable there. On Linux both parse but the decoration strip is not wired
yet. A malformed config file is reported as a load diagnostic rather than
applied.

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

Two optional keys sit beside `root`: `onEnter` names an action to run each time
the scene is entered, and `guard` declares the precondition for entering it at
all — see [Navigation & routing](spec/navigation-spec.md).

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
