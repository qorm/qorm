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

`{{ … }}` 内部是一个完整的表达式(可以读取 `state.*` / 动作参数并进行算术运算);面向列表的步骤使用 `matchKey` + `match` 来定位某个特定项。完整的语言说明见[表达式](../expressions.md)。

## 用 `if` 分支

`if` 步骤根据一个条件运行两个嵌套步骤列表中的一个:

```json
{
  "type": "if",
  "condition": "{{ len(trim(state.name)) > 0 }}",
  "then": [
    { "type": "state.set", "path": "message", "value": "Hello {{ state.name }}" },
    { "type": "state.set", "path": "formError", "value": "" }
  ],
  "else": [
    { "type": "state.set", "path": "formError", "value": "Name is required" }
  ]
}
```

两个分支都是可选的,`if` 可以嵌套(最深 32 层)。条件使用与节点 `if` 属性相同的
真值规则:`null`、`false`、`0`、`""` 和空集合为假。别忘了 `{{ … }}`——像
`"state.count > 0"` 这样的裸字符串是一个非空常量,因此恒为真;加载器会针对这个
错误给出警告。

## 调用另一个动作

`invoke` 步骤按名字调用另一个动作,让共享行为只存在于一个文件里,而不必到处复制:

```json
{ "type": "invoke", "name": "resetForm", "args": { "keepEmail": "{{ true }}" } }
```

`args` 在**调用方**的上下文中求值,然后并入被调方的作用域——与按钮 `onPress` 上的
args 语义完全一致。调用深度上限为 16,因此递归或相互递归的链会终止而不是挂死;
目标动作不存在时加载器会报错。

## 调用后端

`http.get` 会把响应写入某个状态路径,并把失败写入 `error` 路径:

```json
{ "type": "http.get", "url": "https://catfact.ninja/fact", "result": "fact", "error": "err" }
```

### 成功与失败分支

任何 `http.*` 步骤都可以带 `onSuccess` / `onError` 步骤列表,它们在请求返回后运行。
在 `onSuccess` 内部,解码后的响应体绑定在 `{{ response }}`;在 `onError` 内部,
失败信息绑定在 `{{ error }}`:

```json
{
  "type": "http.get",
  "url": "https://api.example.com/items",
  "result": "items",
  "error": "loadError",
  "onSuccess": [
    { "type": "state.set", "path": "total",  "value": "{{ count(response) }}" },
    { "type": "state.set", "path": "status", "value": "loaded" }
  ],
  "onError": [
    { "type": "state.set", "path": "status", "value": "Could not load: {{ error }}" }
  ]
}
```

原有的写入依旧发生在前、行为不变:成功时写 `result` 并清空过期的 `error` 路径,
失败时写 `error` 路径而不动 `result`。分支在这之后运行,所以在 `onSuccess` 内部
`{{ state.items }}` 已经就绪。

失败包括传输错误和任何非 2xx 状态(其信息就是状态行,如 `500 Internal Server Error`)。
请求 20 秒超时;没有按步骤配置超时的字段。

## 定时器

`timer` 是一个放在场景树里的不可见节点——它不渲染任何东西,而是调度一个动作。
重复触发用 `every`,一次性触发用 `after`,单位都是毫秒:

```json
{ "type": "timer", "id": "poll",      "every": 5000, "onTick": "refresh" }
{ "type": "timer", "id": "hint_once", "after": 2000, "onTick": "showHint" }
```

`onTick` 接受动作名或 `{ "name": …, "args": … }` 形式,派发路径与按钮点击完全相同。
因为定时器是一个节点,它的**生命周期就是它在树中的存在**——给它加一个 `if`,
它就会自己停下:

```json
{ "type": "timer", "id": "countdown", "every": 1000,
  "if": "{{ state.running }}", "onTick": "tick" }
```

调度器在每次重新渲染后做一次对账,因此同一个 `id` 绝不会被重复调度、消失的定时器
会被取消、间隔变了会重新调度。`every` 的下限是 250 毫秒,并且在浏览器标签页不可见时
重复触发会暂停。定时器必须有 `id`——那正是调度器用来对账的键。

## 场景生命周期:`onEnter`

场景可以指定一个动作,在每次进入该场景时运行——也就是常见的"加载本屏数据"钩子。
它与 `root` 平级写在场景文件里:

```json
{
  "type": "scene",
  "id": "main",
  "onEnter": "loadData",
  "root": { "type": "column", "id": "root", "children": [] }
}
```

对象形式 `{ "name": "load", "args": { … } }` 同样可用,args 在场景上下文中求值。
`onEnter` 会在入口场景首次加载、深链直达该场景、`navigate` 步骤跳入,以及**后退**
回到该场景时触发。它刻意不会被页面刷新、SSE 重连或开发期热重载重放——所以一次访问
不会重复执行加载动作。但仍建议用 `if` 守卫把它写成幂等的。

