# Example: Todo

Todo 示例验证列表、输入、数组状态和条件渲染。

## 关键状态

```json
{
  "taskInput": "",
  "tasks": []
}
```

## 新增 Action

```json
{
  "type": "batch",
  "steps": [
    {
      "type": "state.append",
      "path": "tasks",
      "value": { "title": "{{ taskInput }}", "done": false }
    },
    {
      "type": "state.set",
      "path": "taskInput",
      "value": ""
    }
  ]
}
```

## 列表节点

```json
{
  "type": "list",
  "id": "task_list",
  "data": "{{ tasks }}",
  "item": {
    "type": "row",
    "children": [
      { "type": "checkbox", "bind": "item.done" },
      { "type": "text", "value": "{{ item.title }}" }
    ]
  }
}
```

说明：
- `data` 是完整表达式字段，求值后返回数组。
- `item.*` 只在该列表项模板上下文内可见。