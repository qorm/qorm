# QORM 架构总览

> 权威、贴合实现的架构见 [Go 架构](go-architecture.md)。运行时是**纯 Go**：默认服务端渲染为 HTML/CSS 交给浏览器/WebView，离线场景通过 `cmd/qorm-wasm` 编译为 WASM 在客户端运行。本文描述概念分层，不再指向已移除的 Rust 参考实现。

QORM 的架构分为五层：

```text
Source JSON / Bundle
        ↓
Parser / Resolver / Validator
        ↓
Typed IR / Execution Plan
        ↓
Runtime / Layout / Render / Host Capability
        ↓
Platform Pack / Agent Pack / SDK
```

## 核心模块

```text
Core              Scene IR、Node、State、ID、Value、Diagnostic
Parser            JSON 文件读取和类型识别
Resolver          import/include/resource 引用解析
Validator         schema 与语义校验
Bundle            Bundle 编译、签名、版本、缓存
Runtime           Event、Action、State、Motion、Patch
Layout            LayoutSpec、measure、dirty layout
Render            Display List、Render Graph、Renderer Contract
Style             Token、Variant、State Style、Resolved Style
Text              文本测量、字体 fallback、IME 相关抽象
Host              Host Capability、权限、平台调用
Plugin            Go 接口、WASM、用户中间层（pkg/qormext）插件抽象
Bridge            Swift/Kotlin 等 thin bridge（Go 侧统一派发 qormToNative）
Agent             Inspect、Patch、Simulate、Explain
MCP               MCP Server 工具暴露
LSP               编辑器语言服务
Build             Build Options / Tooling API 封装
```

## 数据流

```text
qorm.json
  ↓
加载 scene/component/style/action/motion/resource/platform
  ↓
Resolver 解析文件路径和逻辑引用
  ↓
Validator 校验字段、表达式、语义、能力和平台
  ↓
生成 Typed IR
  ↓
生成 Execution Plan
  ↓
Runtime 执行事件、状态、Action、Motion
  ↓
Layout 计算布局
  ↓
Render 生成 Display List 和 Render Graph
  ↓
Platform Renderer 输出到桌面、移动、Web 等宿主；如启用 `game-ui` profile，则按 HUD / Overlay 渲染策略输出。 
```

## 运行模式

### 开发模式

```text
源 JSON → 校验 → 逻辑模型 → Agent Preview Patch → 重新校验 → Apply Patch → 运行
```

### 生产模式

```text
源 JSON → qorm build → qorm.bundle.json → canonicalize + 签名 → App 内置解释器加载
```

### 移动动态更新模式

```text
远程 Bundle → 校验 version/hash/signature/keyId → 预解析 → 切换 → 失败回滚
```

## 平台边界

QORM Core 保持平台无关。平台相关能力由 Platform Pack 提供：

```text
renderer
host adapter
event adapter
native bridge
capability manifest
build target
agent skill
mcp profile
```

- Bundle 只能声明需要的 capability，不能授予自己权限。
- Host Adapter 是 runtime 与平台 API 之间的执行边界。
- Web 端 custom client 只能做 transport adapter，不能替代权限层。

## Render Profile / Runtime Mode

概念边界以 [Concept Boundaries](../spec/concept-boundaries-spec.md) 为准。

除平台之外，QORM 还允许通过 Render Profile / Runtime Mode 切换渲染与更新策略，例如：

```text
document
app
realtime
game-ui
external-game
```

external-game 也属于 profile / integration mode，而不是平台类型。

## Agent 边界

Agent 通过 MCP / Skill / VS Code 扩展 / LSP 与 QORM 交互。

```text
inspect / validate 默认允许
preview_patch 必须无副作用
apply_patch 必须绑定 preview 结果
危险操作必须要求确认
```

Patch 的规范路径作用于逻辑模型，Source Map 的 JSON Pointer 只用于回源定位与诊断。