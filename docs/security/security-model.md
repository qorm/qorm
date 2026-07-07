# QORM Security Model

> **目标模型 vs. 当前实现。** 本文件描述 QORM 的**目标**安全模型。当前 Go 运行时**已强制**的部分:
> ed25519 「验证产物再运行」的 Bundle 签名 + 内容完整性(整性优先于签名);OTA 更新需要可信公钥
> (`--trust`,否则拒绝);密钥撤销绑定到实际验证密钥;本地服务器对危险端点(`/window` eval、
> `/update`、`/mcp`)做跨源(CSRF/DNS-rebind)拦截;移动端原生能力由系统权限弹窗把关,生成工程按
> 已用 widget 派生 `Info.plist` / `AndroidManifest` 声明。
> **尚未实现**(本文所述的目标,勿当作已生效的保证):独立的运行时「能力审批层」——即逐能力的
> 白名单裁决、approval 生命周期(撤销/过期)、以及「桌面原生 op 在传输层之外再受门控」。桌面原生
> op 目前不做独立审批;下面的「安全不变量」是设计意图,不是当前强制项。

QORM 支持动态 Bundle、Agent Patch、Host Capability、Plugin 和 Native Bridge，因此安全模型必须内置。

## 安全目标

- Bundle 不能绕过平台权限。
- Agent 默认不能执行危险操作。
- Host Capability 必须白名单化。
- Plugin 必须声明权限和能力。
- 移动端动态更新必须可校验、可回滚。

## 信任边界

```text
Source JSON
Bundle
Runtime
Host Capability
Native Bridge
Plugin
Agent
Platform
```

每个边界都必须可校验。

## 决策边界

- Bundle 是声明式输入，不授予权限。
- Runtime 负责执行校验、解析、Patch 与权限调度。
- Host / Platform 是底层能力最终裁决者。
- Agent 不能自行批准自己的危险操作。
- 自定义 Web client、Plugin 或 Bridge 只能实现 transport / adapter，不能替代权限裁决层。

## 安全不变量

- 不存在任何路径可以绕过 Host Capability 直接访问底层 API。
- 没有任何 approval 可以把 unsupported capability 变成 supported。
- 已签名 Bundle 也不能绕过 app/system policy。
- 被撤销或过期的 approval 不得继续用于未来调用。
- MCP 工具不得进入渲染热路径。

## Bundle 安全

Bundle 必须包含：

```text
bundleVersion
minRuntimeVersion
hash
signature
keyId
capability requirements
source manifest
```

签名、canonicalization、信任根、轮换与吊销规则以 `bundle-signing.md` 为准。

## Agent 安全

Agent 默认只能 inspect、validate、preview。apply、host.call、filesystem.saveFile 等写入类能力、deploy 必须经过策略和必要审批。

Agent 权限与系统权限的关系以 `agent/permissions.md` 和 `permission-model.md` 为准。

## Host Capability 安全

每个 Host Capability 必须声明：

```text
name
input schema
output schema
permissions
platform support
requires approval
```

运行时权限优先级、审批生命周期和审计规则以 `permission-model.md` 和 `host-capability-spec.md` 为准。

## Plugin 安全

Plugin 需要：

```text
manifest
permission declaration
sandbox strategy
version
signature
```

Plugin 只能收窄自身能力，不得扩大宿主策略允许范围。

## Web / Custom Client 安全边界

- Browser sandbox 是 Web 端外层能力边界。
- Web Host Adapter 是 QORM 在浏览器内的权限执行点。
- injected/custom HttpClient 只是 transport adapter，不能跳过域名、方法、凭证和审批检查。
- 浏览器原生权限提示不等于 QORM approval；两者都必须满足时才可放行。

## 禁止行为

- Bundle 动态新增未审核 Native API。
- Agent 无确认写入文件系统。
- UI Action 直接执行 shell。
- 插件绕过 Host Capability。
- 自定义 client 充当权限裁决者。