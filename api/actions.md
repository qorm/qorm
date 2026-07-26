# Actions & State

> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The step vocabulary below is extracted from the code, so it can never drift.

An action is `{ "type": "action", "id": …, "steps": [ … ] }`. Each step mutates state, calls a backend, or navigates. `onPress`/`onChange` run an action by id (or inline steps).

## Step types

Extracted from the runtime dispatch (`internal/runtime`):

| `type` | What it does |
|---|---|
| `delay` | wait `ms` milliseconds, then run the steps that FOLLOW it in the same list — `render` / `delay` / `render` paces a staged reveal declaratively. It never blocks: the wait goes to the host's background sink, and on a host with no sink (an offline render, an MCP simulation) it degrades to no wait at all, so the action still reaches the same final state |
| `render` | publish an intermediate frame right here, so state written by the steps before it (a loading flag) reaches the screen before a slow step runs. No-op on a host with no frame sink; capped at 64 frames per dispatch |
| `if` | run `then` steps when `condition` is truthy, `else` steps otherwise (nestable) |
| `forEach` | run `steps` once per element of `in`, with the element bound under `as` (default `item`) plus `index` / `first` / `last` |
| `invoke` | call another action by `name`, merging evaluated `args` into its scope |
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
| `state.clear` | empty an array, or clear a string/number/boolean to its zero |
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
| `body` | string | `http.*`: request body — a string is sent verbatim (an inline JSON template is not double-encoded); a bound non-string value (map/list/number/bool) is JSON-encoded |
| `headers` | object | `http.*`: request headers |
| `result` | string | `http.*`: state path to store the parsed response |
| `error` | string | `http.*`: state path to store an error message |
| `async` | bool | `http.*`: run the request in the background — the dispatch returns immediately (so the frame at its boundary already shows the loading state and the session stays responsive) and the result branch runs when the reply arrives. Defaults to `false`, which blocks the dispatch; it also falls back to `false` on a host with no background sink, so the same JSON stays portable |
| `key` | string | `http.*`: name a request slot — starting a new request on a key cancels whichever request was still open on it AND discards that one's outcome entirely (no `result`/`error` write, no branch). This is what makes search-as-you-type land the reply to the LAST keystroke instead of whichever round trip finished last. Only an `async` request can be superseded; unkeyed requests never cancel each other |
| `timeout` | number | `http.*`: this request's ceiling in milliseconds, overriding the shared client's 20s. Expiry is an ordinary failure — the `error` path is written and `onError` runs, with the message `request timed out after <n>ms`. Applies to synchronous requests too. Omit (or 0) to keep the 20s ceiling |
| `pending` | string | `http.*`: a state path held `true` for exactly as long as the request is open — set on launch, cleared when it settles, INCLUDING on failure, timeout and refusal. Replaces the hand-written pair of `state.set` steps (the one that reliably forgets the error path); bind a spinner to `{{ state.<path> }}` as usual. Reference-counted, so overlapping requests on one path hold it until the last settles and a superseded request cannot switch off its successor's spinner |
| `ms` | number | `delay`: how long to wait, in milliseconds. The steps FOLLOWING the delay in the same list run when it expires |
| `onSuccess` | array | `http.*`: steps run after a 2xx response; the decoded response is bound as `{{ response }}` (the `result` path is written first). With `async` they run in the completion callback, after the dispatch has ended: `{{ state.x }}` reads live, the action's args stay frozen at dispatch time |
| `onError` | array | `http.*`: steps run after a failure; the message is bound as `{{ error }}` (the `error` path is written first). With `async` they run in the completion callback, on the same terms as `onSuccess` |
| `condition` | string | `if`: a `{{ … }}` expression selecting `then` (truthy) or `else` |
| `then` | array | `if`: steps run when `condition` is truthy (branches nest, depth-capped at 32) |
| `else` | array | `if`: steps run when `condition` is falsy |
| `name` | string | `invoke`: the target action id (call depth capped at 16) |
| `args` | object | `invoke`: arg → value expressions, evaluated in the caller's context and merged into the callee's scope (same semantics as an event invoke's args) |
| `in` | string | `forEach`: a `{{ … }}` expression yielding the array to iterate; anything that is not an array iterates zero times |
| `as` | string | `forEach`: name the current element (default `item`), plus the derived `<as>Index` / `<as>First` / `<as>Last` keys — the same alias rule a list's `renderItem` uses |
| `steps` | array | `forEach`: the loop body, run once per element (iterations capped at 10000; the body nests under the same depth cap as `if`) |

```json
// actions/addTodo.json — append a new object, then clear the input
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "todos",
    "item": { "id": "{{ now }}", "title": "{{ state.draft }}", "done": "false" } },
  { "type": "state.set", "path": "draft", "value": "" }
] }
```

## Derived values (`computed`)

Declare a value ONCE in the manifest instead of repeating the expression in every binding. Derived values are evaluated once per frame (not once per binding), may read each other, and are **read-only** — a step that writes into the namespace is a load-time error and is dropped at dispatch.

```json
// qorm.json — beside "globalState" (or nested inside it)
"computed": {
  "subtotal": "{{ sum(map(state.items, \"it.price * it.qty\")) }}",
  "isEmpty":  "{{ len(state.items) == 0 }}",
  "withTax":  "{{ computed.subtotal * 1.2 }}"
}
```

Read them as `{{ state.computed.subtotal }}` in a scene binding, and as `{{ computed.subtotal }}` inside an action (which also sees every top-level state key bare). A dependency cycle is reported at load time and those values evaluate to nothing rather than recursing.
