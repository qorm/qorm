# Example: Todo

Lists, text input, and array state. Source: [`examples/todo`](https://github.com/qorm/qorm/tree/main/examples/todo).

```sh
qorm run examples/todo
```

Type a task and add it; toggle tasks done. The list is data-bound to an array in
state, so adding/toggling re-renders it.

## The pieces

An input bound two-way to state, and an add button that invokes an action:

```json
{ "type": "input", "id": "field", "binding": "inputValue", "placeholder": "New task…" }
{ "type": "button", "id": "add", "label": "Add",
  "onPress": { "type": "invoke", "name": "addTodo", "args": { "text": "{{state.inputValue}}" } } }
```

The add action appends an object to the array and clears the input
(`actions/addTodo.json`):

```json
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "items",
    "item": { "id": "{{ text }}", "text": "{{ text }}", "done": "{{ false }}" } },
  { "type": "state.set", "path": "inputValue", "value": "" }
] }
```

A data-bound list renders each item; `{{item.*}}` is the per-row scope:

```json
{ "type": "list", "id": "items", "data": "{{ state.items }}",
  "renderItem": { "type": "row", "children": [
    { "type": "checkbox", "onChange": { "type": "invoke", "name": "toggleTodo", "args": { "id": "{{item.id}}" } } },
    { "type": "text", "text": "{{ item.text }}" }
  ] } }
```

## Format notes

- The repeat is `list` with `data: "{{ state.items }}"` and a `renderItem`
  template (not `item`); `{{ item.* }}` is visible only inside that template.
- Append with `state.appendObject`; two-way input via `binding`.
