# 动作与状态

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的步骤词汇表从代码抽取,不会与实现漂移。

一个动作是 `{ "type": "action", "id": …, "steps": [ … ] }`。每个步骤修改状态、调用后端或导航。`onPress`/`onChange` 按 id 触发动作(或内联 steps)。

## 步骤类型

从运行时分发(`internal/runtime`)抽取:

| `type` | 作用 |
|---|---|
| `delay` | 等待 `ms` 毫秒,然后执行同一列表中位于其**后面**的步骤——`render` / `delay` / `render` 即可声明式地编排分段展示。它不会阻塞:等待交给宿主的后台汇执行;宿主未安装后台汇时(离线渲染、MCP 模拟)退化为完全不等待,动作仍然到达同样的最终状态 |
| `render` | 在此处提交一帧中间渲染,让它之前的步骤写入的状态(如 loading 标志)在慢步骤执行前就显示到屏幕上。宿主未安装帧汇时为空操作;每次派发上限 64 帧 |
| `if` | `condition` 为真执行 `then` 步骤,否则执行 `else` 步骤(可嵌套) |
| `forEach` | 对 `in` 的每个元素执行一次 `steps`,元素以 `as`(默认 `item`)绑定,并附带 `index` / `first` / `last` |
| `invoke` | 按 `name` 调用另一个动作,求值后的 `args` 并入其作用域 |
| `navigate` | 跳转到另一个场景(或 `back`) |
| `state.set` | 把状态路径设为某值 |
| `state.setAt` | — |
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
| `key` | string | `http.*`:给请求命名一个槽位——同 key 的新请求会取消该槽位上仍在途的旧请求,并**整体丢弃**它的结果(不写 `result`/`error`,不跑分支)。这正是「搜索框每输入一次都发请求」时保证落地的是**最后一次**输入的结果、而不是最后返回的那一次。只有 `async` 请求会被取代;不带 key 的请求之间互不取消 |
| `timeout` | number | `http.*`:该请求的超时上限(毫秒),覆盖共享客户端的 20 秒默认值。超时按普通失败处理——写入 `error` 路径并执行 `onError`,错误信息为 `request timed out after <n>ms`。同步请求同样生效。不写(或写 0)则保持 20 秒上限 |
| `pending` | string | `http.*`:一个状态路径,在请求在途期间恒为 `true`——发起时置真,结束时复位,**失败、超时、被上限拒绝时同样复位**。用它替代手写的那对 `state.set`(手写版最容易漏掉错误路径);转圈组件照常绑定 `{{ state.<路径> }}`。该路径按引用计数,多个请求共用一个标志时要等最后一个结束才复位,被取代的请求也不会关掉其后继的转圈 |
| `ms` | number | `delay`:等待的毫秒数。同一列表中位于该步骤**之后**的步骤会在等待结束后执行 |
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

## 脚本动作(`script`)

动作可以携带一段 qscript 程序来代替 steps:`{ "type": "action", "id": "tick", "script": "…" }`。JSON 继续声明场景与数据,逻辑由脚本承担——`let`、赋值(`state.a =`、`state.arr[i] =`)、`if`/`else`、`for x in …` 与 `while`(支持 `break`/`continue`)、`fn` 函数定义与调用、表达式语言的运算符与内建函数、可读写的 `state` 句柄以及注入派发参数的 `args`(见 `internal/qscript`)。加载器在加载期编译脚本——解析错误会以带行号的诊断报出——`script` 与 `steps` 同时声明时给出警告,且一律执行 script。运行期失败(治理超限或类型错误)会带着脚本行号记录在运行时上。脚本有界(单次运行 20 万操作、单循环 10 万次迭代、64 层嵌套调用)且天然确定——无 IO,唯一的时钟是显式的 `now()` 内建(Unix 毫秒;调用它的脚本仅在那一值内可复现)。脚本还能用 `call("id" [, args])` 派发兄弟动作——派发重新进入运行时,因此与 `invoke` 步骤相同的调用深度上限(16)同样管着 `call()` 链,动作不存在或深度被拒都会以带调用方行号的脚本错误浮现——但永远触达不到应用之外。完整示例见 `examples/tetris`。

## 脚本文件动作(`actions/*.qs`)

脚本动作可以住在独立文件里,而不必塞进 JSON 字符串字段:`actions/tick.qs` 即动作 `tick`,文件全文就是它的 qscript 源码。这套布局就是把 DOM+CSS+JS 三层分离搬进应用——结构在 `scenes/*.json`,逻辑在 `actions/*.qs`,两者按动作 id 相互绑定(场景的 `onPress`/`keys`/定时器点名动作,脚本经 `state` 触达结构)。加载器把 `actions/` 目录下的每个 `*.qs` 文件收集为与 JSON 写法相同的 `type:"action"` 文档,因此下游一切划一:场景引用、加载期编译(解析错误是同时带文件名和行号的诊断)、打包哈希(`.qs` 与 JSON 动作一样被签名——`qorm build` 与 `qorm run` 永远一致)。两种写法可以并存;同 id 的 `.json` 与 `.qs` 属于重复定义——目录加载报错并保留最先出现的定义(`.json` 排序在前),`qorm build` 则直接拒绝构建。有一个保留文件名:`actions/lib.qs` 不是动作,而是**共享函数库**——它的 fn 定义会在派发时拼接到每个脚本动作之前,让应用把公共助手函数放在一个文件里,而不是在每个动作里复制粘贴。库文件只应包含 fn 定义与注释(它被编译在每个动作体之前,任何顶层语句都会在每个动作之前执行);重复定义第二个库会像重复样式表一样被诊断,库文件与其他文档一样被打包、被哈希。`examples/tetris` 的全部十个动作都采用这种写法,棋盘核心逻辑放在 `lib.qs` 里。

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
