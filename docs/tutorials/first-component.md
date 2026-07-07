# First Component

Component 用于复用 UI 结构。组件是**在 `qorm.json` 的 `components` 里**声明的模板;模板内用
`{{ prop.x }}` 读取实例传入的属性。用一个 `type` 等于组件名的节点来实例化它。

## 声明组件(qorm.json)

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

## 使用组件(scene)

节点的 `type` 就是组件名;属性作为普通字段直接写在节点上。

```json
{ "type": "user_card", "id": "u1", "name": "Ada", "email": "ada@example.com" }
```

## Slot(填充子内容)

模板里放一个 `{ "type": "slot" }` 占位;实例的 `children` 会填进去。

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

实例传 `children` 填 slot:

```json
{ "type": "panel", "id": "acct", "title": "Account", "children": [
  { "type": "text", "text": "Plan: Pro" },
  { "type": "text", "text": "Seats: 12" }
] }
```

- `{{ prop.* }}` 只在组件模板内可见;实例上同名字段即是传入的值。
- 完整可运行示例见 [`examples/uikit`](../../examples/uikit)(metric / kv / panel)。
