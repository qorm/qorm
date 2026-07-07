# QORM Action Specification

Action 是 QORM 的声明式行为系统，用于替代大部分任意脚本。

与本规范直接相关的高级能力以对应专门规格为准：
- Global Store / Context：`global-state-spec.md`
- Error Boundary：`error-boundary-spec.md`
- Query Selector：`query-selector-spec.md`
- Test Runner：`test-runner-spec.md`

## 当前 RC 示例口径

当前 Login V1 RC canonical 示例以 `examples/login/qorm.json` 和
`examples/login/login.test.json` 为准，只把 `state.set`、`host.call`、
`resultPath` 与 mock `network.request` 作为 Login 示例的可复核能力。
本规范下文保留的 `condition`、`batch`、`toast.show` 与嵌套
`output.path` 仍属于 Action DSL / 后续教程方向，不构成当前 Login
canonical 示例的 source/test 验收承诺。

## Action 目标

- 可校验。
- 可追踪。
- 可模拟。
- 可回滚。
- 可被 Agent 理解。
- 可跨平台执行。

## Action Step 类型

```text
state.set
state.toggle
state.append
state.remove
state.update
action.call
motion.play
host.call
plugin.call
native.call
navigation.go
toast.show
modal.open
modal.close
event.emit
condition
delay
batch
```

V1 规范以以上集合为主来源。Runtime 和 IR 必须与此保持一致。

## state.set

```json
{
  "type": "state.set",
  "path": "count",
  "value": "{{ count + 1 }}"
}
```

规则：
- `path` 使用 State Path 语法。
- `value` 可以是字面量、完整表达式，或包含表达式的结构化对象。
- `state.set` 用于覆盖目标路径的值。
- `global.*` / `context.*` 路径前缀的正式语义以 `global-state-spec.md` 为准。

## state.toggle

```json
{
  "type": "state.toggle",
  "path": "loading"
}
```

- 目标值必须为 boolean。

## state.append

```json
{
  "type": "state.append",
  "path": "tasks",
  "value": {
    "title": "{{ taskInput }}",
    "done": false
  }
}
```

- 目标路径必须为数组。
- `value` 会先求值，再追加到数组末尾。

## state.remove

```json
{
  "type": "state.remove",
  "path": "tasks",
  "at": "{{ selectedIndex }}"
}
```

- `at` 可选；省略时按实现约定删除最后一项或直接报错，建议 V1 显式提供。
- `at` 求值结果必须为合法数组索引。

## state.update

```json
{
  "type": "state.update",
  "path": "form",
  "value": {
    "username": "{{ username }}",
    "password": "{{ password }}"
  }
}
```

- `state.update` 用于对象级局部更新。
- V1 建议只对 object 使用；数组请使用 `state.set` / `state.append` / `state.remove`。

## action.call

```json
{
  "type": "action.call",
  "name": "form.save"
}
```

- `name` 使用逻辑 action ID；可简写为 `form.save`，规范形式为 `action://form.save`。
- 调用时在当前上下文内联执行目标 action。

## motion.play

```json
{
  "type": "motion.play",
  "target": "self",
  "motion": "button.tap"
}
```

- `target` 可以是 `self`、node ID 或语义 selector。
- `motion` 使用逻辑 motion ID；规范形式为 `motion://button.tap`。

## host.call

用于平台底层能力：

```json
{
  "type": "host.call",
  "capability": "clipboard.write",
  "input": {
    "text": "{{ shareLink }}"
  }
}
```

Host Call 必须经过权限校验。

### output

```json
{
  "type": "host.call",
  "capability": "network.request",
  "input": {
    "method": "GET",
    "url": "/api/tasks"
  },
  "output": {
    "path": "tasksResponse"
  }
}
```

- `output.path` 使用 State Path。
- Host 返回的规范化结果会写入目标路径。
- 如果未提供 `output.path`，结果只在当前执行链内部可见，不持久写入 scene state。

## network.request

Web、桌面和移动端都通过 Host Capability 抽象网络请求。`network.request` 不是独立 Action 类型，而是 `host.call` 的 capability。

```json
{
  "type": "host.call",
  "capability": "network.request",
  "input": {
    "method": "POST",
    "url": "/api/login",
    "headers": {
      "content-type": "application/json"
    },
    "body": {
      "username": "{{ username }}",
      "password": "{{ password }}"
    },
    "timeoutMs": 10000,
    "responseType": "json"
  },
  "output": {
    "path": "loginResponse"
  }
}
```

最小约束：
- `method`：如 `GET` / `POST`。
- `url`：相对路径或允许域名下的绝对 URL。
- `headers`：字符串字典。
- `body`：JSON 值；是否允许二进制由 capability contract 决定。
- `timeoutMs`：可选。
- `responseType`：`json` / `text` / `bytes`。

返回值的正式结构由 `host-capability-spec.md` 定义。

## plugin.call

```json
{
  "type": "plugin.call",
  "plugin": "analytics",
  "operation": "track",
  "input": {
    "name": "login_submit"
  }
}
```

## native.call

```json
{
  "type": "native.call",
  "operation": "camera.capture",
  "input": {}
}
```

`native.call` 仅在平台和权限显式允许时存在；V1 应优先通过 `host.call` 抽象。

## navigation.go

```json
{
  "type": "navigation.go",
  "target": "profile"
}
```

## toast.show

```json
{
  "type": "toast.show",
  "message": "保存成功"
}
```

## modal.open

```json
{
  "type": "modal.open",
  "target": "settings_modal"
}
```

## modal.close

```json
{
  "type": "modal.close"
}
```

## event.emit

```json
{
  "type": "event.emit",
  "name": "form.saved",
  "payload": {
    "id": "{{ result.id }}"
  }
}
```

V1 默认事件作用域为当前 scene。

## condition

```json
{
  "type": "condition",
  "when": "{{ form.valid && !form.loading }}",
  "then": [
    { "type": "host.call", "capability": "network.request" }
  ],
  "else": [
    { "type": "toast.show", "message": "表单未完成" }
  ]
}
```

- `when` 必须是完整表达式字符串。
- 只执行一个分支。

## delay

```json
{
  "type": "delay",
  "ms": 200
}
```

- `delay` 是异步 step；完成前后续 step 不执行。

## batch

```json
{
  "type": "batch",
  "steps": [
    { "type": "state.set", "path": "loading", "value": true },
    { "type": "host.call", "capability": "network.request" },
    { "type": "state.set", "path": "loading", "value": false }
  ]
}
```

- `batch` 自身不引入并发，内部 step 仍按顺序执行。

## 纯 Action 与非纯 Action

纯 Action：

```text
state.set
state.toggle
state.append
state.remove
state.update
action.call
motion.play
event.emit
condition
batch
```

非纯 Action：

```text
host.call
plugin.call
native.call
navigation.go
toast.show
modal.open
modal.close
delay
```

非纯 Action 需要权限和平台能力校验；其中 `delay` 不访问外部系统，但会引入时序副作用，因此不视为纯计算步骤。

## 错误传播

- step 校验失败时，整个 Action 链失败。
- `host.call` 失败时返回结构化 Host Error。
- 如果 step 已声明 `output.path` 并使用约定的结果包装结构，则调用方可以通过后续条件分支读取错误字段。
- 未声明可消费错误结果时，失败默认中断后续步骤。
- 局部运行时失败是否被 Error Boundary 捕获，以 `error-boundary-spec.md` 为准。

## 不允许

Action 不允许：

- 执行任意脚本。
- 直接访问 OS API。
- 绕过 Host Capability。
- 修改未授权文件。
- 执行 shell，除非平台和 Agent 权限显式允许。
