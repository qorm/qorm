# 动作与状态

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的步骤词汇表从代码抽取,不会与实现漂移。

一个动作是 `{ "type": "action", "id": …, "steps": [ … ] }`。每个步骤修改状态、调用后端或导航。`onPress`/`onChange` 按 id 触发动作(或内联 steps)。

## 步骤类型

从运行时分发(`internal/runtime`)抽取:

| `type` | 作用 |
|---|---|
| `render` | 在此处提交一帧中间渲染,让它之前的步骤写入的状态(如 loading 标志)在慢步骤执行前就显示到屏幕上。宿主未安装帧汇时为空操作;每次派发上限 64 帧 |
| `if` | `condition` 为真执行 `then` 步骤,否则执行 `else` 步骤(可嵌套) |
| `forEach` | 对 `in` 的每个元素执行一次 `steps`,元素以 `as`(默认 `item`)绑定,并附带 `index` / `first` / `last` |
| `invoke` | 按 `name` 调用另一个动作,求值后的 `args` 并入其作用域 |
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
| `async` | bool | `http.*`:在后台发起请求——派发立即返回(因此其边界处的那一帧已显示 loading 状态,会话全程可用),结果分支在响应到达时执行。默认 `false`,即阻塞派发;宿主未安装后台汇时同样退化为 `false`,保证同一份 JSON 可移植 |
| `onSuccess` | array | `http.*`:2xx 响应后执行的步骤;解码后的响应绑定为 `{{ response }}`(`result` 路径先写入)。配合 `async` 时它们在完成回调里运行(派发已结束):`{{ state.x }}` 读取当前值,动作参数则冻结在派发时刻 |
| `onError` | array | `http.*`:失败后执行的步骤;错误信息绑定为 `{{ error }}`(`error` 路径先写入)。配合 `async` 时同样在完成回调里运行,规则与 `onSuccess` 相同 |
| `condition` | string | `if`:一个 `{{ … }}` 表达式,真值执行 `then`,否则执行 `else` |
| `then` | array | `if`:`condition` 为真时执行的步骤(分支可嵌套,深度上限 32) |
| `else` | array | `if`:`condition` 为假时执行的步骤 |
| `name` | string | `invoke`:目标动作 id(调用深度上限 16) |
| `args` | object | `invoke`:参数→值表达式,在调用方上下文求值后并入被调动作的作用域(与事件 invoke 的 args 同语义) |
| `in` | string | `forEach`:求值为待遍历数组的 `{{ … }}` 表达式;非数组则一次都不遍历 |
| `as` | string | `forEach`:当前元素的别名(默认 `item`),并派生 `<as>Index` / `<as>First` / `<as>Last` 三个键——与列表 `renderItem` 的别名规则一致 |
| `steps` | array | `forEach`:循环体,每个元素执行一次(迭代上限 10000;循环体与 `if` 共用同一嵌套深度上限) |

```json
// actions/addTodo.json — 追加一个新对象,然后清空输入
{ "type": "action", "id": "addTodo", "steps": [
  { "type": "state.appendObject", "path": "todos",
    "item": { "id": "{{ now }}", "title": "{{ state.draft }}", "done": "false" } },
  { "type": "state.set", "path": "draft", "value": "" }
] }
```

## 派生值(`computed`)

在清单里**声明一次**,而不是把同一个表达式抄进每一处绑定。派生值每帧求值一次(而非每个绑定一次),可以互相引用,并且**只读**——写入该命名空间的步骤是加载期错误,派发时也会被丢弃。

```json
// qorm.json —— 与 "globalState" 平级(也可嵌在其内部)
"computed": {
  "subtotal": "{{ sum(map(state.items, \"it.price * it.qty\")) }}",
  "isEmpty":  "{{ len(state.items) == 0 }}",
  "withTax":  "{{ computed.subtotal * 1.2 }}"
}
```

在场景绑定中读作 `{{ state.computed.subtotal }}`,在动作中读作 `{{ computed.subtotal }}`(动作作用域同样能裸读顶层状态键)。循环依赖会在加载期报错,相关派生值求值为空而不会无限递归。
