# First Action

Action 是 QORM 的声明式行为。一个 action 是一串 `steps`,放在 `actions/<id>.json`,由界面上的
`onPress` 按名字触发。

## 定义一个 action

`actions/increment.json`:

```json
{
  "type": "action",
  "id": "increment",
  "steps": [
    { "type": "state.set", "path": "count", "value": "{{ state.count + 1 }}" }
  ]
}
```

## 从界面触发

按钮的 `onPress` 就是动作名(字符串);需要传参时用对象 `{ "name": …, "args": … }`。

```json
{ "type": "button", "id": "inc", "text": "+1", "onPress": "increment" }

{ "type": "button", "id": "toggleTask", "text": "完成",
  "onPress": { "name": "toggle", "args": { "id": "{{ item.id }}" } } }
```

## 常用 step 类型

```json
{ "type": "state.set",       "path": "name",  "value": "Ada" }
{ "type": "state.increment", "path": "count", "value": 1 }
{ "type": "state.toggle",    "path": "dark" }
{ "type": "state.append",    "path": "items", "value": { "id": 3, "text": "new" } }
{ "type": "state.toggle",    "path": "items", "matchKey": "id", "match": "{{ id }}", "field": "done" }
```

`{{ … }}` 里是完整表达式(能读 `state.*` / 动作参数、做算术);列表类 step 用 `matchKey` +
`match` 定位某一项。

## 调用后端

`http.get` 把响应写入状态路径,失败写入 `error` 路径:

```json
{ "type": "http.get", "url": "https://catfact.ninja/fact", "result": "fact", "error": "err" }
```

- 动作全部是声明式数据——没有任意代码。需要自定义原生逻辑时,见[用户中间层](../platforms/native-middlelayer.md)。
- 涉及外部副作用/系统能力时,遵循[权限模型](../security/permission-model.md)。
