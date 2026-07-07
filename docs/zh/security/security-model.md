<!-- data-lang-nav --> [English](../../security/security-model.md) · 中文

# QORM 安全模型

> **目标模型 vs. 当前实现。** 本文档描述的是 QORM 的**目标**安全模型。当前 Go 运行时**已经强制执行**的部分:
> ed25519 的"运行前先验证工件"式 Bundle 签名 + 内容完整性(完整性优先于签名);OTA 更新需要一个受信任的公钥
> (`--trust`,否则拒绝);密钥吊销与实际的验证密钥绑定;本地服务器阻止跨源(CSRF/DNS-rebind)访问危险端点(`/window` eval、
> `/update`、`/mcp`);移动端原生能力由系统权限提示把关,生成的项目会根据实际使用的组件派生出 `Info.plist` / `AndroidManifest` 声明。
> **尚未实现**的部分(这里描述的目标;不要把它们当作已经生效的保证):一个独立的运行时"能力批准层"——即
> 按能力粒度的允许列表裁决、批准生命周期(吊销/过期),以及"桌面原生操作在传输层之外的把关"。桌面原生
> 操作目前并未独立批准;下文的"安全不变量"是设计意图,而非当前强制执行的要求。

QORM 支持动态 Bundle、Agent Patch、Host Capabilities、Plugin 和 Native Bridge,因此安全模型必须内建其中。

## 安全目标

- Bundle 不得绕过平台权限。
- Agent 默认不得执行危险操作。
- Host Capabilities 必须列入允许列表。
- Plugin 必须声明其权限和能力。
- 移动端的动态更新必须可验证、可回滚。

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

每一条边界都必须可验证。

## 决策边界

- Bundle 是声明式输入;它不授予权限。
- Runtime 负责执行验证、解析、打补丁和权限派发。
- Host / Platform 是底层能力的最终裁决者。
- Agent 不能自行批准其自身的危险操作。
- 自定义的 Web 客户端、Plugin 或 Bridge 只能实现传输/适配逻辑;它不能替代权限裁决层。

## 安全不变量

- 不存在任何可以绕过 Host Capabilities 直接访问底层 API 的路径。
- 任何批准都不能把一个不受支持的能力变成受支持的能力。
- 一个已签名的 Bundle 仍然不能绕过应用/系统策略。
- 一份已吊销或已过期的批准不得继续用于后续调用。
- MCP 工具不得进入渲染热路径。

## Bundle 安全

一个 Bundle 必须包含:

```text
bundleVersion
minRuntimeVersion
hash
signature
keyId
capability requirements
source manifest
```

签名、规范化、信任根、轮换和吊销的规则由 `bundle-signing.md` 管辖。

## Agent 安全

默认情况下,Agent 只能检查、验证和预览。诸如 apply、host.call、filesystem.saveFile 和 deploy 之类的写入类能力,必须经过策略以及任何必需的批准。

Agent 权限与系统权限之间的关系由 `agent/permissions.md` 和 `permission-model.md` 管辖。

## Host Capability 安全

每个 Host Capability 都必须声明:

```text
name
input schema
output schema
permissions
platform support
requires approval
```

运行时权限优先级、批准生命周期和审计规则由 `permission-model.md` 和 `host-capability-spec.md` 管辖。

## Plugin 安全

一个 Plugin 需要:

```text
manifest
permission declaration
sandbox strategy
version
signature
```

Plugin 只能收窄自身的能力;它不得放宽宿主策略所允许的范围。

## Web / 自定义客户端安全边界

- 浏览器沙箱是 Web 侧最外层的能力边界。
- Web Host Adapter 是 QORM 在浏览器内部的权限强制执行点。
- 注入的/自定义的 HttpClient 只是一个传输适配器;它不能跳过域、方法、凭证和批准检查。
- 浏览器的原生权限提示不等同于 QORM 的批准;两者都必须满足才能授予访问权限。

## 禁止行为

- Bundle 动态添加未经审查的 Native API。
- Agent 未经确认就写入文件系统。
- UI Action 直接执行 shell。
- Plugin 绕过 Host Capabilities。
- 自定义客户端充当权限裁决者。
