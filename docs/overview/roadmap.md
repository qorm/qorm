# QORM Roadmap

## Phase 0: 格式与仓库初始化

目标：形成可开发的 GitHub 仓库。

交付：

- `qorm.json` 规范草案。
- scene/component/style/action/motion/resource/platform JSON 样例。
- Go module 初始化（`github.com/qorm/qorm`，`cmd/` + `internal/`）。
- JSON Schema 初版。
- 文档目录与 ADR。

## Phase 1: Core IR 与 Parser

目标：把 JSON 源文件解析为 Typed IR。

交付：

- `core` crate。
- `parser` crate。
- `resolver` crate。
- `validator` crate。
- 表达式和路径校验。
- IR snapshot test。
- Counter 示例。

## Phase 2: Runtime MVP

目标：能够运行状态、事件和基础 Action。

交付：

- State Store。
- Event Dispatch。
- Action Interpreter。
- Motion 基础模型。
- Patch Preview / Apply / Rollback。
- Todo 示例。

## Phase 3: Layout + Render MVP

目标：能够渲染基础 UI。

交付：

- row / column / stack / absolute / scroll。
- Display List。
- Dirty Tree。
- Desktop Preview Renderer。
- Text measurement 基础能力。
- Login 示例。

## Phase 4: Host Capability

目标：QORM 能调用平台底层能力，但保持 UI 层边界。

交付：

- Host Capability Spec。
- desktop host adapter。
- web host adapter。
- `network.request` / storage / clipboard / navigation。
- Capability 权限模型与 approval 生命周期。

## Phase 5: Agent Integration

目标：让 Codex、Claude、OpenClaw、Cursor 等 Agent 能安全操作 QORM。

交付：

- MCP Server。
- Skills。
- Agent Packs。
- inspect / validate / preview_patch / apply_patch / simulate / explain。
- VS Code 扩展初版。

## Phase 6: Mobile Pack

目标：支持 iOS / Android 内置动态解释器。

交付：

- Mobile Platform Pack。
- Swift thin bridge。
- Kotlin thin bridge。
- Safe Area / Keyboard / IME / Gesture / Lifecycle。
- Bundle 签名、回滚、灰度。

## Phase 7: Render Optimization

目标：强化前台渲染效率。

交付：

- Render Graph。
- Display List diff。
- Texture Atlas。
- Text cache。
- Batching。
- Performance budget。

## Phase 8: SDK / Plugin Ecosystem

目标：形成开发者生态。

交付：

- Go SDK（一等公民，直接复用核心包）。
- TypeScript SDK。
- Swift / Kotlin SDK。
- Python SDK。
- WASM（Go→WASM）Plugin。
- C ABI Plugin。

## Phase 9: DX & Testing

目标：形成稳定的开发预览、热更新和无头测试能力。

交付：

- DevServer。
- HMR / Fast Refresh。
- Test Runner API。
- 节点查询与状态断言 API。

## Phase 10: Advanced Runtime & Ecosystem Specs

目标：补齐高级运行时能力与生态资产的正式规范。

交付：

- Global Store / Context 规范。
- Error Boundary 规范。
- Asset Package / Dependency Resolution 规范。
- Miniapp vendor capability profiles。

## Phase 11: Website & Ecosystem Services

目标：建设官方站点、文档门户、playground 和 Registry 基础设施。

交付：

- 官方站点与文档门户。
- Playground MVP。
- Ecosystem Portal。
- Registry API 与发布流。