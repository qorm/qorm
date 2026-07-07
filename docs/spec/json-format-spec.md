# QORM JSON Format Specification

> **规格 vs. 运行时现状。** 本文件是 QORM JSON 的**目标设计草案**(逻辑引用
> `scene://`、`imports`/`targets` 清单、`value`/`on` 等)。**当前 Go 运行时接受的可运行
> 形态**与此有差异——以能跑的为准:
> - 上手与可运行格式:[Getting Started](../tutorials/getting-started.md) 及
>   [`examples/`](../../examples)(`entry:"main"`、`globalState`、文本用 `text`、
>   `onPress` 触发 `actions/`、组件在 `qorm.json` 的 `components` 里用 `{{prop.x}}`)。
> - 节点类型的权威来源(从代码自动生成):[组件目录](../reference/widgets.md)。
> - 能力(从注册表自动生成):[能力清单](../platforms/capabilities.md)。
>
> 需要照着建一个能跑的应用,请从上面三处入手;本规格用于理解设计意图与未来标准。

## 版本

当前草案版本：`0.1`

所有 QORM JSON 文件必须包含：

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "main"
}
```

## 通用字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `qorm` | string | 是 | QORM 格式版本 |
| `type` | string | 是 | 文件类型 |
| `id` | string | 是 | 文件或对象 ID |
| `name` | string | 否 | 人类可读名称 |
| `description` | string | 否 | 描述 |
| `meta` | object | 否 | 元信息 |

ID 在其所属命名空间内必须唯一。`scene`、`component`、`style`、`action`、`motion`、`resource`、`platform` 各自独立命名；同一 scene 内的 node ID 必须唯一。

## `qorm.json`

根文件类型为 `app`。依赖声明、全局状态和生态包规则分别以 `dependency-resolution-spec.md`、`global-state-spec.md`、`asset-package-spec.md` 为准。

```json
{
  "qorm": "0.1",
  "type": "app",
  "id": "demo_app",
  "name": "Demo App",
  "entry": "scene://main",
  "imports": [
    "./scenes/main.json",
    "./styles/theme.json",
    "./actions/counter.json"
  ],
  "targets": ["desktop", "web", "mobile"],
  "render": {
    "profile": "app"
  }
}
```

- `entry` 使用逻辑引用，规范形式为 `scene://<id>`。
- `imports` / `includes` / `embeds` 使用相对文件路径。

## Scene 文件

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "main",
  "state": {
    "count": 0
  },
  "root": {
    "type": "column",
    "id": "root",
    "children": []
  }
}
```

- `state` 表示 scene 初始状态。
- `root` 是 scene 根节点，必须包含 `id`。
- scene local state、global state、context scope 的分层语义以 `global-state-spec.md` 为准。

## Node

```json
{
  "type": "button",
  "id": "submit_button",
  "semantic": "primary_action",
  "text": "提交",
  "layout": {},
  "style": {},
  "on": {},
  "children": []
}
```

通用 Node 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 节点类型 |
| `id` | string | 节点 ID，scene 内唯一 |
| `semantic` | string | 语义角色 |
| `layout` | object | 布局配置 |
| `style` | object/string | 内联样式或样式引用 |
| `on` | object | 事件处理 |
| `children` | array | 子节点 |
| `visibleWhen` | string | 可见条件表达式 |
| `disabledWhen` | string | 禁用条件表达式 |
| `errorBoundary` | object | 局部错误边界声明 |
| `context` | object | 子树 context scope 定义 |

`visibleWhen`、`disabledWhen` 必须是完整表达式字符串，例如 `"{{ count > 0 }}"`。

## Component 文件

组件定义：

```json
{
  "qorm": "0.1",
  "type": "component",
  "id": "panel",
  "props": {
    "title": "string"
  },
  "slots": {
    "header": { "required": false },
    "body": { "required": true }
  },
  "template": {
    "type": "column",
    "id": "panel_root",
    "children": [
      { "type": "slot", "name": "header" },
      { "type": "slot", "name": "body" }
    ]
  }
}
```

组件实例：

```json
{
  "type": "component",
  "id": "settings_panel",
  "ref": "panel",
  "props": {
    "title": "设置"
  },
  "slots": {
    "header": { "type": "text", "value": "设置" },
    "body": { "type": "text", "value": "内容" }
  }
}
```

规则：
- 定义文件中的 `type: "component"` 表示组件定义。
- scene/tree 中的 `type: "component"` + `ref` 表示组件实例。
- `ref` 的规范形式为 `component://panel`，在组件实例字段中可简写为 `"panel"`。
- `props.*` 只在组件模板求值上下文中可见。
- `slots` 在定义中声明名称和约束，在实例中提供填充值。

## Style 文件

```json
{
  "qorm": "0.1",
  "type": "style",
  "id": "theme",
  "tokens": {
    "color": {
      "primary": "#4f46e5"
    }
  },
  "variants": {
    "button.primary": {
      "fill": "color.primary",
      "textColor": "#ffffff"
    }
  }
}
```

## Action 文件

```json
{
  "qorm": "0.1",
  "type": "action",
  "id": "counter_actions",
  "actions": {
    "counter.increment": [
      {
        "type": "state.set",
        "path": "count",
        "value": "{{ count + 1 }}"
      }
    ]
  }
}
```

`actions` 中的 key 为逻辑 action ID。事件处理可以内联 step 数组，也可以通过 `action.call` 引用逻辑 action。

## Motion 文件

```json
{
  "qorm": "0.1",
  "type": "motion",
  "id": "basic_motion",
  "motions": {
    "tap": {
      "from": { "scale": 1 },
      "to": { "scale": 0.96 },
      "duration": 80,
      "easing": "easeOut",
      "affectsLayout": false
    }
  }
}
```

