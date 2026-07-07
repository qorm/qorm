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

## Acceptance

- The sidebar has a fixed width.
- The content fills the remaining area.
- Table scrolling does not affect the overall layout.
- Data updates only affect the relevant cards and charts.
