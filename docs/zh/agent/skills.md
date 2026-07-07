<!-- data-lang-nav --> [English](../../agent/skills.md) · 中文

# QORM Skills

Skill 是供 agent 使用的工作流描述。QORM 应为不同任务提供可复用的 Skill。

## Skill 类型

```text
scene-authoring
layout-debugging
agent-patch
platform-porting
motion-design
host-capability-check
mobile-adaptation
```

## Skill 基本结构

```text
Goal
Applicable scope
Input files
Recommended tools
Steps
Prohibited actions
Output format
Permission requirements
```

## scene-authoring

用途:让 agent 创建或修改场景 JSON。

规则:

- JSON 必须保持有效。
- 必须使用 `type` 字段来区分文件语义。
- 不得凭空添加 Host Capabilities。
- 修改后,必须执行 validate。

## layout-debugging

用途:分析布局异常。

步骤:

```text
inspect_scene
layout_debug
Inspect LayoutSpec
Inspect text measurement
Inspect safe area / scroll / absolute
preview_patch
```
