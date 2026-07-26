# Example: Dashboard

The Dashboard example verifies complex layouts, cards, lists, tables, and responsiveness.

## Page Structure

```text
page
├─ sidebar
├─ topbar
└─ content
   ├─ stat cards
   ├─ chart area
   └─ table
```

## Layout Example

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

## The table

A dashboard table is rarely plain text. `table` and `datatable` take a child
node carrying `column: "<key>"` as that column's **cell template**, rendered in
a row scope (`{{ row.x }}`, `{{ rowIndex }}`, `{{ cell.value }}`), so a status
column can be a tag and a trend column a chart. `stickyHeader` freezes the
header while the body scrolls, `scrollX` + `minWidth` keep a wide table usable,
and a column's own `sticky` freezes it as the rest scrolls sideways — which is
what "table scrolling does not affect the overall layout" means in practice.

The numbers above the table are a good fit for a manifest `computed` value:
declare the total once and bind it from every card instead of repeating the
expression. See [First scene](../tutorials/first-scene.md) and
[Expressions](../expressions.md).

## Acceptance

- The sidebar has a fixed width.
- The content fills the remaining area.
- Table scrolling does not affect the overall layout.
- Data updates only affect the relevant cards and charts.