## Resource 文件

```json
{
  "qorm": "0.1",
  "type": "resource",
  "id": "zh-CN",
  "locale": "zh-CN",
  "messages": {
    "app.title": "示例应用"
  }
}
```

## Platform 文件

```json
{
  "qorm": "0.1",
  "type": "platform",
  "id": "mobile",
  "capabilities": {
    "network.request": {
      "supported": true,
      "permission": "network.request"
    },
    "clipboard.write": {
      "supported": true,
      "permission": "clipboard.write"
    }
  }
}
```

布尔值 `true` 可以作为 `{"supported": true}` 的简写；规范写法应使用对象形式，以便声明 `permission`、`requiresApproval`、域名范围等额外约束。

## Patch 文件

```json
{
  "qorm": "0.1",
  "type": "patch",
  "id": "rename_button",
  "patches": [
    {
      "op": "replace",
      "path": "/scenes/main/nodes/submit_button/text",
      "value": "立即提交"
    }
  ]
}
```

Patch path 作用于解析后的逻辑文档模型，而不是源 JSON 的 `children[0]` 之类位置索引。

测试用 `type: "test"` 文件、查询/断言目标和 patch 相关测试行为分别以 `test-runner-spec.md` 与 `query-selector-spec.md` 为准。

## 引用与解析规则

### 文件级引用

以下字段使用相对路径：

```json
{
  "imports": ["./scenes/main.json"],
  "includes": ["./styles/theme.json"],
  "embeds": ["./assets/icons.json"]
}
```

- `import`：引入定义，不展开。
- `include`：构建时合并。
- `embed`：资源嵌入 Bundle。

### 逻辑引用

跨文档逻辑引用的规范形式：

```text
scene://main
component://panel
style://theme
action://counter.increment
motion://button.tap
resource://zh-CN
platform://desktop
```

### 简写规则

在字段语义已经固定的场景中，可使用同命名空间简写：

- `entry: "scene://main"` 必须写全。
- 组件实例 `ref: "panel"` 等价于 `component://panel`。
- `action.call` 中 `name: "counter.increment"` 等价于 `action://counter.increment`。
- `motion.play` 中 `motion: "button.tap"` 等价于 `motion://button.tap`。

### 解析规则

- Resolver 先加载文件路径，再建立逻辑 ID 命名空间。
- 同一命名空间内重复 ID 必须报错。
- 未解析引用必须产生诊断，不能静默忽略。
- IR 中所有逻辑引用都必须被规范化为稳定 ID。

## State Path

QORM 使用 `State Path` 访问和写入 scene 运行时状态。

### 语法

```text
count
form.username
tasks[0].title
player.inventory[2].count
```

规则：
- 使用点号访问对象属性。
- 使用 `[index]` 访问数组元素。
- 不支持通配符、函数调用和任意 JSON Pointer。
- `state.set` 可以覆盖已有路径；是否允许创建缺失中间对象由 Action 规格定义。

## Patch Path

QORM Patch 使用稳定逻辑路径，不使用源文件 children 下标作为规范路径。

### 语法

```text
/scenes/<sceneId>/nodes/<nodeId>/<field>
/scenes/<sceneId>/state/<key>
/components/<componentId>/template/<field>
/motions/<motionId>/<field>
```

示例：

```text
/scenes/main/nodes/submit_button/text
/scenes/main/state/count
/components/panel/slots/header
```

规则：
- scene 和 node 必须优先使用逻辑 ID 定位。
- 对数组状态值可以在状态路径内部使用索引，例如 `/scenes/main/state/tasks[0].title`。
- 运行时 Patch 校验基于逻辑模型和 schema，而不是源文件文本位置。
- 源文件 JSON Pointer 只用于 Source Map 和诊断。

## 表达式

表达式使用 `{{ ... }}` 标记。

### 完整表达式字段

```json
{
  "value": "{{ count + 1 }}"
}
```

当整个字段值恰好是单个表达式时，结果保留原始 JSON 类型：
- `"{{ count + 1 }}"` → number
- `"{{ loading }}"` → boolean
- `"{{ form }}"` → object

### 模板插值字段

```json
{
  "value": "当前数值：{{ count }}"
}
```

当表达式嵌入普通字符串时，表达式结果会被字符串化后插入。`null` 或缺失值在模板中按空字符串处理。

### V1 允许的语法

```text
literals        1, 1.5, true, false, null, "text"
path access     count, form.username, tasks[0].title, props.name, item.done
arithmetic      + - * / %
comparison      == != > >= < <=
logic           && || !
fallback        ??
grouping        ( ... )
```

### V1 禁止的语法

- 函数调用
- 访问系统 API
- 任意脚本执行
- 异步操作
- 随机数、时间、I/O 等副作用

### 求值上下文

Runtime 可以提供以下上下文：

| 名称 | 含义 |
|---|---|
| `state` | scene 当前状态 |
| 顶层状态键 | `state` 的简写，例如 `count` 等价于 `state.count` |
| `props` | 组件实例传入的 props |
| `item` | 列表 / 模板项上下文 |
| `event` | 当前事件输入 |
| `host` | 最近一次 `host.call` 写入的局部结果，仅在显式绑定时可见 |

### 缺失值与布尔上下文

- 缺失路径结果为 `null`。
- `visibleWhen`、`disabledWhen`、`condition.when` 处于布尔上下文。
- V1 的 falsey 值：`false`、`null`、`0`、`""`。
- 其他值按 truthy 处理，但推荐写显式比较表达式。

### 依赖提取

表达式必须可静态提取依赖路径，例如：

```json
{
  "value": "{{ player.hp }} / {{ player.maxHp }}"
}
```

依赖为：

```text
player.hp
player.maxHp
```

这使 Runtime 可以只重新计算受影响的 binding。