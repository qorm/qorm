# 导航与路由

QORM 应用如何在场景(scene)之间跳转、在跳转时携带数据,以及这些数据相对于应用其余
状态处于什么位置。

## 场景栈

运行中的应用同一时刻只显示一个场景。当前显示哪个场景是**运行时**的属性,而非应用定义
的属性——manifest 只声明应用启动时打开的 `entry` 场景,之后的一切都由 `navigate`
动作步骤驱动。

运行时维护一个**返回栈(back stack)**,记录你一路走来的场景。向前导航会把当前场景
*压栈*并显示目标场景;返回导航则把栈顶*弹出*并回到它。

```
entry: home
  home                      栈: []
  → navigate 到 profile     栈: [home]           显示: profile
  → navigate 到 settings    栈: [home, profile]  显示: settings
  → back                    栈: [home]           显示: profile
  → back                    栈: []               显示: home
  → back                    栈: []               显示: home   (空操作)
```

对空栈执行返回是空操作,因此在入口场景上按硬件/返回键永远不会让应用走进死胡同。导航到
当前已显示的场景、或导航到未知的场景 id,都会被忽略。

### 导航

`navigate` 步骤通过 id 指定目标场景(`to`),或弹出栈(`back: true`):

```json
{ "type": "action", "id": "openProfile",
  "steps": [ { "type": "navigate", "to": "profile" } ] }

{ "type": "action", "id": "back",
  "steps": [ { "type": "navigate", "back": true } ] }
```

`to` 本身也可以是绑定表达式——`"to": "{{ state.nextScene }}"`——因此一个动作就能实现
动态路由。

### 页面过渡

每次导航都会记录一个**方向**——向前 `navigate` 记为 `push`,返回记为 `pop`。客户端每帧
读取一次该方向(读取后即清除),以播放对应的页面过渡:向前 push 时新场景从尾侧边缘滑入,
pop 时向反方向滑回。方向纯属表现层,绝不影响状态。

## 导航参数——`route.*`

一个 navigate 步骤可以携带**路由参数**:在派发时计算、附着到目标场景上的具名值。目标
场景通过 `route.*` 命名空间读取它们,与 `state.*`、`viewport.*`、`t.*` 并列。

在 `params` 下声明(参数名 → 值表达式):

```json
{ "type": "navigate", "to": "profile",
  "params": { "userId": "{{ userId }}", "name": "{{ name }}" } }
```

每个表达式在动作的上下文中求值一次,因此可以读取动作的调用参数(如上例)、`state.*`
或作用域内的任何东西。求得的带类型的值即成为目标场景的 route。

目标场景用 `{{ route.<名字> }}` 绑定它们:

```json
{ "type": "text", "text": "{{ route.name }}" }
{ "type": "text", "text": "User id: {{ route.userId }}" }
```

缺失的键解析为 nil(渲染为空文本),因此没有携带某个参数就到达的场景会优雅降级,而不会
报错。

### 参数随栈帧走

路由参数是**帧局部(frame-local)**的:它属于显示该场景的那一个具体栈帧,而不属于场景
id。向前导航时,当前场景*连同其当前 route*一起压栈;返回时两者一起恢复。因此从详情页
返回,会把上一屏原样放回——参数也一并恢复。

```
home  (route: {})                  → openProfile(userId=u-101)
profile  (route: {userId:u-101})   → openProfile(userId=u-102)   [继续下钻]
profile  (route: {userId:u-102})   → back
profile  (route: {userId:u-101})   ← 恢复更早那个帧的 route
home  (route: {})                  ← 再 back 恢复入口的空 route
```

入口场景从一个空 route 开始(`{}`,永不为 nil)。

## 场景局部 route vs. 全局 state

QORM 有两个截然不同的数据存放处,而导航正是二者边界最关键的地方:

| | `globalState`(`state.*`) | 路由参数(`route.*`) |
|---|---|---|
| 作用范围 | **所有**场景共享的一个存储 | 仅**当前栈帧** |
| 生命周期 | 整个应用会话 | 该帧位于栈上期间 |
| 由谁写入 | `state.*` 动作步骤、`http.*` 结果 | `navigate` 步骤的 `params` |
| 如何读取 | `{{ state.x }}` | `{{ route.x }}` |
| 在哪声明 | `qorm.json` 的 `globalState.schema` | 每次导航临时指定 |

**全局 state** 用于存放跨越单个屏幕、或被多个屏幕共享的数据——登录用户、购物车、缓存
列表、当前主题/语言。**路由参数**用于存放那些说明*这是哪一个实例*的小小标识符——某个
profile 屏正在展示的 `userId`、某个详情屏打开的订单 id。路由参数是 QORM 里函数实参的
类比:它是调用方告诉目标屏该渲染什么的方式,而无需改动其他屏幕都能看见的共享状态。

经验法则:如果返回时应当把它忘掉,它就是路由参数;如果应当持久保留,它就属于全局
state。

## URL 路由(已实现)

内存中的场景栈会镜像到浏览器地址栏,于是深链 URL 与浏览器的前进/后退按钮都从同一个
模型里自然导出。URL 用查询串编码当前场景及其路由参数:

```
/?scene=profile&userId=u-101&name=Ada
   │       │        └────┬─────┘
   │       │             └── 路由参数  → route.userId、route.name
   │       └── 场景 id                  → "profile" 场景
   └── 入口场景就是 "/"
```

规则如下:

- **`RoutePath`** —— 运行时把当前场景 + 路由参数渲染成这个路径(入口场景且无参数时为
  `/`;键会排序以保证稳定)。它是"地址栏该显示什么"的唯一真相来源。
- **深链入口** —— 加载 `/?scene=<id>&k=v` 会在首次渲染*之前*把运行时导航到该场景,于是
  页面直接打开到那里,且查询参数绑定到 `route.*`。未知场景 id 会被忽略,回落到入口场景。
- **地址栏同步** —— 每次导航都会把它的 `RoutePath` 送给客户端(`/event` 响应的
  `X-Qorm-Route` 头,以及 SSE/轮询负载里的 `route` 字段)。当它变化时客户端
  `history.pushState`,于是 URL 无需刷新就跟随导航。
- **前进 / 后退** —— 浏览器的历史移动会把目标 URL 的场景 + 参数回报给服务端
  (`POST /navigate`,与 `/event` 一样有人侧 token 校验),由它驱动运行时对齐。回到上一帧
  会弹栈(恢复其参数);前进则压栈。

来自 URL 的路由参数值是字符串(查询串是无类型的),所以通过深链到达的场景看到的
`route.userId` 是 `"u-101"`。由于路由参数是 QORM 中函数实参的类比,把标识符作为路由参数
传递(而不是塞进全局 state)的应用,可以免费获得可分享、可刷新的深链。
