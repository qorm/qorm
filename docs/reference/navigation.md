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
