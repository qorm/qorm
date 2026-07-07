# QORM Skills

Skill 是给 Agent 使用的工作流说明。QORM 应为不同任务提供可复用 Skill。

## Skill 类型

```text
scene-authoring
layout-debugging
agent-patch
platform-porting
motion-design
host-capability-check
mobile-adaptation
game-ui-design
```

## Skill 基本结构

```text
目标
适用范围
输入文件
推荐工具
步骤
禁止事项
输出格式
权限要求
```

## scene-authoring

用途：让 Agent 创建或修改 scene JSON。

规则：

- 必须保持 JSON 合法。
- 必须使用 `type` 字段区分文件语义。
- 不允许凭空添加 Host Capability。
- 修改后必须 validate。

## layout-debugging

用途：分析布局异常。

步骤：

```text
inspect_scene
layout_debug
检查 LayoutSpec
检查文本测量
检查 safe area / scroll / absolute
preview_patch
```

## game-ui-design

用途：生成游戏 HUD。

规则：

- `game-ui-design` 面向 `render.profile = game-ui` 的 HUD / Overlay 设计，不代表独立平台。
- 优先使用 absolute/overlay。
- 高频动画不得触发布局。
- 使用 fastText 和 progressBar。
- 不实现游戏逻辑。
