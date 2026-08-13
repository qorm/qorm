<!-- data-lang-nav --><p align="right"><a href="../README.md">English</a> · <b>中文</b></p>

# QORM 文档

QORM(Query · Observe · Render · Mutate,查询 · 观察 · 渲染 · 变更)是一个纯 Go、
智能体原生的跨平台应用平台:用 JSON 编写应用,实时运行它,签名它,并将其打包为
web / iOS / Android / 桌面——人和 AI 智能体都能读写。

这个名字同时也是你对一个活动应用所做的四件事:经由 HTTP/MCP **查询(Query)**
节点树与状态,通过 SSE 实时**观察(Observe)**它,把它**渲染(Render)**到每个平台,
再经由 actions 与写接口**变更(Mutate)**它。

> **2-Prompt Agent 快速开始提示词：**
> 1. *“请加载 QORM 框架的 MCP 配置与 Skill 技能库（https://github.com/qorm/qorm），搭好环境，保持 DevTool 可见。”*
> 2. *“使用 QORM 在 `./my-app` 创建应用，启动原生窗口，然后根据以下需求构建应用：<在此输入你的应用想法，例如：带连续打卡天数的习惯追踪器>。”*

初来乍到?先读[顶层 README](https://github.com/qorm/qorm/blob/main/README.zh.md) 了解全局与 CLI,然后从下方深入。
[`examples/`](https://github.com/qorm/qorm/tree/main/examples) 应用是权威的、可运行的参考——当文档与运行中的示例不一致时,
以示例为准。

## 学习

- [工程结构](project-structure.md)——一个 QORM 应用文件夹的布局,逐个文件讲解
- [快速上手](tutorials/getting-started.md)——安装、你的第一个应用、运行循环
- [第一个场景](tutorials/first-scene.md) · [第一个 action](tutorials/first-action.md) · [第一个组件](tutorials/first-component.md) · [第一个平台包](tutorials/first-platform-pack.md)
- [表达式](expressions.md)——`{{ … }}` 语言:作用域、运算符、下标访问,以及全部内置函数
- [QScript](qscript.md)——`.qs` 脚本语言:`let`、`if`、`for`、`while`、`fn`、`call()`,以及共享库 `lib.qs`
- [QSS 与 canvas 样式](styles.md)——样式表与声明式 canvas 效果(滤镜、蒙版、吸附、FLIP、弹簧、`fx` / `timeline`、tint、pixelated、rotate/scale/flip/skew、`board` 镜头 + `tilemap`)

## 参考

完整的、由代码自动生成的契约在独立的 **[API 参考站](/api/zh/)**——节点与组件属性、
组件目录、动作与状态、手势、动画、导航、HTTP/SSE 接口面,以及公开 Go 包。它从运行时
源码抽取,永不漂移。

面向应用的能力文档仍留在这里,与平台指南放在一起:

- [能力清单](../platforms/capabilities.md)——内置的硬件/OS 操作、回调与平台
- [MCP 工具](../agent/mcp-tools.md)——AI 智能体驱动活动应用所用的工具

## 平台与打包

- [平台支持矩阵](../platforms/support-matrix.md)——一眼看清各平台的支持情况
- [移动端](platforms/mobile.md) · [桌面端](platforms/desktop.md) · [Web](platforms/web.md) · [小程序](platforms/miniapp.md)
- [用户中间层](platforms/native-middlelayer.md)——在一个 Go 文件中添加你自己的原生操作,它会同时编译进桌面*和*移动/web WASM

## 示例(逐步讲解)

- [Counter](examples/counter.md) · [Todo](examples/todo.md) · [Login](examples/login.md) · [Dashboard](examples/dashboard.md)
- 游戏(canvas):[`tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris) · [`mario`](https://github.com/qorm/qorm/tree/main/examples/mario) · [`raiden`](https://github.com/qorm/qorm/tree/main/examples/raiden) · [`g2048`](https://github.com/qorm/qorm/tree/main/examples/g2048)——在线 [qorm.com/games](https://qorm.com/games/)
- 动效展示:[`canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx)
- 全部可运行应用位于 [`examples/`](https://github.com/qorm/qorm/tree/main/examples)。

## 人机协作

- [与你的 AI 助手一起构建](build-with-ai.md)——让你的 AI 指向 QORM 来搭建、编辑、运行并验证应用
- [在运行中的应用上协作](collaboration.md)——一个人和一个 AI 智能体在同一个运行中的应用上,彼此都看得见对方(QORM 的前提)

## 面向 AI 智能体

- [智能体集成](https://github.com/qorm/qorm/tree/main/integrations)——即插即用的 MCP 配置 + 面向 Claude / Cursor / Windsurf 的 QORM 技能
- [MCP 工具](../agent/mcp-tools.md)——用于读取、编辑并验证运行中应用的模型上下文协议表面
- [验证一个应用](verification.md)——用 `qorm measure` / `qorm check` 自我验证编辑
- [技能](agent/skills.md) · [权限](agent/permissions.md)

## 信任与安全

- [Bundle 签名](security/bundle-signing.md)——ed25519 验证 bundle 的下发方式
- [权限模型](security/permission-model.md) · [安全模型](security/security-model.md)

## 商业使用

- [条款](https://github.com/qorm/qorm/blob/main/ops/TERMS.md)——源码为 MIT;一个 Patreon 会员涵盖商业白标;创业团队或不便订阅,可邮件 github@qorm.com 免费申请一年授权(可续期)
