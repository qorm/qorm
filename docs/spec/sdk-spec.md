# QORM SDK Specification

QORM 支持多语言 SDK，但核心逻辑优先保留在纯 Go Core 中（`github.com/qorm/qorm`）。

SDK 与以下专门规格存在直接耦合：
- Dependency Resolution：`dependency-resolution-spec.md`
- Asset Package：`asset-package-spec.md`
- Test Runner：`test-runner-spec.md`
- Query Selector：`query-selector-spec.md`
- DevServer / HMR：`devserver-hmr-spec.md`
- Miniapp Vendor Profiles：`miniapp-vendor-profiles-spec.md`

## SDK 目标

- 加载 Bundle。
- 调用 Runtime。
- 对接 Host Capability。
- 对接 Agent / MCP。
- 对接平台 Bridge。
- 提供类型定义和工具函数。

## 优先级

```text
1. Go SDK
2. TypeScript SDK
3. Swift SDK
4. Kotlin SDK
5. Python SDK
6. WASM（Go→WASM）/ C ABI SDK
```

## Go SDK

Go SDK 是一等公民，直接使用核心包（无需重实现 Core）。

```go
import (
    "github.com/qorm/qorm/internal/loader"
    "github.com/qorm/qorm/internal/runtime"
)
```

## TypeScript SDK

用于 Web、MCP client、工具链和 VS Code 扩展。

功能：

```text
JSON 类型定义
schema validation helper
MCP client helper
Web Host Adapter
HttpClient abstraction
query/assert helpers
devserver client hooks
```

## Swift / Kotlin SDK

移动端绑定层，主要提供 thin bridge：

```text
load bundle
call Go Runtime（离线为 Go→WASM）
register host capability
lifecycle forwarding
keyboard / IME / gesture forwarding
```

## Python SDK

主要用于工具链、服务端、自动化和 Agent 集成（Go 侧核心逻辑见上文 Go SDK）。

这些 SDK 应优先支持：
- bundle / schema 校验
- dependency resolution client
- registry metadata client
- test runner integration

## C ABI / WASM Component

用于插件和跨语言桥接。WASM Component 应使用明确的接口描述 Host 与 Plugin 的 import/export。

## 不重复实现 Core

除 Go SDK 外，其它 SDK 不应重写 QORM Core 逻辑。应尽量通过：

```text
Go 包 / cgo FFI
WASM（Go→WASM）
MCP
JSON schema
Bundle API
```

复用核心能力。
