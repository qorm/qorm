<!-- data-lang-nav --> [English](../../tutorials/first-action.md) · 中文

# 第一个动作

动作是 QORM 的声明式行为。一个动作是一系列 `steps`,放在 `actions/<id>.json` 中,并通过 UI 中 `onPress` 里的名称来触发。

## 定义一个动作

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

## 从 UI 触发

按钮的 `onPress` 就是动作名(字符串);要传递参数,则使用一个对象 `{ "name": …, "args": … }`。

```json
{ "type": "button", "id": "inc", "text": "+1", "onPress": "increment" }

{ "type": "button", "id": "toggleTask", "text": "Done",
  "onPress": { "name": "toggle", "args": { "id": "{{ item.id }}" } } }
```

## 常见步骤类型

```json
{ "type": "state.set",       "path": "name",  "value": "Ada" }
{ "type": "state.increment", "path": "count", "value": 1 }
{ "type": "state.toggle",    "path": "dark" }
{ "type": "state.append",    "path": "items", "value": { "id": 3, "text": "new" } }
{ "type": "state.toggle",    "path": "items", "matchKey": "id", "match": "{{ id }}", "field": "done" }
```

`{{ … }}` 内部是一个完整的表达式(可以读取 `state.*` / 动作参数并进行算术运算);面向列表的步骤使用 `matchKey` + `match` 来定位某个特定项。

## 调用后端

`http.get` 会把响应写入某个状态路径,并把失败写入 `error` 路径:

```json
{ "type": "http.get", "url": "https://catfact.ninja/fact", "result": "fact", "error": "err" }
```

- 动作完全是声明式数据 —— 没有任意代码。当你需要自定义的原生逻辑时,参见 [用户中间层](../platforms/native-middlelayer.md)。
- 当涉及外部副作用 / 系统能力时,请遵循 [权限模型](../security/permission-model.md)。
