# Example: Login

本页记录当前 V1 RC canonical subset。它对齐真实可运行示例
[`examples/login/qorm.json`](../../examples/login/qorm.json) 与测试
[`examples/login/login.test.json`](../../examples/login/login.test.json)，不是完整 Login
表单教程。

## Current V1 RC canonical subset

当前 Login canonical 示例只覆盖一个最小登录流：

- scene state 只有 `status` 与 `loginResult`。
- `submit_login` 先把 `status` 设为 `loading`。
- 随后通过 `host.call` 调用 `network.request`，并用 `resultPath` 把 mock
  结果写入 `loginResult`。
- 最后把 `status` 设为 `authenticated`。

## 状态

```json
{
  "status": "idle",
  "loginResult": null
}
```

UI 绑定也保持最小：

- `status_text` 显示 `Status: {{ status }}`。
- `result_text` 显示 `User: {{ loginResult.user.name }}`。
- `login_button` 的 `press` 事件触发 `submit_login`。

## Submit Action

```json
[
  {
    "type": "state.set",
    "path": "status",
    "value": "loading"
  },
  {
    "type": "host.call",
    "capability": "network.request",
    "input": {
      "method": "POST",
      "url": "https://example.invalid/login",
      "body": {
        "username": "demo"
      }
    },
    "resultPath": "loginResult"
  },
  {
    "type": "state.set",
    "path": "status",
    "value": "authenticated"
  }
]
```

## Test / 验收

当前测试使用 `examples/login/login.test.json` 的 conformance 口径：

- mock `network.request` 返回 `{ "ok": true, "user": { "name": "Ada" } }`。
- simulate `login_button` 的 `press` 事件。
- assert `host_called` 覆盖 `network.request`。
- assert state：`status = authenticated`，`loginResult.user.name = Ada`。
- assert text：`status_text = Status: authenticated`，
  `result_text = User: Ada`。

## 非当前承诺

当前 Login canonical 示例不使用、也不承诺以下完整教程语义：

- `condition`
- `batch`
- `toast.show`
- 嵌套 `output.path` 形式
- `username` / `password` 表单校验
- 独立 `loading` boolean / `error` / `loginResponse` 状态模型

## 后续方向

完整 Login 教程可以在后续 backlog 中升级为包含表单输入、校验分支、
loading/error 状态、Toast 提示，以及 `condition` / `batch` /
`toast.show` / `output.path` 的正式教程。升级前，当前 V1 RC Login
示例仍以 `examples/login/qorm.json` 与 `examples/login/login.test.json`
为准。
