# Example: Dashboard

Dashboard 示例验证复杂布局、卡片、列表、表格和响应式。

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

## Layout 示例

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

## 验收

- sidebar 固定宽度。
- content 填充剩余区域。
- 表格滚动不影响整体 layout。
- 数据更新只影响相关卡片和图表。
