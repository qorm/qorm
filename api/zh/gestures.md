# 手势

QORM 把触摸/指针手势作为组件属性提供——无需编写 JavaScript。

| 手势 | 怎么用 |
|---|---|
| 点按 / 双击 / 长按 | 任意节点上的 `onPress` / `onDoubleTap` / `onLongPress` |
| 滑动删除 | 带 `onDismissed` 的 `dismissible` 组件 |
| 滑动露出操作 | 带 `actions` 列表的 `swipeactions` 组件 |
| 可滑动分页 | 水平方向的 `scroll`(滚动吸附) |
| 下拉刷新 | 带 `onRefresh` 的 `scroll` |
| 拖拽重排 | 带 `reorderable: true` + `onReorder` 的 `list` |
| 上下文菜单 | 节点上的 `contextMenu` |

## 拖拽重排

把一个数据绑定的 `list` 标记为可重排,并给它一个 `onReorder` 动作,用 `state.move`
步骤移动数组元素。客户端辅助脚本负责拖拽交互(按住某项、拖动时相邻项让位、松手);
新顺序会持久化到状态——因此智能体能看到它,且刷新后依然保留。

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

参见 `examples/reorder`。
