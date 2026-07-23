# 动作与状态

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的步骤词汇表从代码抽取,不会与实现漂移。

一个动作是 `{ "type": "action", "id": …, "steps": [ … ] }`。每个步骤修改状态、调用后端或导航。`onPress`/`onChange` 按 id 触发动作(或内联 steps)。

## 步骤类型

从运行时分发(`internal/runtime`)抽取:

| `type` | 作用 |
|---|---|
| `navigate` | 跳转到另一个场景(或 `back`) |
| `state.set` | 把状态路径设为某值 |
| `state.append` | 向数组追加一个值 |
| `state.appendObject` | 追加一个对象(由 `item` 字段表达式构建) |
| `state.toggle` | 翻转布尔值,或匹配数组元素上的某个 `field`;对标量数组则切换 `match` 的成员资格 |
| `state.increment` | 对数字累加(`value` 为增量,默认 +1) |
| `state.remove` | 移除 `match` 选中的数组元素 |
| `state.updateWhere` | 更新所有匹配 `match` 的元素的 `field` |
| `state.merge` | 把一个对象浅合并进状态路径 |
| `state.sort` | 按 `field` 对数组排序 |
| `state.move` | 把数组元素从 `from` 移到 `to` |
| `state.clear` | 清空数组,或把字符串 / 数字 / 布尔值清除为其零值 |
| `state.reset` | 恢复清单中的初始值——带 `path` 时仅重置该键,不带则重置全部状态 |
| `http.get` | GET 一个 URL,把解析后的 JSON 存到 `result` |
| `http.post` | POST `body`,把响应存到 `result` |
| `http.put` | PUT `body`,把响应存到 `result` |
| `http.delete` | DELETE 一个 URL |
| `http.request` | 带显式 `method` 的通用请求 |

## 步骤字段

每个步骤是一个 JSON 对象;哪些字段生效取决于其 `type`:

| 字段 | 类型 | 用于 |
|---|---|---|
| `type` | string | 步骤类型(见上表)——必填 |
| `path` | string | 目标状态路径,如 `todos` 或 `user.name` |
| `value` | string | 值表达式;可含 `{{ bindings }}` |
| `match` | string | 选中某个数组元素的表达式(配合 `matchKey`) |
| `matchKey` | string | 与 `match` 比较的对象键(默认 `id`) |
| `field` | string | 在匹配对象内切换 / 更新的字段 |
| `item` | object | `state.appendObject` 的字段→值表达式 |
| `to` | string | `navigate`:目标场景 id · `state.move`:目标索引 |
| `back` | bool | `navigate`:弹出返回栈而非压入 |
| `from` | string | `state.move`:源索引 |
| `url` | string | `http.*`:请求 URL(可含 `{{ bindings }}`) |
| `method` | string | `http.request`:覆盖 HTTP 方法 |
| `body` | string | `http.*`:请求体——字符串原样发送(内联 JSON 模板不会被二次编码);绑定的非字符串值(map/list/number/bool)会被 JSON 编码 |
| `headers` | object | `http.*`:请求头 |
| `result` | string | `http.*`:存放解析后响应的状态路径 |
| `error` | string | `http.*`:存放错误信息的状态路径 |

```json
// actions/addTodo.json — 追加一个新对象,然后清空输入
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "todos",
    "item": { "id": "{{ now }}", "title": "{{ state.draft }}", "done": "false" } },
  { "type": "state.set", "path": "draft", "value": "" }
] }
```
