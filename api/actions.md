# Actions & State

> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The step vocabulary below is extracted from the code, so it can never drift.

An action is `{ "type": "action", "id": …, "steps": [ … ] }`. Each step mutates state, calls a backend, or navigates. `onPress`/`onChange` run an action by id (or inline steps).

## Step types

Extracted from the runtime dispatch (`internal/runtime`):

| `type` | What it does |
|---|---|
| `navigate` | go to another scene (or `back`) |
| `state.set` | set a state path to a value |
| `state.append` | append a value to an array |
| `state.appendObject` | append an object (built from `item` field expressions) |
| `state.toggle` | flip a boolean, or a `field` on a matched array element; on a scalar array toggles membership of `match` |
| `state.increment` | add to a number (`value` is the delta, default +1) |
| `state.remove` | remove the array element selected by `match` |
| `state.updateWhere` | update `field` on every element matching `match` |
| `state.merge` | shallow-merge an object into a state path |
| `state.sort` | sort an array by `field` |
| `state.move` | move an array element `from` index `to` index |
| `state.clear` | empty an array or clear a string/number |
| `state.reset` | restore the manifest's initial values — one key with `path`, all state without |
| `http.get` | GET a URL, store the parsed JSON at `result` |
| `http.post` | POST `body`, store the response at `result` |
| `http.put` | PUT `body`, store the response at `result` |
| `http.delete` | DELETE a URL |
| `http.request` | generic request with an explicit `method` |

## Step fields

Every step is one JSON object; which fields apply depends on its `type`:

| Field | Type | Used by |
|---|---|---|
| `type` | string | the step kind (table above) — required |
| `path` | string | target state path, e.g. `todos` or `user.name` |
| `value` | string | value expression; may contain `{{ bindings }}` |
| `match` | string | expression selecting an array element (with `matchKey`) |
| `matchKey` | string | object key compared against `match` (default `id`) |
| `field` | string | field to toggle/update within the matched object |
| `item` | object | field → value expressions for `state.appendObject` |
| `to` | string | `navigate`: target scene id · `state.move`: target index |
| `back` | bool | `navigate`: pop the back stack instead of pushing |
| `from` | string | `state.move`: source index |
| `url` | string | `http.*`: request URL (may contain `{{ bindings }}`) |
| `method` | string | `http.request`: HTTP method override |
| `body` | string | `http.*`: request body |
| `headers` | object | `http.*`: request headers |
| `result` | string | `http.*`: state path to store the parsed response |
| `error` | string | `http.*`: state path to store an error message |

```json
// actions/addTodo.json — append a new object, then clear the input
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "todos",
    "item": { "id": "{{ now }}", "title": "{{ state.draft }}", "done": "false" } },
  { "type": "state.set", "path": "draft", "value": "" }
] }
```
