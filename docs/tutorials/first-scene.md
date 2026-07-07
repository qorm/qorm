# First Scene

Scene 是 QORM 的界面入口。一个 scene 有 `id` 和一个根节点 `root`;节点树描述界面。
文本内容放在 `text` 字段,`{{ state.x }}` 把全局状态插进来。

## 最小 Scene

```json
{
  "type": "scene",
  "id": "main",
  "root": { "type": "text", "id": "hello", "text": "Hello QORM" }
}
```

## 带布局的 Scene

容器节点(`column` / `row`)用 `style`(内边距、间距、背景)和 `layout`(尺寸、对齐)
排列子节点。

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
      { "type": "text",   "id": "title",  "text": "标题" },
      { "type": "button", "id": "submit", "text": "提交", "onPress": "save" }
    ]
  }
}
```

- 文本用 `text`(不是 `value`);`"欢迎,{{ state.user }}"` 这样的模板插值也放在 `text` 里。
- 按钮用 `onPress` 触发动作(见 [First Action](first-action.md))。
- 全部可用节点类型见[组件目录](../reference/widgets.md)。
