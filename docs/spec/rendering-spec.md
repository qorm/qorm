# QORM Rendering Specification

QORM 渲染层必须是 GPU-first、缓存驱动、增量更新的高性能架构。

## 渲染目标

- 普通应用 UI。
- 移动端动态 UI。
- 实时动效 UI。
- 游戏 HUD / Overlay。
- 外部游戏 Surface 集成。

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
pass: game_surface
pass: qorm_hud
pass: post_effect
pass: composite
```

Render Graph 用于：

- 多 pass 渲染。
- external texture。
- overlay 合成。
- blur、mask、post effect。
- 游戏 HUD 与游戏画面组合。

## Render Profile

```text
document      文档/表单/app 基础 UI
app           通用应用 UI
realtime      动效密集 UI
game-ui       游戏 HUD / 菜单 / Overlay
game-lite     轻量 2D 互动场景
external-game 外部游戏引擎承载，QORM 以 integration mode 负责 UI 层
```

示例：

```json
{
  "render": {
    "profile": "game-ui",
    "frameDriven": true,
    "targetFps": 60,
    "textMode": "fast"
  }
}
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
fast_text  HUD、数字、标签等高频文本
```

## External Game Integration Mode

`external-game` 不是平台，而是一种 integration mode：宿主游戏引擎负责主画面与内部渲染，QORM 负责 HUD、菜单、Overlay、布局、事件路由和合成策略。

它通常依赖 `externalSurface` / `game.surface` 等 host capability，但这些 capability 仍由 Platform Pack 提供。

## External Surface

QORM 支持外部 Surface：

```json
{
  "type": "externalSurface",
  "id": "game_view",
  "capability": "game.surface",
  "layout": {
    "width": "fill",
    "height": "fill"
  }
}
```

QORM 负责布局、生命周期、事件路由和合成，不负责外部 Surface 内部渲染。

## 性能原则

- 每帧不解析 JSON。
- 每帧不全量 layout。
- 每帧不全量重建 Display List。
- 动画默认不触发布局。
- 游戏 HUD 使用 frame-driven update。
- 高频文本使用 fast_text 和缓存。
- 图片、sprite、icon 使用 texture atlas。
