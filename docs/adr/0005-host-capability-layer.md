# ADR 0005: Host Capability Layer

## Status

Accepted

## Context

QORM 是 UI 层，不可能实现所有底层能力。桌面、移动、Web 和插件都有不同系统 API。

## Decision

引入 Host Capability Layer。

QORM Action 通过 `host.call` 调用底层能力，底层由 Platform Pack、Native Bridge、Plugin 或 Web Adapter 实现。

## Consequences

优点：

- UI 层和底层能力解耦。
- Agent 可以检查能力是否支持。
- 权限可声明、可校验。
- 多平台适配更清晰。

代价：

- 需要维护 Capability Manifest。
- 每个平台需要实现 Host Adapter。
