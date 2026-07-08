<!-- data-lang-nav --> [English](https://github.com/qorm/qorm/blob/main/examples/login.md) · 中文

# 示例:登录

一个带样式的登录表单——文本输入框、绑定状态和一个提交按钮。源码:
[`examples/login`](https://github.com/qorm/qorm/tree/main/examples/login)。

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

一行错误信息绑定到状态,以便在尝试失败时显示提示:

```json
{ "type": "text", "id": "err", "text": "{{state.errorMessage}}" }
```

登录流程由 [`login.test.json`](https://github.com/qorm/qorm/blob/main/examples/login/login.test.json) 进行验证
(这是一个 `type: test` 测试装置,加载器在运行时会跳过它,但测试框架会运行它)。

## 格式说明

- 输入框用 `binding` 绑定(双向);按钮的 `onPress` 指定一个动作
  并将状态值作为参数传入。
