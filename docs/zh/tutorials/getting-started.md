<!-- data-lang-nav --> [English](../../tutorials/getting-started.md) · 中文

# QORM 快速上手

本教程将从零构建一个最小的 QORM 应用:计数器。三个文件 —— 清单、场景和动作 —— 再加上 `qorm run` 就能把它跑起来。

## 目录结构

一个 QORM 应用就是一个目录:`qorm.json` 是清单,`scenes/` 存放 UI,`actions/` 存放动作。

```text
my-app/
├─ qorm.json
├─ scenes/
│  └─ main.json
└─ actions/
   └─ increment.json
```

## qorm.json —— 清单

声明应用元数据、入口场景(`entry`)以及全局状态(`globalState`:一份 schema 加上初始值)。

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

## scenes/main.json —— UI

以节点树的形式声明 UI。文本内容放在 `text` 字段里,`{{ state.count }}` 会插值全局状态;按钮通过 `onPress` 触发一个动作(字符串即动作名)。

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

## actions/increment.json —— 动作

一个动作是一系列步骤。这里 `state.set` 把 `count` 设为 `{{ state.count + 1 }}` —— `{{ … }}` 内部是一个完整的表达式,可以读取全局状态并进行算术运算。

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

指向应用目录(而不是单个文件):

```bash
qorm run my-app          # opens live in the browser; click +1 and the count increments
```

服务器托管应用、处理按钮事件、重新运行动作,并把重新渲染后的 UI 换回页面 —— 这就是运行循环。

## 渲染静态快照

不启动浏览器就渲染一份静态 HTML 快照(适合 CI / 预览):

```bash
qorm render my-app -o my-app.html
```

## 下一步

- [组件目录](/api/widgets.md) —— 渲染器接受的每一种节点类型(代码生成,权威来源)。
- [组件目录](/api/widgets.md) —— 所有可用的节点类型(自动生成)。
- [能力](../../platforms/capabilities.md) —— 相机、定位、蓝牙等原生能力。
- [用户中间层](../platforms/native-middlelayer.md) —— 用一个 Go 文件添加你自己的原生操作。
- 更多可运行示例,参见仓库中的 [`examples/`](https://github.com/qorm/qorm/tree/main/examples)(counter / todo / dashboard / hardware / …)。
