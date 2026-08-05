# QSS — QORM 样式表

QSS 是 QORM 的样式表语言:类 CSS 的规则语法,用于跨场景共享样式,不必在每个节点上重复写内联 `style` 块。

样式表存放在 `styles/<id>.qss`,由运行时在加载期读取。俄罗斯方块示例(`examples/tetris/styles/app.qss`)是完整、可运行的参考——一个游戏的整个外观由一张样式表驱动。

## 规则语法

```qss
# 这是注释

/* 选择器:类型、类、id */
button { borderRadius: 12 }
.accent { background: var(--primary); color: var(--on-primary) }
#submit { fontSize: 16 }

/* 嵌套对象与 style 值一样留在节点内联 */
.card { margin: { top: 8, bottom: 8 } }

/* {{绑定}} 与内联 style 值一样求值 */
.statValue { color: {{ state.dark ? "#fff" : "#111" }} }
```

- **类型选择器**(`button`、`text`、`box` …)匹配该类型的节点。
- **类选择器**(`.accent`)匹配 `class` prop 列出该类名的节点(空格分隔;prop 中后列出的类胜出)。
- **ID 选择器**(`#submit`)匹配节点的 `id`。
- **`#` 注释**——数字保持数字,字符串与 `{{绑定}}` 与内联 style 值一样求值。

## 级联

样式解析是逐键级联,后者覆盖前者:

```text
主题组件默认 < 类型规则 < 类规则 < id 规则 < 内联 `style`
```

- 同一类名内,声明顺序胜出;节点自己的 `class` 列表顺序决定类之间胜负。
- 节点上的内联 `style` 永远压过所有规则。

## 使用样式表

场景节点用 `class` prop 引用规则:

```json
{ "type": "text", "text": "TETRIS", "class": "title" }
```

## Canvas 渲染

原生 canvas 后端(`-tags desktop`)在 measure 阶段应用 QSS 规则——主题组件默认,然后是类型/类/id 规则,最后内联 style——与上述级联完全一致。HTML 渲染器仅使用内联样式。

## 诊断

解析错误是加载期诊断,同时指名文件和行号(`[Stylesheet: app] app.qss:3: …`)。未知样式键(对照 `render.KnownStyleKeys`)是警告。错误前后的规则照常加载,与场景带着自身诊断继续加载一样——一条坏规则永远不会让应用白屏。

## 加载器契约

- `.qss` 文件只在 `styles/` 目录内被收集。
- 它的 id 是文件名(去掉 `.qss`):`styles/app.qss` → 样式表 `app`。
- 重复 id 在目录加载时是错误诊断(先者胜出),`qorm build` 直接拒绝。
- 原始源码保留在应用上,序列化器逐字写回——与组件文档相同的固定点特性。
