# Getting Started with QORM

本教程从零建一个最小 QORM 应用:一个计数器。三个文件——清单、场景、动作——`qorm run`
直接跑起来。

## 目录结构

一个 QORM 应用就是一个目录:`qorm.json` 是清单,`scenes/` 放界面,`actions/` 放动作。

```text
my-app/
├─ qorm.json
├─ scenes/
│  └─ main.json
└─ actions/
   └─ increment.json
```

## qorm.json — 清单

声明应用元信息、入口场景(`entry`)、以及全局状态(`globalState`:一个 schema + 初始值)。

```json
{
  "type": "app",
  "id": "my_app",
  "name": "My App",
  "entry": "main",
  "globalState": {
    "schema":  { "count": "number" },
    "initial": { "count": 0 }
  }
}
```

## scenes/main.json — 界面

用节点树声明 UI。文本用 `text` 字段,`{{ state.count }}` 把全局状态插进来;按钮用
`onPress` 触发一个动作(字符串就是动作名)。

```json
{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style":  { "padding": 32, "gap": 16 },
    "layout": { "width": "fill", "height": "fill", "align": "center", "justify": "center" },
    "children": [
      { "type": "text",   "id": "count_text", "text": "Count: {{ state.count }}" },
      { "type": "button", "id": "inc", "text": "+1", "onPress": "increment" }
    ]
  }
}
```

## actions/increment.json — 动作

一个动作是一串步骤。这里 `state.set` 把 `count` 设为 `{{ state.count + 1 }}`——`{{ … }}`
里是完整表达式,能读全局状态、做算术。

```json
{
  "type": "action",
  "id": "increment",
  "steps": [
    { "type": "state.set", "path": "count", "value": "{{ state.count + 1 }}" }
  ]
}
```

## 运行

指向应用目录(不是单个文件):

```bash
qorm run my-app          # 在浏览器里实时打开;点 +1,计数自增
```

服务器托管应用、处理按钮事件、重跑动作、把重渲染的 UI 换回页面——这就是运行回路。

## 渲染静态快照

不启动浏览器,渲染一张静态 HTML 快照(适合 CI / 预览):

```bash
qorm render my-app -o my-app.html
```

## 下一步

- [组件目录](../reference/widgets.md) —— 渲染器接受的每一个节点 type(代码生成,权威)。
- [组件目录](../reference/widgets.md) —— 所有可用节点类型(自动生成)。
- [能力清单](../platforms/capabilities.md) —— 摄像头、定位、蓝牙等原生能力。
- [用户中间层](../platforms/native-middlelayer.md) —— 用一份 Go 文件加你自己的原生 op。
- 更多可运行示例见仓库 [`examples/`](../../examples)(counter / todo / dashboard / hardware / …)。
