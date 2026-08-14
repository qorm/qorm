# 动画

QORM 的动画是声明式且横切的:任意节点——内置组件**或组件实例**——都能携带一个
`animation` 属性并播放入场效果。入场效果在节点挂载时触发。实时更新会就地变形 DOM,
因此当节点被新建时(例如向绑定列表追加一项)效果会重播,而不是每次状态变化都播。

## `animation` 属性(任意节点)

```json
{ "type": "card", "animation": "fadeup", "duration": 450, "children": [ … ] }
```

对组件实例同样适用:

```json
{ "type": "ProductCard", "animation": "pop", "props": { "name": "Cup" } }
```

调节属性(全部可选):

| 属性 | 默认 | 含义 |
|---|---|---|
| `animation` | — | 效果名(见下);**可绑定**——`"{{state.effect}}"` 让智能体通过改状态切换动画 |
| `duration` | `450` | 毫秒 |
| `delay` | `0` | 开始前的延迟毫秒数(绑定索引可让列表逐项错开) |
| `curve` | `cubic-bezier(.34,1.2,.64,1)` | 缓动曲线 |
| `repeat` | `1` | 播放次数(`infinite` 用于持续吸引注意) |

## 效果

- **入场**:`fade`、`fadeup`、`fadedown`、`slideup`、`slidedown`、`slideleft`、
  `slideright`、`scale`、`zoomout`、`rotate`、`flip`、`pop`。
- **吸引注意**:`bounce`、`shake`、`pulse`、`spin`(配合 `repeat`)。

### `curve`(入场缓动)

可选。入场插值的具名缓动——与 `transitionEasing` 同一注册表(含游戏引擎词汇):

`linear` · `easeIn` / `easeOut` / `easeInOut` · `spring` · `back` / `backOut` ·
`elastic` / `elasticOut` · `bounce` / `bounceOut` · `quadOut` · `sineOut` ·
`expoOut` · …

```json
{ "type": "card", "animation": "pop", "duration": 500, "curve": "backOut" }
```

## 游戏反馈 FX(`fx` 属性,canvas)

一次性 / 短循环的**游戏式反馈**,对标常见 2D 引擎 API(DOTween 的 `DOShake` /
`DOPunchScale`、Phaser 镜头 shake、Godot Tween 单次动画)。与入场 `animation`
(挂载时)不同,**`fx` 在效果名或 `fxToken` 变化时重启动画**——用 qscript 递增
计数器即可触发:

```json
{
  "type": "box",
  "id": "enemy",
  "fx": "hit",
  "fxToken": "{{ state.hits }}",
  "fxDuration": 320,
  "fxIntensity": 12,
  "style": { "width": 48, "height": 48, "background": "#ff375f" }
}
```

```
# actions/on_damage.qs
state.hits = state.hits + 1
```

| 属性 | 默认 | 含义 |
|---|---|---|
| `fx` | — | 效果名(见下);可绑定;`none` / 空 清除 |
| `fxToken` / `fxKey` | — | 重启令牌——变化则重播同一效果 |
| `fxDuration` | 按效果 | 毫秒(未设时回退 `duration`) |
| `fxIntensity` | 按效果 | 幅度(像素、缩放增量或角度) |
| `fxDelay` | `0` | 开始前延迟毫秒 |
| `fxLoop` | 自动 | `true` / `infinite` 强制循环;`float`/`bob`/`blink` 默认循环 |

### FX 名

| 名 | 引擎类比 | 运动 |
|---|---|---|
| `shake` | DOTween DOShake · Phaser cameras.shake | 位置抖动,衰减 |
| `punch` | DOTween DOPunchScale | 缩放弹出再回落 |
| `flash` / `blink` | DOFade 闪烁 | 透明度脉冲 |
| `hit` | 受击组合包 | shake + punch + flash |
| `float` / `bob` | 拾取物悬浮 | 循环纵向正弦 |
| `wobble` | 旋转摆动 | 衰减旋转 |
| `knockback` | 平台受击推开 | 横向推出再回 |
| `burst` | 爆炸包 | 径向位移 + 缩放 + 闪烁(无多 sprite 粒子系统) |

可与入场 `animation`、`transition` / 弹簧按压、FLIP、以及样式 `rotate` /
`scale` / `flipX` / `skewX` / `skewY` 叠加——偏移走同一套变换通道(枢轴:样式
`transformOrigin`,默认中心)。持久样式变换不改变布局盒。

