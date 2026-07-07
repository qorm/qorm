<!-- data-lang-nav --><p align="right"><a href="../README.md">English</a> · <b>中文</b></p>

# QORM 文档

QORM(Queryable Object Rendering Model,可查询对象渲染模型)是一个纯 Go、面向智能体的
声明式 UI 运行时:用 JSON 编写 UI,实时运行它,签名它,并将其打包为
web / iOS / Android / 桌面——人和 AI 智能体都能读写。

初来乍到?先读[顶层 README](../../README.zh.md) 了解全局与 CLI,然后从下方深入。
[`examples/`](../../examples) 应用是权威的、可运行的参考——当文档与运行中的示例不一致时,
以示例为准。

## 学习

- [快速上手](tutorials/getting-started.md)——安装、你的第一个应用、运行循环
- [第一个场景](tutorials/first-scene.md) · [第一个 action](tutorials/first-action.md) · [第一个组件](tutorials/first-component.md) · [第一个平台包](tutorials/first-platform-pack.md)

## 参考(由代码自动生成——权威)

- [组件目录](../reference/widgets.md)——渲染器接受的每一种节点 `type`
- [能力清单](../platforms/capabilities.md)——内置的硬件/OS 操作、回调与平台

## 平台与打包

- [平台支持矩阵](../platforms/support-matrix.md)——一眼看清各平台的支持情况
- [移动端](platforms/mobile.md) · [桌面端](platforms/desktop.md) · [Web](platforms/web.md) · [小程序](platforms/miniapp.md)
- [用户中间层](platforms/native-middlelayer.md)——在一个 Go 文件中添加你自己的原生操作,它会同时编译进桌面*和*移动/web WASM

## 示例(逐步讲解)

- [Counter](examples/counter.md) · [Todo](examples/todo.md) · [Login](examples/login.md) · [Dashboard](examples/dashboard.md)
- 全部可运行应用位于 [`examples/`](../../examples)。

## 人机协作

- [与你的 AI 助手一起构建](build-with-ai.md)——让你的 AI 指向 QORM 来搭建、编辑、运行并验证应用
- [在运行中的应用上协作](collaboration.md)——一个人和一个 AI 智能体在同一个运行中的应用上,彼此都看得见对方(QORM 的前提)

## 面向 AI 智能体

- [智能体集成](../../integrations)——即插即用的 MCP 配置 + 面向 Claude / Cursor / Windsurf 的 QORM 技能
- [MCP 工具](../agent/mcp-tools.md)——用于读取、编辑并验证运行中应用的模型上下文协议表面
- [验证一个应用](verification.md)——用 `qorm measure` / `qorm check` 自我验证编辑
- [技能](agent/skills.md) · [权限](agent/permissions.md)

## 信任与安全

- [Bundle 签名](security/bundle-signing.md)——ed25519 验证 bundle 的下发方式
- [权限模型](security/permission-model.md) · [安全模型](security/security-model.md)

## 商业使用

- [条款](../../ops/TERMS.md)——源码为 MIT;一个 Patreon 会员涵盖商业白标
