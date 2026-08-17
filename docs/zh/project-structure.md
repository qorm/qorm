# 工程结构

一个 QORM 应用就是一小撮 JSON 文件——没有构建步骤,没有打包器。运行时直接加载这个
文件夹(`qorm run <目录>`),打包器把同一个文件夹变成桌面应用、移动应用或 PWA。一个
可选的 Go 文件即可加入编译进所有目标的原生代码。

```
myapp/
  qorm.json            清单——唯一必需的文件
  qorm.config.json     可选——宿主窗口 / 构建期配置(不打进 bundle、不参与签名)
  scenes/              每个屏幕一个文件
    main.json          { "type": "scene", "id": "main", "root": { … 节点树 … } }
  actions/             每个动作一个文件
    addTodo.json       { "type": "action", "id": "addTodo", "steps": [ … ] }
  components/          可选——可复用的组件定义({ "type": "component" })
  native/             可选——应用自己的中间层
    desktop.go         Go 原生操作(同时编译进桌面与移动/web WASM)
    web.js             可选——纯 web 构建的浏览器端操作
  assets/             节点引用的图片 / 图标(如 "assets/icon.png")
```

文档按**类型识别,而非按目录**:`{ "type": "component" }` 文件放在哪里都能生效,上面的目录名只是约定。收集应用时有三条边界——bundle 是要签名并分发的产物,只能包含你能审阅的内容:

- `locales/` 作为消息目录单独读取,不当作应用文档。
- 带有自己 `qorm.json` 的子目录视为嵌套项目并跳过,其文档不会与父应用的 id 相撞。
- 指向应用目录之外的符号链接会被拒绝(`.json` 链接使加载失败;逃逸的 `locales/` 被跳过)。目录**内部**的链接正常工作。

两个文档声明同一个 id 是错误。`qorm run` 仍会启动以便你继续迭代,但 `qorm build` 与 `qorm package` 会拒绝——签名产物不该取决于哪个文件恰好排在后面。

## `qorm.json` —— 清单

唯一必需的文件。它命名应用、指定入口场景,并声明全局状态:

```json
{
  "type": "app",
  "id": "qorm_todo",
  "name": "Productive Todo",
  "entry": "main",
  "theme": "apple",
  "globalState": {
    "schema":  { "items": "array", "inputValue": "string" },
    "initial": { "items": [], "inputValue": "" }
  },
  "platforms": {
    "desktop": { "window": { "width": 500, "height": 700 } }
  }
}
```

| 键 | 含义 |
|---|---|
| `id` · `name` | 应用标识与显示名 |
| `entry` | 首先显示的场景 id |
| `theme` | `apple` / `material` / `dark`,或 `auto`(默认——跟随系统明暗的 Apple 配色) |
| `globalState` | 供 `state.*` 使用的 `schema`(类型化结构)+ `initial`(初始值) |
| `computed` | 派生值——名字 → 表达式,用 `{{ state.computed.x }}` 读取,每帧重新求值一次(见[表达式](expressions.md)) |
| `components` | 可复用的组件定义(或一个组件文件夹) |
| `platforms` | 各平台配置——桌面 `window`、以及打包选项 |
| `defaultLocale` | 多语言应用的初始语言 |
| `breakpoints` | 可选宽度阈值（px），以 `breakpoint.<name>` 布尔值暴露 |
| `agent.policy` | Agent 的 MCP 工具权限（见 [permissions](agent/permissions.md)） |
| `capabilities` | 运行时 native-op 门禁（`used-only` / `manifest` / `open`） |

## `qorm.config.json` —— 宿主窗口(可选)

与 `qorm.json` **并列**的可选文件,用来配置应用打开时的窗口。它属于宿主/构建期配置:
不会被打进 bundle、也不参与签名(签名载荷是应用的*内容*),因此构建农场或本地检出
可以在不改动应用本身的情况下重新设定窗口。属于应用身份、且必须随签名 bundle 分发的
窗口设置,应写在清单内(`platforms.desktop.window`)。

