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

## 结构 · 样式 · 逻辑(与 canvas 新能力)

三层共用同一套样式词汇:

| 层 | 位置 | 职责 |
|---|---|---|
| 结构 | `scenes/*.json` | 节点树、`class`、一次性内联 `style` |
| 样式 | `styles/*.qss` | 共享规则——**包含全部 canvas FX 键**(`filter`、`clipPath`、`layoutMotion`、`scrollSnapType`、弹簧 `transition` 等) |
| 逻辑 | `actions/*.qs`(或 JSON steps) | 改写 `state`;QSS/`style` 绑定会重新求值 |

**QSS** 接受与内联 `style` 相同的键(`render.KnownStyleKeys`)。规则体可写数字、
字符串、`var(--x)` 与 `{{绑定}}`——每帧求值,与内联一致。嵌套对象
(`margin: {top: …}`)仍写在节点上。

**qscript** 不直接写样式。写状态,再绑定:

```qss
/* styles/app.qss */
.filterCard {
  filter: {{ state.filterOn ? "saturate(0.3) brightness(1.15)" : "none" }}
}
.flipChip {
  x: {{ state.flipLeft ? 16 : 280 }}
  layoutMotion: true
  transition: 0.35s spring
}
```

```
# actions/toggle_filter.qs
state.filterOn = !state.filterOn
```

```json
{ "type": "box", "id": "filter_card", "class": "filterCard", "children": [ … ] }
```

