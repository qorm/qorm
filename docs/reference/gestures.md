# Gestures

QORM ships touch/pointer gestures as widget props — no JavaScript to write.

| gesture | how |
|---|---|
| tap / double-tap / long-press | `onPress` / `onDoubleTap` / `onLongPress` on any node |
| swipe-to-dismiss | a `dismissible` widget with `onDismissed` |
| swipeable pages | a horizontal `scroll` (scroll-snap) |
| pull-to-refresh | a `scroll` with `onRefresh` |
| drag-to-reorder | a `list` with `reorderable: true` + `onReorder` |
| context menu | `contextMenu` on a node |

## Drag-to-reorder

Mark a data-bound `list` reorderable and give it an `onReorder` action that moves
the array element with a `state.move` step. The client helper handles the drag
(press an item, drag it while siblings slide aside, release); the new order is
persisted to state — so an agent sees it and it survives a reload.

```json
// scene: the list
{ "type": "list", "id": "tasks", "reorderable": true,
  "onReorder": { "type": "invoke", "name": "onReorder" },
  "data": "{{state.items}}", "renderItem": { … } }

// actions/onReorder.json — the client passes _reorderFrom / _reorderTo
{ "type": "action", "id": "onReorder",
  "steps": [ { "type": "state.move", "path": "items",
               "from": "{{ _reorderFrom }}", "to": "{{ _reorderTo }}" } ] }
```

See `examples/reorder`.
