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

## Standard action patterns

These are the reusable shapes built entirely from the step types above. Each is a real, load-clean recipe — copy the JSON and rename the paths. Working examples live in `examples/form` (form validation) and `examples/tasks` (optimistic update + error handling).

### Loading state

Set a flag before a call and clear it after, so the UI can bind `{{ state.loading }}` to a spinner or disabled button:

```json
[
  { "type": "state.set", "path": "loading", "value": "{{ true }}" },
  { "type": "http.get", "url": "https://api.example.com/items", "result": "items", "error": "error" },
  { "type": "state.set", "path": "loading", "value": "{{ false }}" }
]
```

### Error handling

`http.*` writes any failure message to the `error` path (and clears it on success). Bind `{{ state.error }}` in the UI and show it with an `if`:

```json
{ "type": "http.post", "url": "https://api.example.com/save", "body": "{{ state.draft }}", "error": "error" }
```

```json
{ "type": "text", "if": "{{ len(state.error) > 0 }}", "text": "Could not save: {{ state.error }}" }
```

### Optimistic update (with rollback)

Mutate state immediately, call the backend, then revert **only if** the call set the error path. The rollback re-applies the same toggle, but its `match` collapses to an empty string (matching nothing) on success — so success is a no-op and failure reverts:

```json
[
  { "type": "state.toggle", "path": "tasks", "matchKey": "id", "match": "{{ id }}", "field": "done" },
  { "type": "http.put", "url": "https://api.example.com/tasks/{{ id }}", "error": "error" },
  { "type": "state.toggle", "path": "tasks", "matchKey": "id", "match": "{{ len(state.error) > 0 ? id : \"\" }}", "field": "done" }
]
```

### Form validation

Write each field's error with one conditional `state.set` (a ternary picks the message or an empty string), then bind `{{ state.fieldErrors.email }}`. A later step can read the errors it just wrote to derive an overall status:

```json
[
  { "type": "state.set", "path": "fieldErrors.email",
    "value": "{{ len(trim(state.email)) == 0 ? \"Email is required\" : (matches(state.email, \"^[^@\\\\s]+@[^@\\\\s]+\\\\.[^@\\\\s]+$\") ? \"\" : \"Enter a valid email address\") }}" },
  { "type": "state.set", "path": "status",
    "value": "{{ len(state.fieldErrors.email) == 0 ? \"OK\" : \"Please fix the highlighted fields\" }}" }
]
```

```json
{ "type": "text", "if": "{{ len(state.fieldErrors.email) > 0 }}", "text": "{{ state.fieldErrors.email }}" }
```

### Pagination

Keep a `page` counter in state and advance it; the offset is computed in the request URL binding:

```json
[
  { "type": "state.increment", "path": "page", "value": 1 },
  { "type": "http.get", "url": "https://api.example.com/items?offset={{ state.page * 20 }}&limit=20", "result": "items", "error": "error" }
]
```

### Debounced search — *pattern via existing mechanism*

There is no `debounce` step. Debounce is a client-side concern: bind the input to `{{ state.q }}` via `onChange` and let the UI throttle how often it invokes the search action. The action itself is just an `http.get`:

```json
{ "type": "http.get", "url": "https://api.example.com/search?q={{ state.q }}", "result": "results", "error": "error" }
```

Request cancellation (cancel token) is likewise not modeled by a step today — treat it as **planned**; the last response written to `result` wins.

- Actions are entirely declarative data — no arbitrary code. When you need custom native logic, see the [User middle layer](../platforms/native-middlelayer.md).
- When external side effects / system capabilities are involved, follow the [Permission model](../security/permission-model.md).
