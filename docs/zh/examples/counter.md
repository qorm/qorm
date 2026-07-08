<!-- data-lang-nav --> [English](https://github.com/qorm/qorm/blob/main/examples/counter.md) · 中文

# 示例:计数器

最小的完整 QORM 应用——全局状态、一个动作和一个绑定。源码:
[`examples/counter`](https://github.com/qorm/qorm/tree/main/examples/counter)。

```sh
qorm run examples/counter
```

按 `+` / `-`:按钮派发一个动作,运行时更新状态,绑定的文本随之重新渲染。

## 组成部分

全局状态,在 `qorm.json` 中声明:

```json
"globalState": { "schema": { "count": "number" }, "initial": { "count": 0 } }
```

计数值,在场景中绑定:

```json
{ "type": "text", "id": "number", "text": "{{state.count}}" }
```

按钮调用一个动作,并将当前值作为参数传入:

```json
{ "type": "button", "id": "btn_plus", "label": "+",
  "onPress": { "type": "invoke", "name": "increment", "args": { "count": "{{state.count}}" } } }
```

该动作(`actions/increment.json`)计算出新值:

```json
{ "type": "action", "id": "increment",
  "steps": [ { "type": "state.set", "path": "count", "value": "{{ count + 1 }}" } ] }
```

## 格式说明(这是可运行的格式)

- 文本使用 `text` 字段(而非 `value`);用 `{{ state.x }}` 绑定。
- 按钮的回调是 `onPress`(而非 `on: { press }`),用于指定一个动作。
- 在动作内部,`value` 能看到来自 `onPress` 的参数(此处为 `count`),因此
  `{{ count + 1 }}` 可以生效。[JSON format spec] 设计意图草案与此有出入——
  请以示例为准。