```json
{
  "window": {
    "width": 1024,
    "height": 480,
    "title": "Raiden",
    "resizable": false,
    "chromeless": true,
    "transparent": true,
    "hideLog": true,
    "hideTray": true
  }
}
```

| 键 | 含义 |
|---|---|
| `width` · `height` | 启动时的窗口尺寸(点);运行时视口在首帧渲染前就由其初始化(无需客户端往返)。`0`/缺省 = 流式自适应 |
| `title` | 窗口标题;缺省回退到应用 `name` |
| `resizable` | 默认 `true`;设为 `false` 时窗口锁定为声明的尺寸(固定棋盘的游戏、HUD) |
| `chromeless` | 无标题栏/边框——挂件/悬浮窗风格;通过应用自身的拖拽区域拖动 |
| `transparent` | 透明背景 → **异形窗口**:由应用自己渲染的内容决定可见形状,其余区域点击穿透 |
| `hideLog` · `hideTray` | 不弹出活动日志窗口 / 不显示菜单栏托盘图标 |

优先级(高者生效):`qorm.config.json` 的 `window` → `qorm.json` 的
`platforms.desktop.window` → `qorm.json` 顶层 `display`。顶层 `display` 块是向后
兼容的写法(仅几何属性);`qorm.config.json` 内的 `display` 块同样仍被接受 ——
两块同时存在时按键合并(`window` 只覆盖它声明的键)。config 是覆盖层,因此显式写
`"width": 0` 会把清单声明的尺寸重置回流式自适应。`chromeless` + `transparent`
组合即异形(自定义形状)窗口 —— 在 macOS 的 **WebView** 宿主(`-tags desktop`)上
完整支持;默认的纯 Go canvas 窗口只遵循尺寸/resizable,不遵循 chrome 标志。
Windows 上 `chromeless` 生效(`transparent` 待实现);仍为 `resizable` 的无边框
窗口会保留一条细的边缘调整框 —— 那个框正是 Windows 上窗口可被用户调整大小的位。
Linux 上两者均可解析但去装饰尚未接线。配置文件格式错误会作为加载诊断报告,而不是
被静默应用。

## 实时开发(热重载)

`qorm run <目录>` 会监视应用文件夹:编辑场景、动作或清单并保存,所有已连接的
浏览器/窗口会立即更新——无需重启。重载会保留当前会话(你正在进行的状态、当前
场景和视口都不变),停在原地。半途保存导致解析失败时会给出提示并保留正在运行
的应用,直到下一次成功保存。加 `--no-watch` 可关闭。

## `scenes/` —— 屏幕

每个场景是一个 JSON 文件:`{ "type": "scene", "id": …, "root": <节点> }`。`root`
是一棵节点树——节点结构见[节点与组件属性](/api/props.md),每种 `type` 见
[组件目录](/api/widgets.md)。用 `navigate` 步骤在场景间跳转,见
[导航](/api/navigation.md)。

`root` 旁边还有两个可选键:`onEnter` 指定每次进入该场景时运行的动作,`guard` 声明
进入该场景的前置条件——见[导航与路由](spec/navigation-spec.md)。

## `actions/` —— 行为

每个动作是 `{ "type": "action", "id": …, "steps": [ … ] }`,由节点的 `onPress` /
`onChange` 引用。步骤修改状态、调用后端或导航——完整词汇见
[动作与状态](/api/actions.md)。

## `native/` —— 应用自己的代码

可选。一个 Go 文件(`native/desktop.go`)通过 [`pkg/qormext`](/api/go-api.md)
注册应用**自己的**原生操作;打包器把它编译进桌面二进制**和**移动/web WASM,因此同一份
自定义逻辑在所有目标上运行。`native/web.js` 可加入仅浏览器的操作。这是应用的扩展点——
见[中间层指南](platforms/native-middlelayer.md)。

## 应用文件夹里**没有**什么

运行时、渲染器、打包器,以及内置(vendored)的 WebView,都是 QORM 的职责,而非应用的——
应用永远不携带工具链。应用文件夹只包含它自己的声明,外加那一个可选的原生文件。
