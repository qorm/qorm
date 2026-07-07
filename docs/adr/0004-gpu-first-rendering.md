# ADR 0004: GPU-first Rendering

> 历史说明：本 ADR 最初的主要驱动力之一是游戏 HUD / GPU 渲染场景。QORM 已放弃游戏支持，相关理由已从本文移除；本 ADR 作为历史记录保留。

## Status

Accepted

## Context

QORM 需要支持普通 UI、移动端动态 UI、实时 UI。

## Decision

QORM 渲染层采用 GPU-first 架构，使用 Display List、Render Graph、Dirty Tree、缓存和批处理。

## Consequences

优点：

- 支持高性能前台渲染。
- 能跨平台映射到不同 Renderer。

代价：

- 渲染系统复杂度高于 DOM-only 模式。
- 需要额外处理文本、输入、缓存和资源管理。

## Scope

QORM 不做完整游戏引擎。
