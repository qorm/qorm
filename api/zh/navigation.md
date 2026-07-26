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

## 路由守卫

场景可以声明"进入它所需满足的前置条件"。守卫在**每一条**进入路径上都会执行——动作的
`navigate` 步骤、浏览器前进/后退、直达该场景的深链,以及初始入口场景——因此受保护的
路由无法靠拼 URL 绕过,而且这个判断只写一处,不必抄进每个跳转动作。

```json
// scenes/dashboard.json
{ "type": "scene", "id": "dashboard",
  "guard": {
    "condition": "{{ state.user != null }}",
    "redirect": "login",
    "params": { "next": "{{ 'dashboard' }}" }
  },
  "root": { ... } }
```

- `condition` 是场景作用域下的 `{{ … }}` 表达式(`state.*`、`t.*`、`viewport.*`、
  `route.*`、`state.computed.*`)。为真即可进入。
- `redirect` 是守卫不通过时改去的场景,其 `params` 成为目标场景的 `{{ route.* }}`。
  不写 `redirect` 则直接拒绝本次导航(停在原场景)——这也意味着它无法保护入口场景,
  加载器会给出告警。
- 重定向目标本身也受其守卫保护,因此守卫可以串联。若链路重复经过同一场景,则拒绝该次
  导航而不是打转;加载器会在加载期对重定向环给出告警。
- 守卫在场景的 `onEnter` **之前**执行,因此"加载私有数据"的钩子不会为被拒绝的访客触发。
  守卫重定向会**替换**当前帧而非压栈,所以返回键永远不会退回到你被拒绝的场景。

参见 `examples/derived`。
