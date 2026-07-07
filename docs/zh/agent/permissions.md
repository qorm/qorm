<!-- data-lang-nav --> [English](../../agent/permissions.md) · 中文

# QORM Agent 权限

QORM agent 权限模型控制一个 agent 能看到什么、能更改什么以及能执行什么。

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

默认的 agent 权限是:

```text
read-only + preview-only
```

允许:

```text
inspect
validate
preview_patch
explain
platform_check
```

拒绝:

```text
apply_patch
host.call
filesystem.saveFile
shell
deploy
```

这里 `filesystem.saveFile` 表示文件写入能力;其权限键通常是 `filesystem.write`。

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

## Agent 权限与系统权限之间的关系

- Agent 策略是一个附加的约束层,而非授权来源。
- Agent 的允许不能放松平台 / 应用 / 宿主策略。
- Agent 的拒绝可以进一步限制该 agent。
- `preview_patch` 默认可用,但仍必须遵循无副作用原则。

## 危险操作

危险操作包括:

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

这里文件写入的危险性对应于 `filesystem.write` 权限域。

这些操作必须要求用户确认。

## 批准语义

对于一个 agent,以下操作通常需要批准:

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

最低规则:
- 批准范围必须至少绑定 agent、操作和目标资源范围。
- 一个通过的 `preview_patch` 结果不能被自动视为 `apply_patch` 批准。
- 如果策略允许,单次批准可以覆盖"同一 previewToken 的 apply"。
- 在目标文件、补丁内容、Bundle 版本或权限策略发生变化后,原有批准必须失效。

## 批准生命周期与吊销

一条批准记录至少包含:

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

吊销触发条件至少包括:
- 用户手动吊销。
- Agent Pack 发生变化。
- Bundle 或目标文档发生变化。
- 系统策略发生变化。
- 超时过期。

## 审计日志

所有 agent 操作都应被记录:

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

审计日志应尽可能避免存储完整的敏感输入;如果涉及令牌、密码或绝对文件内容,应记录摘要或脱敏字段。
