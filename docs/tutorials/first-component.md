---
title: First Component
description: Components are reusable UI templates declared in qorm.json, instantiated by node type and fed props plus optional slot children.
---

# First Component

Components let you reuse UI structure. A component is a template declared **in `components` within `qorm.json`**; inside the template, `{{ prop.x }}` reads the properties passed in by the instance. Instantiate it with a node whose `type` equals the component name.

## Declaring a component (qorm.json)

```json
{
  "type": "app",
  "id": "my_app",
  "entry": "main",
  "components": {
    "user_card": {
      "type": "card",
      "style": { "padding": 16, "gap": 4 },
      "children": [
        { "type": "text", "text": "{{ prop.name }}",  "style": { "fontWeight": 700 } },
        { "type": "text", "text": "{{ prop.email }}", "style": { "color": "#8e8e93" } }
      ]
    }
  }
}
```

## Using a component (scene)

The node's `type` is the component name; properties are written directly on the node as ordinary fields.

```json
{ "type": "user_card", "id": "u1", "name": "Ada", "email": "ada@example.com" }
```

![Reusable JSON components rendered on iOS](../assets/screenshots/uikit.png)
*Components declared once in JSON — metric tiles and key/value panels — reused across a screen (`examples/uikit`).*

## Slot (filling in child content)

Place a `{ "type": "slot" }` placeholder in the template; the instance's `children` are filled into it.

```json
"components": {
  "panel": {
    "type": "card",
    "style": { "padding": 16, "gap": 6 },
    "children": [
      { "type": "text", "text": "{{ prop.title }}", "style": { "fontWeight": 800 } },
      { "type": "slot" }
    ]
  }
}
```

The instance passes `children` to fill the slot:

```json
{ "type": "panel", "id": "acct", "title": "Account", "children": [
  { "type": "text", "text": "Plan: Pro" },
  { "type": "text", "text": "Seats: 12" }
] }
```

- `{{ prop.* }}` is only visible inside the component template; a field of the same name on the instance is the value passed in.
- For a complete runnable example, see [`examples/uikit`](https://github.com/qorm/qorm/tree/main/examples/uikit) (metric / kv / panel).
