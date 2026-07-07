# QORM 包发布规划

## 发布对象

QORM 的发布体系包含：

```text
Go module (github.com/qorm/qorm)
CLI
VS Code extension
MCP server
Agent packs
Platform packs
SDKs
Schemas
Examples
Docs
```

## Go module

运行时是根目录下的单个 Go module（`github.com/qorm/qorm`），核心包位于 `internal/`，可执行入口位于 `cmd/`：

| 目录 | 角色 |
|---|---|
| `internal/model` | 数据模型 |
| `internal/loader` | 解析目录/bundle → model.App |
| `internal/runtime` | 运行时（state / action / 表达式 / i18n） |
| `internal/render` | 渲染为 HTML/CSS |
| `internal/server` | 服务端渲染 + SSE + qormToNative 派发 |
| `internal/mcp` | MCP server |
| `cmd/qorm` | CLI（run / build / package / inspect） |
| `cmd/qorm-wasm` | 离线 Go→WASM 客户端 |

CLI 命令为：

```bash
qorm check qorm.json
qorm build qorm.json
qorm run qorm.json
qorm inspect qorm.json
qorm fmt qorm.json
```

## Platform Packs

发布目录：

```text
platform-packs/web
platform-packs/desktop
platform-packs/mobile
platform-packs/miniapp
```

每个包包含：

```text
capabilities.json
renderer.json
host-adapter.json
build-target.json
skill.md
mcp-profile.json
examples/
```

## Render Profiles

发布目录：

```text
render-profiles/game-ui
render-profiles/external-game
```

每个 Profile 应包含：

```text
profile.json
examples/
performance-notes.md
integration-notes.md
```

## Agent Packs

```text
agent-packs/codex
agent-packs/claude
agent-packs/openclaw
agent-packs/generic
```

每个包包含：

```text
skill.md
mcp.json
permissions.json
workflows/
```

## SDK

优先发布顺序：

```text
Go SDK
TypeScript SDK
Swift SDK
Kotlin SDK
Python SDK
WASM（Go→WASM）/ C ABI SDK
```

SDK 不应重复实现 Core 逻辑。除 Go SDK 外，其它 SDK 主要提供：

```text
调用封装
数据类型
Host Adapter
MCP Client
Bundle Loader
Platform Bridge
```

## 社区生态与资产包 (Ecosystem Packs)

除了官方支持的 SDK 和基础 Pack 外，QORM 规划了第三方资产的打包与分发机制：

```text
ecosystem/components  # 第三方 UI 组件库
ecosystem/styles      # 第三方主题与样式库
ecosystem/plugins     # 社区扩展的 Host 插件
```

- **分发规范**：初期通过 Git Repository 或现有的 NPM/Cargo registry 分发，未来规划专属的 QORM Registry。
- **依赖引用**：开发者可在 `qorm.json` 的 `dependencies` 字段中声明外部资产包，Resolver 会在编译期自动解析并抓取。

## Schema

所有 JSON 格式都应发布 schema：

```text
schemas/qorm.schema.json
schemas/scene.schema.json
schemas/component.schema.json
schemas/style.schema.json
schemas/action.schema.json
schemas/motion.schema.json
schemas/platform.schema.json
schemas/agent.schema.json
schemas/bundle.schema.json
```

## 版本策略

使用语义化版本：

```text
MAJOR: 格式或 IR 不兼容变更
MINOR: 新能力或兼容扩展
PATCH: bugfix 和文档修复
```

Bundle 应包含：

```json
{
  "qorm": "0.1",
  "bundleVersion": "1.0.0",
  "minRuntimeVersion": "0.1.0",
  "keyId": "release-2026-q2"
}
```

## 发布流程

```text
1. 更新版本
2. 运行测试
3. 生成 schema
4. 构建 CLI
5. 构建 docs
6. 构建 examples
7. 生成 trust metadata / revocation metadata
8. 发布 crates
9. 发布 extension
10. 发布 agent packs
11. 发布 platform packs
12. 生成 release notes
```

## 发布校验

发布前应确认：
- Bundle 签名与 `keyId` 可被目标 Runtime 验证。
- 平台包与 Agent 包未放宽主权限模型。
- docs / examples / schemas 保持同一版本口径。
- Registry 与 Portal 使用一致的包元数据、信任状态和兼容性信息。