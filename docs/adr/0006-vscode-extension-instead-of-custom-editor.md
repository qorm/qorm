# ADR 0006: VS Code Extension Instead of Custom Editor

## Status

Accepted

## Context

QORM 需要编辑体验、诊断、补全、预览和 Agent 协作。但自研完整编辑器成本高，且开发者已经使用 VS Code-compatible 编辑器。

## Decision

QORM 不自研完整编辑器。提供：

```text
VS Code Extension
LSP
MCP Server
CLI
```

## Consequences

优点：

- 降低研发成本。
- 兼容现有开发工作流。
- 通过 LSP 和 MCP 对接多种 Agent。
- 可用 Webview 提供预览和 Inspector。

代价：

- 深度编辑体验受扩展平台限制。
- 非 VS Code-compatible 环境需要额外适配。
