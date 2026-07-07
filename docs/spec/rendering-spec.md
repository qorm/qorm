# QORM Rendering Specification

QORM 渲染层必须是 GPU-first、缓存驱动、增量更新的高性能架构。

## 渲染目标

- 普通应用 UI。
- 移动端动态 UI。
- 实时动效 UI。

QORM 不做完整游戏引擎。

## 渲染管线

```text
State Change
  ↓
Dirty Marking
  ↓
Style Resolve
  ↓
Incremental Layout
  ↓
Display List Diff
  ↓
Render Graph Build
  ↓
GPU Submit
  ↓
Present
```

## Display List

Display List 是 2D UI 的绘制命令列表：

```text
FillRect
StrokeRect
DrawText
DrawImage
DrawPath
PushClip
PushLayer
PopLayer
SetOpacity
Transform
Shadow
Blur
```

组件不直接绘制到屏幕，而是生成 Display List。

## Render Graph

复杂实时场景使用 Render Graph：

```text
pass: base_layer
pass: overlay
pass: post_effect
pass: composite
```

Render Graph 用于：

- 多 pass 渲染。
- external texture。
- overlay 合成。
- blur、mask、post effect。

## Render Profile

```text
document      文档/表单/app 基础 UI
app           通用应用 UI
realtime      动效密集 UI
```

## 缓存

必须支持：

```text
Resolved Style Cache
Layout Cache
Display List Cache
Text Layout Cache
Glyph Cache
Image Cache
Texture Atlas
Hit Test Index
```

## Batching

渲染命令应按以下维度合并：

```text
pipeline
texture
clip
blend mode
shader
z-order
```

## 文本模式

```text
rich_text  高质量多语言排版
fast_text  数字、标签等高频文本
```

## 性能原则

- 每帧不解析 JSON。
- 每帧不全量 layout。
- 每帧不全量重建 Display List。
- 动画默认不触发布局。
- 高频文本使用 fast_text 和缓存。
- 图片、sprite、icon 使用 texture atlas。