目前没有 `onExit` 对应物。

`examples/lifecycle` 把这些串在了一起——`onEnter` 加载、轮询定时器、一次性 `after`
提示、会自己停下的 `if` 守卫倒计时,以及带 `if`/`else` 分支和 `invoke` 重置的表单提交:

```sh
qorm run examples/lifecycle
```

## 标准动作模式

以下是完全由上述步骤类型组成的可复用范式。每一个都是真实、可干净加载的配方 —— 复制 JSON 并改写路径即可。可运行的示例见 `examples/form`(表单校验)和 `examples/tasks`(乐观更新 + 错误处理)。

### 加载状态(Loading state)

一次派发默认只绘制一帧,而且是在**所有步骤跑完之后**——那时 `loading` 已经被改回
`false`,所以这个标志根本不会被看见。`render` 步骤解决的正是这一点:它在自己所在的位置
把此前写入的状态作为一帧提交出去,赶在慢步骤阻塞之前。

```json
[
  { "type": "state.set", "path": "loading", "value": "{{ true }}" },
  { "type": "render" },
  { "type": "http.get", "url": "https://api.example.com/items", "result": "items", "error": "error" },
  { "type": "state.set", "path": "loading", "value": "{{ false }}" }
]
```

这一帧就是普通的渲染,所以任何以该标志为条件的节点都会显示出来:

```json
{ "type": "row", "if": "{{ state.loading }}", "children": [
  { "type": "spinner", "id": "spin", "size": 16 },
  { "type": "text", "id": "lbl", "text": "Loading…" }
] }
```

`render` 只会**增加**帧:去掉它,动作依旧和以前一样只落一帧;而这些额外的帧会不会真的
被绘制,取决于 app 运行在哪个宿主上。在不支持动作中途绘制的宿主上它是空操作,因此同一份
JSON 在任何地方都是安全的。`examples/netdemo` 与 `examples/tasks` 都采用了这一写法。

### 错误处理(Error handling)

`http.*` 会把任何失败信息写入 `error` 路径(成功时清空)。在 UI 中绑定 `{{ state.error }}` 并用 `if` 显示:

```json
{ "type": "http.post", "url": "https://api.example.com/save", "body": "{{ state.draft }}", "error": "error" }
```

```json
{ "type": "text", "if": "{{ len(state.error) > 0 }}", "text": "保存失败:{{ state.error }}" }
```

当两种结果需要的是**不同的后续动作**而不只是不同的提示语时,请改用上文的
`onSuccess` / `onError` 分支,而不是事后去检查 error 路径。

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

当一个字段的结果多于两种时,`if` 步骤比三元表达式更好读,而且它每个分支可以写入
多个路径。

输入类组件同时带有浏览器的原生约束属性——`required`、`pattern`、`maxLength`,
以及决定软键盘形态的 `inputMode`。它们给用户即时的原生反馈,但**不会**阻断一个
动作:按钮的 `onPress` 是从它自己的点击处理器派发的。请自行把提交按钮的
`disabled` 绑定到你的有效性表达式来做门禁。

### 分页(Pagination)

**服务端分页**:在状态里保留一个 `page` 计数并递增;偏移量在请求 URL 绑定中计算:

```json
[
  { "type": "state.increment", "path": "page", "value": 1 },
  { "type": "http.get", "url": "https://api.example.com/items?offset={{ state.page * 20 }}&limit=20", "result": "items", "error": "error" }
]
```

**客户端分页**(数组已经在手上时)不需要动作:给 `list`(或 `gridview`)一个
`pageSize`,再把 `page` 绑定到计数器即可——见[第一个场景](first-scene.md)。
`datatable` 没有内建分页,仍需对状态数组写 `slice()` 表达式。

### 防抖搜索(Debounced search)—— *通过现有机制实现的模式*

没有 `debounce` 步骤。防抖是客户端的关注点:通过 `onChange` 把输入绑定到 `{{ state.q }}`,由 UI 控制调用搜索动作的频率。动作本身就是一个 `http.get`:

```json
{ "type": "http.get", "url": "https://api.example.com/search?q={{ state.q }}", "result": "results", "error": "error" }
```

请求取消(cancel token)目前同样没有对应的步骤 —— 视为**计划中(planned)**;最后一次写入 `result` 的响应生效。

- 动作完全是声明式数据 —— 没有任意代码。当你需要自定义的原生逻辑时,参见 [用户中间层](../platforms/native-middlelayer.md)。
- 当涉及外部副作用 / 系统能力时,请遵循 [权限模型](../security/permission-model.md)。
