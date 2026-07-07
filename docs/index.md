# QORM 文档索引

本目录包含 QORM 的规划、规格、平台、Agent、开发、安全、教程、示例和 ADR 文档。

## 推荐先读

- [JSON 格式规格](spec/json-format-spec.md)
- [IR 规格](spec/ir-spec.md)
- [Runtime 规格](spec/runtime-spec.md)
- [Action 规格](spec/action-spec.md)
- [Motion 规格](spec/motion-spec.md)
- [Host Capability 规格](spec/host-capability-spec.md)
- [Agent Protocol 规格](spec/agent-protocol-spec.md)
- [Permission Model](security/permission-model.md)
- [Bundle Signing](security/bundle-signing.md)

## 规划文档

- [标准化格式方案](planning/standard-format-plan.md)
- [技术架构规划](planning/technical-architecture-plan.md)
- [研发实施规划](planning/implementation-plan.md)
- [包发布规划](planning/package-release-plan.md)
- [官方站点与开发者门户规划](planning/official-website-plan.md)
- [生态 Registry 与分发服务规划](planning/ecosystem-registry-plan.md)
- [正式规格补完计划](planning/formal-spec-expansion-plan.md)
- [V1 实施切面规划](planning/v1-implementation-cut-plan.md)
- [V1 Crate / Service 实施拆解](planning/v1-crate-implementation-plan.md)
- [QORM 主控研发进度规划](planning/qorm-development-progress-plan.md)
- [QORM V1 RC 阶段总结](planning/v1-rc-stage-summary.md)
- [QORM V1 RC Release Gate Evidence](planning/v1-rc-release-gate-evidence.md)
- [QORM V1 Post-RC Hardening Evidence](planning/v1-post-rc-hardening-evidence.md)

## 核心规格

- [JSON 格式规格](spec/json-format-spec.md)
- [IR 规格](spec/ir-spec.md)
- [Runtime 规格](spec/runtime-spec.md)
- [Layout 规格](spec/layout-spec.md)
- [Rendering 规格](spec/rendering-spec.md)
- [Action 规格](spec/action-spec.md)
- [Motion 规格](spec/motion-spec.md)
- [Host Capability 规格](spec/host-capability-spec.md)
- [Agent Protocol 规格](spec/agent-protocol-spec.md)
- [Platform Pack 规格](spec/platform-pack-spec.md)
- [SDK 规格](spec/sdk-spec.md)
- [Concept Boundaries](spec/concept-boundaries-spec.md)
- [Global State 规格](spec/global-state-spec.md)
- [Error Boundary 规格](spec/error-boundary-spec.md)
- [Dependency Resolution 规格](spec/dependency-resolution-spec.md)
- [Test Runner 规格](spec/test-runner-spec.md)
- [Query Selector 规格](spec/query-selector-spec.md)
- [Asset Package 规格](spec/asset-package-spec.md)
- [DevServer / HMR 规格](spec/devserver-hmr-spec.md)
- [Miniapp Vendor Profiles 规格](spec/miniapp-vendor-profiles-spec.md)

## 平台

- [Desktop](platforms/desktop.md)
- [Mobile](platforms/mobile.md)
- [能力清单(自动生成)](platforms/capabilities.md)
- [组件目录(自动生成)](reference/widgets.md)
- [用户中间层:加自己的原生能力](platforms/native-middlelayer.md)
- [Web](platforms/web.md)
- [Miniapp](platforms/miniapp.md)

## Render Profiles

- [Game UI](platforms/game-ui.md)

## Agent

- [MCP Tools](agent/mcp-tools.md)
- [Skills](agent/skills.md)
- [Codex Pack](agent/codex-pack.md)
- [Claude Pack](agent/claude-pack.md)
- [OpenClaw Pack](agent/openclaw-pack.md)
- [Agent 权限](agent/permissions.md)

## 工程

- [Workspace Guide](development/workspace-guide.md)
- [Coding Guidelines](development/coding-guidelines.md)
- [Testing Strategy](development/testing-strategy.md)
- [Performance Budget](development/performance-budget.md)
- [Release Process](development/release-process.md)

## 安全

- [Security Model](security/security-model.md)
- [Bundle Signing](security/bundle-signing.md)
- [Permission Model](security/permission-model.md)

## 教程与示例

- [Getting Started](tutorials/getting-started.md)
- [First Scene](tutorials/first-scene.md)
- [First Component](tutorials/first-component.md)
- [First Action](tutorials/first-action.md)
- [First Platform Pack](tutorials/first-platform-pack.md)
- [Counter](examples/counter.md)
- [Todo](examples/todo.md)
- [Login](examples/login.md)
- [Dashboard](examples/dashboard.md)
- [Game HUD](examples/game-hud.md)

## ADR

- [0001 Use JSON as Source Format](adr/0001-use-json-as-source-format.md)
- 0002 ~~Use Rust for Core~~ — 已作废：运行时为纯 Go（服务端渲染 + 离线 Go→WASM），见 [Go 架构](overview/go-architecture.md)。
- [0003 No Native JIT](adr/0003-no-native-jit.md)
- [0004 GPU-first Rendering](adr/0004-gpu-first-rendering.md)
- [0005 Host Capability Layer](adr/0005-host-capability-layer.md)
- [0006 VS Code Extension Instead of Custom Editor](adr/0006-vscode-extension-instead-of-custom-editor.md)
