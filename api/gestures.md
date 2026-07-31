# Gestures

QORM ships touch/pointer gestures as widget props — no JavaScript to write.

| gesture | how |
|---|---|
| tap / double-tap / long-press | `onPress` / `onDoubleTap` / `onLongPress` on any node |
| swipe-to-dismiss | a `dismissible` widget with `onDismissed` |
| swipe-to-reveal actions | a `swipeactions` widget with an `actions` list |
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

## Keyboard focus (native canvas backend)

The native canvas window implements real keyboard navigation:

- **Tab / Shift-Tab** moves focus between focusable nodes: buttons, nodes with
  an `onPress`, or any node with `"focusable": true`; `"focusable": false`
  opts out; `"tabIndex": N` (N > 0) sorts ahead of natural order, ascending.
- **Enter / Space** activates the focused node's `onPress`.
- **Escape** clears focus.
- The focus ring is drawn only for keyboard-driven focus (focus-visible
  semantics — clicking a node moves focus without showing a ring).
- `onKeyDown` / `onKeyUp` dispatch to the focused node first and bubble up to
  the scene root when unhandled; the pressed key is available to the action as
  `{{ arg.key }}` (`tab`, `return`, `space`, `escape`, `up` / `down` / `left` /
  `right`, `delete`, `a`..`z`, `0`..`9`).

On the HTML/WebView path, keyboard focus follows native browser behavior;
declare `focusBorderColor` in a node's style to get a matching `:focus-visible`
outline.