端到端可运行示例:[`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx)
(`styles/app.qss` + `actions/*.qs`)。游戏外观的同款分离见
[`examples/tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris)。

## 渲染

两个后端使用相同级联(主题组件默认 < 类型 < 类 < id < 内联)。原生 canvas
后端(macOS 默认纯 Go 窗口;游戏 WASM 使用 `qorm_canvas`)在 measure 阶段合并
匹配规则;HTML 路径把规则并入节点发出的内联 CSS(`boxCSS` / `textCSS`)。HTML
路径上的组件 chrome 默认值(按钮变体、shell 主题变量)位于 QSS 之下,与 canvas
主题组件默认值的层级一致。

## 可接受的样式键

加载器白名单是 `render.KnownStyleKeys`(约 100 个键)。未知键是加载期**警告**
(应用仍会运行)。完整分组列表见自动生成的[通用样式属性](/api/zh/props.md#通用样式属性)。

HTML 与 canvas 共同生效的包括盒模型、文字颜色/字号/字重、伪状态
(`hover*` / `pressed*` / `disabled*`)、以及 `backdropBlur` / `backdropTint`。
仅 canvas(软件光栅)的视觉效果见下。

在 flex 尺寸中,内联样式、QSS 和节点 `layout` 对象都接受
`width: "fill"` / `height: "fill"`。Canvas 将它解析为父容器内容盒减去
该子节点 margin,即使父容器交叉轴对齐不是 stretch 也一样。HTML 发出
`100%`,随后按常规 CSS 尺寸与 margin 规则处理。数字尺寸仍是像素。

Canvas 还会从内联 `style` 或 QSS 解析 CSS 式几何:

- `aspectRatio` 表示宽/高。恰好一个正数轴显式设置时,Canvas 推导另一个轴
  (`width: 120; aspectRatio: 1.5` 得到高度 80)。两轴都显式设置或都未设置时,
  作者尺寸/固有尺寸保持权威;之后再应用 min/max 约束。
- `position: "absolute"` 让节点脱离容器流。用 `left`/`top`(或 Canvas 别名
  `x`/`y`)从起始边锚定,或用 `right`/`bottom` 从父内容盒的尾边锚定。同一轴上,
  显式的起始边值优先于尾边锚点。
- `cursor` 把 `pointer`(或 `hand`)、`text`(或 `ibeam`)、`not-allowed`(或
  `forbidden`)和 `default`(或 `arrow`)映射到原生光标。完整 QSS 级联生效;
  未显式设置时,Canvas 按悬停组件推导 pointer/text/disabled 光标。

## Canvas 视觉效果

纯 Go canvas 后端消费的声明式样式键,可写在任意节点的内联 `style` 或 QSS 中。
可运行展示:[`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx)。

### 描边、阴影、outline

```json
{
  "style": {
    "background": "#1c1c1e",
    "borderRadius": 12,
    "boxShadowColor": "#00000088",
    "boxShadowBlur": 16,
    "boxShadowX": 0,
    "boxShadowY": 8,
    "boxShadowInset": false,
    "outlineColor": "#0a84ff",
    "outlineWidth": 2,
    "outlineOffset": 4
  }
}
```

- `strokeColor` / `strokeWidth` —— 盒子 RRect 的矢量描边(与 CSS border 不同)
- `boxShadowInset: true` —— CSS inset 内阴影
- `outline*` —— 边框盒外侧的外环(焦点式 chrome)

### 文字描边、阴影、装饰

```json
{
  "type": "text",
  "text": "标题",
  "style": {
    "fontSize": 28,
    "fontWeight": "700",
    "textDecoration": "underline",
    "textTransform": "uppercase",
    "textStrokeColor": "#000",
    "textStrokeWidth": 2,
    "textShadowColor": "#00000066",
    "textShadowBlur": 4,
    "textShadowX": 0,
    "textShadowY": 2,
    "lineClamp": 2
  }
}
```

- `textDecoration`:`underline` / `line-through` / `overline`
- `textTransform`:`uppercase` / `lowercase` / `capitalize`
- `textOverflow: "ellipsis"` + 多行 `lineClamp`

### 渐变

`background` / `gradient` 支持:

- `linear-gradient(...)` 与 `radial-gradient(...)`(支持 stop 百分比)
- `conic-gradient(from 0deg, #f00, #00f)` —— 自盒子中心扫角(canvas)

### 滤镜、混合、蒙版、裁剪

```json
{
  "style": {
    "filter": "blur(8px) brightness(1.1) saturate(1.2)",
    "mixBlendMode": "multiply",
    "maskFade": "right",
    "maskFadeSize": 40,
    "clipPath": "circle(50%)",
    "layerCache": true,
    "overflow": "hidden"
  }
}
```

| 键 | 含义 |
|---|---|
| `filter` | CSS 滤镜栈:`blur()` `brightness()` `contrast()` `saturate()` `grayscale()` `hue-rotate()` `opacity()` `drop-shadow()` `invert()` `sepia()` |
| `blur` / `filterBlur` | 组模糊半径简写(px) |
| `mixBlendMode` | 离屏层合成时:`multiply` / `screen` / `overlay` / `darken` / `lighten` / `difference` / `exclusion` / `color-dodge` / `color-burn` / `hard-light` / `plus-lighter` / `lighter`(`lighter` 是 `plus-lighter` 的 Porter-Duff 别名:`min(1, Cs+Cb)`) |
| `maskFade` + `maskFadeSize` | 边缘软溶解(`top` / `bottom` / `left` / `right`) |
| `maskImage` | 如 `linear-gradient(to bottom, black, transparent)` |
| `clipPath` | `circle(50%)` / `ellipse(50% 40%)` / `inset(10px round 12px)` / `polygon(50% 0%, 100% 100%, 0% 100%)`(可选 `evenodd` / `nonzero` 填充规则) |
| `layerCache` | 内容指纹未变时复用离屏位图 |
| `overflow: "hidden"` | 子节点裁到盒子(有 `borderRadius` 时圆角裁剪) |
| `tint` | 子树离屏层 RGB 调制(Godot `modulate` / Phaser tint)。alpha 为 0 表示关闭 |
| `imageRendering` | `pixelated` 在非整数缩放时仍用最近邻(像素画) |

### 变换(canvas)

持久视觉变换——布局盒不变(Godot `rotation` / `scale`,Phaser `setFlip`):

```json
{
  "style": {
    "rotate": 15,
    "scale": 1.2,
    "scaleX": 1,
    "scaleY": 1,
    "flipX": true,
    "flipY": false,
    "skewX": 12,
    "skewY": 0,
    "transformOrigin": "left top"
  }
}
```

- `rotate` — 绕 `transformOrigin` 的角度(度;默认中心)
- `scale` / `scaleX` / `scaleY` — 0 表示未设置(按 1)
- `flipX` / `flipY` — 对应轴取负
- `skewX` / `skewY` — 剪切角(度;CSS skew;图剪切)。布局盒不变
- `transformOrigin` — rotate / scale / flip / skew 的 CSS 枢轴:`center`、`left top`、`50% 0`、`12px 8px`。空 / 省略 = 中心。布局盒不变
- 与入场 `animation`、`fx`、以及 `pressedScale` / `hoverScale` 叠加

### 层叠(`zIndex`, canvas)

`zIndex` 早已在 `render.KnownStyleKeys` 中(HTML 发出 CSS `z-index`)。canvas 后端现在实现它:兄弟节点的**绘制与命中顺序**。

```json
{
  "type": "stack",
  "children": [
    { "id": "z_back", "type": "box", "style": { "zIndex": 1, "background": "#0a84ff" } },
    { "id": "z_front", "type": "box", "style": { "zIndex": 2, "background": "#ff375f" } }
  ]
}
```

- `0`(或缺省 / `"auto"`) = auto——同 z 兄弟按文档顺序(后者绘制并命中在上)
- 数值大的绘制(与命中)在上;负值画在 auto 兄弟后面
- 布局盒不变——只改兄弟绘制/命中顺序
- Measure 报告数字 `zIndex`,为 0 时报告 `"auto"`

### 滚动吸附

在 `scroll` / `scrollview` 视口上:

```json
{ "type": "scroll", "style": { "scrollSnapType": "y mandatory", "height": 320 }, "children": [
  { "type": "box", "style": { "height": 320, "scrollSnapAlign": "start" }, "children": [ … ] }
] }
```

- `scrollSnapType`:`x|y|both` + `mandatory|proximity`
- 子节点 `scrollSnapAlign`:`start` / `center` / `end`
- 拖拽松手或惯性滑行后吸附(轮播 / 分页风格)

### 交互 + 弹簧过渡

```json
{
  "style": {
    "pressedScale": 0.96,
    "hoverScale": 1.02,
    "hoverColor": "var(--label)",
    "hoverOpacity": 0.9,
    "pressedBackground": "var(--accent)",
    "pressedOpacity": 0.8,
    "focusBorderColor": "var(--accent)",
    "transition": "0.3s spring",
    "transitionEasing": "spring"
  }
}
```

- `hoverBackground` / `pressedBackground`、`hoverColor` 与
  `hoverOpacity` / `pressedOpacity` 使用完整 QSS 级联(类型 < class < id < 内联),
  不仅限内联样式
- `focusBorderColor` 设置原生 canvas 焦点环颜色
- `disabled: true` 阻止指针/键盘激活,并使用 `disabledOpacity`
  (默认 0.5)和禁止光标
- `transition: "0.2s"` / `"200ms"` —— 缓动交互效果与绝对 `x`/`y` 位移
- `transition: "0.3s spring"` 或 `transitionEasing: "spring"` —— canvas 路径上
  的欠阻尼弹簧(过冲后回落)
- 入场效果与 FLIP 布局动效见[动画](/api/zh/animation.md)

### 游戏反馈 FX(`fx` 属性)

Canvas 一次性反馈,对标 DOTween / Phaser / Godot:

```json
{
  "fx": "hit",
  "fxToken": "{{ state.hits }}",
  "fxDuration": 320,
  "fxIntensity": 12
}
```

```
# actions/on_damage.qs — 不重挂载节点即可重播
state.hits = state.hits + 1
```

名称:`shake`、`punch`、`flash`/`blink`、`hit`、`float`/`bob`、`wobble`、
`knockback`、`burst`。完整表见[动画 — 游戏反馈 FX](/api/zh/animation.md#游戏反馈-fxfx-属性canvas)。

过渡缓动也接受游戏引擎名:`backOut`、`elastic`、`bounce`、`quadOut`、
`sineOut`、`expoOut` …

属性补间还支持 DOTween 式循环:`transitionYoyo`、`transitionLoop`、
`transitionRepeat`。

### 时间轴序列

任意节点上的 DOTween Sequence / Godot Tween 链——默认 Append,
`"parallel": true` 为 Join;用 qscript 递增 `timelineToken` 重播:

```json
{
  "timeline": [
    { "scale": 1.3, "duration": 180, "ease": "backOut" },
    { "dx": 48, "duration": 200, "ease": "easeOut", "parallel": true },
    { "wait": 80 },
    { "scale": 1, "dx": 0, "duration": 200, "ease": "easeInOut" }
  ],
  "timelineToken": "{{ state.tlPlay }}"
}
```

完整表见[动画 — 时间轴](/api/zh/animation.md#时间轴序列timeline-属性canvas)。

另有:`path` 步骤(折线/三次贝塞尔 + `orient`)、`timelineOnComplete`、列表
`stagger`(入场/fx/timeline 按 index×ms 延迟)、timeline `{ yoyo, loop }`。

### FLIP 布局动效

```json
{
  "id": "chip",
  "type": "box",
  "style": {
    "position": "absolute",
    "x": "{{ state.chipX }}",
    "y": 40,
    "layoutMotion": true,
    "transition": "0.35s"
  }
}
```

当 `layoutMotion` 为真、节点有稳定 `id` 且设置了 `transition` 时,绝对位置/
尺寸跳变会缓动而非瞬切(共享元素风格)。演示:`examples/canvas-fx` 的 FLIP 芯片。

### 横版与瓦片世界(`board` + `tilemap`)

把 `board` 当**世界平面**,不要用 `row`/`column`/`list` 铺整张地图。引擎镜头
跟随目标;`tilemap` 把字符网格 + 图集烘焙成**一张**世界位图(`rows` 或 bump
变化前会缓存)。

```json
{
  "type": "board",
  "cameraTarget": "{{ state.mario }}",
  "cameraCenter": "x",
  "cameraCell": 32,
  "cameraViewport": 16,
  "cameraDeadZone": 160,
  "cameraLockLeft": true,
  "cameraResetToken": "{{ state.cameraGen }}",
  "cameraMax": { "x": 6240 },
  "disablePan": true,
  "children": [
    {
      "type": "tilemap",
      "id": "tiles",
      "rows": "{{ state.rows }}",
      "cell": 32,
      "bumpX": "{{ state.bumpCX }}",
      "bumpY": "{{ state.bumpCY }}",
      "bumpT": "{{ state.bumpT }}",
      "atlas": { "1": "assets/ground.png", "2": "assets/brick.png" }
    }
  ]
}
```

| 属性 | 含义 |
|---|---|
| `cameraTarget` | 带 `x`/`y`(px)的对象,通常是 `{{state.player}}` |
| `cameraCenter` | `true` / `"x"` / `"y"` |
| `cameraCell` + `cameraViewport` | 例如 `32` 与 `16` → 512 px 跟随窗口 |
| `cameraDeadZone` | 像素;镜头滚动前的 NES 式左侧空档 |
| `cameraLockLeft` | 永不回滚(SMB)。必须搭配 `cameraResetToken` |
| `cameraResetToken` | 重开时递增 `{{state.cameraGen}}`,否则 lock-left 不会回到起点 |
| `cameraMax` | `{ "x": (关卡宽 - 视口宽) * cell }` |
| `disablePan` | 禁止玩家拖动画布(游戏) |

角色仍是同一 `board` 上带绝对 `x`/`y` 的 `image` / `list`。HUD 放在 board
**外面**(stack 叠加)。不要给物理 `x`/`y` 做补间,也不要对 60fps 移动体开
`layoutMotion`。像素画:设 `imageRendering: pixelated`(QSS `image { … }`
即可)。范例:[`examples/mario`](https://github.com/qorm/qorm/tree/main/examples/mario)。
属性:[board / tilemap](/api/zh/props.md)。

## 诊断

解析错误是加载期诊断,同时指名文件和行号(`[Stylesheet: app] app.qss:3: …`)。未知样式键(对照 `render.KnownStyleKeys`)是警告。错误前后的规则照常加载,与场景带着自身诊断继续加载一样——一条坏规则永远不会让应用白屏。

## 加载器契约

- `.qss` 文件只在 `styles/` 目录内被收集。
- 它的 id 是文件名(去掉 `.qss`):`styles/app.qss` → 样式表 `app`。
- 重复 id 在目录加载时是错误诊断(先者胜出),`qorm build` 直接拒绝。
- 原始源码保留在应用上,序列化器逐字写回——与组件文档相同的固定点特性。
