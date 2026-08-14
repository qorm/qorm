<!-- data-lang-nav --> [English](https://github.com/qorm/platform/blob/main/examples/login.md) · 中文

# 示例:登录

一个带样式的登录表单——文本输入框、绑定状态和一个提交按钮。源码:
[`examples/login`](https://github.com/qorm/platform/tree/main/examples/login)。

```sh
qorm run examples/login
```

## 组成部分

全局状态保存表单字段和状态(在 `qorm.json` 中):

```json
"globalState": {
  "schema": { "email": "string", "password": "string", "isLoggingIn": "boolean", "errorMessage": "string" },
  "initial": { "email": "", "password": "", "isLoggingIn": false, "errorMessage": "" }
}
```

输入框与字段双向绑定,提交按钮以所输入的值调用一个动作:

```json
{ "type": "input", "id": "email", "binding": "email", "placeholder": "Email Address" }
{ "type": "button", "id": "submit", "label": "Sign In",
  "onPress": { "type": "invoke", "name": "performLogin", "args": { "email": "{{state.email}}", "password": "{{state.password}}" } } }
```

输入框同样可以带上浏览器的原生输入属性——它们零成本,却能给用户即时反馈和正确的软键盘:

```json
{ "type": "input", "id": "email", "binding": "email", "placeholder": "Email Address",
  "inputMode": "email", "required": true, "autocomplete": "email", "autofocus": true }
{ "type": "input", "id": "password", "binding": "password", "placeholder": "Password",
  "required": true, "maxLength": 64, "pattern": ".{8,}", "autocomplete": "current-password" }
```

这些是原生约束,不是校验引擎——它们表达不了"两次密码要一致"。但它们**可以**阻断
提交:把字段放进 `form`,并给提交按钮标上 `"submit": true`,浏览器自己的检查就会
拦住这次派发,在出问题的字段上弹出它的提示气泡,而不是运行动作。标了
`"submit": false` 的按钮(Cancel)永远不被拦截,表单上的 `"novalidate": true` 则
关掉整个检查。见[第一个动作](../tutorials/first-action.md)。

没有 `form`、或者按钮两个属性都没写时,什么也拦不住:按钮的 `onPress` 是从它自己的
点击处理器派发的。这种情况下请把按钮的 `disabled` 绑定到一个有效性表达式,并把提示
信息保存在状态里。

一行错误信息绑定到状态,以便在尝试失败时显示提示:

```json
{ "type": "text", "id": "err", "text": "{{state.errorMessage}}" }
```

可以针对运行中的应用,用 `qorm check`(布局审计)或智能体侧的
`qorm_assert` / `qorm_dispatch` MCP 工具来验证这个流程——见[验证](../verification.md)。

## 格式说明

- 输入框用 `binding` 绑定(双向);按钮的 `onPress` 指定一个动作
  并将状态值作为参数传入。
- `input` / `textarea` / `textformfield` 可用的原生输入属性:`required`、
  `maxLength`、`pattern`(仅 input)、`inputMode`、`autofocus`、`readonly`、
  `autocomplete`。
