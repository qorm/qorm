# QORM Release Process

## 版本策略

使用语义化版本：

```text
MAJOR.MINOR.PATCH
```

## 发布前检查

```bash
gofmt -l .
go vet ./...
go test ./...
```

还需执行：

```text
schema validation tests
IR snapshot tests
layout golden tests
render golden tests
agent protocol tests
security permission tests
performance smoke tests
cross-spec consistency review
docs example validation
```

## 发布内容

```text
crates
CLI
schemas
docs
examples
VS Code extension
MCP server
Agent packs
Platform packs
SDKs
```

## Release Notes

每次发布应包含：

```text
新增功能
格式变更
IR 变更
Runtime 变更
平台支持
Agent 工具变更
安全修复
破坏性变更
迁移指南
```

## Bundle 兼容性

Runtime 必须检查：

```text
qorm version
bundle version
minRuntimeVersion
capability requirements
platform target
signature/hash
keyId / trust status
```

## 文档一致性门槛

发布前必须确认：
- 所有被引用的 IR / Action / Motion 类型都有正式定义。
- Action 支持集合在 JSON / IR / Runtime 三层一致。
- Patch 示例路径全部使用逻辑路径规范。
- 教程与示例不依赖未定义或未授权能力。

## 回滚

移动端和生产环境必须支持 Bundle 回滚：

```text
current bundle
previous known-good bundle
rollback reason
rollback timestamp
```