可运行:[`examples/canvas-fx`](https://github.com/qorm/platform/tree/main/examples/canvas-fx)(FX 段)。完整游戏:
[`examples/tetris`](https://github.com/qorm/platform/tree/main/examples/tetris)(局部消行闪白 + SINGLE/DOUBLE/TRIPLE/TETRIS
横幅 + 金色描边;NEXT/SCORE/LINES punch;棋盘本身不 shake / burst)、
[`examples/g2048`](https://github.com/qorm/platform/tree/main/examples/g2048)(仅格子生成/合并变色闪;SCORE punch;
棋盘整体不位移)、[`examples/mario`](https://github.com/qorm/platform/tree/main/examples/mario)(`fxJump`/`fxCoin`/`fxDeath`)、
[`examples/raiden`](https://github.com/qorm/platform/tree/main/examples/raiden)(`fxHit`/`fxBomb`/`fxBoss`,爆炸
`burst`)。物理仍写 `x`/`y`;`fx` 只做视觉偏移。益智棋盘把动效留在格子和 HUD,
不晃整块板。

## 时间轴序列(`timeline` 属性,canvas)

任意节点上的 **DOTween Sequence / Godot Tween** 链。步骤默认 **Append**;
`"parallel": true` 表示 **Join**(与上一步同时开始)。用 qscript 递增
`timelineToken` 重播。

```json
{
  "id": "hero",
  "timeline": [
    { "scale": 1.35, "duration": 180, "ease": "backOut" },
    { "dx": 56, "dy": -6, "duration": 220, "ease": "easeOut", "parallel": true },
    { "wait": 80 },
    { "scale": 1, "dx": 0, "dy": 0, "duration": 240, "ease": "easeInOut" }
  ],
  "timelineToken": "{{ state.tlPlay }}"
}
```

```
# actions/play_timeline.qs
state.tlPlay = state.tlPlay + 1
```

对象写法(整段 loop / yoyo):

```json
{
  "timeline": {
    "yoyo": true,
    "repeat": 2,
    "steps": [
      { "scale": 1.2, "duration": 200, "ease": "sineOut" },
      { "opacity": 0.5, "duration": 200, "ease": "linear", "parallel": true }
    ]
  },
  "timelineToken": "{{ state.tlPlay }}"
}
```

| 属性 | 含义 |
|---|---|
| `timeline` | 步骤数组,或 `{ steps, loop, yoyo, repeat, token }` |
| `timelineToken` / `timelineKey` | 重启令牌 |
| `timelineLoop` / `timelineYoyo` / `timelineRepeat` | 节点级覆盖 |

### 步骤字段

| 字段 | 含义 |
|---|---|
| `duration` / `ms` | 毫秒(也可用 CSS `"0.2s"`) |
| `delay` | 步骤内前置等待 |
| `wait` | 纯停顿(不改通道) |
| `ease` / `curve` | 具名缓动 |
| `parallel` / `join` | 并入上一组(DOTween Join) |
| `scale` `opacity` `dx`/`x` `dy`/`y` `rotation`/`rotate` | 终点值(旋转为度) |
| `path` | 折线 `[[x,y],…]`,或 `"cubic": true` + 4 点三次贝塞尔(DOTween DOPath) |
| `orient` / `orientToPath` | 沿路径切线旋转 |

未写出的通道**保持**上一步姿态。结束后**保持终态**(DOTween 默认),直到下次
token 变化。

### `timelineOnComplete` / `onComplete`

**有限**时间轴结束时(非无限 loop/yoyo)派发一次动作——DOTween `OnComplete` /
Godot `finished`:

```json
{
  "timeline": [ { "scale": 1.2, "duration": 200 } ],
  "timelineToken": "{{ state.tlPlay }}",
  "timelineOnComplete": "timeline_done"
}
```

```
# actions/timeline_done.qs
state.tlDone = state.tlDone + 1
```

也接受 `{ "name": "act", "args": { … } }`。注入参数含 `timeline`(节点 id)与
`token`。

### 路径跟随示例

```json
{
  "timeline": [
    {
      "path": [[0, 20], [60, -10], [120, 30], [160, 10]],
      "duration": 700,
      "ease": "easeInOut",
      "orient": true
    }
  ],
  "timelineToken": "{{ state.pathPlay }}"
}
```

### Stagger(列表)

`stagger`(毫秒 × 列表下标)推迟入场 `animation`、`fx` 与 `timeline`——GSAP
stagger / DOTween `SetDelay(i * step)`:

```json
{
  "type": "list",
  "data": "{{ state.items }}",
  "renderItem": {
    "type": "box",
    "animation": "fadeup",
    "stagger": 80,
    "duration": 400,
    "curve": "backOut"
  }
}
```

### 额外 FX:`burst`

`fx: "burst"`——轻量爆炸包(径向位移 + 缩放 + 闪烁),无需完整粒子系统。

### 样式过渡的 yoyo / loop

属性补间(`style.transition`)支持 DOTween 式循环:

```json
{
  "style": {
    "opacity": "{{ state.pulse ? 0.4 : 1 }}",
    "transition": "0.35s",
    "transitionEasing": "sineInOut",
    "transitionYoyo": true,
    "transitionRepeat": 2
  }
}
```

| 样式键 | 含义 |
|---|---|
| `transitionYoyo` | begin↔target 乒乓 |
| `transitionLoop` | 正向重复(无数次若无 count) |
| `transitionRepeat` | 次数,或 `"infinite"` / `-1` |

## 动画组件

对于值驱动(而非入场)的运动,使用 Flutter 风格的组件:

- `animatedcontainer` / `animatedpadding` / `animatedalign` / `animatedpositioned`
  ——每当绑定值变化时平滑过渡样式(`duration`、`curve`)。
- `animatedopacity` ——把子节点淡入到绑定的 `opacity`(0..1)。
- `transform` / `rotatedbox` ——静态的旋转 / 缩放 / 平移。
- `motion`(以及 `fadetransition`、`slidetransition`、`scaletransition`、
  `rotationtransition`、`sizetransition`、`hero`、`animatedswitcher`)——与专门的
  包裹组件相同的入场效果。

普通的 `transition` 样式属性(如 `"transition": "0.2s"` 或 `"200ms"`)也适用于
任意节点。在原生 **canvas** 后端,它驱动交互效果(`pressedScale`、
`hoverScale`、颜色/透明度切换)、绝对 `x`/`y`(以及 left/top)位移,以及 FLIP
布局动效——不只是 HTML 路径上的 CSS。

### 弹簧缓动(canvas)

```json
{ "style": { "pressedScale": 0.95, "transition": "0.3s spring" } }
```

或等价写法:

```json
{ "style": { "pressedScale": 0.95, "transition": "0.3s", "transitionEasing": "spring" } }
```

欠阻尼弹簧:数值先过冲再回落。具名 CSS 缓动(`easeOut`、`easeInOut` …)与主题
动效 token 照常可用。

### FLIP 布局动效(canvas)

当节点的绝对位置或尺寸发生跳变(例如绑定的 `x` 变化)时,设置
`layoutMotion: true`、稳定 `id` 与 `transition`,canvas 会缓动该跳变而非瞬切:

```json
{
  "id": "chip",
  "type": "box",
  "style": {
    "position": "absolute",
    "x": "{{ state.chipX }}",
    "layoutMotion": true,
    "transition": "0.35s"
  }
}
```

可运行演示:[`examples/canvas-fx`](https://github.com/qorm/platform/tree/main/examples/canvas-fx)(同一场景覆盖
scroll-snap、滤镜、蒙版、clip-path、弹簧按压与 FLIP)。完整样式键列表:
[通用样式属性](props.md#通用样式属性)与
[QSS / canvas 效果](../../docs/zh/styles.md#canvas-视觉效果)。

## 主题动效 token

皮肤除颜色外还携带动效词汇。每个 `themes/*.json` 可声明 `motion` 段;原生 canvas
后端直接消费它,HTML/WebView 后端则以同名 CSS 自定义属性暴露同一套数值:

```json
"motion": {
  "durationFast": 120,
  "durationNormal": 250,
  "durationSlow": 400,
  "easingStandard": "easeOutCubic",
  "easingEmphasized": "easeInOutCubic"
}
```

- 节点未给 `duration` / `curve` 属性时,`animatedcontainer` / `animatedopacity`
  默认采用 `durationNormal` + `easingStandard`——显式属性仍然优先。HTML 端落地为
  `var(--qorm-motion-normal)` / `var(--qorm-motion-standard)`,手写 `transition`
  样式同样可以引用。
- 缓动名:`linear`、`easeIn`、`easeOut`、`easeInOut`(及 cubic 写法)、`spring`、
  游戏引擎族(`back` / `elastic` / `bounce` / `quad` / `sine` / `expo` 及
  `*In` / `*Out` / `*InOut`)、以及主题 token 别名 `standard` / `emphasized`。
- 内建皮肤有意不同:Apple 系约 250ms,WinUI 系约 167ms——切换皮肤会改变应用的节奏感。
- 注意:HTML 端不读取 `themes/*.json`,而是为 `--qorm-motion-*` 变量提供同值的
  *默认值*;逐皮肤的 JSON 数值在原生 canvas 后端生效。
