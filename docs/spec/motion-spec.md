# QORM Motion Specification

Motion 是 QORM 的声明式动效系统。

## 目标

- 跨平台。
- 可被 Agent 理解。
- 默认不触发布局。
- 可用于普通 UI、实时 UI、游戏 HUD。

## Motion 定义

```json
{
  "qorm": "0.1",
  "type": "motion",
  "id": "basic_motion",
  "motions": {
    "button.tap": {
      "from": { "scale": 1 },
      "to": { "scale": 0.96 },
      "duration": 80,
      "easing": "easeOut",
      "affectsLayout": false
    }
  }
}
```

## Motion 对象模型

V1 支持两种定义方式：

1. `from` + `to` + `duration`
2. `timeline` 关键帧

两者可同时存在；若同时存在，`timeline` 优先作为显式关键帧定义。

## 可动画属性

V1 的主属性列表：

```text
opacity
x
y
transform
scale
scaleX
scaleY
rotate
color
backgroundColor
borderColor
radius
shadow
clip
progress
```

说明：
- `transform` 是复合抽象属性。
- `x` / `y` 可视为平移抽象属性。
- 只有出现在该列表中的属性才属于 V1 保证支持范围。

## Layout 影响

默认：

```json
{
  "affectsLayout": false
}
```

只有明确设置：

```json
{
  "affectsLayout": true
}
```

才允许触发布局。

V1 建议只有会改变几何尺寸或文本度量的属性才使用 `affectsLayout: true`。`opacity`、`transform`、`clip`、`progress` 等属性默认不得触发布局。

## Timeline

```json
{
  "name": "damage.flash",
  "timeline": [
    { "at": 0, "props": { "opacity": 1, "scale": 1 } },
    { "at": 80, "props": { "opacity": 0.5, "scale": 1.08 } },
    { "at": 160, "props": { "opacity": 1, "scale": 1 } }
  ],
  "affectsLayout": false
}
```

### MotionFrame

每个关键帧包含：

```text
at      毫秒时间点
props   属性值集合
easing  可选，覆盖区间 easing
```

约束：
- `at` 必须单调递增。
- 第一帧建议从 `0` 开始。
- `timeline` 中每帧的 `props` 只需声明变化属性。

## 插值规则

- number 属性做线性或 easing 插值。
- color 类属性做颜色插值。
- `shadow`、`clip` 等复合属性只有在平台实现明确支持时才允许插值；否则使用 `fallback`。
- 平台不支持的属性不能静默越权执行。

## 并发与中断

V1 默认规则：
- 同一节点同一属性上的新 motion 替换旧 motion。
- 同一节点不同属性的 motion 可并行。
- 节点被移除时，其挂载 motion 自动终止。
- 不定义 pause / resume / reverse 作为 V1 必需能力。

## Game UI 动效

游戏 HUD 动效应优先使用：

```text
opacity
transform
progress
color
```

避免高频修改：

```text
width
height
fontSize
children
gap
padding
```

## Fallback

平台不支持某些动效时可降级：

```json
{
  "name": "glass.enter",
  "from": { "opacity": 0, "shadow": 12 },
  "to": { "opacity": 1, "shadow": 0 },
  "fallback": {
    "from": { "opacity": 0 },
    "to": { "opacity": 1 }
  }
}
```

- `fallback` 必须仍然满足相同的语义目标。
- `fallback` 不得引入更高权限或更重的布局代价。