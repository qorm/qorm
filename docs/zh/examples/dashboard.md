<!-- data-lang-nav --> [English](https://github.com/qorm/platform/blob/main/examples/dashboard.md) · 中文

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

## 表格

仪表盘里的表格很少只是纯文本。`table` 与 `datatable` 会把带 `column: "<键>"` 的
子节点当作该列的**单元格模板**,在行作用域(`{{ row.x }}`、`{{ rowIndex }}`、
`{{ cell.value }}`)中渲染,于是状态列可以是一个标签、趋势列可以是一张图。
`stickyHeader` 在表体滚动时冻结表头,`scrollX` + `minWidth` 让宽表仍然可用,列
自己的 `sticky` 则在横向滚动时冻结该列——这正是"表格滚动不影响整体布局"在实现上
的含义。

表格上方的那些数字很适合做成清单里的 `computed` 派生值:合计只声明一次,由每张
卡片各自绑定,而不是把同一段表达式抄很多遍。参见[第一个场景](../tutorials/first-scene.md)
与[表达式](../expressions.md)。

## 验收标准

- 侧边栏具有固定宽度。
- 内容区填充剩余区域。
- 表格滚动不影响整体布局。
- 数据更新只影响相关的卡片和图表。
