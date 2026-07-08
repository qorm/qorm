# 导航

一个 QORM 应用可以有多个场景(`scenes/*.json`,每个 `{"type":"scene","id":...}`)。
清单里的 `entry` 最先显示;`navigate` 动作步骤在场景间移动,并带一个返回栈。

```json
// actions/openProfile.json — go to a scene
{ "type": "action", "id": "openProfile",
  "steps": [ { "type": "navigate", "to": "profile" } ] }

// actions/back.json — return to the previous scene
{ "type": "action", "id": "back",
  "steps": [ { "type": "navigate", "back": true } ] }
```

- `to` 是一个场景 id(可含 `{{bindings}}`);导航到未知场景或当前场景是空操作。
- `back` 弹出导航栈。
- 共享的实时会话会跟随导航:一个派发 navigate 动作的智能体也会移动人的视图(反之亦然)。
  桌面窗口可用 `?scene=<id>` 固定某个特定场景。

## 页面转场

切换场景会自动播放一段协调的、iOS 风格的转场:进入的场景从边缘滑入,离开的场景以
视差方式向另一方向滑动(幅度更小)并变暗,营造纵深。`navigate` 向前滑,`back` 反向。
滑动过程中每个场景被当作不透明块处理,因此没有自身背景的场景不会互相透出。

参见 `examples/navigation`。
