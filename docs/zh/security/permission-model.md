<!-- data-lang-nav --> [English](../../security/permission-model.md) · 中文

# QORM 权限模型

权限模型控制 Host Capabilities、Agents、Plugins、Bundles 和 Platform Packs 的可用范围。

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

单次调用按以下顺序裁决:

```text
1. capability exists
2. platform supports capability
3. bundle declares capability
4. app/system policy allows capability
5. agent/plugin policy allows capability
6. user approved if required
```

规则:
- 默认拒绝。
- 任一阶段的拒绝都会导致最终拒绝。
- 用户批准不能覆盖平台不支持或应用/系统的硬性拒绝。
- `bundle declares capability` 仅表达一种需求声明;它并不授予权限。

## 冲突解决

- 拒绝优先于允许。
- 缺失的声明被视为拒绝。
- 结构化限制只能收窄,绝不能放宽。
- 例如,当多个来源都声明了 `domains` 时,最终允许的范围是它们的交集。
- 如果一个来源未声明 `methods` 而另一个来源声明了 `methods`,最终结果只允许已声明的方法集合。

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

`requiresApproval` 意味着,即使某项能力或操作已经被允许,仍然需要一份有效的批准凭证。

最低规则:
- 其范围必须至少绑定 `capability`、调用方和目标资源范围。
- 批准可以是一次性的或会话级的,但必须由策略显式定义。
- 目标资源发生变化后,原有批准不得自动扩展到更大的范围。
- 在策略、Bundle、Platform Pack、Agent Pack、用户身份或运行时环境发生变化后,必须重新评估批准。

## 批准生命周期

一条批准记录至少包含:

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

失效条件至少包括:
- 超时过期。
- 用户显式吊销。
- Bundle 或 Pack 版本切换。
- 权限策略变更。
- 目标文件、目标域或目标能力的范围发生变化。

## Agent 权限

Agent 侧的权限应更为严格:

```text
read-only
preview-only
edit-json
apply-patch
host-call
admin
```

`apply-patch` 和 `host-call` 默认不授予。

Agent 的允许不能放松应用/系统策略;Agent 的拒绝可以进一步限制该 Agent。

## 审计

所有与权限相关的操作都必须记录在审计日志中。

最低字段:

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

审计日志应避免记录原始的敏感载荷;如果必须保留上下文,应记录摘要或脱敏字段。
