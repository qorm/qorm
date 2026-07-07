<!-- data-lang-nav --> [English](../../examples/dashboard.md) · 中文

# 示例:仪表盘

仪表盘示例用于验证复杂布局、卡片、列表、表格以及响应式能力。

## 页面结构

```text
page
├─ sidebar
├─ topbar
└─ content
   ├─ stat cards
   ├─ chart area
   └─ table
```

## 布局示例

```json
{
  "type": "row",
  "id": "dashboard_root",
  "layout": { "width": "fill", "height": "fill" },
  "children": [
    {
      "type": "section",
      "id": "sidebar",
      "layout": { "width": 240, "height": "fill" }
    },
    {
      "type": "column",
      "id": "content",
      "layout": { "width": "fill", "height": "fill", "gap": 16 }
    }
  ]
}
```

## 验收标准

- 侧边栏具有固定宽度。
- 内容区填充剩余区域。
- 表格滚动不影响整体布局。
- 数据更新只影响相关的卡片和图表。
