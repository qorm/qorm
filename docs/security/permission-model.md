# QORM Permission Model

权限模型控制 Host Capability、Agent、Plugin、Bundle 和 Platform Pack 的可用范围。

## 权限来源

```text
platform capabilities
bundle requirements
app policy
agent policy
plugin manifest
user approval
```

## 权限优先级

一次调用按以下顺序裁决：

```text
1. capability exists
2. platform supports capability
3. bundle declares capability
4. app/system policy allows capability
5. agent/plugin policy allows capability
6. user approved if required
```

规则：
- 默认 deny。
- 任一环节拒绝即拒绝。
- 用户 approval 不能越过 platform unsupported 或 app/system hard deny。
- `bundle declares capability` 只表示声明需求，不表示授予权限。

## 冲突解决

- deny 覆盖 allow。
- 缺失声明视为 deny。
- 结构化限制只能收窄，不能放宽。
- 例如多个来源都声明 `domains` 时，最终允许范围取交集。
- 若某来源未声明 `methods` 而另一个来源声明了 `methods`，最终只允许已声明的方法集合。

## 权限示例

```json
{
  "permissions": {
    "network.request": {
      "allowed": true,
      "domains": ["api.example.com"],
      "methods": ["GET", "POST"]
    },
    "filesystem.write": {
      "allowed": true,
      "requiresApproval": true
    },
    "shell": {
      "allowed": false
    }
  }
}
```

## requiresApproval 语义

`requiresApproval` 表示该能力或操作即使已被允许，也仍需有效审批凭证。

最小规则：
- scope 至少绑定 `capability`、调用方、目标资源范围。
- 审批可以是一次性或会话级，但必须由策略显式定义。
- 目标资源变化后，原审批不得自动扩展到更大范围。
- 策略、Bundle、Platform Pack、Agent Pack、用户身份或运行环境变化后，审批必须重新评估。

## Approval 生命周期

审批记录至少包含：

```text
approval id
issuer
scope
issuedAt
expiresAt
reuse policy
revokedAt
revocation reason
```

失效条件至少包括：
- 超时过期。
- 用户主动撤销。
- Bundle 或 Pack 版本切换。
- 权限策略变化。
- 目标文件、目标域名或目标 capability 范围变化。

## Agent 权限

Agent 侧权限应更严格：

```text
read-only
preview-only
edit-json
apply-patch
host-call
admin
```

默认不授予 `apply-patch` 和 `host-call`。

Agent allow 不能放宽 app/system policy；Agent deny 可以进一步限制该 Agent。

## 审计

所有权限相关操作必须记录审计日志。

最小字段：

```text
decision id
subject type
subject id
capability or operation
input summary
policy result
approval id
reason
timestamp
```

审计日志应避免记录原始敏感载荷；若必须保留上下文，应记录摘要或脱敏字段。