# QORM Agent Protocol Specification

QORM Agent Protocol 定义 Agent 如何检查、修改、模拟和解释 QORM UI。

## 工具列表

```text
qorm.inspect_scene
qorm.inspect_node
qorm.validate_bundle
qorm.preview_patch
qorm.apply_patch
qorm.rollback_patch
qorm.simulate_event
qorm.explain_node
qorm.platform_check
qorm.layout_debug
qorm.render_profile_check
qorm.list_capabilities
```

## inspect_scene

输入：

```json
{
  "scene": "main"
}
```

输出：

```json
{
  "scene": "main",
  "nodes": [
    {
      "id": "submit_button",
      "type": "button",
      "semantic": "primary_action",
      "events": ["press"]
    }
  ]
}
```

## Patch Path 规范

Agent Patch 使用规范逻辑路径：

```text
/scenes/<sceneId>/nodes/<nodeId>/<field>
/scenes/<sceneId>/state/<statePath>
/components/<componentId>/template/<field>
/motions/<motionId>/<field>
```

示例：

```text
/scenes/main/nodes/submit_button/text
/scenes/main/state/count
```

规则：
- Patch Path 作用于逻辑模型，不直接作用于源 JSON children 下标路径。
- Source Map 的 JSON Pointer 只用于诊断和回源。
- node 定位优先使用稳定 `id`，避免位置索引。

## preview_patch

Patch 先预览，不直接应用：

```json
{
  "patches": [
    {
      "op": "replace",
      "path": "/scenes/main/nodes/submit_button/text",
      "value": "立即提交"
    }
  ]
}
```

返回：

```json
{
  "status": "ok",
  "normalizedPatches": [
    {
      "op": "replace",
      "path": "/scenes/main/nodes/submit_button/text",
      "value": "立即提交"
    }
  ],
  "diagnostics": [],
  "diff": [
    {
      "path": "/scenes/main/nodes/submit_button/text",
      "before": "提交",
      "after": "立即提交"
    }
  ],
  "affectedNodes": ["submit_button"],
  "requiresApproval": false
}
```

约束：
- preview 必须无副作用。
- preview 必须经过 path / schema / semantic 校验。
- preview 结果可以返回规范化 patch 集，供 apply 复用。

## apply_patch

仅在 preview 通过后允许 apply。危险操作必须要求确认。

请求：

```json
{
  "patches": [
    {
      "op": "replace",
      "path": "/scenes/main/nodes/submit_button/text",
      "value": "立即提交"
    }
  ],
  "previewToken": "preview_123",
  "approvalId": "approval_456"
}
```

返回：

```json
{
  "status": "applied",
  "appliedPatches": 1,
  "historyId": "patch_history_1",
  "auditEventId": "audit_789"
}
```

规则：
- `previewToken` 绑定一次 preview 的规范化结果。
- 若 patch、目标文档、权限策略或 Bundle 版本变化，`previewToken` 必须失效。
- `approvalId` 仅在需要审批时提供。

## rollback_patch

```json
{
  "historyId": "patch_history_1"
}
```

- rollback 只能回滚已成功 apply 的 patch 历史项。

## simulate_event

```json
{
  "scene": "main",
  "target": "submit_button",
  "event": "press"
}
```

返回：

```json
{
  "stateChanges": [],
  "hostCalls": [],
  "motions": []
}
```

- `simulate_event` 可以报告潜在 `host.call`，但默认不真正执行外部副作用。
- 若工具支持受控执行，必须额外经过权限与审批。

## explain_node

用于回答：为什么这个节点可见、禁用、样式变化或触发某个 Action。

## 诊断与错误

诊断最小字段：

```json
{
  "code": "invalid_patch_path",
  "message": "unknown node id submit_btn",
  "path": "/scenes/main/nodes/submit_btn/text",
  "severity": "error"
}
```

标准错误建议覆盖：

```text
invalid_patch_path
schema_violation
semantic_violation
permission_denied
approval_required
approval_expired
preview_token_stale
platform_unsupported
```

## 权限原则

Agent 默认允许：

```text
inspect
validate
preview_patch
explain
layout_debug
platform_check
```

Agent 默认不允许：

```text
apply_patch
host.call
filesystem.saveFile
network.request
shell
deploy
```

其中 `filesystem.saveFile` 表示文件写入类 host capability；其权限键通常为 `filesystem.write`。

除非用户或策略显式授权。

## Agent Pack

QORM 提供：

```text
Codex Pack
Claude Pack
OpenClaw Pack
Generic Pack
```

每个 Pack 包含 skill、mcp 配置、权限和工作流说明。