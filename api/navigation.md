# Navigation

A QORM app can have many scenes (`scenes/*.json`, each `{"type":"scene","id":...}`).
The `entry` in the manifest is shown first; the `navigate` action step moves between
them, with a back stack.

```json
// actions/openProfile.json — go to a scene
{ "type": "action", "id": "openProfile",
  "steps": [ { "type": "navigate", "to": "profile" } ] }

// actions/back.json — return to the previous scene
{ "type": "action", "id": "back",
  "steps": [ { "type": "navigate", "back": true } ] }
```

- `to` is a scene id (may contain `{{bindings}}`); navigating to an unknown scene
  or the current one is a no-op.
- `back` pops the navigation stack.
- The shared live session follows navigation: an agent that dispatches a navigate
  action moves the human's view too (and vice versa). A desktop window may pin a
  specific scene with `?scene=<id>`.

## Page transition

Switching scenes plays a coordinated, iOS-style transition automatically: the
incoming scene slides in from the edge while the outgoing one parallax-slides the
other way (less far) and dims, giving depth. `navigate` slides forward; `back`
reverses it. Each scene is treated as an opaque block during the slide, so scenes
without their own background don't bleed through each other.

See `examples/navigation`.

## Route guards

A scene can declare the precondition for entering it. The guard runs on **every**
entry path — an action's `navigate` step, a browser Back/Forward, a deep link
straight into the scene, and the initial entry scene — so a protected route
cannot be reached by spelling a URL, and the check lives in one place instead of
being repeated in every navigating action.

```json
// scenes/dashboard.json
{ "type": "scene", "id": "dashboard",
  "guard": {
    "condition": "{{ state.user != null }}",
    "redirect": "login",
    "params": { "next": "{{ 'dashboard' }}" }
  },
  "root": { ... } }
```

- `condition` is a `{{ … }}` expression in scene scope (`state.*`, `t.*`,
  `viewport.*`, `route.*`, `state.computed.*`). Truthy = enter.
- `redirect` is where a failing guard sends you instead; its `params` become the
  target's `{{ route.* }}`. Without a `redirect` the navigation is simply
  refused (you stay where you are) — which also means it cannot protect the
  entry scene, so the loader warns.
- The redirect target is itself guarded, so guards chain. A chain that revisits a
  scene refuses the navigation rather than looping; the loader warns about a
  redirect cycle at load time.
- A guard runs **before** the scene's `onEnter`, so a hook that loads private
  data never fires for a visitor who was refused the page. A guard redirect
  REPLACES the current frame rather than pushing one, so Back never returns to a
  scene you were denied.

See `examples/derived`.
