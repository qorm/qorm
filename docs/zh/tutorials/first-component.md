<!-- data-lang-nav --> [English](../../tutorials/first-component.md) · 中文

# 第一个组件

组件让你可以复用 UI 结构。组件是一个模板,**声明在 `qorm.json` 的 `components` 中**;在模板内部,`{{ prop.x }}` 读取实例传入的属性。用一个 `type` 等于组件名的节点来实例化它。

## 声明一个组件(qorm.json)

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

## 使用一个组件(场景)

节点的 `type` 就是组件名;属性作为普通字段直接写在节点上。

```json
{ "type": "user_card", "id": "u1", "name": "Ada", "email": "ada@example.com" }
```

## 插槽(填充子内容)

在模板中放置一个 `{ "type": "slot" }` 占位符;实例的 `children` 会被填入其中。

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

实例传入 `children` 来填充插槽:

```json
{ "type": "panel", "id": "acct", "title": "Account", "children": [
  { "type": "text", "text": "Plan: Pro" },
  { "type": "text", "text": "Seats: 12" }
] }
```

- `{{ prop.* }}` 只在组件模板内部可见;实例上同名的字段就是传入的值。
- 完整的可运行示例,参见 [`examples/uikit`](../../../examples/uikit)(metric / kv / panel)。
