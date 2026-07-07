<!-- data-lang-nav --> [English](../../agent/claude-pack.md) · 中文

# QORM Claude Pack

Claude Pack 为 Claude 风格的 agent 提供 QORM 工作流。

## 适用任务

```text
Documentation writing
Architecture analysis
JSON generation
Layout inspection
Platform compatibility analysis
Agent Patch generation
```

## 推荐工具

```text
qorm.inspect_scene
qorm.validate_bundle
qorm.preview_patch
qorm.explain_node
qorm.platform_check
```

## 安全要求

默认情况下,Claude Pack 不应允许:

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

带有副作用的操作在运行前必须经过用户确认。

## 权限边界

- Claude Pack 不能扩展主权限模型所授予的范围。
- 即使 Pack 层允许某项操作,它仍必须通过平台 / 应用 / 宿主策略。
- `preview_patch` 与 `apply_patch` 之间的批准关系由 Agent Protocol 和 Permission Model 管辖。

## 提示词规则

Claude 应遵循以下规则:

- 先分析现有结构,然后再生成 Patch。
- 不要直接重写整个 Bundle。
- 不要添加平台不支持的能力。
- 不要把 QORM 当作一个完整的游戏引擎。
- 移动端能力必须通过 platform_check。
