# 工程结构

一个 QORM 应用就是一小撮 JSON 文件——没有构建步骤,没有打包器。运行时直接加载这个
文件夹(`qorm run <目录>`),打包器把同一个文件夹变成桌面应用、移动应用或 PWA。一个
可选的 Go 文件即可加入编译进所有目标的原生代码。

```
myapp/
  qorm.json            清单——唯一必需的文件
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
    "desktop": { "window": { "width": 500, "height": 700, "icon": "assets/icon.png" } }
  }
}
```

| 键 | 含义 |
|---|---|
| `id` · `name` | 应用标识与显示名 |
| `entry` | 首先显示的场景 id |
| `theme` | `apple` / `material` / `dark`,或 `auto`(默认——跟随系统明暗的 Apple 配色) |
| `globalState` | 供 `state.*` 使用的 `schema`(类型化结构)+ `initial`(初始值) |
| `components` | 可复用的组件定义(或一个组件文件夹) |
| `platforms` | 各平台配置——桌面 `window`、以及打包选项 |
| `defaultLocale` | 多语言应用的初始语言 |

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
