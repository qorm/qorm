# QORM 项目愿景

QORM 是 **Queryable Object Rendering Model**，中文为 **可查询对象渲染模型**。

它的愿景是让 UI 从“前端代码集合”变成一种 Agent 可以理解、查询、修改、模拟、验证和解释的对象化运行时模型。

## QORM 要解决的问题

传统 UI 技术将结构、样式、状态、事件、动效、底层能力和平台差异分散在不同系统中。对于 Agent 来说，这会导致：

- 难以稳定理解界面结构。
- 难以安全修改局部 UI。
- 难以追踪事件和状态变化。
- 难以判断底层能力是否可用。
- 难以跨平台保持一致行为。
- 难以对动态更新做权限控制和回滚。

QORM 的目标是把这些能力统一到可检查、可验证、可 Patch 的 UI Runtime 中。

## QORM 不是什么

QORM 不是：

- 简化版 HTML/CSS/JavaScript。
- 低代码拖拽平台。
- 完整游戏引擎。
- 完整操作系统能力封装。
- 数据库 ORM。
- 任意脚本执行平台。

QORM 是 UI 层，它通过 Host Capability 和 Platform Pack 调用外部能力，而不是自己实现所有底层能力。

## 目标用户

- 需要 Agent 参与 UI 构建的开发者。
- 需要跨平台动态界面的产品团队。
- 需要移动端动态 UI 更新能力的应用。
- 需要实时 UI 的项目。
- 需要可解释、可验证、可 Patch 界面系统的工具平台。

## 核心成果

QORM 最终应提供：

- JSON 源格式与 Bundle 格式。
- 纯 Go Core：服务端渲染 + 离线 Go→WASM 客户端（`cmd/qorm-wasm`）。
- Layout、Render、Runtime、Host Capability。
- 桌面、移动、Web、Miniapp 等 Platform Pack。
- document / app / realtime 等 Render Profile。
- MCP、Skill、Agent Pack。
- VS Code 扩展与 LSP。
- 多语言 SDK。
- 完整文档、测试、性能预算和安全模型。
