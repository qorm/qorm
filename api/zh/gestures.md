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

## 键盘焦点(原生 canvas 后端)

原生 canvas 窗口实现了真正的键盘导航:

- **Tab / Shift-Tab** 在可聚焦节点间移动焦点:按钮、带 `onPress` 的节点,或任意
  声明 `"focusable": true` 的节点;`"focusable": false` 可退出;`"tabIndex": N`
  (N > 0)按升序排在自然顺序之前。
- **Enter / Space** 触发焦点节点的 `onPress`。
- **Escape** 清除焦点。
- 焦点环仅在键盘导航时绘制(focus-visible 语义——点击节点会移走焦点但不显示环)。
- `onKeyDown` / `onKeyUp` 先发给焦点节点,未处理则冒泡到场景根;按键名可通过
  `{{ arg.key }}` 在动作中使用(`tab`、`return`、`space`、`escape`、`up` /
  `down` / `left` / `right`、`delete`、`a`..`z`、`0`..`9`)。

HTML/WebView 端的键盘焦点沿用浏览器原生行为;在节点样式中声明 `focusBorderColor`
即可获得与之配套的 `:focus-visible` 焦点环。
