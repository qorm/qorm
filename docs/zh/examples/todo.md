<!-- data-lang-nav --> [English](../../examples/todo.md) · 中文

# 示例:待办事项

列表、文本输入和数组状态。源码:[`examples/todo`](../../../examples/todo)。

```sh
qorm run examples/todo
```

输入一项任务并添加;切换任务的完成状态。列表与状态中的一个数组进行数据绑定,
因此添加/切换会使其重新渲染。

## 组成部分

一个与状态双向绑定的输入框,以及一个调用动作的添加按钮:

```json
{ "type": "input", "id": "field", "binding": "inputValue", "placeholder": "New task…" }
{ "type": "button", "id": "add", "label": "Add",
  "onPress": { "type": "invoke", "name": "addTodo", "args": { "text": "{{state.inputValue}}" } } }
```

添加动作将一个对象追加到数组中并清空输入框
(`actions/addTodo.json`):

```json
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "items",
    "item": { "id": "{{ text }}", "text": "{{ text }}", "done": "{{ false }}" } },
  { "type": "state.set", "path": "inputValue", "value": "" }
] }
```

一个数据绑定的列表渲染每一项;`{{item.*}}` 是每行的作用域:

```json
{ "type": "list", "id": "items", "data": "{{ state.items }}",
  "renderItem": { "type": "row", "children": [
    { "type": "checkbox", "onChange": { "type": "invoke", "name": "toggleTodo", "args": { "id": "{{item.id}}" } } },
    { "type": "text", "text": "{{ item.text }}" }
  ] } }
```

## 格式说明

- 重复渲染使用 `list`,配合 `data: "{{ state.items }}"` 和一个 `renderItem`
  模板(而非 `item`);`{{ item.* }}` 仅在该模板内部可见。
- 使用 `state.appendObject` 追加;通过 `binding` 实现双向输入。
