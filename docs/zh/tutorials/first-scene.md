<!-- data-lang-nav --> [English](../../tutorials/first-scene.md) · 中文

# 第一个场景

场景是 QORM 的 UI 入口。一个场景有一个 `id` 和一个根节点 `root`;节点树描述 UI。文本内容放在 `text` 字段里,`{{ state.x }}` 会插值全局状态。

## 最小场景

```json
{
  "type": "scene",
  "id": "main",
  "root": { "type": "text", "id": "hello", "text": "Hello QORM" }
}
```

## 带布局的场景

容器节点(`column` / `row`)使用 `style`(内边距、间距、背景)和 `layout`(尺寸、对齐)来排布它们的子节点。

```json
{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style":  { "padding": 24, "gap": 12 },
    "layout": { "width": "fill", "height": "fill", "align": "center" },
    "children": [
      { "type": "text",   "id": "title",  "text": "Title" },
      { "type": "button", "id": "submit", "text": "Submit", "onPress": "save" }
    ]
  }
}
```

- 文本使用 `text`(而非 `value`);模板插值(如 `"Welcome, {{ state.user }}"`)同样放在 `text` 里。
- 按钮通过 `onPress` 触发动作(参见 [第一个动作](first-action.md))。
- 所有可用的节点类型,参见 [组件目录](/api/widgets.md)。
