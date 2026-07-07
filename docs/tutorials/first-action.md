# First Action

An action is QORM's declarative behavior. An action is a sequence of `steps`, placed in `actions/<id>.json`, and triggered by name from an `onPress` in the UI.

## Defining an action

`actions/increment.json`:

```json
{
  "type": "action",
  "id": "increment",
  "steps": [
    { "type": "state.set", "path": "count", "value": "{{ state.count + 1 }}" }
  ]
}
```

## Triggering from the UI

A button's `onPress` is the action name (a string); to pass arguments, use an object `{ "name": …, "args": … }`.

```json
{ "type": "button", "id": "inc", "text": "+1", "onPress": "increment" }

{ "type": "button", "id": "toggleTask", "text": "Done",
  "onPress": { "name": "toggle", "args": { "id": "{{ item.id }}" } } }
```

## Common step types

```json
{ "type": "state.set",       "path": "name",  "value": "Ada" }
{ "type": "state.increment", "path": "count", "value": 1 }
{ "type": "state.toggle",    "path": "dark" }
{ "type": "state.append",    "path": "items", "value": { "id": 3, "text": "new" } }
{ "type": "state.toggle",    "path": "items", "matchKey": "id", "match": "{{ id }}", "field": "done" }
```

Inside `{{ … }}` is a full expression (it can read `state.*` / action arguments and do arithmetic); list-oriented steps use `matchKey` + `match` to locate a specific item.

## Calling a backend

`http.get` writes the response to a state path, and writes failures to the `error` path:

```json
{ "type": "http.get", "url": "https://catfact.ninja/fact", "result": "fact", "error": "err" }
```

- Actions are entirely declarative data — no arbitrary code. When you need custom native logic, see the [User middle layer](../platforms/native-middlelayer.md).
- When external side effects / system capabilities are involved, follow the [Permission model](../security/permission-model.md).
