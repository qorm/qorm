# Example: Counter

The smallest complete QORM app — global state, an action, and a binding. Source:
[`examples/counter`](https://github.com/qorm/qorm/tree/main/examples/counter).

```sh
qorm run examples/counter
```

Press `+` / `-`: the button dispatches an action, the runtime updates state, and
the bound text re-renders.

## The pieces

Global state, declared in `qorm.json`:

```json
"globalState": { "schema": { "count": "number" }, "initial": { "count": 0 } }
```

The count, bound in the scene:

```json
{ "type": "text", "id": "number", "text": "{{state.count}}" }
```

A button invokes an action, passing the current value as an argument:

```json
{ "type": "button", "id": "btn_plus", "label": "+",
  "onPress": { "type": "invoke", "name": "increment", "args": { "count": "{{state.count}}" } } }
```

The action (`actions/increment.json`) computes the new value:

```json
{ "type": "action", "id": "increment",
  "steps": [ { "type": "state.set", "path": "count", "value": "{{ count + 1 }}" } ] }
```

## Format notes (this is the runnable format)

- Text is the `text` field (not `value`); bind with `{{ state.x }}`.
- A button's callback is `onPress` (not `on: { press }`), naming an action.
- Inside the action, `value` sees the args from `onPress` (here `count`), so
  `{{ count + 1 }}` works. The [JSON format spec] design-intent draft diverges —
  trust the examples.
