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

## 标准动作模式

以下是完全由上述步骤类型组成的可复用范式。每一个都是真实、可干净加载的配方 —— 复制 JSON 并改写路径即可。可运行的示例见 `examples/form`(表单校验)和 `examples/tasks`(乐观更新 + 错误处理)。

### 加载状态(Loading state)

在调用前置一个标志、调用后清除,这样 UI 就能把 `{{ state.loading }}` 绑定到加载指示或禁用按钮:

```json
[
  { "type": "state.set", "path": "loading", "value": "{{ true }}" },
  { "type": "http.get", "url": "https://api.example.com/items", "result": "items", "error": "error" },
  { "type": "state.set", "path": "loading", "value": "{{ false }}" }
]
```

### 错误处理(Error handling)

`http.*` 会把任何失败信息写入 `error` 路径(成功时清空)。在 UI 中绑定 `{{ state.error }}` 并用 `if` 显示:

```json
{ "type": "http.post", "url": "https://api.example.com/save", "body": "{{ state.draft }}", "error": "error" }
```

```json
{ "type": "text", "if": "{{ len(state.error) > 0 }}", "text": "保存失败:{{ state.error }}" }
```

### 乐观更新(Optimistic update,含回滚)

先立即修改状态,再调用后端,然后**仅当**该调用写入了 error 路径时才回滚。回滚步骤重新应用同一个 toggle,但它的 `match` 在成功时会塌缩为空字符串(匹配不到任何元素)—— 因此成功是无操作,失败才会撤销:

```json
[
  { "type": "state.toggle", "path": "tasks", "matchKey": "id", "match": "{{ id }}", "field": "done" },
  { "type": "http.put", "url": "https://api.example.com/tasks/{{ id }}", "error": "error" },
  { "type": "state.toggle", "path": "tasks", "matchKey": "id", "match": "{{ len(state.error) > 0 ? id : \"\" }}", "field": "done" }
]
```

### 表单校验(Form validation)

用一个条件式 `state.set` 写入每个字段的错误(用三元表达式在错误信息与空字符串之间取舍),然后绑定 `{{ state.fieldErrors.email }}`。后续步骤可以读取它刚写入的错误来推导一个总体状态:

```json
[
  { "type": "state.set", "path": "fieldErrors.email",
    "value": "{{ len(trim(state.email)) == 0 ? \"Email is required\" : (matches(state.email, \"^[^@\\\\s]+@[^@\\\\s]+\\\\.[^@\\\\s]+$\") ? \"\" : \"Enter a valid email address\") }}" },
  { "type": "state.set", "path": "status",
    "value": "{{ len(state.fieldErrors.email) == 0 ? \"OK\" : \"Please fix the highlighted fields\" }}" }
]
```

```json
{ "type": "text", "if": "{{ len(state.fieldErrors.email) > 0 }}", "text": "{{ state.fieldErrors.email }}" }
```

### 分页(Pagination)

在状态里保留一个 `page` 计数并递增;偏移量在请求 URL 绑定中计算:

```json
[
  { "type": "state.increment", "path": "page", "value": 1 },
  { "type": "http.get", "url": "https://api.example.com/items?offset={{ state.page * 20 }}&limit=20", "result": "items", "error": "error" }
]
```

### 防抖搜索(Debounced search)—— *通过现有机制实现的模式*

没有 `debounce` 步骤。防抖是客户端的关注点:通过 `onChange` 把输入绑定到 `{{ state.q }}`,由 UI 控制调用搜索动作的频率。动作本身就是一个 `http.get`:

```json
{ "type": "http.get", "url": "https://api.example.com/search?q={{ state.q }}", "result": "results", "error": "error" }
```

请求取消(cancel token)目前同样没有对应的步骤 —— 视为**计划中(planned)**;最后一次写入 `result` 的响应生效。

- 动作完全是声明式数据 —— 没有任意代码。当你需要自定义的原生逻辑时,参见 [用户中间层](../platforms/native-middlelayer.md)。
- 当涉及外部副作用 / 系统能力时,请遵循 [权限模型](../security/permission-model.md)。
