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

## 动画组件

对于值驱动(而非入场)的运动,使用 Flutter 风格的组件:

- `animatedcontainer` / `animatedpadding` / `animatedalign` / `animatedpositioned`
  ——每当绑定值变化时平滑过渡样式(`duration`、`curve`)。
- `animatedopacity` ——把子节点淡入到绑定的 `opacity`(0..1)。
- `transform` / `rotatedbox` ——静态的旋转 / 缩放 / 平移。
- `motion`(以及 `fadetransition`、`slidetransition`、`scaletransition`、
  `rotationtransition`、`sizetransition`、`hero`、`animatedswitcher`)——与专门的
  包裹组件相同的入场效果。

普通的 `transition` 样式属性(如 `"transition": "all .2s"`)也适用于任意节点,
用于简单的 CSS 过渡。
