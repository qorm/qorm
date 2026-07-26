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

## 用 `forEach` 循环

`forEach` 步骤对一个绑定集合的每个元素各执行一遍循环体:

```json
{
  "type": "forEach",
  "in": "{{ state.items }}",
  "as": "line",
  "steps": [
    { "type": "state.updateWhere", "path": "items", "matchKey": "id",
      "match": "{{ line.id }}", "item": { "gift": "{{ true }}" } }
  ]
}
```

元素绑定在 `as` 上(默认 `item`),同时还有 `index`、`first`、`last`——别名会把整组
名字一起改掉,所以 `"as": "line"` 得到 `line` / `lineIndex` / `lineFirst` /
`lineLast`。这与列表 `renderItem` 的作用域名字完全一致,包括别名是保留名或非法时
回退为 `item` 的规则。

- `in` 只求值**一次**,所以在循环体里往同一个数组追加元素并不会把循环撑长。元素
  本身是活的,因此修改正在遍历的数组的步骤能看到自己写入的结果。
- 任何不是非空数组的东西——`null`、数字、对象、空数组、不存在的状态键——都是循环
  零次,而不是报错。
- 迭代次数上限 10000,循环体与 `if` 步骤共用 32 层嵌套上限,循环体里的 `invoke`
  仍然计入调用深度上限。任何嵌套方式都挂不死一次派发。

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

### 后台请求(`async`)

`render` 让转圈可见,但请求本身仍要跑完才结束这次派发——在后端回应之前,会话里的其它
一切都动不了。加上 `"async": true`,请求就会交给后台工作者:派发立即返回(它的那一帧
已经显示了 loading 状态),结果分支等响应到达时再执行。

```json
[
  { "type": "state.set", "path": "loading", "value": "{{ true }}" },
  { "type": "http.get", "url": "https://api.example.com/items", "async": true,
    "result": "items", "error": "error",
    "onSuccess": [ { "type": "state.set", "path": "loading", "value": "{{ false }}" } ],
    "onError":   [ { "type": "state.set", "path": "loading", "value": "{{ false }}" } ] }
]
```

请求活得比派发更久,由此有两条规则:

- **在 `onSuccess` / `onError` 里读结果,不要在兄弟步骤里读。** 异步请求之后的步骤会
  立刻执行,那时请求还开着,兄弟步骤读到的是调用**之前**的值。
- **两个分支都要复位 loading 标志。** 只在 `onSuccess` 里清除的标志,后端第一次失败
  就会永远卡在转圈上。

在分支里,`{{ state.x }}` 读到的是**此刻**的状态(用户可能还在输入),而动作自己的参数
仍然保持点击时携带的值。`async` 默认为 `false`,在没有后台工作者的宿主上同样退化为
`false`,所以那些从兄弟步骤读取响应的写法照旧可用。`examples/netdemo` 同时演示了这两种
写法。

#### 治理一个后台请求:`key`、`pending`、`timeout`

三个可选字段把上面那两条规则变成不必再记的东西。`examples/netdemo` 在同一个搜索框上
同时演示了三者:

```json
{ "type": "http.get", "url": "https://api.example.com/search?q={{ state.q }}",
  "async": true, "key": "search", "pending": "searching", "timeout": 4000,
  "result": "hits", "error": "searchErr" }
```

- **`key`** 给请求命名一个槽位。同 key 的新请求会取消该槽位上仍在途的旧请求,并
  **整体丢弃**它的结果——不写 `result`、不写 `error`、不跑分支。每次按键都发一次,
  落地的就是**最后一次**输入的结果,而不是最后返回的那一次。没有它,旧查询的快响应
  会覆盖当前查询的慢响应,这正是「搜索框显示错结果」的经典 bug。
- **`pending`** 是一个状态路径,在请求在途期间恒为 `true`:发起时置真,结束时复位,
  **失败、超时、被上限拒绝时同样复位**。它替代那对 `state.set` 步骤(连同上面第二条
  规则——已经没有分支可以漏了)。转圈组件照常绑定 `{{ state.searching }}`。该路径按
  引用计数,两个重叠的请求会一直保持到最后一个结束,被取代的请求也绝不会关掉其后继
  的转圈。
- **`timeout`** 以毫秒为单位限定本次请求,覆盖共享的 20 秒上限。超时按普通失败处理:
  写入 `error` 路径并执行 `onError`,错误信息为 `request timed out after 4000ms`。

还有一道护栏不需要写 JSON:一个运行时最多允许 **64** 个在途后台请求。超出后该步骤会
立刻走 `error` 路径失败,错误信息为 `too many concurrent requests (64 in flight)`,
而不是悄悄排队——当一个 250ms 的定时器去轮询一个要好几秒才响应的后端时,救你的正是
这条规则。

#### 编排节奏:`delay`

`{ "type": "delay", "ms": 500 }` 会在等待结束后执行**同一列表中位于它之后**的步骤,
于是 `render` / `delay` / `render` 就能编排分段展示,不需要 timer 节点,也不需要第二个
动作:

```json
[ { "type": "state.set", "path": "phase", "value": "{{ 'one' }}" },
  { "type": "render" },
  { "type": "delay", "ms": 400 },
  { "type": "state.set", "path": "phase", "value": "{{ 'two' }}" },
  { "type": "render" } ]
```

