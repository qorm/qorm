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

See `examples/navigation`.
