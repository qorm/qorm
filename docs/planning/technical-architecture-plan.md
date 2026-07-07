# QORM 技术架构规划

> **更正**：本文早期设想 Rust 参考实现，已整体移除。QORM 运行时现为**纯 Go**：默认服务端渲染为 HTML/CSS，离线通过 `cmd/qorm-wasm` 编译为 WASM 在客户端运行。以 [Go 架构](../overview/go-architecture.md) 为准。

## 技术目标

QORM 要成为一个 Go 原生、Agent 可查询、跨平台可渲染的 UI Runtime。

核心不是语法，而是：

```text
JSON Source → Typed IR → Runtime → Layout → Render → Platform
```

## 技术栈建议

| 层级 | 技术 |
|---|---|
| Core | Go（纯 Go，module `github.com/qorm/qorm`） |
| 源格式 | JSON |
| Schema | JSON Schema |
| Layout | 渲染为 HTML/CSS，交给浏览器/WebView 布局 |
| Render | Go 渲染 HTML/CSS（`internal/render`），服务端或 Go→WASM 客户端 |
| Desktop | Go 二进制（`-tags desktop` 用原生 WebView）+ 平台桥 |
| Mobile | Go→WASM（`cmd/qorm-wasm`）+ Swift/Kotlin thin bridge |
| Web | Go 服务端渲染 / 离线 Go→WASM + Web Host Adapter |
| Agent | MCP + Skill + Agent Pack |
| 编辑器 | VS Code 扩展 + LSP |

概念边界以 `docs/spec/concept-boundaries-spec.md` 为准。

## 模块分层

```text
Core Layer
  Scene IR / Node / State / Value / Diagnostic

Source Layer
  Parser / Resolver / Validator / Bundle

Runtime Layer
  Local State / Global Store / Event / Action / Motion / Patch / Dirty Tracking / Error Boundary

Rendering Layer
  Layout / Text / Display List / Render Graph / Renderer Contract

Host Layer
  Host Capability / Native Bridge / Plugin

Agent Layer
  MCP / Skill / Inspect / Patch / Simulate / Explain

Platform Layer
  Desktop / Mobile / Web / Miniapp

Render/Profile Layer
  document / app / realtime
```

## 动态解释器

桌面端和移动端可内置 Go 运行时（移动端为 Go→WASM）动态加载 bundle：

```text
load qorm.bundle.json
  ↓
verify version/hash/signature/keyId
  ↓
parse bundle
  ↓
build Typed IR
  ↓
build Execution Plan
  ↓
run Runtime
```

## 不考虑 JIT

QORM 不实现 Native JIT。执行层采用：

```text
Typed IR
Execution Plan
Interpreter
Dirty Dependency Graph
```

性能重点放在：

```text
增量更新
布局缓存
Display List 缓存
GPU 批处理
文本缓存
纹理图集
Render Graph
```

## 状态管理与错误隔离

- **全局状态 (Global Store / Context)**：除局部的 Scene 状态外，QORM 支持跨组件、跨场景的状态共享机制。为防止 Agent 产生的意外 Patch 或逻辑污染，全局状态的读写将被严格限制在定义好的作用域内。
- **错误边界 (Error Boundary)**：Runtime 支持组件级错误捕获与降级渲染（Fallback）。局部的组件解析失败、Action 崩溃或资源缺失，不会导致整个应用的 Render Tree 或宿主进程崩溃。

## 平台能力

QORM 不实现所有底层能力。通过 Host Capability 调用：

```text
network.request
storage.read/write
clipboard.read/write
filesystem.open/save
notification.show
navigation.go
camera.capture
audio.play / video.play
window.fullscreen
```

桌面端和移动端优先通过 Go Host Capability（`qormToNative` op）和 thin bridge 调用底层能力。Web 端通过 Web Host Adapter 封装受控 HttpClient adapter。

### 安全 authority stack

```text
Platform support
  ↓
Bundle declaration
  ↓
App / system policy
  ↓
Agent / plugin policy
  ↓
Approval if required
```

- Bundle 只声明需求，不授予权限。
- Host Adapter 是最终 dispatch 前的权限执行点。
- custom HttpClient 只能实现 transport，不能替代裁决层。

## 游戏引擎非目标

QORM 不做：

```text
完整游戏引擎
物理系统
3D 场景
角色控制
骨骼动画
游戏 AI
```