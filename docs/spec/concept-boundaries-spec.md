# QORM Concept Boundaries Specification

本规范用于定义 QORM 中四类容易混淆的概念：`Platform`、`Render Profile`、`Integration Mode`、`Host Capability`。

## 目标

- 避免把运行宿主、渲染策略、外部系统集成方式和能力调用混为一谈。
- 为架构、实现、文档、SDK、Agent 工具和发布物提供统一边界。
- 降低后续扩展时的概念漂移和目录污染。

## Platform

Platform 解决：**QORM 运行在哪里**。

Platform 负责：
- 宿主环境接入
- Host Adapter
- 事件桥接
- Native Bridge
- 平台能力清单
- 平台权限边界
- 平台生命周期

典型 Platform：

```text
desktop
mobile
web
miniapp
```

规则：
- Platform 通过 Platform Pack 描述。
- Platform 决定哪些 Host Capability 可用。
- Platform 不定义 UI 的渲染风格，也不决定业务组件偏好。

## Render Profile

Render Profile 解决：**QORM 怎么渲染、怎么更新**。

Render Profile 负责：
- 渲染策略
- 更新节奏
- 性能偏好
- 文本模式偏好
- 默认组件偏好
- Display List / Render Graph 侧的约束

典型 Render Profile：

```text
document
app
realtime
game-ui
game-lite
```

规则：
- Render Profile 不是 Platform。
- 同一个 Render Profile 可以运行在多个 Platform 之上。
- Render Profile 不直接授予新权限。

## Integration Mode

Integration Mode 解决：**QORM 如何与外部系统或外部渲染主体协作**。

Integration Mode 负责：
- 外部系统承载关系
- 渲染职责边界
- 事件路由边界
- 合成与嵌入策略

典型 Integration Mode：

```text
external-game
```

规则：
- Integration Mode 不是 Platform。
- Integration Mode 也不是单纯的 Render Profile；它强调与外部系统的协作边界。
- Integration Mode 仍然运行在某个 Platform 上。
- Integration Mode 若依赖外部 surface 或外部引擎能力，必须通过 Host Capability 和 Platform Pack 提供。

## Host Capability

Host Capability 解决：**QORM 可以调用什么宿主能力**。

Host Capability 负责：
- 能力命名与契约
- 输入输出结构
- 权限校验
- 审批要求
- 平台支持矩阵

典型 Host Capability：

```text
network.request
clipboard.write
filesystem.saveFile
game.surface
```

规则：
- Host Capability 不是 Platform。
- Host Capability 不是 Render Profile。
- Host Capability 由 Platform 提供支持，由权限模型裁决是否允许调用。

## 四者关系

```text
Platform         运行在哪里
Render Profile   怎么渲染、怎么更新
Integration Mode 怎么和外部系统协作
Host Capability  能调用什么宿主能力
```

### 例子 1：普通 Web 表单

```text
Platform: web
Render Profile: document / app
Integration Mode: none
Host Capability: network.request, clipboard.write
```

### 例子 2：桌面 HUD 编辑器预览

```text
Platform: desktop
Render Profile: game-ui
Integration Mode: none
Host Capability: game.surface, filesystem.saveFile
```

### 例子 3：外部游戏引擎中的覆盖层 UI

```text
Platform: desktop 或 mobile
Render Profile: game-ui
Integration Mode: external-game
Host Capability: game.surface
```

## 文档与目录约束

- Platform 文档放在 `docs/platforms/`。
- Render Profile 文档可以放在 `docs/render-profiles/`；在当前仓库迁移前，允许临时保存在 `docs/platforms/`，但必须明确声明“不是 Platform”。
- Integration Mode 文档可以独立成文，或作为 Render / Architecture 规格中的子章节。
- Host Capability 文档放在 `docs/spec/host-capability-spec.md`。

## 迁移原则

若某概念同时具备多种角色，按以下顺序拆分：

1. 先确定它是否决定宿主环境；若是，则属于 Platform。
2. 再判断它是否主要改变渲染与更新策略；若是，则属于 Render Profile。
3. 再判断它是否主要描述外部系统协作；若是，则属于 Integration Mode。
4. 最后将具体可调用能力下沉为 Host Capability。

`game-ui` 应归入 Render Profile。
`external-game` 应归入 Integration Mode。
`game.surface` 应归入 Host Capability。