它不会阻塞:等待交给 `async` 用的那个后台工作者,所以会话全程可用。在没有后台工作者的
宿主上(离线渲染、`qorm render`、MCP 模拟)它退化为完全不等待,后续步骤立即执行——
动作仍然到达同样的最终状态,只是没人等过。

#### 在打包后的应用里,每个请求都是异步的

这是同一份 JSON 会因运行环境而行为不同的唯一一处,发布之前值得先弄清楚。

客户端宿主——`qorm package` 产出的离线包(web / iOS / Android)、独立的 WASM 运行时,
以及在线 playground——都是**单线程**的:执行你 action 的那个 goroutine,正是给浏览器
事件循环提供服务的那一个。在那里做阻塞请求不只是卡住界面,而是把整个应用死锁。
因此这些宿主会把**每一个** `http.*` 步骤都交给后台工作者,无论它的 JSON 有没有写
`"async": true`。

由此得到的是一条规则,而不是一条建议:

> 在打包后的应用里,`http.*` 步骤之后的同级步骤会在**请求仍在进行时**执行。任何
> 依赖响应的步骤都必须写进 `onSuccess` / `onError`。

```json
[
  { "type": "http.get", "url": "…/items", "result": "items",
    "onSuccess": [ { "type": "state.set", "path": "count", "value": "{{ count(response) }}" } ] },

  { "type": "state.set", "path": "count", "value": "{{ count(state.items) }}" }
]
```

第一种写法到哪里都对。第二种是同级读取:它在 `qorm run` 下能工作(服务端宿主除非
你主动选择,否则是阻塞的),而同一个应用一旦打包,它就会悄悄读到**上一次**的值。
显式写上 `"async": true` 是最省事的验证手段——它让开发服务器上也呈现打包后的行为。

请求最终落到哪里则没有变化:所有响应都回来之后,状态——因而渲染结果——与同步写法
完全一致。异步改变的是一次派发产生的帧序列,而不是它的终点。

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

### 让浏览器阻断提交

输入类组件带有浏览器的原生约束属性——`required`、`pattern`、`maxLength`,以及决定
软键盘形态的 `inputMode`。渲染器只输出这几个校验属性:没有 `minlength`、`min`、
`max`、`step`,而 `pattern` 仅对 `input` 生效(`textarea` 没有这个属性)。

放在 `form` 里、且**没有** `onPress` 的按钮本来就已经被拦住了:HTML 让它成为提交
按钮,约束不满足时浏览器根本不会发起提交,表单自己的处理器也就不会运行。真正的
漏洞是带有**自己** `onPress` 的按钮——它的点击处理器先于、且独立于约束检查运行。
`submit` 属性堵上了这个口子:

```json
{ "type": "form", "id": "signup", "onPress": "createAccount", "children": [
  { "type": "input",  "id": "email", "binding": "email", "required": true,
    "inputMode": "email", "pattern": "[^@\\s]+@[^@\\s]+\\.[^@\\s]+" },
  { "type": "button", "id": "save",   "label": "Create", "submit": true },
  { "type": "button", "id": "cancel", "label": "Cancel", "submit": false, "onPress": "goBack" }
] }
```

`form` 用 `onPress` 指定它的提交动作——在字段里按 Enter 和点击原生提交按钮触发的
都是它。

- `"submit": true` 让按钮成为真正的提交按钮。它自己没有 `onPress` 时,这次点击
  **就是**提交,而提交已经由浏览器校验过;它带有 `onPress` 时,处理器会被表单的
  有效性检查拦下,于是校验不通过的表单既不派发动作也不提交,浏览器自己的提示
  气泡会指向出问题的字段。
- `"submit": false` 让它成为普通按钮,永远不被拦截。这正是 Cancel 按钮需要的:
  它不该被一个无关的无效字段挡住。
- 不写 `submit` 时输出的标记与以前完全一致,所以你已经写好的东西行为不变。
- 这个门禁刻意不从"位于表单内部"推断:渲染器没有祖先通道,而且那样推断会毁掉
  Cancel 的场景。这个属性表达的是意图而不是位置;不在表单里的按钮上它是惰性的。
- `form` 上的 `"novalidate": true` 关掉浏览器的检查,按钮的门禁读的正是表单的这个
  标志,因此两者是一起关掉的。写在**按钮**上的 `"novalidate": true`(与 `submit`
  同用)则是单个按钮的逃生舱——比如一个应当绕过校验的"保存草稿"。

通过门禁的按钮会既运行自己的动作**又**让表单提交,所以把工作放在按钮上或表单上,
不要两边都放。

`textformfield` 把两条通道接在了一起:当字段选择了原生校验——它带有字面量形式的
`required`,或非空的 `pattern`;这两个键按字面读取,不当作绑定——它的 `error` 文案
也会经由原生通道输出为 `title` 与 `aria-invalid`,于是你写的措辞会出现在浏览器
自己的气泡里。而用户真正交互过的字段会由原生 `:user-invalid` 画上红框,因此一个
还没被碰过的必填字段不会在任何人输入之前就显得出错。

原生约束仍然不是校验引擎:它表达不了"两次密码要一致",也做不了服务端校验。这些
仍请使用上面的 `state.set` 写法;当你希望控件显得不可用、而不是点击后才抱怨时,
再去绑定按钮的 `disabled`。

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
