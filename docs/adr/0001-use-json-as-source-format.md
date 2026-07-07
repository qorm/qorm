# ADR 0001: Use JSON as Source Format

## Status

Accepted

## Context

QORM 需要一种源格式，用于 scene、component、style、motion、action、resource、platform、agent、patch、bundle。

候选：JSON、YAML、TOML、自定义 DSL。

## Decision

QORM V1 统一使用 JSON。

根文件为 `qorm.json`，其它文件统一使用 `.json` 后缀，通过 `type` 字段区分语义。

## Consequences

优点：

- Agent 生成和 Patch 稳定。
- JSON Schema 直接校验。
- 跨语言 SDK 容易支持。
- MCP / LSP / Bundle / Patch 统一。
- 移动端内置解释器更简单。

代价：

- 人类手写不如 DSL 简洁。
- 注释和复杂表达能力有限。

## Follow-up

未来可选增加 `.qorm` DSL，但必须编译到同一 JSON IR。
