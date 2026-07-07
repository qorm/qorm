# ADR 0003: No Native JIT

## Status

Accepted

## Context

QORM 可能在桌面、移动、Web 场景运行。用户关心前台渲染效率，而不是任意脚本执行速度。

## Decision

QORM 不实现 Native JIT。

执行层使用：

```text
Typed IR
Execution Plan
Interpreter
Dependency Graph
Dirty Tracking
```

## Consequences

优点：

- 降低复杂度。
- 更适合移动端和动态 Bundle。
- 避免平台合规风险。
- 架构更简单。

代价：

- 高频表达式执行不能通过机器码加速。

## Mitigation

性能通过以下方式保证：

```text
缓存
增量更新
表达式依赖图
布局 dirty tree
Display List diff
GPU-first rendering
```
