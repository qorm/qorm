# ADR 0004: GPU-first Rendering

## Status

Accepted

## Context

QORM 需要支持普通 UI、移动端动态 UI、实时 UI 和游戏 HUD。

## Decision

QORM 渲染层采用 GPU-first 架构，使用 Display List、Render Graph、Dirty Tree、缓存和批处理。

## Consequences

优点：

- 支持高性能前台渲染。
- 支持游戏 HUD / Overlay。
- 支持 external surface。
- 能跨平台映射到不同 Renderer。

代价：

- 渲染系统复杂度高于 DOM-only 模式。
- 需要额外处理文本、输入、缓存和资源管理。

## Scope

QORM 支持游戏 UI，不做完整游戏引擎。
