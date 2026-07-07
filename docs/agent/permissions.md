# QORM Agent Permissions

QORM Agent 权限模型用于控制 Agent 可以看什么、改什么、执行什么。

## 权限级别

```text
read-only
preview-only
edit-json
apply-patch
host-call
build
run
deploy
admin
```

## 默认权限

默认 Agent 权限为：

```text
read-only + preview-only
```

允许：

```text
inspect
validate
preview_patch
explain
platform_check
```

禁止：

```text
apply_patch
host.call
filesystem.saveFile
shell
deploy
```

其中 `filesystem.saveFile` 表示文件写入类 capability；其权限键通常为 `filesystem.write`。

## 权限声明

```json
{
  "agent": "codex",
  "permissions": {
    "inspect": true,
    "validate": true,
    "previewPatch": true,
    "applyPatch": "requiresApproval",
    "hostCall": false,
    "shell": false
  }
}
```

## Agent 权限与系统权限的关系

- Agent policy 是额外约束层，不是授权源。
- Agent allow 不能放宽 platform / app / host policy。
- Agent deny 可以进一步限制该 Agent。
- `preview_patch` 默认可用，但仍必须遵守无副作用原则。

## 危险操作

危险操作包括：

```text
filesystem.saveFile
network.request to external domain
shell
process.spawn
native.call
plugin.install
deploy
bundle.publish
```

这里的文件写入危险性对应 `filesystem.write` 权限域。

必须要求用户确认。

## Approval 语义

对 Agent 而言，以下操作通常需要审批：

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

最小规则：
- 审批 scope 至少绑定 agent、operation、目标资源范围。
- `preview_patch` 的通过结果不能自动等价于 `apply_patch` 审批。
- 若策略允许，可以让一次审批覆盖“同一 previewToken 的 apply”。
- 目标文件、patch 内容、Bundle 版本、权限策略变化后，原审批必须失效。

## Approval 生命周期与撤销

审批记录至少包含：

```text
approval id
agent id
operation
scope
issuedAt
expiresAt
reuse policy
revokedAt
```

撤销触发条件至少包括：
- 用户主动撤销。
- Agent Pack 变化。
- Bundle 或目标文档变化。
- 系统策略变化。
- 超时过期。

## 审计日志

所有 Agent 操作应记录：

```text
agent id
tool name
input summary
output summary
files changed
permissions used
timestamp
approval id
audit event id
```

审计日志应尽量避免存储完整敏感输入；如涉及 token、密码、绝对文件内容，应记录摘要或脱敏字段。