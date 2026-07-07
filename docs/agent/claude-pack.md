# QORM Claude Pack

Claude Pack 用于 Claude 类 Agent 的 QORM 工作流。

## 适用任务

```text
文档编写
架构分析
JSON 生成
Layout 检查
平台兼容性分析
Agent Patch 生成
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

Claude Pack 默认不应允许：

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

需要用户确认后才能执行有副作用操作。

## 权限边界

- Claude Pack 不能提升主权限模型授予范围。
- 即便 Pack 层允许某操作，仍需通过 platform / app / host policy。
- `preview_patch` 与 `apply_patch` 的审批关系以 Agent Protocol 和 Permission Model 为准。

## Prompt 规则

Claude 应遵守：

- 先分析现有结构，再生成 Patch。
- 不直接重写整个 Bundle。
- 不添加平台不支持能力。
- 不把 QORM 当成完整游戏引擎。
- 移动端能力必须经过 platform_check。